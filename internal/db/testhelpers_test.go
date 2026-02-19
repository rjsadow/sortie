package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/lib/pq"
)

// testDBType returns the configured test database type (default: "sqlite").
func testDBType() string {
	if v := os.Getenv("SORTIE_TEST_DB_TYPE"); v != "" {
		return v
	}
	return "sqlite"
}

// newTestDatabase creates a test database for the db package's own tests.
// This mirrors dbtest.NewTestDB but lives inside the db package to avoid
// a circular import (db -> dbtest -> db).
func newTestDatabase(t *testing.T) *DB {
	t.Helper()

	dbType := testDBType()

	switch dbType {
	case "sqlite":
		dbPath := filepath.Join(t.TempDir(), "test.db")
		database, err := OpenDB("sqlite", dbPath)
		if err != nil {
			t.Fatalf("failed to open SQLite test database: %v", err)
		}
		t.Cleanup(func() { database.Close() })
		return database

	case "postgres":
		dsn := os.Getenv("SORTIE_TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("SORTIE_TEST_POSTGRES_DSN not set; skipping Postgres test")
		}
		database, err := OpenDB("postgres", dsn)
		if err != nil {
			t.Fatalf("failed to open Postgres test database: %v", err)
		}
		t.Cleanup(func() { database.Close() })
		truncateAllTables(t, database)
		return database

	default:
		t.Fatalf("unsupported SORTIE_TEST_DB_TYPE: %s", dbType)
		return nil
	}
}

// truncateAllTables removes all data from Postgres tables and re-seeds the
// default tenant. Used before each test to ensure a clean state.
func truncateAllTables(t *testing.T, database *DB) {
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
			t.Fatalf("failed to truncate %s: %v", table, err)
		}
	}

	_, err := database.ExecRaw(
		"INSERT INTO tenants (id, name, slug, settings, quotas) VALUES (?, ?, ?, ?, ?)",
		"default", "Default", "default", "{}", "{}",
	)
	if err != nil {
		t.Fatalf("failed to re-seed default tenant: %v", err)
	}
}

// testPostgresDSN returns the Postgres DSN from SORTIE_TEST_POSTGRES_DSN
// or skips the test if not set.
func testPostgresDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("SORTIE_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("SORTIE_TEST_POSTGRES_DSN not set; skipping Postgres test")
	}
	return dsn
}

// resetPostgresDB drops ALL tables in the public schema including schema_migrations.
// Unlike truncateAllTables (which preserves table structures), this produces a
// completely empty database — required by migration tests that need to test
// table creation from scratch.
func resetPostgresDB(t *testing.T, dsn string) {
	t.Helper()

	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("resetPostgresDB: failed to open connection: %v", err)
	}
	defer conn.Close()

	_, err = conn.Exec(`
		DO $$ DECLARE
			r RECORD;
		BEGIN
			FOR r IN (SELECT tablename FROM pg_tables WHERE schemaname = 'public') LOOP
				EXECUTE 'DROP TABLE IF EXISTS public.' || quote_ident(r.tablename) || ' CASCADE';
			END LOOP;
		END $$;
	`)
	if err != nil {
		t.Fatalf("resetPostgresDB: failed to drop tables: %v", err)
	}
}

// openRawPostgresConn opens a raw *sql.DB (not Bun-wrapped) for Postgres.
// The connection is closed automatically via t.Cleanup.
// This is needed for tests that call handleMigrationUpgrade() directly
// before OpenDB has been called.
func openRawPostgresConn(t *testing.T, dsn string) *sql.DB {
	t.Helper()

	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("openRawPostgresConn: failed to open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// --- Tests for the test helper infrastructure itself ---

func TestNewTestDatabase_ReturnsWorkingDB(t *testing.T) {
	database := newTestDatabase(t)

	if err := database.Ping(); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	if database.DBType() != testDBType() {
		t.Errorf("DBType() = %q, want %q", database.DBType(), testDBType())
	}
}

func TestNewTestDatabase_HasDefaultTenant(t *testing.T) {
	database := newTestDatabase(t)

	tenant, err := database.GetTenant(DefaultTenantID)
	if err != nil {
		t.Fatalf("GetTenant(%q) error = %v", DefaultTenantID, err)
	}
	if tenant == nil {
		t.Fatal("default tenant not found — newTestDatabase must ensure it exists")
	}
	if tenant.Slug != "default" {
		t.Errorf("default tenant slug = %q, want %q", tenant.Slug, "default")
	}
}

func TestNewTestDatabase_SupportsCRUD(t *testing.T) {
	database := newTestDatabase(t)

	// Insert via ExecRaw
	_, err := database.ExecRaw(
		"INSERT INTO settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)",
		"helper_test_key", "helper_test_value",
	)
	if err != nil {
		t.Fatalf("ExecRaw INSERT error = %v", err)
	}

	// Read back via ORM
	val, err := database.GetSetting("helper_test_key")
	if err != nil {
		t.Fatalf("GetSetting() error = %v", err)
	}
	if val != "helper_test_value" {
		t.Errorf("got value = %q, want %q", val, "helper_test_value")
	}
}

func TestNewTestDatabase_IndependentInstances(t *testing.T) {
	db1 := newTestDatabase(t)
	db2 := newTestDatabase(t)

	// Insert a unique value into db1
	if err := db1.SetSetting("instance_test", "from_db1"); err != nil {
		t.Fatalf("db1 SetSetting error: %v", err)
	}

	// db2 should not see db1's data (proves isolation)
	if testDBType() == "sqlite" {
		// SQLite: each call creates a separate temp file
		val, err := db2.GetSetting("instance_test")
		if err != nil {
			t.Fatalf("db2 GetSetting error: %v", err)
		}
		if val != "" {
			t.Errorf("db2 saw db1's data: got %q, want empty (separate databases)", val)
		}
	}
	// For Postgres: both share the same DB, but truncation before each
	// newTestDatabase call ensures clean state at creation time.
	// Cross-contamination during the same test is expected with shared PG.
}

func TestTestDBType_ReturnsValidType(t *testing.T) {
	dbType := testDBType()
	switch dbType {
	case "sqlite", "postgres":
		// valid
	default:
		t.Errorf("testDBType() = %q, want sqlite or postgres", dbType)
	}
}
