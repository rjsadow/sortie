package db

import (
	"testing"
)

// TestPostgresSchema_AllTablesExist verifies all 15 application tables plus
// schema_migrations are present after a full migration.
func TestPostgresSchema_AllTablesExist(t *testing.T) {
	if testDBType() != "postgres" {
		t.Skip("Postgres-specific schema test")
	}
	dsn := testPostgresDSN(t)
	resetPostgresDB(t, dsn)

	database, err := OpenDB("postgres", dsn)
	if err != nil {
		t.Fatalf("OpenDB() error = %v", err)
	}
	defer database.Close()

	expectedTables := []string{
		"applications", "audit_log", "analytics", "sessions",
		"users", "settings", "templates", "app_specs",
		"oidc_states", "tenants", "categories",
		"category_admins", "category_approved_users",
		"recordings", "session_shares",
		"schema_migrations",
	}

	for _, table := range expectedTables {
		var exists bool
		err := database.bun.DB.QueryRow(
			`SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = $1
			)`, table).Scan(&exists)
		if err != nil {
			t.Errorf("query for table %q error: %v", table, err)
			continue
		}
		if !exists {
			t.Errorf("table %q does not exist in public schema", table)
		}
	}
}

// TestPostgresSchema_ApplicationsColumns verifies the applications table has
// all 18 expected columns with correct Postgres types and defaults.
func TestPostgresSchema_ApplicationsColumns(t *testing.T) {
	if testDBType() != "postgres" {
		t.Skip("Postgres-specific schema test")
	}
	dsn := testPostgresDSN(t)
	resetPostgresDB(t, dsn)

	database, err := OpenDB("postgres", dsn)
	if err != nil {
		t.Fatalf("OpenDB() error = %v", err)
	}
	defer database.Close()

	type colInfo struct {
		name       string
		dataType   string
		isNullable string
		dfltVal    *string
	}

	rows, err := database.bun.DB.Query(`
		SELECT column_name, data_type, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'applications'
		ORDER BY ordinal_position
	`)
	if err != nil {
		t.Fatalf("information_schema query error: %v", err)
	}
	defer rows.Close()

	cols := make(map[string]colInfo)
	for rows.Next() {
		var c colInfo
		if err := rows.Scan(&c.name, &c.dataType, &c.isNullable, &c.dfltVal); err != nil {
			t.Fatalf("scan error: %v", err)
		}
		cols[c.name] = c
	}

	// Verify all expected columns exist
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

	if len(cols) != 18 {
		t.Errorf("applications table has %d columns, want 18", len(cols))
	}

	// Verify specific column properties
	if c, ok := cols["visibility"]; ok {
		if c.isNullable != "NO" {
			t.Error("applications.visibility should be NOT NULL")
		}
		if c.dfltVal == nil || *c.dfltVal != "'public'::text" {
			val := "<nil>"
			if c.dfltVal != nil {
				val = *c.dfltVal
			}
			t.Errorf("applications.visibility default = %v, want 'public'::text", val)
		}
	}
	if c, ok := cols["launch_type"]; ok {
		if c.isNullable != "NO" {
			t.Error("applications.launch_type should be NOT NULL")
		}
		if c.dfltVal == nil || *c.dfltVal != "'url'::text" {
			val := "<nil>"
			if c.dfltVal != nil {
				val = *c.dfltVal
			}
			t.Errorf("applications.launch_type default = %v, want 'url'::text", val)
		}
	}
	if c, ok := cols["tenant_id"]; ok {
		if c.dfltVal == nil || *c.dfltVal != "'default'::text" {
			val := "<nil>"
			if c.dfltVal != nil {
				val = *c.dfltVal
			}
			t.Errorf("applications.tenant_id default = %v, want 'default'::text", val)
		}
	}
	if c, ok := cols["os_type"]; ok {
		if c.dfltVal == nil || *c.dfltVal != "'linux'::text" {
			val := "<nil>"
			if c.dfltVal != nil {
				val = *c.dfltVal
			}
			t.Errorf("applications.os_type default = %v, want 'linux'::text", val)
		}
	}
}

// TestPostgresSchema_AllTableColumnCounts verifies that every table has the
// expected column count, catching missing or extra columns from SQL file typos.
func TestPostgresSchema_AllTableColumnCounts(t *testing.T) {
	if testDBType() != "postgres" {
		t.Skip("Postgres-specific schema test")
	}
	dsn := testPostgresDSN(t)
	resetPostgresDB(t, dsn)

	database, err := OpenDB("postgres", dsn)
	if err != nil {
		t.Fatalf("OpenDB() error = %v", err)
	}
	defer database.Close()

	// Same expected column counts as SQLite tests
	expectedColumnCounts := map[string]int{
		"applications":            18,
		"audit_log":               6,
		"analytics":               4,
		"sessions":                10,
		"users":                   12,
		"settings":                3,
		"templates":               23,
		"app_specs":               16,
		"oidc_states":             3,
		"tenants":                 7,
		"categories":              6,
		"category_admins":         2,
		"category_approved_users": 2,
		"recordings":              14,
		"session_shares":          7,
	}

	for table, expected := range expectedColumnCounts {
		var count int
		err := database.bun.DB.QueryRow(`
			SELECT COUNT(*) FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = $1
		`, table).Scan(&count)
		if err != nil {
			t.Errorf("column count query for %q error: %v", table, err)
			continue
		}
		if count != expected {
			t.Errorf("table %q has %d columns, want %d", table, count, expected)
		}
	}
}

// TestPostgresSchema_Indexes verifies that all expected indexes are present.
func TestPostgresSchema_Indexes(t *testing.T) {
	if testDBType() != "postgres" {
		t.Skip("Postgres-specific schema test")
	}
	dsn := testPostgresDSN(t)
	resetPostgresDB(t, dsn)

	database, err := OpenDB("postgres", dsn)
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

	// Query all indexes from pg_indexes
	rows, err := database.bun.DB.Query(`
		SELECT indexname FROM pg_indexes
		WHERE schemaname = 'public'
	`)
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
