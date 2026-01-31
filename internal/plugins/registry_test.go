package plugins

import (
	"context"
	"testing"
)

// mockPlugin is a test plugin implementation
type mockPlugin struct {
	name        string
	pluginType  PluginType
	healthy     bool
	initialized bool
	closed      bool
}

func (p *mockPlugin) Name() string           { return p.name }
func (p *mockPlugin) Type() PluginType       { return p.pluginType }
func (p *mockPlugin) Version() string        { return "1.0.0" }
func (p *mockPlugin) Description() string    { return "Mock plugin for testing" }
func (p *mockPlugin) Healthy(ctx context.Context) bool { return p.healthy }
func (p *mockPlugin) Close() error           { p.closed = true; return nil }
func (p *mockPlugin) Initialize(ctx context.Context, config map[string]string) error {
	p.initialized = true
	return nil
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
}
