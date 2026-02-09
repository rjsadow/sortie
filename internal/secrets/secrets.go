// Package secrets provides centralized secrets management for Sortie.
// It supports multiple external secret stores with a unified interface.
//
// Supported providers:
//   - HashiCorp Vault (vault)
//   - AWS Secrets Manager (aws)
//   - Kubernetes Secrets (kubernetes)
//   - Environment variables (env) - default fallback
//
// The package follows clear boundaries:
//   - Provider interface defines the contract for all secret stores
//   - Each provider is responsible for its own authentication
//   - Secrets are fetched on-demand, not cached by default
//   - Configuration errors fail fast at startup
package secrets

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Provider defines the interface for secret store backends.
// All providers must implement this interface to be usable with Sortie.
type Provider interface {
	// Name returns the provider name for logging and debugging.
	Name() string

	// Get retrieves a secret by key.
	// Returns ErrSecretNotFound if the secret doesn't exist.
	Get(ctx context.Context, key string) (string, error)

	// GetWithMetadata retrieves a secret along with metadata.
	// Useful for checking versions, expiry, etc.
	GetWithMetadata(ctx context.Context, key string) (*Secret, error)

	// List returns all available secret keys (if supported).
	// Returns ErrNotSupported if the provider doesn't support listing.
	List(ctx context.Context) ([]string, error)

	// Close releases any resources held by the provider.
	Close() error

	// Healthy returns true if the provider is accessible.
	Healthy(ctx context.Context) bool
}

// Secret represents a secret value with optional metadata.
type Secret struct {
	Key       string
	Value     string
	Version   string
	CreatedAt time.Time
	ExpiresAt *time.Time
	Metadata  map[string]string
}

// Common errors returned by providers.
var (
	ErrSecretNotFound = errors.New("secret not found")
	ErrNotSupported   = errors.New("operation not supported by this provider")
	ErrNotConfigured  = errors.New("provider not configured")
	ErrAuthFailed     = errors.New("authentication failed")
	ErrTimeout        = errors.New("operation timed out")
)

// ProviderType represents the type of secret provider.
type ProviderType string

const (
	ProviderTypeEnv        ProviderType = "env"
	ProviderTypeVault      ProviderType = "vault"
	ProviderTypeAWS        ProviderType = "aws"
	ProviderTypeKubernetes ProviderType = "kubernetes"
)

// Config holds the configuration for secrets management.
type Config struct {
	// Provider specifies which secret store to use.
	// Valid values: env, vault, aws, kubernetes
	Provider ProviderType

	// Vault configuration
	VaultAddr      string
	VaultToken     string
	VaultMountPath string
	VaultNamespace string

	// AWS configuration
	AWSRegion       string
	AWSSecretPrefix string

	// Kubernetes configuration
	K8sNamespace   string
	K8sSecretName  string
	K8sKubeconfig  string
	K8sInCluster   bool
}

// DefaultConfig returns the default secrets configuration.
func DefaultConfig() *Config {
	return &Config{
		Provider:       ProviderTypeEnv,
		VaultMountPath: "secret",
		K8sNamespace:   "default",
		K8sInCluster:   true,
	}
}

// LoadConfig loads secrets configuration from environment variables.
func LoadConfig() *Config {
	cfg := DefaultConfig()

	// Provider selection
	if v := os.Getenv("SORTIE_SECRETS_PROVIDER"); v != "" {
		cfg.Provider = ProviderType(strings.ToLower(v))
	}

	// Vault configuration
	if v := os.Getenv("SORTIE_VAULT_ADDR"); v != "" {
		cfg.VaultAddr = v
	} else if v := os.Getenv("VAULT_ADDR"); v != "" {
		cfg.VaultAddr = v
	}

	if v := os.Getenv("SORTIE_VAULT_TOKEN"); v != "" {
		cfg.VaultToken = v
	} else if v := os.Getenv("VAULT_TOKEN"); v != "" {
		cfg.VaultToken = v
	}

	if v := os.Getenv("SORTIE_VAULT_MOUNT_PATH"); v != "" {
		cfg.VaultMountPath = v
	}

	if v := os.Getenv("SORTIE_VAULT_NAMESPACE"); v != "" {
		cfg.VaultNamespace = v
	} else if v := os.Getenv("VAULT_NAMESPACE"); v != "" {
		cfg.VaultNamespace = v
	}

	// AWS configuration
	if v := os.Getenv("SORTIE_AWS_REGION"); v != "" {
		cfg.AWSRegion = v
	} else if v := os.Getenv("AWS_REGION"); v != "" {
		cfg.AWSRegion = v
	} else if v := os.Getenv("AWS_DEFAULT_REGION"); v != "" {
		cfg.AWSRegion = v
	}

	if v := os.Getenv("SORTIE_AWS_SECRET_PREFIX"); v != "" {
		cfg.AWSSecretPrefix = v
	}

	// Kubernetes configuration
	if v := os.Getenv("SORTIE_K8S_SECRET_NAMESPACE"); v != "" {
		cfg.K8sNamespace = v
	} else if v := os.Getenv("SORTIE_NAMESPACE"); v != "" {
		cfg.K8sNamespace = v
	}

	if v := os.Getenv("SORTIE_K8S_SECRET_NAME"); v != "" {
		cfg.K8sSecretName = v
	}

	if v := os.Getenv("KUBECONFIG"); v != "" {
		cfg.K8sKubeconfig = v
		cfg.K8sInCluster = false
	}

	return cfg
}

// Validate checks that the configuration is valid for the selected provider.
func (c *Config) Validate() error {
	switch c.Provider {
	case ProviderTypeEnv:
		// No validation needed for env provider
		return nil

	case ProviderTypeVault:
		if c.VaultAddr == "" {
			return fmt.Errorf("SORTIE_VAULT_ADDR or VAULT_ADDR is required for vault provider")
		}
		// Token can be obtained via other auth methods, so not strictly required

	case ProviderTypeAWS:
		if c.AWSRegion == "" {
			return fmt.Errorf("SORTIE_AWS_REGION or AWS_REGION is required for aws provider")
		}
		// AWS credentials are obtained from environment, instance profile, etc.

	case ProviderTypeKubernetes:
		if c.K8sSecretName == "" {
			return fmt.Errorf("SORTIE_K8S_SECRET_NAME is required for kubernetes provider")
		}

	default:
		return fmt.Errorf("unknown provider type: %q (valid: env, vault, aws, kubernetes)", c.Provider)
	}

	return nil
}

// Manager provides access to secrets through the configured provider.
type Manager struct {
	provider Provider
	config   *Config
}

// NewManager creates a new secrets manager with the given configuration.
func NewManager(cfg *Config) (*Manager, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid secrets configuration: %w", err)
	}

	var provider Provider
	var err error

	switch cfg.Provider {
	case ProviderTypeEnv:
		provider = NewEnvProvider()
	case ProviderTypeVault:
		provider, err = NewVaultProvider(cfg)
	case ProviderTypeAWS:
		provider, err = NewAWSProvider(cfg)
	case ProviderTypeKubernetes:
		provider, err = NewKubernetesProvider(cfg)
	default:
		return nil, fmt.Errorf("unknown provider: %s", cfg.Provider)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to initialize %s provider: %w", cfg.Provider, err)
	}

	return &Manager{
		provider: provider,
		config:   cfg,
	}, nil
}

// Get retrieves a secret by key.
func (m *Manager) Get(ctx context.Context, key string) (string, error) {
	return m.provider.Get(ctx, key)
}

// GetWithMetadata retrieves a secret with metadata.
func (m *Manager) GetWithMetadata(ctx context.Context, key string) (*Secret, error) {
	return m.provider.GetWithMetadata(ctx, key)
}

// GetOrDefault retrieves a secret or returns the default value if not found.
func (m *Manager) GetOrDefault(ctx context.Context, key, defaultValue string) string {
	value, err := m.provider.Get(ctx, key)
	if err != nil {
		return defaultValue
	}
	return value
}

// MustGet retrieves a secret or panics if not found.
// Use only for required secrets during startup.
func (m *Manager) MustGet(ctx context.Context, key string) string {
	value, err := m.provider.Get(ctx, key)
	if err != nil {
		panic(fmt.Sprintf("required secret %q not found: %v", key, err))
	}
	return value
}

// List returns all available secret keys.
func (m *Manager) List(ctx context.Context) ([]string, error) {
	return m.provider.List(ctx)
}

// Healthy returns true if the secrets provider is accessible.
func (m *Manager) Healthy(ctx context.Context) bool {
	return m.provider.Healthy(ctx)
}

// ProviderName returns the name of the active provider.
func (m *Manager) ProviderName() string {
	return m.provider.Name()
}

// Close releases resources held by the manager.
func (m *Manager) Close() error {
	if m.provider != nil {
		return m.provider.Close()
	}
	return nil
}
