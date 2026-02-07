package middleware

import (
	"net/http"
	"slices"
)

// Role constants define the available roles in the system.
const (
	RoleAdmin     = "admin"
	RoleAppAuthor = "app-author"
	RoleUser      = "user"
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
