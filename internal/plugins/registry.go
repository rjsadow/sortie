package plugins

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

// Registry manages all registered plugins and provides access to active plugins.
type Registry struct {
	mu sync.RWMutex

	// factories stores plugin factories by type and name
	factories map[PluginType]map[string]PluginFactory

	// active stores initialized plugin instances by type
	activeLauncher LauncherPlugin
	activeAuth     AuthProvider
	activeStorage  StorageProvider

	// config stores the registry configuration
	config *RegistryConfig
}

// RegistryConfig holds configuration for the plugin registry.
type RegistryConfig struct {
	// Launcher is the name of the launcher plugin to use.
	Launcher string

	// Auth is the name of the auth plugin to use.
	Auth string

	// Storage is the name of the storage plugin to use.
	Storage string

	// PluginConfigs holds configuration for individual plugins.
	// Key format: "type.name" (e.g., "launcher.container", "auth.oidc")
	PluginConfigs map[string]map[string]string
}

// DefaultRegistryConfig returns the default registry configuration.
func DefaultRegistryConfig() *RegistryConfig {
	return &RegistryConfig{
		Launcher:      "url",     // Default to URL launcher
		Auth:          "noop",    // Default to no authentication
		Storage:       "sqlite",  // Default to SQLite storage
		PluginConfigs: make(map[string]map[string]string),
	}
}

// LoadRegistryConfig loads registry configuration from environment variables.
func LoadRegistryConfig() *RegistryConfig {
	cfg := DefaultRegistryConfig()

	if v := os.Getenv("LAUNCHPAD_PLUGIN_LAUNCHER"); v != "" {
		cfg.Launcher = strings.ToLower(v)
	}

	if v := os.Getenv("LAUNCHPAD_PLUGIN_AUTH"); v != "" {
		cfg.Auth = strings.ToLower(v)
	}

	if v := os.Getenv("LAUNCHPAD_PLUGIN_STORAGE"); v != "" {
		cfg.Storage = strings.ToLower(v)
	}

	return cfg
}

// NewRegistry creates a new plugin registry.
func NewRegistry() *Registry {
	return &Registry{
		factories: map[PluginType]map[string]PluginFactory{
			PluginTypeLauncher: make(map[string]PluginFactory),
			PluginTypeAuth:     make(map[string]PluginFactory),
			PluginTypeStorage:  make(map[string]PluginFactory),
		},
	}
}

// Register adds a plugin factory to the registry.
// This should be called during init() in plugin packages.
func (r *Registry) Register(pluginType PluginType, name string, factory PluginFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[pluginType]; !exists {
		return fmt.Errorf("unknown plugin type: %s", pluginType)
	}

	if _, exists := r.factories[pluginType][name]; exists {
		return fmt.Errorf("plugin already registered: %s.%s", pluginType, name)
	}

	r.factories[pluginType][name] = factory
	log.Printf("Registered plugin: %s.%s", pluginType, name)
	return nil
}

// Initialize initializes the registry with the given configuration.
// It creates and initializes plugin instances for the configured plugins.
func (r *Registry) Initialize(ctx context.Context, cfg *RegistryConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.config = cfg

	// Initialize launcher plugin
	if err := r.initLauncher(ctx, cfg.Launcher, cfg.PluginConfigs); err != nil {
		return fmt.Errorf("failed to initialize launcher plugin: %w", err)
	}

	// Initialize auth plugin
	if err := r.initAuth(ctx, cfg.Auth, cfg.PluginConfigs); err != nil {
		return fmt.Errorf("failed to initialize auth plugin: %w", err)
	}

	// Initialize storage plugin
	if err := r.initStorage(ctx, cfg.Storage, cfg.PluginConfigs); err != nil {
		return fmt.Errorf("failed to initialize storage plugin: %w", err)
	}

	return nil
}

func (r *Registry) initLauncher(ctx context.Context, name string, configs map[string]map[string]string) error {
	factory, exists := r.factories[PluginTypeLauncher][name]
	if !exists {
		return fmt.Errorf("launcher plugin not found: %s", name)
	}

	plugin := factory()
	launcher, ok := plugin.(LauncherPlugin)
	if !ok {
		return fmt.Errorf("plugin %s does not implement LauncherPlugin", name)
	}

	configKey := fmt.Sprintf("%s.%s", PluginTypeLauncher, name)
	pluginConfig := configs[configKey]
	if pluginConfig == nil {
		pluginConfig = make(map[string]string)
	}

	if err := launcher.Initialize(ctx, pluginConfig); err != nil {
		return fmt.Errorf("failed to initialize %s: %w", name, err)
	}

	r.activeLauncher = launcher
	log.Printf("Initialized launcher plugin: %s", name)
	return nil
}

func (r *Registry) initAuth(ctx context.Context, name string, configs map[string]map[string]string) error {
	factory, exists := r.factories[PluginTypeAuth][name]
	if !exists {
		return fmt.Errorf("auth plugin not found: %s", name)
	}

	plugin := factory()
	auth, ok := plugin.(AuthProvider)
	if !ok {
		return fmt.Errorf("plugin %s does not implement AuthProvider", name)
	}

	configKey := fmt.Sprintf("%s.%s", PluginTypeAuth, name)
	pluginConfig := configs[configKey]
	if pluginConfig == nil {
		pluginConfig = make(map[string]string)
	}

	if err := auth.Initialize(ctx, pluginConfig); err != nil {
		return fmt.Errorf("failed to initialize %s: %w", name, err)
	}

	r.activeAuth = auth
	log.Printf("Initialized auth plugin: %s", name)
	return nil
}

func (r *Registry) initStorage(ctx context.Context, name string, configs map[string]map[string]string) error {
	factory, exists := r.factories[PluginTypeStorage][name]
	if !exists {
		return fmt.Errorf("storage plugin not found: %s", name)
	}

	plugin := factory()
	storage, ok := plugin.(StorageProvider)
	if !ok {
		return fmt.Errorf("plugin %s does not implement StorageProvider", name)
	}

	configKey := fmt.Sprintf("%s.%s", PluginTypeStorage, name)
	pluginConfig := configs[configKey]
	if pluginConfig == nil {
		pluginConfig = make(map[string]string)
	}

	if err := storage.Initialize(ctx, pluginConfig); err != nil {
		return fmt.Errorf("failed to initialize %s: %w", name, err)
	}

	r.activeStorage = storage
	log.Printf("Initialized storage plugin: %s", name)
	return nil
}

// Launcher returns the active launcher plugin.
func (r *Registry) Launcher() LauncherPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.activeLauncher
}

// Auth returns the active auth plugin.
func (r *Registry) Auth() AuthProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.activeAuth
}

// Storage returns the active storage plugin.
func (r *Registry) Storage() StorageProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.activeStorage
}

// ListPlugins returns information about all registered plugins.
func (r *Registry) ListPlugins(ctx context.Context) []PluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var plugins []PluginInfo

	for pluginType, factories := range r.factories {
		for name, factory := range factories {
			plugin := factory()
			plugins = append(plugins, PluginInfo{
				Name:        name,
				Type:        pluginType,
				Version:     plugin.Version(),
				Description: plugin.Description(),
			})
		}
	}

	return plugins
}

// ListPluginsByType returns information about plugins of a specific type.
func (r *Registry) ListPluginsByType(pluginType PluginType) []PluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var plugins []PluginInfo

	if factories, exists := r.factories[pluginType]; exists {
		for name, factory := range factories {
			plugin := factory()
			plugins = append(plugins, PluginInfo{
				Name:        name,
				Type:        pluginType,
				Version:     plugin.Version(),
				Description: plugin.Description(),
			})
		}
	}

	return plugins
}

// HealthCheck performs health checks on all active plugins.
func (r *Registry) HealthCheck(ctx context.Context) []HealthStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var statuses []HealthStatus

	if r.activeLauncher != nil {
		statuses = append(statuses, checkHealth(ctx, r.activeLauncher))
	}

	if r.activeAuth != nil {
		statuses = append(statuses, checkHealth(ctx, r.activeAuth))
	}

	if r.activeStorage != nil {
		statuses = append(statuses, checkHealth(ctx, r.activeStorage))
	}

	return statuses
}

func checkHealth(ctx context.Context, plugin Plugin) HealthStatus {
	healthy := plugin.Healthy(ctx)
	status := HealthStatus{
		PluginName: plugin.Name(),
		PluginType: plugin.Type(),
		Healthy:    healthy,
	}

	if healthy {
		status.Message = "OK"
	} else {
		status.Message = "Unhealthy"
	}

	return status
}

// Close releases resources for all active plugins.
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error

	if r.activeLauncher != nil {
		if err := r.activeLauncher.Close(); err != nil {
			errs = append(errs, fmt.Errorf("launcher close: %w", err))
		}
	}

	if r.activeAuth != nil {
		if err := r.activeAuth.Close(); err != nil {
			errs = append(errs, fmt.Errorf("auth close: %w", err))
		}
	}

	if r.activeStorage != nil {
		if err := r.activeStorage.Close(); err != nil {
			errs = append(errs, fmt.Errorf("storage close: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing plugins: %v", errs)
	}

	return nil
}

// Global registry instance
var globalRegistry *Registry
var globalRegistryOnce sync.Once

// Global returns the global plugin registry.
func Global() *Registry {
	globalRegistryOnce.Do(func() {
		globalRegistry = NewRegistry()
	})
	return globalRegistry
}

// RegisterGlobal registers a plugin with the global registry.
// This is a convenience function for use in plugin init() functions.
func RegisterGlobal(pluginType PluginType, name string, factory PluginFactory) {
	if err := Global().Register(pluginType, name, factory); err != nil {
		log.Printf("Warning: failed to register plugin %s.%s: %v", pluginType, name, err)
	}
}
