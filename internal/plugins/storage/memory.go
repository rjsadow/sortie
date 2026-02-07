package storage

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/rjsadow/launchpad/internal/plugins"
)

// MemoryStorage implements StorageProvider using in-memory storage.
// Useful for testing and development only.
//
// WARNING: NOT suitable for multi-replica deployments. Use the SQLite-backed
// storage (or the direct db.DB layer) for production horizontal scalability.
type MemoryStorage struct {
	mu           sync.RWMutex
	apps         map[string]*plugins.Application
	sessions     map[string]*plugins.Session
	auditEntries []*plugins.AuditEntry
	analytics    []string // app IDs
	config       map[string]string
	auditCounter int64
}

func init() {
	plugins.RegisterGlobal(plugins.PluginTypeStorage, "memory", func() plugins.Plugin {
		return NewMemoryStorage()
	})
}

// NewMemoryStorage creates a new in-memory storage provider.
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		apps:         make(map[string]*plugins.Application),
		sessions:     make(map[string]*plugins.Session),
		auditEntries: make([]*plugins.AuditEntry, 0),
		analytics:    make([]string, 0),
	}
}

// Name returns the plugin name.
func (s *MemoryStorage) Name() string {
	return "memory"
}

// Type returns the plugin type.
func (s *MemoryStorage) Type() plugins.PluginType {
	return plugins.PluginTypeStorage
}

// Version returns the plugin version.
func (s *MemoryStorage) Version() string {
	return "1.0.0"
}

// Description returns a human-readable description.
func (s *MemoryStorage) Description() string {
	return "In-memory storage provider for testing and development"
}

// Initialize sets up the plugin with configuration.
func (s *MemoryStorage) Initialize(ctx context.Context, config map[string]string) error {
	s.config = config
	log.Printf("Memory storage initialized")
	return nil
}

// Healthy returns true if the plugin is operational.
func (s *MemoryStorage) Healthy(ctx context.Context) bool {
	return true
}

// Close releases resources.
func (s *MemoryStorage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.apps = make(map[string]*plugins.Application)
	s.sessions = make(map[string]*plugins.Session)
	s.auditEntries = make([]*plugins.AuditEntry, 0)
	s.analytics = make([]string, 0)

	return nil
}

// Application CRUD

// CreateApp creates a new application.
func (s *MemoryStorage) CreateApp(ctx context.Context, app *plugins.Application) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.apps[app.ID]; exists {
		return plugins.ErrResourceExists
	}

	// Clone the app to prevent external modification
	clone := *app
	clone.CreatedAt = time.Now()
	clone.UpdatedAt = clone.CreatedAt

	s.apps[app.ID] = &clone
	return nil
}

// GetApp retrieves an application by ID.
func (s *MemoryStorage) GetApp(ctx context.Context, id string) (*plugins.Application, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	app, exists := s.apps[id]
	if !exists {
		return nil, nil
	}

	// Return a clone
	clone := *app
	return &clone, nil
}

// UpdateApp updates an existing application.
func (s *MemoryStorage) UpdateApp(ctx context.Context, app *plugins.Application) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, exists := s.apps[app.ID]
	if !exists {
		return plugins.ErrResourceNotFound
	}

	// Clone and update
	clone := *app
	clone.CreatedAt = existing.CreatedAt
	clone.UpdatedAt = time.Now()

	s.apps[app.ID] = &clone
	return nil
}

// DeleteApp deletes an application.
func (s *MemoryStorage) DeleteApp(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.apps[id]; !exists {
		return plugins.ErrResourceNotFound
	}

	delete(s.apps, id)
	return nil
}

// ListApps lists all applications.
func (s *MemoryStorage) ListApps(ctx context.Context) ([]*plugins.Application, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	apps := make([]*plugins.Application, 0, len(s.apps))
	for _, app := range s.apps {
		clone := *app
		apps = append(apps, &clone)
	}

	return apps, nil
}

// Session management

// CreateSession creates a new session.
func (s *MemoryStorage) CreateSession(ctx context.Context, session *plugins.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.sessions[session.ID]; exists {
		return plugins.ErrResourceExists
	}

	// Clone the session
	clone := *session
	clone.CreatedAt = time.Now()

	s.sessions[session.ID] = &clone
	return nil
}

// GetSession retrieves a session by ID.
func (s *MemoryStorage) GetSession(ctx context.Context, id string) (*plugins.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, exists := s.sessions[id]
	if !exists {
		return nil, nil
	}

	clone := *session
	return &clone, nil
}

// UpdateSession updates an existing session.
func (s *MemoryStorage) UpdateSession(ctx context.Context, session *plugins.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, exists := s.sessions[session.ID]
	if !exists {
		return plugins.ErrResourceNotFound
	}

	// Clone and update, preserving created_at
	clone := *session
	clone.CreatedAt = existing.CreatedAt

	s.sessions[session.ID] = &clone
	return nil
}

// DeleteSession deletes a session.
func (s *MemoryStorage) DeleteSession(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.sessions[id]; !exists {
		return plugins.ErrResourceNotFound
	}

	delete(s.sessions, id)
	return nil
}

// ListSessions lists sessions, optionally filtered by user ID.
func (s *MemoryStorage) ListSessions(ctx context.Context, userID string) ([]*plugins.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessions := make([]*plugins.Session, 0)
	for _, session := range s.sessions {
		if userID == "" || session.UserID == userID {
			clone := *session
			sessions = append(sessions, &clone)
		}
	}

	return sessions, nil
}

// ListExpiredSessions lists sessions that have exceeded their expiry time.
func (s *MemoryStorage) ListExpiredSessions(ctx context.Context) ([]*plugins.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	sessions := make([]*plugins.Session, 0)
	for _, session := range s.sessions {
		if session.ExpiresAt != nil && session.ExpiresAt.Before(now) {
			if session.Status == plugins.LaunchStatusCreating || session.Status == plugins.LaunchStatusRunning {
				clone := *session
				sessions = append(sessions, &clone)
			}
		}
	}

	return sessions, nil
}

// Audit logging

// LogAudit logs an audit entry.
func (s *MemoryStorage) LogAudit(ctx context.Context, entry *plugins.AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.auditCounter++
	clone := *entry
	clone.ID = fmt.Sprintf("%d", s.auditCounter)
	clone.Timestamp = time.Now()

	s.auditEntries = append(s.auditEntries, &clone)
	return nil
}

// GetAuditLogs retrieves recent audit log entries.
func (s *MemoryStorage) GetAuditLogs(ctx context.Context, limit int) ([]*plugins.AuditEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return most recent entries first
	entries := make([]*plugins.AuditEntry, 0)
	start := len(s.auditEntries) - limit
	if start < 0 {
		start = 0
	}

	for i := len(s.auditEntries) - 1; i >= start; i-- {
		clone := *s.auditEntries[i]
		entries = append(entries, &clone)
	}

	return entries, nil
}

// Analytics

// RecordLaunch records an app launch for analytics.
func (s *MemoryStorage) RecordLaunch(ctx context.Context, appID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.analytics = append(s.analytics, appID)
	return nil
}

// GetAnalyticsStats returns analytics statistics.
func (s *MemoryStorage) GetAnalyticsStats(ctx context.Context) (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := make(map[string]any)
	stats["total_launches"] = len(s.analytics)
	stats["total_apps"] = len(s.apps)

	// Count launches per app
	counts := make(map[string]int)
	for _, appID := range s.analytics {
		counts[appID]++
	}

	// Get top apps
	var topApps []map[string]any
	for appID, count := range counts {
		name := appID
		if app, exists := s.apps[appID]; exists {
			name = app.Name
		}
		topApps = append(topApps, map[string]any{
			"app_id":   appID,
			"name":     name,
			"launches": count,
		})
	}
	stats["top_apps"] = topApps

	return stats, nil
}

// Verify interface compliance
var _ plugins.StorageProvider = (*MemoryStorage)(nil)
