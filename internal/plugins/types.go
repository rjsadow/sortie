// Package plugins provides a plugin architecture for extensible connectors.
// It allows adding new app launchers, auth providers, and storage providers
// without modifying core code.
//
// Plugin Types:
//   - Launcher: Handles different application launch mechanisms (URL, container, etc.)
//   - AuthProvider: Handles authentication and authorization
//   - StorageProvider: Handles data persistence
//
// Adding new plugins:
//  1. Implement the appropriate interface (Launcher, AuthProvider, or StorageProvider)
//  2. Register the plugin with the Registry
//  3. Configure via environment variables or config file
package plugins

import (
	"context"
	"errors"
	"time"
)

// Common errors returned by plugins.
var (
	ErrPluginNotFound    = errors.New("plugin not found")
	ErrPluginNotReady    = errors.New("plugin not ready")
	ErrInvalidConfig     = errors.New("invalid plugin configuration")
	ErrOperationFailed   = errors.New("plugin operation failed")
	ErrNotImplemented    = errors.New("operation not implemented")
	ErrAuthRequired      = errors.New("authentication required")
	ErrPermissionDenied  = errors.New("permission denied")
	ErrResourceNotFound  = errors.New("resource not found")
	ErrResourceExists    = errors.New("resource already exists")
	ErrConnectionFailed  = errors.New("connection failed")
	ErrTimeout           = errors.New("operation timed out")
)

// PluginType represents the category of a plugin.
type PluginType string

const (
	PluginTypeLauncher PluginType = "launcher"
	PluginTypeAuth     PluginType = "auth"
	PluginTypeStorage  PluginType = "storage"
)

// Plugin is the base interface all plugins must implement.
type Plugin interface {
	// Name returns the unique identifier for this plugin.
	Name() string

	// Type returns the plugin type (launcher, auth, storage).
	Type() PluginType

	// Version returns the plugin version.
	Version() string

	// Description returns a human-readable description.
	Description() string

	// Initialize sets up the plugin with the given configuration.
	// Called once during application startup.
	Initialize(ctx context.Context, config map[string]string) error

	// Healthy returns true if the plugin is operational.
	Healthy(ctx context.Context) bool

	// Close releases any resources held by the plugin.
	Close() error
}

// PluginInfo contains metadata about a registered plugin.
type PluginInfo struct {
	Name        string            `json:"name"`
	Type        PluginType        `json:"type"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Healthy     bool              `json:"healthy"`
	Config      map[string]string `json:"config,omitempty"`
}

// HealthStatus represents the health check result for a plugin.
type HealthStatus struct {
	PluginName string    `json:"plugin_name"`
	PluginType PluginType `json:"plugin_type"`
	Healthy    bool      `json:"healthy"`
	Message    string    `json:"message,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
}

// PluginFactory is a function that creates a new instance of a plugin.
type PluginFactory func() Plugin

// ConfigSchema defines the configuration options for a plugin.
type ConfigSchema struct {
	// Fields lists the configuration fields.
	Fields []ConfigField `json:"fields"`
}

// ConfigField describes a single configuration option.
type ConfigField struct {
	// Name is the configuration key.
	Name string `json:"name"`

	// Type is the value type (string, int, bool, etc.).
	Type string `json:"type"`

	// Required indicates if this field must be provided.
	Required bool `json:"required"`

	// Default is the default value if not provided.
	Default string `json:"default,omitempty"`

	// Description explains what this field configures.
	Description string `json:"description"`

	// EnvVar is the environment variable that can set this field.
	EnvVar string `json:"env_var,omitempty"`
}

// Configurable is an optional interface for plugins that expose their config schema.
type Configurable interface {
	// ConfigSchema returns the configuration schema for this plugin.
	ConfigSchema() ConfigSchema
}

// LaunchType identifies how an application should be launched.
type LaunchType string

const (
	LaunchTypeURL       LaunchType = "url"
	LaunchTypeContainer LaunchType = "container"
)

// LaunchRequest contains parameters for launching an application.
type LaunchRequest struct {
	AppID          string            `json:"app_id"`
	AppName        string            `json:"app_name"`
	UserID         string            `json:"user_id"`
	LaunchType     LaunchType        `json:"launch_type"`
	URL            string            `json:"url,omitempty"`
	ContainerImage string            `json:"container_image,omitempty"`
	ResourceLimits *ResourceLimits   `json:"resource_limits,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// ResourceLimits specifies CPU and memory constraints for containers.
type ResourceLimits struct {
	CPURequest    string `json:"cpu_request,omitempty"`
	CPULimit      string `json:"cpu_limit,omitempty"`
	MemoryRequest string `json:"memory_request,omitempty"`
	MemoryLimit   string `json:"memory_limit,omitempty"`
}

// LaunchResult contains the result of a launch operation.
type LaunchResult struct {
	SessionID    string            `json:"session_id"`
	Status       LaunchStatus      `json:"status"`
	URL          string            `json:"url,omitempty"`
	WebSocketURL string            `json:"websocket_url,omitempty"`
	Message      string            `json:"message,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// LaunchStatus represents the status of a launch operation.
type LaunchStatus string

const (
	LaunchStatusPending  LaunchStatus = "pending"
	LaunchStatusCreating LaunchStatus = "creating"
	LaunchStatusRunning  LaunchStatus = "running"
	LaunchStatusFailed   LaunchStatus = "failed"
	LaunchStatusStopped  LaunchStatus = "stopped"
	LaunchStatusExpired  LaunchStatus = "expired"
	LaunchStatusRedirect LaunchStatus = "redirect"
)

// LauncherPlugin defines the interface for application launchers.
type LauncherPlugin interface {
	Plugin

	// SupportedTypes returns the launch types this launcher supports.
	SupportedTypes() []LaunchType

	// Launch starts an application and returns the result.
	Launch(ctx context.Context, req *LaunchRequest) (*LaunchResult, error)

	// GetStatus returns the current status of a launch session.
	GetStatus(ctx context.Context, sessionID string) (*LaunchResult, error)

	// Terminate stops a running launch session.
	Terminate(ctx context.Context, sessionID string) error

	// ListSessions returns all active sessions for a user.
	ListSessions(ctx context.Context, userID string) ([]*LaunchResult, error)
}

// User represents an authenticated user.
type User struct {
	ID       string            `json:"id"`
	Username string            `json:"username"`
	Email    string            `json:"email,omitempty"`
	Name     string            `json:"name,omitempty"`
	Roles    []string          `json:"roles,omitempty"`
	Groups   []string          `json:"groups,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// AuthResult contains the result of an authentication operation.
type AuthResult struct {
	Authenticated bool   `json:"authenticated"`
	User          *User  `json:"user,omitempty"`
	Token         string `json:"token,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	Message       string `json:"message,omitempty"`
}

// AuthProvider defines the interface for authentication providers.
type AuthProvider interface {
	Plugin

	// Authenticate validates credentials and returns the authenticated user.
	Authenticate(ctx context.Context, token string) (*AuthResult, error)

	// GetUser retrieves user information by ID.
	GetUser(ctx context.Context, userID string) (*User, error)

	// HasPermission checks if a user has a specific permission.
	HasPermission(ctx context.Context, userID, permission string) (bool, error)

	// GetLoginURL returns the URL for initiating login (for OAuth/OIDC).
	GetLoginURL(redirectURL string) string

	// HandleCallback processes the OAuth/OIDC callback.
	HandleCallback(ctx context.Context, code, state string) (*AuthResult, error)

	// Logout invalidates the user's session.
	Logout(ctx context.Context, token string) error
}

// Application represents an application in storage.
type Application struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Description    string          `json:"description,omitempty"`
	URL            string          `json:"url"`
	Icon           string          `json:"icon,omitempty"`
	Category       string          `json:"category,omitempty"`
	LaunchType     LaunchType      `json:"launch_type,omitempty"`
	ContainerImage string          `json:"container_image,omitempty"`
	ResourceLimits *ResourceLimits `json:"resource_limits,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// Session represents a launch session in storage.
type Session struct {
	ID        string       `json:"id"`
	AppID     string       `json:"app_id"`
	UserID    string       `json:"user_id"`
	Status    LaunchStatus `json:"status"`
	PodName   string       `json:"pod_name,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
	ExpiresAt *time.Time   `json:"expires_at,omitempty"`
}

// AuditEntry represents an audit log entry.
type AuditEntry struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Action    string    `json:"action"`
	Details   string    `json:"details,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// StorageProvider defines the interface for data persistence.
type StorageProvider interface {
	Plugin

	// Application CRUD
	CreateApp(ctx context.Context, app *Application) error
	GetApp(ctx context.Context, id string) (*Application, error)
	UpdateApp(ctx context.Context, app *Application) error
	DeleteApp(ctx context.Context, id string) error
	ListApps(ctx context.Context) ([]*Application, error)

	// Session management
	CreateSession(ctx context.Context, session *Session) error
	GetSession(ctx context.Context, id string) (*Session, error)
	UpdateSession(ctx context.Context, session *Session) error
	DeleteSession(ctx context.Context, id string) error
	ListSessions(ctx context.Context, userID string) ([]*Session, error)
	ListExpiredSessions(ctx context.Context) ([]*Session, error)

	// Audit logging
	LogAudit(ctx context.Context, entry *AuditEntry) error
	GetAuditLogs(ctx context.Context, limit int) ([]*AuditEntry, error)

	// Analytics (optional)
	RecordLaunch(ctx context.Context, appID string) error
	GetAnalyticsStats(ctx context.Context) (map[string]interface{}, error)
}
