package sessions

import (
	"testing"
	"time"

	"github.com/rjsadow/launchpad/internal/db"
)

func TestSessionFromDB(t *testing.T) {
	now := time.Now()
	session := &db.Session{
		ID:          "sess-123",
		UserID:      "user-456",
		AppID:       "app-789",
		PodName:     "pod-abc",
		Status:      db.SessionStatusRunning,
		IdleTimeout: 3600,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	resp := SessionFromDB(session, "My App", "/ws/sessions/sess-123", "/ws/guac/sessions/sess-123", "/api/sessions/sess-123/proxy/")

	if resp.ID != "sess-123" {
		t.Errorf("ID = %q, want %q", resp.ID, "sess-123")
	}
	if resp.UserID != "user-456" {
		t.Errorf("UserID = %q, want %q", resp.UserID, "user-456")
	}
	if resp.AppID != "app-789" {
		t.Errorf("AppID = %q, want %q", resp.AppID, "app-789")
	}
	if resp.AppName != "My App" {
		t.Errorf("AppName = %q, want %q", resp.AppName, "My App")
	}
	if resp.PodName != "pod-abc" {
		t.Errorf("PodName = %q, want %q", resp.PodName, "pod-abc")
	}
	if resp.Status != db.SessionStatusRunning {
		t.Errorf("Status = %q, want %q", resp.Status, db.SessionStatusRunning)
	}
	if resp.IdleTimeout != 3600 {
		t.Errorf("IdleTimeout = %d, want %d", resp.IdleTimeout, 3600)
	}
	if resp.WebSocketURL != "/ws/sessions/sess-123" {
		t.Errorf("WebSocketURL = %q, want %q", resp.WebSocketURL, "/ws/sessions/sess-123")
	}
	if resp.GuacamoleURL != "/ws/guac/sessions/sess-123" {
		t.Errorf("GuacamoleURL = %q, want %q", resp.GuacamoleURL, "/ws/guac/sessions/sess-123")
	}
	if resp.ProxyURL != "/api/sessions/sess-123/proxy/" {
		t.Errorf("ProxyURL = %q, want %q", resp.ProxyURL, "/api/sessions/sess-123/proxy/")
	}
	if !resp.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", resp.CreatedAt, now)
	}
	if !resp.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt = %v, want %v", resp.UpdatedAt, now)
	}
}

func TestSessionFromDB_EmptyURLs(t *testing.T) {
	session := &db.Session{
		ID:     "sess-empty",
		UserID: "user1",
		AppID:  "app1",
		Status: db.SessionStatusCreating,
	}

	resp := SessionFromDB(session, "", "", "", "")

	if resp.WebSocketURL != "" {
		t.Errorf("WebSocketURL = %q, want empty", resp.WebSocketURL)
	}
	if resp.GuacamoleURL != "" {
		t.Errorf("GuacamoleURL = %q, want empty", resp.GuacamoleURL)
	}
	if resp.ProxyURL != "" {
		t.Errorf("ProxyURL = %q, want empty", resp.ProxyURL)
	}
	if resp.AppName != "" {
		t.Errorf("AppName = %q, want empty", resp.AppName)
	}
}

func TestSessionFromDB_ZeroIdleTimeout(t *testing.T) {
	session := &db.Session{
		ID:          "sess-zero",
		UserID:      "user1",
		AppID:       "app1",
		IdleTimeout: 0,
	}

	resp := SessionFromDB(session, "App", "", "", "")

	if resp.IdleTimeout != 0 {
		t.Errorf("IdleTimeout = %d, want 0", resp.IdleTimeout)
	}
}
