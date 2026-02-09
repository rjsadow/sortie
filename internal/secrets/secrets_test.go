package secrets

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestEnvProvider_Get(t *testing.T) {
	// Set up test environment variables
	os.Setenv("SORTIE_SECRET_DB_PASSWORD", "test-password")
	os.Setenv("API_KEY", "test-api-key")
	defer func() {
		os.Unsetenv("SORTIE_SECRET_DB_PASSWORD")
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
	os.Setenv("SORTIE_SECRET_TEST_KEY", "test-value")
	defer os.Unsetenv("SORTIE_SECRET_TEST_KEY")

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
	os.Setenv("SORTIE_SECRET_KEY1", "value1")
	os.Setenv("SORTIE_SECRET_KEY2", "value2")
	defer func() {
		os.Unsetenv("SORTIE_SECRET_KEY1")
		os.Unsetenv("SORTIE_SECRET_KEY2")
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
		"SORTIE_SECRETS_PROVIDER",
		"SORTIE_VAULT_ADDR",
		"VAULT_ADDR",
		"SORTIE_AWS_REGION",
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
	os.Setenv("SORTIE_SECRETS_PROVIDER", "vault")
	os.Setenv("SORTIE_VAULT_ADDR", "http://vault:8200")

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
	os.Setenv("SORTIE_SECRET_EXISTING", "found-value")
	defer os.Unsetenv("SORTIE_SECRET_EXISTING")

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

func TestNewManager_UnknownProvider(t *testing.T) {
	cfg := &Config{Provider: "redis"}
	// Will fail at validation, not at provider creation
	_, err := NewManager(cfg)
	if err == nil {
		t.Error("NewManager() should fail with unknown provider")
	}
}

func TestManager_Get(t *testing.T) {
	os.Setenv("SORTIE_SECRET_MGR_TEST", "manager-value")
	defer os.Unsetenv("SORTIE_SECRET_MGR_TEST")

	mgr, err := NewManager(&Config{Provider: ProviderTypeEnv})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	ctx := context.Background()

	value, err := mgr.Get(ctx, "mgr_test")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if value != "manager-value" {
		t.Errorf("Get() = %v, want manager-value", value)
	}
}

func TestManager_GetNotFound(t *testing.T) {
	mgr, err := NewManager(&Config{Provider: ProviderTypeEnv})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	_, err = mgr.Get(context.Background(), "definitely_not_set_12345")
	if err != ErrSecretNotFound {
		t.Errorf("Get() error = %v, want ErrSecretNotFound", err)
	}
}

func TestManager_GetWithMetadata(t *testing.T) {
	os.Setenv("SORTIE_SECRET_META_TEST", "meta-value")
	defer os.Unsetenv("SORTIE_SECRET_META_TEST")

	mgr, err := NewManager(&Config{Provider: ProviderTypeEnv})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	secret, err := mgr.GetWithMetadata(context.Background(), "meta_test")
	if err != nil {
		t.Fatalf("GetWithMetadata() error = %v", err)
	}
	if secret.Value != "meta-value" {
		t.Errorf("Value = %v, want meta-value", secret.Value)
	}
}

func TestManager_List(t *testing.T) {
	os.Setenv("SORTIE_SECRET_LIST_A", "a")
	os.Setenv("SORTIE_SECRET_LIST_B", "b")
	defer func() {
		os.Unsetenv("SORTIE_SECRET_LIST_A")
		os.Unsetenv("SORTIE_SECRET_LIST_B")
	}()

	mgr, err := NewManager(&Config{Provider: ProviderTypeEnv})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	keys, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	found := make(map[string]bool)
	for _, k := range keys {
		found[k] = true
	}
	if !found["LIST_A"] {
		t.Error("List() should contain LIST_A")
	}
	if !found["LIST_B"] {
		t.Error("List() should contain LIST_B")
	}
}

func TestManager_MustGet_Success(t *testing.T) {
	os.Setenv("SORTIE_SECRET_MUST_GET", "required-value")
	defer os.Unsetenv("SORTIE_SECRET_MUST_GET")

	mgr, err := NewManager(&Config{Provider: ProviderTypeEnv})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	value := mgr.MustGet(context.Background(), "must_get")
	if value != "required-value" {
		t.Errorf("MustGet() = %v, want required-value", value)
	}
}

func TestManager_MustGet_Panics(t *testing.T) {
	mgr, err := NewManager(&Config{Provider: ProviderTypeEnv})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	defer func() {
		r := recover()
		if r == nil {
			t.Error("MustGet() should panic when secret not found")
		}
	}()

	mgr.MustGet(context.Background(), "definitely_not_set_panic_test")
}

func TestManager_Close_NilProvider(t *testing.T) {
	mgr := &Manager{provider: nil}
	if err := mgr.Close(); err != nil {
		t.Errorf("Close() with nil provider error = %v", err)
	}
}

func TestManager_Close(t *testing.T) {
	mgr, err := NewManager(&Config{Provider: ProviderTypeEnv})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if err := mgr.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Provider != ProviderTypeEnv {
		t.Errorf("Provider = %v, want env", cfg.Provider)
	}
	if cfg.VaultMountPath != "secret" {
		t.Errorf("VaultMountPath = %v, want secret", cfg.VaultMountPath)
	}
	if cfg.K8sNamespace != "default" {
		t.Errorf("K8sNamespace = %v, want default", cfg.K8sNamespace)
	}
	if !cfg.K8sInCluster {
		t.Error("K8sInCluster should default to true")
	}
}

func TestLoadConfig_VaultFallback(t *testing.T) {
	// Clear relevant env vars
	envVars := []string{
		"SORTIE_SECRETS_PROVIDER", "SORTIE_VAULT_ADDR", "VAULT_ADDR",
		"SORTIE_VAULT_TOKEN", "VAULT_TOKEN", "SORTIE_VAULT_MOUNT_PATH",
		"SORTIE_VAULT_NAMESPACE", "VAULT_NAMESPACE",
	}
	for _, v := range envVars {
		os.Unsetenv(v)
	}

	// Test VAULT_ADDR fallback
	os.Setenv("VAULT_ADDR", "http://vault-fallback:8200")
	defer os.Unsetenv("VAULT_ADDR")

	cfg := LoadConfig()
	if cfg.VaultAddr != "http://vault-fallback:8200" {
		t.Errorf("VaultAddr = %v, want http://vault-fallback:8200", cfg.VaultAddr)
	}
}

func TestLoadConfig_VaultToken(t *testing.T) {
	os.Unsetenv("SORTIE_VAULT_TOKEN")
	os.Unsetenv("VAULT_TOKEN")

	// Test SORTIE_VAULT_TOKEN
	os.Setenv("SORTIE_VAULT_TOKEN", "lp-token")
	defer os.Unsetenv("SORTIE_VAULT_TOKEN")

	cfg := LoadConfig()
	if cfg.VaultToken != "lp-token" {
		t.Errorf("VaultToken = %v, want lp-token", cfg.VaultToken)
	}
}

func TestLoadConfig_VaultTokenFallback(t *testing.T) {
	os.Unsetenv("SORTIE_VAULT_TOKEN")
	os.Setenv("VAULT_TOKEN", "fallback-token")
	defer os.Unsetenv("VAULT_TOKEN")

	cfg := LoadConfig()
	if cfg.VaultToken != "fallback-token" {
		t.Errorf("VaultToken = %v, want fallback-token", cfg.VaultToken)
	}
}

func TestLoadConfig_VaultMountPath(t *testing.T) {
	os.Setenv("SORTIE_VAULT_MOUNT_PATH", "custom/mount")
	defer os.Unsetenv("SORTIE_VAULT_MOUNT_PATH")

	cfg := LoadConfig()
	if cfg.VaultMountPath != "custom/mount" {
		t.Errorf("VaultMountPath = %v, want custom/mount", cfg.VaultMountPath)
	}
}

func TestLoadConfig_VaultNamespace(t *testing.T) {
	os.Unsetenv("SORTIE_VAULT_NAMESPACE")
	os.Unsetenv("VAULT_NAMESPACE")

	os.Setenv("SORTIE_VAULT_NAMESPACE", "my-namespace")
	defer os.Unsetenv("SORTIE_VAULT_NAMESPACE")

	cfg := LoadConfig()
	if cfg.VaultNamespace != "my-namespace" {
		t.Errorf("VaultNamespace = %v, want my-namespace", cfg.VaultNamespace)
	}
}

func TestLoadConfig_VaultNamespaceFallback(t *testing.T) {
	os.Unsetenv("SORTIE_VAULT_NAMESPACE")
	os.Setenv("VAULT_NAMESPACE", "fallback-ns")
	defer os.Unsetenv("VAULT_NAMESPACE")

	cfg := LoadConfig()
	if cfg.VaultNamespace != "fallback-ns" {
		t.Errorf("VaultNamespace = %v, want fallback-ns", cfg.VaultNamespace)
	}
}

func TestLoadConfig_AWSRegionFallbacks(t *testing.T) {
	os.Unsetenv("SORTIE_AWS_REGION")
	os.Unsetenv("AWS_REGION")
	os.Unsetenv("AWS_DEFAULT_REGION")

	// Test AWS_REGION fallback
	os.Setenv("AWS_REGION", "eu-west-1")
	defer os.Unsetenv("AWS_REGION")

	cfg := LoadConfig()
	if cfg.AWSRegion != "eu-west-1" {
		t.Errorf("AWSRegion = %v, want eu-west-1", cfg.AWSRegion)
	}

	// Test AWS_DEFAULT_REGION fallback
	os.Unsetenv("AWS_REGION")
	os.Setenv("AWS_DEFAULT_REGION", "ap-southeast-1")
	defer os.Unsetenv("AWS_DEFAULT_REGION")

	cfg = LoadConfig()
	if cfg.AWSRegion != "ap-southeast-1" {
		t.Errorf("AWSRegion = %v, want ap-southeast-1", cfg.AWSRegion)
	}
}

func TestLoadConfig_AWSSecretPrefix(t *testing.T) {
	os.Setenv("SORTIE_AWS_SECRET_PREFIX", "prod/sortie")
	defer os.Unsetenv("SORTIE_AWS_SECRET_PREFIX")

	cfg := LoadConfig()
	if cfg.AWSSecretPrefix != "prod/sortie" {
		t.Errorf("AWSSecretPrefix = %v, want prod/sortie", cfg.AWSSecretPrefix)
	}
}

func TestLoadConfig_K8sNamespace(t *testing.T) {
	os.Unsetenv("SORTIE_K8S_SECRET_NAMESPACE")
	os.Unsetenv("SORTIE_NAMESPACE")

	os.Setenv("SORTIE_K8S_SECRET_NAMESPACE", "k8s-ns")
	defer os.Unsetenv("SORTIE_K8S_SECRET_NAMESPACE")

	cfg := LoadConfig()
	if cfg.K8sNamespace != "k8s-ns" {
		t.Errorf("K8sNamespace = %v, want k8s-ns", cfg.K8sNamespace)
	}
}

func TestLoadConfig_K8sNamespaceFallback(t *testing.T) {
	os.Unsetenv("SORTIE_K8S_SECRET_NAMESPACE")
	os.Setenv("SORTIE_NAMESPACE", "fallback-ns")
	defer os.Unsetenv("SORTIE_NAMESPACE")

	cfg := LoadConfig()
	if cfg.K8sNamespace != "fallback-ns" {
		t.Errorf("K8sNamespace = %v, want fallback-ns", cfg.K8sNamespace)
	}
}

func TestLoadConfig_K8sSecretName(t *testing.T) {
	os.Setenv("SORTIE_K8S_SECRET_NAME", "my-secret")
	defer os.Unsetenv("SORTIE_K8S_SECRET_NAME")

	cfg := LoadConfig()
	if cfg.K8sSecretName != "my-secret" {
		t.Errorf("K8sSecretName = %v, want my-secret", cfg.K8sSecretName)
	}
}

func TestLoadConfig_Kubeconfig(t *testing.T) {
	os.Setenv("KUBECONFIG", "/tmp/kubeconfig")
	defer os.Unsetenv("KUBECONFIG")

	cfg := LoadConfig()
	if cfg.K8sKubeconfig != "/tmp/kubeconfig" {
		t.Errorf("K8sKubeconfig = %v, want /tmp/kubeconfig", cfg.K8sKubeconfig)
	}
	if cfg.K8sInCluster {
		t.Error("K8sInCluster should be false when KUBECONFIG is set")
	}
}

func TestEnvProvider_Name(t *testing.T) {
	p := NewEnvProvider()
	if got := p.Name(); got != "env" {
		t.Errorf("Name() = %v, want env", got)
	}
}

func TestEnvProvider_Close(t *testing.T) {
	p := NewEnvProvider()
	if err := p.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestNewEnvProviderWithPrefix(t *testing.T) {
	p := NewEnvProviderWithPrefix("CUSTOM_PREFIX_")

	os.Setenv("CUSTOM_PREFIX_MY_KEY", "custom-value")
	defer os.Unsetenv("CUSTOM_PREFIX_MY_KEY")

	value, err := p.Get(context.Background(), "my_key")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if value != "custom-value" {
		t.Errorf("Get() = %v, want custom-value", value)
	}
}

func TestEnvProvider_GetWithMetadata_NotFound(t *testing.T) {
	p := NewEnvProvider()
	_, err := p.GetWithMetadata(context.Background(), "definitely_not_set_meta_test")
	if err != ErrSecretNotFound {
		t.Errorf("GetWithMetadata() error = %v, want ErrSecretNotFound", err)
	}
}

func TestEnvProvider_ListEmpty(t *testing.T) {
	// Use a custom prefix that no env vars should match
	p := NewEnvProviderWithPrefix("XYZZY_TEST_PREFIX_")
	keys, err := p.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("List() returned %d keys, want 0", len(keys))
	}
}

func TestProviderInterface(t *testing.T) {
	// Verify that EnvProvider implements the Provider interface
	var _ Provider = (*EnvProvider)(nil)
	var _ Provider = (*VaultProvider)(nil)
	var _ Provider = (*AWSProvider)(nil)
	var _ Provider = (*KubernetesProvider)(nil)
}

func TestNewManager_EnvProvider(t *testing.T) {
	mgr, err := NewManager(&Config{Provider: ProviderTypeEnv})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	if mgr.ProviderName() != "env" {
		t.Errorf("ProviderName() = %v, want env", mgr.ProviderName())
	}
}

func TestLoadConfig_ProviderCaseInsensitive(t *testing.T) {
	os.Setenv("SORTIE_SECRETS_PROVIDER", "VAULT")
	defer os.Unsetenv("SORTIE_SECRETS_PROVIDER")

	cfg := LoadConfig()
	if cfg.Provider != ProviderType("vault") {
		t.Errorf("Provider = %v, want vault (lowercase)", cfg.Provider)
	}
}
