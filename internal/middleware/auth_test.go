package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rjsadow/launchpad/internal/plugins"
)

// mockAuthProvider implements plugins.AuthProvider for testing.
type mockAuthProvider struct {
	authenticateFunc func(ctx context.Context, token string) (*plugins.AuthResult, error)
}

func (m *mockAuthProvider) Name() string                        { return "mock" }
func (m *mockAuthProvider) Type() plugins.PluginType            { return plugins.PluginTypeAuth }
func (m *mockAuthProvider) Version() string                     { return "1.0.0" }
func (m *mockAuthProvider) Description() string                 { return "mock auth" }
func (m *mockAuthProvider) Initialize(_ context.Context, _ map[string]string) error {
	return nil
}
func (m *mockAuthProvider) Healthy(_ context.Context) bool { return true }
func (m *mockAuthProvider) Close() error                   { return nil }
func (m *mockAuthProvider) Authenticate(ctx context.Context, token string) (*plugins.AuthResult, error) {
	return m.authenticateFunc(ctx, token)
}
func (m *mockAuthProvider) GetUser(_ context.Context, _ string) (*plugins.User, error) {
	return nil, nil
}
func (m *mockAuthProvider) HasPermission(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
func (m *mockAuthProvider) GetLoginURL(_ string) string { return "" }
func (m *mockAuthProvider) HandleCallback(_ context.Context, _, _ string) (*plugins.AuthResult, error) {
	return nil, nil
}
func (m *mockAuthProvider) Logout(_ context.Context, _ string) error { return nil }

func newMockProvider(user *plugins.User) *mockAuthProvider {
	return &mockAuthProvider{
		authenticateFunc: func(_ context.Context, token string) (*plugins.AuthResult, error) {
			if token == "valid-token" {
				expiresAt := time.Now().Add(15 * time.Minute)
				return &plugins.AuthResult{
					Authenticated: true,
					User:          user,
					Token:         token,
					ExpiresAt:     &expiresAt,
				}, nil
			}
			return &plugins.AuthResult{
				Authenticated: false,
				Message:       "Invalid token",
			}, nil
		},
	}
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	user := &plugins.User{
		ID:       "user-1",
		Username: "testuser",
		Roles:    []string{"user"},
	}
	provider := newMockProvider(user)

	var capturedUser *plugins.User
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUser = GetUserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := AuthMiddleware(provider)(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if capturedUser == nil {
		t.Fatal("expected user in context")
	}
	if capturedUser.Username != "testuser" {
		t.Errorf("expected username 'testuser', got %q", capturedUser.Username)
	}
}

func TestAuthMiddleware_MissingHeader(t *testing.T) {
	provider := newMockProvider(nil)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	handler := AuthMiddleware(provider)(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuthMiddleware_InvalidFormat(t *testing.T) {
	provider := newMockProvider(nil)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := AuthMiddleware(provider)(inner)

	tests := []struct {
		name     string
		header   string
		wantCode int
	}{
		{"no bearer prefix", "just-a-token", http.StatusUnauthorized},
		{"basic auth", "Basic dXNlcjpwYXNz", http.StatusUnauthorized},
		{"bearer with empty token", "Bearer ", http.StatusUnauthorized},
		{"bearer lowercase", "bearer valid-token", http.StatusOK}, // EqualFold accepts case-insensitive Bearer
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			req.Header.Set("Authorization", tc.header)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantCode {
				t.Errorf("expected %d, got %d", tc.wantCode, rec.Code)
			}
		})
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	provider := newMockProvider(nil)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	handler := AuthMiddleware(provider)(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer bad-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuthMiddleware_AuthError(t *testing.T) {
	provider := &mockAuthProvider{
		authenticateFunc: func(_ context.Context, _ string) (*plugins.AuthResult, error) {
			return nil, context.DeadlineExceeded
		},
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	handler := AuthMiddleware(provider)(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer some-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuthMiddlewareFunc(t *testing.T) {
	user := &plugins.User{
		ID:       "user-1",
		Username: "testuser",
		Roles:    []string{"user"},
	}
	provider := newMockProvider(user)

	var called bool
	inner := func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}

	handler := AuthMiddlewareFunc(provider, inner)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()

	handler(rec, req)

	if !called {
		t.Error("inner handler was not called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestOptionalAuthMiddleware_WithToken(t *testing.T) {
	user := &plugins.User{
		ID:       "user-1",
		Username: "testuser",
		Roles:    []string{"user"},
	}
	provider := newMockProvider(user)

	var capturedUser *plugins.User
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUser = GetUserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := OptionalAuthMiddleware(provider)(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if capturedUser == nil {
		t.Fatal("expected user in context when valid token provided")
	}
	if capturedUser.Username != "testuser" {
		t.Errorf("expected username 'testuser', got %q", capturedUser.Username)
	}
}

func TestOptionalAuthMiddleware_WithoutToken(t *testing.T) {
	provider := newMockProvider(nil)

	var capturedUser *plugins.User
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUser = GetUserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := OptionalAuthMiddleware(provider)(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if capturedUser != nil {
		t.Error("expected nil user when no token provided")
	}
}

func TestOptionalAuthMiddleware_InvalidToken(t *testing.T) {
	provider := newMockProvider(nil)

	var called bool
	var capturedUser *plugins.User
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		capturedUser = GetUserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := OptionalAuthMiddleware(provider)(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer bad-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("inner handler should still be called with invalid token in optional mode")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if capturedUser != nil {
		t.Error("expected nil user for invalid token in optional mode")
	}
}

func TestOptionalAuthMiddleware_MalformedHeader(t *testing.T) {
	provider := newMockProvider(nil)

	var called bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := OptionalAuthMiddleware(provider)(inner)

	tests := []struct {
		name   string
		header string
	}{
		{"basic auth", "Basic dXNlcjpwYXNz"},
		{"no space", "Bearertoken"},
		{"empty bearer", "Bearer "},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			called = false
			req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			req.Header.Set("Authorization", tc.header)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if !called {
				t.Error("inner handler should be called in optional mode")
			}
			if rec.Code != http.StatusOK {
				t.Errorf("expected 200, got %d", rec.Code)
			}
		})
	}
}

func TestGetUserFromContext_NoUser(t *testing.T) {
	ctx := context.Background()
	user := GetUserFromContext(ctx)
	if user != nil {
		t.Error("expected nil user from empty context")
	}
}

func TestGetUserFromContext_WrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), UserContextKey, "not-a-user")
	user := GetUserFromContext(ctx)
	if user != nil {
		t.Error("expected nil user when context value is wrong type")
	}
}

func TestGetUserFromContext_ValidUser(t *testing.T) {
	expected := &plugins.User{
		ID:       "user-1",
		Username: "testuser",
	}
	ctx := context.WithValue(context.Background(), UserContextKey, expected)
	user := GetUserFromContext(ctx)
	if user == nil {
		t.Fatal("expected non-nil user")
	}
	if user.ID != expected.ID {
		t.Errorf("expected user ID %q, got %q", expected.ID, user.ID)
	}
}
