package plugins

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// mockPlugin is a test plugin implementation
type mockPlugin struct {
	name        string
	pluginType  PluginType
	healthy     bool
	initialized bool
	closed      bool
	initErr     error
	closeErr    error
	initConfig  map[string]string
}

func (p *mockPlugin) Name() string           { return p.name }
func (p *mockPlugin) Type() PluginType       { return p.pluginType }
func (p *mockPlugin) Version() string        { return "1.0.0" }
func (p *mockPlugin) Description() string    { return "Mock plugin for testing" }
func (p *mockPlugin) Healthy(ctx context.Context) bool { return p.healthy }
func (p *mockPlugin) Close() error           { p.closed = true; return p.closeErr }
func (p *mockPlugin) Initialize(ctx context.Context, config map[string]string) error {
	p.initialized = true
	p.initConfig = config
	return p.initErr
}

// mockLauncher implements LauncherPlugin
type mockLauncher struct {
	mockPlugin
}

func (l *mockLauncher) SupportedTypes() []LaunchType { return []LaunchType{LaunchTypeURL} }
func (l *mockLauncher) Launch(ctx context.Context, req *LaunchRequest) (*LaunchResult, error) {
	return &LaunchResult{SessionID: "test-session", Status: LaunchStatusRunning}, nil
}
func (l *mockLauncher) GetStatus(ctx context.Context, sessionID string) (*LaunchResult, error) {
	return &LaunchResult{SessionID: sessionID, Status: LaunchStatusRunning}, nil
}
func (l *mockLauncher) Terminate(ctx context.Context, sessionID string) error { return nil }
func (l *mockLauncher) ListSessions(ctx context.Context, userID string) ([]*LaunchResult, error) {
	return nil, nil
}

// mockAuth implements AuthProvider
type mockAuth struct {
	mockPlugin
}

func (a *mockAuth) Authenticate(ctx context.Context, token string) (*AuthResult, error) {
	return &AuthResult{Authenticated: true}, nil
}
func (a *mockAuth) GetUser(ctx context.Context, userID string) (*User, error) {
	return &User{ID: userID}, nil
}
func (a *mockAuth) HasPermission(ctx context.Context, userID, permission string) (bool, error) {
	return true, nil
}
func (a *mockAuth) GetLoginURL(redirectURL string) string { return redirectURL }
func (a *mockAuth) HandleCallback(ctx context.Context, code, state string) (*AuthResult, error) {
	return &AuthResult{Authenticated: true}, nil
}
func (a *mockAuth) Logout(ctx context.Context, token string) error { return nil }

// mockStorage implements StorageProvider
type mockStorage struct {
	mockPlugin
}

func (s *mockStorage) CreateApp(ctx context.Context, app *Application) error { return nil }
func (s *mockStorage) GetApp(ctx context.Context, id string) (*Application, error) { return nil, nil }
func (s *mockStorage) UpdateApp(ctx context.Context, app *Application) error { return nil }
func (s *mockStorage) DeleteApp(ctx context.Context, id string) error { return nil }
func (s *mockStorage) ListApps(ctx context.Context) ([]*Application, error) { return nil, nil }
func (s *mockStorage) CreateSession(ctx context.Context, session *Session) error { return nil }
func (s *mockStorage) GetSession(ctx context.Context, id string) (*Session, error) { return nil, nil }
func (s *mockStorage) UpdateSession(ctx context.Context, session *Session) error { return nil }
func (s *mockStorage) DeleteSession(ctx context.Context, id string) error { return nil }
func (s *mockStorage) ListSessions(ctx context.Context, userID string) ([]*Session, error) {
	return nil, nil
}
func (s *mockStorage) ListExpiredSessions(ctx context.Context) ([]*Session, error) { return nil, nil }
func (s *mockStorage) LogAudit(ctx context.Context, entry *AuditEntry) error { return nil }
func (s *mockStorage) GetAuditLogs(ctx context.Context, limit int) ([]*AuditEntry, error) {
	return nil, nil
}
func (s *mockStorage) RecordLaunch(ctx context.Context, appID string) error { return nil }
func (s *mockStorage) GetAnalyticsStats(ctx context.Context) (map[string]any, error) {
	return nil, nil
}

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
	if r.factories == nil {
		t.Fatal("factories map is nil")
	}
}

func TestRegisterPlugin(t *testing.T) {
	r := NewRegistry()

	// Register a launcher
	err := r.Register(PluginTypeLauncher, "test", func() Plugin {
		return &mockLauncher{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeLauncher, healthy: true}}
	})
	if err != nil {
		t.Fatalf("failed to register launcher: %v", err)
	}

	// Try to register again - should fail
	err = r.Register(PluginTypeLauncher, "test", func() Plugin {
		return &mockLauncher{}
	})
	if err == nil {
		t.Fatal("expected error when registering duplicate plugin")
	}
}

func TestInitialize(t *testing.T) {
	r := NewRegistry()

	// Register all required plugins
	r.Register(PluginTypeLauncher, "test", func() Plugin {
		return &mockLauncher{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeLauncher, healthy: true}}
	})
	r.Register(PluginTypeAuth, "test", func() Plugin {
		return &mockAuth{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeAuth, healthy: true}}
	})
	r.Register(PluginTypeStorage, "test", func() Plugin {
		return &mockStorage{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeStorage, healthy: true}}
	})

	cfg := &RegistryConfig{
		Launcher:      "test",
		Auth:          "test",
		Storage:       "test",
		PluginConfigs: make(map[string]map[string]string),
	}

	err := r.Initialize(context.Background(), cfg)
	if err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}

	// Check plugins are available
	if r.Launcher() == nil {
		t.Fatal("launcher is nil after initialize")
	}
	if r.Auth() == nil {
		t.Fatal("auth is nil after initialize")
	}
	if r.Storage() == nil {
		t.Fatal("storage is nil after initialize")
	}
}

func TestHealthCheck(t *testing.T) {
	r := NewRegistry()

	r.Register(PluginTypeLauncher, "test", func() Plugin {
		return &mockLauncher{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeLauncher, healthy: true}}
	})
	r.Register(PluginTypeAuth, "test", func() Plugin {
		return &mockAuth{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeAuth, healthy: true}}
	})
	r.Register(PluginTypeStorage, "test", func() Plugin {
		return &mockStorage{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeStorage, healthy: true}}
	})

	cfg := &RegistryConfig{
		Launcher:      "test",
		Auth:          "test",
		Storage:       "test",
		PluginConfigs: make(map[string]map[string]string),
	}

	r.Initialize(context.Background(), cfg)

	statuses := r.HealthCheck(context.Background())
	if len(statuses) != 3 {
		t.Fatalf("expected 3 health statuses, got %d", len(statuses))
	}

	for _, status := range statuses {
		if !status.Healthy {
			t.Errorf("plugin %s is unhealthy", status.PluginName)
		}
	}
}

func TestListPlugins(t *testing.T) {
	r := NewRegistry()

	r.Register(PluginTypeLauncher, "launcher1", func() Plugin {
		return &mockLauncher{mockPlugin: mockPlugin{name: "launcher1", pluginType: PluginTypeLauncher}}
	})
	r.Register(PluginTypeLauncher, "launcher2", func() Plugin {
		return &mockLauncher{mockPlugin: mockPlugin{name: "launcher2", pluginType: PluginTypeLauncher}}
	})
	r.Register(PluginTypeAuth, "auth1", func() Plugin {
		return &mockAuth{mockPlugin: mockPlugin{name: "auth1", pluginType: PluginTypeAuth}}
	})

	plugins := r.ListPlugins(context.Background())
	if len(plugins) != 3 {
		t.Fatalf("expected 3 plugins, got %d", len(plugins))
	}

	launchers := r.ListPluginsByType(PluginTypeLauncher)
	if len(launchers) != 2 {
		t.Fatalf("expected 2 launchers, got %d", len(launchers))
	}
}

func TestClose(t *testing.T) {
	r := NewRegistry()

	launcher := &mockLauncher{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeLauncher, healthy: true}}
	auth := &mockAuth{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeAuth, healthy: true}}
	storage := &mockStorage{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeStorage, healthy: true}}

	r.Register(PluginTypeLauncher, "test", func() Plugin { return launcher })
	r.Register(PluginTypeAuth, "test", func() Plugin { return auth })
	r.Register(PluginTypeStorage, "test", func() Plugin { return storage })

	cfg := &RegistryConfig{
		Launcher:      "test",
		Auth:          "test",
		Storage:       "test",
		PluginConfigs: make(map[string]map[string]string),
	}

	r.Initialize(context.Background(), cfg)
	r.Close()

	// Verify Close was called on active plugins
	if !r.activeLauncher.(*mockLauncher).closed {
		t.Error("launcher was not closed")
	}
	if !r.activeAuth.(*mockAuth).closed {
		t.Error("auth was not closed")
	}
	if !r.activeStorage.(*mockStorage).closed {
		t.Error("storage was not closed")
	}
}

func TestDefaultRegistryConfig(t *testing.T) {
	cfg := DefaultRegistryConfig()

	if cfg.Launcher != "url" {
		t.Errorf("expected default launcher 'url', got '%s'", cfg.Launcher)
	}
	if cfg.Auth != "noop" {
		t.Errorf("expected default auth 'noop', got '%s'", cfg.Auth)
	}
	if cfg.Storage != "sqlite" {
		t.Errorf("expected default storage 'sqlite', got '%s'", cfg.Storage)
	}
	if cfg.PluginConfigs == nil {
		t.Error("expected PluginConfigs to be initialized")
	}
}

func TestRegisterUnknownPluginType(t *testing.T) {
	r := NewRegistry()
	err := r.Register(PluginType("unknown"), "test", func() Plugin {
		return &mockPlugin{name: "test"}
	})
	if err == nil {
		t.Fatal("expected error when registering unknown plugin type")
	}
	if !strings.Contains(err.Error(), "unknown plugin type") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRegisterAllPluginTypes(t *testing.T) {
	r := NewRegistry()

	types := []struct {
		pluginType PluginType
		factory    PluginFactory
	}{
		{PluginTypeLauncher, func() Plugin {
			return &mockLauncher{mockPlugin: mockPlugin{name: "l", pluginType: PluginTypeLauncher}}
		}},
		{PluginTypeAuth, func() Plugin {
			return &mockAuth{mockPlugin: mockPlugin{name: "a", pluginType: PluginTypeAuth}}
		}},
		{PluginTypeStorage, func() Plugin {
			return &mockStorage{mockPlugin: mockPlugin{name: "s", pluginType: PluginTypeStorage}}
		}},
	}

	for _, tt := range types {
		err := r.Register(tt.pluginType, "test", tt.factory)
		if err != nil {
			t.Errorf("failed to register %s plugin: %v", tt.pluginType, err)
		}
	}
}

func TestLoadRegistryConfig(t *testing.T) {
	// Test with no env vars set - should return defaults
	t.Setenv("SORTIE_PLUGIN_LAUNCHER", "")
	t.Setenv("SORTIE_PLUGIN_AUTH", "")
	t.Setenv("SORTIE_PLUGIN_STORAGE", "")

	cfg := LoadRegistryConfig()
	if cfg.Launcher != "url" {
		t.Errorf("expected default launcher 'url', got '%s'", cfg.Launcher)
	}
	if cfg.Auth != "noop" {
		t.Errorf("expected default auth 'noop', got '%s'", cfg.Auth)
	}
	if cfg.Storage != "sqlite" {
		t.Errorf("expected default storage 'sqlite', got '%s'", cfg.Storage)
	}
}

func TestLoadRegistryConfigWithEnvVars(t *testing.T) {
	t.Setenv("SORTIE_PLUGIN_LAUNCHER", "Container")
	t.Setenv("SORTIE_PLUGIN_AUTH", "OIDC")
	t.Setenv("SORTIE_PLUGIN_STORAGE", "Postgres")

	cfg := LoadRegistryConfig()
	if cfg.Launcher != "container" {
		t.Errorf("expected launcher 'container', got '%s'", cfg.Launcher)
	}
	if cfg.Auth != "oidc" {
		t.Errorf("expected auth 'oidc', got '%s'", cfg.Auth)
	}
	if cfg.Storage != "postgres" {
		t.Errorf("expected storage 'postgres', got '%s'", cfg.Storage)
	}
}

func TestInitializeMissingLauncher(t *testing.T) {
	r := NewRegistry()
	r.Register(PluginTypeAuth, "test", func() Plugin {
		return &mockAuth{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeAuth}}
	})
	r.Register(PluginTypeStorage, "test", func() Plugin {
		return &mockStorage{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeStorage}}
	})

	cfg := &RegistryConfig{
		Launcher:      "nonexistent",
		Auth:          "test",
		Storage:       "test",
		PluginConfigs: make(map[string]map[string]string),
	}

	err := r.Initialize(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for missing launcher plugin")
	}
	if !strings.Contains(err.Error(), "launcher") {
		t.Errorf("error should mention launcher: %v", err)
	}
}

func TestInitializeMissingAuth(t *testing.T) {
	r := NewRegistry()
	r.Register(PluginTypeLauncher, "test", func() Plugin {
		return &mockLauncher{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeLauncher}}
	})
	r.Register(PluginTypeStorage, "test", func() Plugin {
		return &mockStorage{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeStorage}}
	})

	cfg := &RegistryConfig{
		Launcher:      "test",
		Auth:          "nonexistent",
		Storage:       "test",
		PluginConfigs: make(map[string]map[string]string),
	}

	err := r.Initialize(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for missing auth plugin")
	}
	if !strings.Contains(err.Error(), "auth") {
		t.Errorf("error should mention auth: %v", err)
	}
}

func TestInitializeMissingStorage(t *testing.T) {
	r := NewRegistry()
	r.Register(PluginTypeLauncher, "test", func() Plugin {
		return &mockLauncher{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeLauncher}}
	})
	r.Register(PluginTypeAuth, "test", func() Plugin {
		return &mockAuth{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeAuth}}
	})

	cfg := &RegistryConfig{
		Launcher:      "test",
		Auth:          "test",
		Storage:       "nonexistent",
		PluginConfigs: make(map[string]map[string]string),
	}

	err := r.Initialize(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for missing storage plugin")
	}
	if !strings.Contains(err.Error(), "storage") {
		t.Errorf("error should mention storage: %v", err)
	}
}

func TestInitializeLauncherInitError(t *testing.T) {
	r := NewRegistry()
	r.Register(PluginTypeLauncher, "bad", func() Plugin {
		return &mockLauncher{mockPlugin: mockPlugin{
			name: "bad", pluginType: PluginTypeLauncher, initErr: errors.New("init boom"),
		}}
	})
	r.Register(PluginTypeAuth, "test", func() Plugin {
		return &mockAuth{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeAuth}}
	})
	r.Register(PluginTypeStorage, "test", func() Plugin {
		return &mockStorage{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeStorage}}
	})

	cfg := &RegistryConfig{
		Launcher:      "bad",
		Auth:          "test",
		Storage:       "test",
		PluginConfigs: make(map[string]map[string]string),
	}

	err := r.Initialize(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when launcher init fails")
	}
	if !strings.Contains(err.Error(), "init boom") {
		t.Errorf("error should contain init error: %v", err)
	}
}

func TestInitializeAuthInitError(t *testing.T) {
	r := NewRegistry()
	r.Register(PluginTypeLauncher, "test", func() Plugin {
		return &mockLauncher{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeLauncher}}
	})
	r.Register(PluginTypeAuth, "bad", func() Plugin {
		return &mockAuth{mockPlugin: mockPlugin{
			name: "bad", pluginType: PluginTypeAuth, initErr: errors.New("auth init fail"),
		}}
	})
	r.Register(PluginTypeStorage, "test", func() Plugin {
		return &mockStorage{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeStorage}}
	})

	cfg := &RegistryConfig{
		Launcher:      "test",
		Auth:          "bad",
		Storage:       "test",
		PluginConfigs: make(map[string]map[string]string),
	}

	err := r.Initialize(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when auth init fails")
	}
	if !strings.Contains(err.Error(), "auth init fail") {
		t.Errorf("error should contain init error: %v", err)
	}
}

func TestInitializeStorageInitError(t *testing.T) {
	r := NewRegistry()
	r.Register(PluginTypeLauncher, "test", func() Plugin {
		return &mockLauncher{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeLauncher}}
	})
	r.Register(PluginTypeAuth, "test", func() Plugin {
		return &mockAuth{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeAuth}}
	})
	r.Register(PluginTypeStorage, "bad", func() Plugin {
		return &mockStorage{mockPlugin: mockPlugin{
			name: "bad", pluginType: PluginTypeStorage, initErr: errors.New("storage init fail"),
		}}
	})

	cfg := &RegistryConfig{
		Launcher:      "test",
		Auth:          "test",
		Storage:       "bad",
		PluginConfigs: make(map[string]map[string]string),
	}

	err := r.Initialize(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when storage init fails")
	}
	if !strings.Contains(err.Error(), "storage init fail") {
		t.Errorf("error should contain init error: %v", err)
	}
}

func TestInitializeWithPluginConfig(t *testing.T) {
	var capturedLauncher *mockLauncher
	var capturedAuth *mockAuth
	var capturedStorage *mockStorage

	r := NewRegistry()
	r.Register(PluginTypeLauncher, "test", func() Plugin {
		capturedLauncher = &mockLauncher{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeLauncher}}
		return capturedLauncher
	})
	r.Register(PluginTypeAuth, "test", func() Plugin {
		capturedAuth = &mockAuth{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeAuth}}
		return capturedAuth
	})
	r.Register(PluginTypeStorage, "test", func() Plugin {
		capturedStorage = &mockStorage{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeStorage}}
		return capturedStorage
	})

	cfg := &RegistryConfig{
		Launcher: "test",
		Auth:     "test",
		Storage:  "test",
		PluginConfigs: map[string]map[string]string{
			"launcher.test": {"base_url": "http://example.com"},
			"auth.test":     {"provider": "oidc"},
			"storage.test":  {"dsn": "sqlite://test.db"},
		},
	}

	err := r.Initialize(context.Background(), cfg)
	if err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}

	if capturedLauncher.initConfig["base_url"] != "http://example.com" {
		t.Error("launcher did not receive its config")
	}
	if capturedAuth.initConfig["provider"] != "oidc" {
		t.Error("auth did not receive its config")
	}
	if capturedStorage.initConfig["dsn"] != "sqlite://test.db" {
		t.Error("storage did not receive its config")
	}
}

func TestInitializeWithNilPluginConfig(t *testing.T) {
	var capturedLauncher *mockLauncher

	r := NewRegistry()
	r.Register(PluginTypeLauncher, "test", func() Plugin {
		capturedLauncher = &mockLauncher{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeLauncher}}
		return capturedLauncher
	})
	r.Register(PluginTypeAuth, "test", func() Plugin {
		return &mockAuth{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeAuth}}
	})
	r.Register(PluginTypeStorage, "test", func() Plugin {
		return &mockStorage{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeStorage}}
	})

	cfg := &RegistryConfig{
		Launcher:      "test",
		Auth:          "test",
		Storage:       "test",
		PluginConfigs: make(map[string]map[string]string), // no config for any plugin
	}

	err := r.Initialize(context.Background(), cfg)
	if err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}

	// Plugin should receive an empty (non-nil) config map
	if capturedLauncher.initConfig == nil {
		t.Error("expected non-nil config map when no plugin config provided")
	}
}

func TestInitializeWrongInterfaceLauncher(t *testing.T) {
	r := NewRegistry()
	// Register a plain mockPlugin that does NOT implement LauncherPlugin
	r.Register(PluginTypeLauncher, "bad", func() Plugin {
		return &mockPlugin{name: "bad", pluginType: PluginTypeLauncher}
	})
	r.Register(PluginTypeAuth, "test", func() Plugin {
		return &mockAuth{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeAuth}}
	})
	r.Register(PluginTypeStorage, "test", func() Plugin {
		return &mockStorage{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeStorage}}
	})

	cfg := &RegistryConfig{
		Launcher:      "bad",
		Auth:          "test",
		Storage:       "test",
		PluginConfigs: make(map[string]map[string]string),
	}

	err := r.Initialize(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when plugin doesn't implement LauncherPlugin")
	}
	if !strings.Contains(err.Error(), "does not implement LauncherPlugin") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInitializeWrongInterfaceAuth(t *testing.T) {
	r := NewRegistry()
	r.Register(PluginTypeLauncher, "test", func() Plugin {
		return &mockLauncher{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeLauncher}}
	})
	// Register a plain mockPlugin that does NOT implement AuthProvider
	r.Register(PluginTypeAuth, "bad", func() Plugin {
		return &mockPlugin{name: "bad", pluginType: PluginTypeAuth}
	})
	r.Register(PluginTypeStorage, "test", func() Plugin {
		return &mockStorage{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeStorage}}
	})

	cfg := &RegistryConfig{
		Launcher:      "test",
		Auth:          "bad",
		Storage:       "test",
		PluginConfigs: make(map[string]map[string]string),
	}

	err := r.Initialize(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when plugin doesn't implement AuthProvider")
	}
	if !strings.Contains(err.Error(), "does not implement AuthProvider") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInitializeWrongInterfaceStorage(t *testing.T) {
	r := NewRegistry()
	r.Register(PluginTypeLauncher, "test", func() Plugin {
		return &mockLauncher{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeLauncher}}
	})
	r.Register(PluginTypeAuth, "test", func() Plugin {
		return &mockAuth{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeAuth}}
	})
	// Register a plain mockPlugin that does NOT implement StorageProvider
	r.Register(PluginTypeStorage, "bad", func() Plugin {
		return &mockPlugin{name: "bad", pluginType: PluginTypeStorage}
	})

	cfg := &RegistryConfig{
		Launcher:      "test",
		Auth:          "test",
		Storage:       "bad",
		PluginConfigs: make(map[string]map[string]string),
	}

	err := r.Initialize(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when plugin doesn't implement StorageProvider")
	}
	if !strings.Contains(err.Error(), "does not implement StorageProvider") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHealthCheckUnhealthyPlugins(t *testing.T) {
	r := NewRegistry()

	r.Register(PluginTypeLauncher, "test", func() Plugin {
		return &mockLauncher{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeLauncher, healthy: false}}
	})
	r.Register(PluginTypeAuth, "test", func() Plugin {
		return &mockAuth{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeAuth, healthy: false}}
	})
	r.Register(PluginTypeStorage, "test", func() Plugin {
		return &mockStorage{mockPlugin: mockPlugin{name: "test", pluginType: PluginTypeStorage, healthy: true}}
	})

	cfg := &RegistryConfig{
		Launcher:      "test",
		Auth:          "test",
		Storage:       "test",
		PluginConfigs: make(map[string]map[string]string),
	}

	r.Initialize(context.Background(), cfg)

	statuses := r.HealthCheck(context.Background())
	if len(statuses) != 3 {
		t.Fatalf("expected 3 health statuses, got %d", len(statuses))
	}

	unhealthyCount := 0
	for _, s := range statuses {
		if !s.Healthy {
			unhealthyCount++
			if s.Message != "Unhealthy" {
				t.Errorf("expected message 'Unhealthy' for unhealthy plugin, got '%s'", s.Message)
			}
		} else {
			if s.Message != "OK" {
				t.Errorf("expected message 'OK' for healthy plugin, got '%s'", s.Message)
			}
		}
	}
	if unhealthyCount != 2 {
		t.Errorf("expected 2 unhealthy plugins, got %d", unhealthyCount)
	}
}

func TestHealthCheckNoActivePlugins(t *testing.T) {
	r := NewRegistry()
	statuses := r.HealthCheck(context.Background())
	if len(statuses) != 0 {
		t.Errorf("expected 0 health statuses for uninitialized registry, got %d", len(statuses))
	}
}

func TestCloseNoActivePlugins(t *testing.T) {
	r := NewRegistry()
	err := r.Close()
	if err != nil {
		t.Errorf("Close on empty registry should succeed, got: %v", err)
	}
}

func TestCloseWithErrors(t *testing.T) {
	r := NewRegistry()

	launcher := &mockLauncher{mockPlugin: mockPlugin{
		name: "test", pluginType: PluginTypeLauncher, closeErr: errors.New("launcher close fail"),
	}}
	auth := &mockAuth{mockPlugin: mockPlugin{
		name: "test", pluginType: PluginTypeAuth, closeErr: errors.New("auth close fail"),
	}}
	storage := &mockStorage{mockPlugin: mockPlugin{
		name: "test", pluginType: PluginTypeStorage,
	}}

	r.Register(PluginTypeLauncher, "test", func() Plugin { return launcher })
	r.Register(PluginTypeAuth, "test", func() Plugin { return auth })
	r.Register(PluginTypeStorage, "test", func() Plugin { return storage })

	cfg := &RegistryConfig{
		Launcher:      "test",
		Auth:          "test",
		Storage:       "test",
		PluginConfigs: make(map[string]map[string]string),
	}

	r.Initialize(context.Background(), cfg)

	err := r.Close()
	if err == nil {
		t.Fatal("expected error when plugins fail to close")
	}
	if !strings.Contains(err.Error(), "launcher close") {
		t.Errorf("error should mention launcher close failure: %v", err)
	}
	if !strings.Contains(err.Error(), "auth close") {
		t.Errorf("error should mention auth close failure: %v", err)
	}
}

func TestAccessorsBeforeInitialize(t *testing.T) {
	r := NewRegistry()

	if r.Launcher() != nil {
		t.Error("expected nil launcher before initialize")
	}
	if r.Auth() != nil {
		t.Error("expected nil auth before initialize")
	}
	if r.Storage() != nil {
		t.Error("expected nil storage before initialize")
	}
}

func TestListPluginsByTypeEmpty(t *testing.T) {
	r := NewRegistry()

	// No plugins registered - should return empty
	plugins := r.ListPluginsByType(PluginTypeLauncher)
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestListPluginsByTypeUnknown(t *testing.T) {
	r := NewRegistry()

	plugins := r.ListPluginsByType(PluginType("unknown"))
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins for unknown type, got %d", len(plugins))
	}
}

func TestListPluginsEmpty(t *testing.T) {
	r := NewRegistry()
	plugins := r.ListPlugins(context.Background())
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestListPluginsInfo(t *testing.T) {
	r := NewRegistry()
	r.Register(PluginTypeLauncher, "mylaunch", func() Plugin {
		return &mockLauncher{mockPlugin: mockPlugin{name: "mylaunch", pluginType: PluginTypeLauncher}}
	})

	plugins := r.ListPlugins(context.Background())
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}

	p := plugins[0]
	if p.Name != "mylaunch" {
		t.Errorf("expected name 'mylaunch', got '%s'", p.Name)
	}
	if p.Type != PluginTypeLauncher {
		t.Errorf("expected type launcher, got '%s'", p.Type)
	}
	if p.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got '%s'", p.Version)
	}
	if p.Description != "Mock plugin for testing" {
		t.Errorf("unexpected description: '%s'", p.Description)
	}
}

func TestNewRegistryHasAllTypes(t *testing.T) {
	r := NewRegistry()
	expectedTypes := []PluginType{PluginTypeLauncher, PluginTypeAuth, PluginTypeStorage}
	for _, pt := range expectedTypes {
		if _, exists := r.factories[pt]; !exists {
			t.Errorf("registry missing factory map for type: %s", pt)
		}
	}
}

func TestPluginTypeConstants(t *testing.T) {
	if PluginTypeLauncher != "launcher" {
		t.Errorf("expected PluginTypeLauncher='launcher', got '%s'", PluginTypeLauncher)
	}
	if PluginTypeAuth != "auth" {
		t.Errorf("expected PluginTypeAuth='auth', got '%s'", PluginTypeAuth)
	}
	if PluginTypeStorage != "storage" {
		t.Errorf("expected PluginTypeStorage='storage', got '%s'", PluginTypeStorage)
	}
}

func TestLaunchTypeConstants(t *testing.T) {
	if LaunchTypeURL != "url" {
		t.Errorf("expected LaunchTypeURL='url', got '%s'", LaunchTypeURL)
	}
	if LaunchTypeContainer != "container" {
		t.Errorf("expected LaunchTypeContainer='container', got '%s'", LaunchTypeContainer)
	}
}

func TestLaunchStatusConstants(t *testing.T) {
	statuses := map[LaunchStatus]string{
		LaunchStatusPending:  "pending",
		LaunchStatusCreating: "creating",
		LaunchStatusRunning:  "running",
		LaunchStatusFailed:   "failed",
		LaunchStatusStopped:  "stopped",
		LaunchStatusExpired:  "expired",
		LaunchStatusRedirect: "redirect",
	}
	for status, expected := range statuses {
		if string(status) != expected {
			t.Errorf("expected '%s', got '%s'", expected, status)
		}
	}
}

func TestSentinelErrors(t *testing.T) {
	errs := map[string]error{
		"plugin not found":             ErrPluginNotFound,
		"plugin not ready":             ErrPluginNotReady,
		"invalid plugin configuration": ErrInvalidConfig,
		"plugin operation failed":      ErrOperationFailed,
		"operation not implemented":    ErrNotImplemented,
		"authentication required":      ErrAuthRequired,
		"permission denied":            ErrPermissionDenied,
		"resource not found":           ErrResourceNotFound,
		"resource already exists":      ErrResourceExists,
		"connection failed":            ErrConnectionFailed,
		"operation timed out":          ErrTimeout,
	}

	for expected, err := range errs {
		if err.Error() != expected {
			t.Errorf("expected error '%s', got '%s'", expected, err.Error())
		}
	}
}

func TestLoadRegistryConfigPartialEnv(t *testing.T) {
	// Set only one env var
	t.Setenv("SORTIE_PLUGIN_LAUNCHER", "k8s")
	t.Setenv("SORTIE_PLUGIN_AUTH", "")
	t.Setenv("SORTIE_PLUGIN_STORAGE", "")

	cfg := LoadRegistryConfig()
	if cfg.Launcher != "k8s" {
		t.Errorf("expected launcher 'k8s', got '%s'", cfg.Launcher)
	}
	if cfg.Auth != "noop" {
		t.Errorf("expected default auth 'noop', got '%s'", cfg.Auth)
	}
	if cfg.Storage != "sqlite" {
		t.Errorf("expected default storage 'sqlite', got '%s'", cfg.Storage)
	}
}

func TestClosePartialErrors(t *testing.T) {
	r := NewRegistry()

	launcher := &mockLauncher{mockPlugin: mockPlugin{
		name: "test", pluginType: PluginTypeLauncher,
	}}
	auth := &mockAuth{mockPlugin: mockPlugin{
		name: "test", pluginType: PluginTypeAuth,
	}}
	storage := &mockStorage{mockPlugin: mockPlugin{
		name: "test", pluginType: PluginTypeStorage, closeErr: errors.New("storage close fail"),
	}}

	r.Register(PluginTypeLauncher, "test", func() Plugin { return launcher })
	r.Register(PluginTypeAuth, "test", func() Plugin { return auth })
	r.Register(PluginTypeStorage, "test", func() Plugin { return storage })

	cfg := &RegistryConfig{
		Launcher:      "test",
		Auth:          "test",
		Storage:       "test",
		PluginConfigs: make(map[string]map[string]string),
	}

	r.Initialize(context.Background(), cfg)

	err := r.Close()
	if err == nil {
		t.Fatal("expected error from storage close failure")
	}
	if !strings.Contains(err.Error(), "storage close fail") {
		t.Errorf("error should mention storage: %v", err)
	}
	// All plugins should still have Close() called
	if !launcher.closed {
		t.Error("launcher should be closed even when storage fails")
	}
	if !auth.closed {
		t.Error("auth should be closed even when storage fails")
	}
}
