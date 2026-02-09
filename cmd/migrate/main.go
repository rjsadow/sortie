package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

var migrationPattern = regexp.MustCompile(`^(\d+)_.*\.(up|down)\.sql$`)

type migration struct {
	version int
	name    string
	upFile  string
	downFile string
}

func main() {
	dbPath := flag.String("db", "sortie.db", "Path to SQLite database")
	migrationsDir := flag.String("dir", "migrations", "Path to migrations directory")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Println("Usage: migrate [up|down|status] [-db path] [-dir migrations]")
		os.Exit(1)
	}

	command := flag.Arg(0)

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if err := ensureMigrationsTable(db); err != nil {
		log.Fatalf("Failed to create migrations table: %v", err)
	}

	migrations, err := loadMigrations(*migrationsDir)
	if err != nil {
		log.Fatalf("Failed to load migrations: %v", err)
	}

	switch command {
	case "up":
		if err := migrateUp(db, migrations, *migrationsDir); err != nil {
			log.Fatalf("Migration failed: %v", err)
		}
	case "down":
		if err := migrateDown(db, migrations, *migrationsDir); err != nil {
			log.Fatalf("Rollback failed: %v", err)
		}
	case "status":
		if err := showStatus(db, migrations); err != nil {
			log.Fatalf("Failed to show status: %v", err)
		}
	default:
		fmt.Printf("Unknown command: %s\n", command)
		fmt.Println("Usage: migrate [up|down|status] [-db path] [-dir migrations]")
		os.Exit(1)
	}
}

func ensureMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

func loadMigrations(dir string) ([]migration, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	migrationMap := make(map[int]*migration)

	for _, f := range files {
		if f.IsDir() {
			continue
		}

		matches := migrationPattern.FindStringSubmatch(f.Name())
		if matches == nil {
			continue
		}

		version, _ := strconv.Atoi(matches[1])
		direction := matches[2]

		if _, exists := migrationMap[version]; !exists {
			name := strings.TrimSuffix(strings.TrimSuffix(f.Name(), ".up.sql"), ".down.sql")
			name = strings.TrimPrefix(name, matches[1]+"_")
			migrationMap[version] = &migration{
				version: version,
				name:    name,
			}
		}

		if direction == "up" {
			migrationMap[version].upFile = f.Name()
		} else {
			migrationMap[version].downFile = f.Name()
		}
	}

	var migrations []migration
	for _, m := range migrationMap {
		migrations = append(migrations, *m)
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	return migrations, nil
}

func getAppliedVersions(db *sql.DB) (map[int]bool, error) {
	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}

	return applied, rows.Err()
}

func migrateUp(db *sql.DB, migrations []migration, dir string) error {
	applied, err := getAppliedVersions(db)
	if err != nil {
		return err
	}

	count := 0
	for _, m := range migrations {
		if applied[m.version] {
			continue
		}

		if m.upFile == "" {
			return fmt.Errorf("migration %d has no up file", m.version)
		}

		content, err := os.ReadFile(filepath.Join(dir, m.upFile))
		if err != nil {
			return fmt.Errorf("failed to read migration %d: %w", m.version, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d failed: %w", m.version, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.version); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %d: %w", m.version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %d: %w", m.version, err)
		}

		fmt.Printf("Applied migration %03d: %s\n", m.version, m.name)
		count++
	}

	if count == 0 {
		fmt.Println("No migrations to apply")
	} else {
		fmt.Printf("Applied %d migration(s)\n", count)
	}

	return nil
}

func migrateDown(db *sql.DB, migrations []migration, dir string) error {
	applied, err := getAppliedVersions(db)
	if err != nil {
		return err
	}

	// Find the highest applied migration
	var maxVersion int
	for v := range applied {
		if v > maxVersion {
			maxVersion = v
		}
	}

	if maxVersion == 0 {
		fmt.Println("No migrations to rollback")
		return nil
	}

	// Find the migration to rollback
	var target *migration
	for i := range migrations {
		if migrations[i].version == maxVersion {
			target = &migrations[i]
			break
		}
	}

	if target == nil {
		return fmt.Errorf("migration %d not found in migrations directory", maxVersion)
	}

	if target.downFile == "" {
		return fmt.Errorf("migration %d has no down file", maxVersion)
	}

	content, err := os.ReadFile(filepath.Join(dir, target.downFile))
	if err != nil {
		return fmt.Errorf("failed to read rollback for migration %d: %w", maxVersion, err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	if _, err := tx.Exec(string(content)); err != nil {
		tx.Rollback()
		return fmt.Errorf("rollback of migration %d failed: %w", maxVersion, err)
	}

	if _, err := tx.Exec("DELETE FROM schema_migrations WHERE version = ?", maxVersion); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to remove migration record %d: %w", maxVersion, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit rollback of migration %d: %w", maxVersion, err)
	}

	fmt.Printf("Rolled back migration %03d: %s\n", target.version, target.name)
	return nil
}

func showStatus(db *sql.DB, migrations []migration) error {
	applied, err := getAppliedVersions(db)
	if err != nil {
		return err
	}

	fmt.Println("Migration Status:")
	fmt.Println("-----------------")

	for _, m := range migrations {
		status := "pending"
		if applied[m.version] {
			status = "applied"
		}
		fmt.Printf("%03d: %-30s [%s]\n", m.version, m.name, status)
	}

	return nil
}
