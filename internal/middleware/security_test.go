package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	// Create a simple handler to wrap
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Wrap with security middleware
	handler := SecurityHeaders(innerHandler)

	// Create test request
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	// Execute
	handler.ServeHTTP(rec, req)

	// Verify response status
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	// Test security headers are set
	tests := []struct {
		header   string
		expected string
	}{
		{"X-Frame-Options", "DENY"},
		{"X-Content-Type-Options", "nosniff"},
		{"X-XSS-Protection", "1; mode=block"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
		{"Permissions-Policy", "geolocation=(), microphone=(), camera=()"},
	}

	for _, tc := range tests {
		t.Run(tc.header, func(t *testing.T) {
			got := rec.Header().Get(tc.header)
			if got != tc.expected {
				t.Errorf("Header %s: expected %q, got %q", tc.header, tc.expected, got)
			}
		})
	}

	// Test CSP header contains expected directives
	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Error("Content-Security-Policy header not set")
	}

	cspDirectives := []string{
		"default-src 'self'",
		"script-src 'self' 'unsafe-inline'",
		"frame-ancestors 'none'",
	}

	for _, directive := range cspDirectives {
		if !containsSubstring(csp, directive) {
			t.Errorf("CSP missing directive: %s", directive)
		}
	}
}

func TestSecureHeadersFunc(t *testing.T) {
	// Create a simple handler func to wrap
	innerHandler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}

	// Wrap with security middleware
	handler := SecureHeadersFunc(innerHandler)

	// Create test request
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	// Execute
	handler(rec, req)

	// Verify X-Frame-Options is set
	if rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("SecureHeadersFunc did not set security headers")
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstringHelper(s, substr))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
