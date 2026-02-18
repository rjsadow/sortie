package db

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRunMigrations_SQLite(t *testing.T) {
	if testDBType() != "sqlite" {
		t.Skip("SQLite-specific migration test")
	}
	tmpFile, err := os.CreateTemp("", "test-migrate-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Run migrations on a fresh database
	if err := runMigrations("sqlite", tmpFile.Name()); err != nil {
		t.Fatalf("runMigrations() error = %v", err)
	}

	// Open a separate connection to verify results
	conn, err := sql.Open("sqlite", tmpFile.Name())
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
	if err := runMigrations("sqlite", tmpFile.Name()); err != nil {
		t.Fatalf("second runMigrations() error = %v (not idempotent)", err)
	}
}

func TestHandleMigrationUpgrade_ExistingDB(t *testing.T) {
	if testDBType() != "sqlite" {
		t.Skip("SQLite-specific migration test")
	}
	tmpFile, err := os.CreateTemp("", "test-upgrade-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	conn, err := sql.Open("sqlite", tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer conn.Close()

	// Simulate an existing database with tables but no golang-migrate tracking.
	// Create just the applications table to trigger detection.
	_, err = conn.Exec(`CREATE TABLE applications (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatalf("failed to create applications table: %v", err)
	}

	// Run upgrade detection
	if err := handleMigrationUpgrade(conn, "sqlite"); err != nil {
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

func TestHandleMigrationUpgrade_OldSchemaTable(t *testing.T) {
	if testDBType() != "sqlite" {
		t.Skip("SQLite-specific migration test")
	}
	tmpFile, err := os.CreateTemp("", "test-oldschema-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	conn, err := sql.Open("sqlite", tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer conn.Close()

	// Simulate old custom schema_migrations table (from cmd/migrate)
	_, err = conn.Exec(`CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
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
	if err := handleMigrationUpgrade(conn, "sqlite"); err != nil {
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
	if hasOldSchemaTable(conn, "sqlite") {
		t.Error("old schema_migrations table still detected after upgrade")
	}
}

func TestHandleMigrationUpgrade_FreshDB(t *testing.T) {
	if testDBType() != "sqlite" {
		t.Skip("SQLite-specific migration test")
	}
	tmpFile, err := os.CreateTemp("", "test-fresh-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	conn, err := sql.Open("sqlite", tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer conn.Close()

	// Fresh DB — no tables at all
	if err := handleMigrationUpgrade(conn, "sqlite"); err != nil {
		t.Fatalf("handleMigrationUpgrade() on fresh DB error = %v", err)
	}

	// schema_migrations should NOT have been created (no existing tables to detect)
	var count int
	err = conn.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	if err == nil {
		t.Error("schema_migrations should not exist on fresh DB before migrations run")
	}
}

func TestOpenDB_SQLite(t *testing.T) {
	if testDBType() != "sqlite" {
		t.Skip("SQLite-specific migration test")
	}
	tmpFile, err := os.CreateTemp("", "test-opendb-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	database, err := OpenDB("sqlite", tmpFile.Name())
	if err != nil {
		t.Fatalf("OpenDB() error = %v", err)
	}
	defer database.Close()

	if database.DBType() != "sqlite" {
		t.Errorf("DBType() = %q, want %q", database.DBType(), "sqlite")
	}

	// Verify tables were created
	var count int
	err = database.bun.DB.QueryRow("SELECT COUNT(*) FROM applications").Scan(&count)
	if err != nil {
		t.Fatalf("applications table not created: %v", err)
	}
}

func TestOpenDB_UnsupportedType(t *testing.T) {
	_, err := OpenDB("mysql", "test.db")
	if err == nil {
		t.Fatal("expected error for unsupported db type, got nil")
	}
}

// TestOpenDB_InMemory exercises the :memory: special-case path in OpenDB
// which rewrites the DSN to file::memory:?cache=shared.
func TestOpenDB_InMemory(t *testing.T) {
	if testDBType() != "sqlite" {
		t.Skip("SQLite-specific migration test")
	}
	database, err := OpenDB("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("OpenDB(:memory:) error = %v", err)
	}
	defer database.Close()

	// Verify tables exist on the in-memory DB
	var count int
	if err := database.bun.DB.QueryRow("SELECT COUNT(*) FROM applications").Scan(&count); err != nil {
		t.Fatalf("applications table not created on in-memory DB: %v", err)
	}

	// Verify we can write and read back (proves migration ran on same connection)
	app := Application{
		ID: "mem-1", Name: "MemApp", Description: "d",
		URL: "http://x", Icon: "i", Category: "test",
	}
	if err := database.CreateApp(app); err != nil {
		t.Fatalf("CreateApp() on in-memory DB error = %v", err)
	}
	got, err := database.GetApp("mem-1")
	if err != nil {
		t.Fatalf("GetApp() on in-memory DB error = %v", err)
	}
	if got == nil || got.Name != "MemApp" {
		t.Error("data not persisted on in-memory DB after migration")
	}
}

// TestSchemaColumns_Applications verifies that the applications table has all
// expected columns with correct types and defaults after migration.
func TestSchemaColumns_Applications(t *testing.T) {
	if testDBType() != "sqlite" {
		t.Skip("SQLite-specific schema test")
	}
	tmpFile, err := os.CreateTemp("", "test-schema-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	database, err := OpenDB("sqlite", tmpFile.Name())
	if err != nil {
		t.Fatalf("OpenDB() error = %v", err)
	}
	defer database.Close()

	// Query pragma_table_info to get column metadata
	type colInfo struct {
		name     string
		colType  string
		notNull  bool
		dfltVal  *string
		pk       bool
	}

	rows, err := database.bun.DB.Query("SELECT name, type, \"notnull\", dflt_value, pk FROM pragma_table_info('applications')")
	if err != nil {
		t.Fatalf("pragma_table_info error: %v", err)
	}
	defer rows.Close()

	cols := make(map[string]colInfo)
	for rows.Next() {
		var c colInfo
		var pkInt int
		var notNullInt int
		if err := rows.Scan(&c.name, &c.colType, &notNullInt, &c.dfltVal, &pkInt); err != nil {
			t.Fatalf("scan error: %v", err)
		}
		c.notNull = notNullInt != 0
		c.pk = pkInt != 0
		cols[c.name] = c
	}

	// Verify critical columns exist
	expectedCols := []string{
		"id", "name", "description", "url", "icon", "category",
		"visibility", "launch_type", "os_type", "container_image",
		"container_port", "container_args", "cpu_request", "cpu_limit",
		"memory_request", "memory_limit", "egress_policy", "tenant_id",
	}
	for _, name := range expectedCols {
		if _, ok := cols[name]; !ok {
			t.Errorf("applications table missing column %q", name)
		}
	}

	// Verify specific column properties
	if c, ok := cols["id"]; ok {
		if !c.pk {
			t.Error("applications.id should be PRIMARY KEY")
		}
	}
	if c, ok := cols["visibility"]; ok {
		if !c.notNull {
			t.Error("applications.visibility should be NOT NULL")
		}
		if c.dfltVal == nil || *c.dfltVal != "'public'" {
			t.Errorf("applications.visibility default = %v, want 'public'", c.dfltVal)
		}
	}
	if c, ok := cols["launch_type"]; ok {
		if !c.notNull {
			t.Error("applications.launch_type should be NOT NULL")
		}
		if c.dfltVal == nil || *c.dfltVal != "'url'" {
			t.Errorf("applications.launch_type default = %v, want 'url'", c.dfltVal)
		}
	}
	if c, ok := cols["tenant_id"]; ok {
		if c.dfltVal == nil || *c.dfltVal != "'default'" {
			t.Errorf("applications.tenant_id default = %v, want 'default'", c.dfltVal)
		}
	}
	if c, ok := cols["os_type"]; ok {
		if c.dfltVal == nil || *c.dfltVal != "'linux'" {
			t.Errorf("applications.os_type default = %v, want 'linux'", c.dfltVal)
		}
	}
}

// TestSchemaColumns_AllTables verifies that every table has the expected column
// count, catching missing or extra columns from SQL file typos.
func TestSchemaColumns_AllTables(t *testing.T) {
	if testDBType() != "sqlite" {
		t.Skip("SQLite-specific schema test")
	}
	tmpFile, err := os.CreateTemp("", "test-allcols-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	database, err := OpenDB("sqlite", tmpFile.Name())
	if err != nil {
		t.Fatalf("OpenDB() error = %v", err)
	}
	defer database.Close()

	// Expected column counts per table (from baseline schema)
	expectedColumnCounts := map[string]int{
		"applications":           18,
		"audit_log":              6,
		"analytics":              4,
		"sessions":               10,
		"users":                  12,
		"settings":               3,
		"templates":              23,
		"app_specs":              16,
		"oidc_states":            3,
		"tenants":                7,
		"categories":             6,
		"category_admins":        2,
		"category_approved_users": 2,
		"recordings":             14,
		"session_shares":         7,
	}

	for table, expected := range expectedColumnCounts {
		var count int
		err := database.bun.DB.QueryRow("SELECT COUNT(*) FROM pragma_table_info(?)", table).Scan(&count)
		if err != nil {
			t.Errorf("pragma_table_info(%q) error: %v", table, err)
			continue
		}
		if count != expected {
			t.Errorf("table %q has %d columns, want %d", table, count, expected)
		}
	}
}

// TestSchemaIndexes verifies that all expected indexes are created.
func TestSchemaIndexes(t *testing.T) {
	if testDBType() != "sqlite" {
		t.Skip("SQLite-specific schema test")
	}
	tmpFile, err := os.CreateTemp("", "test-indexes-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	database, err := OpenDB("sqlite", tmpFile.Name())
	if err != nil {
		t.Fatalf("OpenDB() error = %v", err)
	}
	defer database.Close()

	expectedIndexes := []string{
		"idx_audit_timestamp",
		"idx_analytics_app_id",
		"idx_analytics_timestamp",
		"idx_sessions_user_id",
		"idx_sessions_status",
		"idx_users_username",
		"idx_templates_template_id",
		"idx_templates_category",
		"idx_oidc_states_expires",
		"idx_tenants_slug",
		"idx_categories_tenant",
		"idx_recordings_session",
		"idx_recordings_user",
		"idx_recordings_status",
		"idx_session_shares_session",
		"idx_session_shares_user",
		"idx_session_shares_token",
	}

	// Query all indexes from sqlite_master
	rows, err := database.bun.DB.Query("SELECT name FROM sqlite_master WHERE type = 'index' AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		t.Fatalf("failed to query indexes: %v", err)
	}
	defer rows.Close()

	existing := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan error: %v", err)
		}
		existing[name] = true
	}

	for _, idx := range expectedIndexes {
		if !existing[idx] {
			t.Errorf("missing index %q", idx)
		}
	}
}

// TestDefaultTenantSeedIdempotent verifies that running migrations twice
// does not duplicate the default tenant.
func TestDefaultTenantSeedIdempotent(t *testing.T) {
	if testDBType() != "sqlite" {
		t.Skip("SQLite-specific migration test")
	}
	tmpFile, err := os.CreateTemp("", "test-seedidemp-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Open twice to run migrations twice
	db1, err := OpenDB("sqlite", tmpFile.Name())
	if err != nil {
		t.Fatalf("first OpenDB() error = %v", err)
	}
	db1.Close()

	db2, err := OpenDB("sqlite", tmpFile.Name())
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

// TestNewMigrator_SQLite tests the exported NewMigrator function used by the CLI tool.
func TestNewMigrator_SQLite(t *testing.T) {
	if testDBType() != "sqlite" {
		t.Skip("SQLite-specific migration test")
	}
	tmpFile, err := os.CreateTemp("", "test-newmigrator-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	m, err := NewMigrator("sqlite", tmpFile.Name())
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

// TestNewMigrator_UnsupportedType verifies error handling for invalid types.
func TestNewMigrator_UnsupportedType(t *testing.T) {
	_, err := NewMigrator("mysql", "test.db")
	if err == nil {
		t.Fatal("expected error for unsupported type, got nil")
	}
}

// TestDownMigration verifies the baseline down migration drops all tables.
func TestDownMigration(t *testing.T) {
	if testDBType() != "sqlite" {
		t.Skip("SQLite-specific migration test")
	}
	tmpFile, err := os.CreateTemp("", "test-down-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	m, err := NewMigrator("sqlite", tmpFile.Name())
	if err != nil {
		t.Fatalf("NewMigrator() error = %v", err)
	}

	// Migrate up fully
	if err := m.Up(); err != nil {
		t.Fatalf("m.Up() error = %v", err)
	}
	m.Close()

	// Reopen and step down once (undoes 000002 data migration — no-op)
	m, err = NewMigrator("sqlite", tmpFile.Name())
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
	conn, err := sql.Open("sqlite", tmpFile.Name())
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

// TestUpDownUpCycle verifies that a full up → down → up cycle works cleanly.
func TestUpDownUpCycle(t *testing.T) {
	if testDBType() != "sqlite" {
		t.Skip("SQLite-specific migration test")
	}
	tmpFile, err := os.CreateTemp("", "test-cycle-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Up
	m, err := NewMigrator("sqlite", tmpFile.Name())
	if err != nil {
		t.Fatalf("NewMigrator() error = %v", err)
	}
	if err := m.Up(); err != nil {
		t.Fatalf("first m.Up() error = %v", err)
	}
	m.Close()

	// Down fully
	m, err = NewMigrator("sqlite", tmpFile.Name())
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
	m, err = NewMigrator("sqlite", tmpFile.Name())
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
	conn, err := sql.Open("sqlite", tmpFile.Name())
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

// TestEndToEndUpgrade_OldDBWithData simulates a real upgrade scenario:
// an old database with data and the legacy schema_migrations table,
// upgrading to golang-migrate with all data preserved.
func TestEndToEndUpgrade_OldDBWithData(t *testing.T) {
	if testDBType() != "sqlite" {
		t.Skip("SQLite-specific migration test")
	}
	tmpFile, err := os.CreateTemp("", "test-e2e-upgrade-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	conn, err := sql.Open("sqlite", tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	// Simulate a pre-golang-migrate database: create all tables manually
	// (as the old migrate() would have), insert data, and create the old
	// schema_migrations table.
	schema := `
	CREATE TABLE applications (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT NOT NULL,
		url TEXT NOT NULL,
		icon TEXT NOT NULL,
		category TEXT NOT NULL,
		visibility TEXT NOT NULL DEFAULT 'public',
		launch_type TEXT NOT NULL DEFAULT 'url',
		os_type TEXT DEFAULT 'linux',
		container_image TEXT DEFAULT '',
		container_port INTEGER DEFAULT 0,
		container_args TEXT DEFAULT '[]',
		cpu_request TEXT DEFAULT '',
		cpu_limit TEXT DEFAULT '',
		memory_request TEXT DEFAULT '',
		memory_limit TEXT DEFAULT '',
		egress_policy TEXT DEFAULT '',
		tenant_id TEXT DEFAULT 'default'
	);
	CREATE TABLE users (
		id TEXT PRIMARY KEY,
		username TEXT NOT NULL UNIQUE,
		email TEXT,
		display_name TEXT,
		password_hash TEXT NOT NULL,
		roles TEXT DEFAULT '["user"]',
		auth_provider TEXT DEFAULT 'local',
		auth_provider_id TEXT DEFAULT '',
		tenant_id TEXT DEFAULT 'default',
		tenant_roles TEXT DEFAULT '[]',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE sessions (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		app_id TEXT NOT NULL,
		pod_name TEXT NOT NULL,
		pod_ip TEXT DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending',
		idle_timeout INTEGER DEFAULT 0,
		tenant_id TEXT DEFAULT 'default',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE tenants (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		slug TEXT NOT NULL UNIQUE,
		settings TEXT DEFAULT '{}',
		quotas TEXT DEFAULT '{}',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	INSERT INTO tenants (id, name, slug) VALUES ('default', 'Default', 'default');
	CREATE TABLE audit_log (id INTEGER PRIMARY KEY AUTOINCREMENT, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP, user TEXT NOT NULL, action TEXT NOT NULL, details TEXT NOT NULL, tenant_id TEXT DEFAULT 'default');
	CREATE TABLE analytics (id INTEGER PRIMARY KEY AUTOINCREMENT, app_id TEXT NOT NULL, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP, tenant_id TEXT DEFAULT 'default');
	CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);
	CREATE TABLE templates (id INTEGER PRIMARY KEY AUTOINCREMENT, template_id TEXT UNIQUE NOT NULL, template_version TEXT DEFAULT '1.0.0', template_category TEXT NOT NULL, name TEXT NOT NULL, description TEXT NOT NULL, url TEXT DEFAULT '', icon TEXT DEFAULT '', category TEXT NOT NULL, launch_type TEXT DEFAULT 'container', os_type TEXT DEFAULT 'linux', container_image TEXT, container_port INTEGER DEFAULT 8080, container_args TEXT DEFAULT '[]', tags TEXT DEFAULT '[]', maintainer TEXT, documentation_url TEXT, cpu_request TEXT DEFAULT '', cpu_limit TEXT DEFAULT '', memory_request TEXT DEFAULT '', memory_limit TEXT DEFAULT '', created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);
	CREATE TABLE app_specs (id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT DEFAULT '', image TEXT NOT NULL, launch_command TEXT DEFAULT '', cpu_request TEXT DEFAULT '', cpu_limit TEXT DEFAULT '', memory_request TEXT DEFAULT '', memory_limit TEXT DEFAULT '', env_vars TEXT DEFAULT '[]', volumes TEXT DEFAULT '[]', network_rules TEXT DEFAULT '[]', egress_policy TEXT DEFAULT '', tenant_id TEXT DEFAULT 'default', created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);
	CREATE TABLE oidc_states (state TEXT PRIMARY KEY, redirect_url TEXT NOT NULL DEFAULT '', expires_at DATETIME NOT NULL);
	CREATE TABLE categories (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, description TEXT DEFAULT '', tenant_id TEXT DEFAULT 'default', created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);
	CREATE TABLE category_admins (category_id TEXT NOT NULL, user_id TEXT NOT NULL, PRIMARY KEY (category_id, user_id));
	CREATE TABLE category_approved_users (category_id TEXT NOT NULL, user_id TEXT NOT NULL, PRIMARY KEY (category_id, user_id));
	CREATE TABLE recordings (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, user_id TEXT NOT NULL, filename TEXT NOT NULL, size_bytes INTEGER DEFAULT 0, duration_seconds REAL DEFAULT 0, format TEXT NOT NULL DEFAULT 'webm', storage_backend TEXT NOT NULL DEFAULT 'local', storage_path TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'recording', tenant_id TEXT DEFAULT 'default', created_at DATETIME DEFAULT CURRENT_TIMESTAMP, completed_at DATETIME, video_path TEXT DEFAULT '');
	CREATE TABLE session_shares (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, user_id TEXT NOT NULL DEFAULT '', permission TEXT NOT NULL DEFAULT 'read_only', share_token TEXT UNIQUE, created_by TEXT NOT NULL, created_at DATETIME DEFAULT CURRENT_TIMESTAMP);
	`
	if _, err := conn.Exec(schema); err != nil {
		t.Fatalf("failed to create old schema: %v", err)
	}

	// Create old custom schema_migrations table
	_, err = conn.Exec(`CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		t.Fatalf("failed to create old schema_migrations: %v", err)
	}
	conn.Exec("INSERT INTO schema_migrations (version) VALUES (1), (2), (3), (4), (5)")

	// Insert some test data
	conn.Exec("INSERT INTO applications (id, name, description, url, icon, category) VALUES ('app-1', 'TestApp', 'Test', 'http://x', 'icon', 'DevTools')")
	conn.Exec("INSERT INTO users (id, username, email, display_name, password_hash) VALUES ('user-1', 'admin', 'admin@test.com', 'Admin', '$hash')")
	conn.Close()

	// Now open via OpenDB — this should:
	// 1. Drop the old schema_migrations table
	// 2. Detect existing tables, force version to 1 (baseline)
	// 3. Run migration 000002 (data migration — creates categories)
	database, err := OpenDB("sqlite", tmpFile.Name())
	if err != nil {
		t.Fatalf("OpenDB() on old DB error = %v", err)
	}
	defer database.Close()

	// Verify data survived the upgrade
	app, err := database.GetApp("app-1")
	if err != nil {
		t.Fatalf("GetApp() error = %v", err)
	}
	if app == nil || app.Name != "TestApp" {
		t.Error("application data lost during upgrade")
	}

	user, err := database.GetUserByUsername("admin")
	if err != nil {
		t.Fatalf("GetUserByUsername() error = %v", err)
	}
	if user == nil {
		t.Error("user data lost during upgrade")
	}

	// Verify schema_migrations is in golang-migrate format
	var version int
	var dirty bool
	err = database.bun.DB.QueryRow("SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty)
	if err != nil {
		t.Fatalf("schema_migrations query error: %v", err)
	}
	if version != 2 {
		t.Errorf("version = %d, want 2 (both migrations applied)", version)
	}
	if dirty {
		t.Error("dirty = true, want false")
	}

	// Verify category was auto-created from app's category
	cat, err := database.GetCategoryByName("DevTools")
	if err != nil {
		t.Fatalf("GetCategoryByName error: %v", err)
	}
	if cat == nil {
		t.Error("expected DevTools category to be auto-created during data migration")
	}
}

// TestDataMigration_EmptyCategories verifies the data migration does not
// create categories for apps with empty category strings.
func TestDataMigration_EmptyCategories(t *testing.T) {
	if testDBType() != "sqlite" {
		t.Skip("SQLite-specific migration test")
	}
	tmpFile, err := os.CreateTemp("", "test-emptycat-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Create DB at version 1, insert apps with empty categories
	if err := runMigrations("sqlite", tmpFile.Name()); err != nil {
		t.Fatalf("runMigrations error: %v", err)
	}

	conn, err := sql.Open("sqlite", tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}

	// Insert app with empty category
	conn.Exec("INSERT INTO applications (id, name, description, url, icon, category) VALUES ('empty-1', 'NoCategory', 'test', 'http://x', 'i', '')")

	// Force version back to 1 to re-run data migration
	conn.Exec("UPDATE schema_migrations SET version = 1")
	conn.Close()

	// Re-run migrations (will run version 2 again)
	if err := runMigrations("sqlite", tmpFile.Name()); err != nil {
		t.Fatalf("re-run migrations error: %v", err)
	}

	// Verify no empty-name category was created
	conn, err = sql.Open("sqlite", tmpFile.Name())
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

// TestDataMigration_DuplicateCategories verifies that re-running the data
// migration does not create duplicate categories.
func TestDataMigration_DuplicateCategories(t *testing.T) {
	if testDBType() != "sqlite" {
		t.Skip("SQLite-specific migration test")
	}
	tmpFile, err := os.CreateTemp("", "test-dupecat-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Create DB, insert apps
	if err := runMigrations("sqlite", tmpFile.Name()); err != nil {
		t.Fatalf("runMigrations error: %v", err)
	}

	conn, err := sql.Open("sqlite", tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}

	conn.Exec("INSERT INTO applications (id, name, description, url, icon, category) VALUES ('d-1', 'App1', 't', 'http://x', 'i', 'Tools')")

	// Manually create the category first (simulating it already existing)
	conn.Exec("INSERT INTO categories (id, name, description, tenant_id) VALUES ('existing-cat', 'Tools', 'existing', 'default')")

	// Force version back and re-run
	conn.Exec("UPDATE schema_migrations SET version = 1")
	conn.Close()

	if err := runMigrations("sqlite", tmpFile.Name()); err != nil {
		t.Fatalf("re-run migrations error: %v", err)
	}

	conn, err = sql.Open("sqlite", tmpFile.Name())
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
		t.Errorf("found %d 'Tools' categories, want exactly 1 (INSERT OR IGNORE failed)", count)
	}
}

// TestSchemaColumns_Recordings verifies the recordings table has all expected
// columns including the video_path column added via later migrations.
func TestSchemaColumns_Recordings(t *testing.T) {
	if testDBType() != "sqlite" {
		t.Skip("SQLite-specific schema test")
	}
	tmpFile, err := os.CreateTemp("", "test-reccols-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	database, err := OpenDB("sqlite", tmpFile.Name())
	if err != nil {
		t.Fatalf("OpenDB() error = %v", err)
	}
	defer database.Close()

	expectedCols := []string{
		"id", "session_id", "user_id", "filename", "size_bytes",
		"duration_seconds", "format", "storage_backend", "storage_path",
		"status", "tenant_id", "created_at", "completed_at", "video_path",
	}

	rows, err := database.bun.DB.Query("SELECT name FROM pragma_table_info('recordings')")
	if err != nil {
		t.Fatalf("pragma_table_info error: %v", err)
	}
	defer rows.Close()

	existing := make(map[string]bool)
	for rows.Next() {
		var name string
		rows.Scan(&name)
		existing[name] = true
	}

	for _, col := range expectedCols {
		if !existing[col] {
			t.Errorf("recordings table missing column %q", col)
		}
	}
}

// TestSchemaColumns_Users verifies the users table has auth_provider and
// tenant-related columns from later migrations.
func TestSchemaColumns_Users(t *testing.T) {
	if testDBType() != "sqlite" {
		t.Skip("SQLite-specific schema test")
	}
	tmpFile, err := os.CreateTemp("", "test-usercols-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	database, err := OpenDB("sqlite", tmpFile.Name())
	if err != nil {
		t.Fatalf("OpenDB() error = %v", err)
	}
	defer database.Close()

	expectedCols := []string{
		"id", "username", "email", "display_name", "password_hash",
		"roles", "auth_provider", "auth_provider_id",
		"tenant_id", "tenant_roles",
		"created_at", "updated_at",
	}

	rows, err := database.bun.DB.Query("SELECT name FROM pragma_table_info('users')")
	if err != nil {
		t.Fatalf("pragma_table_info error: %v", err)
	}
	defer rows.Close()

	existing := make(map[string]bool)
	for rows.Next() {
		var name string
		rows.Scan(&name)
		existing[name] = true
	}

	for _, col := range expectedCols {
		if !existing[col] {
			t.Errorf("users table missing column %q", col)
		}
	}
}
