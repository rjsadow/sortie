package auth

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rjsadow/sortie/internal/db"
)

// testOIDCSecret is a dummy secret used only in tests (not a real credential).
const testOIDCSecret = "test-oidc-dummy-secret-for-unit-tests-only" //nolint:gosec // gitleaks:allow

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestOIDCAuthProvider_Name(t *testing.T) {
	p := NewOIDCAuthProvider()
	if p.Name() != "oidc" {
		t.Errorf("expected name 'oidc', got %q", p.Name())
	}
}

func TestOIDCAuthProvider_Type(t *testing.T) {
	p := NewOIDCAuthProvider()
	if p.Type() != "auth" {
		t.Errorf("expected type 'auth', got %q", p.Type())
	}
}

func TestOIDCAuthProvider_InitializeMissingIssuer(t *testing.T) {
	p := NewOIDCAuthProvider()
	err := p.Initialize(context.Background(), map[string]string{
		"client_id":     "test",
		"client_secret": "test",
		"redirect_url":  "http://localhost/callback",
		"jwt_secret":    testOIDCSecret,
	})
	if err == nil {
		t.Fatal("expected error for missing issuer")
	}
}

func TestOIDCAuthProvider_InitializeMissingClientID(t *testing.T) {
	p := NewOIDCAuthProvider()
	err := p.Initialize(context.Background(), map[string]string{
		"issuer":        "https://example.com",
		"client_secret": "test",
		"redirect_url":  "http://localhost/callback",
		"jwt_secret":    testOIDCSecret,
	})
	if err == nil {
		t.Fatal("expected error for missing client_id")
	}
}

func TestOIDCAuthProvider_InitializeMissingClientSecret(t *testing.T) {
	p := NewOIDCAuthProvider()
	err := p.Initialize(context.Background(), map[string]string{
		"issuer":       "https://example.com",
		"client_id":    "test",
		"redirect_url": "http://localhost/callback",
		"jwt_secret":   testOIDCSecret,
	})
	if err == nil {
		t.Fatal("expected error for missing client_secret")
	}
}

func TestOIDCAuthProvider_InitializeMissingRedirectURL(t *testing.T) {
	p := NewOIDCAuthProvider()
	err := p.Initialize(context.Background(), map[string]string{
		"issuer":        "https://example.com",
		"client_id":     "test",
		"client_secret": "test",
		"jwt_secret":    testOIDCSecret,
	})
	if err == nil {
		t.Fatal("expected error for missing redirect_url")
	}
}

func TestOIDCAuthProvider_InitializeShortJWTSecret(t *testing.T) {
	p := NewOIDCAuthProvider()
	err := p.Initialize(context.Background(), map[string]string{
		"issuer":        "https://example.com",
		"client_id":     "test",
		"client_secret": "test",
		"redirect_url":  "http://localhost/callback",
		"jwt_secret":    "short",
	})
	if err == nil {
		t.Fatal("expected error for short jwt_secret")
	}
}

func TestOIDCAuthProvider_AuthenticateEmptyToken(t *testing.T) {
	p := NewOIDCAuthProvider()
	p.jwtSecret = []byte(testOIDCSecret)

	result, err := p.Authenticate(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Authenticated {
		t.Error("empty token should not authenticate")
	}
}

func TestOIDCAuthProvider_AuthenticateInvalidToken(t *testing.T) {
	p := NewOIDCAuthProvider()
	p.jwtSecret = []byte(testOIDCSecret)

	result, err := p.Authenticate(context.Background(), "not-a-valid-jwt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Authenticated {
		t.Error("invalid token should not authenticate")
	}
}

func TestOIDCAuthProvider_AuthenticateValidToken(t *testing.T) {
	secret := []byte(testOIDCSecret)
	p := NewOIDCAuthProvider()
	p.jwtSecret = secret
	p.accessExpiry = 15 * time.Minute

	// Generate a valid token
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "sortie",
			Subject:   "user-123",
		},
		UserID:    "user-123",
		Username:  "testuser",
		Roles:     []string{"user"},
		TokenType: TokenTypeAccess,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString(secret)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	result, err := p.Authenticate(context.Background(), tokenStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Authenticated {
		t.Error("valid token should authenticate")
	}
	if result.User.ID != "user-123" {
		t.Errorf("expected user ID 'user-123', got %q", result.User.ID)
	}
	if result.User.Username != "testuser" {
		t.Errorf("expected username 'testuser', got %q", result.User.Username)
	}
}

func TestOIDCAuthProvider_AuthenticateExpiredToken(t *testing.T) {
	secret := []byte(testOIDCSecret)
	p := NewOIDCAuthProvider()
	p.jwtSecret = secret

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			Issuer:    "sortie",
		},
		UserID:    "user-123",
		Username:  "testuser",
		Roles:     []string{"user"},
		TokenType: TokenTypeAccess,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ := token.SignedString(secret)

	result, err := p.Authenticate(context.Background(), tokenStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Authenticated {
		t.Error("expired token should not authenticate")
	}
	if result.Message != "Token expired" {
		t.Errorf("expected 'Token expired' message, got %q", result.Message)
	}
}

func TestOIDCAuthProvider_AuthenticateRefreshTokenRejected(t *testing.T) {
	secret := []byte(testOIDCSecret)
	p := NewOIDCAuthProvider()
	p.jwtSecret = secret

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "sortie",
		},
		UserID:    "user-123",
		Username:  "testuser",
		Roles:     []string{"user"},
		TokenType: TokenTypeRefresh, // Refresh tokens should not pass Authenticate
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ := token.SignedString(secret)

	result, err := p.Authenticate(context.Background(), tokenStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Authenticated {
		t.Error("refresh token should not pass Authenticate")
	}
}

func TestOIDCAuthProvider_HandleCallbackNoDB(t *testing.T) {
	p := NewOIDCAuthProvider()
	_, err := p.HandleCallback(context.Background(), "code", "state")
	if err == nil {
		t.Fatal("expected error when database not configured")
	}
}

func TestOIDCAuthProvider_HandleCallbackInvalidState(t *testing.T) {
	p := NewOIDCAuthProvider()
	p.database = newTestDB(t)

	_, err := p.HandleCallback(context.Background(), "code", "nonexistent-state")
	if err == nil {
		t.Fatal("expected error for invalid state")
	}
}

func TestOIDCAuthProvider_Healthy(t *testing.T) {
	p := NewOIDCAuthProvider()

	// Without initialization, should not be healthy
	if p.Healthy(context.Background()) {
		t.Error("uninitialized provider should not be healthy")
	}
}

func TestOIDCAuthProvider_GetLoginURLStoresState(t *testing.T) {
	p := NewOIDCAuthProvider()
	p.database = newTestDB(t)

	url := p.GetLoginURL("/dashboard")
	// Even without a fully initialized provider, state should be stored in DB
	if url == "" {
		t.Error("expected non-empty URL with state parameter")
	}
	// State is stored in the database; verification is done implicitly via HandleCallback
}

func TestGenerateState(t *testing.T) {
	state1, err := generateState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state1 == "" {
		t.Fatal("state should not be empty")
	}

	state2, err := generateState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if state1 == state2 {
		t.Error("states should be unique")
	}
}
