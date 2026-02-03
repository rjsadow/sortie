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
	ContainerImage string          `json:"container_image,omitempty"`
	ResourceLimits *ResourceLimits `json:"resource_limits,omitempty"` // Resource limits for container apps
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
	ID        string        `json:"id"`
	UserID    string        `json:"user_id"`
	AppID     string        `json:"app_id"`
	PodName   string        `json:"pod_name"`
	PodIP     string        `json:"pod_ip,omitempty"`
	Status    SessionStatus `json:"status"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
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
		category TEXT NOT NULL,
		launch_type TEXT NOT NULL DEFAULT 'url',
		container_image TEXT DEFAULT '',
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
	`
	_, err := db.conn.Exec(schema)
	if err != nil {
		return err
	}

	// Run migrations for existing databases (add new columns if they don't exist)
	migrations := []string{
		"ALTER TABLE applications ADD COLUMN launch_type TEXT NOT NULL DEFAULT 'url'",
		"ALTER TABLE applications ADD COLUMN container_image TEXT DEFAULT ''",
		"ALTER TABLE applications ADD COLUMN cpu_request TEXT DEFAULT ''",
		"ALTER TABLE applications ADD COLUMN cpu_limit TEXT DEFAULT ''",
		"ALTER TABLE applications ADD COLUMN memory_request TEXT DEFAULT ''",
		"ALTER TABLE applications ADD COLUMN memory_limit TEXT DEFAULT ''",
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
	rows, err := db.conn.Query("SELECT id, name, description, url, icon, category, launch_type, container_image, cpu_request, cpu_limit, memory_request, memory_limit FROM applications ORDER BY category, name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []Application
	for rows.Next() {
		var app Application
		var launchType, containerImage string
		var cpuRequest, cpuLimit, memoryRequest, memoryLimit string
		if err := rows.Scan(&app.ID, &app.Name, &app.Description, &app.URL, &app.Icon, &app.Category, &launchType, &containerImage, &cpuRequest, &cpuLimit, &memoryRequest, &memoryLimit); err != nil {
			return nil, err
		}
		app.LaunchType = LaunchType(launchType)
		if app.LaunchType == "" {
			app.LaunchType = LaunchTypeURL
		}
		app.ContainerImage = containerImage
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
	var launchType, containerImage string
	var cpuRequest, cpuLimit, memoryRequest, memoryLimit string
	err := db.conn.QueryRow(
		"SELECT id, name, description, url, icon, category, launch_type, container_image, cpu_request, cpu_limit, memory_request, memory_limit FROM applications WHERE id = ?",
		id,
	).Scan(&app.ID, &app.Name, &app.Description, &app.URL, &app.Icon, &app.Category, &launchType, &containerImage, &cpuRequest, &cpuLimit, &memoryRequest, &memoryLimit)

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
	app.ContainerImage = containerImage
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
	// Extract resource limits (use empty strings if not specified)
	var cpuRequest, cpuLimit, memoryRequest, memoryLimit string
	if app.ResourceLimits != nil {
		cpuRequest = app.ResourceLimits.CPURequest
		cpuLimit = app.ResourceLimits.CPULimit
		memoryRequest = app.ResourceLimits.MemoryRequest
		memoryLimit = app.ResourceLimits.MemoryLimit
	}
	_, err := db.conn.Exec(
		"INSERT INTO applications (id, name, description, url, icon, category, launch_type, container_image, cpu_request, cpu_limit, memory_request, memory_limit) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		app.ID, app.Name, app.Description, app.URL, app.Icon, app.Category, launchType, app.ContainerImage, cpuRequest, cpuLimit, memoryRequest, memoryLimit,
	)
	return err
}

// UpdateApp updates an existing application
func (db *DB) UpdateApp(app Application) error {
	launchType := string(app.LaunchType)
	if launchType == "" {
		launchType = string(LaunchTypeURL)
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
		"UPDATE applications SET name = ?, description = ?, url = ?, icon = ?, category = ?, launch_type = ?, container_image = ?, cpu_request = ?, cpu_limit = ?, memory_request = ?, memory_limit = ? WHERE id = ?",
		app.Name, app.Description, app.URL, app.Icon, app.Category, launchType, app.ContainerImage, cpuRequest, cpuLimit, memoryRequest, memoryLimit, app.ID,
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
		"INSERT INTO sessions (id, user_id, app_id, pod_name, pod_ip, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		session.ID, session.UserID, session.AppID, session.PodName, session.PodIP, string(session.Status), session.CreatedAt, session.UpdatedAt,
	)
	return err
}

// GetSession returns a session by ID
func (db *DB) GetSession(id string) (*Session, error) {
	var session Session
	var status string
	err := db.conn.QueryRow(
		"SELECT id, user_id, app_id, pod_name, pod_ip, status, created_at, updated_at FROM sessions WHERE id = ?",
		id,
	).Scan(&session.ID, &session.UserID, &session.AppID, &session.PodName, &session.PodIP, &status, &session.CreatedAt, &session.UpdatedAt)

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
		"SELECT id, user_id, app_id, pod_name, pod_ip, status, created_at, updated_at FROM sessions WHERE status NOT IN ('terminated', 'failed') ORDER BY created_at DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var session Session
		var status string
		if err := rows.Scan(&session.ID, &session.UserID, &session.AppID, &session.PodName, &session.PodIP, &status, &session.CreatedAt, &session.UpdatedAt); err != nil {
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
		"SELECT id, user_id, app_id, pod_name, pod_ip, status, created_at, updated_at FROM sessions WHERE user_id = ? AND status NOT IN ('terminated', 'failed') ORDER BY created_at DESC",
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
		if err := rows.Scan(&session.ID, &session.UserID, &session.AppID, &session.PodName, &session.PodIP, &status, &session.CreatedAt, &session.UpdatedAt); err != nil {
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

// GetStaleSessions returns sessions that have been in a non-terminal state for longer than the timeout
func (db *DB) GetStaleSessions(timeout time.Duration) ([]Session, error) {
	cutoff := time.Now().Add(-timeout)
	rows, err := db.conn.Query(
		"SELECT id, user_id, app_id, pod_name, pod_ip, status, created_at, updated_at FROM sessions WHERE status NOT IN ('terminated', 'failed') AND updated_at < ?",
		cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var session Session
		var status string
		if err := rows.Scan(&session.ID, &session.UserID, &session.AppID, &session.PodName, &session.PodIP, &status, &session.CreatedAt, &session.UpdatedAt); err != nil {
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
