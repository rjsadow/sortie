package auth

import (
	"context"

	"github.com/rjsadow/sortie/internal/plugins"
)

// NoopAuthProvider implements AuthProvider with no authentication.
// All users are considered authenticated and have all permissions.
// This is suitable for development and trusted environments.
type NoopAuthProvider struct {
	config map[string]string
}

func init() {
	plugins.RegisterGlobal(plugins.PluginTypeAuth, "noop", func() plugins.Plugin {
		return NewNoopAuthProvider()
	})
}

// NewNoopAuthProvider creates a new noop auth provider.
func NewNoopAuthProvider() *NoopAuthProvider {
	return &NoopAuthProvider{}
}

// Name returns the plugin name.
func (p *NoopAuthProvider) Name() string {
	return "noop"
}

// Type returns the plugin type.
func (p *NoopAuthProvider) Type() plugins.PluginType {
	return plugins.PluginTypeAuth
}

// Version returns the plugin version.
func (p *NoopAuthProvider) Version() string {
	return "1.0.0"
}

// Description returns a human-readable description.
func (p *NoopAuthProvider) Description() string {
	return "No-operation auth provider that allows all access (for development)"
}

// Initialize sets up the plugin with configuration.
func (p *NoopAuthProvider) Initialize(ctx context.Context, config map[string]string) error {
	p.config = config
	return nil
}

// Healthy returns true if the plugin is operational.
func (p *NoopAuthProvider) Healthy(ctx context.Context) bool {
	return true
}

// Close releases resources.
func (p *NoopAuthProvider) Close() error {
	return nil
}

// Authenticate validates credentials and returns the authenticated user.
// With noop auth, all tokens are considered valid.
func (p *NoopAuthProvider) Authenticate(ctx context.Context, token string) (*plugins.AuthResult, error) {
	return &plugins.AuthResult{
		Authenticated: true,
		User: &plugins.User{
			ID:       "anonymous",
			Username: "anonymous",
			Name:     "Anonymous User",
			Roles:    []string{"user"},
		},
		Message: "No authentication required",
	}, nil
}

// GetUser retrieves user information by ID.
func (p *NoopAuthProvider) GetUser(ctx context.Context, userID string) (*plugins.User, error) {
	return &plugins.User{
		ID:       userID,
		Username: userID,
		Name:     "User " + userID,
		Roles:    []string{"user"},
	}, nil
}

// HasPermission checks if a user has a specific permission.
// With noop auth, all permissions are granted.
func (p *NoopAuthProvider) HasPermission(ctx context.Context, userID, permission string) (bool, error) {
	return true, nil
}

// GetLoginURL returns the URL for initiating login.
// With noop auth, no login is required.
func (p *NoopAuthProvider) GetLoginURL(redirectURL string) string {
	return redirectURL
}

// HandleCallback processes the OAuth/OIDC callback.
// With noop auth, this immediately returns success.
func (p *NoopAuthProvider) HandleCallback(ctx context.Context, code, state string) (*plugins.AuthResult, error) {
	return &plugins.AuthResult{
		Authenticated: true,
		User: &plugins.User{
			ID:       "anonymous",
			Username: "anonymous",
			Name:     "Anonymous User",
			Roles:    []string{"user"},
		},
		Message: "No authentication required",
	}, nil
}

// Logout invalidates the user's session.
// With noop auth, this is a no-op.
func (p *NoopAuthProvider) Logout(ctx context.Context, token string) error {
	return nil
}

// Verify interface compliance
var _ plugins.AuthProvider = (*NoopAuthProvider)(nil)
