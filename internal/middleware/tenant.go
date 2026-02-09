package middleware

import (
	"context"
	"net/http"

	"github.com/rjsadow/sortie/internal/db"
)

const (
	// TenantContextKey is the key used to store the tenant in the request context
	TenantContextKey contextKey = "tenant"

	// TenantHeader is the HTTP header used to specify the tenant
	TenantHeader = "X-Tenant-ID"
)

// TenantMiddleware resolves the tenant from the X-Tenant-ID header (or defaults to "default")
// and injects the tenant into the request context. Returns 404 if the specified tenant doesn't exist.
func TenantMiddleware(database *db.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenantID := r.Header.Get(TenantHeader)
			if tenantID == "" {
				tenantID = db.DefaultTenantID
			}

			tenant, err := database.GetTenant(tenantID)
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			if tenant == nil {
				// Try by slug
				tenant, err = database.GetTenantBySlug(tenantID)
				if err != nil {
					http.Error(w, "Internal server error", http.StatusInternalServerError)
					return
				}
			}
			if tenant == nil {
				http.Error(w, "Tenant not found", http.StatusNotFound)
				return
			}

			ctx := context.WithValue(r.Context(), TenantContextKey, tenant)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetTenantFromContext retrieves the tenant from the request context
func GetTenantFromContext(ctx context.Context) *db.Tenant {
	tenant, ok := ctx.Value(TenantContextKey).(*db.Tenant)
	if !ok {
		return nil
	}
	return tenant
}

// GetTenantIDFromContext returns the tenant ID from context, or the default tenant ID
func GetTenantIDFromContext(ctx context.Context) string {
	tenant := GetTenantFromContext(ctx)
	if tenant == nil {
		return db.DefaultTenantID
	}
	return tenant.ID
}

// RequireTenantAccess returns middleware that verifies the authenticated user
// belongs to the tenant in the request context. System admins bypass this check.
func RequireTenantAccess() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := GetUserFromContext(r.Context())
			if user == nil {
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			// System admins can access any tenant
			if HasRole(user.Roles, RoleAdmin) {
				next.ServeHTTP(w, r)
				return
			}

			tenant := GetTenantFromContext(r.Context())
			if tenant == nil {
				http.Error(w, "Tenant context required", http.StatusBadRequest)
				return
			}

			// Check user belongs to this tenant
			if user.Metadata != nil && user.Metadata["tenant_id"] == tenant.ID {
				next.ServeHTTP(w, r)
				return
			}

			http.Error(w, "Access denied: user does not belong to this tenant", http.StatusForbidden)
		})
	}
}
