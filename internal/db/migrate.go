package db

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed all:migrations/sqlite
var sqliteMigrations embed.FS

//go:embed all:migrations/postgres
var postgresMigrations embed.FS

// runMigrations executes all pending migrations for the given database type.
// It opens a separate connection for the migration to avoid golang-migrate
// closing the application's main connection via m.Close().
func runMigrations(dbType, dsn string) error {
	m, err := NewMigrator(dbType, dsn)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration failed: %w", err)
	}

	return nil
}

// newMigrator creates a golang-migrate instance for the given database type
// using embedded SQL migration files.
func newMigrator(conn *sql.DB, dbType string) (*migrate.Migrate, error) {
	var migrationFS fs.FS
	var err error

	switch dbType {
	case "sqlite":
		migrationFS, err = fs.Sub(sqliteMigrations, "migrations/sqlite")
	case "postgres":
		migrationFS, err = fs.Sub(postgresMigrations, "migrations/postgres")
	default:
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create sub filesystem: %w", err)
	}

	source, err := iofs.New(migrationFS, ".")
	if err != nil {
		return nil, fmt.Errorf("failed to create migration source: %w", err)
	}

	var driver database.Driver
	switch dbType {
	case "sqlite":
		driver, err = migratesqlite.WithInstance(conn, &migratesqlite.Config{})
		if err != nil {
			return nil, fmt.Errorf("failed to create sqlite driver: %w", err)
		}
	case "postgres":
		driver, err = migratepostgres.WithInstance(conn, &migratepostgres.Config{})
		if err != nil {
			return nil, fmt.Errorf("failed to create postgres driver: %w", err)
		}
	}

	m, err := migrate.NewWithInstance("iofs", source, dbType, driver)
	if err != nil {
		return nil, fmt.Errorf("failed to create migrator: %w", err)
	}

	return m, nil
}

// handleMigrationUpgrade detects existing databases that predate golang-migrate
// and sets up the schema_migrations table so that the baseline migration (000001)
// is marked as already applied.
func handleMigrationUpgrade(conn *sql.DB, dbType string) error {
	// Step 1: Check for and drop old custom schema_migrations table.
	// The old CLI tool used a table with (version INTEGER, applied_at DATETIME).
	// golang-migrate uses (version BIGINT, dirty BOOLEAN).
	if hasOldSchemaTable(conn, dbType) {
		slog.Info("detected old custom schema_migrations table, dropping it")
		if _, err := conn.Exec("DROP TABLE schema_migrations"); err != nil {
			return fmt.Errorf("failed to drop old schema_migrations table: %w", err)
		}
	}

	// Step 2: Check if application tables exist (meaning this is an existing database)
	// but golang-migrate's schema_migrations doesn't exist yet.
	if !hasGolangMigrateTable(conn, dbType) && hasApplicationTables(conn) {
		slog.Info("detected existing database without golang-migrate tracking, forcing version to baseline")
		if err := forceBaselineVersion(conn, dbType); err != nil {
			return fmt.Errorf("failed to force baseline version: %w", err)
		}
	}

	return nil
}

// hasOldSchemaTable checks if the old custom schema_migrations table exists.
// The old table has (version INTEGER, applied_at DATETIME) vs golang-migrate's
// (version BIGINT, dirty BOOLEAN).
func hasOldSchemaTable(conn *sql.DB, dbType string) bool {
	var query string
	switch dbType {
	case "sqlite":
		query = "SELECT COUNT(*) FROM pragma_table_info('schema_migrations') WHERE name = 'applied_at'"
	case "postgres":
		query = `SELECT COUNT(*) FROM information_schema.columns
			WHERE table_name = 'schema_migrations' AND column_name = 'applied_at'`
	default:
		return false
	}

	var count int
	if err := conn.QueryRow(query).Scan(&count); err != nil {
		return false
	}
	return count > 0
}

// hasGolangMigrateTable checks if golang-migrate's schema_migrations table exists.
// golang-migrate creates a table with (version BIGINT, dirty BOOLEAN).
func hasGolangMigrateTable(conn *sql.DB, dbType string) bool {
	var query string
	switch dbType {
	case "sqlite":
		query = "SELECT COUNT(*) FROM pragma_table_info('schema_migrations') WHERE name = 'dirty'"
	case "postgres":
		query = `SELECT COUNT(*) FROM information_schema.columns
			WHERE table_name = 'schema_migrations' AND column_name = 'dirty'`
	default:
		return false
	}

	var count int
	if err := conn.QueryRow(query).Scan(&count); err != nil {
		return false
	}
	return count > 0
}

// hasApplicationTables checks if the core application tables exist,
// indicating this is a pre-existing database.
func hasApplicationTables(conn *sql.DB) bool {
	var count int
	err := conn.QueryRow("SELECT COUNT(*) FROM applications LIMIT 1").Scan(&count)
	return err == nil
}

// forceBaselineVersion creates golang-migrate's schema_migrations table and
// sets version=1, dirty=false, indicating the baseline migration is already applied.
func forceBaselineVersion(conn *sql.DB, _ string) error {
	createTable := `CREATE TABLE IF NOT EXISTS schema_migrations (
		version bigint NOT NULL PRIMARY KEY,
		dirty boolean NOT NULL
	)`

	if _, err := conn.Exec(createTable); err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	// Force version 1 (baseline) as already applied.
	// Version 2 (data migration) will run normally via m.Up().
	if _, err := conn.Exec("INSERT INTO schema_migrations (version, dirty) VALUES (1, false)"); err != nil {
		return fmt.Errorf("failed to set baseline version: %w", err)
	}

	return nil
}

// NewMigrator creates an exported golang-migrate instance for use by the CLI tool.
// The caller is responsible for calling Close() on the returned Migrate instance.
func NewMigrator(dbType, dsn string) (*migrate.Migrate, error) {
	var driverName string
	switch dbType {
	case "sqlite":
		driverName = "sqlite"
	case "postgres":
		driverName = "postgres"
	default:
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}

	conn, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	return newMigrator(conn, dbType)
}
