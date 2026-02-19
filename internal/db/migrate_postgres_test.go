package db

import (
	"database/sql"
	"testing"

	_ "github.com/lib/pq"
)

func TestRunMigrations_Postgres(t *testing.T) {
	if testDBType() != "postgres" {
		t.Skip("Postgres-specific migration test")
	}
	dsn := testPostgresDSN(t)
	resetPostgresDB(t, dsn)

	// Run migrations on a fresh database
	if err := runMigrations("postgres", dsn); err != nil {
		t.Fatalf("runMigrations() error = %v", err)
	}

	// Open a separate connection to verify results
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer conn.Close()

	// Verify all expected tables exist
	tables := []string{
		"applications", "audit_log", "analytics", "sessions",
		"users", "settings", "templates", "app_specs",
		"oidc_states", "tenants", "categories",
		"category_admins", "category_approved_users",
		"recordings", "session_shares",
	}

	for _, table := range tables {
		var count int
		err := conn.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
		if err != nil {
			t.Errorf("table %q does not exist: %v", table, err)
		}
	}

	// Verify default tenant was seeded
	var tenantName string
	err = conn.QueryRow("SELECT name FROM tenants WHERE id = 'default'").Scan(&tenantName)
	if err != nil {
		t.Fatalf("default tenant not found: %v", err)
	}
	if tenantName != "Default" {
		t.Errorf("default tenant name = %q, want %q", tenantName, "Default")
	}

	// Verify schema_migrations was created with correct version
	var version int
	var dirty bool
	err = conn.QueryRow("SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty)
	if err != nil {
		t.Fatalf("schema_migrations query error: %v", err)
	}
	if version != 2 {
		t.Errorf("migration version = %d, want 2", version)
	}
	if dirty {
		t.Error("migration is dirty, want clean")
	}

	// Running again should be idempotent (ErrNoChange handled)
	if err := runMigrations("postgres", dsn); err != nil {
		t.Fatalf("second runMigrations() error = %v (not idempotent)", err)
	}
}

func TestOpenDB_Postgres(t *testing.T) {
	if testDBType() != "postgres" {
		t.Skip("Postgres-specific migration test")
	}
	dsn := testPostgresDSN(t)
	resetPostgresDB(t, dsn)

	database, err := OpenDB("postgres", dsn)
	if err != nil {
		t.Fatalf("OpenDB() error = %v", err)
	}
	defer database.Close()

	if database.DBType() != "postgres" {
		t.Errorf("DBType() = %q, want %q", database.DBType(), "postgres")
	}

	// Verify tables were created
	var count int
	err = database.bun.DB.QueryRow("SELECT COUNT(*) FROM applications").Scan(&count)
	if err != nil {
		t.Fatalf("applications table not created: %v", err)
	}
}

func TestHandleMigrationUpgrade_ExistingDB_Postgres(t *testing.T) {
	if testDBType() != "postgres" {
		t.Skip("Postgres-specific migration test")
	}
	dsn := testPostgresDSN(t)
	resetPostgresDB(t, dsn)

	conn := openRawPostgresConn(t, dsn)

	// Simulate an existing database with tables but no golang-migrate tracking.
	_, err := conn.Exec(`CREATE TABLE applications (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatalf("failed to create applications table: %v", err)
	}

	// Run upgrade detection
	if err := handleMigrationUpgrade(conn, "postgres"); err != nil {
		t.Fatalf("handleMigrationUpgrade() error = %v", err)
	}

	// Verify schema_migrations was created with baseline version
	var version int
	var dirty bool
	err = conn.QueryRow("SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty)
	if err != nil {
		t.Fatalf("schema_migrations not created: %v", err)
	}
	if version != 1 {
		t.Errorf("version = %d, want 1 (baseline)", version)
	}
	if dirty {
		t.Error("dirty = true, want false")
	}
}

func TestHandleMigrationUpgrade_OldSchemaTable_Postgres(t *testing.T) {
	if testDBType() != "postgres" {
		t.Skip("Postgres-specific migration test")
	}
	dsn := testPostgresDSN(t)
	resetPostgresDB(t, dsn)

	conn := openRawPostgresConn(t, dsn)

	// Simulate old custom schema_migrations table
	_, err := conn.Exec(`CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TIMESTAMPTZ DEFAULT NOW()
	)`)
	if err != nil {
		t.Fatalf("failed to create old schema_migrations: %v", err)
	}
	_, err = conn.Exec("INSERT INTO schema_migrations (version) VALUES (1), (2), (3)")
	if err != nil {
		t.Fatalf("failed to insert old versions: %v", err)
	}

	// Also create applications table so it detects an existing DB
	_, err = conn.Exec(`CREATE TABLE applications (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatalf("failed to create applications table: %v", err)
	}

	// Run upgrade detection
	if err := handleMigrationUpgrade(conn, "postgres"); err != nil {
		t.Fatalf("handleMigrationUpgrade() error = %v", err)
	}

	// Verify old table was dropped and replaced with golang-migrate format
	var version int
	var dirty bool
	err = conn.QueryRow("SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty)
	if err != nil {
		t.Fatalf("new schema_migrations not created: %v", err)
	}
	if version != 1 {
		t.Errorf("version = %d, want 1", version)
	}
	if dirty {
		t.Error("dirty = true, want false")
	}

	// Verify the old 'applied_at' column no longer exists
	if hasOldSchemaTable(conn, "postgres") {
		t.Error("old schema_migrations table still detected after upgrade")
	}
}

func TestHandleMigrationUpgrade_FreshDB_Postgres(t *testing.T) {
	if testDBType() != "postgres" {
		t.Skip("Postgres-specific migration test")
	}
	dsn := testPostgresDSN(t)
	resetPostgresDB(t, dsn)

	conn := openRawPostgresConn(t, dsn)

	// Fresh DB — no tables at all
	if err := handleMigrationUpgrade(conn, "postgres"); err != nil {
		t.Fatalf("handleMigrationUpgrade() on fresh DB error = %v", err)
	}

	// schema_migrations should NOT have been created (no existing tables to detect)
	var count int
	err := conn.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	if err == nil {
		t.Error("schema_migrations should not exist on fresh DB before migrations run")
	}
}

func TestDefaultTenantSeedIdempotent_Postgres(t *testing.T) {
	if testDBType() != "postgres" {
		t.Skip("Postgres-specific migration test")
	}
	dsn := testPostgresDSN(t)
	resetPostgresDB(t, dsn)

	// Open twice to run migrations twice
	db1, err := OpenDB("postgres", dsn)
	if err != nil {
		t.Fatalf("first OpenDB() error = %v", err)
	}
	db1.Close()

	db2, err := OpenDB("postgres", dsn)
	if err != nil {
		t.Fatalf("second OpenDB() error = %v", err)
	}
	defer db2.Close()

	var count int
	err = db2.bun.DB.QueryRow("SELECT COUNT(*) FROM tenants WHERE id = 'default'").Scan(&count)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if count != 1 {
		t.Errorf("default tenant count = %d, want exactly 1 (seed not idempotent)", count)
	}
}

func TestNewMigrator_Postgres(t *testing.T) {
	if testDBType() != "postgres" {
		t.Skip("Postgres-specific migration test")
	}
	dsn := testPostgresDSN(t)
	resetPostgresDB(t, dsn)

	m, err := NewMigrator("postgres", dsn)
	if err != nil {
		t.Fatalf("NewMigrator() error = %v", err)
	}
	defer m.Close()

	// Run up
	if err := m.Up(); err != nil {
		t.Fatalf("m.Up() error = %v", err)
	}

	// Verify version
	version, dirty, err := m.Version()
	if err != nil {
		t.Fatalf("m.Version() error = %v", err)
	}
	if version != 2 {
		t.Errorf("version = %d, want 2", version)
	}
	if dirty {
		t.Error("dirty = true, want false")
	}
}

func TestDownMigration_Postgres(t *testing.T) {
	if testDBType() != "postgres" {
		t.Skip("Postgres-specific migration test")
	}
	dsn := testPostgresDSN(t)
	resetPostgresDB(t, dsn)

	m, err := NewMigrator("postgres", dsn)
	if err != nil {
		t.Fatalf("NewMigrator() error = %v", err)
	}

	// Migrate up fully
	if err := m.Up(); err != nil {
		t.Fatalf("m.Up() error = %v", err)
	}
	m.Close()

	// Reopen and step down once (undoes 000002 data migration — no-op)
	m, err = NewMigrator("postgres", dsn)
	if err != nil {
		t.Fatalf("NewMigrator() reopen error = %v", err)
	}

	if err := m.Steps(-1); err != nil {
		t.Fatalf("m.Steps(-1) error = %v", err)
	}
	version, _, _ := m.Version()
	if version != 1 {
		t.Errorf("after one step down, version = %d, want 1", version)
	}

	// Step down again (undoes 000001 baseline — drops tables)
	if err := m.Steps(-1); err != nil {
		t.Fatalf("second m.Steps(-1) error = %v", err)
	}
	m.Close()

	// Verify tables were dropped
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer conn.Close()

	tables := []string{"applications", "users", "sessions", "tenants", "recordings"}
	for _, table := range tables {
		var count int
		err := conn.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
		if err == nil {
			t.Errorf("table %q should have been dropped by down migration", table)
		}
	}
}

func TestUpDownUpCycle_Postgres(t *testing.T) {
	if testDBType() != "postgres" {
		t.Skip("Postgres-specific migration test")
	}
	dsn := testPostgresDSN(t)
	resetPostgresDB(t, dsn)

	// Up
	m, err := NewMigrator("postgres", dsn)
	if err != nil {
		t.Fatalf("NewMigrator() error = %v", err)
	}
	if err := m.Up(); err != nil {
		t.Fatalf("first m.Up() error = %v", err)
	}
	m.Close()

	// Down fully
	m, err = NewMigrator("postgres", dsn)
	if err != nil {
		t.Fatalf("NewMigrator() error = %v", err)
	}
	if err := m.Steps(-1); err != nil {
		t.Fatalf("m.Steps(-1) step 1 error = %v", err)
	}
	if err := m.Steps(-1); err != nil {
		t.Fatalf("m.Steps(-1) step 2 error = %v", err)
	}
	m.Close()

	// Up again — should recreate everything cleanly
	m, err = NewMigrator("postgres", dsn)
	if err != nil {
		t.Fatalf("NewMigrator() error = %v", err)
	}
	if err := m.Up(); err != nil {
		t.Fatalf("second m.Up() error = %v", err)
	}
	version, dirty, _ := m.Version()
	m.Close()

	if version != 2 {
		t.Errorf("after up-down-up, version = %d, want 2", version)
	}
	if dirty {
		t.Error("after up-down-up, dirty = true, want false")
	}

	// Verify tables exist again
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer conn.Close()

	var count int
	if err := conn.QueryRow("SELECT COUNT(*) FROM applications").Scan(&count); err != nil {
		t.Fatal("applications table missing after up-down-up cycle")
	}
	if err := conn.QueryRow("SELECT COUNT(*) FROM tenants WHERE id = 'default'").Scan(&count); err != nil {
		t.Fatal("default tenant missing after up-down-up cycle")
	}
}

func TestDataMigration_EmptyCategories_Postgres(t *testing.T) {
	if testDBType() != "postgres" {
		t.Skip("Postgres-specific migration test")
	}
	dsn := testPostgresDSN(t)
	resetPostgresDB(t, dsn)

	// Create DB at version 2 (full migration)
	if err := runMigrations("postgres", dsn); err != nil {
		t.Fatalf("runMigrations error: %v", err)
	}

	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}

	// Insert app with empty category
	_, err = conn.Exec("INSERT INTO applications (id, name, description, url, icon, category) VALUES ('empty-1', 'NoCategory', 'test', 'http://x', 'i', '')")
	if err != nil {
		t.Fatalf("failed to insert app: %v", err)
	}

	// Force version back to 1 to re-run data migration
	_, err = conn.Exec("UPDATE schema_migrations SET version = 1")
	if err != nil {
		t.Fatalf("failed to update version: %v", err)
	}
	conn.Close()

	// Re-run migrations (will run version 2 again)
	if err := runMigrations("postgres", dsn); err != nil {
		t.Fatalf("re-run migrations error: %v", err)
	}

	// Verify no empty-name category was created
	conn, err = sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to reopen: %v", err)
	}
	defer conn.Close()

	var count int
	err = conn.QueryRow("SELECT COUNT(*) FROM categories WHERE name = ''").Scan(&count)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if count != 0 {
		t.Errorf("found %d categories with empty name, want 0", count)
	}
}

func TestDataMigration_DuplicateCategories_Postgres(t *testing.T) {
	if testDBType() != "postgres" {
		t.Skip("Postgres-specific migration test")
	}
	dsn := testPostgresDSN(t)
	resetPostgresDB(t, dsn)

	// Create DB with full migrations
	if err := runMigrations("postgres", dsn); err != nil {
		t.Fatalf("runMigrations error: %v", err)
	}

	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}

	_, err = conn.Exec("INSERT INTO applications (id, name, description, url, icon, category) VALUES ('d-1', 'App1', 't', 'http://x', 'i', 'Tools')")
	if err != nil {
		t.Fatalf("failed to insert app: %v", err)
	}

	// Manually create the category first (simulating it already existing)
	_, err = conn.Exec("INSERT INTO categories (id, name, description, tenant_id) VALUES ('existing-cat', 'Tools', 'existing', 'default')")
	if err != nil {
		t.Fatalf("failed to insert category: %v", err)
	}

	// Force version back and re-run
	_, err = conn.Exec("UPDATE schema_migrations SET version = 1")
	if err != nil {
		t.Fatalf("failed to update version: %v", err)
	}
	conn.Close()

	if err := runMigrations("postgres", dsn); err != nil {
		t.Fatalf("re-run migrations error: %v", err)
	}

	conn, err = sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to reopen: %v", err)
	}
	defer conn.Close()

	var count int
	err = conn.QueryRow("SELECT COUNT(*) FROM categories WHERE name = 'Tools'").Scan(&count)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if count != 1 {
		t.Errorf("found %d 'Tools' categories, want exactly 1 (ON CONFLICT DO NOTHING failed)", count)
	}
}
