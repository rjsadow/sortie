// Package dbtest provides shared test helpers for creating test databases.
// All test packages that need a database should use NewTestDB instead of
// writing their own setup functions. The backend is controlled by the
// SORTIE_TEST_DB_TYPE environment variable ("sqlite" or "postgres").
package dbtest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rjsadow/sortie/internal/db"
)

// testDBType returns the configured test database type (default: "sqlite").
func testDBType() string {
	if v := os.Getenv("SORTIE_TEST_DB_TYPE"); v != "" {
		return v
	}
	return "sqlite"
}

// NewTestDB creates a test database appropriate for the current backend.
//
// For SQLite (default): creates a temp-file database in t.TempDir().
// For Postgres: connects using SORTIE_TEST_POSTGRES_DSN, truncates all
// tables, and re-seeds the default tenant. Skips the test if no DSN is set.
//
// Cleanup (Close) is registered via t.Cleanup automatically.
func NewTestDB(t *testing.T) *db.DB {
	t.Helper()

	dbType := testDBType()

	switch dbType {
	case "sqlite":
		return newSQLiteTestDB(t)
	case "postgres":
		return newPostgresTestDB(t)
	default:
		t.Fatalf("unsupported SORTIE_TEST_DB_TYPE: %s", dbType)
		return nil
	}
}

func newSQLiteTestDB(t *testing.T) *db.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenDB("sqlite", dbPath)
	if err != nil {
		t.Fatalf("dbtest: failed to open SQLite database: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func newPostgresTestDB(t *testing.T) *db.DB {
	t.Helper()

	dsn := os.Getenv("SORTIE_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("SORTIE_TEST_POSTGRES_DSN not set; skipping Postgres test")
	}

	database, err := db.OpenDB("postgres", dsn)
	if err != nil {
		t.Fatalf("dbtest: failed to open Postgres database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	truncateAllTables(t, database)
	return database
}

// truncateAllTables removes all data from Postgres tables in FK-safe order
// (using CASCADE) and re-seeds the default tenant.
func truncateAllTables(t *testing.T, database *db.DB) {
	t.Helper()

	tables := []string{
		"session_shares", "recordings",
		"category_approved_users", "category_admins",
		"categories", "oidc_states", "app_specs", "templates",
		"settings", "analytics", "sessions", "audit_log",
		"users", "applications", "tenants",
	}

	for _, table := range tables {
		if _, err := database.ExecRaw("TRUNCATE TABLE " + table + " CASCADE"); err != nil {
			t.Fatalf("dbtest: failed to truncate %s: %v", table, err)
		}
	}

	// Re-seed the default tenant (required by FK constraints and app logic)
	_, err := database.ExecRaw(
		"INSERT INTO tenants (id, name, slug, settings, quotas) VALUES (?, ?, ?, ?, ?)",
		"default", "Default", "default", "{}", "{}",
	)
	if err != nil {
		t.Fatalf("dbtest: failed to re-seed default tenant: %v", err)
	}
}
