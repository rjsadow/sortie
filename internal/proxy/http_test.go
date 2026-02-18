package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rjsadow/sortie/internal/db"
	"github.com/rjsadow/sortie/internal/db/dbtest"
	"github.com/rjsadow/sortie/internal/sessions"
)

func TestExtractSessionID(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "valid session path",
			path: "/api/sessions/abc-123/proxy/some/path",
			want: "abc-123",
		},
		{
			name: "valid session path with no trailing path",
			path: "/api/sessions/abc-123/proxy",
			want: "abc-123",
		},
		{
			name: "valid session path with proxy trailing slash",
			path: "/api/sessions/abc-123/proxy/",
			want: "abc-123",
		},
		{
			name: "uuid session id",
			path: "/api/sessions/550e8400-e29b-41d4-a716-446655440000/proxy/index.html",
			want: "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name: "missing proxy segment",
			path: "/api/sessions/abc-123/other",
			want: "",
		},
		{
			name: "empty path",
			path: "",
			want: "",
		},
		{
			name: "no session id",
			path: "/api/sessions//proxy/path",
			want: "",
		},
		{
			name: "only prefix",
			path: "/api/sessions/",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSessionID(tt.path)
			if got != tt.want {
				t.Errorf("extractSessionID(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestStripProxyPrefix(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		sessionID string
		want      string
	}{
		{
			name:      "strips prefix with trailing path",
			path:      "/api/sessions/abc-123/proxy/some/path",
			sessionID: "abc-123",
			want:      "/some/path",
		},
		{
			name:      "strips prefix with no trailing path",
			path:      "/api/sessions/abc-123/proxy",
			sessionID: "abc-123",
			want:      "/",
		},
		{
			name:      "strips prefix with trailing slash only",
			path:      "/api/sessions/abc-123/proxy/",
			sessionID: "abc-123",
			want:      "/",
		},
		{
			name:      "preserves deep paths",
			path:      "/api/sessions/abc-123/proxy/api/v1/resource",
			sessionID: "abc-123",
			want:      "/api/v1/resource",
		},
		{
			name:      "non-matching prefix returns path as-is with slash",
			path:      "/other/path",
			sessionID: "abc-123",
			want:      "/other/path",
		},
		{
			name:      "empty path returns slash",
			path:      "",
			sessionID: "abc-123",
			want:      "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripProxyPrefix(tt.path, tt.sessionID)
			if got != tt.want {
				t.Errorf("stripProxyPrefix(%q, %q) = %q, want %q", tt.path, tt.sessionID, got, tt.want)
			}
		})
	}
}

func TestIsWebSocketRequest(t *testing.T) {
	tests := []struct {
		name       string
		connection string
		upgrade    string
		want       bool
	}{
		{
			name:       "valid websocket upgrade",
			connection: "Upgrade",
			upgrade:    "websocket",
			want:       true,
		},
		{
			name:       "case insensitive headers",
			connection: "UPGRADE",
			upgrade:    "WEBSOCKET",
			want:       true,
		},
		{
			name:       "connection with multiple values",
			connection: "keep-alive, Upgrade",
			upgrade:    "websocket",
			want:       true,
		},
		{
			name:       "missing upgrade header",
			connection: "Upgrade",
			upgrade:    "",
			want:       false,
		},
		{
			name:       "missing connection header",
			connection: "",
			upgrade:    "websocket",
			want:       false,
		},
		{
			name:       "wrong upgrade value",
			connection: "Upgrade",
			upgrade:    "h2c",
			want:       false,
		},
		{
			name:       "no headers",
			connection: "",
			upgrade:    "",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.connection != "" {
				req.Header.Set("Connection", tt.connection)
			}
			if tt.upgrade != "" {
				req.Header.Set("Upgrade", tt.upgrade)
			}

			got := isWebSocketRequest(req)
			if got != tt.want {
				t.Errorf("isWebSocketRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

// setupTestDBAndManager creates a test database and session manager for integration tests.
func setupTestDBAndManager(t *testing.T) (*db.DB, *sessions.Manager) {
	t.Helper()
	database := dbtest.NewTestDB(t)
	mgr := sessions.NewManager(database)
	return database, mgr
}


func TestServeHTTP_InvalidSessionPath(t *testing.T) {
	_, mgr := setupTestDBAndManager(t)
	proxy := NewHTTPProxy(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/sessions//other/path", nil)
	rr := httptest.NewRecorder()

	proxy.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestServeHTTP_SessionNotFound(t *testing.T) {
	_, mgr := setupTestDBAndManager(t)
	proxy := NewHTTPProxy(mgr)

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/nonexistent/proxy/path", nil)
	rr := httptest.NewRecorder()

	proxy.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestServeHTTP_SessionNotRunning(t *testing.T) {
	database, mgr := setupTestDBAndManager(t)

	// Create a session in "creating" status
	app := db.Application{
		ID:             "app-creating",
		Name:           "Test App",
		Description:    "test",
		URL:            "https://example.com",
		Icon:           "icon.png",
		Category:       "test",
		LaunchType:     db.LaunchTypeWebProxy,
		ContainerImage: "nginx:latest",
		ContainerPort:  8080,
	}
	if err := database.CreateApp(app); err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	now := time.Now()
	session := db.Session{
		ID:        "sess-creating",
		UserID:    "user-1",
		AppID:     "app-creating",
		PodName:   "pod-creating",
		PodIP:     "10.0.0.1",
		Status:    db.SessionStatusCreating,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := database.CreateSession(session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	proxy := NewHTTPProxy(mgr)
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/sess-creating/proxy/path", nil)
	rr := httptest.NewRecorder()

	proxy.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, rr.Code)
	}
}

func TestServeHTTP_SessionNoPodIP(t *testing.T) {
	database, mgr := setupTestDBAndManager(t)

	app := db.Application{
		ID:             "app-noip",
		Name:           "Test App",
		Description:    "test",
		URL:            "https://example.com",
		Icon:           "icon.png",
		Category:       "test",
		LaunchType:     db.LaunchTypeWebProxy,
		ContainerImage: "nginx:latest",
		ContainerPort:  8080,
	}
	if err := database.CreateApp(app); err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	now := time.Now()
	session := db.Session{
		ID:        "sess-noip",
		UserID:    "user-1",
		AppID:     "app-noip",
		PodName:   "pod-noip",
		PodIP:     "", // No pod IP
		Status:    db.SessionStatusRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := database.CreateSession(session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	proxy := NewHTTPProxy(mgr)
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/sess-noip/proxy/path", nil)
	rr := httptest.NewRecorder()

	proxy.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, rr.Code)
	}
}

func TestServeHTTP_ProxiesRequest(t *testing.T) {
	// Start a test backend server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend-Path", r.URL.Path)
		w.Header().Set("X-Backend-Query", r.URL.RawQuery)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "hello from backend")
	}))
	defer backend.Close()

	// Extract host:port from the backend URL (e.g., "127.0.0.1:12345")
	// The backend URL is http://127.0.0.1:PORT
	backendHost := backend.Listener.Addr().String()

	database, mgr := setupTestDBAndManager(t)

	// Create app with the backend's port
	app := db.Application{
		ID:             "app-proxy",
		Name:           "Proxy Test App",
		Description:    "test",
		URL:            "https://example.com",
		Icon:           "icon.png",
		Category:       "test",
		LaunchType:     db.LaunchTypeWebProxy,
		ContainerImage: "nginx:latest",
		ContainerPort:  backend.Listener.Addr().(*net.TCPAddr).Port,
	}
	if err := database.CreateApp(app); err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	// Parse the backend host to get IP
	host, _, _ := net.SplitHostPort(backendHost)

	now := time.Now()
	session := db.Session{
		ID:        "sess-proxy",
		UserID:    "user-1",
		AppID:     "app-proxy",
		PodName:   "pod-proxy",
		PodIP:     host,
		Status:    db.SessionStatusRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := database.CreateSession(session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	proxy := NewHTTPProxy(mgr)

	t.Run("proxies GET request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/sessions/sess-proxy/proxy/some/path?key=value", nil)
		req.Header.Set("X-Forwarded-For", "192.168.1.1")
		rr := httptest.NewRecorder()

		proxy.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d; body: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		body := rr.Body.String()
		if body != "hello from backend" {
			t.Errorf("unexpected body: %q", body)
		}
	})

	t.Run("strips authorization header", func(t *testing.T) {
		var receivedAuth string
		backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
		}))
		defer backend2.Close()

		// Create another app for this sub-test with the new backend
		app2 := db.Application{
			ID:             "app-auth",
			Name:           "Auth Test App",
			Description:    "test",
			URL:            "https://example.com",
			Icon:           "icon.png",
			Category:       "test",
			LaunchType:     db.LaunchTypeWebProxy,
			ContainerImage: "nginx:latest",
			ContainerPort:  backend2.Listener.Addr().(*net.TCPAddr).Port,
		}
		if err := database.CreateApp(app2); err != nil {
			t.Fatalf("failed to create app: %v", err)
		}

		host2, _, _ := net.SplitHostPort(backend2.Listener.Addr().String())
		session2 := db.Session{
			ID:        "sess-auth",
			UserID:    "user-1",
			AppID:     "app-auth",
			PodName:   "pod-auth",
			PodIP:     host2,
			Status:    db.SessionStatusRunning,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := database.CreateSession(session2); err != nil {
			t.Fatalf("failed to create session: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/sessions/sess-auth/proxy/", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		rr := httptest.NewRecorder()

		proxy.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d; body: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		if receivedAuth != "" {
			t.Errorf("expected Authorization header to be stripped, got %q", receivedAuth)
		}
	})

	t.Run("sets X-Real-IP from X-Forwarded-For", func(t *testing.T) {
		var receivedRealIP string
		backend3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedRealIP = r.Header.Get("X-Real-IP")
			w.WriteHeader(http.StatusOK)
		}))
		defer backend3.Close()

		app3 := db.Application{
			ID:             "app-realip",
			Name:           "RealIP Test",
			Description:    "test",
			URL:            "https://example.com",
			Icon:           "icon.png",
			Category:       "test",
			LaunchType:     db.LaunchTypeWebProxy,
			ContainerImage: "nginx:latest",
			ContainerPort:  backend3.Listener.Addr().(*net.TCPAddr).Port,
		}
		if err := database.CreateApp(app3); err != nil {
			t.Fatalf("failed to create app: %v", err)
		}

		host3, _, _ := net.SplitHostPort(backend3.Listener.Addr().String())
		session3 := db.Session{
			ID:        "sess-realip",
			UserID:    "user-1",
			AppID:     "app-realip",
			PodName:   "pod-realip",
			PodIP:     host3,
			Status:    db.SessionStatusRunning,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := database.CreateSession(session3); err != nil {
			t.Fatalf("failed to create session: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/sessions/sess-realip/proxy/", nil)
		req.Header.Set("X-Forwarded-For", "203.0.113.50, 70.41.3.18")
		rr := httptest.NewRecorder()

		proxy.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
		if receivedRealIP != "203.0.113.50" {
			t.Errorf("expected X-Real-IP = %q, got %q", "203.0.113.50", receivedRealIP)
		}
	})

	t.Run("sets X-Real-IP from RemoteAddr when no X-Forwarded-For", func(t *testing.T) {
		var receivedRealIP string
		backend4 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedRealIP = r.Header.Get("X-Real-IP")
			w.WriteHeader(http.StatusOK)
		}))
		defer backend4.Close()

		app4 := db.Application{
			ID:             "app-remoteaddr",
			Name:           "RemoteAddr Test",
			Description:    "test",
			URL:            "https://example.com",
			Icon:           "icon.png",
			Category:       "test",
			LaunchType:     db.LaunchTypeWebProxy,
			ContainerImage: "nginx:latest",
			ContainerPort:  backend4.Listener.Addr().(*net.TCPAddr).Port,
		}
		if err := database.CreateApp(app4); err != nil {
			t.Fatalf("failed to create app: %v", err)
		}

		host4, _, _ := net.SplitHostPort(backend4.Listener.Addr().String())
		session4 := db.Session{
			ID:        "sess-remoteaddr",
			UserID:    "user-1",
			AppID:     "app-remoteaddr",
			PodName:   "pod-remoteaddr",
			PodIP:     host4,
			Status:    db.SessionStatusRunning,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := database.CreateSession(session4); err != nil {
			t.Fatalf("failed to create session: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/sessions/sess-remoteaddr/proxy/", nil)
		// httptest.NewRequest sets RemoteAddr to "192.0.2.1:1234"
		rr := httptest.NewRecorder()

		proxy.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
		if receivedRealIP != "192.0.2.1" {
			t.Errorf("expected X-Real-IP = %q, got %q", "192.0.2.1", receivedRealIP)
		}
	})
}

func TestServeHTTP_RedirectRewriting(t *testing.T) {
	// Backend that issues a redirect
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/login")
		w.WriteHeader(http.StatusFound)
	}))
	defer backend.Close()

	database, mgr := setupTestDBAndManager(t)

	app := db.Application{
		ID:             "app-redirect",
		Name:           "Redirect Test",
		Description:    "test",
		URL:            "https://example.com",
		Icon:           "icon.png",
		Category:       "test",
		LaunchType:     db.LaunchTypeWebProxy,
		ContainerImage: "nginx:latest",
		ContainerPort:  backend.Listener.Addr().(*net.TCPAddr).Port,
	}
	if err := database.CreateApp(app); err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	host, _, _ := net.SplitHostPort(backend.Listener.Addr().String())
	session := db.Session{
		ID:        "sess-redirect",
		UserID:    "user-1",
		AppID:     "app-redirect",
		PodName:   "pod-redirect",
		PodIP:     host,
		Status:    db.SessionStatusRunning,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.CreateSession(session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	proxy := NewHTTPProxy(mgr)
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/sess-redirect/proxy/", nil)
	rr := httptest.NewRecorder()

	proxy.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Errorf("expected status %d, got %d", http.StatusFound, rr.Code)
	}

	location := rr.Header().Get("Location")
	expected := "/api/sessions/sess-redirect/proxy/login"
	if location != expected {
		t.Errorf("expected Location = %q, got %q", expected, location)
	}
}

func TestServeHTTP_BackendError(t *testing.T) {
	// Start a backend that immediately closes the connection
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Trigger an error by hijacking and immediately closing
		hj, ok := w.(http.Hijacker)
		if !ok {
			return
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer backend.Close()

	database, mgr := setupTestDBAndManager(t)

	app := db.Application{
		ID:             "app-error",
		Name:           "Error Test",
		Description:    "test",
		URL:            "https://example.com",
		Icon:           "icon.png",
		Category:       "test",
		LaunchType:     db.LaunchTypeWebProxy,
		ContainerImage: "nginx:latest",
		ContainerPort:  backend.Listener.Addr().(*net.TCPAddr).Port,
	}
	if err := database.CreateApp(app); err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	host, _, _ := net.SplitHostPort(backend.Listener.Addr().String())
	session := db.Session{
		ID:        "sess-error",
		UserID:    "user-1",
		AppID:     "app-error",
		PodName:   "pod-error",
		PodIP:     host,
		Status:    db.SessionStatusRunning,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.CreateSession(session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	proxy := NewHTTPProxy(mgr)
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/sess-error/proxy/", nil)
	rr := httptest.NewRecorder()

	proxy.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("expected status %d, got %d; body: %s", http.StatusBadGateway, rr.Code, rr.Body.String())
	}
}

func TestServeHTTP_PreservesQueryString(t *testing.T) {
	var receivedQuery string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	database, mgr := setupTestDBAndManager(t)

	app := db.Application{
		ID:             "app-query",
		Name:           "Query Test",
		Description:    "test",
		URL:            "https://example.com",
		Icon:           "icon.png",
		Category:       "test",
		LaunchType:     db.LaunchTypeWebProxy,
		ContainerImage: "nginx:latest",
		ContainerPort:  backend.Listener.Addr().(*net.TCPAddr).Port,
	}
	if err := database.CreateApp(app); err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	host, _, _ := net.SplitHostPort(backend.Listener.Addr().String())
	session := db.Session{
		ID:        "sess-query",
		UserID:    "user-1",
		AppID:     "app-query",
		PodName:   "pod-query",
		PodIP:     host,
		Status:    db.SessionStatusRunning,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.CreateSession(session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	proxy := NewHTTPProxy(mgr)
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/sess-query/proxy/path?foo=bar&baz=qux", nil)
	rr := httptest.NewRecorder()

	proxy.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	if receivedQuery != "foo=bar&baz=qux" {
		t.Errorf("expected query = %q, got %q", "foo=bar&baz=qux", receivedQuery)
	}
}

func TestServeHTTP_SetsForwardedHeaders(t *testing.T) {
	var receivedForwardedHost, receivedForwardedProto string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedForwardedHost = r.Header.Get("X-Forwarded-Host")
		receivedForwardedProto = r.Header.Get("X-Forwarded-Proto")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	database, mgr := setupTestDBAndManager(t)

	app := db.Application{
		ID:             "app-fwd",
		Name:           "Forward Test",
		Description:    "test",
		URL:            "https://example.com",
		Icon:           "icon.png",
		Category:       "test",
		LaunchType:     db.LaunchTypeWebProxy,
		ContainerImage: "nginx:latest",
		ContainerPort:  backend.Listener.Addr().(*net.TCPAddr).Port,
	}
	if err := database.CreateApp(app); err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	host, _, _ := net.SplitHostPort(backend.Listener.Addr().String())
	session := db.Session{
		ID:        "sess-fwd",
		UserID:    "user-1",
		AppID:     "app-fwd",
		PodName:   "pod-fwd",
		PodIP:     host,
		Status:    db.SessionStatusRunning,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.CreateSession(session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	proxy := NewHTTPProxy(mgr)
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/sess-fwd/proxy/", nil)
	req.Host = "sortie.example.com"
	rr := httptest.NewRecorder()

	proxy.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if receivedForwardedHost != "sortie.example.com" {
		t.Errorf("expected X-Forwarded-Host = %q, got %q", "sortie.example.com", receivedForwardedHost)
	}
	if receivedForwardedProto != "http" {
		t.Errorf("expected X-Forwarded-Proto = %q, got %q", "http", receivedForwardedProto)
	}
}

func TestServeHTTP_StoppedSession(t *testing.T) {
	database, mgr := setupTestDBAndManager(t)

	app := db.Application{
		ID:             "app-stopped",
		Name:           "Stopped Test",
		Description:    "test",
		URL:            "https://example.com",
		Icon:           "icon.png",
		Category:       "test",
		LaunchType:     db.LaunchTypeWebProxy,
		ContainerImage: "nginx:latest",
		ContainerPort:  8080,
	}
	if err := database.CreateApp(app); err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	now := time.Now()
	session := db.Session{
		ID:        "sess-stopped",
		UserID:    "user-1",
		AppID:     "app-stopped",
		PodName:   "pod-stopped",
		PodIP:     "10.0.0.1",
		Status:    db.SessionStatusStopped,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := database.CreateSession(session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	proxy := NewHTTPProxy(mgr)
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/sess-stopped/proxy/", nil)
	rr := httptest.NewRecorder()

	proxy.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, rr.Code)
	}
}

func TestNewHTTPProxy(t *testing.T) {
	_, mgr := setupTestDBAndManager(t)
	proxy := NewHTTPProxy(mgr)

	if proxy == nil {
		t.Fatal("NewHTTPProxy returned nil")
	}
	if proxy.sessionManager != mgr {
		t.Error("session manager not set correctly")
	}
}

func TestServeHTTP_ContextCancellation(t *testing.T) {
	_, mgr := setupTestDBAndManager(t)
	proxy := NewHTTPProxy(mgr)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/test-id/proxy/", nil)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	proxy.ServeHTTP(rr, req)

	// With a cancelled context, GetSession should fail
	// The exact behavior depends on whether the DB query respects context cancellation
	// We just ensure it doesn't panic and returns some error status
	if rr.Code == http.StatusOK {
		t.Error("expected non-OK status with cancelled context")
	}
}
