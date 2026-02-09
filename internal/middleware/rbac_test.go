package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rjsadow/sortie/internal/plugins"
)

func TestHasRole_AdminAlwaysGranted(t *testing.T) {
	if !HasRole([]string{RoleAdmin}, RoleAppAuthor) {
		t.Error("admin should have access to app-author routes")
	}
	if !HasRole([]string{RoleAdmin}, RoleUser) {
		t.Error("admin should have access to user routes")
	}
	if !HasRole([]string{RoleAdmin, RoleUser}, RoleAppAuthor) {
		t.Error("admin+user should have access to app-author routes")
	}
}

func TestHasRole_ExactMatch(t *testing.T) {
	if !HasRole([]string{RoleAppAuthor}, RoleAppAuthor) {
		t.Error("app-author should match app-author requirement")
	}
	if !HasRole([]string{RoleUser}, RoleUser) {
		t.Error("user should match user requirement")
	}
}

func TestHasRole_MultipleRequired(t *testing.T) {
	if !HasRole([]string{RoleAppAuthor}, RoleAdmin, RoleAppAuthor) {
		t.Error("app-author should match when app-author is one of the required roles")
	}
	if !HasRole([]string{RoleUser}, RoleAppAuthor, RoleUser) {
		t.Error("user should match when user is one of the required roles")
	}
}

func TestHasRole_NoMatch(t *testing.T) {
	if HasRole([]string{RoleUser}, RoleAppAuthor) {
		t.Error("user should not have app-author access")
	}
	if HasRole([]string{RoleUser}, RoleAdmin) {
		t.Error("user should not have admin access")
	}
	if HasRole([]string{RoleAppAuthor}, RoleAdmin) {
		t.Error("app-author should not have admin-only access (admin is checked separately)")
	}
}

func TestHasRole_EmptyRoles(t *testing.T) {
	if HasRole(nil, RoleUser) {
		t.Error("nil roles should not match anything")
	}
	if HasRole([]string{}, RoleUser) {
		t.Error("empty roles should not match anything")
	}
}

func TestHasRole_NoRequired(t *testing.T) {
	if HasRole([]string{RoleUser}) {
		t.Error("no required roles should not match")
	}
}

func TestRequireRole_AdminAccess(t *testing.T) {
	user := &plugins.User{
		ID:       "admin-1",
		Username: "admin",
		Roles:    []string{RoleAdmin, RoleUser},
	}

	var called bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := RequireRole(RoleAdmin)(inner)

	ctx := context.WithValue(context.Background(), UserContextKey, user)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/test", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !called {
		t.Error("handler should have been called")
	}
}

func TestRequireRole_AppAuthorAccess(t *testing.T) {
	user := &plugins.User{
		ID:       "author-1",
		Username: "author",
		Roles:    []string{RoleAppAuthor, RoleUser},
	}

	var called bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := RequireRole(RoleAppAuthor)(inner)

	ctx := context.WithValue(context.Background(), UserContextKey, user)
	req := httptest.NewRequest(http.MethodPost, "/api/apps", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !called {
		t.Error("handler should have been called")
	}
}

func TestRequireRole_AdminCanAccessAppAuthorRoutes(t *testing.T) {
	user := &plugins.User{
		ID:       "admin-1",
		Username: "admin",
		Roles:    []string{RoleAdmin},
	}

	var called bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := RequireRole(RoleAppAuthor)(inner)

	ctx := context.WithValue(context.Background(), UserContextKey, user)
	req := httptest.NewRequest(http.MethodPost, "/api/apps", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d (admin should access app-author routes)", rec.Code)
	}
	if !called {
		t.Error("handler should have been called")
	}
}

func TestRequireRole_UserDeniedAppAuthorRoute(t *testing.T) {
	user := &plugins.User{
		ID:       "user-1",
		Username: "regularuser",
		Roles:    []string{RoleUser},
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for unauthorized user")
	})

	handler := RequireRole(RoleAppAuthor)(inner)

	ctx := context.WithValue(context.Background(), UserContextKey, user)
	req := httptest.NewRequest(http.MethodPost, "/api/apps", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestRequireRole_UserDeniedAdminRoute(t *testing.T) {
	user := &plugins.User{
		ID:       "user-1",
		Username: "regularuser",
		Roles:    []string{RoleUser},
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for unauthorized user")
	})

	handler := RequireRole(RoleAdmin)(inner)

	ctx := context.WithValue(context.Background(), UserContextKey, user)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/settings", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestRequireRole_NoUserInContext(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called without user")
	})

	handler := RequireRole(RoleUser)(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRequireRole_MultipleAllowedRoles(t *testing.T) {
	tests := []struct {
		name      string
		userRoles []string
		wantCode  int
	}{
		{"admin allowed", []string{RoleAdmin}, http.StatusOK},
		{"app-author allowed", []string{RoleAppAuthor}, http.StatusOK},
		{"user denied", []string{RoleUser}, http.StatusForbidden},
		{"app-author+user allowed", []string{RoleAppAuthor, RoleUser}, http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			user := &plugins.User{
				ID:       "test-1",
				Username: "test",
				Roles:    tc.userRoles,
			}

			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			handler := RequireRole(RoleAdmin, RoleAppAuthor)(inner)

			ctx := context.WithValue(context.Background(), UserContextKey, user)
			req := httptest.NewRequest(http.MethodGet, "/api/test", nil).WithContext(ctx)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantCode {
				t.Errorf("expected %d, got %d", tc.wantCode, rec.Code)
			}
		})
	}
}
