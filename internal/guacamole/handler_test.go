package guacamole

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rjsadow/sortie/internal/db"
	"github.com/rjsadow/sortie/internal/db/dbtest"
	"github.com/rjsadow/sortie/internal/sessions"
)

func setupTestDB(t *testing.T) *db.DB {
	t.Helper()
	return dbtest.NewTestDB(t)
}

func setupWindowsApp(t *testing.T, database *db.DB) {
	t.Helper()
	app := db.Application{
		ID:             "win-app",
		Name:           "Windows App",
		Description:    "A Windows test application",
		URL:            "https://example.com",
		Icon:           "icon.png",
		Category:       "test",
		LaunchType:     db.LaunchTypeContainer,
		ContainerImage: "windows-test:latest",
		OsType:         "windows",
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
		AppID:     "win-app",
		PodName:   "pod-" + id,
		PodIP:     podIP,
		Status:    status,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := database.CreateSession(session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
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

func TestNewGuacHandler(t *testing.T) {
	database := setupTestDB(t)
	mgr := sessions.NewManager(database)

	h := NewHandler(mgr)
	if h == nil {
		t.Fatal("NewHandler() returned nil")
	}
	if h.sessionManager != mgr {
		t.Error("NewHandler() did not set sessionManager correctly")
	}
	if h.registry == nil {
		t.Error("NewHandler() did not initialize registry")
	}
}

func TestGuacHandler_MissingSessionID(t *testing.T) {
	database := setupTestDB(t)
	mgr := sessions.NewManager(database)
	h := NewHandler(mgr)

	tests := []struct {
		name string
		path string
	}{
		{"empty path", "/ws/guac/sessions/"},
		{"trailing slash only", "/ws/guac/sessions//"},
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

func TestGuacHandler_SessionNotFound(t *testing.T) {
	database := setupTestDB(t)
	setupWindowsApp(t, database)
	mgr := sessions.NewManager(database)
	h := NewHandler(mgr)

	req := httptest.NewRequest(http.MethodGet, "/ws/guac/sessions/nonexistent-id", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestGuacHandler_SessionNotRunning(t *testing.T) {
	database := setupTestDB(t)
	setupWindowsApp(t, database)
	mgr := sessions.NewManager(database)
	h := NewHandler(mgr)

	setupTestSession(t, database, "guac-creating", db.SessionStatusCreating, "")

	req := httptest.NewRequest(http.MethodGet, "/ws/guac/sessions/guac-creating", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestGuacHandler_MissingPodIP(t *testing.T) {
	database := setupTestDB(t)
	setupWindowsApp(t, database)
	mgr := sessions.NewManager(database)
	h := NewHandler(mgr)

	setupTestSession(t, database, "guac-no-ip", db.SessionStatusCreating, "")
	if err := database.UpdateSessionStatus("guac-no-ip", db.SessionStatusRunning); err != nil {
		t.Fatalf("failed to update session status: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ws/guac/sessions/guac-no-ip", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestUpgraderConfig(t *testing.T) {
	// Verify the upgrader is configured with the guacamole subprotocol
	if len(upgrader.Subprotocols) != 1 || upgrader.Subprotocols[0] != "guacamole" {
		t.Errorf("upgrader.Subprotocols = %v, want [guacamole]", upgrader.Subprotocols)
	}

	if upgrader.ReadBufferSize != 4096 {
		t.Errorf("upgrader.ReadBufferSize = %d, want 4096", upgrader.ReadBufferSize)
	}

	if upgrader.WriteBufferSize != 4096 {
		t.Errorf("upgrader.WriteBufferSize = %d, want 4096", upgrader.WriteBufferSize)
	}
}
