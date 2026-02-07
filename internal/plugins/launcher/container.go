package launcher

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rjsadow/launchpad/internal/k8s"
	"github.com/rjsadow/launchpad/internal/plugins"
)

const (
	DefaultSessionTimeout  = 2 * time.Hour
	DefaultCleanupInterval = 5 * time.Minute
	DefaultPodReadyTimeout = 2 * time.Minute
)

// ContainerLauncher implements LauncherPlugin for Kubernetes container-based launches.
// It creates pods with VNC sidecars for interactive desktop applications.
//
// NOTE: This plugin maintains session state in-memory and is NOT horizontally scalable.
// For multi-replica deployments, use the sessions.Manager which stores all state in the
// database. This plugin is intended for single-instance or plugin-system use only.
type ContainerLauncher struct {
	mu              sync.RWMutex
	sessions        map[string]*containerSession
	config          map[string]string
	sessionTimeout  time.Duration
	cleanupInterval time.Duration
	podReadyTimeout time.Duration
	stopCh          chan struct{}
}

type containerSession struct {
	result    *plugins.LaunchResult
	podName   string
	podIP     string
	userID    string
	appID     string
	createdAt time.Time
}

func init() {
	plugins.RegisterGlobal(plugins.PluginTypeLauncher, "container", func() plugins.Plugin {
		return NewContainerLauncher()
	})
}

// NewContainerLauncher creates a new container launcher plugin.
func NewContainerLauncher() *ContainerLauncher {
	return &ContainerLauncher{
		sessions:        make(map[string]*containerSession),
		sessionTimeout:  DefaultSessionTimeout,
		cleanupInterval: DefaultCleanupInterval,
		podReadyTimeout: DefaultPodReadyTimeout,
		stopCh:          make(chan struct{}),
	}
}

// Name returns the plugin name.
func (l *ContainerLauncher) Name() string {
	return "container"
}

// Type returns the plugin type.
func (l *ContainerLauncher) Type() plugins.PluginType {
	return plugins.PluginTypeLauncher
}

// Version returns the plugin version.
func (l *ContainerLauncher) Version() string {
	return "1.0.0"
}

// Description returns a human-readable description.
func (l *ContainerLauncher) Description() string {
	return "Kubernetes container launcher with VNC sidecar for interactive desktop applications"
}

// Initialize sets up the plugin with configuration.
func (l *ContainerLauncher) Initialize(ctx context.Context, config map[string]string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.config = config

	// Parse configuration
	if timeout, ok := config["session_timeout"]; ok {
		if d, err := time.ParseDuration(timeout); err == nil {
			l.sessionTimeout = d
		}
	}

	if interval, ok := config["cleanup_interval"]; ok {
		if d, err := time.ParseDuration(interval); err == nil {
			l.cleanupInterval = d
		}
	}

	if timeout, ok := config["pod_ready_timeout"]; ok {
		if d, err := time.ParseDuration(timeout); err == nil {
			l.podReadyTimeout = d
		}
	}

	// Configure Kubernetes client if needed
	namespace := config["namespace"]
	kubeconfig := config["kubeconfig"]
	vncImage := config["vnc_sidecar_image"]
	if namespace != "" || kubeconfig != "" || vncImage != "" {
		k8s.Configure(namespace, kubeconfig, vncImage)
	}

	// Start cleanup goroutine
	go l.cleanupLoop()

	log.Printf("Container launcher initialized (timeout: %v, cleanup: %v)", l.sessionTimeout, l.cleanupInterval)
	return nil
}

// Healthy returns true if the plugin is operational.
func (l *ContainerLauncher) Healthy(ctx context.Context) bool {
	// Check if we can access Kubernetes
	_, err := k8s.GetClient()
	return err == nil
}

// Close releases resources.
func (l *ContainerLauncher) Close() error {
	close(l.stopCh)
	return nil
}

// SupportedTypes returns the launch types this launcher supports.
func (l *ContainerLauncher) SupportedTypes() []plugins.LaunchType {
	return []plugins.LaunchType{plugins.LaunchTypeContainer}
}

// Launch starts a container and returns the result.
func (l *ContainerLauncher) Launch(ctx context.Context, req *plugins.LaunchRequest) (*plugins.LaunchResult, error) {
	if req.LaunchType != plugins.LaunchTypeContainer {
		return nil, fmt.Errorf("unsupported launch type: %s", req.LaunchType)
	}

	if req.ContainerImage == "" {
		return nil, fmt.Errorf("container image is required")
	}

	// Generate session ID
	sessionID := uuid.New().String()

	// Create pod configuration
	podConfig := k8s.DefaultPodConfig(sessionID, req.AppID, req.AppName, req.ContainerImage)

	// Apply resource limits if provided
	if req.ResourceLimits != nil {
		if req.ResourceLimits.CPURequest != "" {
			podConfig.CPURequest = req.ResourceLimits.CPURequest
		}
		if req.ResourceLimits.CPULimit != "" {
			podConfig.CPULimit = req.ResourceLimits.CPULimit
		}
		if req.ResourceLimits.MemoryRequest != "" {
			podConfig.MemoryRequest = req.ResourceLimits.MemoryRequest
		}
		if req.ResourceLimits.MemoryLimit != "" {
			podConfig.MemoryLimit = req.ResourceLimits.MemoryLimit
		}
	}

	// Build and create the pod
	pod := k8s.BuildPodSpec(podConfig)
	createdPod, err := k8s.CreatePod(ctx, pod)
	if err != nil {
		return nil, fmt.Errorf("failed to create pod: %w", err)
	}

	// Create result
	result := &plugins.LaunchResult{
		SessionID: sessionID,
		Status:    plugins.LaunchStatusCreating,
		Message:   "Creating container",
		Metadata: map[string]string{
			"pod_name": createdPod.Name,
		},
	}

	// Store session
	session := &containerSession{
		result:    result,
		podName:   createdPod.Name,
		userID:    req.UserID,
		appID:     req.AppID,
		createdAt: time.Now(),
	}

	l.mu.Lock()
	l.sessions[sessionID] = session
	l.mu.Unlock()

	// Start goroutine to wait for pod ready
	go l.waitForPodReady(sessionID, createdPod.Name)

	return result, nil
}

// waitForPodReady waits for the pod to be ready and updates the session.
func (l *ContainerLauncher) waitForPodReady(sessionID, podName string) {
	ctx, cancel := context.WithTimeout(context.Background(), l.podReadyTimeout)
	defer cancel()

	// Wait for pod to be ready
	if err := k8s.WaitForPodReady(ctx, podName, l.podReadyTimeout); err != nil {
		l.updateSessionStatus(sessionID, plugins.LaunchStatusFailed, fmt.Sprintf("Pod failed to become ready: %v", err))
		return
	}

	// Get pod IP
	podIP, err := k8s.GetPodIP(ctx, podName)
	if err != nil {
		l.updateSessionStatus(sessionID, plugins.LaunchStatusFailed, fmt.Sprintf("Failed to get pod IP: %v", err))
		return
	}

	// Update session with running status
	l.mu.Lock()
	if session, ok := l.sessions[sessionID]; ok {
		session.podIP = podIP
		session.result.Status = plugins.LaunchStatusRunning
		session.result.WebSocketURL = fmt.Sprintf("/ws/sessions/%s", sessionID)
		session.result.Message = "Container running"
	}
	l.mu.Unlock()

	log.Printf("Container session %s is running (pod: %s, ip: %s)", sessionID, podName, podIP)
}

func (l *ContainerLauncher) updateSessionStatus(sessionID string, status plugins.LaunchStatus, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if session, ok := l.sessions[sessionID]; ok {
		session.result.Status = status
		session.result.Message = message
	}
}

// GetStatus returns the current status of a launch session.
func (l *ContainerLauncher) GetStatus(ctx context.Context, sessionID string) (*plugins.LaunchResult, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	session, exists := l.sessions[sessionID]
	if !exists {
		return nil, plugins.ErrResourceNotFound
	}

	return session.result, nil
}

// Terminate stops a running container session.
func (l *ContainerLauncher) Terminate(ctx context.Context, sessionID string) error {
	l.mu.Lock()
	session, exists := l.sessions[sessionID]
	if !exists {
		l.mu.Unlock()
		return plugins.ErrResourceNotFound
	}
	podName := session.podName
	l.mu.Unlock()

	// Delete the pod
	if err := k8s.DeletePod(ctx, podName); err != nil {
		log.Printf("Warning: failed to delete pod %s: %v", podName, err)
	}

	// Update session status
	l.mu.Lock()
	if session, ok := l.sessions[sessionID]; ok {
		session.result.Status = plugins.LaunchStatusStopped
		session.result.Message = "Session terminated"
	}
	delete(l.sessions, sessionID)
	l.mu.Unlock()

	return nil
}

// ListSessions returns all active sessions for a user.
func (l *ContainerLauncher) ListSessions(ctx context.Context, userID string) ([]*plugins.LaunchResult, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var results []*plugins.LaunchResult
	for _, session := range l.sessions {
		if userID == "" || session.userID == userID {
			results = append(results, session.result)
		}
	}

	return results, nil
}

// GetPodIP returns the pod IP for a session.
func (l *ContainerLauncher) GetPodIP(sessionID string) string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if session, ok := l.sessions[sessionID]; ok {
		return session.podIP
	}
	return ""
}

// cleanupLoop periodically cleans up stale sessions.
func (l *ContainerLauncher) cleanupLoop() {
	ticker := time.NewTicker(l.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.cleanupStaleSessions()
		case <-l.stopCh:
			return
		}
	}
}

func (l *ContainerLauncher) cleanupStaleSessions() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	for sessionID, session := range l.sessions {
		if now.Sub(session.createdAt) > l.sessionTimeout {
			log.Printf("Expiring stale session: %s (pod: %s)", sessionID, session.podName)

			// Delete the pod
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := k8s.DeletePod(ctx, session.podName); err != nil {
				log.Printf("Warning: failed to delete pod %s: %v", session.podName, err)
			}
			cancel()

			session.result.Status = plugins.LaunchStatusExpired
			session.result.Message = "Session expired"
			delete(l.sessions, sessionID)
		}
	}
}

// Verify interface compliance
var _ plugins.LauncherPlugin = (*ContainerLauncher)(nil)
