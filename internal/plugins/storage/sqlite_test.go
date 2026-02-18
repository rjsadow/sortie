package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/rjsadow/sortie/internal/db"
	"github.com/rjsadow/sortie/internal/plugins"
)

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestSQLiteStorage_Metadata(t *testing.T) {
	s := NewSQLiteStorage(nil)

	if s.Name() != "sqlite" {
		t.Errorf("expected name 'sqlite', got %q", s.Name())
	}
	if s.Type() != plugins.PluginTypeStorage {
		t.Errorf("expected type %q, got %q", plugins.PluginTypeStorage, s.Type())
	}
	if s.Version() != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", s.Version())
	}
	if s.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestSQLiteStorage_Initialize(t *testing.T) {
	s := NewSQLiteStorage(nil)
	cfg := map[string]string{"foo": "bar"}

	if err := s.Initialize(context.Background(), cfg); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	if s.config["foo"] != "bar" {
		t.Error("expected config to be stored")
	}
}

func TestSQLiteStorage_Healthy(t *testing.T) {
	database := openTestDB(t)
	s := NewSQLiteStorage(database)
	ctx := context.Background()

	if !s.Healthy(ctx) {
		t.Error("expected Healthy() == true with valid DB")
	}
}

func TestSQLiteStorage_HealthyNilDB(t *testing.T) {
	s := NewSQLiteStorage(nil)

	if s.Healthy(context.Background()) {
		t.Error("expected Healthy() == false with nil DB")
	}
}

func TestSQLiteStorage_HealthyClosedDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	database.Close()

	s := NewSQLiteStorage(database)
	if s.Healthy(context.Background()) {
		t.Error("expected Healthy() == false with closed DB")
	}
}

func TestSQLiteStorage_Close(t *testing.T) {
	s := NewSQLiteStorage(nil)
	if err := s.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

func TestSQLiteStorage_CRUDStubs(t *testing.T) {
	s := NewSQLiteStorage(nil)
	ctx := context.Background()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"CreateApp", func() error { return s.CreateApp(ctx, &plugins.Application{}) }},
		{"GetApp", func() error { _, err := s.GetApp(ctx, "x"); return err }},
		{"UpdateApp", func() error { return s.UpdateApp(ctx, &plugins.Application{}) }},
		{"DeleteApp", func() error { return s.DeleteApp(ctx, "x") }},
		{"ListApps", func() error { _, err := s.ListApps(ctx); return err }},
		{"CreateSession", func() error { return s.CreateSession(ctx, &plugins.Session{}) }},
		{"GetSession", func() error { _, err := s.GetSession(ctx, "x"); return err }},
		{"UpdateSession", func() error { return s.UpdateSession(ctx, &plugins.Session{}) }},
		{"DeleteSession", func() error { return s.DeleteSession(ctx, "x") }},
		{"ListSessions", func() error { _, err := s.ListSessions(ctx, ""); return err }},
		{"ListExpiredSessions", func() error { _, err := s.ListExpiredSessions(ctx); return err }},
		{"LogAudit", func() error { return s.LogAudit(ctx, &plugins.AuditEntry{}) }},
		{"GetAuditLogs", func() error { _, err := s.GetAuditLogs(ctx, 10); return err }},
		{"RecordLaunch", func() error { return s.RecordLaunch(ctx, "x") }},
		{"GetAnalyticsStats", func() error { _, err := s.GetAnalyticsStats(ctx); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err != plugins.ErrNotImplemented {
				t.Errorf("expected ErrNotImplemented, got %v", err)
			}
		})
	}
}

func TestSetDB(t *testing.T) {
	database := openTestDB(t)

	// Reset package state after test
	original := sharedDB
	t.Cleanup(func() { sharedDB = original })

	SetDB(database)
	if sharedDB != database {
		t.Error("SetDB did not set the package-level sharedDB")
	}
}

func TestSetDB_FactoryUsesSharedDB(t *testing.T) {
	database := openTestDB(t)

	original := sharedDB
	t.Cleanup(func() { sharedDB = original })

	SetDB(database)

	// Simulate what the registry does: call the factory
	s := NewSQLiteStorage(sharedDB)
	if !s.Healthy(context.Background()) {
		t.Error("plugin created via sharedDB should be healthy")
	}
}

func TestSQLiteStorage_InterfaceCompliance(t *testing.T) {
	var _ plugins.StorageProvider = (*SQLiteStorage)(nil)
}
