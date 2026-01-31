# Plugin System Architecture

Launchpad uses a plugin architecture for extensible connectors, allowing new
functionality to be added without modifying core code.

## Overview

The plugin system supports three types of plugins:

1. **Launcher Plugins** - Handle different application launch mechanisms
2. **Auth Plugins** - Handle authentication and authorization
3. **Storage Plugins** - Handle data persistence

## Quick Start

### Configuration

Configure plugins via environment variables:

```bash
# Select plugins (defaults shown)
export LAUNCHPAD_PLUGIN_LAUNCHER=url       # or: container
export LAUNCHPAD_PLUGIN_AUTH=noop          # default, no auth
export LAUNCHPAD_PLUGIN_STORAGE=sqlite     # or: memory
```

### Using the Plugin Registry

```go
import (
    "context"
    "github.com/rjsadow/launchpad/internal/plugins"
    _ "github.com/rjsadow/launchpad/internal/plugins/launcher"
    _ "github.com/rjsadow/launchpad/internal/plugins/auth"
    _ "github.com/rjsadow/launchpad/internal/plugins/storage"
)

func main() {
    ctx := context.Background()

    // Load configuration
    cfg := plugins.LoadRegistryConfig()

    // Initialize registry
    registry := plugins.Global()
    if err := registry.Initialize(ctx, cfg); err != nil {
        log.Fatal(err)
    }
    defer registry.Close()

    // Use plugins
    launcher := registry.Launcher()
    auth := registry.Auth()
    storage := registry.Storage()
}
```

## Built-in Plugins

### Launcher Plugins

#### URL Launcher (`url`)

Simple redirect-based launching where users are directed to the application URL.

```bash
export LAUNCHPAD_PLUGIN_LAUNCHER=url
```

#### Container Launcher (`container`)

Kubernetes container-based launching with VNC sidecar for interactive desktop applications.

```bash
export LAUNCHPAD_PLUGIN_LAUNCHER=container

# Optional configuration
export LAUNCHPAD_NAMESPACE=default
export KUBECONFIG=/path/to/kubeconfig
export LAUNCHPAD_VNC_SIDECAR_IMAGE=theasp/novnc:latest
```

### Auth Plugins

#### Noop Auth (`noop`)

No-operation auth provider that allows all access. Suitable for
development and trusted environments.

```bash
export LAUNCHPAD_PLUGIN_AUTH=noop
```

### Storage Plugins

#### SQLite Storage (`sqlite`)

SQLite database storage provider. Default and recommended for production.

```bash
export LAUNCHPAD_PLUGIN_STORAGE=sqlite
export LAUNCHPAD_DB=launchpad.db
```

#### Memory Storage (`memory`)

In-memory storage for testing and development. Data is lost on restart.

```bash
export LAUNCHPAD_PLUGIN_STORAGE=memory
```

## Creating Custom Plugins

### Step 1: Implement the Interface

Choose the appropriate interface based on your plugin type:

```go
// For launcher plugins
type LauncherPlugin interface {
    Plugin
    SupportedTypes() []LaunchType
    Launch(ctx context.Context, req *LaunchRequest) (*LaunchResult, error)
    GetStatus(ctx context.Context, sessionID string) (*LaunchResult, error)
    Terminate(ctx context.Context, sessionID string) error
    ListSessions(ctx context.Context, userID string) ([]*LaunchResult, error)
}

// For auth plugins
type AuthProvider interface {
    Plugin
    Authenticate(ctx context.Context, token string) (*AuthResult, error)
    GetUser(ctx context.Context, userID string) (*User, error)
    HasPermission(ctx context.Context, userID, permission string) (bool, error)
    GetLoginURL(redirectURL string) string
    HandleCallback(ctx context.Context, code, state string) (*AuthResult, error)
    Logout(ctx context.Context, token string) error
}

// For storage plugins
type StorageProvider interface {
    Plugin
    CreateApp(ctx context.Context, app *Application) error
    GetApp(ctx context.Context, id string) (*Application, error)
    UpdateApp(ctx context.Context, app *Application) error
    DeleteApp(ctx context.Context, id string) error
    ListApps(ctx context.Context) ([]*Application, error)
    // ... session and audit methods
}
```

### Step 2: Register the Plugin

Register your plugin in an `init()` function:

```go
package myplugin

import "github.com/rjsadow/launchpad/internal/plugins"

func init() {
    plugins.RegisterGlobal(plugins.PluginTypeLauncher, "myplugin", func() plugins.Plugin {
        return NewMyPlugin()
    })
}
```

### Step 3: Import the Plugin

Import your plugin package to register it:

```go
import _ "github.com/myorg/myplugin"
```

## Example: OIDC Auth Plugin

Here's a skeleton for an OpenID Connect authentication plugin:

```go
package oidc

import (
    "context"
    "github.com/rjsadow/launchpad/internal/plugins"
)

type OIDCAuthProvider struct {
    issuer       string
    clientID     string
    clientSecret string
    redirectURL  string
}

func init() {
    plugins.RegisterGlobal(plugins.PluginTypeAuth, "oidc", func() plugins.Plugin {
        return &OIDCAuthProvider{}
    })
}

func (p *OIDCAuthProvider) Name() string { return "oidc" }
func (p *OIDCAuthProvider) Type() plugins.PluginType { return plugins.PluginTypeAuth }
func (p *OIDCAuthProvider) Version() string { return "1.0.0" }
func (p *OIDCAuthProvider) Description() string {
    return "OpenID Connect authentication provider"
}

func (p *OIDCAuthProvider) Initialize(ctx context.Context, config map[string]string) error {
    p.issuer = config["issuer"]
    p.clientID = config["client_id"]
    p.clientSecret = config["client_secret"]
    p.redirectURL = config["redirect_url"]
    // Initialize OIDC client...
    return nil
}

func (p *OIDCAuthProvider) Authenticate(ctx context.Context, token string) (*plugins.AuthResult, error) {
    // Validate token with OIDC provider...
    return &plugins.AuthResult{Authenticated: true}, nil
}

// ... implement remaining methods
```

## Plugin Configuration

Plugins receive configuration through the `Initialize` method as a
`map[string]string`. Configuration can be set via:

1. **Environment variables** - Loaded by `LoadRegistryConfig()`
2. **Config files** - Passed through `RegistryConfig.PluginConfigs`
3. **Programmatic configuration** - Set directly on `RegistryConfig`

### Example Configuration Structure

```go
cfg := &plugins.RegistryConfig{
    Launcher: "container",
    Auth:     "oidc",
    Storage:  "sqlite",
    PluginConfigs: map[string]map[string]string{
        "launcher.container": {
            "namespace":         "launchpad",
            "session_timeout":   "2h",
            "pod_ready_timeout": "5m",
        },
        "auth.oidc": {
            "issuer":        "https://auth.example.com",
            "client_id":     "launchpad",
            "client_secret": "secret",
        },
        "storage.sqlite": {
            "db_path": "/data/launchpad.db",
        },
    },
}
```

## Health Checks

All plugins implement health checking:

```go
// Check individual plugin
if !registry.Launcher().Healthy(ctx) {
    log.Warn("Launcher unhealthy")
}

// Check all plugins
statuses := registry.HealthCheck(ctx)
for _, status := range statuses {
    log.Printf("%s.%s: healthy=%v", status.PluginType, status.PluginName, status.Healthy)
}
```

## Plugin Discovery

List all registered plugins:

```go
// List all plugins
plugins := registry.ListPlugins(ctx)
for _, p := range plugins {
    fmt.Printf("%s: %s v%s - %s\n", p.Type, p.Name, p.Version, p.Description)
}

// List by type
launchers := registry.ListPluginsByType(plugins.PluginTypeLauncher)
```

## Best Practices

1. **Thread Safety** - Plugins may be called concurrently; use appropriate synchronization
2. **Context Propagation** - Respect context cancellation and timeouts
3. **Error Handling** - Return meaningful errors using the standard error types
in `plugins.Err*`
4. **Configuration Validation** - Validate configuration in `Initialize()` and
fail fast
5. **Resource Cleanup** - Implement `Close()` to release resources properly
6. **Health Checks** - Implement meaningful `Healthy()` checks

## Error Handling

Use standard error types for consistency:

```go
var (
    ErrPluginNotFound    // Plugin not registered
    ErrPluginNotReady    // Plugin not initialized
    ErrInvalidConfig     // Invalid configuration
    ErrOperationFailed   // Generic operation failure
    ErrNotImplemented    // Operation not supported
    ErrAuthRequired      // Authentication required
    ErrPermissionDenied  // Permission denied
    ErrResourceNotFound  // Resource not found
    ErrResourceExists    // Resource already exists
    ErrConnectionFailed  // Connection failed
    ErrTimeout           // Operation timed out
)
```
