package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/rjsadow/sortie/internal/plugins"
	_ "modernc.org/sqlite"
)

// SQLiteStorage implements StorageProvider using SQLite.
type SQLiteStorage struct {
	conn   *sql.DB
	config map[string]string
	dbPath string
}

func init() {
	plugins.RegisterGlobal(plugins.PluginTypeStorage, "sqlite", func() plugins.Plugin {
		return NewSQLiteStorage()
	})
}

// NewSQLiteStorage creates a new SQLite storage provider.
func NewSQLiteStorage() *SQLiteStorage {
	return &SQLiteStorage{}
}

// Name returns the plugin name.
func (s *SQLiteStorage) Name() string {
	return "sqlite"
}

// Type returns the plugin type.
func (s *SQLiteStorage) Type() plugins.PluginType {
	return plugins.PluginTypeStorage
}

// Version returns the plugin version.
func (s *SQLiteStorage) Version() string {
	return "1.0.0"
}

// Description returns a human-readable description.
func (s *SQLiteStorage) Description() string {
	return "SQLite database storage provider"
}

// Initialize sets up the plugin with configuration.
func (s *SQLiteStorage) Initialize(ctx context.Context, config map[string]string) error {
	s.config = config

	// Get database path from config or default
	s.dbPath = config["db_path"]
	if s.dbPath == "" {
		s.dbPath = os.Getenv("SORTIE_DB")
	}
	if s.dbPath == "" {
		s.dbPath = "sortie.db"
	}

	// Open database connection
	conn, err := sql.Open("sqlite", s.dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	s.conn = conn

	// Run migrations
	if err := s.migrate(); err != nil {
		conn.Close()
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	// Seed from JSON if configured
	if seedPath := config["seed_path"]; seedPath != "" {
		if err := s.seedFromJSON(seedPath); err != nil {
			log.Printf("Warning: failed to seed from JSON: %v", err)
		}
	}

	log.Printf("SQLite storage initialized at %s", s.dbPath)
	return nil
}

// Healthy returns true if the plugin is operational.
func (s *SQLiteStorage) Healthy(ctx context.Context) bool {
	if s.conn == nil {
		return false
	}
	return s.conn.PingContext(ctx) == nil
}

// Close releases resources.
func (s *SQLiteStorage) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// migrate creates the necessary tables.
func (s *SQLiteStorage) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS applications (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		url TEXT NOT NULL,
		icon TEXT DEFAULT '',
		category TEXT DEFAULT '',
		launch_type TEXT NOT NULL DEFAULT 'url',
		container_image TEXT DEFAULT '',
		cpu_request TEXT DEFAULT '',
		cpu_limit TEXT DEFAULT '',
		memory_request TEXT DEFAULT '',
		memory_limit TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		app_id TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'creating',
		pod_name TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		expires_at DATETIME
	);

	CREATE TABLE IF NOT EXISTS audit_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id TEXT NOT NULL,
		action TEXT NOT NULL,
		details TEXT DEFAULT '',
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS analytics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		app_id TEXT NOT NULL,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
	CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);
	CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_log(timestamp);
	CREATE INDEX IF NOT EXISTS idx_analytics_app_id ON analytics(app_id);
	`

	_, err := s.conn.Exec(schema)
	return err
}

// seedFromJSON seeds the database from a JSON file.
func (s *SQLiteStorage) seedFromJSON(path string) error {
	// Check if database already has apps
	var count int
	if err := s.conn.QueryRow("SELECT COUNT(*) FROM applications").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil // Already seeded
	}

	// Read JSON file
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var config struct {
		Applications []plugins.Application `json:"applications"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}

	// Insert applications
	ctx := context.Background()
	for _, app := range config.Applications {
		if err := s.CreateApp(ctx, &app); err != nil {
			log.Printf("Warning: failed to seed app %s: %v", app.ID, err)
		}
	}

	log.Printf("Seeded %d applications from %s", len(config.Applications), path)
	return nil
}

// Application CRUD

// CreateApp creates a new application.
func (s *SQLiteStorage) CreateApp(ctx context.Context, app *plugins.Application) error {
	var cpuReq, cpuLim, memReq, memLim string
	if app.ResourceLimits != nil {
		cpuReq = app.ResourceLimits.CPURequest
		cpuLim = app.ResourceLimits.CPULimit
		memReq = app.ResourceLimits.MemoryRequest
		memLim = app.ResourceLimits.MemoryLimit
	}

	_, err := s.conn.ExecContext(ctx, `
		INSERT INTO applications (id, name, description, url, icon, category, launch_type, container_image,
			cpu_request, cpu_limit, memory_request, memory_limit)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		app.ID, app.Name, app.Description, app.URL, app.Icon, app.Category,
		string(app.LaunchType), app.ContainerImage,
		cpuReq, cpuLim, memReq, memLim)
	if err != nil {
		return fmt.Errorf("failed to create app: %w", err)
	}
	return nil
}

// GetApp retrieves an application by ID.
func (s *SQLiteStorage) GetApp(ctx context.Context, id string) (*plugins.Application, error) {
	var app plugins.Application
	var launchType string
	var cpuReq, cpuLim, memReq, memLim string

	err := s.conn.QueryRowContext(ctx, `
		SELECT id, name, description, url, icon, category, launch_type, container_image,
			cpu_request, cpu_limit, memory_request, memory_limit, created_at, updated_at
		FROM applications WHERE id = ?`, id).Scan(
		&app.ID, &app.Name, &app.Description, &app.URL, &app.Icon, &app.Category,
		&launchType, &app.ContainerImage,
		&cpuReq, &cpuLim, &memReq, &memLim,
		&app.CreatedAt, &app.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get app: %w", err)
	}

	app.LaunchType = plugins.LaunchType(launchType)
	if cpuReq != "" || cpuLim != "" || memReq != "" || memLim != "" {
		app.ResourceLimits = &plugins.ResourceLimits{
			CPURequest:    cpuReq,
			CPULimit:      cpuLim,
			MemoryRequest: memReq,
			MemoryLimit:   memLim,
		}
	}

	return &app, nil
}

// UpdateApp updates an existing application.
func (s *SQLiteStorage) UpdateApp(ctx context.Context, app *plugins.Application) error {
	var cpuReq, cpuLim, memReq, memLim string
	if app.ResourceLimits != nil {
		cpuReq = app.ResourceLimits.CPURequest
		cpuLim = app.ResourceLimits.CPULimit
		memReq = app.ResourceLimits.MemoryRequest
		memLim = app.ResourceLimits.MemoryLimit
	}

	result, err := s.conn.ExecContext(ctx, `
		UPDATE applications SET name = ?, description = ?, url = ?, icon = ?, category = ?,
			launch_type = ?, container_image = ?,
			cpu_request = ?, cpu_limit = ?, memory_request = ?, memory_limit = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`,
		app.Name, app.Description, app.URL, app.Icon, app.Category,
		string(app.LaunchType), app.ContainerImage,
		cpuReq, cpuLim, memReq, memLim,
		app.ID)
	if err != nil {
		return fmt.Errorf("failed to update app: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return plugins.ErrResourceNotFound
	}

	return nil
}

// DeleteApp deletes an application.
func (s *SQLiteStorage) DeleteApp(ctx context.Context, id string) error {
	result, err := s.conn.ExecContext(ctx, "DELETE FROM applications WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete app: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return plugins.ErrResourceNotFound
	}

	return nil
}

// ListApps lists all applications.
func (s *SQLiteStorage) ListApps(ctx context.Context) ([]*plugins.Application, error) {
	rows, err := s.conn.QueryContext(ctx, `
		SELECT id, name, description, url, icon, category, launch_type, container_image,
			cpu_request, cpu_limit, memory_request, memory_limit, created_at, updated_at
		FROM applications ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("failed to list apps: %w", err)
	}
	defer rows.Close()

	var apps []*plugins.Application
	for rows.Next() {
		var app plugins.Application
		var launchType string
		var cpuReq, cpuLim, memReq, memLim string

		if err := rows.Scan(
			&app.ID, &app.Name, &app.Description, &app.URL, &app.Icon, &app.Category,
			&launchType, &app.ContainerImage,
			&cpuReq, &cpuLim, &memReq, &memLim,
			&app.CreatedAt, &app.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan app: %w", err)
		}

		app.LaunchType = plugins.LaunchType(launchType)
		if cpuReq != "" || cpuLim != "" || memReq != "" || memLim != "" {
			app.ResourceLimits = &plugins.ResourceLimits{
				CPURequest:    cpuReq,
				CPULimit:      cpuLim,
				MemoryRequest: memReq,
				MemoryLimit:   memLim,
			}
		}

		apps = append(apps, &app)
	}

	return apps, nil
}

// Session management

// CreateSession creates a new session.
func (s *SQLiteStorage) CreateSession(ctx context.Context, session *plugins.Session) error {
	_, err := s.conn.ExecContext(ctx, `
		INSERT INTO sessions (id, user_id, app_id, status, pod_name, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		session.ID, session.UserID, session.AppID, string(session.Status),
		session.PodName, session.ExpiresAt)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	return nil
}

// GetSession retrieves a session by ID.
func (s *SQLiteStorage) GetSession(ctx context.Context, id string) (*plugins.Session, error) {
	var session plugins.Session
	var status string

	err := s.conn.QueryRowContext(ctx, `
		SELECT id, user_id, app_id, status, pod_name, created_at, expires_at
		FROM sessions WHERE id = ?`, id).Scan(
		&session.ID, &session.UserID, &session.AppID, &status,
		&session.PodName, &session.CreatedAt, &session.ExpiresAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	session.Status = plugins.LaunchStatus(status)
	return &session, nil
}

// UpdateSession updates an existing session.
func (s *SQLiteStorage) UpdateSession(ctx context.Context, session *plugins.Session) error {
	result, err := s.conn.ExecContext(ctx, `
		UPDATE sessions SET status = ?, pod_name = ?, expires_at = ?
		WHERE id = ?`,
		string(session.Status), session.PodName, session.ExpiresAt, session.ID)
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return plugins.ErrResourceNotFound
	}

	return nil
}

// DeleteSession deletes a session.
func (s *SQLiteStorage) DeleteSession(ctx context.Context, id string) error {
	result, err := s.conn.ExecContext(ctx, "DELETE FROM sessions WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return plugins.ErrResourceNotFound
	}

	return nil
}

// ListSessions lists sessions, optionally filtered by user ID.
func (s *SQLiteStorage) ListSessions(ctx context.Context, userID string) ([]*plugins.Session, error) {
	var rows *sql.Rows
	var err error

	if userID != "" {
		rows, err = s.conn.QueryContext(ctx, `
			SELECT id, user_id, app_id, status, pod_name, created_at, expires_at
			FROM sessions WHERE user_id = ? ORDER BY created_at DESC`, userID)
	} else {
		rows, err = s.conn.QueryContext(ctx, `
			SELECT id, user_id, app_id, status, pod_name, created_at, expires_at
			FROM sessions ORDER BY created_at DESC`)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*plugins.Session
	for rows.Next() {
		var session plugins.Session
		var status string

		if err := rows.Scan(
			&session.ID, &session.UserID, &session.AppID, &status,
			&session.PodName, &session.CreatedAt, &session.ExpiresAt); err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		session.Status = plugins.LaunchStatus(status)
		sessions = append(sessions, &session)
	}

	return sessions, nil
}

// ListExpiredSessions lists sessions that have exceeded their expiry time.
func (s *SQLiteStorage) ListExpiredSessions(ctx context.Context) ([]*plugins.Session, error) {
	rows, err := s.conn.QueryContext(ctx, `
		SELECT id, user_id, app_id, status, pod_name, created_at, expires_at
		FROM sessions WHERE expires_at IS NOT NULL AND expires_at < CURRENT_TIMESTAMP
		AND status IN ('creating', 'running')`)
	if err != nil {
		return nil, fmt.Errorf("failed to list expired sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*plugins.Session
	for rows.Next() {
		var session plugins.Session
		var status string

		if err := rows.Scan(
			&session.ID, &session.UserID, &session.AppID, &status,
			&session.PodName, &session.CreatedAt, &session.ExpiresAt); err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		session.Status = plugins.LaunchStatus(status)
		sessions = append(sessions, &session)
	}

	return sessions, nil
}

// Audit logging

// LogAudit logs an audit entry.
func (s *SQLiteStorage) LogAudit(ctx context.Context, entry *plugins.AuditEntry) error {
	_, err := s.conn.ExecContext(ctx, `
		INSERT INTO audit_log (user_id, action, details)
		VALUES (?, ?, ?)`,
		entry.UserID, entry.Action, entry.Details)
	if err != nil {
		return fmt.Errorf("failed to log audit: %w", err)
	}
	return nil
}

// GetAuditLogs retrieves recent audit log entries.
func (s *SQLiteStorage) GetAuditLogs(ctx context.Context, limit int) ([]*plugins.AuditEntry, error) {
	rows, err := s.conn.QueryContext(ctx, `
		SELECT id, user_id, action, details, timestamp
		FROM audit_log ORDER BY timestamp DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get audit logs: %w", err)
	}
	defer rows.Close()

	var entries []*plugins.AuditEntry
	for rows.Next() {
		var entry plugins.AuditEntry
		var id int64

		if err := rows.Scan(&id, &entry.UserID, &entry.Action, &entry.Details, &entry.Timestamp); err != nil {
			return nil, fmt.Errorf("failed to scan audit entry: %w", err)
		}

		entry.ID = fmt.Sprintf("%d", id)
		entries = append(entries, &entry)
	}

	return entries, nil
}

// Analytics

// RecordLaunch records an app launch for analytics.
func (s *SQLiteStorage) RecordLaunch(ctx context.Context, appID string) error {
	_, err := s.conn.ExecContext(ctx, `
		INSERT INTO analytics (app_id) VALUES (?)`, appID)
	if err != nil {
		return fmt.Errorf("failed to record launch: %w", err)
	}
	return nil
}

// GetAnalyticsStats returns analytics statistics.
func (s *SQLiteStorage) GetAnalyticsStats(ctx context.Context) (map[string]any, error) {
	stats := make(map[string]any)

	// Total launches
	var totalLaunches int64
	if err := s.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM analytics").Scan(&totalLaunches); err != nil {
		return nil, fmt.Errorf("failed to get total launches: %w", err)
	}
	stats["total_launches"] = totalLaunches

	// Total apps
	var totalApps int64
	if err := s.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM applications").Scan(&totalApps); err != nil {
		return nil, fmt.Errorf("failed to get total apps: %w", err)
	}
	stats["total_apps"] = totalApps

	// Launches per app (top 10)
	rows, err := s.conn.QueryContext(ctx, `
		SELECT a.app_id, COALESCE(ap.name, a.app_id), COUNT(*) as launches
		FROM analytics a
		LEFT JOIN applications ap ON a.app_id = ap.id
		GROUP BY a.app_id
		ORDER BY launches DESC
		LIMIT 10`)
	if err != nil {
		return nil, fmt.Errorf("failed to get launches per app: %w", err)
	}
	defer rows.Close()

	var topApps []map[string]any
	for rows.Next() {
		var appID, name string
		var launches int64
		if err := rows.Scan(&appID, &name, &launches); err != nil {
			continue
		}
		topApps = append(topApps, map[string]any{
			"app_id":   appID,
			"name":     name,
			"launches": launches,
		})
	}
	stats["top_apps"] = topApps

	return stats, nil
}

// GetConnection returns the underlying database connection.
// This can be used for advanced operations or testing.
func (s *SQLiteStorage) GetConnection() *sql.DB {
	return s.conn
}

// Verify interface compliance
var _ plugins.StorageProvider = (*SQLiteStorage)(nil)
