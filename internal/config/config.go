// Package config provides centralized configuration management for Launchpad.
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
	DB   string
	Seed string

	// Branding configuration
	BrandingConfigPath string
	LogoURL            string
	PrimaryColor       string
	SecondaryColor     string
	TenantName         string

	// Kubernetes configuration
	Namespace          string
	Kubeconfig         string
	VNCSidecarImage    string
	GuacdSidecarImage  string

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
	DefaultDBPath                 = "launchpad.db"
	DefaultBrandingConfigPath     = "branding.json"
	DefaultPrimaryColor           = "#1F2A3C"
	DefaultSecondaryColor         = "#2B3445"
	DefaultTenantName             = "Sortie"
	DefaultNamespace              = "default"
	DefaultVNCSidecarImage        = "ghcr.io/rjsadow/launchpad-vnc-sidecar:latest"
	DefaultGuacdSidecarImage      = "guacamole/guacd:1.5.5"
	DefaultSessionTimeout         = 2 * time.Hour
	DefaultSessionCleanupInterval = 5 * time.Minute
	DefaultPodReadyTimeout        = 2 * time.Minute
	DefaultJWTAccessExpiry        = 15 * time.Minute
	DefaultJWTRefreshExpiry       = 24 * time.Hour
	DefaultAdminUsername          = "admin"
)

// Load reads configuration from environment variables and returns a Config.
// It applies defaults for optional values and validates the configuration.
// Returns an error if validation fails.
func Load() (*Config, error) {
	cfg := &Config{
		// Server defaults
		Port: DefaultPort,
		DB:   DefaultDBPath,

		// Branding defaults
		BrandingConfigPath: DefaultBrandingConfigPath,
		PrimaryColor:       DefaultPrimaryColor,
		SecondaryColor:     DefaultSecondaryColor,
		TenantName:         DefaultTenantName,

		// Kubernetes defaults
		Namespace:         DefaultNamespace,
		VNCSidecarImage:   DefaultVNCSidecarImage,
		GuacdSidecarImage: DefaultGuacdSidecarImage,

		// Session defaults
		SessionTimeout:         DefaultSessionTimeout,
		SessionCleanupInterval: DefaultSessionCleanupInterval,
		PodReadyTimeout:        DefaultPodReadyTimeout,

		// JWT defaults
		JWTAccessExpiry:  DefaultJWTAccessExpiry,
		JWTRefreshExpiry: DefaultJWTRefreshExpiry,
		AdminUsername:    DefaultAdminUsername,
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
	if v := os.Getenv("LAUNCHPAD_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "LAUNCHPAD_PORT",
				Message: fmt.Sprintf("invalid port number: %q (must be an integer)", v),
			})
		} else {
			c.Port = port
		}
	}

	if v := os.Getenv("LAUNCHPAD_DB"); v != "" {
		c.DB = v
	}

	if v := os.Getenv("LAUNCHPAD_SEED"); v != "" {
		c.Seed = v
	}

	// Branding configuration
	if v := os.Getenv("LAUNCHPAD_CONFIG"); v != "" {
		c.BrandingConfigPath = v
	}

	if v := os.Getenv("LAUNCHPAD_LOGO_URL"); v != "" {
		c.LogoURL = v
	}

	if v := os.Getenv("LAUNCHPAD_PRIMARY_COLOR"); v != "" {
		c.PrimaryColor = v
	}

	if v := os.Getenv("LAUNCHPAD_SECONDARY_COLOR"); v != "" {
		c.SecondaryColor = v
	}

	if v := os.Getenv("LAUNCHPAD_TENANT_NAME"); v != "" {
		c.TenantName = v
	}

	// Kubernetes configuration
	if v := os.Getenv("LAUNCHPAD_NAMESPACE"); v != "" {
		c.Namespace = v
	}

	if v := os.Getenv("KUBECONFIG"); v != "" {
		c.Kubeconfig = v
	}

	if v := os.Getenv("LAUNCHPAD_VNC_SIDECAR_IMAGE"); v != "" {
		c.VNCSidecarImage = v
	}

	if v := os.Getenv("LAUNCHPAD_GUACD_SIDECAR_IMAGE"); v != "" {
		c.GuacdSidecarImage = v
	}

	// Session configuration
	if v := os.Getenv("LAUNCHPAD_SESSION_TIMEOUT"); v != "" {
		minutes, err := strconv.Atoi(v)
		if err != nil {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "LAUNCHPAD_SESSION_TIMEOUT",
				Message: fmt.Sprintf("invalid timeout: %q (must be an integer representing minutes)", v),
			})
		} else if minutes <= 0 {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "LAUNCHPAD_SESSION_TIMEOUT",
				Message: fmt.Sprintf("timeout must be positive: %d", minutes),
			})
		} else {
			c.SessionTimeout = time.Duration(minutes) * time.Minute
		}
	}

	if v := os.Getenv("LAUNCHPAD_SESSION_CLEANUP_INTERVAL"); v != "" {
		minutes, err := strconv.Atoi(v)
		if err != nil {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "LAUNCHPAD_SESSION_CLEANUP_INTERVAL",
				Message: fmt.Sprintf("invalid interval: %q (must be an integer representing minutes)", v),
			})
		} else if minutes <= 0 {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "LAUNCHPAD_SESSION_CLEANUP_INTERVAL",
				Message: fmt.Sprintf("interval must be positive: %d", minutes),
			})
		} else {
			c.SessionCleanupInterval = time.Duration(minutes) * time.Minute
		}
	}

	if v := os.Getenv("LAUNCHPAD_POD_READY_TIMEOUT"); v != "" {
		seconds, err := strconv.Atoi(v)
		if err != nil {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "LAUNCHPAD_POD_READY_TIMEOUT",
				Message: fmt.Sprintf("invalid timeout: %q (must be an integer representing seconds)", v),
			})
		} else if seconds <= 0 {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "LAUNCHPAD_POD_READY_TIMEOUT",
				Message: fmt.Sprintf("timeout must be positive: %d", seconds),
			})
		} else {
			c.PodReadyTimeout = time.Duration(seconds) * time.Second
		}
	}

	// JWT configuration
	if v := os.Getenv("LAUNCHPAD_JWT_SECRET"); v != "" {
		c.JWTSecret = v
	}

	if v := os.Getenv("LAUNCHPAD_JWT_ACCESS_EXPIRY"); v != "" {
		minutes, err := strconv.Atoi(v)
		if err != nil {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "LAUNCHPAD_JWT_ACCESS_EXPIRY",
				Message: fmt.Sprintf("invalid expiry: %q (must be an integer representing minutes)", v),
			})
		} else if minutes <= 0 {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "LAUNCHPAD_JWT_ACCESS_EXPIRY",
				Message: fmt.Sprintf("expiry must be positive: %d", minutes),
			})
		} else {
			c.JWTAccessExpiry = time.Duration(minutes) * time.Minute
		}
	}

	if v := os.Getenv("LAUNCHPAD_JWT_REFRESH_EXPIRY"); v != "" {
		hours, err := strconv.Atoi(v)
		if err != nil {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "LAUNCHPAD_JWT_REFRESH_EXPIRY",
				Message: fmt.Sprintf("invalid expiry: %q (must be an integer representing hours)", v),
			})
		} else if hours <= 0 {
			parseErrors = append(parseErrors, ValidationError{
				Field:   "LAUNCHPAD_JWT_REFRESH_EXPIRY",
				Message: fmt.Sprintf("expiry must be positive: %d", hours),
			})
		} else {
			c.JWTRefreshExpiry = time.Duration(hours) * time.Hour
		}
	}

	if v := os.Getenv("LAUNCHPAD_ADMIN_USERNAME"); v != "" {
		c.AdminUsername = v
	}

	if v := os.Getenv("LAUNCHPAD_ADMIN_PASSWORD"); v != "" {
		c.AdminPassword = v
	}

	if v := os.Getenv("LAUNCHPAD_ALLOW_REGISTRATION"); v != "" {
		c.AllowRegistration = strings.EqualFold(v, "true") || v == "1"
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
			Field:   "LAUNCHPAD_PORT",
			Message: fmt.Sprintf("port must be between 1 and 65535, got %d", c.Port),
		})
	}

	// Validate DB path is not empty
	if c.DB == "" {
		errs = append(errs, ValidationError{
			Field:   "LAUNCHPAD_DB",
			Message: "database path cannot be empty",
		})
	}

	// Validate color format (basic check for hex color)
	if c.PrimaryColor != "" && !isValidHexColor(c.PrimaryColor) {
		errs = append(errs, ValidationError{
			Field:   "LAUNCHPAD_PRIMARY_COLOR",
			Message: fmt.Sprintf("invalid hex color: %q (expected format: #RRGGBB)", c.PrimaryColor),
		})
	}

	if c.SecondaryColor != "" && !isValidHexColor(c.SecondaryColor) {
		errs = append(errs, ValidationError{
			Field:   "LAUNCHPAD_SECONDARY_COLOR",
			Message: fmt.Sprintf("invalid hex color: %q (expected format: #RRGGBB)", c.SecondaryColor),
		})
	}

	// Validate VNC sidecar image is not empty
	if c.VNCSidecarImage == "" {
		errs = append(errs, ValidationError{
			Field:   "LAUNCHPAD_VNC_SIDECAR_IMAGE",
			Message: "VNC sidecar image cannot be empty",
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
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
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
