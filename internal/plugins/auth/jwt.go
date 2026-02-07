package auth

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rjsadow/launchpad/internal/db"
	"github.com/rjsadow/launchpad/internal/plugins"
	"golang.org/x/crypto/bcrypt"
)

// TokenType distinguishes access tokens from refresh tokens
type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

// Claims represents JWT claims for Launchpad tokens
type Claims struct {
	jwt.RegisteredClaims
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Roles     []string  `json:"roles"`
	TokenType TokenType `json:"token_type"`
}

// LoginResult contains the result of a successful login
type LoginResult struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresIn    int64        `json:"expires_in"`
	User         *plugins.User `json:"user"`
}

// JWTAuthProvider implements AuthProvider using JWT tokens
type JWTAuthProvider struct {
	config        map[string]string
	database      *db.DB
	jwtSecret     []byte
	accessExpiry  time.Duration
	refreshExpiry time.Duration
}

func init() {
	plugins.RegisterGlobal(plugins.PluginTypeAuth, "jwt", func() plugins.Plugin {
		return NewJWTAuthProvider()
	})
}

// NewJWTAuthProvider creates a new JWT auth provider
func NewJWTAuthProvider() *JWTAuthProvider {
	return &JWTAuthProvider{}
}

// Name returns the plugin name
func (p *JWTAuthProvider) Name() string {
	return "jwt"
}

// Type returns the plugin type
func (p *JWTAuthProvider) Type() plugins.PluginType {
	return plugins.PluginTypeAuth
}

// Version returns the plugin version
func (p *JWTAuthProvider) Version() string {
	return "1.0.0"
}

// Description returns a human-readable description
func (p *JWTAuthProvider) Description() string {
	return "JWT-based authentication provider with bcrypt password hashing"
}

// Initialize sets up the plugin with configuration
func (p *JWTAuthProvider) Initialize(ctx context.Context, config map[string]string) error {
	p.config = config

	// JWT secret is required
	secret, ok := config["jwt_secret"]
	if !ok || len(secret) < 32 {
		return fmt.Errorf("jwt_secret must be at least 32 characters")
	}
	p.jwtSecret = []byte(secret)

	// Parse access expiry (default 15 minutes)
	if expiry, ok := config["access_expiry"]; ok {
		d, err := time.ParseDuration(expiry)
		if err != nil {
			return fmt.Errorf("invalid access_expiry: %w", err)
		}
		p.accessExpiry = d
	} else {
		p.accessExpiry = 15 * time.Minute
	}

	// Parse refresh expiry (default 24 hours)
	if expiry, ok := config["refresh_expiry"]; ok {
		d, err := time.ParseDuration(expiry)
		if err != nil {
			return fmt.Errorf("invalid refresh_expiry: %w", err)
		}
		p.refreshExpiry = d
	} else {
		p.refreshExpiry = 24 * time.Hour
	}

	return nil
}

// SetDatabase sets the database connection for user lookups
func (p *JWTAuthProvider) SetDatabase(database *db.DB) {
	p.database = database
}

// Healthy returns true if the plugin is operational
func (p *JWTAuthProvider) Healthy(ctx context.Context) bool {
	return len(p.jwtSecret) > 0
}

// Close releases resources
func (p *JWTAuthProvider) Close() error {
	return nil
}

// Authenticate validates a JWT token and returns the authenticated user
func (p *JWTAuthProvider) Authenticate(ctx context.Context, tokenString string) (*plugins.AuthResult, error) {
	if tokenString == "" {
		return &plugins.AuthResult{
			Authenticated: false,
			Message:       "No token provided",
		}, nil
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
			return &plugins.AuthResult{
				Authenticated: false,
				Message:       "Token expired",
			}, nil
		}
		return &plugins.AuthResult{
			Authenticated: false,
			Message:       "Invalid token",
		}, nil
	}

	if !token.Valid {
		return &plugins.AuthResult{
			Authenticated: false,
			Message:       "Invalid token",
		}, nil
	}

	// Verify this is an access token
	if claims.TokenType != TokenTypeAccess {
		return &plugins.AuthResult{
			Authenticated: false,
			Message:       "Invalid token type",
		}, nil
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

// LoginWithCredentials authenticates a user with username and password
func (p *JWTAuthProvider) LoginWithCredentials(ctx context.Context, username, password string) (*LoginResult, error) {
	if p.database == nil {
		return nil, errors.New("database not configured")
	}

	user, err := p.database.GetUserByUsername(username)
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}
	if user == nil {
		return nil, errors.New("invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, errors.New("invalid credentials")
	}

	// Generate tokens
	accessToken, err := p.generateToken(user, TokenTypeAccess)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, err := p.generateToken(user, TokenTypeRefresh)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return &LoginResult{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
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

// RefreshAccessToken generates a new access token from a valid refresh token
func (p *JWTAuthProvider) RefreshAccessToken(ctx context.Context, refreshTokenString string) (*LoginResult, error) {
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

	// Get fresh user data from database
	user, err := p.database.GetUserByID(claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	// Generate new access token
	accessToken, err := p.generateToken(user, TokenTypeAccess)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	return &LoginResult{
		AccessToken:  accessToken,
		RefreshToken: refreshTokenString, // Return same refresh token
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

// generateToken creates a new JWT token for the user
func (p *JWTAuthProvider) generateToken(user *db.User, tokenType TokenType) (string, error) {
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
			Issuer:    "launchpad",
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

// GetUser retrieves user information by ID
func (p *JWTAuthProvider) GetUser(ctx context.Context, userID string) (*plugins.User, error) {
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
// Permissions map to roles: "admin" requires admin, "app-author" requires
// admin or app-author, all other permissions require any authenticated user.
func (p *JWTAuthProvider) HasPermission(ctx context.Context, userID, permission string) (bool, error) {
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

	// Admin has all permissions
	if slices.Contains(user.Roles, "admin") {
		return true, nil
	}

	switch permission {
	case "admin":
		return false, nil
	case "app-author":
		return slices.Contains(user.Roles, "app-author"), nil
	default:
		// Any authenticated user has basic permissions
		return true, nil
	}
}

// GetLoginURL returns the URL for initiating login
// For JWT auth, this returns an empty string since we use direct login
func (p *JWTAuthProvider) GetLoginURL(redirectURL string) string {
	return ""
}

// HandleCallback processes OAuth/OIDC callbacks
// For JWT auth, this is not used
func (p *JWTAuthProvider) HandleCallback(ctx context.Context, code, state string) (*plugins.AuthResult, error) {
	return nil, errors.New("JWT auth does not support OAuth callbacks")
}

// Logout invalidates the user's session
// For JWT, tokens are stateless; client should discard them
func (p *JWTAuthProvider) Logout(ctx context.Context, token string) error {
	// JWT tokens are stateless - nothing to invalidate server-side
	return nil
}

// HashPassword creates a bcrypt hash of the password
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// Verify interface compliance
var _ plugins.AuthProvider = (*JWTAuthProvider)(nil)
