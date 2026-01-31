package secrets

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestEnvProvider_Get(t *testing.T) {
	// Set up test environment variables
	os.Setenv("LAUNCHPAD_SECRET_DB_PASSWORD", "test-password")
	os.Setenv("API_KEY", "test-api-key")
	defer func() {
		os.Unsetenv("LAUNCHPAD_SECRET_DB_PASSWORD")
		os.Unsetenv("API_KEY")
	}()

	p := NewEnvProvider()
	ctx := context.Background()

	tests := []struct {
		name      string
		key       string
		want      string
		wantErr   error
	}{
		{
			name: "get prefixed secret",
			key:  "db_password",
			want: "test-password",
		},
		{
			name: "get raw key",
			key:  "API_KEY",
			want: "test-api-key",
		},
		{
			name:    "secret not found",
			key:     "nonexistent",
			wantErr: ErrSecretNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.Get(ctx, tt.key)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("Get() unexpected error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("Get() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnvProvider_GetWithMetadata(t *testing.T) {
	os.Setenv("LAUNCHPAD_SECRET_TEST_KEY", "test-value")
	defer os.Unsetenv("LAUNCHPAD_SECRET_TEST_KEY")

	p := NewEnvProvider()
	ctx := context.Background()

	secret, err := p.GetWithMetadata(ctx, "test_key")
	if err != nil {
		t.Fatalf("GetWithMetadata() error = %v", err)
	}

	if secret.Key != "test_key" {
		t.Errorf("Key = %v, want test_key", secret.Key)
	}
	if secret.Value != "test-value" {
		t.Errorf("Value = %v, want test-value", secret.Value)
	}
	if secret.Version != "env" {
		t.Errorf("Version = %v, want env", secret.Version)
	}
	if secret.Metadata["source"] != "environment" {
		t.Errorf("Metadata[source] = %v, want environment", secret.Metadata["source"])
	}
}

func TestEnvProvider_List(t *testing.T) {
	// Clear any existing secrets and set test ones
	os.Setenv("LAUNCHPAD_SECRET_KEY1", "value1")
	os.Setenv("LAUNCHPAD_SECRET_KEY2", "value2")
	defer func() {
		os.Unsetenv("LAUNCHPAD_SECRET_KEY1")
		os.Unsetenv("LAUNCHPAD_SECRET_KEY2")
	}()

	p := NewEnvProvider()
	ctx := context.Background()

	keys, err := p.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	// Should contain at least our two test keys
	found := make(map[string]bool)
	for _, k := range keys {
		found[k] = true
	}

	if !found["KEY1"] {
		t.Error("List() should contain KEY1")
	}
	if !found["KEY2"] {
		t.Error("List() should contain KEY2")
	}
}

func TestEnvProvider_Healthy(t *testing.T) {
	p := NewEnvProvider()
	ctx := context.Background()

	if !p.Healthy(ctx) {
		t.Error("Healthy() should always return true for env provider")
	}
}

func TestEnvProvider_NormalizeKey(t *testing.T) {
	p := NewEnvProvider()

	tests := []struct {
		input string
		want  string
	}{
		{"database.password", "DATABASE_PASSWORD"},
		{"api-key", "API_KEY"},
		{"path/to/secret", "PATH_TO_SECRET"},
		{"ALREADY_UPPER", "ALREADY_UPPER"},
		{"mixed.Case-key/path", "MIXED_CASE_KEY_PATH"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := p.normalizeKey(tt.input)
			if got != tt.want {
				t.Errorf("normalizeKey(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestConfig_LoadConfig(t *testing.T) {
	// Clear environment
	envVars := []string{
		"LAUNCHPAD_SECRETS_PROVIDER",
		"LAUNCHPAD_VAULT_ADDR",
		"VAULT_ADDR",
		"LAUNCHPAD_AWS_REGION",
		"AWS_REGION",
	}
	originalValues := make(map[string]string)
	for _, v := range envVars {
		originalValues[v] = os.Getenv(v)
		os.Unsetenv(v)
	}
	defer func() {
		for k, v := range originalValues {
			if v != "" {
				os.Setenv(k, v)
			}
		}
	}()

	// Test default config
	cfg := LoadConfig()
	if cfg.Provider != ProviderTypeEnv {
		t.Errorf("default Provider = %v, want %v", cfg.Provider, ProviderTypeEnv)
	}
	if cfg.VaultMountPath != "secret" {
		t.Errorf("default VaultMountPath = %v, want secret", cfg.VaultMountPath)
	}

	// Test with environment variables
	os.Setenv("LAUNCHPAD_SECRETS_PROVIDER", "vault")
	os.Setenv("LAUNCHPAD_VAULT_ADDR", "http://vault:8200")

	cfg = LoadConfig()
	if cfg.Provider != ProviderTypeVault {
		t.Errorf("Provider = %v, want vault", cfg.Provider)
	}
	if cfg.VaultAddr != "http://vault:8200" {
		t.Errorf("VaultAddr = %v, want http://vault:8200", cfg.VaultAddr)
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name:    "env provider - no validation needed",
			cfg:     &Config{Provider: ProviderTypeEnv},
			wantErr: false,
		},
		{
			name: "vault provider - valid",
			cfg: &Config{
				Provider:  ProviderTypeVault,
				VaultAddr: "http://vault:8200",
			},
			wantErr: false,
		},
		{
			name:    "vault provider - missing address",
			cfg:     &Config{Provider: ProviderTypeVault},
			wantErr: true,
		},
		{
			name: "aws provider - valid",
			cfg: &Config{
				Provider:  ProviderTypeAWS,
				AWSRegion: "us-east-1",
			},
			wantErr: false,
		},
		{
			name:    "aws provider - missing region",
			cfg:     &Config{Provider: ProviderTypeAWS},
			wantErr: true,
		},
		{
			name: "kubernetes provider - valid",
			cfg: &Config{
				Provider:      ProviderTypeKubernetes,
				K8sSecretName: "my-secrets",
			},
			wantErr: false,
		},
		{
			name:    "kubernetes provider - missing secret name",
			cfg:     &Config{Provider: ProviderTypeKubernetes},
			wantErr: true,
		},
		{
			name:    "unknown provider",
			cfg:     &Config{Provider: "unknown"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestManager_GetOrDefault(t *testing.T) {
	os.Setenv("LAUNCHPAD_SECRET_EXISTING", "found-value")
	defer os.Unsetenv("LAUNCHPAD_SECRET_EXISTING")

	cfg := &Config{Provider: ProviderTypeEnv}
	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	ctx := context.Background()

	// Existing secret
	if got := mgr.GetOrDefault(ctx, "existing", "default"); got != "found-value" {
		t.Errorf("GetOrDefault(existing) = %v, want found-value", got)
	}

	// Non-existing secret
	if got := mgr.GetOrDefault(ctx, "nonexistent", "default-value"); got != "default-value" {
		t.Errorf("GetOrDefault(nonexistent) = %v, want default-value", got)
	}
}

func TestManager_ProviderName(t *testing.T) {
	cfg := &Config{Provider: ProviderTypeEnv}
	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	if got := mgr.ProviderName(); got != "env" {
		t.Errorf("ProviderName() = %v, want env", got)
	}
}

func TestManager_Healthy(t *testing.T) {
	cfg := &Config{Provider: ProviderTypeEnv}
	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if !mgr.Healthy(ctx) {
		t.Error("Healthy() should return true for env provider")
	}
}

func TestNewManager_InvalidConfig(t *testing.T) {
	cfg := &Config{
		Provider: ProviderTypeVault,
		// Missing VaultAddr
	}

	_, err := NewManager(cfg)
	if err == nil {
		t.Error("NewManager() should fail with invalid config")
	}
}
