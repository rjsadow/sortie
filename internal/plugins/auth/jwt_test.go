package auth

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rjsadow/sortie/internal/db"
)

const testSecret = "this-is-a-test-secret-that-is-at-least-32-characters-long"

// setupTestProvider creates a JWTAuthProvider with an in-memory SQLite database.
func setupTestProvider(t *testing.T) (*JWTAuthProvider, *db.DB) {
	t.Helper()

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	provider := NewJWTAuthProvider()
	err = provider.Initialize(context.Background(), map[string]string{
		"jwt_secret":     testSecret,
		"access_expiry":  "15m",
		"refresh_expiry": "24h",
	})
	if err != nil {
		t.Fatalf("failed to initialize provider: %v", err)
	}
	provider.SetDatabase(database)

	return provider, database
}

// seedTestUser creates a user in the test database and returns the user.
func seedTestUser(t *testing.T, database *db.DB, username, password string, roles []string) *db.User {
	t.Helper()

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	user := db.User{
		ID:           "test-user-" + username,
		Username:     username,
		Email:        username + "@test.com",
		DisplayName:  "Test " + username,
		PasswordHash: hash,
		Roles:        roles,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := database.CreateUser(user); err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	return &user
}

func TestNewJWTAuthProvider(t *testing.T) {
	p := NewJWTAuthProvider()
	if p == nil {
		t.Fatal("NewJWTAuthProvider returned nil")
	}
	if p.Name() != "jwt" {
		t.Errorf("expected name 'jwt', got %q", p.Name())
	}
	if p.Type() != "auth" {
		t.Errorf("expected type 'auth', got %q", p.Type())
	}
	if p.Version() == "" {
		t.Error("expected non-empty version")
	}
	if p.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestInitialize(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]string
		wantErr bool
	}{
		{
			name: "valid config",
			config: map[string]string{
				"jwt_secret": testSecret,
			},
			wantErr: false,
		},
		{
			name: "valid config with custom expiry",
			config: map[string]string{
				"jwt_secret":     testSecret,
				"access_expiry":  "30m",
				"refresh_expiry": "48h",
			},
			wantErr: false,
		},
		{
			name:    "missing secret",
			config:  map[string]string{},
			wantErr: true,
		},
		{
			name: "secret too short",
			config: map[string]string{
				"jwt_secret": "short",
			},
			wantErr: true,
		},
		{
			name: "invalid access expiry",
			config: map[string]string{
				"jwt_secret":    testSecret,
				"access_expiry": "not-a-duration",
			},
			wantErr: true,
		},
		{
			name: "invalid refresh expiry",
			config: map[string]string{
				"jwt_secret":     testSecret,
				"refresh_expiry": "bad",
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := NewJWTAuthProvider()
			err := p.Initialize(context.Background(), tc.config)
			if (err != nil) != tc.wantErr {
				t.Errorf("Initialize() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestHealthy(t *testing.T) {
	p := NewJWTAuthProvider()
	if p.Healthy(context.Background()) {
		t.Error("uninitialized provider should not be healthy")
	}

	err := p.Initialize(context.Background(), map[string]string{
		"jwt_secret": testSecret,
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if !p.Healthy(context.Background()) {
		t.Error("initialized provider should be healthy")
	}
}

func TestLoginWithCredentials(t *testing.T) {
	provider, database := setupTestProvider(t)
	defer database.Close()

	seedTestUser(t, database, "alice", "password123", []string{"user"})

	t.Run("valid credentials", func(t *testing.T) {
		result, err := provider.LoginWithCredentials(context.Background(), "alice", "password123")
		if err != nil {
			t.Fatalf("LoginWithCredentials failed: %v", err)
		}
		if result.AccessToken == "" {
			t.Error("expected non-empty access token")
		}
		if result.RefreshToken == "" {
			t.Error("expected non-empty refresh token")
		}
		if result.ExpiresIn <= 0 {
			t.Error("expected positive expires_in")
		}
		if result.User == nil {
			t.Fatal("expected non-nil user")
		}
		if result.User.Username != "alice" {
			t.Errorf("expected username 'alice', got %q", result.User.Username)
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		_, err := provider.LoginWithCredentials(context.Background(), "alice", "wrong")
		if err == nil {
			t.Error("expected error for wrong password")
		}
	})

	t.Run("nonexistent user", func(t *testing.T) {
		_, err := provider.LoginWithCredentials(context.Background(), "nobody", "password123")
		if err == nil {
			t.Error("expected error for nonexistent user")
		}
	})

	t.Run("no database", func(t *testing.T) {
		p := NewJWTAuthProvider()
		p.Initialize(context.Background(), map[string]string{"jwt_secret": testSecret})
		_, err := p.LoginWithCredentials(context.Background(), "alice", "password123")
		if err == nil {
			t.Error("expected error when database not configured")
		}
	})
}

func TestAuthenticate(t *testing.T) {
	provider, database := setupTestProvider(t)
	defer database.Close()

	seedTestUser(t, database, "bob", "securepass", []string{"user", "admin"})

	// Login to get tokens
	loginResult, err := provider.LoginWithCredentials(context.Background(), "bob", "securepass")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	t.Run("valid access token", func(t *testing.T) {
		result, err := provider.Authenticate(context.Background(), loginResult.AccessToken)
		if err != nil {
			t.Fatalf("Authenticate failed: %v", err)
		}
		if !result.Authenticated {
			t.Errorf("expected authenticated, got message: %s", result.Message)
		}
		if result.User == nil {
			t.Fatal("expected non-nil user")
		}
		if result.User.Username != "bob" {
			t.Errorf("expected username 'bob', got %q", result.User.Username)
		}
		if len(result.User.Roles) != 2 {
			t.Errorf("expected 2 roles, got %d", len(result.User.Roles))
		}
	})

	t.Run("empty token", func(t *testing.T) {
		result, err := provider.Authenticate(context.Background(), "")
		if err != nil {
			t.Fatalf("Authenticate failed: %v", err)
		}
		if result.Authenticated {
			t.Error("expected not authenticated for empty token")
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		result, err := provider.Authenticate(context.Background(), "not.a.valid.token")
		if err != nil {
			t.Fatalf("Authenticate failed: %v", err)
		}
		if result.Authenticated {
			t.Error("expected not authenticated for invalid token")
		}
	})

	t.Run("refresh token rejected as access token", func(t *testing.T) {
		result, err := provider.Authenticate(context.Background(), loginResult.RefreshToken)
		if err != nil {
			t.Fatalf("Authenticate failed: %v", err)
		}
		if result.Authenticated {
			t.Error("refresh token should not be accepted as access token")
		}
	})

	t.Run("expired token", func(t *testing.T) {
		// Create a provider with very short expiry
		shortProvider := NewJWTAuthProvider()
		shortProvider.Initialize(context.Background(), map[string]string{
			"jwt_secret":    testSecret,
			"access_expiry": "1ms",
		})
		shortProvider.SetDatabase(database)

		loginRes, err := shortProvider.LoginWithCredentials(context.Background(), "bob", "securepass")
		if err != nil {
			t.Fatalf("login failed: %v", err)
		}

		// Wait for token to expire
		time.Sleep(10 * time.Millisecond)

		result, err := shortProvider.Authenticate(context.Background(), loginRes.AccessToken)
		if err != nil {
			t.Fatalf("Authenticate failed: %v", err)
		}
		if result.Authenticated {
			t.Error("expected not authenticated for expired token")
		}
		if result.Message != "Token expired" {
			t.Errorf("expected 'Token expired' message, got %q", result.Message)
		}
	})

	t.Run("token signed with wrong key", func(t *testing.T) {
		// Create a token with a different secret
		claims := Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				Issuer:    "sortie",
			},
			UserID:    "test-user-bob",
			Username:  "bob",
			Roles:     []string{"user"},
			TokenType: TokenTypeAccess,
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenString, _ := token.SignedString([]byte("a-completely-different-secret-key-that-is-long"))

		result, err := provider.Authenticate(context.Background(), tokenString)
		if err != nil {
			t.Fatalf("Authenticate failed: %v", err)
		}
		if result.Authenticated {
			t.Error("expected not authenticated for token with wrong key")
		}
	})
}

func TestRefreshAccessToken(t *testing.T) {
	provider, database := setupTestProvider(t)
	defer database.Close()

	seedTestUser(t, database, "carol", "mypass", []string{"user"})

	loginResult, err := provider.LoginWithCredentials(context.Background(), "carol", "mypass")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	t.Run("valid refresh", func(t *testing.T) {
		result, err := provider.RefreshAccessToken(context.Background(), loginResult.RefreshToken)
		if err != nil {
			t.Fatalf("RefreshAccessToken failed: %v", err)
		}
		if result.AccessToken == "" {
			t.Error("expected non-empty access token")
		}
		if result.User == nil {
			t.Fatal("expected non-nil user")
		}
		if result.User.Username != "carol" {
			t.Errorf("expected username 'carol', got %q", result.User.Username)
		}

		// Verify new access token works
		authResult, err := provider.Authenticate(context.Background(), result.AccessToken)
		if err != nil {
			t.Fatalf("Authenticate failed: %v", err)
		}
		if !authResult.Authenticated {
			t.Error("new access token should be valid")
		}
	})

	t.Run("invalid refresh token", func(t *testing.T) {
		_, err := provider.RefreshAccessToken(context.Background(), "invalid-token")
		if err == nil {
			t.Error("expected error for invalid refresh token")
		}
	})

	t.Run("access token as refresh token", func(t *testing.T) {
		_, err := provider.RefreshAccessToken(context.Background(), loginResult.AccessToken)
		if err == nil {
			t.Error("access token should not work as refresh token")
		}
	})

	t.Run("no database", func(t *testing.T) {
		p := NewJWTAuthProvider()
		p.Initialize(context.Background(), map[string]string{"jwt_secret": testSecret})
		_, err := p.RefreshAccessToken(context.Background(), loginResult.RefreshToken)
		if err == nil {
			t.Error("expected error when database not configured")
		}
	})
}

func TestGetUser(t *testing.T) {
	provider, database := setupTestProvider(t)
	defer database.Close()

	user := seedTestUser(t, database, "dave", "pass", []string{"user"})

	t.Run("existing user", func(t *testing.T) {
		result, err := provider.GetUser(context.Background(), user.ID)
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil user")
		}
		if result.Username != "dave" {
			t.Errorf("expected username 'dave', got %q", result.Username)
		}
	})

	t.Run("nonexistent user", func(t *testing.T) {
		result, err := provider.GetUser(context.Background(), "nonexistent-id")
		if err != nil {
			t.Fatalf("GetUser failed: %v", err)
		}
		if result != nil {
			t.Error("expected nil for nonexistent user")
		}
	})

	t.Run("no database", func(t *testing.T) {
		p := NewJWTAuthProvider()
		p.Initialize(context.Background(), map[string]string{"jwt_secret": testSecret})
		_, err := p.GetUser(context.Background(), user.ID)
		if err == nil {
			t.Error("expected error when database not configured")
		}
	})
}

func TestHasPermission(t *testing.T) {
	provider, database := setupTestProvider(t)
	defer database.Close()

	admin := seedTestUser(t, database, "admin", "pass", []string{"admin", "user"})
	regular := seedTestUser(t, database, "regular", "pass", []string{"user"})

	t.Run("admin has permission", func(t *testing.T) {
		has, err := provider.HasPermission(context.Background(), admin.ID, "anything")
		if err != nil {
			t.Fatalf("HasPermission failed: %v", err)
		}
		if !has {
			t.Error("admin should have permission")
		}
	})

	t.Run("regular user lacks admin permission", func(t *testing.T) {
		has, err := provider.HasPermission(context.Background(), regular.ID, "admin")
		if err != nil {
			t.Fatalf("HasPermission failed: %v", err)
		}
		if has {
			t.Error("regular user should not have admin permission")
		}
	})

	t.Run("regular user lacks app-author permission", func(t *testing.T) {
		has, err := provider.HasPermission(context.Background(), regular.ID, "app-author")
		if err != nil {
			t.Fatalf("HasPermission failed: %v", err)
		}
		if has {
			t.Error("regular user should not have app-author permission")
		}
	})

	t.Run("regular user has basic permission", func(t *testing.T) {
		has, err := provider.HasPermission(context.Background(), regular.ID, "view")
		if err != nil {
			t.Fatalf("HasPermission failed: %v", err)
		}
		if !has {
			t.Error("regular user should have basic permissions")
		}
	})

	t.Run("app-author has app-author permission", func(t *testing.T) {
		author := seedTestUser(t, database, "author", "pass", []string{"app-author", "user"})
		has, err := provider.HasPermission(context.Background(), author.ID, "app-author")
		if err != nil {
			t.Fatalf("HasPermission failed: %v", err)
		}
		if !has {
			t.Error("app-author should have app-author permission")
		}
	})

	t.Run("nonexistent user", func(t *testing.T) {
		has, err := provider.HasPermission(context.Background(), "nonexistent", "admin")
		if err != nil {
			t.Fatalf("HasPermission failed: %v", err)
		}
		if has {
			t.Error("nonexistent user should not have permission")
		}
	})
}

func TestHashPassword(t *testing.T) {
	hash, err := HashPassword("mypassword")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if hash == "" {
		t.Error("expected non-empty hash")
	}
	if hash == "mypassword" {
		t.Error("hash should not equal plaintext password")
	}

	// Different calls should produce different hashes (due to salt)
	hash2, err := HashPassword("mypassword")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if hash == hash2 {
		t.Error("expected different hashes for same password (bcrypt uses random salt)")
	}
}

func TestLogoutAndCallbackStubs(t *testing.T) {
	provider, database := setupTestProvider(t)
	defer database.Close()

	t.Run("logout returns nil", func(t *testing.T) {
		err := provider.Logout(context.Background(), "any-token")
		if err != nil {
			t.Errorf("Logout should return nil for JWT, got: %v", err)
		}
	})

	t.Run("HandleCallback returns error", func(t *testing.T) {
		_, err := provider.HandleCallback(context.Background(), "code", "state")
		if err == nil {
			t.Error("HandleCallback should return error for JWT provider")
		}
	})

	t.Run("GetLoginURL returns empty string", func(t *testing.T) {
		url := provider.GetLoginURL("/redirect")
		if url != "" {
			t.Errorf("GetLoginURL should return empty string for JWT, got %q", url)
		}
	})
}

func TestClose(t *testing.T) {
	p := NewJWTAuthProvider()
	if err := p.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}
