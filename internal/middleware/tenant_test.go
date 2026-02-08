package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/rjsadow/launchpad/internal/db"
)

func setupTestDatabase(t *testing.T) *db.DB {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "tenant-mw-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	database, err := db.Open(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	return database
}

func TestTenantMiddleware_DefaultTenant(t *testing.T) {
	database := setupTestDatabase(t)

	handler := TenantMiddleware(database)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := GetTenantFromContext(r.Context())
		if tenant == nil {
			t.Error("expected tenant in context")
			return
		}
		if tenant.ID != db.DefaultTenantID {
			t.Errorf("expected default tenant, got %q", tenant.ID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestTenantMiddleware_SpecificTenant(t *testing.T) {
	database := setupTestDatabase(t)

	// Create a test tenant
	if err := database.CreateTenant(db.Tenant{ID: "acme", Name: "Acme", Slug: "acme"}); err != nil {
		t.Fatal(err)
	}

	handler := TenantMiddleware(database)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := GetTenantFromContext(r.Context())
		if tenant == nil {
			t.Error("expected tenant in context")
			return
		}
		if tenant.ID != "acme" {
			t.Errorf("expected tenant 'acme', got %q", tenant.ID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set(TenantHeader, "acme")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestTenantMiddleware_UnknownTenant(t *testing.T) {
	database := setupTestDatabase(t)

	handler := TenantMiddleware(database)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for unknown tenant")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set(TenantHeader, "nonexistent")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown tenant, got %d", rr.Code)
	}
}

func TestGetTenantIDFromContext_NilTenant(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	id := GetTenantIDFromContext(req.Context())
	if id != db.DefaultTenantID {
		t.Errorf("expected default tenant ID, got %q", id)
	}
}

func TestHasTenantRole(t *testing.T) {
	tests := []struct {
		name     string
		roles    []string
		required []string
		want     bool
	}{
		{"tenant-admin has all", []string{"tenant-admin"}, []string{"user"}, true},
		{"matching role", []string{"app-author"}, []string{"app-author"}, true},
		{"no match", []string{"user"}, []string{"app-author"}, false},
		{"empty roles", nil, []string{"user"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasTenantRole(tt.roles, tt.required...); got != tt.want {
				t.Errorf("HasTenantRole(%v, %v) = %v, want %v", tt.roles, tt.required, got, tt.want)
			}
		})
	}
}
