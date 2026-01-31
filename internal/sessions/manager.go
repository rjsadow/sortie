package sessions

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rjsadow/launchpad/internal/db"
	"github.com/rjsadow/launchpad/internal/k8s"
)

const (
	// DefaultSessionTimeout is the default timeout for stale sessions
	DefaultSessionTimeout = 2 * time.Hour

	// DefaultCleanupInterval is the default interval for cleanup goroutine
	DefaultCleanupInterval = 5 * time.Minute

	// DefaultPodReadyTimeout is the default timeout for waiting for pod ready
	DefaultPodReadyTimeout = 2 * time.Minute
)

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

// NewManager creates a new session manager
func NewManager(database *db.DB) *Manager {
	timeout := DefaultSessionTimeout
	if envTimeout := os.Getenv("SESSION_TIMEOUT"); envTimeout != "" {
		if minutes, err := strconv.Atoi(envTimeout); err == nil {
			timeout = time.Duration(minutes) * time.Minute
		}
	}

	cleanupInterval := DefaultCleanupInterval
	if envInterval := os.Getenv("SESSION_CLEANUP_INTERVAL"); envInterval != "" {
		if minutes, err := strconv.Atoi(envInterval); err == nil {
			cleanupInterval = time.Duration(minutes) * time.Minute
		}
	}

	podReadyTimeout := DefaultPodReadyTimeout
	if envTimeout := os.Getenv("POD_READY_TIMEOUT"); envTimeout != "" {
		if seconds, err := strconv.Atoi(envTimeout); err == nil {
			podReadyTimeout = time.Duration(seconds) * time.Second
		}
	}

	return &Manager{
		db:              database,
		sessionTimeout:  timeout,
		cleanupInterval: cleanupInterval,
		podReadyTimeout: podReadyTimeout,
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

// cleanupStaleSessions terminates sessions that have been running too long
func (m *Manager) cleanupStaleSessions() error {
	sessions, err := m.db.GetStaleSessions(m.sessionTimeout)
	if err != nil {
		return fmt.Errorf("failed to get stale sessions: %w", err)
	}

	for _, session := range sessions {
		log.Printf("Cleaning up stale session: %s (pod: %s)", session.ID, session.PodName)
		if err := m.TerminateSession(context.Background(), session.ID); err != nil {
			log.Printf("Error terminating stale session %s: %v", session.ID, err)
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
	if app.LaunchType != db.LaunchTypeContainer {
		return nil, fmt.Errorf("application %s is not a container application", req.AppID)
	}
	if app.ContainerImage == "" {
		return nil, fmt.Errorf("application %s has no container image configured", req.AppID)
	}

	// Generate session ID
	sessionID := uuid.New().String()

	// Create pod configuration
	podConfig := k8s.DefaultPodConfig(sessionID, app.ID, app.Name, app.ContainerImage)

	// Build and create the pod
	pod := k8s.BuildPodSpec(podConfig)
	createdPod, err := k8s.CreatePod(ctx, pod)
	if err != nil {
		return nil, fmt.Errorf("failed to create pod: %w", err)
	}

	// Create session in database
	now := time.Now()
	session := &db.Session{
		ID:        sessionID,
		UserID:    req.UserID,
		AppID:     req.AppID,
		PodName:   createdPod.Name,
		Status:    db.SessionStatusCreating,
		CreatedAt: now,
		UpdatedAt: now,
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
		log.Printf("Pod %s failed to become ready: %v", podName, err)
		m.db.UpdateSessionStatus(sessionID, db.SessionStatusFailed)
		return
	}

	// Get pod IP
	podIP, err := k8s.GetPodIP(ctx, podName)
	if err != nil {
		log.Printf("Failed to get pod IP for %s: %v", podName, err)
		m.db.UpdateSessionStatus(sessionID, db.SessionStatusFailed)
		return
	}

	// Update session with pod IP and running status
	if err := m.db.UpdateSessionPodIP(sessionID, podIP); err != nil {
		log.Printf("Failed to update pod IP for session %s: %v", sessionID, err)
	}
	if err := m.db.UpdateSessionStatus(sessionID, db.SessionStatusRunning); err != nil {
		log.Printf("Failed to update session status for %s: %v", sessionID, err)
	}

	// Update cache
	m.mu.Lock()
	if session, ok := m.sessions[sessionID]; ok {
		session.PodIP = podIP
		session.Status = db.SessionStatusRunning
	}
	m.mu.Unlock()

	log.Printf("Session %s is now running (pod: %s, IP: %s)", sessionID, podName, podIP)
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

// TerminateSession terminates a session and deletes the pod
func (m *Manager) TerminateSession(ctx context.Context, sessionID string) error {
	// Get session
	session, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// Update status to terminating
	if err := m.db.UpdateSessionStatus(sessionID, db.SessionStatusTerminating); err != nil {
		log.Printf("Failed to update session status to terminating: %v", err)
	}

	// Delete the pod
	if err := k8s.DeletePod(ctx, session.PodName); err != nil {
		log.Printf("Warning: failed to delete pod %s: %v", session.PodName, err)
	}

	// Update status to terminated
	if err := m.db.UpdateSessionStatus(sessionID, db.SessionStatusTerminated); err != nil {
		return fmt.Errorf("failed to update session status: %w", err)
	}

	// Remove from cache
	m.mu.Lock()
	delete(m.sessions, sessionID)
	m.mu.Unlock()

	log.Printf("Session %s terminated (pod: %s)", sessionID, session.PodName)
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
	return fmt.Sprintf("ws://%s:6080", session.PodIP)
}
