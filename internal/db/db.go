package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/dialect/sqlitedialect"

	_ "modernc.org/sqlite"
)

// ctx returns a background context for bun queries.
func ctx() context.Context { return context.Background() }

// CategoryVisibility controls who can see apps in a category
type CategoryVisibility string

const (
	CategoryVisibilityPublic    CategoryVisibility = "public"
	CategoryVisibilityApproved  CategoryVisibility = "approved"
	CategoryVisibilityAdminOnly CategoryVisibility = "admin_only"
)

// Category represents a first-class category with access controls
type Category struct {
	bun.BaseModel `bun:"table:categories"`

	ID          string    `json:"id" bun:"id,pk"`
	Name        string    `json:"name" bun:"name,notnull"`
	Description string    `json:"description" bun:"description"`
	TenantID    string    `json:"tenant_id,omitempty" bun:"tenant_id"`
	CreatedAt   time.Time `json:"created_at" bun:"created_at,nullzero,notnull,default:current_timestamp"`
	UpdatedAt   time.Time `json:"updated_at" bun:"updated_at,nullzero,notnull,default:current_timestamp"`
}

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

// Application represents an app in the sortie
type Application struct {
	bun.BaseModel `bun:"table:applications"`

	ID             string             `json:"id" bun:"id,pk"`
	Name           string             `json:"name" bun:"name,notnull"`
	Description    string             `json:"description" bun:"description"`
	URL            string             `json:"url" bun:"url"`
	Icon           string             `json:"icon" bun:"icon"`
	Category       string             `json:"category" bun:"category"`
	Visibility     CategoryVisibility `json:"visibility" bun:"visibility"`
	LaunchType     LaunchType         `json:"launch_type" bun:"launch_type"`
	OsType         string             `json:"os_type,omitempty" bun:"os_type"`
	ContainerImage string             `json:"container_image,omitempty" bun:"container_image"`
	ContainerPort  int                `json:"container_port,omitempty" bun:"container_port"`
	ContainerArgs  []string           `json:"container_args,omitempty" bun:"-"`
	ResourceLimits *ResourceLimits    `json:"resource_limits,omitempty" bun:"-"`
	EgressPolicy   *EgressPolicy      `json:"egress_policy,omitempty" bun:"-"`
	TenantID       string             `json:"tenant_id,omitempty" bun:"tenant_id"`

	// Flattened DB columns for ResourceLimits (not exported to JSON)
	CPURequest    string `json:"-" bun:"cpu_request"`
	CPULimit      string `json:"-" bun:"cpu_limit"`
	MemoryRequest string `json:"-" bun:"memory_request"`
	MemoryLimit   string `json:"-" bun:"memory_limit"`

	// JSON-serialized DB columns
	ContainerArgsJSON string `json:"-" bun:"container_args"`
	EgressPolicyJSON  string `json:"-" bun:"egress_policy"`
}

// AppConfig is the JSON structure for apps.json
type AppConfig struct {
	Applications []Application `json:"applications"`
}

// Template represents an application template in the marketplace
type Template struct {
	bun.BaseModel `bun:"table:templates"`

	ID                int             `json:"id" bun:"id,pk,autoincrement"`
	TemplateID        string          `json:"template_id" bun:"template_id,unique,notnull"`
	TemplateVersion   string          `json:"template_version" bun:"template_version"`
	TemplateCategory  string          `json:"template_category" bun:"template_category"`
	Name              string          `json:"name" bun:"name,notnull"`
	Description       string          `json:"description" bun:"description"`
	URL               string          `json:"url" bun:"url"`
	Icon              string          `json:"icon" bun:"icon"`
	Category          string          `json:"category" bun:"category"`
	LaunchType        string          `json:"launch_type" bun:"launch_type"`
	OsType            string          `json:"os_type,omitempty" bun:"os_type"`
	ContainerImage    string          `json:"container_image,omitempty" bun:"container_image"`
	ContainerPort     int             `json:"container_port,omitempty" bun:"container_port"`
	ContainerArgs     []string        `json:"container_args,omitempty" bun:"-"`
	Tags              []string        `json:"tags" bun:"-"`
	Maintainer        string          `json:"maintainer,omitempty" bun:"maintainer"`
	DocumentationURL  string          `json:"documentation_url,omitempty" bun:"documentation_url"`
	RecommendedLimits *ResourceLimits `json:"recommended_limits,omitempty" bun:"-"`
	CreatedAt         time.Time       `json:"created_at" bun:"created_at,nullzero,notnull,default:current_timestamp"`
	UpdatedAt         time.Time       `json:"updated_at" bun:"updated_at,nullzero,notnull,default:current_timestamp"`

	// Flattened DB columns for RecommendedLimits (not exported to JSON)
	CPURequest    string `json:"-" bun:"cpu_request"`
	CPULimit      string `json:"-" bun:"cpu_limit"`
	MemoryRequest string `json:"-" bun:"memory_request"`
	MemoryLimit   string `json:"-" bun:"memory_limit"`

	// JSON-serialized DB columns
	ContainerArgsJSON string `json:"-" bun:"container_args"`
	TagsJSON          string `json:"-" bun:"tags"`
}

// TemplateCatalog is the JSON structure for templates.json
type TemplateCatalog struct {
	Version   string     `json:"version"`
	Templates []Template `json:"templates"`
}

// AuditLog represents an audit log entry
type AuditLog struct {
	bun.BaseModel `bun:"table:audit_log"`

	ID        int64     `json:"id" bun:"id,pk,autoincrement"`
	Timestamp time.Time `json:"timestamp" bun:"timestamp,nullzero,notnull,default:current_timestamp"`
	User      string    `json:"user" bun:"user"`
	Action    string    `json:"action" bun:"action"`
	Details   string    `json:"details" bun:"details"`
	TenantID  string    `json:"-" bun:"tenant_id"`
}

// User represents a user account
type User struct {
	bun.BaseModel `bun:"table:users"`

	ID             string    `json:"id" bun:"id,pk"`
	Username       string    `json:"username" bun:"username,unique,notnull"`
	Email          string    `json:"email,omitempty" bun:"email"`
	DisplayName    string    `json:"display_name,omitempty" bun:"display_name"`
	PasswordHash   string    `json:"-" bun:"password_hash"`
	Roles          []string  `json:"roles" bun:"-"`
	AuthProvider   string    `json:"auth_provider,omitempty" bun:"auth_provider"`
	AuthProviderID string    `json:"auth_provider_id,omitempty" bun:"auth_provider_id"`
	TenantID       string    `json:"tenant_id,omitempty" bun:"tenant_id"`
	TenantRoles    []string  `json:"tenant_roles,omitempty" bun:"-"`
	CreatedAt      time.Time `json:"created_at" bun:"created_at,nullzero,notnull,default:current_timestamp"`
	UpdatedAt      time.Time `json:"updated_at" bun:"updated_at,nullzero,notnull,default:current_timestamp"`

	// JSON-serialized DB columns
	RolesJSON       string `json:"-" bun:"roles"`
	TenantRolesJSON string `json:"-" bun:"tenant_roles"`
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
	bun.BaseModel `bun:"table:sessions"`

	ID          string        `json:"id" bun:"id,pk"`
	UserID      string        `json:"user_id" bun:"user_id,notnull"`
	AppID       string        `json:"app_id" bun:"app_id,notnull"`
	PodName     string        `json:"pod_name" bun:"pod_name"`
	PodIP       string        `json:"pod_ip,omitempty" bun:"pod_ip"`
	Status      SessionStatus `json:"status" bun:"status,notnull"`
	IdleTimeout int64         `json:"idle_timeout,omitempty" bun:"idle_timeout"`
	TenantID    string        `json:"tenant_id,omitempty" bun:"tenant_id"`
	CreatedAt   time.Time     `json:"created_at" bun:"created_at,nullzero,notnull,default:current_timestamp"`
	UpdatedAt   time.Time     `json:"updated_at" bun:"updated_at,nullzero,notnull,default:current_timestamp"`
}

// EnvVar represents an environment variable for an AppSpec
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// VolumeMount represents a volume mount for an AppSpec
type VolumeMount struct {
	Name      string `json:"name"`
	MountPath string `json:"mount_path"`
	Size      string `json:"size,omitempty"` // e.g., "1Gi"
	ReadOnly  bool   `json:"read_only,omitempty"`
}

// NetworkRule represents a network access rule for an AppSpec
type NetworkRule struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol,omitempty"` // "TCP" (default) or "UDP"
	AllowFrom string `json:"allow_from,omitempty"` // CIDR or empty for all
}

// EgressRule defines a single network egress rule
type EgressRule struct {
	CIDR     string `json:"cidr"`                // Destination CIDR (e.g., "10.0.0.0/8", "0.0.0.0/0")
	Port     int    `json:"port,omitempty"`      // Destination port (0 = all ports)
	Protocol string `json:"protocol,omitempty"`  // "TCP", "UDP", or empty for both
}

// EgressPolicy defines the network egress policy for an application.
// Mode "allowlist" permits only DNS and listed destinations (default, most secure).
// Mode "denylist" permits all traffic except listed destinations.
// Mode "" (empty) inherits the cluster-level default NetworkPolicy.
type EgressPolicy struct {
	Mode  string       `json:"mode,omitempty"`  // "allowlist" or "denylist"; empty = inherit cluster default
	Rules []EgressRule `json:"rules,omitempty"` // Egress rules
}

// AppSpec defines an application specification for launching containers
type AppSpec struct {
	bun.BaseModel `bun:"table:app_specs"`

	ID            string          `json:"id" bun:"id,pk"`
	Name          string          `json:"name" bun:"name,notnull"`
	Description   string          `json:"description,omitempty" bun:"description"`
	Image         string          `json:"image" bun:"image,notnull"`
	LaunchCommand string          `json:"launch_command,omitempty" bun:"launch_command"`
	Resources     *ResourceLimits `json:"resources,omitempty" bun:"-"`
	EnvVars       []EnvVar        `json:"env_vars,omitempty" bun:"-"`
	Volumes       []VolumeMount   `json:"volumes,omitempty" bun:"-"`
	NetworkRules  []NetworkRule   `json:"network_rules,omitempty" bun:"-"`
	EgressPolicy  *EgressPolicy   `json:"egress_policy,omitempty" bun:"-"`
	CreatedAt     time.Time       `json:"created_at" bun:"created_at,nullzero,notnull,default:current_timestamp"`
	UpdatedAt     time.Time       `json:"updated_at" bun:"updated_at,nullzero,notnull,default:current_timestamp"`

	// Flattened DB columns for Resources (not exported to JSON)
	CPURequest    string `json:"-" bun:"cpu_request"`
	CPULimit      string `json:"-" bun:"cpu_limit"`
	MemoryRequest string `json:"-" bun:"memory_request"`
	MemoryLimit   string `json:"-" bun:"memory_limit"`

	// JSON-serialized DB columns
	EnvVarsJSON      string `json:"-" bun:"env_vars"`
	VolumesJSON      string `json:"-" bun:"volumes"`
	NetworkRulesJSON string `json:"-" bun:"network_rules"`
	EgressPolicyJSON string `json:"-" bun:"egress_policy"`
}

// Setting represents a key-value setting
type Setting struct {
	bun.BaseModel `bun:"table:settings"`

	Key       string    `bun:"key,pk"`
	Value     string    `bun:"value,notnull"`
	UpdatedAt time.Time `bun:"updated_at"`
}

// OIDCState represents an OIDC CSRF state token
type OIDCState struct {
	bun.BaseModel `bun:"table:oidc_states"`

	State       string    `bun:"state,pk"`
	RedirectURL string    `bun:"redirect_url,notnull"`
	ExpiresAt   time.Time `bun:"expires_at,notnull"`
}

// Analytics represents an analytics launch event
type Analytics struct {
	bun.BaseModel `bun:"table:analytics"`

	ID        int64     `bun:"id,pk,autoincrement"`
	AppID     string    `bun:"app_id,notnull"`
	Timestamp time.Time `bun:"timestamp,nullzero,default:current_timestamp"`
	TenantID  string    `bun:"tenant_id"`
}

// CategoryAdmin represents a junction table entry for category admins
type CategoryAdmin struct {
	bun.BaseModel `bun:"table:category_admins"`

	CategoryID string `bun:"category_id,pk"`
	UserID     string `bun:"user_id,pk"`
}

// CategoryApprovedUser represents a junction table entry for approved users
type CategoryApprovedUser struct {
	bun.BaseModel `bun:"table:category_approved_users"`

	CategoryID string `bun:"category_id,pk"`
	UserID     string `bun:"user_id,pk"`
}

// DB wraps the bun.DB connection
type DB struct {
	bun    *bun.DB
	dbType string
}

// DBType returns the database type ("sqlite" or "postgres").
func (db *DB) DBType() string {
	return db.dbType
}

// Open opens a SQLite database at the given path.
// This is a convenience wrapper around OpenDB for backward compatibility.
func Open(dbPath string) (*DB, error) {
	return OpenDB("sqlite", dbPath)
}

// OpenDB opens a database connection for the given type and DSN,
// runs any pending migrations, and returns the DB handle.
func OpenDB(dbType, dsn string) (*DB, error) {
	var driverName string
	switch dbType {
	case "sqlite":
		driverName = "sqlite"
	case "postgres":
		driverName = "postgres"
	default:
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}

	// For SQLite in-memory databases, use shared cache so that the migration
	// connection (opened separately by golang-migrate) sees the same database.
	migrateDSN := dsn
	if dbType == "sqlite" && dsn == ":memory:" {
		dsn = "file::memory:?cache=shared"
		migrateDSN = dsn
	}

	conn, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure SQLite-specific settings
	if dbType == "sqlite" {
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

		// Keep at least one connection open to prevent in-memory databases
		// from being destroyed when all connections close.
		conn.SetMaxIdleConns(1)
	}

	// Handle upgrade from pre-golang-migrate databases
	if err := handleMigrationUpgrade(conn, dbType); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to handle migration upgrade: %w", err)
	}

	// Run all pending migrations (uses its own connection to avoid m.Close() side effects)
	if err := runMigrations(dbType, migrateDSN); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	// Wrap with bun using the appropriate dialect
	var bunDB *bun.DB
	switch dbType {
	case "sqlite":
		bunDB = bun.NewDB(conn, sqlitedialect.New())
	case "postgres":
		bunDB = bun.NewDB(conn, pgdialect.New())
	}

	return &DB{bun: bunDB, dbType: dbType}, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.bun.Close()
}

// Ping verifies the database connection is alive.
func (db *DB) Ping() error {
	return db.bun.PingContext(ctx())
}

// SeedFromJSON loads initial apps from a JSON file if the database is empty
func (db *DB) SeedFromJSON(jsonPath string) error {
	count, err := db.bun.NewSelect().Model((*Application)(nil)).Count(ctx())
	if err != nil {
		return fmt.Errorf("failed to count applications: %w", err)
	}

	if count > 0 {
		return nil // Already seeded
	}

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("failed to read JSON file: %w", err)
	}

	var config AppConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	for _, app := range config.Applications {
		if err := db.CreateApp(app); err != nil {
			return fmt.Errorf("failed to insert app %s: %w", app.ID, err)
		}
	}

	return nil
}

// ListApps returns all applications
func (db *DB) ListApps() ([]Application, error) {
	var apps []Application
	err := db.bun.NewSelect().Model(&apps).OrderExpr("category, name").Scan(ctx())
	return apps, err
}

// GetApp returns a single application by ID
func (db *DB) GetApp(id string) (*Application, error) {
	var app Application
	err := db.bun.NewSelect().Model(&app).Where("id = ?", id).Scan(ctx())
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
	_, err := db.bun.NewInsert().Model(&app).Exec(ctx())
	return err
}

// UpdateApp updates an existing application
func (db *DB) UpdateApp(app Application) error {
	result, err := db.bun.NewUpdate().Model(&app).WherePK().Exec(ctx())
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
	result, err := db.bun.NewDelete().Model((*Application)(nil)).Where("id = ?", id).Exec(ctx())
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
	entry := AuditLog{
		User:    user,
		Action:  action,
		Details: details,
	}
	_, err := db.bun.NewInsert().Model(&entry).Exec(ctx())
	return err
}

// GetAuditLogs returns recent audit log entries
func (db *DB) GetAuditLogs(limit int) ([]AuditLog, error) {
	var logs []AuditLog
	err := db.bun.NewSelect().Model(&logs).
		OrderExpr("timestamp DESC").
		Limit(limit).
		Scan(ctx())
	return logs, err
}

// AuditLogFilter holds query parameters for filtering audit logs
type AuditLogFilter struct {
	User   string
	Action string
	From   time.Time
	To     time.Time
	Limit  int
	Offset int
}

// AuditLogPage holds a page of audit log results with total count
type AuditLogPage struct {
	Logs  []AuditLog `json:"logs"`
	Total int        `json:"total"`
}

// QueryAuditLogs returns audit logs matching the given filter with pagination
func (db *DB) QueryAuditLogs(filter AuditLogFilter) (*AuditLogPage, error) {
	q := db.bun.NewSelect().Model((*AuditLog)(nil))

	if filter.User != "" {
		q = q.Where("\"user\" = ?", filter.User)
	}
	if filter.Action != "" {
		q = q.Where("action = ?", filter.Action)
	}
	if !filter.From.IsZero() {
		q = q.Where("timestamp >= ?", filter.From)
	}
	if !filter.To.IsZero() {
		q = q.Where("timestamp <= ?", filter.To)
	}

	// Get total count
	total, err := q.Count(ctx())
	if err != nil {
		return nil, fmt.Errorf("failed to count audit logs: %w", err)
	}

	// Apply pagination defaults
	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	offset := max(filter.Offset, 0)

	var logs []AuditLog
	err = q.OrderExpr("timestamp DESC").
		Limit(limit).
		Offset(offset).
		Scan(ctx(), &logs)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit logs: %w", err)
	}

	return &AuditLogPage{Logs: logs, Total: total}, nil
}

// GetAuditLogActions returns all distinct action values in the audit log
func (db *DB) GetAuditLogActions() ([]string, error) {
	var actions []string
	err := db.bun.NewSelect().Model((*AuditLog)(nil)).
		ColumnExpr("DISTINCT action").
		OrderExpr("action").
		Scan(ctx(), &actions)
	return actions, err
}

// GetAuditLogUsers returns all distinct user values in the audit log
func (db *DB) GetAuditLogUsers() ([]string, error) {
	var users []string
	err := db.bun.NewSelect().Model((*AuditLog)(nil)).
		ColumnExpr("DISTINCT \"user\"").
		OrderExpr("\"user\"").
		Scan(ctx(), &users)
	return users, err
}

// RecordLaunch records an app launch for analytics
func (db *DB) RecordLaunch(appID string) error {
	entry := Analytics{AppID: appID}
	_, err := db.bun.NewInsert().Model(&entry).Exec(ctx())
	return err
}

// AppStats represents analytics statistics for an application
type AppStats struct {
	AppID       string `json:"app_id" bun:"app_id"`
	AppName     string `json:"app_name" bun:"app_name"`
	LaunchCount int    `json:"launch_count" bun:"launch_count"`
}

// AnalyticsStats represents overall analytics statistics
type AnalyticsStats struct {
	TotalLaunches int        `json:"total_launches"`
	AppStats      []AppStats `json:"app_stats"`
}

// GetAnalyticsStats returns analytics statistics
func (db *DB) GetAnalyticsStats() (*AnalyticsStats, error) {
	// Get total launches
	totalLaunches, err := db.bun.NewSelect().Model((*Analytics)(nil)).Count(ctx())
	if err != nil {
		return nil, err
	}

	// Get per-app stats
	var appStats []AppStats
	err = db.bun.NewRaw(`
		SELECT a.app_id, COALESCE(ap.name, a.app_id) as app_name, COUNT(*) as launch_count
		FROM analytics a
		LEFT JOIN applications ap ON a.app_id = ap.id
		GROUP BY a.app_id
		ORDER BY launch_count DESC
	`).Scan(ctx(), &appStats)
	if err != nil {
		return nil, err
	}

	return &AnalyticsStats{
		TotalLaunches: totalLaunches,
		AppStats:      appStats,
	}, nil
}

// CreateSession creates a new session
func (db *DB) CreateSession(session Session) error {
	if session.TenantID == "" {
		session.TenantID = DefaultTenantID
	}
	_, err := db.bun.NewInsert().Model(&session).Exec(ctx())
	return err
}

// GetSession returns a session by ID
func (db *DB) GetSession(id string) (*Session, error) {
	var session Session
	err := db.bun.NewSelect().Model(&session).Where("id = ?", id).Scan(ctx())
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// ListSessions returns all active sessions
func (db *DB) ListSessions() ([]Session, error) {
	var sessions []Session
	err := db.bun.NewSelect().Model(&sessions).
		Where("status NOT IN ('terminated', 'failed')").
		OrderExpr("created_at DESC").
		Scan(ctx())
	return sessions, err
}

// ListSessionsByUser returns all sessions for a specific user
func (db *DB) ListSessionsByUser(userID string) ([]Session, error) {
	var sessions []Session
	err := db.bun.NewSelect().Model(&sessions).
		Where("user_id = ?", userID).
		Where("status NOT IN ('terminated', 'failed')").
		OrderExpr("created_at DESC").
		Scan(ctx())
	return sessions, err
}

// UpdateSessionStatus updates the status of a session
func (db *DB) UpdateSessionStatus(id string, status SessionStatus) error {
	result, err := db.bun.NewUpdate().Model((*Session)(nil)).
		Set("status = ?", status).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", id).
		Exec(ctx())
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
	result, err := db.bun.NewUpdate().Model((*Session)(nil)).
		Set("pod_ip = ?", podIP).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", id).
		Exec(ctx())
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
	result, err := db.bun.NewUpdate().Model((*Session)(nil)).
		Set("pod_ip = ?", podIP).
		Set("status = ?", status).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", id).
		Exec(ctx())
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
	result, err := db.bun.NewUpdate().Model((*Session)(nil)).
		Set("pod_name = ?", podName).
		Set("pod_ip = ''").
		Set("status = ?", SessionStatusCreating).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", id).
		Exec(ctx())
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
	result, err := db.bun.NewDelete().Model((*Session)(nil)).Where("id = ?", id).Exec(ctx())
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

// CountActiveSessionsByUser returns the number of active (creating or running) sessions for a user.
func (db *DB) CountActiveSessionsByUser(userID string) (int, error) {
	count, err := db.bun.NewSelect().Model((*Session)(nil)).
		Where("user_id = ?", userID).
		Where("status IN ('creating', 'running')").
		Count(ctx())
	return count, err
}

// CountActiveSessions returns the total number of active (creating or running) sessions globally.
func (db *DB) CountActiveSessions() (int, error) {
	count, err := db.bun.NewSelect().Model((*Session)(nil)).
		Where("status IN ('creating', 'running')").
		Count(ctx())
	return count, err
}

// GetStaleSessions returns sessions that have exceeded their idle timeout.
// Sessions with a per-session idle_timeout use that value; others use the global default.
func (db *DB) GetStaleSessions(defaultTimeout time.Duration) ([]Session, error) {
	defaultCutoff := time.Now().Add(-defaultTimeout)

	var query string
	if db.dbType == "postgres" {
		query = `SELECT id, user_id, app_id, pod_name, pod_ip, status, idle_timeout, tenant_id, created_at, updated_at
			 FROM sessions
			 WHERE status NOT IN ('terminated', 'failed', 'stopped', 'expired')
			 AND (
			   (idle_timeout > 0 AND updated_at < NOW() - (idle_timeout || ' seconds')::interval)
			   OR (idle_timeout = 0 AND updated_at < ?)
			 )`
	} else {
		query = `SELECT id, user_id, app_id, pod_name, pod_ip, status, idle_timeout, tenant_id, created_at, updated_at
			 FROM sessions
			 WHERE status NOT IN ('terminated', 'failed', 'stopped', 'expired')
			 AND (
			   (idle_timeout > 0 AND updated_at < datetime('now', '-' || idle_timeout || ' seconds'))
			   OR (idle_timeout = 0 AND updated_at < ?)
			 )`
	}

	var sessions []Session
	err := db.bun.NewRaw(query, defaultCutoff).Scan(ctx(), &sessions)
	return sessions, err
}

// CreateUser creates a new user in the database
func (db *DB) CreateUser(user User) error {
	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now
	_, err := db.bun.NewInsert().Model(&user).Exec(ctx())
	return err
}

// GetUserByID retrieves a user by their ID
func (db *DB) GetUserByID(id string) (*User, error) {
	var user User
	err := db.bun.NewSelect().Model(&user).Where("id = ?", id).Scan(ctx())
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUserByUsername retrieves a user by their username
func (db *DB) GetUserByUsername(username string) (*User, error) {
	var user User
	err := db.bun.NewSelect().Model(&user).Where("username = ?", username).Scan(ctx())
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// UpdateUser updates an existing user
func (db *DB) UpdateUser(user User) error {
	user.UpdatedAt = time.Now()
	result, err := db.bun.NewUpdate().Model(&user).WherePK().Exec(ctx())
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
	var users []User
	err := db.bun.NewSelect().Model(&users).
		OrderExpr("created_at DESC").
		Scan(ctx())
	return users, err
}

// GetUserByAuthProvider retrieves a user by their external auth provider and subject ID.
func (db *DB) GetUserByAuthProvider(provider, providerID string) (*User, error) {
	var user User
	err := db.bun.NewSelect().Model(&user).
		Where("auth_provider = ?", provider).
		Where("auth_provider_id = ?", providerID).
		Scan(ctx())
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// DeleteUser removes a user by ID
func (db *DB) DeleteUser(id string) error {
	result, err := db.bun.NewDelete().Model((*User)(nil)).Where("id = ?", id).Exec(ctx())
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
	var setting Setting
	err := db.bun.NewSelect().Model(&setting).Where("key = ?", key).Scan(ctx())
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return setting.Value, nil
}

// SetSetting creates or updates a setting
func (db *DB) SetSetting(key, value string) error {
	setting := Setting{
		Key:       key,
		Value:     value,
		UpdatedAt: time.Now(),
	}
	_, err := db.bun.NewInsert().Model(&setting).
		On("CONFLICT (key) DO UPDATE").
		Set("value = EXCLUDED.value, updated_at = EXCLUDED.updated_at").
		Exec(ctx())
	return err
}

// GetAllSettings retrieves all settings
func (db *DB) GetAllSettings() (map[string]string, error) {
	var settings []Setting
	err := db.bun.NewRaw("SELECT key, value FROM settings").Scan(ctx(), &settings)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for _, s := range settings {
		result[s.Key] = s.Value
	}
	return result, nil
}

// ListTemplates returns all templates
func (db *DB) ListTemplates() ([]Template, error) {
	var templates []Template
	err := db.bun.NewSelect().Model(&templates).
		OrderExpr("template_category, name").
		Scan(ctx())
	return templates, err
}

// GetTemplate returns a single template by template_id
func (db *DB) GetTemplate(templateID string) (*Template, error) {
	var t Template
	err := db.bun.NewSelect().Model(&t).Where("template_id = ?", templateID).Scan(ctx())
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// CreateTemplate inserts a new template
func (db *DB) CreateTemplate(t Template) error {
	now := time.Now()
	t.CreatedAt = now
	t.UpdatedAt = now
	_, err := db.bun.NewInsert().Model(&t).Exec(ctx())
	return err
}

// UpdateTemplate updates an existing template
func (db *DB) UpdateTemplate(t Template) error {
	t.UpdatedAt = time.Now()
	result, err := db.bun.NewUpdate().Model(&t).Where("template_id = ?", t.TemplateID).Exec(ctx())
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
	result, err := db.bun.NewDelete().Model((*Template)(nil)).Where("template_id = ?", templateID).Exec(ctx())
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

// CreateAppSpec inserts a new application specification
func (db *DB) CreateAppSpec(spec AppSpec) error {
	now := time.Now()
	spec.CreatedAt = now
	spec.UpdatedAt = now
	_, err := db.bun.NewInsert().Model(&spec).Exec(ctx())
	return err
}

// GetAppSpec returns a single application specification by ID
func (db *DB) GetAppSpec(id string) (*AppSpec, error) {
	var spec AppSpec
	err := db.bun.NewSelect().Model(&spec).Where("id = ?", id).Scan(ctx())
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &spec, nil
}

// ListAppSpecs returns all application specifications
func (db *DB) ListAppSpecs() ([]AppSpec, error) {
	var specs []AppSpec
	err := db.bun.NewSelect().Model(&specs).
		OrderExpr("name").
		Scan(ctx())
	return specs, err
}

// UpdateAppSpec updates an existing application specification
func (db *DB) UpdateAppSpec(spec AppSpec) error {
	spec.UpdatedAt = time.Now()
	result, err := db.bun.NewUpdate().Model(&spec).WherePK().Exec(ctx())
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

// DeleteAppSpec removes an application specification by ID
func (db *DB) DeleteAppSpec(id string) error {
	result, err := db.bun.NewDelete().Model((*AppSpec)(nil)).Where("id = ?", id).Exec(ctx())
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

// SaveOIDCState stores an OIDC CSRF state token with its redirect URL and expiry.
func (db *DB) SaveOIDCState(state, redirectURL string, expiresAt time.Time) error {
	entry := OIDCState{
		State:       state,
		RedirectURL: redirectURL,
		ExpiresAt:   expiresAt,
	}
	_, err := db.bun.NewInsert().Model(&entry).Exec(ctx())
	return err
}

// ConsumeOIDCState atomically loads and deletes an OIDC state token.
// Returns the redirect URL and expiry, or empty string if not found.
func (db *DB) ConsumeOIDCState(state string) (redirectURL string, expiresAt time.Time, err error) {
	err = db.bun.RunInTx(ctx(), nil, func(txCtx context.Context, tx bun.Tx) error {
		var entry OIDCState
		if err := tx.NewSelect().Model(&entry).Where("state = ?", state).Scan(txCtx); err != nil {
			return err
		}
		redirectURL = entry.RedirectURL
		expiresAt = entry.ExpiresAt

		_, err := tx.NewDelete().Model((*OIDCState)(nil)).Where("state = ?", state).Exec(txCtx)
		return err
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return redirectURL, expiresAt, nil
}

// CleanupExpiredOIDCStates removes expired OIDC state tokens.
func (db *DB) CleanupExpiredOIDCStates() error {
	_, err := db.bun.NewDelete().Model((*OIDCState)(nil)).
		Where("expires_at < ?", time.Now()).
		Exec(ctx())
	return err
}

// SeedTemplatesFromData loads templates from JSON data if the templates table is empty
func (db *DB) SeedTemplatesFromData(data []byte) error {
	// Check if templates table is empty
	count, err := db.bun.NewSelect().Model((*Template)(nil)).Count(ctx())
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

// SharePermission controls the access level of a session share.
type SharePermission string

const (
	SharePermissionReadOnly  SharePermission = "read_only"
	SharePermissionReadWrite SharePermission = "read_write"
)

// SessionShare represents a share record granting access to a session.
type SessionShare struct {
	bun.BaseModel `bun:"table:session_shares"`

	ID         string          `json:"id" bun:"id,pk"`
	SessionID  string          `json:"session_id" bun:"session_id,notnull"`
	UserID     string          `json:"user_id" bun:"user_id"`
	Permission SharePermission `json:"permission" bun:"permission,notnull"`
	ShareToken string          `json:"share_token,omitempty" bun:"share_token,unique"`
	CreatedBy  string          `json:"created_by" bun:"created_by,notnull"`
	CreatedAt  time.Time       `json:"created_at" bun:"created_at,nullzero,notnull,default:current_timestamp"`
}

// CreateSessionShare inserts a new session share record.
func (db *DB) CreateSessionShare(share SessionShare) error {
	_, err := db.bun.NewInsert().Model(&share).Exec(ctx())
	return err
}

// GetSessionShare returns a session share by ID.
func (db *DB) GetSessionShare(id string) (*SessionShare, error) {
	var share SessionShare
	err := db.bun.NewSelect().Model(&share).Where("id = ?", id).Scan(ctx())
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &share, nil
}

// GetSessionShareByToken returns a session share by its token.
func (db *DB) GetSessionShareByToken(token string) (*SessionShare, error) {
	var share SessionShare
	err := db.bun.NewSelect().Model(&share).Where("share_token = ?", token).Scan(ctx())
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &share, nil
}

// ListSessionShares returns all shares for a given session.
func (db *DB) ListSessionShares(sessionID string) ([]SessionShare, error) {
	var shares []SessionShare
	err := db.bun.NewSelect().Model(&shares).
		Where("session_id = ?", sessionID).
		OrderExpr("created_at DESC").
		Scan(ctx())
	return shares, err
}

// SharedSessionRow holds join data for sessions shared with a user.
type SharedSessionRow struct {
	Session       Session
	AppName       string
	OwnerUsername string
	Permission    SharePermission
	ShareID       string
}

// ListSharedSessionsForUser returns sessions shared with the given user.
func (db *DB) ListSharedSessionsForUser(userID string) ([]SharedSessionRow, error) {
	type rawRow struct {
		SessionID      string        `bun:"session_id"`
		UserID         string        `bun:"user_id"`
		AppID          string        `bun:"app_id"`
		PodName        string        `bun:"pod_name"`
		PodIP          string        `bun:"pod_ip"`
		Status         SessionStatus `bun:"status"`
		IdleTimeout    int64         `bun:"idle_timeout"`
		SessionCreated time.Time     `bun:"session_created_at"`
		SessionUpdated time.Time     `bun:"session_updated_at"`
		AppName        string        `bun:"app_name"`
		OwnerUsername  string        `bun:"owner_username"`
		Permission     string        `bun:"permission"`
		ShareID        string        `bun:"share_id"`
	}

	var rows []rawRow
	err := db.bun.NewRaw(`
		SELECT s.id AS session_id, s.user_id, s.app_id, s.pod_name, s.pod_ip, s.status,
		       s.idle_timeout, s.created_at AS session_created_at, s.updated_at AS session_updated_at,
		       COALESCE(a.name, s.app_id) AS app_name,
		       COALESCE(u.username, s.user_id) AS owner_username,
		       ss.permission, ss.id AS share_id
		FROM session_shares ss
		JOIN sessions s ON ss.session_id = s.id
		LEFT JOIN applications a ON s.app_id = a.id
		LEFT JOIN users u ON s.user_id = u.id
		WHERE ss.user_id = ? AND s.status NOT IN ('terminated', 'failed', 'stopped', 'expired')
		ORDER BY s.created_at DESC`, userID,
	).Scan(ctx(), &rows)
	if err != nil {
		return nil, err
	}

	var results []SharedSessionRow
	for _, r := range rows {
		results = append(results, SharedSessionRow{
			Session: Session{
				ID:          r.SessionID,
				UserID:      r.UserID,
				AppID:       r.AppID,
				PodName:     r.PodName,
				PodIP:       r.PodIP,
				Status:      r.Status,
				IdleTimeout: r.IdleTimeout,
				CreatedAt:   r.SessionCreated,
				UpdatedAt:   r.SessionUpdated,
			},
			AppName:       r.AppName,
			OwnerUsername: r.OwnerUsername,
			Permission:    SharePermission(r.Permission),
			ShareID:       r.ShareID,
		})
	}
	return results, nil
}

// CheckSessionAccess checks whether a user has share access to a session.
func (db *DB) CheckSessionAccess(sessionID, userID string) (*SessionShare, error) {
	var share SessionShare
	err := db.bun.NewSelect().Model(&share).
		Where("session_id = ?", sessionID).
		Where("user_id = ?", userID).
		Limit(1).
		Scan(ctx())
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &share, nil
}

// DeleteSessionShare removes a session share by ID.
func (db *DB) DeleteSessionShare(id string) error {
	result, err := db.bun.NewDelete().Model((*SessionShare)(nil)).Where("id = ?", id).Exec(ctx())
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

// DeleteSessionSharesBySession removes all shares for a session.
func (db *DB) DeleteSessionSharesBySession(sessionID string) error {
	_, err := db.bun.NewDelete().Model((*SessionShare)(nil)).
		Where("session_id = ?", sessionID).
		Exec(ctx())
	return err
}

// UpdateSessionShareUserID sets the user_id on a share (used when joining via link).
func (db *DB) UpdateSessionShareUserID(id, userID string) error {
	_, err := db.bun.NewUpdate().Model((*SessionShare)(nil)).
		Set("user_id = ?", userID).
		Where("id = ?", id).
		Exec(ctx())
	return err
}

// RecordingStatus represents the status of a video recording.
type RecordingStatus string

const (
	RecordingStatusRecording  RecordingStatus = "recording"
	RecordingStatusUploading  RecordingStatus = "uploading"
	RecordingStatusProcessing RecordingStatus = "processing"
	RecordingStatusReady      RecordingStatus = "ready"
	RecordingStatusFailed     RecordingStatus = "failed"
)

// Recording represents a video recording of a session.
type Recording struct {
	bun.BaseModel `bun:"table:recordings"`

	ID              string          `json:"id" bun:"id,pk"`
	SessionID       string          `json:"session_id" bun:"session_id,notnull"`
	UserID          string          `json:"user_id" bun:"user_id,notnull"`
	Filename        string          `json:"filename" bun:"filename,notnull"`
	SizeBytes       int64           `json:"size_bytes" bun:"size_bytes"`
	DurationSeconds float64         `json:"duration_seconds" bun:"duration_seconds"`
	Format          string          `json:"format" bun:"format"`
	StorageBackend  string          `json:"storage_backend" bun:"storage_backend"`
	StoragePath     string          `json:"storage_path" bun:"storage_path"`
	VideoPath       string          `json:"video_path,omitempty" bun:"video_path"`
	Status          RecordingStatus `json:"status" bun:"status,notnull"`
	TenantID        string          `json:"tenant_id,omitempty" bun:"tenant_id"`
	CreatedAt       time.Time       `json:"created_at" bun:"created_at,nullzero,notnull,default:current_timestamp"`
	CompletedAt     *time.Time      `json:"completed_at,omitempty" bun:"completed_at"`
}

// CreateRecording inserts a new recording record.
func (db *DB) CreateRecording(rec Recording) error {
	if rec.TenantID == "" {
		rec.TenantID = DefaultTenantID
	}
	_, err := db.bun.NewInsert().Model(&rec).Exec(ctx())
	return err
}

// GetRecording returns a recording by ID.
func (db *DB) GetRecording(id string) (*Recording, error) {
	var rec Recording
	err := db.bun.NewSelect().Model(&rec).Where("id = ?", id).Scan(ctx())
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// UpdateRecordingStatus updates the status of a recording.
func (db *DB) UpdateRecordingStatus(id string, status RecordingStatus) error {
	result, err := db.bun.NewUpdate().Model((*Recording)(nil)).
		Set("status = ?", status).
		Where("id = ?", id).
		Exec(ctx())
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

// UpdateRecordingComplete marks a recording as ready with final metadata.
func (db *DB) UpdateRecordingComplete(id string, storagePath string, sizeBytes int64, durationSeconds float64) error {
	now := time.Now()
	result, err := db.bun.NewUpdate().Model((*Recording)(nil)).
		Set("status = ?", RecordingStatusReady).
		Set("storage_path = ?", storagePath).
		Set("size_bytes = ?", sizeBytes).
		Set("duration_seconds = ?", durationSeconds).
		Set("completed_at = ?", now).
		Where("id = ?", id).
		Exec(ctx())
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

// UpdateRecordingVideoPath sets the converted video path for a recording.
func (db *DB) UpdateRecordingVideoPath(id string, videoPath string) error {
	result, err := db.bun.NewUpdate().Model((*Recording)(nil)).
		Set("video_path = ?", videoPath).
		Where("id = ?", id).
		Exec(ctx())
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

// ListRecordingsByUser returns all recordings for a given user.
func (db *DB) ListRecordingsByUser(userID string) ([]Recording, error) {
	var recs []Recording
	err := db.bun.NewSelect().Model(&recs).
		Where("user_id = ?", userID).
		OrderExpr("created_at DESC").
		Scan(ctx())
	return recs, err
}

// ListRecordingsBySession returns all recordings for a given session.
func (db *DB) ListRecordingsBySession(sessionID string) ([]Recording, error) {
	var recs []Recording
	err := db.bun.NewSelect().Model(&recs).
		Where("session_id = ?", sessionID).
		OrderExpr("created_at DESC").
		Scan(ctx())
	return recs, err
}

// ListAllRecordings returns all recordings (admin use).
func (db *DB) ListAllRecordings() ([]Recording, error) {
	var recs []Recording
	err := db.bun.NewSelect().Model(&recs).
		OrderExpr("created_at DESC").
		Scan(ctx())
	return recs, err
}

// DeleteRecording removes a recording by ID.
func (db *DB) DeleteRecording(id string) error {
	result, err := db.bun.NewDelete().Model((*Recording)(nil)).Where("id = ?", id).Exec(ctx())
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

// ListExpiredRecordings returns recordings with status 'ready' that completed before the given cutoff time.
func (db *DB) ListExpiredRecordings(olderThan time.Time) ([]Recording, error) {
	var recs []Recording
	err := db.bun.NewSelect().Model(&recs).
		Where("status = ?", RecordingStatusReady).
		Where("completed_at < ?", olderThan).
		OrderExpr("completed_at ASC").
		Scan(ctx())
	return recs, err
}
