package launcher

import (
	"context"
	"testing"

	"github.com/rjsadow/sortie/internal/plugins"
)

func TestNewURLLauncher(t *testing.T) {
	l := NewURLLauncher()
	if l == nil {
		t.Fatal("NewURLLauncher returned nil")
	}
	if l.sessions == nil {
		t.Fatal("sessions map is nil")
	}
}

func TestURLLauncher_PluginMetadata(t *testing.T) {
	l := NewURLLauncher()

	if got := l.Name(); got != "url" {
		t.Errorf("Name() = %q, want %q", got, "url")
	}
	if got := l.Type(); got != plugins.PluginTypeLauncher {
		t.Errorf("Type() = %q, want %q", got, plugins.PluginTypeLauncher)
	}
	if got := l.Version(); got != "1.0.0" {
		t.Errorf("Version() = %q, want %q", got, "1.0.0")
	}
	if got := l.Description(); got == "" {
		t.Error("Description() returned empty string")
	}
}

func TestURLLauncher_Initialize(t *testing.T) {
	l := NewURLLauncher()
	ctx := context.Background()

	config := map[string]string{"key": "value"}
	err := l.Initialize(ctx, config)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	if l.config["key"] != "value" {
		t.Error("Initialize() did not store config")
	}
}

func TestURLLauncher_Healthy(t *testing.T) {
	l := NewURLLauncher()
	if !l.Healthy(context.Background()) {
		t.Error("Healthy() = false, want true")
	}
}

func TestURLLauncher_Close(t *testing.T) {
	l := NewURLLauncher()
	if err := l.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestURLLauncher_SupportedTypes(t *testing.T) {
	l := NewURLLauncher()
	types := l.SupportedTypes()

	if len(types) != 1 {
		t.Fatalf("SupportedTypes() returned %d types, want 1", len(types))
	}
	if types[0] != plugins.LaunchTypeURL {
		t.Errorf("SupportedTypes()[0] = %q, want %q", types[0], plugins.LaunchTypeURL)
	}
}

func TestURLLauncher_Launch(t *testing.T) {
	l := NewURLLauncher()
	ctx := context.Background()

	req := &plugins.LaunchRequest{
		AppID:      "app-1",
		AppName:    "Test App",
		UserID:     "user-1",
		LaunchType: plugins.LaunchTypeURL,
		URL:        "https://example.com",
	}

	result, err := l.Launch(ctx, req)
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}

	if result.SessionID == "" {
		t.Error("Launch() returned empty SessionID")
	}
	if result.Status != plugins.LaunchStatusRedirect {
		t.Errorf("Launch() status = %q, want %q", result.Status, plugins.LaunchStatusRedirect)
	}
	if result.URL != "https://example.com" {
		t.Errorf("Launch() URL = %q, want %q", result.URL, "https://example.com")
	}
	if result.Message == "" {
		t.Error("Launch() returned empty Message")
	}
}

func TestURLLauncher_Launch_WrongType(t *testing.T) {
	l := NewURLLauncher()
	ctx := context.Background()

	req := &plugins.LaunchRequest{
		AppID:      "app-1",
		UserID:     "user-1",
		LaunchType: plugins.LaunchTypeContainer,
		URL:        "https://example.com",
	}

	_, err := l.Launch(ctx, req)
	if err == nil {
		t.Fatal("Launch() expected error for wrong launch type")
	}
}

func TestURLLauncher_Launch_EmptyURL(t *testing.T) {
	l := NewURLLauncher()
	ctx := context.Background()

	req := &plugins.LaunchRequest{
		AppID:      "app-1",
		UserID:     "user-1",
		LaunchType: plugins.LaunchTypeURL,
		URL:        "",
	}

	_, err := l.Launch(ctx, req)
	if err == nil {
		t.Fatal("Launch() expected error for empty URL")
	}
}

func TestURLLauncher_GetStatus(t *testing.T) {
	l := NewURLLauncher()
	ctx := context.Background()

	// Launch first to create a session
	req := &plugins.LaunchRequest{
		AppID:      "app-1",
		UserID:     "user-1",
		LaunchType: plugins.LaunchTypeURL,
		URL:        "https://example.com",
	}
	result, err := l.Launch(ctx, req)
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}

	// Get status of existing session
	status, err := l.GetStatus(ctx, result.SessionID)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if status.SessionID != result.SessionID {
		t.Errorf("GetStatus() SessionID = %q, want %q", status.SessionID, result.SessionID)
	}
	if status.Status != plugins.LaunchStatusRedirect {
		t.Errorf("GetStatus() status = %q, want %q", status.Status, plugins.LaunchStatusRedirect)
	}
}

func TestURLLauncher_GetStatus_NotFound(t *testing.T) {
	l := NewURLLauncher()
	ctx := context.Background()

	_, err := l.GetStatus(ctx, "nonexistent")
	if err != plugins.ErrResourceNotFound {
		t.Errorf("GetStatus() error = %v, want %v", err, plugins.ErrResourceNotFound)
	}
}

func TestURLLauncher_Terminate(t *testing.T) {
	l := NewURLLauncher()
	ctx := context.Background()

	// Launch first
	req := &plugins.LaunchRequest{
		AppID:      "app-1",
		UserID:     "user-1",
		LaunchType: plugins.LaunchTypeURL,
		URL:        "https://example.com",
	}
	result, err := l.Launch(ctx, req)
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}

	// Terminate the session
	err = l.Terminate(ctx, result.SessionID)
	if err != nil {
		t.Fatalf("Terminate() error = %v", err)
	}

	// Session should be gone
	_, err = l.GetStatus(ctx, result.SessionID)
	if err != plugins.ErrResourceNotFound {
		t.Errorf("GetStatus() after Terminate() error = %v, want %v", err, plugins.ErrResourceNotFound)
	}
}

func TestURLLauncher_Terminate_NotFound(t *testing.T) {
	l := NewURLLauncher()
	ctx := context.Background()

	err := l.Terminate(ctx, "nonexistent")
	if err != plugins.ErrResourceNotFound {
		t.Errorf("Terminate() error = %v, want %v", err, plugins.ErrResourceNotFound)
	}
}

func TestURLLauncher_ListSessions(t *testing.T) {
	l := NewURLLauncher()
	ctx := context.Background()

	// Empty initially
	results, err := l.ListSessions(ctx, "user-1")
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("ListSessions() returned %d results, want 0", len(results))
	}

	// Launch sessions with different app IDs (same appID+userID overwrites)
	for i, appID := range []string{"app-1", "app-2"} {
		req := &plugins.LaunchRequest{
			AppID:      appID,
			UserID:     "user-1",
			LaunchType: plugins.LaunchTypeURL,
			URL:        "https://example.com",
		}
		_, err := l.Launch(ctx, req)
		if err != nil {
			t.Fatalf("Launch() %d error = %v", i, err)
		}
	}

	// Should list both (URLLauncher.ListSessions returns all sessions regardless of userID)
	results, err = l.ListSessions(ctx, "user-1")
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(results) != 2 {
		t.Errorf("ListSessions() returned %d results, want 2", len(results))
	}
}

func TestURLLauncher_SessionIDFormat(t *testing.T) {
	l := NewURLLauncher()
	ctx := context.Background()

	req := &plugins.LaunchRequest{
		AppID:      "myapp",
		UserID:     "user42",
		LaunchType: plugins.LaunchTypeURL,
		URL:        "https://example.com",
	}

	result, err := l.Launch(ctx, req)
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}

	expected := "url-myapp-user42"
	if result.SessionID != expected {
		t.Errorf("Launch() SessionID = %q, want %q", result.SessionID, expected)
	}
}

func TestURLLauncher_LaunchOverwritesSession(t *testing.T) {
	l := NewURLLauncher()
	ctx := context.Background()

	// Launch same app+user twice - second should overwrite
	req := &plugins.LaunchRequest{
		AppID:      "app-1",
		UserID:     "user-1",
		LaunchType: plugins.LaunchTypeURL,
		URL:        "https://first.com",
	}
	_, err := l.Launch(ctx, req)
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}

	req.URL = "https://second.com"
	result2, err := l.Launch(ctx, req)
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}

	// Should have the second URL
	status, err := l.GetStatus(ctx, result2.SessionID)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if status.URL != "https://second.com" {
		t.Errorf("GetStatus() URL = %q, want %q", status.URL, "https://second.com")
	}
}

func TestURLLauncher_InterfaceCompliance(t *testing.T) {
	var _ plugins.LauncherPlugin = (*URLLauncher)(nil)
}
