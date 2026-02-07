package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/time/rate"
)

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(1, 2) // 1 req/s, burst 2

	// First two should be allowed (burst)
	if !rl.Allow("10.0.0.1") {
		t.Error("first request should be allowed")
	}
	if !rl.Allow("10.0.0.1") {
		t.Error("second request (burst) should be allowed")
	}

	// Third should be rate-limited
	if rl.Allow("10.0.0.1") {
		t.Error("third request should be rate-limited")
	}

	// Different IP should be allowed
	if !rl.Allow("10.0.0.2") {
		t.Error("request from different IP should be allowed")
	}
}

func TestRateLimiter_AllowDefault(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(10), 20)

	// Should allow many requests in burst
	for i := 0; i < 20; i++ {
		if !rl.Allow("10.0.0.1") {
			t.Errorf("request %d should be allowed within burst", i)
		}
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		xff        string
		xri        string
		remoteAddr string
		want       string
	}{
		{
			name:       "X-Forwarded-For single",
			xff:        "203.0.113.50",
			remoteAddr: "127.0.0.1:1234",
			want:       "203.0.113.50",
		},
		{
			name:       "X-Forwarded-For chain",
			xff:        "203.0.113.50, 70.41.3.18, 150.172.238.178",
			remoteAddr: "127.0.0.1:1234",
			want:       "203.0.113.50",
		},
		{
			name:       "X-Real-Ip",
			xri:        "203.0.113.50",
			remoteAddr: "127.0.0.1:1234",
			want:       "203.0.113.50",
		},
		{
			name:       "RemoteAddr with port",
			remoteAddr: "192.168.1.1:54321",
			want:       "192.168.1.1",
		},
		{
			name:       "RemoteAddr without port",
			remoteAddr: "192.168.1.1",
			want:       "192.168.1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				r.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xri != "" {
				r.Header.Set("X-Real-Ip", tt.xri)
			}
			got := clientIP(r)
			if got != tt.want {
				t.Errorf("clientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseRoute(t *testing.T) {
	h := &Handler{}

	tests := []struct {
		path      string
		wantID    string
		wantBack  string
	}{
		{"/ws/sessions/abc-123", "abc-123", "vnc"},
		{"/ws/sessions/abc-123/", "abc-123", "vnc"},
		{"/ws/guac/sessions/def-456", "def-456", "guac"},
		{"/ws/guac/sessions/def-456/", "def-456", "guac"},
		{"/ws/sessions/", "", ""},
		{"/ws/guac/sessions/", "", ""},
		{"/other/path", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			id, back := h.parseRoute(tt.path)
			if id != tt.wantID || back != tt.wantBack {
				t.Errorf("parseRoute(%q) = (%q, %q), want (%q, %q)",
					tt.path, id, back, tt.wantID, tt.wantBack)
			}
		})
	}
}

func TestHandler_ServeHTTP_RateLimited(t *testing.T) {
	rl := NewRateLimiter(1, 1) // very strict: 1 req/s, burst 1

	h := &Handler{limiter: rl}

	// First request allowed (but will fail at auth)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ws/sessions/test", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	h.ServeHTTP(w, r)
	// Should get 401 (no token), not 429
	if w.Code == http.StatusTooManyRequests {
		t.Error("first request should not be rate-limited")
	}

	// Second request should be rate-limited
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/ws/sessions/test", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	h.ServeHTTP(w, r)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("second request should be rate-limited, got %d", w.Code)
	}
}

func TestHandler_ServeHTTP_Unauthorized(t *testing.T) {
	h := &Handler{
		limiter: NewRateLimiter(100, 100),
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ws/sessions/test", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	h.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandler_ServeHTTP_BadPath(t *testing.T) {
	h := &Handler{
		limiter: NewRateLimiter(100, 100),
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ws/sessions/test?token=fake", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	h.ServeHTTP(w, r)

	// No auth provider configured, so this returns 401 not 400
	// (auth fails before route parsing)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
