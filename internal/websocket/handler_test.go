package websocket

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/rjsadow/launchpad/internal/db"
	"github.com/rjsadow/launchpad/internal/sessions"
)

func setupTestDB(t *testing.T) *db.DB {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "test-ws-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	database, err := db.Open(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	return database
}

func setupTestApp(t *testing.T, database *db.DB) {
	t.Helper()
	app := db.Application{
		ID:             "test-app",
		Name:           "Test App",
		Description:    "A test application",
		URL:            "https://example.com",
		Icon:           "icon.png",
		Category:       "test",
		LaunchType:     db.LaunchTypeContainer,
		ContainerImage: "test-image:latest",
	}
	if err := database.CreateApp(app); err != nil {
		t.Fatalf("failed to create app: %v", err)
	}
}

func setupTestSession(t *testing.T, database *db.DB, id string, status db.SessionStatus, podIP string) {
	t.Helper()
	now := time.Now().Truncate(time.Second)
	session := db.Session{
		ID:        id,
		UserID:    "user-1",
		AppID:     "test-app",
		PodName:   "pod-" + id,
		PodIP:     podIP,
		Status:    status,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := database.CreateSession(session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	// If status differs from creating, update it
	if status != db.SessionStatusCreating {
		if podIP != "" {
			if err := database.UpdateSessionPodIPAndStatus(id, podIP, status); err != nil {
				t.Fatalf("failed to update session: %v", err)
			}
		} else {
			if err := database.UpdateSessionStatus(id, status); err != nil {
				t.Fatalf("failed to update session status: %v", err)
			}
		}
	}
}

func TestNewHandler(t *testing.T) {
	database := setupTestDB(t)
	mgr := sessions.NewManager(database)

	h := NewHandler(mgr)
	if h == nil {
		t.Fatal("NewHandler() returned nil")
	}
	if h.sessionManager != mgr {
		t.Error("NewHandler() did not set sessionManager correctly")
	}
}

func TestHandlerServeHTTP_MissingSessionID(t *testing.T) {
	database := setupTestDB(t)
	mgr := sessions.NewManager(database)
	h := NewHandler(mgr)

	tests := []struct {
		name string
		path string
	}{
		{"empty path", "/ws/sessions/"},
		{"trailing slash only", "/ws/sessions//"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rr := httptest.NewRecorder()

			h.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("got status %d, want %d", rr.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestHandlerServeHTTP_SessionNotFound(t *testing.T) {
	database := setupTestDB(t)
	setupTestApp(t, database)
	mgr := sessions.NewManager(database)
	h := NewHandler(mgr)

	req := httptest.NewRequest(http.MethodGet, "/ws/sessions/nonexistent-id", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestHandlerServeHTTP_SessionNotRunning(t *testing.T) {
	database := setupTestDB(t)
	setupTestApp(t, database)
	mgr := sessions.NewManager(database)
	h := NewHandler(mgr)

	setupTestSession(t, database, "sess-creating", db.SessionStatusCreating, "")

	req := httptest.NewRequest(http.MethodGet, "/ws/sessions/sess-creating", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandlerServeHTTP_SessionStoppedNotRunning(t *testing.T) {
	database := setupTestDB(t)
	setupTestApp(t, database)
	mgr := sessions.NewManager(database)
	h := NewHandler(mgr)

	// Create session as creating, then transition to running, then to stopped
	setupTestSession(t, database, "sess-stopped", db.SessionStatusCreating, "")
	if err := database.UpdateSessionPodIPAndStatus("sess-stopped", "10.0.0.1", db.SessionStatusRunning); err != nil {
		t.Fatalf("failed to update session to running: %v", err)
	}
	if err := database.UpdateSessionStatus("sess-stopped", db.SessionStatusStopped); err != nil {
		t.Fatalf("failed to update session to stopped: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ws/sessions/sess-stopped", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandlerServeHTTP_MissingPodIP(t *testing.T) {
	database := setupTestDB(t)
	setupTestApp(t, database)
	mgr := sessions.NewManager(database)
	h := NewHandler(mgr)

	// Create session as creating, then update to running without pod IP
	// This simulates a race condition where status is running but pod IP is empty
	setupTestSession(t, database, "sess-no-ip", db.SessionStatusCreating, "")
	if err := database.UpdateSessionStatus("sess-no-ip", db.SessionStatusRunning); err != nil {
		t.Fatalf("failed to update session status: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ws/sessions/sess-no-ip", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestHandlerServeHTTP_SessionFailedStatus(t *testing.T) {
	database := setupTestDB(t)
	setupTestApp(t, database)
	mgr := sessions.NewManager(database)
	h := NewHandler(mgr)

	setupTestSession(t, database, "sess-failed", db.SessionStatusCreating, "")
	if err := database.UpdateSessionStatus("sess-failed", db.SessionStatusFailed); err != nil {
		t.Fatalf("failed to update session to failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ws/sessions/sess-failed", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandlerServeHTTP_PathExtraction(t *testing.T) {
	database := setupTestDB(t)
	setupTestApp(t, database)
	mgr := sessions.NewManager(database)
	h := NewHandler(mgr)

	// With a valid session ID that exists but is in "creating" state,
	// we verify the path extraction works by checking we get past the
	// "missing session ID" check (400) to the status check (400 for creating).
	setupTestSession(t, database, "abc-123-def", db.SessionStatusCreating, "")

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{"simple id", "/ws/sessions/abc-123-def", http.StatusBadRequest},       // session not running
		{"with trailing slash", "/ws/sessions/abc-123-def/", http.StatusBadRequest}, // session not running
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rr := httptest.NewRecorder()

			h.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rr.Code, tt.wantStatus)
			}
		})
	}
}
