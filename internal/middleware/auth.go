package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/rjsadow/launchpad/internal/plugins"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	// UserContextKey is the key used to store the authenticated user in the request context
	UserContextKey contextKey = "user"
)

// AuthMiddleware creates middleware that validates JWT tokens from the Authorization header
func AuthMiddleware(authProvider plugins.AuthProvider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}

			// Expect "Bearer <token>" format
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
				return
			}

			token := parts[1]
			if token == "" {
				http.Error(w, "Token required", http.StatusUnauthorized)
				return
			}

			// Authenticate the token
			result, err := authProvider.Authenticate(r.Context(), token)
			if err != nil {
				http.Error(w, "Authentication failed", http.StatusUnauthorized)
				return
			}

			if !result.Authenticated {
				msg := "Unauthorized"
				if result.Message != "" {
					msg = result.Message
				}
				http.Error(w, msg, http.StatusUnauthorized)
				return
			}

			// Add user to request context
			ctx := context.WithValue(r.Context(), UserContextKey, result.User)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AuthMiddlewareFunc creates middleware for use with http.HandlerFunc
func AuthMiddlewareFunc(authProvider plugins.AuthProvider, next http.HandlerFunc) http.HandlerFunc {
	return AuthMiddleware(authProvider)(next).ServeHTTP
}

// GetUserFromContext retrieves the authenticated user from the request context
func GetUserFromContext(ctx context.Context) *plugins.User {
	user, ok := ctx.Value(UserContextKey).(*plugins.User)
	if !ok {
		return nil
	}
	return user
}

// OptionalAuthMiddleware creates middleware that extracts user if token is present, but doesn't require it
func OptionalAuthMiddleware(authProvider plugins.AuthProvider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				next.ServeHTTP(w, r)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				next.ServeHTTP(w, r)
				return
			}

			token := parts[1]
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}

			result, err := authProvider.Authenticate(r.Context(), token)
			if err != nil || !result.Authenticated {
				next.ServeHTTP(w, r)
				return
			}

			ctx := context.WithValue(r.Context(), UserContextKey, result.User)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
