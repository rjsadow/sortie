package sessions

import (
	"context"
	"testing"
	"time"

	"github.com/rjsadow/launchpad/internal/db"
)

// newTestDB creates an in-memory SQLite database for testing.
func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// seedContainerApp inserts a container-type app and returns it.
func seedContainerApp(t *testing.T, database *db.DB, id, name, image string) db.Application {
	t.Helper()
	app := db.Application{
		ID:             id,
		Name:           name,
		Description:    "test app",
		URL:            "",
		Icon:           "icon.png",
		Category:       "test",
		LaunchType:     db.LaunchTypeContainer,
		OsType:         "linux",
		ContainerImage: image,
	}
	if err := database.CreateApp(app); err != nil {
		t.Fatalf("failed to seed app: %v", err)
	}
	return app
}

// --- NewManager / NewManagerWithConfig ---

func TestNewManager(t *testing.T) {
	database := newTestDB(t)
	m := NewManager(database)

	if m.sessionTimeout != DefaultSessionTimeout {
		t.Errorf("sessionTimeout = %v, want %v", m.sessionTimeout, DefaultSessionTimeout)
	}
	if m.cleanupInterval != DefaultCleanupInterval {
		t.Errorf("cleanupInterval = %v, want %v", m.cleanupInterval, DefaultCleanupInterval)
	}
	if m.podReadyTimeout != DefaultPodReadyTimeout {
		t.Errorf("podReadyTimeout = %v, want %v", m.podReadyTimeout, DefaultPodReadyTimeout)
	}
	if m.sessions == nil {
		t.Error("sessions map should be initialized")
	}
}

func TestNewManagerWithConfig(t *testing.T) {
	database := newTestDB(t)

	t.Run("custom values", func(t *testing.T) {
		cfg := ManagerConfig{
			SessionTimeout:  30 * time.Minute,
			CleanupInterval: 1 * time.Minute,
			PodReadyTimeout: 5 * time.Minute,
		}
		m := NewManagerWithConfig(database, cfg)
		if m.sessionTimeout != 30*time.Minute {
			t.Errorf("sessionTimeout = %v, want 30m", m.sessionTimeout)
		}
		if m.cleanupInterval != 1*time.Minute {
			t.Errorf("cleanupInterval = %v, want 1m", m.cleanupInterval)
		}
		if m.podReadyTimeout != 5*time.Minute {
			t.Errorf("podReadyTimeout = %v, want 5m", m.podReadyTimeout)
		}
	})

	t.Run("zero values get defaults", func(t *testing.T) {
		m := NewManagerWithConfig(database, ManagerConfig{})
		if m.sessionTimeout != DefaultSessionTimeout {
			t.Errorf("sessionTimeout = %v, want default %v", m.sessionTimeout, DefaultSessionTimeout)
		}
		if m.cleanupInterval != DefaultCleanupInterval {
			t.Errorf("cleanupInterval = %v, want default %v", m.cleanupInterval, DefaultCleanupInterval)
		}
		if m.podReadyTimeout != DefaultPodReadyTimeout {
			t.Errorf("podReadyTimeout = %v, want default %v", m.podReadyTimeout, DefaultPodReadyTimeout)
		}
	})
}

func TestStartStop(t *testing.T) {
	database := newTestDB(t)
	m := NewManagerWithConfig(database, ManagerConfig{
		CleanupInterval: 100 * time.Millisecond,
	})
	m.Start()
	// Give cleanup loop a chance to run
	time.Sleep(150 * time.Millisecond)
	m.Stop()
	// Verify Stop doesn't panic on second call prevention (channel already closed)
}

// --- GetSession ---

func TestGetSession_CacheHit(t *testing.T) {
	database := newTestDB(t)
	m := NewManager(database)

	session := &db.Session{
		ID:     "cached-session",
		UserID: "user1",
		AppID:  "app1",
		Status: db.SessionStatusRunning,
	}
	m.mu.Lock()
	m.sessions["cached-session"] = session
	m.mu.Unlock()

	got, err := m.GetSession(context.Background(), "cached-session")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if got.ID != "cached-session" {
		t.Errorf("GetSession() ID = %q, want %q", got.ID, "cached-session")
	}
}

func TestGetSession_DBFallback(t *testing.T) {
	database := newTestDB(t)
	m := NewManager(database)

	now := time.Now()
	seedContainerApp(t, database, "app1", "Test App", "test:latest")
	err := database.CreateSession(db.Session{
		ID:        "db-session",
		UserID:    "user1",
		AppID:     "app1",
		PodName:   "pod-1",
		Status:    db.SessionStatusRunning,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}

	got, err := m.GetSession(context.Background(), "db-session")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetSession() returned nil, expected session from DB")
	}
	if got.ID != "db-session" {
		t.Errorf("GetSession() ID = %q, want %q", got.ID, "db-session")
	}
}

func TestGetSession_NotFound(t *testing.T) {
	database := newTestDB(t)
	m := NewManager(database)

	got, err := m.GetSession(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if got != nil {
		t.Errorf("GetSession() = %v, want nil for nonexistent session", got)
	}
}

// --- ListSessions ---

func TestListSessions(t *testing.T) {
	database := newTestDB(t)
	m := NewManager(database)

	seedContainerApp(t, database, "app1", "Test App", "test:latest")
	now := time.Now()

	// Create sessions with different statuses
	for _, s := range []db.Session{
		{ID: "s1", UserID: "u1", AppID: "app1", PodName: "p1", Status: db.SessionStatusRunning, CreatedAt: now, UpdatedAt: now},
		{ID: "s2", UserID: "u2", AppID: "app1", PodName: "p2", Status: db.SessionStatusCreating, CreatedAt: now, UpdatedAt: now},
		{ID: "s3", UserID: "u1", AppID: "app1", PodName: "p3", Status: db.SessionStatusFailed, CreatedAt: now, UpdatedAt: now},
	} {
		if err := database.CreateSession(s); err != nil {
			t.Fatalf("CreateSession error = %v", err)
		}
	}

	sessions, err := m.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	// ListSessions filters out 'terminated' and 'failed' statuses
	// s3 has status 'failed' so should be excluded
	if len(sessions) != 2 {
		t.Errorf("ListSessions() returned %d sessions, want 2", len(sessions))
	}
}

func TestListSessions_Empty(t *testing.T) {
	database := newTestDB(t)
	m := NewManager(database)

	sessions, err := m.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("ListSessions() returned %d sessions, want 0", len(sessions))
	}
}

// --- ListSessionsByUser ---

func TestListSessionsByUser(t *testing.T) {
	database := newTestDB(t)
	m := NewManager(database)

	seedContainerApp(t, database, "app1", "Test App", "test:latest")
	now := time.Now()

	for _, s := range []db.Session{
		{ID: "s1", UserID: "user-a", AppID: "app1", PodName: "p1", Status: db.SessionStatusRunning, CreatedAt: now, UpdatedAt: now},
		{ID: "s2", UserID: "user-b", AppID: "app1", PodName: "p2", Status: db.SessionStatusRunning, CreatedAt: now, UpdatedAt: now},
		{ID: "s3", UserID: "user-a", AppID: "app1", PodName: "p3", Status: db.SessionStatusCreating, CreatedAt: now, UpdatedAt: now},
	} {
		if err := database.CreateSession(s); err != nil {
			t.Fatalf("CreateSession error = %v", err)
		}
	}

	sessions, err := m.ListSessionsByUser(context.Background(), "user-a")
	if err != nil {
		t.Fatalf("ListSessionsByUser() error = %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("ListSessionsByUser() returned %d sessions, want 2", len(sessions))
	}

	sessions, err = m.ListSessionsByUser(context.Background(), "user-c")
	if err != nil {
		t.Fatalf("ListSessionsByUser() error = %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("ListSessionsByUser(user-c) returned %d sessions, want 0", len(sessions))
	}
}

// --- URL generation methods ---

func TestGetSessionWebSocketURL(t *testing.T) {
	database := newTestDB(t)
	m := NewManager(database)

	tests := []struct {
		name    string
		session *db.Session
		want    string
	}{
		{
			name:    "running session with IP",
			session: &db.Session{ID: "sess-1", PodIP: "10.0.0.1", Status: db.SessionStatusRunning},
			want:    "/ws/sessions/sess-1",
		},
		{
			name:    "no pod IP",
			session: &db.Session{ID: "sess-2", PodIP: "", Status: db.SessionStatusRunning},
			want:    "",
		},
		{
			name:    "not running",
			session: &db.Session{ID: "sess-3", PodIP: "10.0.0.1", Status: db.SessionStatusCreating},
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.GetSessionWebSocketURL(tt.session)
			if got != tt.want {
				t.Errorf("GetSessionWebSocketURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetSessionProxyURL(t *testing.T) {
	database := newTestDB(t)
	m := NewManager(database)

	tests := []struct {
		name    string
		session *db.Session
		want    string
	}{
		{
			name:    "running session with IP",
			session: &db.Session{ID: "sess-1", PodIP: "10.0.0.1", Status: db.SessionStatusRunning},
			want:    "/api/sessions/sess-1/proxy/",
		},
		{
			name:    "no pod IP",
			session: &db.Session{ID: "sess-2", PodIP: "", Status: db.SessionStatusRunning},
			want:    "",
		},
		{
			name:    "not running",
			session: &db.Session{ID: "sess-3", PodIP: "10.0.0.1", Status: db.SessionStatusStopped},
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.GetSessionProxyURL(tt.session)
			if got != tt.want {
				t.Errorf("GetSessionProxyURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetSessionGuacWebSocketURL(t *testing.T) {
	database := newTestDB(t)
	m := NewManager(database)

	tests := []struct {
		name    string
		session *db.Session
		want    string
	}{
		{
			name:    "running session with IP",
			session: &db.Session{ID: "sess-1", PodIP: "10.0.0.1", Status: db.SessionStatusRunning},
			want:    "/ws/guac/sessions/sess-1",
		},
		{
			name:    "no pod IP",
			session: &db.Session{ID: "sess-2", PodIP: "", Status: db.SessionStatusRunning},
			want:    "",
		},
		{
			name:    "not running",
			session: &db.Session{ID: "sess-3", PodIP: "10.0.0.1", Status: db.SessionStatusCreating},
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.GetSessionGuacWebSocketURL(tt.session)
			if got != tt.want {
				t.Errorf("GetSessionGuacWebSocketURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetPodWebSocketEndpoint(t *testing.T) {
	database := newTestDB(t)
	m := NewManager(database)

	// Seed apps for different image types
	seedContainerApp(t, database, "jlesage-app", "JLesage App", "jlesage/firefox")
	seedContainerApp(t, database, "regular-app", "Regular App", "ubuntu-vnc:latest")

	tests := []struct {
		name    string
		session *db.Session
		want    string
	}{
		{
			name:    "jlesage image uses port 5800 with websockify",
			session: &db.Session{ID: "s1", AppID: "jlesage-app", PodIP: "10.0.0.1"},
			want:    "ws://10.0.0.1:5800/websockify",
		},
		{
			name:    "regular image uses port 6080",
			session: &db.Session{ID: "s2", AppID: "regular-app", PodIP: "10.0.0.2"},
			want:    "ws://10.0.0.2:6080",
		},
		{
			name:    "empty pod IP returns empty",
			session: &db.Session{ID: "s3", AppID: "regular-app", PodIP: ""},
			want:    "",
		},
		{
			name:    "unknown app falls back to port 6080",
			session: &db.Session{ID: "s4", AppID: "nonexistent", PodIP: "10.0.0.3"},
			want:    "ws://10.0.0.3:6080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.GetPodWebSocketEndpoint(tt.session)
			if got != tt.want {
				t.Errorf("GetPodWebSocketEndpoint() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetPodProxyEndpoint(t *testing.T) {
	database := newTestDB(t)
	m := NewManager(database)

	// Seed apps with different port configs
	app8080 := db.Application{
		ID: "web-8080", Name: "Web 8080", Description: "test", URL: "", Icon: "i",
		Category: "test", LaunchType: db.LaunchTypeWebProxy, ContainerImage: "web:latest",
		ContainerPort: 8080,
	}
	app8443 := db.Application{
		ID: "web-8443", Name: "Web 8443", Description: "test", URL: "", Icon: "i",
		Category: "test", LaunchType: db.LaunchTypeWebProxy, ContainerImage: "code:latest",
		ContainerPort: 8443,
	}
	app443 := db.Application{
		ID: "web-443", Name: "Web 443", Description: "test", URL: "", Icon: "i",
		Category: "test", LaunchType: db.LaunchTypeWebProxy, ContainerImage: "nginx:latest",
		ContainerPort: 443,
	}
	appNoPort := db.Application{
		ID: "web-noport", Name: "Web No Port", Description: "test", URL: "", Icon: "i",
		Category: "test", LaunchType: db.LaunchTypeWebProxy, ContainerImage: "app:latest",
	}
	for _, a := range []db.Application{app8080, app8443, app443, appNoPort} {
		if err := database.CreateApp(a); err != nil {
			t.Fatalf("CreateApp error = %v", err)
		}
	}

	tests := []struct {
		name    string
		session *db.Session
		want    string
	}{
		{
			name:    "port 8080 uses http",
			session: &db.Session{AppID: "web-8080", PodIP: "10.0.0.1"},
			want:    "http://10.0.0.1:8080",
		},
		{
			name:    "port 8443 uses https",
			session: &db.Session{AppID: "web-8443", PodIP: "10.0.0.2"},
			want:    "https://10.0.0.2:8443",
		},
		{
			name:    "port 443 uses https",
			session: &db.Session{AppID: "web-443", PodIP: "10.0.0.3"},
			want:    "https://10.0.0.3:443",
		},
		{
			name:    "no port configured uses default 8080",
			session: &db.Session{AppID: "web-noport", PodIP: "10.0.0.4"},
			want:    "http://10.0.0.4:8080",
		},
		{
			name:    "empty pod IP returns empty",
			session: &db.Session{AppID: "web-8080", PodIP: ""},
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.GetPodProxyEndpoint(tt.session)
			if got != tt.want {
				t.Errorf("GetPodProxyEndpoint() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- IsWindowsApp ---

func TestIsWindowsApp(t *testing.T) {
	database := newTestDB(t)
	m := NewManager(database)

	// Seed a linux and windows app
	linuxApp := db.Application{
		ID: "linux-app", Name: "Linux", Description: "test", URL: "", Icon: "i",
		Category: "test", LaunchType: db.LaunchTypeContainer, OsType: "linux",
		ContainerImage: "ubuntu:latest",
	}
	winApp := db.Application{
		ID: "win-app", Name: "Windows", Description: "test", URL: "", Icon: "i",
		Category: "test", LaunchType: db.LaunchTypeContainer, OsType: "windows",
		ContainerImage: "windows:latest",
	}
	for _, a := range []db.Application{linuxApp, winApp} {
		if err := database.CreateApp(a); err != nil {
			t.Fatalf("CreateApp error = %v", err)
		}
	}

	tests := []struct {
		appID string
		want  bool
	}{
		{"win-app", true},
		{"linux-app", false},
		{"nonexistent", false},
	}

	for _, tt := range tests {
		t.Run(tt.appID, func(t *testing.T) {
			got := m.IsWindowsApp(tt.appID)
			if got != tt.want {
				t.Errorf("IsWindowsApp(%q) = %v, want %v", tt.appID, got, tt.want)
			}
		})
	}
}

// --- CreateSession error paths (before k8s calls) ---

func TestCreateSession_AppNotFound(t *testing.T) {
	database := newTestDB(t)
	m := NewManager(database)

	_, err := m.CreateSession(context.Background(), &CreateSessionRequest{
		AppID:  "nonexistent",
		UserID: "user1",
	})
	if err == nil {
		t.Error("CreateSession() expected error for nonexistent app, got nil")
	}
}

func TestCreateSession_InvalidLaunchType(t *testing.T) {
	database := newTestDB(t)
	m := NewManager(database)

	// Create a URL-type app (not container or web_proxy)
	urlApp := db.Application{
		ID: "url-app", Name: "URL App", Description: "test", URL: "https://example.com",
		Icon: "i", Category: "test", LaunchType: db.LaunchTypeURL,
	}
	if err := database.CreateApp(urlApp); err != nil {
		t.Fatalf("CreateApp error = %v", err)
	}

	_, err := m.CreateSession(context.Background(), &CreateSessionRequest{
		AppID:  "url-app",
		UserID: "user1",
	})
	if err == nil {
		t.Error("CreateSession() expected error for URL launch type, got nil")
	}
}

func TestCreateSession_NoContainerImage(t *testing.T) {
	database := newTestDB(t)
	m := NewManager(database)

	noImageApp := db.Application{
		ID: "no-image", Name: "No Image", Description: "test", URL: "",
		Icon: "i", Category: "test", LaunchType: db.LaunchTypeContainer,
		ContainerImage: "",
	}
	if err := database.CreateApp(noImageApp); err != nil {
		t.Fatalf("CreateApp error = %v", err)
	}

	_, err := m.CreateSession(context.Background(), &CreateSessionRequest{
		AppID:  "no-image",
		UserID: "user1",
	})
	if err == nil {
		t.Error("CreateSession() expected error for missing container image, got nil")
	}
}

// --- StopSession error paths ---

func TestStopSession_NotFound(t *testing.T) {
	database := newTestDB(t)
	m := NewManager(database)

	err := m.StopSession(context.Background(), "nonexistent")
	if err == nil {
		t.Error("StopSession() expected error for nonexistent session, got nil")
	}
}

func TestStopSession_InvalidTransition(t *testing.T) {
	database := newTestDB(t)
	m := NewManager(database)

	// Put a session in "creating" state in cache - can't transition to "stopped"
	m.mu.Lock()
	m.sessions["s1"] = &db.Session{
		ID:     "s1",
		Status: db.SessionStatusCreating,
	}
	m.mu.Unlock()

	err := m.StopSession(context.Background(), "s1")
	if err == nil {
		t.Error("StopSession() expected error for invalid transition creating->stopped, got nil")
	}
}

// --- RestartSession error paths ---

func TestRestartSession_NotFound(t *testing.T) {
	database := newTestDB(t)
	m := NewManager(database)

	_, err := m.RestartSession(context.Background(), "nonexistent")
	if err == nil {
		t.Error("RestartSession() expected error for nonexistent session, got nil")
	}
}

func TestRestartSession_InvalidTransition(t *testing.T) {
	database := newTestDB(t)
	m := NewManager(database)

	// Put a session in "running" state - can't restart
	m.mu.Lock()
	m.sessions["s1"] = &db.Session{
		ID:     "s1",
		Status: db.SessionStatusRunning,
	}
	m.mu.Unlock()

	_, err := m.RestartSession(context.Background(), "s1")
	if err == nil {
		t.Error("RestartSession() expected error for invalid transition running->creating, got nil")
	}
}

// --- TerminateSession / ExpireSession error paths ---

func TestTerminateSession_NotFound(t *testing.T) {
	database := newTestDB(t)
	m := NewManager(database)

	err := m.TerminateSession(context.Background(), "nonexistent")
	if err == nil {
		t.Error("TerminateSession() expected error for nonexistent session, got nil")
	}
}

func TestExpireSession_NotFound(t *testing.T) {
	database := newTestDB(t)
	m := NewManager(database)

	err := m.ExpireSession(context.Background(), "nonexistent")
	if err == nil {
		t.Error("ExpireSession() expected error for nonexistent session, got nil")
	}
}

func TestTerminateSession_AlreadyTerminal(t *testing.T) {
	database := newTestDB(t)
	m := NewManager(database)

	// A session already in expired state should return nil (early return)
	m.mu.Lock()
	m.sessions["s1"] = &db.Session{
		ID:     "s1",
		Status: db.SessionStatusExpired,
	}
	m.mu.Unlock()

	err := m.TerminateSession(context.Background(), "s1")
	if err != nil {
		t.Errorf("TerminateSession() on already-expired session should return nil, got %v", err)
	}
}

func TestExpireSession_AlreadyFailed(t *testing.T) {
	database := newTestDB(t)
	m := NewManager(database)

	m.mu.Lock()
	m.sessions["s1"] = &db.Session{
		ID:     "s1",
		Status: db.SessionStatusFailed,
	}
	m.mu.Unlock()

	err := m.ExpireSession(context.Background(), "s1")
	if err != nil {
		t.Errorf("ExpireSession() on already-failed session should return nil, got %v", err)
	}
}
