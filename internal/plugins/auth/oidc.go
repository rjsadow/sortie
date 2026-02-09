package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/rjsadow/sortie/internal/db"
	"github.com/rjsadow/sortie/internal/plugins"
	"golang.org/x/oauth2"
)

// OIDCAuthProvider implements AuthProvider using OpenID Connect.
// It supports any OIDC-compliant provider (Auth0, Keycloak, Entra ID, Okta, etc.).
// After OIDC authentication, it issues local JWT tokens so the rest of the
// application works identically to password-based login.
// CSRF state tokens are stored in the database for horizontal scalability,
// so any replica can complete an OIDC callback started by another.
type OIDCAuthProvider struct {
	config       map[string]string
	database     *db.DB
	jwtSecret    []byte
	accessExpiry time.Duration
	refreshExpiry time.Duration

	provider     *oidc.Provider
	verifier     *oidc.IDTokenVerifier
	oauth2Config oauth2.Config
}

func init() {
	plugins.RegisterGlobal(plugins.PluginTypeAuth, "oidc", func() plugins.Plugin {
		return NewOIDCAuthProvider()
	})
}

// NewOIDCAuthProvider creates a new OIDC auth provider.
func NewOIDCAuthProvider() *OIDCAuthProvider {
	return &OIDCAuthProvider{
		accessExpiry:  15 * time.Minute,
		refreshExpiry: 24 * time.Hour,
	}
}

func (p *OIDCAuthProvider) Name() string        { return "oidc" }
func (p *OIDCAuthProvider) Type() plugins.PluginType { return plugins.PluginTypeAuth }
func (p *OIDCAuthProvider) Version() string      { return "1.0.0" }
func (p *OIDCAuthProvider) Description() string {
	return "OpenID Connect authentication provider (Auth0, Keycloak, Entra ID, Okta)"
}

// Initialize sets up the OIDC provider with configuration.
// Required config keys: issuer, client_id, client_secret, redirect_url, jwt_secret
// Optional: scopes (comma-separated, defaults to "openid,profile,email")
func (p *OIDCAuthProvider) Initialize(ctx context.Context, config map[string]string) error {
	p.config = config

	issuer := config["issuer"]
	if issuer == "" {
		return fmt.Errorf("oidc: issuer is required")
	}
	clientID := config["client_id"]
	if clientID == "" {
		return fmt.Errorf("oidc: client_id is required")
	}
	clientSecret := config["client_secret"]
	if clientSecret == "" {
		return fmt.Errorf("oidc: client_secret is required")
	}
	redirectURL := config["redirect_url"]
	if redirectURL == "" {
		return fmt.Errorf("oidc: redirect_url is required")
	}

	secret := config["jwt_secret"]
	if secret == "" || len(secret) < 32 {
		return fmt.Errorf("oidc: jwt_secret must be at least 32 characters")
	}
	p.jwtSecret = []byte(secret)

	// Parse expiry durations
	if expiry, ok := config["access_expiry"]; ok {
		d, err := time.ParseDuration(expiry)
		if err != nil {
			return fmt.Errorf("oidc: invalid access_expiry: %w", err)
		}
		p.accessExpiry = d
	}
	if expiry, ok := config["refresh_expiry"]; ok {
		d, err := time.ParseDuration(expiry)
		if err != nil {
			return fmt.Errorf("oidc: invalid refresh_expiry: %w", err)
		}
		p.refreshExpiry = d
	}

	// Discover OIDC provider (fetches .well-known/openid-configuration)
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return fmt.Errorf("oidc: failed to discover provider at %s: %w", issuer, err)
	}
	p.provider = provider

	p.verifier = provider.Verifier(&oidc.Config{
		ClientID: clientID,
	})

	scopes := []string{oidc.ScopeOpenID, "profile", "email"}
	if s, ok := config["scopes"]; ok && s != "" {
		scopes = strings.Split(s, ",")
		for i := range scopes {
			scopes[i] = strings.TrimSpace(scopes[i])
		}
	}

	p.oauth2Config = oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  redirectURL,
		Scopes:       scopes,
	}

	// Start cleanup goroutine for expired state entries
	go p.cleanupStates()

	return nil
}

// SetDatabase sets the database connection for user lookups/creation.
func (p *OIDCAuthProvider) SetDatabase(database *db.DB) {
	p.database = database
}

func (p *OIDCAuthProvider) Healthy(ctx context.Context) bool {
	return p.provider != nil && p.verifier != nil
}

func (p *OIDCAuthProvider) Close() error {
	return nil
}

// Authenticate validates a local JWT token (issued after OIDC callback).
func (p *OIDCAuthProvider) Authenticate(ctx context.Context, tokenString string) (*plugins.AuthResult, error) {
	if tokenString == "" {
		return &plugins.AuthResult{Authenticated: false, Message: "No token provided"}, nil
	}

	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return p.jwtSecret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return &plugins.AuthResult{Authenticated: false, Message: "Token expired"}, nil
		}
		return &plugins.AuthResult{Authenticated: false, Message: "Invalid token"}, nil
	}
	if !token.Valid {
		return &plugins.AuthResult{Authenticated: false, Message: "Invalid token"}, nil
	}
	if claims.TokenType != TokenTypeAccess {
		return &plugins.AuthResult{Authenticated: false, Message: "Invalid token type"}, nil
	}

	expiresAt := claims.ExpiresAt.Time
	return &plugins.AuthResult{
		Authenticated: true,
		User: &plugins.User{
			ID:       claims.UserID,
			Username: claims.Username,
			Roles:    claims.Roles,
		},
		Token:     tokenString,
		ExpiresAt: &expiresAt,
	}, nil
}

// GetLoginURL returns the OIDC authorization URL with a CSRF state token.
// The state token is stored in the database for multi-replica consistency.
func (p *OIDCAuthProvider) GetLoginURL(redirectURL string) string {
	state, err := generateState()
	if err != nil {
		return ""
	}

	if err := p.database.SaveOIDCState(state, redirectURL, time.Now().Add(10*time.Minute)); err != nil {
		log.Printf("oidc: failed to save state: %v", err)
		return ""
	}

	return p.oauth2Config.AuthCodeURL(state)
}

// HandleCallback exchanges the authorization code for tokens, verifies the ID token,
// creates or updates the local user, and issues local JWT tokens.
func (p *OIDCAuthProvider) HandleCallback(ctx context.Context, code, state string) (*plugins.AuthResult, error) {
	if p.database == nil {
		return nil, errors.New("database not configured")
	}

	// Validate and consume state from database (atomic load-and-delete)
	_, expiresAt, err := p.database.ConsumeOIDCState(state)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("invalid or expired state parameter")
		}
		return nil, fmt.Errorf("oidc: failed to validate state: %w", err)
	}
	if time.Now().After(expiresAt) {
		return nil, errors.New("state parameter expired")
	}

	// Exchange code for tokens
	oauth2Token, err := p.oauth2Config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("oidc: failed to exchange code: %w", err)
	}

	// Extract and verify ID token
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return nil, errors.New("oidc: no id_token in token response")
	}

	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("oidc: failed to verify id_token: %w", err)
	}

	// Extract claims from ID token
	var claims struct {
		Sub               string   `json:"sub"`
		Email             string   `json:"email"`
		EmailVerified     bool     `json:"email_verified"`
		Name              string   `json:"name"`
		PreferredUsername  string   `json:"preferred_username"`
		Groups            []string `json:"groups"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("oidc: failed to parse claims: %w", err)
	}

	// Determine username: prefer preferred_username, fall back to email, then sub
	username := claims.PreferredUsername
	if username == "" {
		username = claims.Email
	}
	if username == "" {
		username = claims.Sub
	}

	// Look up or create local user
	user, err := p.findOrCreateUser(claims.Sub, username, claims.Email, claims.Name, claims.Groups)
	if err != nil {
		return nil, fmt.Errorf("oidc: failed to find/create user: %w", err)
	}

	// Issue local JWT tokens
	accessToken, err := p.generateToken(user, TokenTypeAccess)
	if err != nil {
		return nil, fmt.Errorf("oidc: failed to generate access token: %w", err)
	}
	refreshToken, err := p.generateToken(user, TokenTypeRefresh)
	if err != nil {
		return nil, fmt.Errorf("oidc: failed to generate refresh token: %w", err)
	}

	accessExpiresAt := time.Now().Add(p.accessExpiry)
	return &plugins.AuthResult{
		Authenticated: true,
		User: &plugins.User{
			ID:       user.ID,
			Username: user.Username,
			Email:    user.Email,
			Name:     user.DisplayName,
			Roles:    user.Roles,
		},
		Token:     accessToken,
		ExpiresAt: &accessExpiresAt,
		Message:   refreshToken, // Pass refresh token in Message field for the handler to extract
	}, nil
}

// findOrCreateUser looks up a user by their OIDC subject identifier.
// If no user exists, it creates one. If the user exists, it updates profile fields.
func (p *OIDCAuthProvider) findOrCreateUser(sub, username, email, displayName string, groups []string) (*db.User, error) {
	// Try to find user by auth_provider + auth_provider_id
	user, err := p.database.GetUserByAuthProvider("oidc", sub)
	if err != nil {
		return nil, err
	}

	if user != nil {
		// Update profile if changed
		changed := false
		if email != "" && user.Email != email {
			user.Email = email
			changed = true
		}
		if displayName != "" && user.DisplayName != displayName {
			user.DisplayName = displayName
			changed = true
		}
		if changed {
			p.database.UpdateUser(*user)
		}
		return user, nil
	}

	// Check if a local user with same username exists (link accounts)
	existing, err := p.database.GetUserByUsername(username)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		// Link existing local account to SSO
		existing.AuthProvider = "oidc"
		existing.AuthProviderID = sub
		if email != "" {
			existing.Email = email
		}
		if displayName != "" {
			existing.DisplayName = displayName
		}
		if err := p.database.UpdateUser(*existing); err != nil {
			return nil, err
		}
		return existing, nil
	}

	// Create new user
	roles := []string{"user"}
	// Map OIDC groups to roles if present
	for _, g := range groups {
		switch strings.ToLower(g) {
		case "admin", "admins", "administrators":
			roles = append(roles, "admin")
		case "app-author", "app-authors", "authors":
			roles = append(roles, "app-author")
		}
	}

	newUser := db.User{
		ID:              "oidc-" + uuid.New().String(),
		Username:        username,
		Email:           email,
		DisplayName:     displayName,
		PasswordHash:    "", // No password for SSO users
		Roles:           roles,
		AuthProvider:    "oidc",
		AuthProviderID:  sub,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := p.database.CreateUser(newUser); err != nil {
		return nil, err
	}

	return &newUser, nil
}

// generateToken creates a local JWT token for the user.
func (p *OIDCAuthProvider) generateToken(user *db.User, tokenType TokenType) (string, error) {
	var expiry time.Duration
	if tokenType == TokenTypeAccess {
		expiry = p.accessExpiry
	} else {
		expiry = p.refreshExpiry
	}

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "sortie",
			Subject:   user.ID,
		},
		UserID:    user.ID,
		Username:  user.Username,
		Roles:     user.Roles,
		TokenType: tokenType,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(p.jwtSecret)
}

// RefreshAccessToken generates a new access token from a valid refresh token.
func (p *OIDCAuthProvider) RefreshAccessToken(ctx context.Context, refreshTokenString string) (*LoginResult, error) {
	if p.database == nil {
		return nil, errors.New("database not configured")
	}

	claims := &Claims{}
	token, err := jwt.ParseWithClaims(refreshTokenString, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return p.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return nil, errors.New("invalid refresh token")
	}
	if claims.TokenType != TokenTypeRefresh {
		return nil, errors.New("invalid token type")
	}

	user, err := p.database.GetUserByID(claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	accessToken, err := p.generateToken(user, TokenTypeAccess)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	return &LoginResult{
		AccessToken:  accessToken,
		RefreshToken: refreshTokenString,
		ExpiresIn:    int64(p.accessExpiry.Seconds()),
		User: &plugins.User{
			ID:       user.ID,
			Username: user.Username,
			Email:    user.Email,
			Name:     user.DisplayName,
			Roles:    user.Roles,
		},
	}, nil
}

// GetUser retrieves user information by ID.
func (p *OIDCAuthProvider) GetUser(ctx context.Context, userID string) (*plugins.User, error) {
	if p.database == nil {
		return nil, errors.New("database not configured")
	}
	user, err := p.database.GetUserByID(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, nil
	}
	return &plugins.User{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Name:     user.DisplayName,
		Roles:    user.Roles,
	}, nil
}

// HasPermission checks if a user has a specific permission.
func (p *OIDCAuthProvider) HasPermission(ctx context.Context, userID, permission string) (bool, error) {
	if p.database == nil {
		return false, errors.New("database not configured")
	}
	user, err := p.database.GetUserByID(userID)
	if err != nil {
		return false, err
	}
	if user == nil {
		return false, nil
	}

	for _, role := range user.Roles {
		if role == "admin" {
			return true, nil
		}
	}

	switch permission {
	case "admin":
		return false, nil
	case "app-author":
		for _, role := range user.Roles {
			if role == "app-author" {
				return true, nil
			}
		}
		return false, nil
	default:
		return true, nil
	}
}

// Logout is a no-op for JWT tokens (stateless).
func (p *OIDCAuthProvider) Logout(ctx context.Context, token string) error {
	return nil
}

// generateState creates a cryptographically random state string for CSRF protection.
func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// cleanupStates periodically removes expired state entries from the database.
func (p *OIDCAuthProvider) cleanupStates() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		if err := p.database.CleanupExpiredOIDCStates(); err != nil {
			log.Printf("oidc: failed to cleanup expired states: %v", err)
		}
	}
}

// Verify interface compliance
var _ plugins.AuthProvider = (*OIDCAuthProvider)(nil)
