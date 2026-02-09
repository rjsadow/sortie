package secrets

import (
	"context"
	"os"
	"strings"
	"time"
)

// EnvProvider reads secrets from environment variables.
// This is the default provider and requires no external dependencies.
type EnvProvider struct {
	prefix string
}

// NewEnvProvider creates a new environment variable provider.
func NewEnvProvider() *EnvProvider {
	return &EnvProvider{
		prefix: "SORTIE_SECRET_",
	}
}

// NewEnvProviderWithPrefix creates a provider with a custom prefix.
func NewEnvProviderWithPrefix(prefix string) *EnvProvider {
	return &EnvProvider{
		prefix: prefix,
	}
}

// Name returns the provider name.
func (p *EnvProvider) Name() string {
	return "env"
}

// Get retrieves a secret from environment variables.
// It first tries with the configured prefix, then falls back to the raw key.
func (p *EnvProvider) Get(_ context.Context, key string) (string, error) {
	// Normalize key to environment variable format
	envKey := p.normalizeKey(key)

	// Try with prefix first
	if value := os.Getenv(p.prefix + envKey); value != "" {
		return value, nil
	}

	// Try raw key
	if value := os.Getenv(envKey); value != "" {
		return value, nil
	}

	// Try original key as-is
	if value := os.Getenv(key); value != "" {
		return value, nil
	}

	return "", ErrSecretNotFound
}

// GetWithMetadata retrieves a secret with basic metadata.
func (p *EnvProvider) GetWithMetadata(ctx context.Context, key string) (*Secret, error) {
	value, err := p.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	return &Secret{
		Key:       key,
		Value:     value,
		Version:   "env",
		CreatedAt: time.Time{}, // Unknown for env vars
		Metadata:  map[string]string{"source": "environment"},
	}, nil
}

// List returns environment variables that match the prefix.
func (p *EnvProvider) List(_ context.Context) ([]string, error) {
	var keys []string
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 && strings.HasPrefix(parts[0], p.prefix) {
			key := strings.TrimPrefix(parts[0], p.prefix)
			keys = append(keys, key)
		}
	}
	return keys, nil
}

// Close is a no-op for environment provider.
func (p *EnvProvider) Close() error {
	return nil
}

// Healthy always returns true for environment provider.
func (p *EnvProvider) Healthy(_ context.Context) bool {
	return true
}

// normalizeKey converts a key to environment variable format.
// e.g., "database.password" -> "DATABASE_PASSWORD"
func (p *EnvProvider) normalizeKey(key string) string {
	key = strings.ToUpper(key)
	key = strings.ReplaceAll(key, ".", "_")
	key = strings.ReplaceAll(key, "-", "_")
	key = strings.ReplaceAll(key, "/", "_")
	return key
}
