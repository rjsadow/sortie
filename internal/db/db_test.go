package db

import (
	"os"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	database, err := Open(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	return database
}

func TestSessionCRUD(t *testing.T) {
	db := setupTestDB(t)

	// Create an app for the session
	app := Application{
		ID:          "test-app",
		Name:        "Test App",
		Description: "A test application",
		URL:         "https://example.com",
		Icon:        "icon.png",
		Category:    "test",
		LaunchType:  LaunchTypeContainer,
	}
	if err := db.CreateApp(app); err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	now := time.Now().Truncate(time.Second)

	t.Run("create and get session", func(t *testing.T) {
		session := Session{
			ID:          "sess-1",
			UserID:      "user-1",
			AppID:       "test-app",
			PodName:     "pod-sess-1",
			Status:      SessionStatusCreating,
			IdleTimeout: 3600,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		if err := db.CreateSession(session); err != nil {
			t.Fatalf("CreateSession() error = %v", err)
		}

		got, err := db.GetSession("sess-1")
		if err != nil {
			t.Fatalf("GetSession() error = %v", err)
		}
		if got == nil {
			t.Fatal("GetSession() returned nil")
		}
		if got.ID != "sess-1" {
			t.Errorf("got ID = %s, want sess-1", got.ID)
		}
		if got.IdleTimeout != 3600 {
			t.Errorf("got IdleTimeout = %d, want 3600", got.IdleTimeout)
		}
		if got.Status != SessionStatusCreating {
			t.Errorf("got Status = %s, want creating", got.Status)
		}
	})

	t.Run("update session status", func(t *testing.T) {
		if err := db.UpdateSessionStatus("sess-1", SessionStatusRunning); err != nil {
			t.Fatalf("UpdateSessionStatus() error = %v", err)
		}

		got, _ := db.GetSession("sess-1")
		if got.Status != SessionStatusRunning {
			t.Errorf("got Status = %s, want running", got.Status)
		}
	})

	t.Run("update session restart", func(t *testing.T) {
		// First stop the session
		if err := db.UpdateSessionStatus("sess-1", SessionStatusStopped); err != nil {
			t.Fatalf("UpdateSessionStatus(stopped) error = %v", err)
		}

		// Restart with new pod name
		if err := db.UpdateSessionRestart("sess-1", "pod-sess-1-v2"); err != nil {
			t.Fatalf("UpdateSessionRestart() error = %v", err)
		}

		got, _ := db.GetSession("sess-1")
		if got.PodName != "pod-sess-1-v2" {
			t.Errorf("got PodName = %s, want pod-sess-1-v2", got.PodName)
		}
		if got.PodIP != "" {
			t.Errorf("got PodIP = %s, want empty", got.PodIP)
		}
		if got.Status != SessionStatusCreating {
			t.Errorf("got Status = %s, want creating", got.Status)
		}
	})

	t.Run("list sessions", func(t *testing.T) {
		sessions, err := db.ListSessions()
		if err != nil {
			t.Fatalf("ListSessions() error = %v", err)
		}
		if len(sessions) != 1 {
			t.Fatalf("ListSessions() returned %d sessions, want 1", len(sessions))
		}
		if sessions[0].IdleTimeout != 3600 {
			t.Errorf("listed session IdleTimeout = %d, want 3600", sessions[0].IdleTimeout)
		}
	})

	t.Run("list sessions by user", func(t *testing.T) {
		sessions, err := db.ListSessionsByUser("user-1")
		if err != nil {
			t.Fatalf("ListSessionsByUser() error = %v", err)
		}
		if len(sessions) != 1 {
			t.Fatalf("ListSessionsByUser() returned %d sessions, want 1", len(sessions))
		}

		// No sessions for different user
		sessions, err = db.ListSessionsByUser("user-999")
		if err != nil {
			t.Fatalf("ListSessionsByUser() error = %v", err)
		}
		if len(sessions) != 0 {
			t.Errorf("ListSessionsByUser(unknown) returned %d sessions, want 0", len(sessions))
		}
	})

	t.Run("session with default idle timeout", func(t *testing.T) {
		session := Session{
			ID:        "sess-2",
			UserID:    "user-1",
			AppID:     "test-app",
			PodName:   "pod-sess-2",
			Status:    SessionStatusRunning,
			CreatedAt: now,
			UpdatedAt: now,
		}

		if err := db.CreateSession(session); err != nil {
			t.Fatalf("CreateSession() error = %v", err)
		}

		got, _ := db.GetSession("sess-2")
		if got.IdleTimeout != 0 {
			t.Errorf("got IdleTimeout = %d, want 0 (default)", got.IdleTimeout)
		}
	})

	t.Run("delete session", func(t *testing.T) {
		if err := db.DeleteSession("sess-2"); err != nil {
			t.Fatalf("DeleteSession() error = %v", err)
		}

		got, err := db.GetSession("sess-2")
		if err != nil {
			t.Fatalf("GetSession() error = %v", err)
		}
		if got != nil {
			t.Error("expected nil after delete")
		}
	})
}

func TestGetStaleSessions(t *testing.T) {
	db := setupTestDB(t)

	// Create an app
	app := Application{
		ID:          "test-app",
		Name:        "Test App",
		Description: "A test application",
		URL:         "https://example.com",
		Icon:        "icon.png",
		Category:    "test",
		LaunchType:  LaunchTypeContainer,
	}
	if err := db.CreateApp(app); err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	// Create a session with old updated_at using default timeout
	oldTime := time.Now().Add(-3 * time.Hour)
	session1 := Session{
		ID:        "stale-1",
		UserID:    "user-1",
		AppID:     "test-app",
		PodName:   "pod-stale-1",
		Status:    SessionStatusRunning,
		CreatedAt: oldTime,
		UpdatedAt: oldTime,
	}
	if err := db.CreateSession(session1); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	// Force the updated_at to be old
	db.conn.Exec("UPDATE sessions SET updated_at = ? WHERE id = ?", oldTime, "stale-1")

	// Create a fresh session
	freshTime := time.Now()
	session2 := Session{
		ID:        "fresh-1",
		UserID:    "user-1",
		AppID:     "test-app",
		PodName:   "pod-fresh-1",
		Status:    SessionStatusRunning,
		CreatedAt: freshTime,
		UpdatedAt: freshTime,
	}
	if err := db.CreateSession(session2); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	// Query with 2-hour default timeout
	stale, err := db.GetStaleSessions(2 * time.Hour)
	if err != nil {
		t.Fatalf("GetStaleSessions() error = %v", err)
	}

	if len(stale) != 1 {
		t.Fatalf("GetStaleSessions() returned %d, want 1", len(stale))
	}
	if stale[0].ID != "stale-1" {
		t.Errorf("got stale session ID = %s, want stale-1", stale[0].ID)
	}
}
