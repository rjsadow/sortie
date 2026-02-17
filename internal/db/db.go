package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/uptrace/bun"

	_ "modernc.org/sqlite"
)

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

	// JSON-serialized DB columns
	ContainerArgsJSON     string `json:"-" bun:"container_args"`
	TagsJSON              string `json:"-" bun:"tags"`
	RecommendedLimitsJSON string `json:"-" bun:"recommended_limits"`
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

	// JSON-serialized DB columns
	ResourcesJSON    string `json:"-" bun:"resources"`
	EnvVarsJSON      string `json:"-" bun:"env_vars"`
	VolumesJSON      string `json:"-" bun:"volumes"`
	NetworkRulesJSON string `json:"-" bun:"network_rules"`
	EgressPolicyJSON string `json:"-" bun:"egress_policy"`
}

// DB wraps the sql.DB connection
type DB struct {
	conn   *sql.DB
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

	return &DB{conn: conn, dbType: dbType}, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

// Ping verifies the database connection is alive.
func (db *DB) Ping() error {
	return db.conn.Ping()
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
	rows, err := db.conn.Query("SELECT id, name, description, url, icon, category, visibility, launch_type, os_type, container_image, container_port, container_args, cpu_request, cpu_limit, memory_request, memory_limit, egress_policy FROM applications ORDER BY category, name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []Application
	for rows.Next() {
		var app Application
		var visibility string
		var launchType, osType, containerImage string
		var containerPort int
		var containerArgsJSON string
		var cpuRequest, cpuLimit, memoryRequest, memoryLimit string
		var egressPolicyJSON string
		if err := rows.Scan(&app.ID, &app.Name, &app.Description, &app.URL, &app.Icon, &app.Category, &visibility, &launchType, &osType, &containerImage, &containerPort, &containerArgsJSON, &cpuRequest, &cpuLimit, &memoryRequest, &memoryLimit, &egressPolicyJSON); err != nil {
			return nil, err
		}
		app.Visibility = CategoryVisibility(visibility)
		if app.Visibility == "" {
			app.Visibility = CategoryVisibilityPublic
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
		// Parse egress policy from JSON
		if egressPolicyJSON != "" {
			var ep EgressPolicy
			if json.Unmarshal([]byte(egressPolicyJSON), &ep) == nil && ep.Mode != "" {
				app.EgressPolicy = &ep
			}
		}
		apps = append(apps, app)
	}

	return apps, rows.Err()
}

// GetApp returns a single application by ID
func (db *DB) GetApp(id string) (*Application, error) {
	var app Application
	var visibility string
	var launchType, osType, containerImage string
	var containerPort int
	var containerArgsJSON string
	var cpuRequest, cpuLimit, memoryRequest, memoryLimit string
	var egressPolicyJSON string
	err := db.conn.QueryRow(
		"SELECT id, name, description, url, icon, category, visibility, launch_type, os_type, container_image, container_port, container_args, cpu_request, cpu_limit, memory_request, memory_limit, egress_policy FROM applications WHERE id = ?",
		id,
	).Scan(&app.ID, &app.Name, &app.Description, &app.URL, &app.Icon, &app.Category, &visibility, &launchType, &osType, &containerImage, &containerPort, &containerArgsJSON, &cpuRequest, &cpuLimit, &memoryRequest, &memoryLimit, &egressPolicyJSON)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	app.Visibility = CategoryVisibility(visibility)
	if app.Visibility == "" {
		app.Visibility = CategoryVisibilityPublic
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
	// Parse egress policy from JSON
	if egressPolicyJSON != "" {
		var ep EgressPolicy
		if json.Unmarshal([]byte(egressPolicyJSON), &ep) == nil && ep.Mode != "" {
			app.EgressPolicy = &ep
		}
	}
	return &app, nil
}

// CreateApp inserts a new application
func (db *DB) CreateApp(app Application) error {
	visibility := string(app.Visibility)
	if visibility == "" {
		visibility = string(CategoryVisibilityPublic)
	}
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
	// Serialize egress policy to JSON
	egressPolicyJSON := ""
	if app.EgressPolicy != nil && app.EgressPolicy.Mode != "" {
		if b, err := json.Marshal(app.EgressPolicy); err == nil {
			egressPolicyJSON = string(b)
		}
	}
	_, err := db.conn.Exec(
		"INSERT INTO applications (id, name, description, url, icon, category, visibility, launch_type, os_type, container_image, container_port, container_args, cpu_request, cpu_limit, memory_request, memory_limit, egress_policy) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		app.ID, app.Name, app.Description, app.URL, app.Icon, app.Category, visibility, launchType, osType, app.ContainerImage, app.ContainerPort, containerArgsJSON, cpuRequest, cpuLimit, memoryRequest, memoryLimit, egressPolicyJSON,
	)
	return err
}

// UpdateApp updates an existing application
func (db *DB) UpdateApp(app Application) error {
	visibility := string(app.Visibility)
	if visibility == "" {
		visibility = string(CategoryVisibilityPublic)
	}
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
	// Serialize egress policy to JSON
	egressPolicyJSON := ""
	if app.EgressPolicy != nil && app.EgressPolicy.Mode != "" {
		if b, err := json.Marshal(app.EgressPolicy); err == nil {
			egressPolicyJSON = string(b)
		}
	}
	result, err := db.conn.Exec(
		"UPDATE applications SET name = ?, description = ?, url = ?, icon = ?, category = ?, visibility = ?, launch_type = ?, os_type = ?, container_image = ?, container_port = ?, container_args = ?, cpu_request = ?, cpu_limit = ?, memory_request = ?, memory_limit = ?, egress_policy = ? WHERE id = ?",
		app.Name, app.Description, app.URL, app.Icon, app.Category, visibility, launchType, osType, app.ContainerImage, app.ContainerPort, containerArgsJSON, cpuRequest, cpuLimit, memoryRequest, memoryLimit, egressPolicyJSON, app.ID,
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
	where := "WHERE 1=1"
	args := []any{}

	if filter.User != "" {
		where += " AND user = ?"
		args = append(args, filter.User)
	}
	if filter.Action != "" {
		where += " AND action = ?"
		args = append(args, filter.Action)
	}
	if !filter.From.IsZero() {
		where += " AND timestamp >= ?"
		args = append(args, filter.From)
	}
	if !filter.To.IsZero() {
		where += " AND timestamp <= ?"
		args = append(args, filter.To)
	}

	// Get total count
	var total int
	countQuery := "SELECT COUNT(*) FROM audit_log " + where
	if err := db.conn.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("failed to count audit logs: %w", err)
	}

	// Apply pagination defaults
	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	offset := max(filter.Offset, 0)

	query := "SELECT id, timestamp, user, action, details FROM audit_log " + where + " ORDER BY timestamp DESC LIMIT ? OFFSET ?"
	queryArgs := append(args, limit, offset)

	rows, err := db.conn.Query(query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit logs: %w", err)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &AuditLogPage{Logs: logs, Total: total}, nil
}

// GetAuditLogActions returns all distinct action values in the audit log
func (db *DB) GetAuditLogActions() ([]string, error) {
	rows, err := db.conn.Query("SELECT DISTINCT action FROM audit_log ORDER BY action")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []string
	for rows.Next() {
		var action string
		if err := rows.Scan(&action); err != nil {
			return nil, err
		}
		actions = append(actions, action)
	}
	return actions, rows.Err()
}

// GetAuditLogUsers returns all distinct user values in the audit log
func (db *DB) GetAuditLogUsers() ([]string, error) {
	rows, err := db.conn.Query("SELECT DISTINCT user FROM audit_log ORDER BY user")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []string
	for rows.Next() {
		var user string
		if err := rows.Scan(&user); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
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
	tenantID := session.TenantID
	if tenantID == "" {
		tenantID = DefaultTenantID
	}
	_, err := db.conn.Exec(
		"INSERT INTO sessions (id, user_id, app_id, pod_name, pod_ip, status, idle_timeout, tenant_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		session.ID, session.UserID, session.AppID, session.PodName, session.PodIP, string(session.Status), session.IdleTimeout, tenantID, session.CreatedAt, session.UpdatedAt,
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

// CountActiveSessionsByUser returns the number of active (creating or running) sessions for a user.
func (db *DB) CountActiveSessionsByUser(userID string) (int, error) {
	var count int
	err := db.conn.QueryRow(
		"SELECT COUNT(*) FROM sessions WHERE user_id = ? AND status IN ('creating', 'running')",
		userID,
	).Scan(&count)
	return count, err
}

// CountActiveSessions returns the total number of active (creating or running) sessions globally.
func (db *DB) CountActiveSessions() (int, error) {
	var count int
	err := db.conn.QueryRow(
		"SELECT COUNT(*) FROM sessions WHERE status IN ('creating', 'running')",
	).Scan(&count)
	return count, err
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

	authProvider := user.AuthProvider
	if authProvider == "" {
		authProvider = "local"
	}

	tenantID := user.TenantID
	if tenantID == "" {
		tenantID = DefaultTenantID
	}

	tenantRolesJSON := "[]"
	if len(user.TenantRoles) > 0 {
		if b, err := json.Marshal(user.TenantRoles); err == nil {
			tenantRolesJSON = string(b)
		}
	}

	now := time.Now()
	_, err = db.conn.Exec(
		"INSERT INTO users (id, username, email, display_name, password_hash, roles, auth_provider, auth_provider_id, tenant_id, tenant_roles, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		user.ID, user.Username, user.Email, user.DisplayName, user.PasswordHash, string(rolesJSON), authProvider, user.AuthProviderID, tenantID, tenantRolesJSON, now, now,
	)
	return err
}

// GetUserByID retrieves a user by their ID
func (db *DB) GetUserByID(id string) (*User, error) {
	var user User
	var rolesJSON string
	var authProvider, authProviderID sql.NullString
	var tenantID sql.NullString
	var tenantRolesJSON sql.NullString
	err := db.conn.QueryRow(
		"SELECT id, username, email, display_name, password_hash, roles, auth_provider, auth_provider_id, tenant_id, tenant_roles, created_at, updated_at FROM users WHERE id = ?",
		id,
	).Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.PasswordHash, &rolesJSON, &authProvider, &authProviderID, &tenantID, &tenantRolesJSON, &user.CreatedAt, &user.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(rolesJSON), &user.Roles); err != nil {
		return nil, fmt.Errorf("failed to unmarshal roles: %w", err)
	}
	if authProvider.Valid {
		user.AuthProvider = authProvider.String
	}
	if authProviderID.Valid {
		user.AuthProviderID = authProviderID.String
	}
	if tenantID.Valid {
		user.TenantID = tenantID.String
	}
	if tenantRolesJSON.Valid && tenantRolesJSON.String != "" && tenantRolesJSON.String != "[]" {
		json.Unmarshal([]byte(tenantRolesJSON.String), &user.TenantRoles)
	}

	return &user, nil
}

// GetUserByUsername retrieves a user by their username
func (db *DB) GetUserByUsername(username string) (*User, error) {
	var user User
	var rolesJSON string
	var authProvider, authProviderID sql.NullString
	var tenantID sql.NullString
	var tenantRolesJSON sql.NullString
	err := db.conn.QueryRow(
		"SELECT id, username, email, display_name, password_hash, roles, auth_provider, auth_provider_id, tenant_id, tenant_roles, created_at, updated_at FROM users WHERE username = ?",
		username,
	).Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.PasswordHash, &rolesJSON, &authProvider, &authProviderID, &tenantID, &tenantRolesJSON, &user.CreatedAt, &user.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(rolesJSON), &user.Roles); err != nil {
		return nil, fmt.Errorf("failed to unmarshal roles: %w", err)
	}
	if authProvider.Valid {
		user.AuthProvider = authProvider.String
	}
	if authProviderID.Valid {
		user.AuthProviderID = authProviderID.String
	}
	if tenantID.Valid {
		user.TenantID = tenantID.String
	}
	if tenantRolesJSON.Valid && tenantRolesJSON.String != "" && tenantRolesJSON.String != "[]" {
		json.Unmarshal([]byte(tenantRolesJSON.String), &user.TenantRoles)
	}

	return &user, nil
}

// UpdateUser updates an existing user
func (db *DB) UpdateUser(user User) error {
	rolesJSON, err := json.Marshal(user.Roles)
	if err != nil {
		return fmt.Errorf("failed to marshal roles: %w", err)
	}

	authProvider := user.AuthProvider
	if authProvider == "" {
		authProvider = "local"
	}

	tenantRolesJSON := "[]"
	if len(user.TenantRoles) > 0 {
		if b, err := json.Marshal(user.TenantRoles); err == nil {
			tenantRolesJSON = string(b)
		}
	}

	result, err := db.conn.Exec(
		"UPDATE users SET email = ?, display_name = ?, password_hash = ?, roles = ?, auth_provider = ?, auth_provider_id = ?, tenant_id = ?, tenant_roles = ?, updated_at = ? WHERE id = ?",
		user.Email, user.DisplayName, user.PasswordHash, string(rolesJSON), authProvider, user.AuthProviderID, user.TenantID, tenantRolesJSON, time.Now(), user.ID,
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
		"SELECT id, username, email, display_name, password_hash, roles, auth_provider, auth_provider_id, tenant_id, tenant_roles, created_at, updated_at FROM users ORDER BY created_at DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanUsers(rows)
}

// GetUserByAuthProvider retrieves a user by their external auth provider and subject ID.
func (db *DB) GetUserByAuthProvider(provider, providerID string) (*User, error) {
	var user User
	var rolesJSON string
	var authProvider, authProviderIDCol sql.NullString
	var tenantID sql.NullString
	var tenantRolesJSON sql.NullString
	err := db.conn.QueryRow(
		"SELECT id, username, email, display_name, password_hash, roles, auth_provider, auth_provider_id, tenant_id, tenant_roles, created_at, updated_at FROM users WHERE auth_provider = ? AND auth_provider_id = ?",
		provider, providerID,
	).Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.PasswordHash, &rolesJSON, &authProvider, &authProviderIDCol, &tenantID, &tenantRolesJSON, &user.CreatedAt, &user.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(rolesJSON), &user.Roles); err != nil {
		return nil, fmt.Errorf("failed to unmarshal roles: %w", err)
	}
	if authProvider.Valid {
		user.AuthProvider = authProvider.String
	}
	if authProviderIDCol.Valid {
		user.AuthProviderID = authProviderIDCol.String
	}
	if tenantID.Valid {
		user.TenantID = tenantID.String
	}
	if tenantRolesJSON.Valid && tenantRolesJSON.String != "" && tenantRolesJSON.String != "[]" {
		json.Unmarshal([]byte(tenantRolesJSON.String), &user.TenantRoles)
	}

	return &user, nil
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

// CreateAppSpec inserts a new application specification
func (db *DB) CreateAppSpec(spec AppSpec) error {
	envVarsJSON := "[]"
	if len(spec.EnvVars) > 0 {
		if b, err := json.Marshal(spec.EnvVars); err == nil {
			envVarsJSON = string(b)
		}
	}
	volumesJSON := "[]"
	if len(spec.Volumes) > 0 {
		if b, err := json.Marshal(spec.Volumes); err == nil {
			volumesJSON = string(b)
		}
	}
	networkRulesJSON := "[]"
	if len(spec.NetworkRules) > 0 {
		if b, err := json.Marshal(spec.NetworkRules); err == nil {
			networkRulesJSON = string(b)
		}
	}

	var cpuRequest, cpuLimit, memoryRequest, memoryLimit string
	if spec.Resources != nil {
		cpuRequest = spec.Resources.CPURequest
		cpuLimit = spec.Resources.CPULimit
		memoryRequest = spec.Resources.MemoryRequest
		memoryLimit = spec.Resources.MemoryLimit
	}

	egressPolicyJSON := ""
	if spec.EgressPolicy != nil && spec.EgressPolicy.Mode != "" {
		if b, err := json.Marshal(spec.EgressPolicy); err == nil {
			egressPolicyJSON = string(b)
		}
	}

	now := time.Now()
	_, err := db.conn.Exec(
		`INSERT INTO app_specs (id, name, description, image, launch_command,
		 cpu_request, cpu_limit, memory_request, memory_limit,
		 env_vars, volumes, network_rules, egress_policy, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		spec.ID, spec.Name, spec.Description, spec.Image, spec.LaunchCommand,
		cpuRequest, cpuLimit, memoryRequest, memoryLimit,
		envVarsJSON, volumesJSON, networkRulesJSON, egressPolicyJSON, now, now,
	)
	return err
}

// GetAppSpec returns a single application specification by ID
func (db *DB) GetAppSpec(id string) (*AppSpec, error) {
	var spec AppSpec
	var cpuRequest, cpuLimit, memoryRequest, memoryLimit string
	var envVarsJSON, volumesJSON, networkRulesJSON string
	var egressPolicyJSON string

	err := db.conn.QueryRow(
		`SELECT id, name, description, image, launch_command,
		 cpu_request, cpu_limit, memory_request, memory_limit,
		 env_vars, volumes, network_rules, egress_policy, created_at, updated_at
		 FROM app_specs WHERE id = ?`, id,
	).Scan(&spec.ID, &spec.Name, &spec.Description, &spec.Image, &spec.LaunchCommand,
		&cpuRequest, &cpuLimit, &memoryRequest, &memoryLimit,
		&envVarsJSON, &volumesJSON, &networkRulesJSON, &egressPolicyJSON,
		&spec.CreatedAt, &spec.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if cpuRequest != "" || cpuLimit != "" || memoryRequest != "" || memoryLimit != "" {
		spec.Resources = &ResourceLimits{
			CPURequest:    cpuRequest,
			CPULimit:      cpuLimit,
			MemoryRequest: memoryRequest,
			MemoryLimit:   memoryLimit,
		}
	}
	if envVarsJSON != "" && envVarsJSON != "[]" {
		json.Unmarshal([]byte(envVarsJSON), &spec.EnvVars)
	}
	if volumesJSON != "" && volumesJSON != "[]" {
		json.Unmarshal([]byte(volumesJSON), &spec.Volumes)
	}
	if networkRulesJSON != "" && networkRulesJSON != "[]" {
		json.Unmarshal([]byte(networkRulesJSON), &spec.NetworkRules)
	}
	if egressPolicyJSON != "" {
		var ep EgressPolicy
		if json.Unmarshal([]byte(egressPolicyJSON), &ep) == nil && ep.Mode != "" {
			spec.EgressPolicy = &ep
		}
	}

	return &spec, nil
}

// ListAppSpecs returns all application specifications
func (db *DB) ListAppSpecs() ([]AppSpec, error) {
	rows, err := db.conn.Query(
		`SELECT id, name, description, image, launch_command,
		 cpu_request, cpu_limit, memory_request, memory_limit,
		 env_vars, volumes, network_rules, egress_policy, created_at, updated_at
		 FROM app_specs ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var specs []AppSpec
	for rows.Next() {
		var spec AppSpec
		var cpuRequest, cpuLimit, memoryRequest, memoryLimit string
		var envVarsJSON, volumesJSON, networkRulesJSON string
		var egressPolicyJSON string

		if err := rows.Scan(&spec.ID, &spec.Name, &spec.Description, &spec.Image, &spec.LaunchCommand,
			&cpuRequest, &cpuLimit, &memoryRequest, &memoryLimit,
			&envVarsJSON, &volumesJSON, &networkRulesJSON, &egressPolicyJSON,
			&spec.CreatedAt, &spec.UpdatedAt); err != nil {
			return nil, err
		}

		if cpuRequest != "" || cpuLimit != "" || memoryRequest != "" || memoryLimit != "" {
			spec.Resources = &ResourceLimits{
				CPURequest:    cpuRequest,
				CPULimit:      cpuLimit,
				MemoryRequest: memoryRequest,
				MemoryLimit:   memoryLimit,
			}
		}
		if envVarsJSON != "" && envVarsJSON != "[]" {
			json.Unmarshal([]byte(envVarsJSON), &spec.EnvVars)
		}
		if volumesJSON != "" && volumesJSON != "[]" {
			json.Unmarshal([]byte(volumesJSON), &spec.Volumes)
		}
		if networkRulesJSON != "" && networkRulesJSON != "[]" {
			json.Unmarshal([]byte(networkRulesJSON), &spec.NetworkRules)
		}
		if egressPolicyJSON != "" {
			var ep EgressPolicy
			if json.Unmarshal([]byte(egressPolicyJSON), &ep) == nil && ep.Mode != "" {
				spec.EgressPolicy = &ep
			}
		}

		specs = append(specs, spec)
	}

	return specs, rows.Err()
}

// UpdateAppSpec updates an existing application specification
func (db *DB) UpdateAppSpec(spec AppSpec) error {
	envVarsJSON := "[]"
	if len(spec.EnvVars) > 0 {
		if b, err := json.Marshal(spec.EnvVars); err == nil {
			envVarsJSON = string(b)
		}
	}
	volumesJSON := "[]"
	if len(spec.Volumes) > 0 {
		if b, err := json.Marshal(spec.Volumes); err == nil {
			volumesJSON = string(b)
		}
	}
	networkRulesJSON := "[]"
	if len(spec.NetworkRules) > 0 {
		if b, err := json.Marshal(spec.NetworkRules); err == nil {
			networkRulesJSON = string(b)
		}
	}

	var cpuRequest, cpuLimit, memoryRequest, memoryLimit string
	if spec.Resources != nil {
		cpuRequest = spec.Resources.CPURequest
		cpuLimit = spec.Resources.CPULimit
		memoryRequest = spec.Resources.MemoryRequest
		memoryLimit = spec.Resources.MemoryLimit
	}

	egressPolicyJSON := ""
	if spec.EgressPolicy != nil && spec.EgressPolicy.Mode != "" {
		if b, err := json.Marshal(spec.EgressPolicy); err == nil {
			egressPolicyJSON = string(b)
		}
	}

	result, err := db.conn.Exec(
		`UPDATE app_specs SET name = ?, description = ?, image = ?, launch_command = ?,
		 cpu_request = ?, cpu_limit = ?, memory_request = ?, memory_limit = ?,
		 env_vars = ?, volumes = ?, network_rules = ?, egress_policy = ?, updated_at = ?
		 WHERE id = ?`,
		spec.Name, spec.Description, spec.Image, spec.LaunchCommand,
		cpuRequest, cpuLimit, memoryRequest, memoryLimit,
		envVarsJSON, volumesJSON, networkRulesJSON, egressPolicyJSON, time.Now(), spec.ID,
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

// DeleteAppSpec removes an application specification by ID
func (db *DB) DeleteAppSpec(id string) error {
	result, err := db.conn.Exec("DELETE FROM app_specs WHERE id = ?", id)
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
	_, err := db.conn.Exec(
		"INSERT INTO oidc_states (state, redirect_url, expires_at) VALUES (?, ?, ?)",
		state, redirectURL, expiresAt,
	)
	return err
}

// ConsumeOIDCState atomically loads and deletes an OIDC state token.
// Returns the redirect URL and expiry, or empty string if not found.
func (db *DB) ConsumeOIDCState(state string) (redirectURL string, expiresAt time.Time, err error) {
	tx, err := db.conn.Begin()
	if err != nil {
		return "", time.Time{}, err
	}
	defer tx.Rollback()

	err = tx.QueryRow(
		"SELECT redirect_url, expires_at FROM oidc_states WHERE state = ?", state,
	).Scan(&redirectURL, &expiresAt)
	if err != nil {
		return "", time.Time{}, err
	}

	_, err = tx.Exec("DELETE FROM oidc_states WHERE state = ?", state)
	if err != nil {
		return "", time.Time{}, err
	}

	if err := tx.Commit(); err != nil {
		return "", time.Time{}, err
	}
	return redirectURL, expiresAt, nil
}

// CleanupExpiredOIDCStates removes expired OIDC state tokens.
func (db *DB) CleanupExpiredOIDCStates() error {
	_, err := db.conn.Exec("DELETE FROM oidc_states WHERE expires_at < ?", time.Now())
	return err
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
	var token *string
	if share.ShareToken != "" {
		token = &share.ShareToken
	}
	_, err := db.conn.Exec(
		"INSERT INTO session_shares (id, session_id, user_id, permission, share_token, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		share.ID, share.SessionID, share.UserID, string(share.Permission), token, share.CreatedBy, share.CreatedAt,
	)
	return err
}

// GetSessionShare returns a session share by ID.
func (db *DB) GetSessionShare(id string) (*SessionShare, error) {
	var share SessionShare
	var perm string
	var token sql.NullString
	err := db.conn.QueryRow(
		"SELECT id, session_id, user_id, permission, share_token, created_by, created_at FROM session_shares WHERE id = ?", id,
	).Scan(&share.ID, &share.SessionID, &share.UserID, &perm, &token, &share.CreatedBy, &share.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	share.Permission = SharePermission(perm)
	if token.Valid {
		share.ShareToken = token.String
	}
	return &share, nil
}

// GetSessionShareByToken returns a session share by its token.
func (db *DB) GetSessionShareByToken(token string) (*SessionShare, error) {
	var share SessionShare
	var perm string
	var tok sql.NullString
	err := db.conn.QueryRow(
		"SELECT id, session_id, user_id, permission, share_token, created_by, created_at FROM session_shares WHERE share_token = ?", token,
	).Scan(&share.ID, &share.SessionID, &share.UserID, &perm, &tok, &share.CreatedBy, &share.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	share.Permission = SharePermission(perm)
	if tok.Valid {
		share.ShareToken = tok.String
	}
	return &share, nil
}

// ListSessionShares returns all shares for a given session.
func (db *DB) ListSessionShares(sessionID string) ([]SessionShare, error) {
	rows, err := db.conn.Query(
		"SELECT id, session_id, user_id, permission, share_token, created_by, created_at FROM session_shares WHERE session_id = ? ORDER BY created_at DESC",
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shares []SessionShare
	for rows.Next() {
		var share SessionShare
		var perm string
		var token sql.NullString
		if err := rows.Scan(&share.ID, &share.SessionID, &share.UserID, &perm, &token, &share.CreatedBy, &share.CreatedAt); err != nil {
			return nil, err
		}
		share.Permission = SharePermission(perm)
		if token.Valid {
			share.ShareToken = token.String
		}
		shares = append(shares, share)
	}
	return shares, rows.Err()
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
	rows, err := db.conn.Query(
		`SELECT s.id, s.user_id, s.app_id, s.pod_name, s.pod_ip, s.status, s.idle_timeout, s.created_at, s.updated_at,
		        COALESCE(a.name, s.app_id), COALESCE(u.username, s.user_id),
		        ss.permission, ss.id
		 FROM session_shares ss
		 JOIN sessions s ON ss.session_id = s.id
		 LEFT JOIN applications a ON s.app_id = a.id
		 LEFT JOIN users u ON s.user_id = u.id
		 WHERE ss.user_id = ? AND s.status NOT IN ('terminated', 'failed', 'stopped', 'expired')
		 ORDER BY s.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SharedSessionRow
	for rows.Next() {
		var r SharedSessionRow
		var status, perm string
		if err := rows.Scan(
			&r.Session.ID, &r.Session.UserID, &r.Session.AppID, &r.Session.PodName,
			&r.Session.PodIP, &status, &r.Session.IdleTimeout, &r.Session.CreatedAt, &r.Session.UpdatedAt,
			&r.AppName, &r.OwnerUsername, &perm, &r.ShareID,
		); err != nil {
			return nil, err
		}
		r.Session.Status = SessionStatus(status)
		r.Permission = SharePermission(perm)
		results = append(results, r)
	}
	return results, rows.Err()
}

// CheckSessionAccess checks whether a user has share access to a session.
func (db *DB) CheckSessionAccess(sessionID, userID string) (*SessionShare, error) {
	var share SessionShare
	var perm string
	var token sql.NullString
	err := db.conn.QueryRow(
		"SELECT id, session_id, user_id, permission, share_token, created_by, created_at FROM session_shares WHERE session_id = ? AND user_id = ? LIMIT 1",
		sessionID, userID,
	).Scan(&share.ID, &share.SessionID, &share.UserID, &perm, &token, &share.CreatedBy, &share.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	share.Permission = SharePermission(perm)
	if token.Valid {
		share.ShareToken = token.String
	}
	return &share, nil
}

// DeleteSessionShare removes a session share by ID.
func (db *DB) DeleteSessionShare(id string) error {
	result, err := db.conn.Exec("DELETE FROM session_shares WHERE id = ?", id)
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
	_, err := db.conn.Exec("DELETE FROM session_shares WHERE session_id = ?", sessionID)
	return err
}

// UpdateSessionShareUserID sets the user_id on a share (used when joining via link).
func (db *DB) UpdateSessionShareUserID(id, userID string) error {
	_, err := db.conn.Exec("UPDATE session_shares SET user_id = ? WHERE id = ?", userID, id)
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
	tenantID := rec.TenantID
	if tenantID == "" {
		tenantID = DefaultTenantID
	}
	_, err := db.conn.Exec(
		`INSERT INTO recordings (id, session_id, user_id, filename, size_bytes, duration_seconds, format, storage_backend, storage_path, status, tenant_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.ID, rec.SessionID, rec.UserID, rec.Filename, rec.SizeBytes, rec.DurationSeconds,
		rec.Format, rec.StorageBackend, rec.StoragePath, string(rec.Status), tenantID, rec.CreatedAt,
	)
	return err
}

// GetRecording returns a recording by ID.
func (db *DB) GetRecording(id string) (*Recording, error) {
	var rec Recording
	var status string
	var completedAt sql.NullTime
	var videoPath sql.NullString
	err := db.conn.QueryRow(
		`SELECT id, session_id, user_id, filename, size_bytes, duration_seconds, format, storage_backend, storage_path, video_path, status, tenant_id, created_at, completed_at
		 FROM recordings WHERE id = ?`, id,
	).Scan(&rec.ID, &rec.SessionID, &rec.UserID, &rec.Filename, &rec.SizeBytes, &rec.DurationSeconds,
		&rec.Format, &rec.StorageBackend, &rec.StoragePath, &videoPath, &status, &rec.TenantID, &rec.CreatedAt, &completedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rec.Status = RecordingStatus(status)
	if completedAt.Valid {
		rec.CompletedAt = &completedAt.Time
	}
	if videoPath.Valid {
		rec.VideoPath = videoPath.String
	}
	return &rec, nil
}

// UpdateRecordingStatus updates the status of a recording.
func (db *DB) UpdateRecordingStatus(id string, status RecordingStatus) error {
	result, err := db.conn.Exec("UPDATE recordings SET status = ? WHERE id = ?", string(status), id)
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
	result, err := db.conn.Exec(
		"UPDATE recordings SET status = ?, storage_path = ?, size_bytes = ?, duration_seconds = ?, completed_at = ? WHERE id = ?",
		string(RecordingStatusReady), storagePath, sizeBytes, durationSeconds, now, id,
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

// UpdateRecordingVideoPath sets the converted video path for a recording.
func (db *DB) UpdateRecordingVideoPath(id string, videoPath string) error {
	result, err := db.conn.Exec(
		"UPDATE recordings SET video_path = ? WHERE id = ?",
		videoPath, id,
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

// ListRecordingsByUser returns all recordings for a given user.
func (db *DB) ListRecordingsByUser(userID string) ([]Recording, error) {
	rows, err := db.conn.Query(
		`SELECT id, session_id, user_id, filename, size_bytes, duration_seconds, format, storage_backend, storage_path, video_path, status, tenant_id, created_at, completed_at
		 FROM recordings WHERE user_id = ? ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecordings(rows)
}

// ListRecordingsBySession returns all recordings for a given session.
func (db *DB) ListRecordingsBySession(sessionID string) ([]Recording, error) {
	rows, err := db.conn.Query(
		`SELECT id, session_id, user_id, filename, size_bytes, duration_seconds, format, storage_backend, storage_path, video_path, status, tenant_id, created_at, completed_at
		 FROM recordings WHERE session_id = ? ORDER BY created_at DESC`, sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecordings(rows)
}

// ListAllRecordings returns all recordings (admin use).
func (db *DB) ListAllRecordings() ([]Recording, error) {
	rows, err := db.conn.Query(
		`SELECT id, session_id, user_id, filename, size_bytes, duration_seconds, format, storage_backend, storage_path, video_path, status, tenant_id, created_at, completed_at
		 FROM recordings ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecordings(rows)
}

// DeleteRecording removes a recording by ID.
func (db *DB) DeleteRecording(id string) error {
	result, err := db.conn.Exec("DELETE FROM recordings WHERE id = ?", id)
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
	rows, err := db.conn.Query(
		`SELECT id, session_id, user_id, filename, size_bytes, duration_seconds, format, storage_backend, storage_path, video_path, status, tenant_id, created_at, completed_at
		 FROM recordings WHERE status = ? AND completed_at < ? ORDER BY completed_at ASC`,
		string(RecordingStatusReady), olderThan,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecordings(rows)
}

func scanRecordings(rows *sql.Rows) ([]Recording, error) {
	var recs []Recording
	for rows.Next() {
		var rec Recording
		var status string
		var completedAt sql.NullTime
		var videoPath sql.NullString
		if err := rows.Scan(&rec.ID, &rec.SessionID, &rec.UserID, &rec.Filename, &rec.SizeBytes, &rec.DurationSeconds,
			&rec.Format, &rec.StorageBackend, &rec.StoragePath, &videoPath, &status, &rec.TenantID, &rec.CreatedAt, &completedAt); err != nil {
			return nil, err
		}
		rec.Status = RecordingStatus(status)
		if completedAt.Valid {
			rec.CompletedAt = &completedAt.Time
		}
		if videoPath.Valid {
			rec.VideoPath = videoPath.String
		}
		recs = append(recs, rec)
	}
	return recs, rows.Err()
}
