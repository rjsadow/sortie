package launcher

import (
	"context"
	"testing"
	"time"

	"github.com/rjsadow/launchpad/internal/plugins"
)

func TestNewContainerLauncher(t *testing.T) {
	l := NewContainerLauncher()
	if l == nil {
		t.Fatal("NewContainerLauncher returned nil")
	}
	if l.sessions == nil {
		t.Fatal("sessions map is nil")
	}
	if l.sessionTimeout != DefaultSessionTimeout {
		t.Errorf("sessionTimeout = %v, want %v", l.sessionTimeout, DefaultSessionTimeout)
	}
	if l.cleanupInterval != DefaultCleanupInterval {
		t.Errorf("cleanupInterval = %v, want %v", l.cleanupInterval, DefaultCleanupInterval)
	}
	if l.podReadyTimeout != DefaultPodReadyTimeout {
		t.Errorf("podReadyTimeout = %v, want %v", l.podReadyTimeout, DefaultPodReadyTimeout)
	}
	if l.stopCh == nil {
		t.Fatal("stopCh is nil")
	}
}

func TestContainerLauncher_PluginMetadata(t *testing.T) {
	l := NewContainerLauncher()

	if got := l.Name(); got != "container" {
		t.Errorf("Name() = %q, want %q", got, "container")
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

func TestContainerLauncher_SupportedTypes(t *testing.T) {
	l := NewContainerLauncher()
	types := l.SupportedTypes()

	if len(types) != 1 {
		t.Fatalf("SupportedTypes() returned %d types, want 1", len(types))
	}
	if types[0] != plugins.LaunchTypeContainer {
		t.Errorf("SupportedTypes()[0] = %q, want %q", types[0], plugins.LaunchTypeContainer)
	}
}

func TestContainerLauncher_Initialize_DefaultConfig(t *testing.T) {
	l := NewContainerLauncher()
	ctx := context.Background()

	err := l.Initialize(ctx, map[string]string{})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	// Should keep defaults when no config overrides
	if l.sessionTimeout != DefaultSessionTimeout {
		t.Errorf("sessionTimeout = %v, want %v", l.sessionTimeout, DefaultSessionTimeout)
	}
	if l.cleanupInterval != DefaultCleanupInterval {
		t.Errorf("cleanupInterval = %v, want %v", l.cleanupInterval, DefaultCleanupInterval)
	}
	if l.podReadyTimeout != DefaultPodReadyTimeout {
		t.Errorf("podReadyTimeout = %v, want %v", l.podReadyTimeout, DefaultPodReadyTimeout)
	}
	l.Close()
}

func TestContainerLauncher_Initialize_CustomTimeouts(t *testing.T) {
	l := NewContainerLauncher()
	ctx := context.Background()

	config := map[string]string{
		"session_timeout":  "1h",
		"cleanup_interval": "10m",
		"pod_ready_timeout": "3m",
	}

	err := l.Initialize(ctx, config)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	if l.sessionTimeout != 1*time.Hour {
		t.Errorf("sessionTimeout = %v, want %v", l.sessionTimeout, 1*time.Hour)
	}
	if l.cleanupInterval != 10*time.Minute {
		t.Errorf("cleanupInterval = %v, want %v", l.cleanupInterval, 10*time.Minute)
	}
	if l.podReadyTimeout != 3*time.Minute {
		t.Errorf("podReadyTimeout = %v, want %v", l.podReadyTimeout, 3*time.Minute)
	}
	l.Close()
}

func TestContainerLauncher_Initialize_InvalidDuration(t *testing.T) {
	l := NewContainerLauncher()
	ctx := context.Background()

	config := map[string]string{
		"session_timeout": "notaduration",
	}

	err := l.Initialize(ctx, config)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	// Should keep default when duration is invalid
	if l.sessionTimeout != DefaultSessionTimeout {
		t.Errorf("sessionTimeout = %v, want default %v", l.sessionTimeout, DefaultSessionTimeout)
	}
	l.Close()
}

func TestContainerLauncher_Close(t *testing.T) {
	l := NewContainerLauncher()
	err := l.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestContainerLauncher_Launch_WrongType(t *testing.T) {
	l := NewContainerLauncher()
	ctx := context.Background()

	req := &plugins.LaunchRequest{
		AppID:          "app-1",
		UserID:         "user-1",
		LaunchType:     plugins.LaunchTypeURL,
		ContainerImage: "nginx:latest",
	}

	_, err := l.Launch(ctx, req)
	if err == nil {
		t.Fatal("Launch() expected error for wrong launch type")
	}
}

func TestContainerLauncher_Launch_EmptyImage(t *testing.T) {
	l := NewContainerLauncher()
	ctx := context.Background()

	req := &plugins.LaunchRequest{
		AppID:          "app-1",
		UserID:         "user-1",
		LaunchType:     plugins.LaunchTypeContainer,
		ContainerImage: "",
	}

	_, err := l.Launch(ctx, req)
	if err == nil {
		t.Fatal("Launch() expected error for empty container image")
	}
}

func TestContainerLauncher_GetStatus(t *testing.T) {
	l := NewContainerLauncher()
	ctx := context.Background()

	// Insert a session directly for testing
	sessionID := "test-session-1"
	l.sessions[sessionID] = &containerSession{
		result: &plugins.LaunchResult{
			SessionID: sessionID,
			Status:    plugins.LaunchStatusRunning,
			Message:   "Container running",
		},
		podName:   "test-pod",
		podIP:     "10.0.0.1",
		userID:    "user-1",
		appID:     "app-1",
		createdAt: time.Now(),
	}

	result, err := l.GetStatus(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if result.SessionID != sessionID {
		t.Errorf("GetStatus() SessionID = %q, want %q", result.SessionID, sessionID)
	}
	if result.Status != plugins.LaunchStatusRunning {
		t.Errorf("GetStatus() status = %q, want %q", result.Status, plugins.LaunchStatusRunning)
	}
}

func TestContainerLauncher_GetStatus_NotFound(t *testing.T) {
	l := NewContainerLauncher()
	ctx := context.Background()

	_, err := l.GetStatus(ctx, "nonexistent")
	if err != plugins.ErrResourceNotFound {
		t.Errorf("GetStatus() error = %v, want %v", err, plugins.ErrResourceNotFound)
	}
}

func TestContainerLauncher_ListSessions_Empty(t *testing.T) {
	l := NewContainerLauncher()
	ctx := context.Background()

	results, err := l.ListSessions(ctx, "user-1")
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("ListSessions() returned %d results, want 0", len(results))
	}
}

func TestContainerLauncher_ListSessions_FilterByUser(t *testing.T) {
	l := NewContainerLauncher()
	ctx := context.Background()

	// Insert sessions for different users
	l.sessions["session-1"] = &containerSession{
		result:    &plugins.LaunchResult{SessionID: "session-1", Status: plugins.LaunchStatusRunning},
		userID:    "user-1",
		appID:     "app-1",
		createdAt: time.Now(),
	}
	l.sessions["session-2"] = &containerSession{
		result:    &plugins.LaunchResult{SessionID: "session-2", Status: plugins.LaunchStatusRunning},
		userID:    "user-2",
		appID:     "app-2",
		createdAt: time.Now(),
	}
	l.sessions["session-3"] = &containerSession{
		result:    &plugins.LaunchResult{SessionID: "session-3", Status: plugins.LaunchStatusRunning},
		userID:    "user-1",
		appID:     "app-3",
		createdAt: time.Now(),
	}

	// Filter by user-1
	results, err := l.ListSessions(ctx, "user-1")
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(results) != 2 {
		t.Errorf("ListSessions(user-1) returned %d results, want 2", len(results))
	}

	// Filter by user-2
	results, err = l.ListSessions(ctx, "user-2")
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(results) != 1 {
		t.Errorf("ListSessions(user-2) returned %d results, want 1", len(results))
	}
}

func TestContainerLauncher_ListSessions_EmptyUserID(t *testing.T) {
	l := NewContainerLauncher()
	ctx := context.Background()

	l.sessions["session-1"] = &containerSession{
		result:    &plugins.LaunchResult{SessionID: "session-1", Status: plugins.LaunchStatusRunning},
		userID:    "user-1",
		createdAt: time.Now(),
	}
	l.sessions["session-2"] = &containerSession{
		result:    &plugins.LaunchResult{SessionID: "session-2", Status: plugins.LaunchStatusRunning},
		userID:    "user-2",
		createdAt: time.Now(),
	}

	// Empty userID should return all sessions
	results, err := l.ListSessions(ctx, "")
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(results) != 2 {
		t.Errorf("ListSessions('') returned %d results, want 2", len(results))
	}
}

func TestContainerLauncher_GetPodIP(t *testing.T) {
	l := NewContainerLauncher()

	l.sessions["session-1"] = &containerSession{
		result:    &plugins.LaunchResult{SessionID: "session-1"},
		podIP:     "10.0.0.5",
		createdAt: time.Now(),
	}

	ip := l.GetPodIP("session-1")
	if ip != "10.0.0.5" {
		t.Errorf("GetPodIP() = %q, want %q", ip, "10.0.0.5")
	}
}

func TestContainerLauncher_GetPodIP_NotFound(t *testing.T) {
	l := NewContainerLauncher()

	ip := l.GetPodIP("nonexistent")
	if ip != "" {
		t.Errorf("GetPodIP() = %q, want empty string", ip)
	}
}

func TestContainerLauncher_UpdateSessionStatus(t *testing.T) {
	l := NewContainerLauncher()

	l.sessions["session-1"] = &containerSession{
		result: &plugins.LaunchResult{
			SessionID: "session-1",
			Status:    plugins.LaunchStatusCreating,
			Message:   "Creating",
		},
		createdAt: time.Now(),
	}

	l.updateSessionStatus("session-1", plugins.LaunchStatusFailed, "Pod failed")

	if l.sessions["session-1"].result.Status != plugins.LaunchStatusFailed {
		t.Errorf("status = %q, want %q", l.sessions["session-1"].result.Status, plugins.LaunchStatusFailed)
	}
	if l.sessions["session-1"].result.Message != "Pod failed" {
		t.Errorf("message = %q, want %q", l.sessions["session-1"].result.Message, "Pod failed")
	}
}

func TestContainerLauncher_UpdateSessionStatus_NonExistent(t *testing.T) {
	l := NewContainerLauncher()

	// Should not panic when session doesn't exist
	l.updateSessionStatus("nonexistent", plugins.LaunchStatusFailed, "fail")
}

func TestContainerLauncher_CleanupStaleSessions(t *testing.T) {
	l := NewContainerLauncher()
	l.sessionTimeout = 1 * time.Second

	// Add a stale session (created 2 seconds ago with 1s timeout)
	l.sessions["stale-session"] = &containerSession{
		result: &plugins.LaunchResult{
			SessionID: "stale-session",
			Status:    plugins.LaunchStatusRunning,
		},
		podName:   "stale-pod",
		userID:    "user-1",
		createdAt: time.Now().Add(-2 * time.Second),
	}

	// Add a fresh session
	l.sessions["fresh-session"] = &containerSession{
		result: &plugins.LaunchResult{
			SessionID: "fresh-session",
			Status:    plugins.LaunchStatusRunning,
		},
		podName:   "fresh-pod",
		userID:    "user-1",
		createdAt: time.Now(),
	}

	// Run cleanup (will fail on k8s.DeletePod but should still remove the session)
	l.cleanupStaleSessions()

	// Stale session should be removed
	if _, exists := l.sessions["stale-session"]; exists {
		t.Error("stale session should have been cleaned up")
	}

	// Fresh session should remain
	if _, exists := l.sessions["fresh-session"]; !exists {
		t.Error("fresh session should not have been cleaned up")
	}
}

func TestContainerLauncher_Terminate_NotFound(t *testing.T) {
	l := NewContainerLauncher()
	ctx := context.Background()

	err := l.Terminate(ctx, "nonexistent")
	if err != plugins.ErrResourceNotFound {
		t.Errorf("Terminate() error = %v, want %v", err, plugins.ErrResourceNotFound)
	}
}

func TestContainerLauncher_Terminate_RemovesSession(t *testing.T) {
	l := NewContainerLauncher()
	ctx := context.Background()

	l.sessions["session-1"] = &containerSession{
		result: &plugins.LaunchResult{
			SessionID: "session-1",
			Status:    plugins.LaunchStatusRunning,
		},
		podName:   "test-pod",
		userID:    "user-1",
		createdAt: time.Now(),
	}

	// Terminate will fail on k8s.DeletePod (no cluster) but should still remove session
	err := l.Terminate(ctx, "session-1")
	// The error from k8s.DeletePod is logged but not returned
	if err != nil {
		t.Fatalf("Terminate() error = %v", err)
	}

	// Session should be removed
	if _, exists := l.sessions["session-1"]; exists {
		t.Error("session should have been removed after Terminate()")
	}
}

func TestContainerLauncher_DefaultConstants(t *testing.T) {
	if DefaultSessionTimeout != 2*time.Hour {
		t.Errorf("DefaultSessionTimeout = %v, want %v", DefaultSessionTimeout, 2*time.Hour)
	}
	if DefaultCleanupInterval != 5*time.Minute {
		t.Errorf("DefaultCleanupInterval = %v, want %v", DefaultCleanupInterval, 5*time.Minute)
	}
	if DefaultPodReadyTimeout != 5*time.Minute {
		t.Errorf("DefaultPodReadyTimeout = %v, want %v", DefaultPodReadyTimeout, 5*time.Minute)
	}
}

func TestContainerLauncher_InterfaceCompliance(t *testing.T) {
	var _ plugins.LauncherPlugin = (*ContainerLauncher)(nil)
}
