// Package middleware provides HTTP middleware for the Sortie server.
package middleware

import (
	"net/http"
)

// SecurityHeaders wraps an http.Handler and adds security headers to all responses.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent clickjacking - deny all framing
		w.Header().Set("X-Frame-Options", "DENY")

		// Prevent MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Enable XSS filter (legacy browsers)
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Control referrer information
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Content Security Policy
		// - default-src 'self': Only allow resources from same origin
		// - script-src 'self' 'unsafe-inline': Allow scripts from same origin + inline
		//   (VitePress docs use inline scripts for dark mode/platform detection)
		// - style-src 'self' 'unsafe-inline': Allow inline styles for UI frameworks
		// - img-src 'self' data: https:: Allow images from self, data URIs, and HTTPS sources
		// - connect-src 'self' ws: wss:: Allow API calls and WebSocket connections
		// - frame-ancestors 'none': Prevent framing (redundant with X-Frame-Options but more modern)
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' 'unsafe-inline'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: https:; "+
				"connect-src 'self' ws: wss:; "+
				"frame-ancestors 'none'")

		// Permissions Policy - disable unnecessary browser features
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		next.ServeHTTP(w, r)
	})
}

// SecureHeadersFunc wraps an http.HandlerFunc and adds security headers.
func SecureHeadersFunc(next http.HandlerFunc) http.HandlerFunc {
	return SecurityHeaders(next).ServeHTTP
}
