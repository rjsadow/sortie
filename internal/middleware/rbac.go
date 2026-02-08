package middleware

import (
	"net/http"
	"slices"

	"github.com/rjsadow/launchpad/internal/plugins"
)

// Role constants define the available roles in the system.
const (
	RoleAdmin     = "admin"
	RoleAppAuthor = "app-author"
	RoleUser      = "user"

	// Tenant-scoped roles
	RoleTenantAdmin = "tenant-admin" // Admin within a tenant (manage apps, users, settings for this tenant)
)

// RequireRole returns middleware that checks if the authenticated user
// has at least one of the specified roles. The user must already be
// authenticated (use AuthMiddleware first in the chain).
// Admin role always grants access regardless of required roles.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := GetUserFromContext(r.Context())
			if user == nil {
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			if !HasRole(user.Roles, roles...) {
				http.Error(w, "Insufficient permissions", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireTenantRole returns middleware that checks if the authenticated user
// has at least one of the specified tenant-scoped roles within the current tenant.
// System admins and users with global admin role always pass.
// Tenant-admin role grants access for any tenant-scoped role check.
func RequireTenantRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := GetUserFromContext(r.Context())
			if user == nil {
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			// System admins bypass tenant role checks
			if HasRole(user.Roles, RoleAdmin) {
				next.ServeHTTP(w, r)
				return
			}

			// Check tenant-scoped roles from user metadata
			tenant := GetTenantFromContext(r.Context())
			if tenant == nil {
				http.Error(w, "Tenant context required", http.StatusBadRequest)
				return
			}

			// Get tenant roles from metadata
			tenantRoles := getTenantRolesFromUser(user)
			if !HasTenantRole(tenantRoles, roles...) {
				http.Error(w, "Insufficient tenant permissions", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// HasRole checks if the user's roles include at least one of the required roles.
// Admin role always returns true (admins have access to everything).
func HasRole(userRoles []string, required ...string) bool {
	if slices.Contains(userRoles, RoleAdmin) {
		return true
	}
	for _, r := range required {
		if slices.Contains(userRoles, r) {
			return true
		}
	}
	return false
}

// HasTenantRole checks if the user's tenant-scoped roles include at least one of the required roles.
// tenant-admin role always returns true within the tenant.
func HasTenantRole(tenantRoles []string, required ...string) bool {
	if slices.Contains(tenantRoles, RoleTenantAdmin) {
		return true
	}
	for _, r := range required {
		if slices.Contains(tenantRoles, r) {
			return true
		}
	}
	return false
}

// getTenantRolesFromUser extracts tenant roles from the plugins.User metadata.
// The JWT auth provider stores tenant_roles as a comma-separated string in metadata.
func getTenantRolesFromUser(user *plugins.User) []string {
	if user == nil || user.Metadata == nil {
		return nil
	}
	// Tenant roles are stored in the Groups field by convention
	return user.Groups
}
