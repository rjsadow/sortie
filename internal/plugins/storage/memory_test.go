package storage

import (
	"context"
	"testing"

	"github.com/rjsadow/launchpad/internal/plugins"
)

func TestMemoryStorage_AppCRUD(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()

	if err := s.Initialize(ctx, nil); err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}
	defer s.Close()

	// Create
	app := &plugins.Application{
		ID:          "test-app",
		Name:        "Test App",
		Description: "A test application",
		URL:         "https://example.com",
		LaunchType:  plugins.LaunchTypeURL,
	}

	if err := s.CreateApp(ctx, app); err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	// Create duplicate should fail
	if err := s.CreateApp(ctx, app); err != plugins.ErrResourceExists {
		t.Errorf("expected ErrResourceExists, got %v", err)
	}

	// Get
	retrieved, err := s.GetApp(ctx, "test-app")
	if err != nil {
		t.Fatalf("failed to get app: %v", err)
	}
	if retrieved == nil {
		t.Fatal("app not found")
	}
	if retrieved.Name != "Test App" {
		t.Errorf("expected name 'Test App', got '%s'", retrieved.Name)
	}

	// Get non-existent
	notFound, err := s.GetApp(ctx, "non-existent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if notFound != nil {
		t.Error("expected nil for non-existent app")
	}

	// Update
	app.Name = "Updated App"
	if err := s.UpdateApp(ctx, app); err != nil {
		t.Fatalf("failed to update app: %v", err)
	}

	retrieved, _ = s.GetApp(ctx, "test-app")
	if retrieved.Name != "Updated App" {
		t.Errorf("expected name 'Updated App', got '%s'", retrieved.Name)
	}

	// List
	apps, err := s.ListApps(ctx)
	if err != nil {
		t.Fatalf("failed to list apps: %v", err)
	}
	if len(apps) != 1 {
		t.Errorf("expected 1 app, got %d", len(apps))
	}

	// Delete
	if err := s.DeleteApp(ctx, "test-app"); err != nil {
		t.Fatalf("failed to delete app: %v", err)
	}

	// Delete non-existent should fail
	if err := s.DeleteApp(ctx, "test-app"); err != plugins.ErrResourceNotFound {
		t.Errorf("expected ErrResourceNotFound, got %v", err)
	}

	// Verify deleted
	apps, _ = s.ListApps(ctx)
	if len(apps) != 0 {
		t.Errorf("expected 0 apps after delete, got %d", len(apps))
	}
}

func TestMemoryStorage_SessionCRUD(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()

	if err := s.Initialize(ctx, nil); err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}
	defer s.Close()

	// Create
	session := &plugins.Session{
		ID:     "test-session",
		UserID: "user1",
		AppID:  "app1",
		Status: plugins.LaunchStatusCreating,
	}

	if err := s.CreateSession(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Get
	retrieved, err := s.GetSession(ctx, "test-session")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if retrieved == nil {
		t.Fatal("session not found")
	}
	if retrieved.Status != plugins.LaunchStatusCreating {
		t.Errorf("expected status 'creating', got '%s'", retrieved.Status)
	}

	// Update
	session.Status = plugins.LaunchStatusRunning
	if err := s.UpdateSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	retrieved, _ = s.GetSession(ctx, "test-session")
	if retrieved.Status != plugins.LaunchStatusRunning {
		t.Errorf("expected status 'running', got '%s'", retrieved.Status)
	}

	// List by user
	sessions, err := s.ListSessions(ctx, "user1")
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(sessions))
	}

	// List all
	sessions, _ = s.ListSessions(ctx, "")
	if len(sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(sessions))
	}

	// Delete
	if err := s.DeleteSession(ctx, "test-session"); err != nil {
		t.Fatalf("failed to delete session: %v", err)
	}
}

func TestMemoryStorage_AuditLog(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()

	if err := s.Initialize(ctx, nil); err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}
	defer s.Close()

	// Log entries
	for i := 0; i < 5; i++ {
		entry := &plugins.AuditEntry{
			UserID:  "user1",
			Action:  "TEST_ACTION",
			Details: "test details",
		}
		if err := s.LogAudit(ctx, entry); err != nil {
			t.Fatalf("failed to log audit: %v", err)
		}
	}

	// Get logs
	entries, err := s.GetAuditLogs(ctx, 3)
	if err != nil {
		t.Fatalf("failed to get audit logs: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
}

func TestMemoryStorage_Analytics(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()

	if err := s.Initialize(ctx, nil); err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}
	defer s.Close()

	// Record launches
	for i := 0; i < 5; i++ {
		if err := s.RecordLaunch(ctx, "app1"); err != nil {
			t.Fatalf("failed to record launch: %v", err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := s.RecordLaunch(ctx, "app2"); err != nil {
			t.Fatalf("failed to record launch: %v", err)
		}
	}

	// Get stats
	stats, err := s.GetAnalyticsStats(ctx)
	if err != nil {
		t.Fatalf("failed to get analytics stats: %v", err)
	}

	if stats["total_launches"].(int) != 8 {
		t.Errorf("expected 8 total launches, got %v", stats["total_launches"])
	}
}

func TestMemoryStorage_Healthy(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()

	s.Initialize(ctx, nil)

	if !s.Healthy(ctx) {
		t.Error("expected storage to be healthy")
	}
}
