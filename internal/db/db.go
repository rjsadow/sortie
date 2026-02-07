package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

// LaunchType represents how an application is launched
type LaunchType string

const (
	LaunchTypeURL       LaunchType = "url"
	LaunchTypeContainer LaunchType = "container"
	LaunchTypeWebProxy  LaunchType = "web_proxy"
)

// ResourceLimits defines CPU and memory resource constraints for container applications
type ResourceLimits struct {
	CPURequest    string `json:"cpu_request,omitempty"`    // CPU request (e.g., "100m", "0.5")
	CPULimit      string `json:"cpu_limit,omitempty"`      // CPU limit (e.g., "1", "2")
	MemoryRequest string `json:"memory_request,omitempty"` // Memory request (e.g., "256Mi", "1Gi")
	MemoryLimit   string `json:"memory_limit,omitempty"`   // Memory limit (e.g., "512Mi", "2Gi")
}

// Application represents an app in the launchpad
type Application struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	URL            string          `json:"url"`
	Icon           string          `json:"icon"`
	Category       string          `json:"category"`
	LaunchType     LaunchType      `json:"launch_type"`
	OsType         string          `json:"os_type,omitempty"`         // "linux" (default) or "windows"
	ContainerImage string          `json:"container_image,omitempty"`
	ContainerPort  int             `json:"container_port,omitempty"`  // Port web app listens on (default: 8080)
	ContainerArgs  []string        `json:"container_args,omitempty"`  // Extra arguments to pass to the container
	ResourceLimits *ResourceLimits `json:"resource_limits,omitempty"` // Resource limits for container apps
}

// AppConfig is the JSON structure for apps.json
type AppConfig struct {
	Applications []Application `json:"applications"`
}

// Template represents an application template in the marketplace
type Template struct {
	ID                int             `json:"id"`
	TemplateID        string          `json:"template_id"`
	TemplateVersion   string          `json:"template_version"`
	TemplateCategory  string          `json:"template_category"`
	Name              string          `json:"name"`
	Description       string          `json:"description"`
	URL               string          `json:"url"`
	Icon              string          `json:"icon"`
	Category          string          `json:"category"`
	LaunchType        string          `json:"launch_type"`
	OsType            string          `json:"os_type,omitempty"`
	ContainerImage    string          `json:"container_image,omitempty"`
	ContainerPort     int             `json:"container_port,omitempty"`
	ContainerArgs     []string        `json:"container_args,omitempty"`
	Tags              []string        `json:"tags"`
	Maintainer        string          `json:"maintainer,omitempty"`
	DocumentationURL  string          `json:"documentation_url,omitempty"`
	RecommendedLimits *ResourceLimits `json:"recommended_limits,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// TemplateCatalog is the JSON structure for templates.json
type TemplateCatalog struct {
	Version   string     `json:"version"`
	Templates []Template `json:"templates"`
}

// AuditLog represents an audit log entry
type AuditLog struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	User      string    `json:"user"`
	Action    string    `json:"action"`
	Details   string    `json:"details"`
}

// User represents a user account
type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email,omitempty"`
	DisplayName  string    `json:"display_name,omitempty"`
	PasswordHash string    `json:"-"`
	Roles        []string  `json:"roles"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// SessionStatus represents the status of a container session.
// Valid states: creating, running, failed, stopped, expired
// State machine:
//   creating -> running (pod ready)
//   creating -> failed  (pod creation failed)
//   running  -> stopped (user terminated)
//   running  -> expired (timeout cleanup)
//   running  -> failed  (runtime error)
type SessionStatus string

const (
	SessionStatusCreating SessionStatus = "creating"
	SessionStatusRunning  SessionStatus = "running"
	SessionStatusFailed   SessionStatus = "failed"
	SessionStatusStopped  SessionStatus = "stopped"
	SessionStatusExpired  SessionStatus = "expired"
)

// Session represents an active container session
type Session struct {
	ID          string        `json:"id"`
	UserID      string        `json:"user_id"`
	AppID       string        `json:"app_id"`
	PodName     string        `json:"pod_name"`
	PodIP       string        `json:"pod_ip,omitempty"`
	Status      SessionStatus `json:"status"`
	IdleTimeout int64         `json:"idle_timeout,omitempty"` // Per-session idle timeout in seconds (0 = use global default)
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
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

	// Configure SQLite for better concurrency
	// busy_timeout waits up to 5 seconds for locks to clear
	if _, err := conn.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to set busy_timeout: %w", err)
	}

	// WAL mode allows concurrent reads while writing
	if _, err := conn.Exec("PRAGMA journal_mode = WAL"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
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

// Ping verifies the database connection is alive.
func (db *DB) Ping() error {
	return db.conn.Ping()
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
		category TEXT NOT NULL,
		launch_type TEXT NOT NULL DEFAULT 'url',
		os_type TEXT DEFAULT 'linux',
		container_image TEXT DEFAULT '',
		container_port INTEGER DEFAULT 0,
		container_args TEXT DEFAULT '[]',
		cpu_request TEXT DEFAULT '',
		cpu_limit TEXT DEFAULT '',
		memory_request TEXT DEFAULT '',
		memory_limit TEXT DEFAULT ''
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

	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		app_id TEXT NOT NULL,
		pod_name TEXT NOT NULL,
		pod_ip TEXT DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending',
		idle_timeout INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (app_id) REFERENCES applications(id)
	);

	CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_log(timestamp);
	CREATE INDEX IF NOT EXISTS idx_analytics_app_id ON analytics(app_id);
	CREATE INDEX IF NOT EXISTS idx_analytics_timestamp ON analytics(timestamp);
	CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
	CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);

	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		username TEXT NOT NULL UNIQUE,
		email TEXT,
		display_name TEXT,
		password_hash TEXT NOT NULL,
		roles TEXT DEFAULT '["user"]',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);

	CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS templates (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		template_id TEXT UNIQUE NOT NULL,
		template_version TEXT NOT NULL DEFAULT '1.0.0',
		template_category TEXT NOT NULL,
		name TEXT NOT NULL,
		description TEXT NOT NULL,
		url TEXT DEFAULT '',
		icon TEXT DEFAULT '',
		category TEXT NOT NULL,
		launch_type TEXT NOT NULL DEFAULT 'container',
		os_type TEXT DEFAULT 'linux',
		container_image TEXT,
		container_port INTEGER DEFAULT 8080,
		container_args TEXT DEFAULT '[]',
		tags TEXT DEFAULT '[]',
		maintainer TEXT,
		documentation_url TEXT,
		cpu_request TEXT DEFAULT '',
		cpu_limit TEXT DEFAULT '',
		memory_request TEXT DEFAULT '',
		memory_limit TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_templates_template_id ON templates(template_id);
	CREATE INDEX IF NOT EXISTS idx_templates_category ON templates(template_category);
	`
	_, err := db.conn.Exec(schema)
	if err != nil {
		return err
	}

	// Run migrations for existing databases (add new columns if they don't exist)
	migrations := []string{
		"ALTER TABLE applications ADD COLUMN launch_type TEXT NOT NULL DEFAULT 'url'",
		"ALTER TABLE applications ADD COLUMN container_image TEXT DEFAULT ''",
		"ALTER TABLE applications ADD COLUMN container_port INTEGER DEFAULT 0",
		"ALTER TABLE applications ADD COLUMN container_args TEXT DEFAULT '[]'",
		"ALTER TABLE applications ADD COLUMN cpu_request TEXT DEFAULT ''",
		"ALTER TABLE applications ADD COLUMN cpu_limit TEXT DEFAULT ''",
		"ALTER TABLE applications ADD COLUMN memory_request TEXT DEFAULT ''",
		"ALTER TABLE applications ADD COLUMN memory_limit TEXT DEFAULT ''",
		"ALTER TABLE applications ADD COLUMN os_type TEXT DEFAULT 'linux'",
		"ALTER TABLE templates ADD COLUMN os_type TEXT DEFAULT 'linux'",
		"ALTER TABLE sessions ADD COLUMN idle_timeout INTEGER DEFAULT 0",
	}

	for _, migration := range migrations {
		// Ignore errors - column may already exist
		db.conn.Exec(migration)
	}

	return nil
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
	rows, err := db.conn.Query("SELECT id, name, description, url, icon, category, launch_type, os_type, container_image, container_port, container_args, cpu_request, cpu_limit, memory_request, memory_limit FROM applications ORDER BY category, name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []Application
	for rows.Next() {
		var app Application
		var launchType, osType, containerImage string
		var containerPort int
		var containerArgsJSON string
		var cpuRequest, cpuLimit, memoryRequest, memoryLimit string
		if err := rows.Scan(&app.ID, &app.Name, &app.Description, &app.URL, &app.Icon, &app.Category, &launchType, &osType, &containerImage, &containerPort, &containerArgsJSON, &cpuRequest, &cpuLimit, &memoryRequest, &memoryLimit); err != nil {
			return nil, err
		}
		app.LaunchType = LaunchType(launchType)
		if app.LaunchType == "" {
			app.LaunchType = LaunchTypeURL
		}
		app.OsType = osType
		if app.OsType == "" {
			app.OsType = "linux"
		}
		app.ContainerImage = containerImage
		app.ContainerPort = containerPort
		// Parse container args from JSON
		if containerArgsJSON != "" && containerArgsJSON != "[]" {
			json.Unmarshal([]byte(containerArgsJSON), &app.ContainerArgs)
		}
		// Set resource limits if any are specified
		if cpuRequest != "" || cpuLimit != "" || memoryRequest != "" || memoryLimit != "" {
			app.ResourceLimits = &ResourceLimits{
				CPURequest:    cpuRequest,
				CPULimit:      cpuLimit,
				MemoryRequest: memoryRequest,
				MemoryLimit:   memoryLimit,
			}
		}
		apps = append(apps, app)
	}

	return apps, rows.Err()
}

// GetApp returns a single application by ID
func (db *DB) GetApp(id string) (*Application, error) {
	var app Application
	var launchType, osType, containerImage string
	var containerPort int
	var containerArgsJSON string
	var cpuRequest, cpuLimit, memoryRequest, memoryLimit string
	err := db.conn.QueryRow(
		"SELECT id, name, description, url, icon, category, launch_type, os_type, container_image, container_port, container_args, cpu_request, cpu_limit, memory_request, memory_limit FROM applications WHERE id = ?",
		id,
	).Scan(&app.ID, &app.Name, &app.Description, &app.URL, &app.Icon, &app.Category, &launchType, &osType, &containerImage, &containerPort, &containerArgsJSON, &cpuRequest, &cpuLimit, &memoryRequest, &memoryLimit)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	app.LaunchType = LaunchType(launchType)
	if app.LaunchType == "" {
		app.LaunchType = LaunchTypeURL
	}
	app.OsType = osType
	if app.OsType == "" {
		app.OsType = "linux"
	}
	app.ContainerImage = containerImage
	app.ContainerPort = containerPort
	// Parse container args from JSON
	if containerArgsJSON != "" && containerArgsJSON != "[]" {
		json.Unmarshal([]byte(containerArgsJSON), &app.ContainerArgs)
	}
	// Set resource limits if any are specified
	if cpuRequest != "" || cpuLimit != "" || memoryRequest != "" || memoryLimit != "" {
		app.ResourceLimits = &ResourceLimits{
			CPURequest:    cpuRequest,
			CPULimit:      cpuLimit,
			MemoryRequest: memoryRequest,
			MemoryLimit:   memoryLimit,
		}
	}
	return &app, nil
}

// CreateApp inserts a new application
func (db *DB) CreateApp(app Application) error {
	launchType := string(app.LaunchType)
	if launchType == "" {
		launchType = string(LaunchTypeURL)
	}
	osType := app.OsType
	if osType == "" {
		osType = "linux"
	}
	// Serialize container args to JSON
	containerArgsJSON := "[]"
	if len(app.ContainerArgs) > 0 {
		if argsBytes, err := json.Marshal(app.ContainerArgs); err == nil {
			containerArgsJSON = string(argsBytes)
		}
	}
	// Extract resource limits (use empty strings if not specified)
	var cpuRequest, cpuLimit, memoryRequest, memoryLimit string
	if app.ResourceLimits != nil {
		cpuRequest = app.ResourceLimits.CPURequest
		cpuLimit = app.ResourceLimits.CPULimit
		memoryRequest = app.ResourceLimits.MemoryRequest
		memoryLimit = app.ResourceLimits.MemoryLimit
	}
	_, err := db.conn.Exec(
		"INSERT INTO applications (id, name, description, url, icon, category, launch_type, os_type, container_image, container_port, container_args, cpu_request, cpu_limit, memory_request, memory_limit) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		app.ID, app.Name, app.Description, app.URL, app.Icon, app.Category, launchType, osType, app.ContainerImage, app.ContainerPort, containerArgsJSON, cpuRequest, cpuLimit, memoryRequest, memoryLimit,
	)
	return err
}

// UpdateApp updates an existing application
func (db *DB) UpdateApp(app Application) error {
	launchType := string(app.LaunchType)
	if launchType == "" {
		launchType = string(LaunchTypeURL)
	}
	osType := app.OsType
	if osType == "" {
		osType = "linux"
	}
	// Serialize container args to JSON
	containerArgsJSON := "[]"
	if len(app.ContainerArgs) > 0 {
		if argsBytes, err := json.Marshal(app.ContainerArgs); err == nil {
			containerArgsJSON = string(argsBytes)
		}
	}
	// Extract resource limits (use empty strings if not specified)
	var cpuRequest, cpuLimit, memoryRequest, memoryLimit string
	if app.ResourceLimits != nil {
		cpuRequest = app.ResourceLimits.CPURequest
		cpuLimit = app.ResourceLimits.CPULimit
		memoryRequest = app.ResourceLimits.MemoryRequest
		memoryLimit = app.ResourceLimits.MemoryLimit
	}
	result, err := db.conn.Exec(
		"UPDATE applications SET name = ?, description = ?, url = ?, icon = ?, category = ?, launch_type = ?, os_type = ?, container_image = ?, container_port = ?, container_args = ?, cpu_request = ?, cpu_limit = ?, memory_request = ?, memory_limit = ? WHERE id = ?",
		app.Name, app.Description, app.URL, app.Icon, app.Category, launchType, osType, app.ContainerImage, app.ContainerPort, containerArgsJSON, cpuRequest, cpuLimit, memoryRequest, memoryLimit, app.ID,
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

// CreateSession creates a new session
func (db *DB) CreateSession(session Session) error {
	_, err := db.conn.Exec(
		"INSERT INTO sessions (id, user_id, app_id, pod_name, pod_ip, status, idle_timeout, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		session.ID, session.UserID, session.AppID, session.PodName, session.PodIP, string(session.Status), session.IdleTimeout, session.CreatedAt, session.UpdatedAt,
	)
	return err
}

// GetSession returns a session by ID
func (db *DB) GetSession(id string) (*Session, error) {
	var session Session
	var status string
	err := db.conn.QueryRow(
		"SELECT id, user_id, app_id, pod_name, pod_ip, status, idle_timeout, created_at, updated_at FROM sessions WHERE id = ?",
		id,
	).Scan(&session.ID, &session.UserID, &session.AppID, &session.PodName, &session.PodIP, &status, &session.IdleTimeout, &session.CreatedAt, &session.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	session.Status = SessionStatus(status)
	return &session, nil
}

// ListSessions returns all active sessions
func (db *DB) ListSessions() ([]Session, error) {
	rows, err := db.conn.Query(
		"SELECT id, user_id, app_id, pod_name, pod_ip, status, idle_timeout, created_at, updated_at FROM sessions WHERE status NOT IN ('terminated', 'failed') ORDER BY created_at DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var session Session
		var status string
		if err := rows.Scan(&session.ID, &session.UserID, &session.AppID, &session.PodName, &session.PodIP, &status, &session.IdleTimeout, &session.CreatedAt, &session.UpdatedAt); err != nil {
			return nil, err
		}
		session.Status = SessionStatus(status)
		sessions = append(sessions, session)
	}

	return sessions, rows.Err()
}

// ListSessionsByUser returns all sessions for a specific user
func (db *DB) ListSessionsByUser(userID string) ([]Session, error) {
	rows, err := db.conn.Query(
		"SELECT id, user_id, app_id, pod_name, pod_ip, status, idle_timeout, created_at, updated_at FROM sessions WHERE user_id = ? AND status NOT IN ('terminated', 'failed') ORDER BY created_at DESC",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var session Session
		var status string
		if err := rows.Scan(&session.ID, &session.UserID, &session.AppID, &session.PodName, &session.PodIP, &status, &session.IdleTimeout, &session.CreatedAt, &session.UpdatedAt); err != nil {
			return nil, err
		}
		session.Status = SessionStatus(status)
		sessions = append(sessions, session)
	}

	return sessions, rows.Err()
}

// UpdateSessionStatus updates the status of a session
func (db *DB) UpdateSessionStatus(id string, status SessionStatus) error {
	result, err := db.conn.Exec(
		"UPDATE sessions SET status = ?, updated_at = ? WHERE id = ?",
		string(status), time.Now(), id,
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

// UpdateSessionPodIP updates the pod IP of a session
func (db *DB) UpdateSessionPodIP(id string, podIP string) error {
	result, err := db.conn.Exec(
		"UPDATE sessions SET pod_ip = ?, updated_at = ? WHERE id = ?",
		podIP, time.Now(), id,
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

// UpdateSessionPodIPAndStatus updates both pod IP and status in a single operation
func (db *DB) UpdateSessionPodIPAndStatus(id string, podIP string, status SessionStatus) error {
	result, err := db.conn.Exec(
		"UPDATE sessions SET pod_ip = ?, status = ?, updated_at = ? WHERE id = ?",
		podIP, string(status), time.Now(), id,
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

// UpdateSessionRestart updates a session for restart with a new pod name and creating status
func (db *DB) UpdateSessionRestart(id string, podName string) error {
	result, err := db.conn.Exec(
		"UPDATE sessions SET pod_name = ?, pod_ip = '', status = ?, updated_at = ? WHERE id = ?",
		podName, string(SessionStatusCreating), time.Now(), id,
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

// DeleteSession removes a session by ID
func (db *DB) DeleteSession(id string) error {
	result, err := db.conn.Exec("DELETE FROM sessions WHERE id = ?", id)
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

// GetStaleSessions returns sessions that have exceeded their idle timeout.
// Sessions with a per-session idle_timeout use that value; others use the global default.
func (db *DB) GetStaleSessions(defaultTimeout time.Duration) ([]Session, error) {
	now := time.Now()
	defaultCutoff := now.Add(-defaultTimeout)

	// Select sessions that are active and have exceeded either their per-session or global timeout
	rows, err := db.conn.Query(
		`SELECT id, user_id, app_id, pod_name, pod_ip, status, idle_timeout, created_at, updated_at
		 FROM sessions
		 WHERE status NOT IN ('terminated', 'failed', 'stopped', 'expired')
		 AND (
		   (idle_timeout > 0 AND updated_at < datetime('now', '-' || idle_timeout || ' seconds'))
		   OR (idle_timeout = 0 AND updated_at < ?)
		 )`,
		defaultCutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var session Session
		var status string
		if err := rows.Scan(&session.ID, &session.UserID, &session.AppID, &session.PodName, &session.PodIP, &status, &session.IdleTimeout, &session.CreatedAt, &session.UpdatedAt); err != nil {
			return nil, err
		}
		session.Status = SessionStatus(status)
		sessions = append(sessions, session)
	}

	return sessions, rows.Err()
}

// CreateUser creates a new user in the database
func (db *DB) CreateUser(user User) error {
	rolesJSON, err := json.Marshal(user.Roles)
	if err != nil {
		return fmt.Errorf("failed to marshal roles: %w", err)
	}

	now := time.Now()
	_, err = db.conn.Exec(
		"INSERT INTO users (id, username, email, display_name, password_hash, roles, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		user.ID, user.Username, user.Email, user.DisplayName, user.PasswordHash, string(rolesJSON), now, now,
	)
	return err
}

// GetUserByID retrieves a user by their ID
func (db *DB) GetUserByID(id string) (*User, error) {
	var user User
	var rolesJSON string
	err := db.conn.QueryRow(
		"SELECT id, username, email, display_name, password_hash, roles, created_at, updated_at FROM users WHERE id = ?",
		id,
	).Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.PasswordHash, &rolesJSON, &user.CreatedAt, &user.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(rolesJSON), &user.Roles); err != nil {
		return nil, fmt.Errorf("failed to unmarshal roles: %w", err)
	}

	return &user, nil
}

// GetUserByUsername retrieves a user by their username
func (db *DB) GetUserByUsername(username string) (*User, error) {
	var user User
	var rolesJSON string
	err := db.conn.QueryRow(
		"SELECT id, username, email, display_name, password_hash, roles, created_at, updated_at FROM users WHERE username = ?",
		username,
	).Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.PasswordHash, &rolesJSON, &user.CreatedAt, &user.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(rolesJSON), &user.Roles); err != nil {
		return nil, fmt.Errorf("failed to unmarshal roles: %w", err)
	}

	return &user, nil
}

// UpdateUser updates an existing user
func (db *DB) UpdateUser(user User) error {
	rolesJSON, err := json.Marshal(user.Roles)
	if err != nil {
		return fmt.Errorf("failed to marshal roles: %w", err)
	}

	result, err := db.conn.Exec(
		"UPDATE users SET email = ?, display_name = ?, password_hash = ?, roles = ?, updated_at = ? WHERE id = ?",
		user.Email, user.DisplayName, user.PasswordHash, string(rolesJSON), time.Now(), user.ID,
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

// SeedAdminUser creates the admin user if it doesn't exist
func (db *DB) SeedAdminUser(username, passwordHash string) error {
	// Check if admin user already exists
	existing, err := db.GetUserByUsername(username)
	if err != nil {
		return fmt.Errorf("failed to check for existing admin: %w", err)
	}
	if existing != nil {
		return nil // Already exists
	}

	user := User{
		ID:           "admin-" + username,
		Username:     username,
		DisplayName:  "Administrator",
		PasswordHash: passwordHash,
		Roles:        []string{"admin", "user"},
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	return db.CreateUser(user)
}

// ListUsers returns all users
func (db *DB) ListUsers() ([]User, error) {
	rows, err := db.conn.Query(
		"SELECT id, username, email, display_name, password_hash, roles, created_at, updated_at FROM users ORDER BY created_at DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var user User
		var rolesJSON string
		if err := rows.Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.PasswordHash, &rolesJSON, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(rolesJSON), &user.Roles); err != nil {
			user.Roles = []string{"user"}
		}
		users = append(users, user)
	}

	return users, rows.Err()
}

// DeleteUser removes a user by ID
func (db *DB) DeleteUser(id string) error {
	result, err := db.conn.Exec("DELETE FROM users WHERE id = ?", id)
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

// GetSetting retrieves a setting value by key
func (db *DB) GetSetting(key string) (string, error) {
	var value string
	err := db.conn.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// SetSetting creates or updates a setting
func (db *DB) SetSetting(key, value string) error {
	_, err := db.conn.Exec(
		"INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = ?",
		key, value, time.Now(), value, time.Now(),
	)
	return err
}

// GetAllSettings retrieves all settings
func (db *DB) GetAllSettings() (map[string]string, error) {
	rows, err := db.conn.Query("SELECT key, value FROM settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		settings[key] = value
	}

	return settings, rows.Err()
}

// ListTemplates returns all templates
func (db *DB) ListTemplates() ([]Template, error) {
	rows, err := db.conn.Query(`
		SELECT id, template_id, template_version, template_category, name, description,
		       url, icon, category, launch_type, os_type, container_image, container_port,
		       container_args, tags, maintainer, documentation_url,
		       cpu_request, cpu_limit, memory_request, memory_limit, created_at, updated_at
		FROM templates ORDER BY template_category, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []Template
	for rows.Next() {
		var t Template
		var containerArgsJSON, tagsJSON string
		var cpuRequest, cpuLimit, memoryRequest, memoryLimit sql.NullString
		var maintainer, docURL sql.NullString
		var containerImage sql.NullString
		var containerPort sql.NullInt64
		var osType sql.NullString

		if err := rows.Scan(
			&t.ID, &t.TemplateID, &t.TemplateVersion, &t.TemplateCategory,
			&t.Name, &t.Description, &t.URL, &t.Icon, &t.Category, &t.LaunchType,
			&osType, &containerImage, &containerPort, &containerArgsJSON, &tagsJSON,
			&maintainer, &docURL,
			&cpuRequest, &cpuLimit, &memoryRequest, &memoryLimit,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}

		if osType.Valid && osType.String != "" {
			t.OsType = osType.String
		} else {
			t.OsType = "linux"
		}
		if containerImage.Valid {
			t.ContainerImage = containerImage.String
		}
		if containerPort.Valid {
			t.ContainerPort = int(containerPort.Int64)
		}
		if maintainer.Valid {
			t.Maintainer = maintainer.String
		}
		if docURL.Valid {
			t.DocumentationURL = docURL.String
		}

		// Parse JSON arrays
		if containerArgsJSON != "" && containerArgsJSON != "[]" {
			json.Unmarshal([]byte(containerArgsJSON), &t.ContainerArgs)
		}
		if tagsJSON != "" && tagsJSON != "[]" {
			json.Unmarshal([]byte(tagsJSON), &t.Tags)
		}

		// Set resource limits if any are specified
		if cpuRequest.Valid || cpuLimit.Valid || memoryRequest.Valid || memoryLimit.Valid {
			t.RecommendedLimits = &ResourceLimits{
				CPURequest:    cpuRequest.String,
				CPULimit:      cpuLimit.String,
				MemoryRequest: memoryRequest.String,
				MemoryLimit:   memoryLimit.String,
			}
		}

		templates = append(templates, t)
	}

	return templates, rows.Err()
}

// GetTemplate returns a single template by template_id
func (db *DB) GetTemplate(templateID string) (*Template, error) {
	var t Template
	var containerArgsJSON, tagsJSON string
	var cpuRequest, cpuLimit, memoryRequest, memoryLimit sql.NullString
	var maintainer, docURL sql.NullString
	var containerImage sql.NullString
	var containerPort sql.NullInt64
	var osType sql.NullString

	err := db.conn.QueryRow(`
		SELECT id, template_id, template_version, template_category, name, description,
		       url, icon, category, launch_type, os_type, container_image, container_port,
		       container_args, tags, maintainer, documentation_url,
		       cpu_request, cpu_limit, memory_request, memory_limit, created_at, updated_at
		FROM templates WHERE template_id = ?`, templateID).Scan(
		&t.ID, &t.TemplateID, &t.TemplateVersion, &t.TemplateCategory,
		&t.Name, &t.Description, &t.URL, &t.Icon, &t.Category, &t.LaunchType,
		&osType, &containerImage, &containerPort, &containerArgsJSON, &tagsJSON,
		&maintainer, &docURL,
		&cpuRequest, &cpuLimit, &memoryRequest, &memoryLimit,
		&t.CreatedAt, &t.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if osType.Valid && osType.String != "" {
		t.OsType = osType.String
	} else {
		t.OsType = "linux"
	}
	if containerImage.Valid {
		t.ContainerImage = containerImage.String
	}
	if containerPort.Valid {
		t.ContainerPort = int(containerPort.Int64)
	}
	if maintainer.Valid {
		t.Maintainer = maintainer.String
	}
	if docURL.Valid {
		t.DocumentationURL = docURL.String
	}

	// Parse JSON arrays
	if containerArgsJSON != "" && containerArgsJSON != "[]" {
		json.Unmarshal([]byte(containerArgsJSON), &t.ContainerArgs)
	}
	if tagsJSON != "" && tagsJSON != "[]" {
		json.Unmarshal([]byte(tagsJSON), &t.Tags)
	}

	// Set resource limits if any are specified
	if cpuRequest.Valid || cpuLimit.Valid || memoryRequest.Valid || memoryLimit.Valid {
		t.RecommendedLimits = &ResourceLimits{
			CPURequest:    cpuRequest.String,
			CPULimit:      cpuLimit.String,
			MemoryRequest: memoryRequest.String,
			MemoryLimit:   memoryLimit.String,
		}
	}

	return &t, nil
}

// CreateTemplate inserts a new template
func (db *DB) CreateTemplate(t Template) error {
	// Serialize arrays to JSON
	containerArgsJSON := "[]"
	if len(t.ContainerArgs) > 0 {
		if argsBytes, err := json.Marshal(t.ContainerArgs); err == nil {
			containerArgsJSON = string(argsBytes)
		}
	}
	tagsJSON := "[]"
	if len(t.Tags) > 0 {
		if tagsBytes, err := json.Marshal(t.Tags); err == nil {
			tagsJSON = string(tagsBytes)
		}
	}

	// Extract resource limits
	var cpuRequest, cpuLimit, memoryRequest, memoryLimit string
	if t.RecommendedLimits != nil {
		cpuRequest = t.RecommendedLimits.CPURequest
		cpuLimit = t.RecommendedLimits.CPULimit
		memoryRequest = t.RecommendedLimits.MemoryRequest
		memoryLimit = t.RecommendedLimits.MemoryLimit
	}

	osType := t.OsType
	if osType == "" {
		osType = "linux"
	}

	now := time.Now()
	_, err := db.conn.Exec(`
		INSERT INTO templates (template_id, template_version, template_category, name, description,
		                       url, icon, category, launch_type, os_type, container_image, container_port,
		                       container_args, tags, maintainer, documentation_url,
		                       cpu_request, cpu_limit, memory_request, memory_limit, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.TemplateID, t.TemplateVersion, t.TemplateCategory, t.Name, t.Description,
		t.URL, t.Icon, t.Category, t.LaunchType, osType, t.ContainerImage, t.ContainerPort,
		containerArgsJSON, tagsJSON, t.Maintainer, t.DocumentationURL,
		cpuRequest, cpuLimit, memoryRequest, memoryLimit, now, now,
	)
	return err
}

// UpdateTemplate updates an existing template
func (db *DB) UpdateTemplate(t Template) error {
	// Serialize arrays to JSON
	containerArgsJSON := "[]"
	if len(t.ContainerArgs) > 0 {
		if argsBytes, err := json.Marshal(t.ContainerArgs); err == nil {
			containerArgsJSON = string(argsBytes)
		}
	}
	tagsJSON := "[]"
	if len(t.Tags) > 0 {
		if tagsBytes, err := json.Marshal(t.Tags); err == nil {
			tagsJSON = string(tagsBytes)
		}
	}

	// Extract resource limits
	var cpuRequest, cpuLimit, memoryRequest, memoryLimit string
	if t.RecommendedLimits != nil {
		cpuRequest = t.RecommendedLimits.CPURequest
		cpuLimit = t.RecommendedLimits.CPULimit
		memoryRequest = t.RecommendedLimits.MemoryRequest
		memoryLimit = t.RecommendedLimits.MemoryLimit
	}

	osType := t.OsType
	if osType == "" {
		osType = "linux"
	}

	result, err := db.conn.Exec(`
		UPDATE templates SET template_version = ?, template_category = ?, name = ?, description = ?,
		                     url = ?, icon = ?, category = ?, launch_type = ?, os_type = ?, container_image = ?,
		                     container_port = ?, container_args = ?, tags = ?, maintainer = ?,
		                     documentation_url = ?, cpu_request = ?, cpu_limit = ?,
		                     memory_request = ?, memory_limit = ?, updated_at = ?
		WHERE template_id = ?`,
		t.TemplateVersion, t.TemplateCategory, t.Name, t.Description,
		t.URL, t.Icon, t.Category, t.LaunchType, osType, t.ContainerImage,
		t.ContainerPort, containerArgsJSON, tagsJSON, t.Maintainer,
		t.DocumentationURL, cpuRequest, cpuLimit, memoryRequest, memoryLimit,
		time.Now(), t.TemplateID,
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

// DeleteTemplate removes a template by template_id
func (db *DB) DeleteTemplate(templateID string) error {
	result, err := db.conn.Exec("DELETE FROM templates WHERE template_id = ?", templateID)
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

// SeedTemplatesFromJSON loads templates from a JSON file if the templates table is empty
func (db *DB) SeedTemplatesFromJSON(jsonPath string) error {
	// Read JSON file
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("failed to read templates JSON file: %w", err)
	}

	return db.SeedTemplatesFromData(data)
}

// SeedTemplatesFromData loads templates from JSON data if the templates table is empty
func (db *DB) SeedTemplatesFromData(data []byte) error {
	// Check if templates table is empty
	var count int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM templates").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to count templates: %w", err)
	}

	if count > 0 {
		return nil // Already seeded
	}

	var catalog TemplateCatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return fmt.Errorf("failed to parse templates JSON: %w", err)
	}

	// Insert templates
	for _, t := range catalog.Templates {
		if err := db.CreateTemplate(t); err != nil {
			return fmt.Errorf("failed to insert template %s: %w", t.TemplateID, err)
		}
	}

	return nil
}
