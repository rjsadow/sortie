package launcher

import (
	"context"
	"fmt"
	"sync"

	"github.com/rjsadow/sortie/internal/plugins"
)

// URLLauncher implements LauncherPlugin for URL-based application launches.
// It provides simple redirect-based launching where the client is directed
// to open the application URL directly.
//
// NOTE: This plugin maintains session state in-memory and is NOT horizontally scalable.
// For multi-replica deployments, use the sessions.Manager which stores all state in the
// database. This plugin is intended for single-instance or plugin-system use only.
type URLLauncher struct {
	mu       sync.RWMutex
	sessions map[string]*plugins.LaunchResult
	config   map[string]string
}

func init() {
	plugins.RegisterGlobal(plugins.PluginTypeLauncher, "url", func() plugins.Plugin {
		return NewURLLauncher()
	})
}

// NewURLLauncher creates a new URL launcher plugin.
func NewURLLauncher() *URLLauncher {
	return &URLLauncher{
		sessions: make(map[string]*plugins.LaunchResult),
	}
}

// Name returns the plugin name.
func (l *URLLauncher) Name() string {
	return "url"
}

// Type returns the plugin type.
func (l *URLLauncher) Type() plugins.PluginType {
	return plugins.PluginTypeLauncher
}

// Version returns the plugin version.
func (l *URLLauncher) Version() string {
	return "1.0.0"
}

// Description returns a human-readable description.
func (l *URLLauncher) Description() string {
	return "URL-based application launcher that redirects users to application URLs"
}

// Initialize sets up the plugin with configuration.
func (l *URLLauncher) Initialize(ctx context.Context, config map[string]string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.config = config
	return nil
}

// Healthy returns true if the plugin is operational.
func (l *URLLauncher) Healthy(ctx context.Context) bool {
	return true
}

// Close releases resources.
func (l *URLLauncher) Close() error {
	return nil
}

// SupportedTypes returns the launch types this launcher supports.
func (l *URLLauncher) SupportedTypes() []plugins.LaunchType {
	return []plugins.LaunchType{plugins.LaunchTypeURL}
}

// Launch starts an application and returns the result.
func (l *URLLauncher) Launch(ctx context.Context, req *plugins.LaunchRequest) (*plugins.LaunchResult, error) {
	if req.LaunchType != plugins.LaunchTypeURL {
		return nil, fmt.Errorf("unsupported launch type: %s", req.LaunchType)
	}

	if req.URL == "" {
		return nil, fmt.Errorf("URL is required for URL launcher")
	}

	// Generate a session ID for tracking
	sessionID := fmt.Sprintf("url-%s-%s", req.AppID, req.UserID)

	result := &plugins.LaunchResult{
		SessionID: sessionID,
		Status:    plugins.LaunchStatusRedirect,
		URL:       req.URL,
		Message:   "Redirect to application URL",
	}

	// Store the session
	l.mu.Lock()
	l.sessions[sessionID] = result
	l.mu.Unlock()

	return result, nil
}

// GetStatus returns the current status of a launch session.
func (l *URLLauncher) GetStatus(ctx context.Context, sessionID string) (*plugins.LaunchResult, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result, exists := l.sessions[sessionID]
	if !exists {
		return nil, plugins.ErrResourceNotFound
	}

	return result, nil
}

// Terminate stops a running launch session.
func (l *URLLauncher) Terminate(ctx context.Context, sessionID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, exists := l.sessions[sessionID]; !exists {
		return plugins.ErrResourceNotFound
	}

	delete(l.sessions, sessionID)
	return nil
}

// ListSessions returns all active sessions for a user.
func (l *URLLauncher) ListSessions(ctx context.Context, userID string) ([]*plugins.LaunchResult, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var results []*plugins.LaunchResult
	for _, result := range l.sessions {
		results = append(results, result)
	}

	return results, nil
}

// Verify interface compliance
var _ plugins.LauncherPlugin = (*URLLauncher)(nil)
