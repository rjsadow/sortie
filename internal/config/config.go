// Package config provides centralized configuration management for Sortie.
// Configuration is loaded from environment variables with sensible defaults.
// Required configuration that is missing will cause the application to fail fast
// with helpful error messages.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all application configuration.
type Config struct {
	// Server configuration
	Port int
	DB   string // SQLite file path (backward compat, maps to DBPath)
	Seed string

	// Database configuration
	DBType     string // "sqlite" (default) or "postgres"
	DBPath     string // SQLite file path (when DBType="sqlite")
	DBDSN      string // Full PostgreSQL DSN (takes precedence over individual params)
	DBHost     string // PostgreSQL host
	DBPort     int    // PostgreSQL port (default: 5432)
	DBName     string // PostgreSQL database name
	DBUser     string // PostgreSQL user
	DBPassword string // PostgreSQL password
	DBSSLMode  string // PostgreSQL SSL mode (default: "disable")

	// Branding configuration
	BrandingConfigPath string
	LogoURL            string
	PrimaryColor       string
	SecondaryColor     string
	TenantName         string

	// Kubernetes configuration
	Namespace          string
	Kubeconfig         string
	VNCSidecarImage      string
	BrowserSidecarImage  string
	GuacdSidecarImage    string

	// Session configuration
	SessionTimeout         time.Duration
	SessionCleanupInterval time.Duration
	PodReadyTimeout        time.Duration

	// JWT Authentication configuration
	JWTSecret            string
	JWTAccessExpiry      time.Duration
	JWTRefreshExpiry     time.Duration
	AdminUsername        string
	AdminPassword        string
	AllowRegistration    bool

	// OIDC/SSO configuration
	OIDCIssuer       string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCRedirectURL  string
	OIDCScopes       string

	// File transfer configuration
	MaxUploadSize int64 // Maximum upload file size in bytes

	// Gateway configuration
	GatewayRateLimit float64 // Requests per second per IP (0 = disabled)
	GatewayBurst     int     // Maximum burst size for rate limiter

	// Resource quota configuration
	MaxSessionsPerUser int    // Maximum concurrent sessions per user (0 = unlimited)
	MaxGlobalSessions  int    // Maximum concurrent sessions globally (0 = unlimited)
	DefaultCPURequest  string // Default CPU request for sessions (e.g., "500m")
	DefaultCPULimit    string // Default CPU limit for sessions (e.g., "2")
	DefaultMemRequest  string // Default memory request for sessions (e.g., "512Mi")
	DefaultMemLimit    string // Default memory limit for sessions (e.g., "2Gi")

	// Session recording configuration
	RecordingEnabled    bool   // Enable session lifecycle event recording
	RecordingEndpoint   string // Optional endpoint for recording events
	RecordingBufferSize int    // Buffer size for async event processing

	// Video recording configuration
	VideoRecordingEnabled  bool   // Enable client-side video recording of sessions
	RecordingStorageBackend string // Storage backend: "local" or "s3"
	RecordingStoragePath   string // Local storage path for recordings
	RecordingMaxSizeMB     int    // Maximum recording upload size in MB
	RecordingRetentionDays int    // Days to retain recordings (0 = keep forever)
	RecordingS3Bucket   string // S3 bucket name
	RecordingS3Region   string // AWS region
	RecordingS3Endpoint string // Custom endpoint for MinIO/self-hosted S3
	RecordingS3Prefix      string // Key prefix within bucket
	RecordingS3AccessKeyID     string // Explicit AWS access key ID (optional)
	RecordingS3SecretAccessKey string // Explicit AWS secret access key (optional)

	// Billing/metering configuration
	BillingEnabled        bool          // Enable metering event collection
	BillingExporter       string        // Exporter type: "log" or "webhook"
	BillingWebhookURL     string        // Webhook URL for billing export (when exporter=webhook)
	BillingExportInterval time.Duration // How often to export metering events

	// Session queueing configuration
	QueueMaxSize      int           // Max queued requests when at capacity (0 = no queueing)
	QueueTimeout      time.Duration // Per-request queue wait timeout
}

// ValidationError represents a configuration validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationErrors holds multiple validation errors.
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	var msgs []string
	for _, err := range e {
		msgs = append(msgs, err.Error())
	}
	return fmt.Sprintf("configuration errors:\n  - %s", strings.Join(msgs, "\n  - "))
}

// Default values
const (
	DefaultPort                   = 8080
	DefaultDBPath                 = "sortie.db"
	DefaultDBType                 = "sqlite"
	DefaultDBPort                 = 5432
	DefaultDBSSLMode              = "disable"
	DefaultBrandingConfigPath     = "branding.json"
	DefaultPrimaryColor           = "#1F2A3C"
	DefaultSecondaryColor         = "#2B3445"
	DefaultTenantName             = "Sortie"
	DefaultNamespace              = "default"
	DefaultVNCSidecarImage        = "ghcr.io/rjsadow/sortie-vnc-sidecar:latest"
	DefaultBrowserSidecarImage    = "ghcr.io/rjsadow/sortie-browser-sidecar:latest"
	DefaultGuacdSidecarImage      = "guacamole/guacd:1.6.0"
	DefaultSessionTimeout         = 2 * time.Hour
	DefaultSessionCleanupInterval = 5 * time.Minute
	DefaultPodReadyTimeout        = 2 * time.Minute
	DefaultJWTAccessExpiry        = 15 * time.Minute
	DefaultJWTRefreshExpiry       = 24 * time.Hour
	DefaultAdminUsername          = "admin"
	DefaultMaxUploadSize         = int64(100 * 1024 * 1024) // 100MB
	DefaultGatewayRateLimit      = float64(10)              // 10 requests/sec per IP
	DefaultGatewayBurst          = 20                       // burst of 20
	DefaultMaxSessionsPerUser    = 5
	DefaultMaxGlobalSessions     = 100
	DefaultBillingExporter       = "log"
	DefaultBillingExportInterval = 5 * time.Minute
	DefaultDefaultCPURequest     = "500m"
	DefaultDefaultCPULimit       = "2"
	DefaultDefaultMemRequest     = "512Mi"
	DefaultDefaultMemLimit       = "2Gi"
	DefaultQueueMaxSize          = 0                       // disabled by default
	DefaultQueueTimeout          = 30 * time.Second
	DefaultRecordingStorageBackend = "local"
	DefaultRecordingStoragePath    = "/data/recordings"
	DefaultRecordingMaxSizeMB      = 500
	DefaultRecordingS3Region       = "us-east-1"
	DefaultRecordingS3Prefix       = "recordings/"
)

// Load reads configuration from environment variables and returns a Config.
// It applies defaults for optional values and validates the configuration.
// Returns an error if validation fails.
func Load() (*Config, error) {
	cfg := &Config{
		// Server defaults
		Port: DefaultPort,
		DB:   DefaultDBPath,

		// Database defaults
		DBType:    DefaultDBType,
		DBPort:    DefaultDBPort,
		DBSSLMode: DefaultDBSSLMode,

		// Branding defaults
		BrandingConfigPath: DefaultBrandingConfigPath,
		PrimaryColor:       DefaultPrimaryColor,
		SecondaryColor:     DefaultSecondaryColor,
		TenantName:         DefaultTenantName,

		// Kubernetes defaults
		Namespace:         DefaultNamespace,
		VNCSidecarImage:     DefaultVNCSidecarImage,
		BrowserSidecarImage: DefaultBrowserSidecarImage,
		GuacdSidecarImage:   DefaultGuacdSidecarImage,

		// Session defaults
		SessionTimeout:         DefaultSessionTimeout,
		SessionCleanupInterval: DefaultSessionCleanupInterval,
		PodReadyTimeout:        DefaultPodReadyTimeout,

		// JWT defaults
		JWTAccessExpiry:  DefaultJWTAccessExpiry,
		JWTRefreshExpiry: DefaultJWTRefreshExpiry,
		AdminUsername:    DefaultAdminUsername,

		// File transfer defaults
		MaxUploadSize: DefaultMaxUploadSize,

		// Gateway defaults
		GatewayRateLimit: DefaultGatewayRateLimit,
		GatewayBurst:     DefaultGatewayBurst,

		// Resource quota defaults
		MaxSessionsPerUser: DefaultMaxSessionsPerUser,
		MaxGlobalSessions:  DefaultMaxGlobalSessions,
		DefaultCPURequest:  DefaultDefaultCPURequest,
		DefaultCPULimit:    DefaultDefaultCPULimit,
		DefaultMemRequest:  DefaultDefaultMemRequest,
		DefaultMemLimit:    DefaultDefaultMemLimit,

		// Video recording defaults
		RecordingStorageBackend: DefaultRecordingStorageBackend,
		RecordingStoragePath:    DefaultRecordingStoragePath,
		RecordingMaxSizeMB:      DefaultRecordingMaxSizeMB,
		RecordingS3Region:       DefaultRecordingS3Region,
		RecordingS3Prefix:       DefaultRecordingS3Prefix,

		// Queue defaults
		QueueMaxSize: DefaultQueueMaxSize,
		QueueTimeout: DefaultQueueTimeout,
	}

	// Load from environment variables
	if err := cfg.loadFromEnv(); err != nil {
		return nil, err
	}

	// Validate configuration
	if errs := cfg.Validate(); len(errs) > 0 {
		return nil, errs
	}

	return cfg, nil
}

// loadFromEnv populates the config from environment variables.
func (c *Config) loadFromEnv() error {
	var parseErrors ValidationErrors

	// Server configuration
	if v := os.Getenv("SORTIE_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_PORT",
				Message: fmt.Sprintf("invalid port number: %q (must be an integer)", v),
			})
		} else {
			c.Port = port
		}
	}

	if v := os.Getenv("SORTIE_DB"); v != "" {
		c.DB = v
	}

	if v := os.Getenv("SORTIE_SEED"); v != "" {
		c.Seed = v
	}

	// Database configuration
	if v := os.Getenv("SORTIE_DB_TYPE"); v != "" {
		c.DBType = v
	}
	if v := os.Getenv("SORTIE_DB_DSN"); v != "" {
		c.DBDSN = v
	}
	if v := os.Getenv("SORTIE_DB_HOST"); v != "" {
		c.DBHost = v
	}
	if v := os.Getenv("SORTIE_DB_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_DB_PORT",
				Message: fmt.Sprintf("invalid port number: %q (must be an integer)", v),
			})
		} else {
			c.DBPort = port
		}
	}
	if v := os.Getenv("SORTIE_DB_NAME"); v != "" {
		c.DBName = v
	}
	if v := os.Getenv("SORTIE_DB_USER"); v != "" {
		c.DBUser = v
	}
	if v := os.Getenv("SORTIE_DB_PASSWORD"); v != "" {
		c.DBPassword = v
	}
	if v := os.Getenv("SORTIE_DB_SSLMODE"); v != "" {
		c.DBSSLMode = v
	}

	// Sync DBPath with DB for backward compatibility
	c.DBPath = c.DB

	// Branding configuration
	if v := os.Getenv("SORTIE_CONFIG"); v != "" {
		c.BrandingConfigPath = v
	}

	if v := os.Getenv("SORTIE_LOGO_URL"); v != "" {
		c.LogoURL = v
	}

	if v := os.Getenv("SORTIE_PRIMARY_COLOR"); v != "" {
		c.PrimaryColor = v
	}

	if v := os.Getenv("SORTIE_SECONDARY_COLOR"); v != "" {
		c.SecondaryColor = v
	}

	if v := os.Getenv("SORTIE_TENANT_NAME"); v != "" {
		c.TenantName = v
	}

	// Kubernetes configuration
	if v := os.Getenv("SORTIE_NAMESPACE"); v != "" {
		c.Namespace = v
	}

	if v := os.Getenv("KUBECONFIG"); v != "" {
		c.Kubeconfig = v
	}

	if v := os.Getenv("SORTIE_VNC_SIDECAR_IMAGE"); v != "" {
		c.VNCSidecarImage = v
	}

	if v := os.Getenv("SORTIE_BROWSER_SIDECAR_IMAGE"); v != "" {
		c.BrowserSidecarImage = v
	}

	if v := os.Getenv("SORTIE_GUACD_SIDECAR_IMAGE"); v != "" {
		c.GuacdSidecarImage = v
	}

	// Session configuration
	if v := os.Getenv("SORTIE_SESSION_TIMEOUT"); v != "" {
		minutes, err := strconv.Atoi(v)
		if err != nil {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_SESSION_TIMEOUT",
				Message: fmt.Sprintf("invalid timeout: %q (must be an integer representing minutes)", v),
			})
		} else if minutes <= 0 {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_SESSION_TIMEOUT",
				Message: fmt.Sprintf("timeout must be positive: %d", minutes),
			})
		} else {
			c.SessionTimeout = time.Duration(minutes) * time.Minute
		}
	}

	if v := os.Getenv("SORTIE_SESSION_CLEANUP_INTERVAL"); v != "" {
		minutes, err := strconv.Atoi(v)
		if err != nil {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_SESSION_CLEANUP_INTERVAL",
				Message: fmt.Sprintf("invalid interval: %q (must be an integer representing minutes)", v),
			})
		} else if minutes <= 0 {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_SESSION_CLEANUP_INTERVAL",
				Message: fmt.Sprintf("interval must be positive: %d", minutes),
			})
		} else {
			c.SessionCleanupInterval = time.Duration(minutes) * time.Minute
		}
	}

	if v := os.Getenv("SORTIE_POD_READY_TIMEOUT"); v != "" {
		seconds, err := strconv.Atoi(v)
		if err != nil {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_POD_READY_TIMEOUT",
				Message: fmt.Sprintf("invalid timeout: %q (must be an integer representing seconds)", v),
			})
		} else if seconds <= 0 {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_POD_READY_TIMEOUT",
				Message: fmt.Sprintf("timeout must be positive: %d", seconds),
			})
		} else {
			c.PodReadyTimeout = time.Duration(seconds) * time.Second
		}
	}

	// JWT configuration
	if v := os.Getenv("SORTIE_JWT_SECRET"); v != "" {
		c.JWTSecret = v
	}

	if v := os.Getenv("SORTIE_JWT_ACCESS_EXPIRY"); v != "" {
		minutes, err := strconv.Atoi(v)
		if err != nil {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_JWT_ACCESS_EXPIRY",
				Message: fmt.Sprintf("invalid expiry: %q (must be an integer representing minutes)", v),
			})
		} else if minutes <= 0 {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_JWT_ACCESS_EXPIRY",
				Message: fmt.Sprintf("expiry must be positive: %d", minutes),
			})
		} else {
			c.JWTAccessExpiry = time.Duration(minutes) * time.Minute
		}
	}

	if v := os.Getenv("SORTIE_JWT_REFRESH_EXPIRY"); v != "" {
		hours, err := strconv.Atoi(v)
		if err != nil {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_JWT_REFRESH_EXPIRY",
				Message: fmt.Sprintf("invalid expiry: %q (must be an integer representing hours)", v),
			})
		} else if hours <= 0 {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_JWT_REFRESH_EXPIRY",
				Message: fmt.Sprintf("expiry must be positive: %d", hours),
			})
		} else {
			c.JWTRefreshExpiry = time.Duration(hours) * time.Hour
		}
	}

	if v := os.Getenv("SORTIE_ADMIN_USERNAME"); v != "" {
		c.AdminUsername = v
	}

	if v := os.Getenv("SORTIE_ADMIN_PASSWORD"); v != "" {
		c.AdminPassword = v
	}

	if v := os.Getenv("SORTIE_ALLOW_REGISTRATION"); v != "" {
		c.AllowRegistration = strings.EqualFold(v, "true") || v == "1"
	}

	// OIDC/SSO configuration
	if v := os.Getenv("SORTIE_OIDC_ISSUER"); v != "" {
		c.OIDCIssuer = v
	}
	if v := os.Getenv("SORTIE_OIDC_CLIENT_ID"); v != "" {
		c.OIDCClientID = v
	}
	if v := os.Getenv("SORTIE_OIDC_CLIENT_SECRET"); v != "" {
		c.OIDCClientSecret = v
	}
	if v := os.Getenv("SORTIE_OIDC_REDIRECT_URL"); v != "" {
		c.OIDCRedirectURL = v
	}
	if v := os.Getenv("SORTIE_OIDC_SCOPES"); v != "" {
		c.OIDCScopes = v
	}

	// File transfer configuration
	if v := os.Getenv("SORTIE_MAX_UPLOAD_SIZE"); v != "" {
		size, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_MAX_UPLOAD_SIZE",
				Message: fmt.Sprintf("invalid size: %q (must be an integer representing bytes)", v),
			})
		} else if size <= 0 {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_MAX_UPLOAD_SIZE",
				Message: fmt.Sprintf("size must be positive: %d", size),
			})
		} else {
			c.MaxUploadSize = size
		}
	}

	// Gateway configuration
	if v := os.Getenv("SORTIE_GATEWAY_RATE_LIMIT"); v != "" {
		rl, err := strconv.ParseFloat(v, 64)
		if err != nil {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_GATEWAY_RATE_LIMIT",
				Message: fmt.Sprintf("invalid rate: %q (must be a number)", v),
			})
		} else if rl < 0 {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_GATEWAY_RATE_LIMIT",
				Message: fmt.Sprintf("rate must be non-negative: %v", rl),
			})
		} else {
			c.GatewayRateLimit = rl
		}
	}

	if v := os.Getenv("SORTIE_GATEWAY_BURST"); v != "" {
		b, err := strconv.Atoi(v)
		if err != nil {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_GATEWAY_BURST",
				Message: fmt.Sprintf("invalid burst: %q (must be an integer)", v),
			})
		} else if b < 1 {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_GATEWAY_BURST",
				Message: fmt.Sprintf("burst must be positive: %d", b),
			})
		} else {
			c.GatewayBurst = b
		}
	}

	// Resource quota configuration
	if v := os.Getenv("SORTIE_MAX_SESSIONS_PER_USER"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_MAX_SESSIONS_PER_USER",
				Message: fmt.Sprintf("invalid value: %q (must be an integer)", v),
			})
		} else if n < 0 {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_MAX_SESSIONS_PER_USER",
				Message: fmt.Sprintf("value must be non-negative: %d", n),
			})
		} else {
			c.MaxSessionsPerUser = n
		}
	}

	if v := os.Getenv("SORTIE_MAX_GLOBAL_SESSIONS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_MAX_GLOBAL_SESSIONS",
				Message: fmt.Sprintf("invalid value: %q (must be an integer)", v),
			})
		} else if n < 0 {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_MAX_GLOBAL_SESSIONS",
				Message: fmt.Sprintf("value must be non-negative: %d", n),
			})
		} else {
			c.MaxGlobalSessions = n
		}
	}

	if v := os.Getenv("SORTIE_DEFAULT_CPU_REQUEST"); v != "" {
		c.DefaultCPURequest = v
	}

	if v := os.Getenv("SORTIE_DEFAULT_CPU_LIMIT"); v != "" {
		c.DefaultCPULimit = v
	}

	if v := os.Getenv("SORTIE_DEFAULT_MEM_REQUEST"); v != "" {
		c.DefaultMemRequest = v
	}

	if v := os.Getenv("SORTIE_DEFAULT_MEM_LIMIT"); v != "" {
		c.DefaultMemLimit = v
	}

	// Session recording configuration
	if v := os.Getenv("SORTIE_RECORDING_ENABLED"); v != "" {
		c.RecordingEnabled = strings.EqualFold(v, "true") || v == "1"
	}

	if v := os.Getenv("SORTIE_RECORDING_ENDPOINT"); v != "" {
		c.RecordingEndpoint = v
	}

	if v := os.Getenv("SORTIE_RECORDING_BUFFER_SIZE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_RECORDING_BUFFER_SIZE",
				Message: fmt.Sprintf("invalid value: %q (must be an integer)", v),
			})
		} else if n < 0 {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_RECORDING_BUFFER_SIZE",
				Message: fmt.Sprintf("value must be non-negative: %d", n),
			})
		} else {
			c.RecordingBufferSize = n
		}
	}

	// Billing/metering configuration
	if v := os.Getenv("SORTIE_BILLING_ENABLED"); v != "" {
		c.BillingEnabled = strings.EqualFold(v, "true") || v == "1"
	}

	if v := os.Getenv("SORTIE_BILLING_EXPORTER"); v != "" {
		c.BillingExporter = v
	} else if c.BillingExporter == "" {
		c.BillingExporter = DefaultBillingExporter
	}

	if v := os.Getenv("SORTIE_BILLING_WEBHOOK_URL"); v != "" {
		c.BillingWebhookURL = v
	}

	if v := os.Getenv("SORTIE_BILLING_EXPORT_INTERVAL"); v != "" {
		minutes, err := strconv.Atoi(v)
		if err != nil {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_BILLING_EXPORT_INTERVAL",
				Message: fmt.Sprintf("invalid interval: %q (must be an integer representing minutes)", v),
			})
		} else if minutes <= 0 {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_BILLING_EXPORT_INTERVAL",
				Message: fmt.Sprintf("interval must be positive: %d", minutes),
			})
		} else {
			c.BillingExportInterval = time.Duration(minutes) * time.Minute
		}
	} else if c.BillingExportInterval == 0 {
		c.BillingExportInterval = DefaultBillingExportInterval
	}

	// Video recording configuration
	if v := os.Getenv("SORTIE_VIDEO_RECORDING_ENABLED"); v != "" {
		c.VideoRecordingEnabled = strings.EqualFold(v, "true") || v == "1"
	}

	if v := os.Getenv("SORTIE_RECORDING_STORAGE_BACKEND"); v != "" {
		c.RecordingStorageBackend = v
	}

	if v := os.Getenv("SORTIE_RECORDING_STORAGE_PATH"); v != "" {
		c.RecordingStoragePath = v
	}

	if v := os.Getenv("SORTIE_RECORDING_MAX_SIZE_MB"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_RECORDING_MAX_SIZE_MB",
				Message: fmt.Sprintf("invalid value: %q (must be an integer)", v),
			})
		} else if n <= 0 {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_RECORDING_MAX_SIZE_MB",
				Message: fmt.Sprintf("value must be positive: %d", n),
			})
		} else {
			c.RecordingMaxSizeMB = n
		}
	}

	if v := os.Getenv("SORTIE_RECORDING_RETENTION_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_RECORDING_RETENTION_DAYS",
				Message: fmt.Sprintf("invalid value: %q (must be an integer)", v),
			})
		} else if n < 0 {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_RECORDING_RETENTION_DAYS",
				Message: fmt.Sprintf("value must be non-negative: %d", n),
			})
		} else {
			c.RecordingRetentionDays = n
		}
	}

	if v := os.Getenv("SORTIE_RECORDING_S3_BUCKET"); v != "" {
		c.RecordingS3Bucket = v
	}

	if v := os.Getenv("SORTIE_RECORDING_S3_REGION"); v != "" {
		c.RecordingS3Region = v
	}

	if v := os.Getenv("SORTIE_RECORDING_S3_ENDPOINT"); v != "" {
		c.RecordingS3Endpoint = v
	}

	if v := os.Getenv("SORTIE_RECORDING_S3_PREFIX"); v != "" {
		c.RecordingS3Prefix = v
	}

	if v := os.Getenv("SORTIE_RECORDING_S3_ACCESS_KEY_ID"); v != "" {
		c.RecordingS3AccessKeyID = v
	}

	if v := os.Getenv("SORTIE_RECORDING_S3_SECRET_ACCESS_KEY"); v != "" {
		c.RecordingS3SecretAccessKey = v
	}

	// Session queueing configuration
	if v := os.Getenv("SORTIE_QUEUE_MAX_SIZE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_QUEUE_MAX_SIZE",
				Message: fmt.Sprintf("invalid value: %q (must be an integer)", v),
			})
		} else if n < 0 {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_QUEUE_MAX_SIZE",
				Message: fmt.Sprintf("value must be non-negative: %d", n),
			})
		} else {
			c.QueueMaxSize = n
		}
	}

	if v := os.Getenv("SORTIE_QUEUE_TIMEOUT"); v != "" {
		seconds, err := strconv.Atoi(v)
		if err != nil {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_QUEUE_TIMEOUT",
				Message: fmt.Sprintf("invalid timeout: %q (must be an integer representing seconds)", v),
			})
		} else if seconds <= 0 {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "SORTIE_QUEUE_TIMEOUT",
				Message: fmt.Sprintf("timeout must be positive: %d", seconds),
			})
		} else {
			c.QueueTimeout = time.Duration(seconds) * time.Second
		}
	}

	if len(parseErrors) > 0 {
		return parseErrors
	}
	return nil
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() ValidationErrors {
	var errs ValidationErrors

	// Validate port
	if c.Port < 1 || c.Port > 65535 {
		errs = append(errs, ValidationError{
			Field:   "SORTIE_PORT",
			Message: fmt.Sprintf("port must be between 1 and 65535, got %d", c.Port),
		})
	}

	// Validate DB type
	switch c.DBType {
	case "sqlite":
		if c.DB == "" {
			errs = append(errs, ValidationError{
				Field:   "SORTIE_DB",
				Message: "database path cannot be empty",
			})
		}
	case "postgres":
		if c.DBDSN == "" && (c.DBHost == "" || c.DBName == "" || c.DBUser == "") {
			errs = append(errs, ValidationError{
				Field:   "SORTIE_DB_DSN",
				Message: "PostgreSQL requires either SORTIE_DB_DSN or all of SORTIE_DB_HOST, SORTIE_DB_NAME, and SORTIE_DB_USER",
			})
		}
	default:
		errs = append(errs, ValidationError{
			Field:   "SORTIE_DB_TYPE",
			Message: fmt.Sprintf("unsupported database type: %q (must be \"sqlite\" or \"postgres\")", c.DBType),
		})
	}

	// Validate color format (basic check for hex color)
	if c.PrimaryColor != "" && !isValidHexColor(c.PrimaryColor) {
		errs = append(errs, ValidationError{
			Field:   "SORTIE_PRIMARY_COLOR",
			Message: fmt.Sprintf("invalid hex color: %q (expected format: #RRGGBB)", c.PrimaryColor),
		})
	}

	if c.SecondaryColor != "" && !isValidHexColor(c.SecondaryColor) {
		errs = append(errs, ValidationError{
			Field:   "SORTIE_SECONDARY_COLOR",
			Message: fmt.Sprintf("invalid hex color: %q (expected format: #RRGGBB)", c.SecondaryColor),
		})
	}

	// Validate VNC sidecar image is not empty
	if c.VNCSidecarImage == "" {
		errs = append(errs, ValidationError{
			Field:   "SORTIE_VNC_SIDECAR_IMAGE",
			Message: "VNC sidecar image cannot be empty",
		})
	}

	// Validate S3 config when S3 backend is selected
	if c.RecordingStorageBackend == "s3" && c.RecordingS3Bucket == "" {
		errs = append(errs, ValidationError{
			Field:   "SORTIE_RECORDING_S3_BUCKET",
			Message: "S3 bucket is required when storage backend is \"s3\"",
		})
	}

	// Validate S3 credentials: if one is set, both must be set
	if (c.RecordingS3AccessKeyID != "") != (c.RecordingS3SecretAccessKey != "") {
		errs = append(errs, ValidationError{
			Field:   "SORTIE_RECORDING_S3_ACCESS_KEY_ID / SORTIE_RECORDING_S3_SECRET_ACCESS_KEY",
			Message: "both S3 access key ID and secret access key must be set together",
		})
	}

	return errs
}

// isValidHexColor checks if a string is a valid hex color code.
func isValidHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		isDigit := c >= '0' && c <= '9'
		isLowerHex := c >= 'a' && c <= 'f'
		isUpperHex := c >= 'A' && c <= 'F'
		if !isDigit && !isLowerHex && !isUpperHex {
			return false
		}
	}
	return true
}

// DSN returns the database connection string based on the configured database type.
// For SQLite, it returns the file path. For PostgreSQL, it constructs a DSN from
// individual parameters or returns the explicit DSN if set.
func (c *Config) DSN() string {
	switch c.DBType {
	case "postgres":
		if c.DBDSN != "" {
			return c.DBDSN
		}
		dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
			c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode)
		return dsn
	default:
		return c.DB
	}
}

// IsSQLite returns true if the configured database type is SQLite.
func (c *Config) IsSQLite() bool {
	return c.DBType == "" || c.DBType == "sqlite"
}

// IsPostgres returns true if the configured database type is PostgreSQL.
func (c *Config) IsPostgres() bool {
	return c.DBType == "postgres"
}

// OIDCEnabled returns true if OIDC/SSO is configured with the minimum required fields.
func (c *Config) OIDCEnabled() bool {
	return c.OIDCIssuer != "" && c.OIDCClientID != "" && c.OIDCClientSecret != ""
}

// MustLoad loads configuration and panics if it fails.
// Use this for application startup where configuration errors are fatal.
func MustLoad() *Config {
	cfg, err := Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: failed to load configuration\n\n%s\n\nSee .env.example for configuration options.\n", err)
		os.Exit(1)
	}
	return cfg
}

// LoadWithFlags loads configuration from environment variables,
// then applies command-line flag overrides.
func LoadWithFlags(port int, db, seed string) (*Config, error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}

	// Apply flag overrides (only if non-default values provided)
	if port != 0 && port != DefaultPort {
		cfg.Port = port
	}
	if db != "" && db != DefaultDBPath {
		cfg.DB = db
		cfg.DBPath = db
	}
	if seed != "" {
		cfg.Seed = seed
	}

	// Re-validate after applying overrides
	if errs := cfg.Validate(); len(errs) > 0 {
		return nil, errs
	}

	return cfg, nil
}
