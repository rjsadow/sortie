package storage

import (
	"context"
	"log"

	"github.com/rjsadow/sortie/internal/db"
	"github.com/rjsadow/sortie/internal/plugins"
)

// sharedDB holds the application's *db.DB instance, set via SetDB before
// plugin initialization.
var sharedDB *db.DB

// SetDB stores the shared database reference for the SQLite storage plugin.
// Call this from main.go after opening the database but before any plugin
// initialization that may invoke the factory.
func SetDB(database *db.DB) { sharedDB = database }

// SQLiteStorage implements StorageProvider as a thin wrapper around *db.DB.
// All real data operations are handled by db.DB directly; this plugin exists
// to satisfy the StorageProvider interface and provide health-check status
// via the plugin registry.
type SQLiteStorage struct {
	db     *db.DB
	config map[string]string
}

func init() {
	plugins.RegisterGlobal(plugins.PluginTypeStorage, "sqlite", func() plugins.Plugin {
		return NewSQLiteStorage(sharedDB)
	})
}

// NewSQLiteStorage creates a new SQLite storage provider wrapping the given database.
func NewSQLiteStorage(database *db.DB) *SQLiteStorage {
	return &SQLiteStorage{db: database}
}

// Name returns the plugin name.
func (s *SQLiteStorage) Name() string {
	return "sqlite"
}

// Type returns the plugin type.
func (s *SQLiteStorage) Type() plugins.PluginType {
	return plugins.PluginTypeStorage
}

// Version returns the plugin version.
func (s *SQLiteStorage) Version() string {
	return "1.0.0"
}

// Description returns a human-readable description.
func (s *SQLiteStorage) Description() string {
	return "SQLite database storage provider"
}

// Initialize stores config. The database connection and migrations are
// managed by the main application via db.OpenDB.
func (s *SQLiteStorage) Initialize(ctx context.Context, config map[string]string) error {
	s.config = config
	log.Printf("SQLite storage plugin initialized (using shared db.DB)")
	return nil
}

// Healthy returns true if the underlying database is reachable.
func (s *SQLiteStorage) Healthy(ctx context.Context) bool {
	if s.db == nil {
		return false
	}
	return s.db.Ping() == nil
}

// Close is a no-op; the shared db.DB connection is owned by the application.
func (s *SQLiteStorage) Close() error {
	return nil
}

// --- StorageProvider CRUD stubs ---
// These methods are never called by the application (db.DB handles all data
// operations directly). They return ErrNotImplemented to satisfy the interface.

func (s *SQLiteStorage) CreateApp(ctx context.Context, app *plugins.Application) error {
	return plugins.ErrNotImplemented
}

func (s *SQLiteStorage) GetApp(ctx context.Context, id string) (*plugins.Application, error) {
	return nil, plugins.ErrNotImplemented
}

func (s *SQLiteStorage) UpdateApp(ctx context.Context, app *plugins.Application) error {
	return plugins.ErrNotImplemented
}

func (s *SQLiteStorage) DeleteApp(ctx context.Context, id string) error {
	return plugins.ErrNotImplemented
}

func (s *SQLiteStorage) ListApps(ctx context.Context) ([]*plugins.Application, error) {
	return nil, plugins.ErrNotImplemented
}

func (s *SQLiteStorage) CreateSession(ctx context.Context, session *plugins.Session) error {
	return plugins.ErrNotImplemented
}

func (s *SQLiteStorage) GetSession(ctx context.Context, id string) (*plugins.Session, error) {
	return nil, plugins.ErrNotImplemented
}

func (s *SQLiteStorage) UpdateSession(ctx context.Context, session *plugins.Session) error {
	return plugins.ErrNotImplemented
}

func (s *SQLiteStorage) DeleteSession(ctx context.Context, id string) error {
	return plugins.ErrNotImplemented
}

func (s *SQLiteStorage) ListSessions(ctx context.Context, userID string) ([]*plugins.Session, error) {
	return nil, plugins.ErrNotImplemented
}

func (s *SQLiteStorage) ListExpiredSessions(ctx context.Context) ([]*plugins.Session, error) {
	return nil, plugins.ErrNotImplemented
}

func (s *SQLiteStorage) LogAudit(ctx context.Context, entry *plugins.AuditEntry) error {
	return plugins.ErrNotImplemented
}

func (s *SQLiteStorage) GetAuditLogs(ctx context.Context, limit int) ([]*plugins.AuditEntry, error) {
	return nil, plugins.ErrNotImplemented
}

func (s *SQLiteStorage) RecordLaunch(ctx context.Context, appID string) error {
	return plugins.ErrNotImplemented
}

func (s *SQLiteStorage) GetAnalyticsStats(ctx context.Context) (map[string]any, error) {
	return nil, plugins.ErrNotImplemented
}

// Verify interface compliance
var _ plugins.StorageProvider = (*SQLiteStorage)(nil)
