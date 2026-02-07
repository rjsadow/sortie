package sessions

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rjsadow/launchpad/internal/db"
	"github.com/rjsadow/launchpad/internal/k8s"
	corev1 "k8s.io/api/core/v1"
)

const (
	// DefaultSessionTimeout is the default timeout for stale sessions
	DefaultSessionTimeout = 2 * time.Hour

	// DefaultCleanupInterval is the default interval for cleanup goroutine
	DefaultCleanupInterval = 5 * time.Minute

	// DefaultPodReadyTimeout is the default timeout for waiting for pod ready
	DefaultPodReadyTimeout = 2 * time.Minute
)

// ManagerConfig holds configuration for the session manager.
type ManagerConfig struct {
	SessionTimeout  time.Duration
	CleanupInterval time.Duration
	PodReadyTimeout time.Duration
}

// Manager handles session lifecycle
type Manager struct {
	db              *db.DB
	sessionTimeout  time.Duration
	cleanupInterval time.Duration
	podReadyTimeout time.Duration

	mu       sync.RWMutex
	stopCh   chan struct{}
	sessions map[string]*db.Session
}

// NewManager creates a new session manager with default configuration.
// Deprecated: Use NewManagerWithConfig for explicit configuration.
func NewManager(database *db.DB) *Manager {
	return NewManagerWithConfig(database, ManagerConfig{
		SessionTimeout:  DefaultSessionTimeout,
		CleanupInterval: DefaultCleanupInterval,
		PodReadyTimeout: DefaultPodReadyTimeout,
	})
}

// NewManagerWithConfig creates a new session manager with the given configuration.
func NewManagerWithConfig(database *db.DB, cfg ManagerConfig) *Manager {
	// Apply defaults for zero values
	if cfg.SessionTimeout == 0 {
		cfg.SessionTimeout = DefaultSessionTimeout
	}
	if cfg.CleanupInterval == 0 {
		cfg.CleanupInterval = DefaultCleanupInterval
	}
	if cfg.PodReadyTimeout == 0 {
		cfg.PodReadyTimeout = DefaultPodReadyTimeout
	}

	return &Manager{
		db:              database,
		sessionTimeout:  cfg.SessionTimeout,
		cleanupInterval: cfg.CleanupInterval,
		podReadyTimeout: cfg.PodReadyTimeout,
		stopCh:          make(chan struct{}),
		sessions:        make(map[string]*db.Session),
	}
}

// Start begins the background cleanup goroutine
func (m *Manager) Start() {
	go m.cleanupLoop()
	log.Printf("Session manager started (timeout: %v, cleanup interval: %v)", m.sessionTimeout, m.cleanupInterval)
}

// Stop stops the background cleanup goroutine
func (m *Manager) Stop() {
	close(m.stopCh)
}

// cleanupLoop periodically cleans up stale sessions
func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := m.cleanupStaleSessions(); err != nil {
				log.Printf("Error cleaning up stale sessions: %v", err)
			}
		case <-m.stopCh:
			return
		}
	}
}

// cleanupStaleSessions expires sessions that have been running too long
func (m *Manager) cleanupStaleSessions() error {
	sessions, err := m.db.GetStaleSessions(m.sessionTimeout)
	if err != nil {
		return fmt.Errorf("failed to get stale sessions: %w", err)
	}

	for _, session := range sessions {
		log.Printf("Expiring stale session: %s (pod: %s)", session.ID, session.PodName)
		if err := m.ExpireSession(context.Background(), session.ID); err != nil {
			log.Printf("Error expiring stale session %s: %v", session.ID, err)
		}
	}

	return nil
}

// CreateSession creates a new session for an application
func (m *Manager) CreateSession(ctx context.Context, req *CreateSessionRequest) (*db.Session, error) {
	// Get the application
	app, err := m.db.GetApp(req.AppID)
	if err != nil {
		return nil, fmt.Errorf("failed to get application: %w", err)
	}
	if app == nil {
		return nil, fmt.Errorf("application not found: %s", req.AppID)
	}
	if app.LaunchType != db.LaunchTypeContainer && app.LaunchType != db.LaunchTypeWebProxy {
		return nil, fmt.Errorf("application %s is not a container or web_proxy application", req.AppID)
	}
	if app.ContainerImage == "" {
		return nil, fmt.Errorf("application %s has no container image configured", req.AppID)
	}

	// Generate session ID
	sessionID := uuid.New().String()

	// Create pod configuration
	podConfig := k8s.DefaultPodConfig(sessionID, app.ID, app.Name, app.ContainerImage)
	podConfig.ContainerPort = app.ContainerPort
	podConfig.Args = app.ContainerArgs

	// Set screen resolution from client viewport if provided
	if req.ScreenWidth > 0 && req.ScreenHeight > 0 {
		podConfig.ScreenResolution = fmt.Sprintf("%dx%dx24", req.ScreenWidth, req.ScreenHeight)
		podConfig.ScreenWidth = req.ScreenWidth
		podConfig.ScreenHeight = req.ScreenHeight
	}

	// Apply app-specific resource limits if configured
	if app.ResourceLimits != nil {
		if app.ResourceLimits.CPURequest != "" {
			podConfig.CPURequest = app.ResourceLimits.CPURequest
		}
		if app.ResourceLimits.CPULimit != "" {
			podConfig.CPULimit = app.ResourceLimits.CPULimit
		}
		if app.ResourceLimits.MemoryRequest != "" {
			podConfig.MemoryRequest = app.ResourceLimits.MemoryRequest
		}
		if app.ResourceLimits.MemoryLimit != "" {
			podConfig.MemoryLimit = app.ResourceLimits.MemoryLimit
		}
	}

	// Build and create the pod based on launch type and OS type
	var pod *corev1.Pod
	switch app.LaunchType {
	case db.LaunchTypeContainer:
		if app.OsType == "windows" {
			pod = k8s.BuildWindowsPodSpec(podConfig)
		} else {
			pod = k8s.BuildPodSpec(podConfig)
		}
	case db.LaunchTypeWebProxy:
		pod = k8s.BuildWebProxyPodSpec(podConfig)
	default:
		return nil, fmt.Errorf("unsupported launch type: %s", app.LaunchType)
	}

	createdPod, err := k8s.CreatePod(ctx, pod)
	if err != nil {
		return nil, fmt.Errorf("failed to create pod: %w", err)
	}

	// Create session in database
	now := time.Now()
	session := &db.Session{
		ID:          sessionID,
		UserID:      req.UserID,
		AppID:       req.AppID,
		PodName:     createdPod.Name,
		Status:      db.SessionStatusCreating,
		IdleTimeout: req.IdleTimeout,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := m.db.CreateSession(*session); err != nil {
		// Try to clean up the pod
		k8s.DeletePod(ctx, createdPod.Name)
		return nil, fmt.Errorf("failed to create session in database: %w", err)
	}

	// Cache the session
	m.mu.Lock()
	m.sessions[sessionID] = session
	m.mu.Unlock()

	// Start goroutine to wait for pod ready and update session
	go m.waitForPodReady(sessionID, createdPod.Name)

	return session, nil
}

// waitForPodReady waits for the pod to be ready and updates the session
func (m *Manager) waitForPodReady(sessionID, podName string) {
	ctx, cancel := context.WithTimeout(context.Background(), m.podReadyTimeout)
	defer cancel()

	// Wait for pod to be ready
	if err := k8s.WaitForPodReady(ctx, podName, m.podReadyTimeout); err != nil {
		LogTransition(sessionID, db.SessionStatusCreating, db.SessionStatusFailed, fmt.Sprintf("pod failed to become ready: %v", err))
		m.db.UpdateSessionStatus(sessionID, db.SessionStatusFailed)
		if delErr := k8s.DeletePod(context.Background(), podName); delErr != nil {
			log.Printf("Failed to delete pod %s after timeout: %v", podName, delErr)
		}
		return
	}

	// Get pod IP
	podIP, err := k8s.GetPodIP(ctx, podName)
	if err != nil {
		LogTransition(sessionID, db.SessionStatusCreating, db.SessionStatusFailed, fmt.Sprintf("failed to get pod IP: %v", err))
		m.db.UpdateSessionStatus(sessionID, db.SessionStatusFailed)
		if delErr := k8s.DeletePod(context.Background(), podName); delErr != nil {
			log.Printf("Failed to delete pod %s after IP lookup failure: %v", podName, delErr)
		}
		return
	}

	// Update session with pod IP and running status in a single operation
	LogTransition(sessionID, db.SessionStatusCreating, db.SessionStatusRunning, "pod ready")
	if err := m.db.UpdateSessionPodIPAndStatus(sessionID, podIP, db.SessionStatusRunning); err != nil {
		log.Printf("Failed to update session for %s: %v", sessionID, err)
	}

	// Update cache
	m.mu.Lock()
	if session, ok := m.sessions[sessionID]; ok {
		session.PodIP = podIP
		session.Status = db.SessionStatusRunning
	}
	m.mu.Unlock()
}

// GetSession returns a session by ID
func (m *Manager) GetSession(ctx context.Context, sessionID string) (*db.Session, error) {
	// Check cache first
	m.mu.RLock()
	if session, ok := m.sessions[sessionID]; ok {
		m.mu.RUnlock()
		return session, nil
	}
	m.mu.RUnlock()

	// Load from database
	session, err := m.db.GetSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	return session, nil
}

// ListSessions returns all active sessions
func (m *Manager) ListSessions(ctx context.Context) ([]db.Session, error) {
	sessions, err := m.db.ListSessions()
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	return sessions, nil
}

// ListSessionsByUser returns all sessions for a specific user
func (m *Manager) ListSessionsByUser(ctx context.Context, userID string) ([]db.Session, error) {
	sessions, err := m.db.ListSessionsByUser(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions by user: %w", err)
	}
	return sessions, nil
}

// StopSession stops a running session, deleting the pod but keeping the session
// record so it can be restarted later.
func (m *Manager) StopSession(ctx context.Context, sessionID string) error {
	session, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	if err := ValidateAndLogTransition(sessionID, session.Status, db.SessionStatusStopped, "user stopped"); err != nil {
		return err
	}

	// Delete the pod
	if err := k8s.DeletePod(ctx, session.PodName); err != nil {
		log.Printf("Warning: failed to delete pod %s: %v", session.PodName, err)
	}

	// Update status to stopped
	if err := m.db.UpdateSessionStatus(sessionID, db.SessionStatusStopped); err != nil {
		return fmt.Errorf("failed to update session status: %w", err)
	}

	// Update cache
	m.mu.Lock()
	if s, ok := m.sessions[sessionID]; ok {
		s.Status = db.SessionStatusStopped
		s.PodIP = ""
	}
	m.mu.Unlock()

	return nil
}

// RestartSession recreates the pod for a stopped session.
func (m *Manager) RestartSession(ctx context.Context, sessionID string) (*db.Session, error) {
	session, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	if err := ValidateAndLogTransition(sessionID, session.Status, db.SessionStatusCreating, "user restarted"); err != nil {
		return nil, fmt.Errorf("session must be stopped to restart (current status: %s)", session.Status)
	}

	// Get the application to rebuild the pod
	app, err := m.db.GetApp(session.AppID)
	if err != nil {
		return nil, fmt.Errorf("failed to get application: %w", err)
	}
	if app == nil {
		return nil, fmt.Errorf("application not found: %s", session.AppID)
	}

	// Create pod configuration using the existing session ID
	podConfig := k8s.DefaultPodConfig(sessionID, app.ID, app.Name, app.ContainerImage)
	podConfig.ContainerPort = app.ContainerPort
	podConfig.Args = app.ContainerArgs

	if app.ResourceLimits != nil {
		if app.ResourceLimits.CPURequest != "" {
			podConfig.CPURequest = app.ResourceLimits.CPURequest
		}
		if app.ResourceLimits.CPULimit != "" {
			podConfig.CPULimit = app.ResourceLimits.CPULimit
		}
		if app.ResourceLimits.MemoryRequest != "" {
			podConfig.MemoryRequest = app.ResourceLimits.MemoryRequest
		}
		if app.ResourceLimits.MemoryLimit != "" {
			podConfig.MemoryLimit = app.ResourceLimits.MemoryLimit
		}
	}

	// Build and create the pod
	var pod *corev1.Pod
	switch app.LaunchType {
	case db.LaunchTypeContainer:
		if app.OsType == "windows" {
			pod = k8s.BuildWindowsPodSpec(podConfig)
		} else {
			pod = k8s.BuildPodSpec(podConfig)
		}
	case db.LaunchTypeWebProxy:
		pod = k8s.BuildWebProxyPodSpec(podConfig)
	default:
		return nil, fmt.Errorf("unsupported launch type: %s", app.LaunchType)
	}

	createdPod, err := k8s.CreatePod(ctx, pod)
	if err != nil {
		return nil, fmt.Errorf("failed to create pod: %w", err)
	}

	// Update session in database with new pod name and creating status
	if err := m.db.UpdateSessionRestart(sessionID, createdPod.Name); err != nil {
		k8s.DeletePod(ctx, createdPod.Name)
		return nil, fmt.Errorf("failed to update session in database: %w", err)
	}

	// Update cache
	m.mu.Lock()
	session.PodName = createdPod.Name
	session.PodIP = ""
	session.Status = db.SessionStatusCreating
	m.sessions[sessionID] = session
	m.mu.Unlock()

	// Wait for pod ready in background
	go m.waitForPodReady(sessionID, createdPod.Name)

	return session, nil
}

// TerminateSession stops a session (user-initiated) and deletes the pod
func (m *Manager) TerminateSession(ctx context.Context, sessionID string) error {
	return m.terminateWithStatus(ctx, sessionID, db.SessionStatusStopped, "user requested")
}

// ExpireSession expires a session (timeout-initiated) and deletes the pod
func (m *Manager) ExpireSession(ctx context.Context, sessionID string) error {
	return m.terminateWithStatus(ctx, sessionID, db.SessionStatusExpired, "session timeout")
}

// terminateWithStatus handles session termination with the specified final status
func (m *Manager) terminateWithStatus(ctx context.Context, sessionID string, finalStatus db.SessionStatus, reason string) error {
	// Get session
	session, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// Validate state transition
	if err := ValidateAndLogTransition(sessionID, session.Status, finalStatus, reason); err != nil {
		// If already in a terminal state, just log and return success
		if IsTerminalState(session.Status) {
			log.Printf("Session %s already in terminal state: %s", sessionID, session.Status)
			return nil
		}
		return err
	}

	// Delete the pod
	if err := k8s.DeletePod(ctx, session.PodName); err != nil {
		log.Printf("Warning: failed to delete pod %s: %v", session.PodName, err)
	}

	// Update status to final state
	if err := m.db.UpdateSessionStatus(sessionID, finalStatus); err != nil {
		return fmt.Errorf("failed to update session status: %w", err)
	}

	// Remove from cache
	m.mu.Lock()
	delete(m.sessions, sessionID)
	m.mu.Unlock()

	return nil
}

// GetSessionWebSocketURL returns the WebSocket URL for connecting to a session
func (m *Manager) GetSessionWebSocketURL(session *db.Session) string {
	if session.PodIP == "" || session.Status != db.SessionStatusRunning {
		return ""
	}
	// The WebSocket proxy endpoint on the server
	return fmt.Sprintf("/ws/sessions/%s", session.ID)
}

// GetPodWebSocketEndpoint returns the internal pod WebSocket endpoint
func (m *Manager) GetPodWebSocketEndpoint(session *db.Session) string {
	if session.PodIP == "" {
		return ""
	}

	// Determine websocket port and path based on container image
	// jlesage images have built-in VNC on port 5800 with /websockify path
	port := 6080
	path := ""
	if app, err := m.db.GetApp(session.AppID); err == nil && app != nil {
		if strings.HasPrefix(app.ContainerImage, "jlesage/") {
			port = 5800
			path = "/websockify"
		}
	}

	return fmt.Sprintf("ws://%s:%d%s", session.PodIP, port, path)
}

// GetPodProxyEndpoint returns the internal HTTP endpoint for web_proxy sessions
func (m *Manager) GetPodProxyEndpoint(session *db.Session) string {
	if session.PodIP == "" {
		return ""
	}

	// Get the container port from the app configuration
	port := 8080 // default
	scheme := "http"
	if app, err := m.db.GetApp(session.AppID); err == nil && app != nil {
		if app.ContainerPort > 0 {
			port = app.ContainerPort
		}
		// Use HTTPS for common HTTPS ports (code-server uses 8443)
		if port == 443 || port == 8443 {
			scheme = "https"
		}
	}

	return fmt.Sprintf("%s://%s:%d", scheme, session.PodIP, port)
}

// GetSessionProxyURL returns the proxy URL for web_proxy sessions
func (m *Manager) GetSessionProxyURL(session *db.Session) string {
	if session.PodIP == "" || session.Status != db.SessionStatusRunning {
		return ""
	}
	// The HTTP proxy endpoint on the server
	return fmt.Sprintf("/api/sessions/%s/proxy/", session.ID)
}

// GetSessionGuacWebSocketURL returns the Guacamole WebSocket URL for Windows RDP sessions
func (m *Manager) GetSessionGuacWebSocketURL(session *db.Session) string {
	if session.PodIP == "" || session.Status != db.SessionStatusRunning {
		return ""
	}
	return fmt.Sprintf("/ws/guac/sessions/%s", session.ID)
}

// IsWindowsApp checks if the given app is a Windows application
func (m *Manager) IsWindowsApp(appID string) bool {
	app, err := m.db.GetApp(appID)
	if err != nil || app == nil {
		return false
	}
	return app.OsType == "windows"
}
