package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

// Application represents an app in the launchpad
type Application struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Icon        string `json:"icon"`
	Category    string `json:"category"`
}

// AppConfig is the JSON structure for apps.json
type AppConfig struct {
	Applications []Application `json:"applications"`
}

// AuditLog represents an audit log entry
type AuditLog struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	User      string    `json:"user"`
	Action    string    `json:"action"`
	Details   string    `json:"details"`
}

// DB wraps the sql.DB connection
type DB struct {
	conn *sql.DB
}

// Open opens a SQLite database at the given path
func Open(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

// migrate creates the necessary tables
func (db *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS applications (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT NOT NULL,
		url TEXT NOT NULL,
		icon TEXT NOT NULL,
		category TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS audit_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		user TEXT NOT NULL,
		action TEXT NOT NULL,
		details TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS analytics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		app_id TEXT NOT NULL,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (app_id) REFERENCES applications(id)
	);

	CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_log(timestamp);
	CREATE INDEX IF NOT EXISTS idx_analytics_app_id ON analytics(app_id);
	CREATE INDEX IF NOT EXISTS idx_analytics_timestamp ON analytics(timestamp);
	`
	_, err := db.conn.Exec(schema)
	return err
}

// SeedFromJSON loads initial apps from a JSON file if the database is empty
func (db *DB) SeedFromJSON(jsonPath string) error {
	// Check if apps table is empty
	var count int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM applications").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to count applications: %w", err)
	}

	if count > 0 {
		return nil // Already seeded
	}

	// Read and parse JSON file
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("failed to read JSON file: %w", err)
	}

	var config AppConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Insert apps
	for _, app := range config.Applications {
		if err := db.CreateApp(app); err != nil {
			return fmt.Errorf("failed to insert app %s: %w", app.ID, err)
		}
	}

	return nil
}

// ListApps returns all applications
func (db *DB) ListApps() ([]Application, error) {
	rows, err := db.conn.Query("SELECT id, name, description, url, icon, category FROM applications ORDER BY category, name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []Application
	for rows.Next() {
		var app Application
		if err := rows.Scan(&app.ID, &app.Name, &app.Description, &app.URL, &app.Icon, &app.Category); err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}

	return apps, rows.Err()
}

// GetApp returns a single application by ID
func (db *DB) GetApp(id string) (*Application, error) {
	var app Application
	err := db.conn.QueryRow(
		"SELECT id, name, description, url, icon, category FROM applications WHERE id = ?",
		id,
	).Scan(&app.ID, &app.Name, &app.Description, &app.URL, &app.Icon, &app.Category)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &app, nil
}

// CreateApp inserts a new application
func (db *DB) CreateApp(app Application) error {
	_, err := db.conn.Exec(
		"INSERT INTO applications (id, name, description, url, icon, category) VALUES (?, ?, ?, ?, ?, ?)",
		app.ID, app.Name, app.Description, app.URL, app.Icon, app.Category,
	)
	return err
}

// UpdateApp updates an existing application
func (db *DB) UpdateApp(app Application) error {
	result, err := db.conn.Exec(
		"UPDATE applications SET name = ?, description = ?, url = ?, icon = ?, category = ? WHERE id = ?",
		app.Name, app.Description, app.URL, app.Icon, app.Category, app.ID,
	)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteApp removes an application by ID
func (db *DB) DeleteApp(id string) error {
	result, err := db.conn.Exec("DELETE FROM applications WHERE id = ?", id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// LogAudit creates an audit log entry
func (db *DB) LogAudit(user, action, details string) error {
	_, err := db.conn.Exec(
		"INSERT INTO audit_log (user, action, details) VALUES (?, ?, ?)",
		user, action, details,
	)
	return err
}

// GetAuditLogs returns recent audit log entries
func (db *DB) GetAuditLogs(limit int) ([]AuditLog, error) {
	rows, err := db.conn.Query(
		"SELECT id, timestamp, user, action, details FROM audit_log ORDER BY timestamp DESC LIMIT ?",
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		var log AuditLog
		if err := rows.Scan(&log.ID, &log.Timestamp, &log.User, &log.Action, &log.Details); err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}

	return logs, rows.Err()
}

// RecordLaunch records an app launch for analytics
func (db *DB) RecordLaunch(appID string) error {
	_, err := db.conn.Exec("INSERT INTO analytics (app_id) VALUES (?)", appID)
	return err
}

// AppStats represents analytics statistics for an application
type AppStats struct {
	AppID       string `json:"app_id"`
	AppName     string `json:"app_name"`
	LaunchCount int    `json:"launch_count"`
}

// AnalyticsStats represents overall analytics statistics
type AnalyticsStats struct {
	TotalLaunches int        `json:"total_launches"`
	AppStats      []AppStats `json:"app_stats"`
}

// GetAnalyticsStats returns analytics statistics
func (db *DB) GetAnalyticsStats() (*AnalyticsStats, error) {
	// Get total launches
	var totalLaunches int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM analytics").Scan(&totalLaunches)
	if err != nil {
		return nil, err
	}

	// Get per-app stats
	rows, err := db.conn.Query(`
		SELECT a.app_id, COALESCE(ap.name, a.app_id) as app_name, COUNT(*) as launch_count
		FROM analytics a
		LEFT JOIN applications ap ON a.app_id = ap.id
		GROUP BY a.app_id
		ORDER BY launch_count DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var appStats []AppStats
	for rows.Next() {
		var stat AppStats
		if err := rows.Scan(&stat.AppID, &stat.AppName, &stat.LaunchCount); err != nil {
			return nil, err
		}
		appStats = append(appStats, stat)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &AnalyticsStats{
		TotalLaunches: totalLaunches,
		AppStats:      appStats,
	}, nil
}
