package sessions

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rjsadow/launchpad/internal/db"
	"github.com/rjsadow/launchpad/internal/runner"
)

const (
	// DefaultSessionTimeout is the default timeout for stale sessions
	DefaultSessionTimeout = 2 * time.Hour

	// DefaultCleanupInterval is the default interval for cleanup goroutine
	DefaultCleanupInterval = 5 * time.Minute

	// DefaultPodReadyTimeout is the default timeout for waiting for pod ready.
	// Set to 5 minutes to accommodate large container image pulls.
	DefaultPodReadyTimeout = 5 * time.Minute
)

// ManagerConfig holds configuration for the session manager.
type ManagerConfig struct {
	SessionTimeout  time.Duration
	CleanupInterval time.Duration
	PodReadyTimeout time.Duration

	// Resource quota settings
	MaxSessionsPerUser int    // Max concurrent sessions per user (0 = unlimited)
	MaxGlobalSessions  int    // Max concurrent sessions globally (0 = unlimited)
	DefaultCPURequest  string // Default CPU request for new sessions
	DefaultCPULimit    string // Default CPU limit for new sessions
	DefaultMemRequest  string // Default memory request for new sessions
	DefaultMemLimit    string // Default memory limit for new sessions

	// Session recording
	Recorder SessionRecorder // Optional recorder for lifecycle events (nil = noop)

	// Session queueing (when global limit is reached)
	QueueMaxSize      int           // Max queued requests (0 = no queueing, reject immediately)
	QueueTimeout      time.Duration // Per-request queue wait timeout
	QueuePollInterval time.Duration // Capacity check interval

	// Runner is the workload orchestration backend (nil = noop/tests only)
	Runner runner.Runner
}

// Manager handles session lifecycle.
// All session state is stored in the database for horizontal scalability.
// Multiple replicas can share the same database and operate independently.
type Manager struct {
	db              *db.DB
	runner          runner.Runner
	sessionTimeout  time.Duration
	cleanupInterval time.Duration
	podReadyTimeout time.Duration

	// Resource quota settings
	maxSessionsPerUser int
	maxGlobalSessions  int
	defaultCPURequest  string
	defaultCPULimit    string
	defaultMemRequest  string
	defaultMemLimit    string

	// Session recording
	recorder SessionRecorder

	// Session queueing
	queue *SessionQueue

	stopCh chan struct{}
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

	recorder := cfg.Recorder
	if recorder == nil {
		recorder = &NoopRecorder{}
	}

	m := &Manager{
		db:                 database,
		runner:             cfg.Runner,
		sessionTimeout:     cfg.SessionTimeout,
		cleanupInterval:    cfg.CleanupInterval,
		podReadyTimeout:    cfg.PodReadyTimeout,
		maxSessionsPerUser: cfg.MaxSessionsPerUser,
		maxGlobalSessions:  cfg.MaxGlobalSessions,
		defaultCPURequest:  cfg.DefaultCPURequest,
		defaultCPULimit:    cfg.DefaultCPULimit,
		defaultMemRequest:  cfg.DefaultMemRequest,
		defaultMemLimit:    cfg.DefaultMemLimit,
		recorder:           recorder,
		stopCh:             make(chan struct{}),
	}

	// Initialize session queue if configured
	if cfg.QueueMaxSize > 0 && cfg.MaxGlobalSessions > 0 {
		m.queue = NewSessionQueue(QueueConfig{
			MaxSize:      cfg.QueueMaxSize,
			Timeout:      cfg.QueueTimeout,
			PollInterval: cfg.QueuePollInterval,
		}, func() bool {
			count, err := m.db.CountActiveSessions()
			if err != nil {
				return false
			}
			return count < m.maxGlobalSessions
		})
		log.Printf("Session queue enabled (max size: %d, timeout: %v)", cfg.QueueMaxSize, cfg.QueueTimeout)
	}

	return m
}

// emitEvent sends a session lifecycle event to the recorder.
func (m *Manager) emitEvent(ctx context.Context, event SessionEvent, session *db.Session, reason string) {
	m.recorder.OnEvent(ctx, SessionEventData{
		SessionID: session.ID,
		UserID:    session.UserID,
		AppID:     session.AppID,
		Event:     event,
		Timestamp: time.Now(),
		Reason:    reason,
	})
}

// emitEventByID emits a lifecycle event by looking up the session from the database.
// Used in goroutines where the session object may not be readily available.
func (m *Manager) emitEventByID(sessionID string, event SessionEvent, reason string) {
	session, err := m.db.GetSession(sessionID)
	if err != nil || session == nil {
		log.Printf("Warning: could not emit %s event for session %s: session not found", event, sessionID)
		return
	}
	m.emitEvent(context.Background(), event, session, reason)
}

// Start begins the background cleanup goroutine
func (m *Manager) Start() {
	go m.cleanupLoop()
	log.Printf("Session manager started (timeout: %v, cleanup interval: %v)", m.sessionTimeout, m.cleanupInterval)
}

// Stop stops the background cleanup goroutine and session queue.
func (m *Manager) Stop() {
	close(m.stopCh)
	if m.queue != nil {
		m.queue.Stop()
	}
}

// Queue returns the session queue (may be nil if queueing is disabled).
func (m *Manager) Queue() *SessionQueue {
	return m.queue
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

// checkQuotas verifies that creating a new session for the given user would not exceed quotas.
func (m *Manager) checkQuotas(userID string) error {
	return m.checkQuotasWithTenant(userID, "")
}

// checkQuotasWithTenant verifies quotas including tenant-level limits.
func (m *Manager) checkQuotasWithTenant(userID, tenantID string) error {
	// Check per-user session limit
	if m.maxSessionsPerUser > 0 {
		count, err := m.db.CountActiveSessionsByUser(userID)
		if err != nil {
			return fmt.Errorf("failed to check user session count: %w", err)
		}
		if count >= m.maxSessionsPerUser {
			return &QuotaExceededError{
				Reason: fmt.Sprintf("user %s has %d active sessions (max %d)", userID, count, m.maxSessionsPerUser),
			}
		}
	}

	// Check tenant session limit
	if tenantID != "" {
		tenant, err := m.db.GetTenant(tenantID)
		if err != nil {
			return fmt.Errorf("failed to get tenant: %w", err)
		}
		if tenant != nil && tenant.Quotas.MaxTotalSessions > 0 {
			count, err := m.db.CountActiveSessionsByTenant(tenantID)
			if err != nil {
				return fmt.Errorf("failed to check tenant session count: %w", err)
			}
			if count >= tenant.Quotas.MaxTotalSessions {
				return &QuotaExceededError{
					Reason: fmt.Sprintf("tenant %s session limit reached (%d/%d)", tenant.Name, count, tenant.Quotas.MaxTotalSessions),
				}
			}
		}
		// Check tenant-level per-user limit (overrides global if set)
		if tenant != nil && tenant.Quotas.MaxSessionsPerUser > 0 {
			count, err := m.db.CountActiveSessionsByUser(userID)
			if err != nil {
				return fmt.Errorf("failed to check user session count: %w", err)
			}
			if count >= tenant.Quotas.MaxSessionsPerUser {
				return &QuotaExceededError{
					Reason: fmt.Sprintf("tenant per-user session limit reached (%d/%d)", count, tenant.Quotas.MaxSessionsPerUser),
				}
			}
		}
	}

	// Check global session limit
	if m.maxGlobalSessions > 0 {
		count, err := m.db.CountActiveSessions()
		if err != nil {
			return fmt.Errorf("failed to check global session count: %w", err)
		}
		if count >= m.maxGlobalSessions {
			return &QuotaExceededError{
				Reason: fmt.Sprintf("global session limit reached (%d/%d)", count, m.maxGlobalSessions),
			}
		}
	}

	return nil
}

// applyDefaultResourceLimits sets default CPU/memory on a workload config when no app-specific limits are configured.
func (m *Manager) applyDefaultResourceLimits(wc *runner.WorkloadConfig, app *db.Application) {
	// App-specific limits take priority
	if app.ResourceLimits != nil {
		if app.ResourceLimits.CPURequest != "" {
			wc.CPURequest = app.ResourceLimits.CPURequest
		}
		if app.ResourceLimits.CPULimit != "" {
			wc.CPULimit = app.ResourceLimits.CPULimit
		}
		if app.ResourceLimits.MemoryRequest != "" {
			wc.MemoryRequest = app.ResourceLimits.MemoryRequest
		}
		if app.ResourceLimits.MemoryLimit != "" {
			wc.MemoryLimit = app.ResourceLimits.MemoryLimit
		}
		return
	}

	// Apply global defaults from config
	if m.defaultCPURequest != "" {
		wc.CPURequest = m.defaultCPURequest
	}
	if m.defaultCPULimit != "" {
		wc.CPULimit = m.defaultCPULimit
	}
	if m.defaultMemRequest != "" {
		wc.MemoryRequest = m.defaultMemRequest
	}
	if m.defaultMemLimit != "" {
		wc.MemoryLimit = m.defaultMemLimit
	}
}

// buildWorkloadConfig creates a WorkloadConfig from an app and session ID.
func (m *Manager) buildWorkloadConfig(sessionID string, app *db.Application) *runner.WorkloadConfig {
	return &runner.WorkloadConfig{
		SessionID:      sessionID,
		AppID:          app.ID,
		AppName:        app.Name,
		ContainerImage: app.ContainerImage,
		ContainerPort:  app.ContainerPort,
		Args:           app.ContainerArgs,
		LaunchType:     string(app.LaunchType),
		OsType:         app.OsType,
	}
}

// GetQuotaStatus returns current quota usage information for a user.
func (m *Manager) GetQuotaStatus(userID string) (*QuotaStatus, error) {
	return m.GetQuotaStatusWithTenant(userID, "")
}

// GetQuotaStatusWithTenant returns quota usage including tenant-level quotas.
func (m *Manager) GetQuotaStatusWithTenant(userID, tenantID string) (*QuotaStatus, error) {
	userCount, err := m.db.CountActiveSessionsByUser(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to count user sessions: %w", err)
	}

	globalCount, err := m.db.CountActiveSessions()
	if err != nil {
		return nil, fmt.Errorf("failed to count global sessions: %w", err)
	}

	status := &QuotaStatus{
		UserSessions:       userCount,
		MaxSessionsPerUser: m.maxSessionsPerUser,
		GlobalSessions:     globalCount,
		MaxGlobalSessions:  m.maxGlobalSessions,
		DefaultCPURequest:  m.defaultCPURequest,
		DefaultCPULimit:    m.defaultCPULimit,
		DefaultMemRequest:  m.defaultMemRequest,
		DefaultMemLimit:    m.defaultMemLimit,
	}

	// Add tenant quota info if tenant specified
	if tenantID != "" {
		tenantCount, err := m.db.CountActiveSessionsByTenant(tenantID)
		if err != nil {
			return nil, fmt.Errorf("failed to count tenant sessions: %w", err)
		}
		status.TenantSessions = tenantCount

		tenant, err := m.db.GetTenant(tenantID)
		if err != nil {
			return nil, fmt.Errorf("failed to get tenant: %w", err)
		}
		if tenant != nil {
			status.MaxTenantSessions = tenant.Quotas.MaxTotalSessions
			// Tenant-level per-user limit overrides global if set
			if tenant.Quotas.MaxSessionsPerUser > 0 {
				status.MaxSessionsPerUser = tenant.Quotas.MaxSessionsPerUser
			}
			// Tenant-level resource defaults override global if set
			if tenant.Quotas.DefaultCPURequest != "" {
				status.DefaultCPURequest = tenant.Quotas.DefaultCPURequest
			}
			if tenant.Quotas.DefaultCPULimit != "" {
				status.DefaultCPULimit = tenant.Quotas.DefaultCPULimit
			}
			if tenant.Quotas.DefaultMemRequest != "" {
				status.DefaultMemRequest = tenant.Quotas.DefaultMemRequest
			}
			if tenant.Quotas.DefaultMemLimit != "" {
				status.DefaultMemLimit = tenant.Quotas.DefaultMemLimit
			}
		}
	}

	return status, nil
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

	// Check quotas before creating resources.
	// If the global limit is hit and a queue is configured, wait for capacity.
	if err := m.checkQuotas(req.UserID); err != nil {
		if m.queue != nil {
			if _, isQuotaErr := err.(*QuotaExceededError); isQuotaErr {
				log.Printf("Global session limit reached, queueing request for user %s", req.UserID)
				if qErr := m.queue.Enqueue(ctx); qErr != nil {
					return nil, qErr
				}
				// Re-check per-user quota after dequeue (global capacity is now available)
				if err := m.checkQuotas(req.UserID); err != nil {
					return nil, err
				}
			} else {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	// Generate session ID
	sessionID := uuid.New().String()

	// Build workload configuration
	wc := m.buildWorkloadConfig(sessionID, app)

	// Set screen resolution from client viewport if provided
	if req.ScreenWidth > 0 && req.ScreenHeight > 0 {
		wc.ScreenResolution = fmt.Sprintf("%dx%dx24", req.ScreenWidth, req.ScreenHeight)
		wc.ScreenWidth = req.ScreenWidth
		wc.ScreenHeight = req.ScreenHeight
	}

	// Apply resource limits (app-specific override global defaults)
	m.applyDefaultResourceLimits(wc, app)

	// Create the workload via the runner
	result, err := m.runner.CreateWorkload(ctx, wc)
	if err != nil {
		return nil, fmt.Errorf("failed to create workload: %w", err)
	}

	// Create per-session network policy if the runner supports it and the app has egress rules
	if app.EgressPolicy != nil && app.EgressPolicy.Mode != "" {
		if npr, ok := m.runner.(runner.NetworkPolicyRunner); ok {
			if err := npr.CreateNetworkPolicy(ctx, sessionID, app.ID, app.EgressPolicy); err != nil {
				log.Printf("Warning: failed to create egress policy for session %s: %v", sessionID, err)
				// Non-fatal: workload still runs, just without custom egress rules
			}
		}
	}

	// Create session in database
	now := time.Now()
	session := &db.Session{
		ID:          sessionID,
		UserID:      req.UserID,
		AppID:       req.AppID,
		PodName:     result.Name,
		Status:      db.SessionStatusCreating,
		IdleTimeout: req.IdleTimeout,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := m.db.CreateSession(*session); err != nil {
		// Try to clean up the workload and network policy
		m.runner.DeleteWorkload(ctx, result.Name)
		if npr, ok := m.runner.(runner.NetworkPolicyRunner); ok {
			npr.DeleteNetworkPolicy(ctx, sessionID)
		}
		return nil, fmt.Errorf("failed to create session in database: %w", err)
	}

	// Emit session created event
	m.emitEvent(ctx, EventSessionCreated, session, "session created")

	// Start goroutine to wait for workload ready and update session
	go m.waitForWorkloadReady(sessionID, result.Name)

	return session, nil
}

// waitForWorkloadReady waits for the workload to be ready and updates the session
func (m *Manager) waitForWorkloadReady(sessionID, workloadName string) {
	ctx, cancel := context.WithTimeout(context.Background(), m.podReadyTimeout)
	defer cancel()

	// Wait for workload to be ready
	if err := m.runner.WaitForReady(ctx, workloadName, m.podReadyTimeout); err != nil {
		LogTransition(sessionID, db.SessionStatusCreating, db.SessionStatusFailed, fmt.Sprintf("workload failed to become ready: %v", err))
		m.db.UpdateSessionStatus(sessionID, db.SessionStatusFailed)
		m.emitEventByID(sessionID, EventSessionFailed, fmt.Sprintf("workload failed to become ready: %v", err))
		if delErr := m.runner.DeleteWorkload(context.Background(), workloadName); delErr != nil {
			log.Printf("Failed to delete workload %s after timeout: %v", workloadName, delErr)
		}
		return
	}

	// Get workload IP
	ip, err := m.runner.GetIP(ctx, workloadName)
	if err != nil {
		LogTransition(sessionID, db.SessionStatusCreating, db.SessionStatusFailed, fmt.Sprintf("failed to get workload IP: %v", err))
		m.db.UpdateSessionStatus(sessionID, db.SessionStatusFailed)
		m.emitEventByID(sessionID, EventSessionFailed, fmt.Sprintf("failed to get workload IP: %v", err))
		if delErr := m.runner.DeleteWorkload(context.Background(), workloadName); delErr != nil {
			log.Printf("Failed to delete workload %s after IP lookup failure: %v", workloadName, delErr)
		}
		return
	}

	// Update session with IP and running status in a single operation
	LogTransition(sessionID, db.SessionStatusCreating, db.SessionStatusRunning, "workload ready")
	if err := m.db.UpdateSessionPodIPAndStatus(sessionID, ip, db.SessionStatusRunning); err != nil {
		log.Printf("Failed to update session for %s: %v", sessionID, err)
	}

	// Emit session ready event
	m.emitEventByID(sessionID, EventSessionReady, "workload ready")
}

// GetSession returns a session by ID, always reading from the database
// to ensure consistency across multiple replicas.
func (m *Manager) GetSession(ctx context.Context, sessionID string) (*db.Session, error) {
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

// StopSession stops a running session, deleting the workload but keeping the session
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

	// Delete the workload
	if err := m.runner.DeleteWorkload(ctx, session.PodName); err != nil {
		log.Printf("Warning: failed to delete workload %s: %v", session.PodName, err)
	}

	// Clean up network policy (if runner supports it)
	if npr, ok := m.runner.(runner.NetworkPolicyRunner); ok {
		npr.DeleteNetworkPolicy(ctx, sessionID)
	}

	// Update status to stopped
	if err := m.db.UpdateSessionStatus(sessionID, db.SessionStatusStopped); err != nil {
		return fmt.Errorf("failed to update session status: %w", err)
	}

	// Emit session stopped event
	m.emitEvent(ctx, EventSessionStopped, session, "user stopped")

	// Notify the queue that capacity may be available
	if m.queue != nil {
		m.queue.NotifyCapacity()
	}

	return nil
}

// RestartSession recreates the workload for a stopped session.
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

	// Check quotas before recreating resources
	if err := m.checkQuotas(session.UserID); err != nil {
		return nil, err
	}

	// Get the application to rebuild the workload
	app, err := m.db.GetApp(session.AppID)
	if err != nil {
		return nil, fmt.Errorf("failed to get application: %w", err)
	}
	if app == nil {
		return nil, fmt.Errorf("application not found: %s", session.AppID)
	}

	// Build workload configuration using the existing session ID
	wc := m.buildWorkloadConfig(sessionID, app)

	// Apply resource limits (app-specific override global defaults)
	m.applyDefaultResourceLimits(wc, app)

	// Create the workload via the runner
	result, err := m.runner.CreateWorkload(ctx, wc)
	if err != nil {
		return nil, fmt.Errorf("failed to create workload: %w", err)
	}

	// Create per-session network policy if the runner supports it and the app has egress rules
	if app.EgressPolicy != nil && app.EgressPolicy.Mode != "" {
		if npr, ok := m.runner.(runner.NetworkPolicyRunner); ok {
			if err := npr.CreateNetworkPolicy(ctx, sessionID, app.ID, app.EgressPolicy); err != nil {
				log.Printf("Warning: failed to create egress policy for restarted session %s: %v", sessionID, err)
			}
		}
	}

	// Update session in database with new workload name and creating status
	if err := m.db.UpdateSessionRestart(sessionID, result.Name); err != nil {
		m.runner.DeleteWorkload(ctx, result.Name)
		if npr, ok := m.runner.(runner.NetworkPolicyRunner); ok {
			npr.DeleteNetworkPolicy(ctx, sessionID)
		}
		return nil, fmt.Errorf("failed to update session in database: %w", err)
	}

	// Re-read the session from DB to get the updated state
	session, err = m.db.GetSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to re-read session after restart: %w", err)
	}

	// Emit session restarted event
	m.emitEvent(ctx, EventSessionRestarted, session, "user restarted")

	// Wait for workload ready in background
	go m.waitForWorkloadReady(sessionID, result.Name)

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

	// Delete the workload
	if err := m.runner.DeleteWorkload(ctx, session.PodName); err != nil {
		log.Printf("Warning: failed to delete workload %s: %v", session.PodName, err)
	}

	// Clean up network policy (if runner supports it)
	if npr, ok := m.runner.(runner.NetworkPolicyRunner); ok {
		npr.DeleteNetworkPolicy(ctx, sessionID)
	}

	// Update status to final state
	if err := m.db.UpdateSessionStatus(sessionID, finalStatus); err != nil {
		return fmt.Errorf("failed to update session status: %w", err)
	}

	// Emit event based on final status
	switch finalStatus {
	case db.SessionStatusExpired:
		m.emitEvent(ctx, EventSessionExpired, session, reason)
	default:
		m.emitEvent(ctx, EventSessionTerminated, session, reason)
	}

	// Notify the queue that capacity may be available
	if m.queue != nil {
		m.queue.NotifyCapacity()
	}

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
