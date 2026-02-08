package testutil

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rjsadow/launchpad/internal/config"
	"github.com/rjsadow/launchpad/internal/db"
	"github.com/rjsadow/launchpad/internal/diagnostics"
	"github.com/rjsadow/launchpad/internal/files"
	"github.com/rjsadow/launchpad/internal/plugins"
	"github.com/rjsadow/launchpad/internal/plugins/auth"
	"github.com/rjsadow/launchpad/internal/server"
	"github.com/rjsadow/launchpad/internal/sessions"
)

const (
	// TestJWTSecret is the JWT secret used for all integration tests.
	TestJWTSecret = "test-jwt-secret-for-integration-tests"
	// TestAdminUsername is the admin username seeded in every test DB.
	TestAdminUsername = "admin"
	// TestAdminPassword is the admin password seeded in every test DB.
	TestAdminPassword = "admin123"
)

// TestServer wraps an httptest.Server with test-specific helpers.
type TestServer struct {
	// Server is the underlying httptest.Server.
	Server *httptest.Server
	// URL is the base URL of the test server (e.g. "http://127.0.0.1:12345").
	URL string
	// AdminToken is a pre-generated admin access token.
	AdminToken string
	// DB is the in-memory database.
	DB *db.DB
	// Runner is the mock workload runner.
	Runner *MockRunner
	// SessionManager is the session manager instance.
	SessionManager *sessions.Manager
	// Config is the test configuration.
	Config *config.Config
}

// Option is a function that modifies the test config before server creation.
type Option func(*config.Config)

// WithMaxSessionsPerUser sets the per-user session quota.
func WithMaxSessionsPerUser(n int) Option {
	return func(c *config.Config) { c.MaxSessionsPerUser = n }
}

// WithMaxGlobalSessions sets the global session quota.
func WithMaxGlobalSessions(n int) Option {
	return func(c *config.Config) { c.MaxGlobalSessions = n }
}

// NewTestServer creates a fully wired test server with:
//   - Fresh in-memory SQLite database
//   - JWT auth provider with test secret
//   - Seeded admin user (admin/admin123)
//   - Mock runner (no real Kubernetes)
//   - All routes registered via server.App.Handler()
//
// The server is automatically cleaned up when the test completes.
// Optional Option functions can modify the config before the server is built.
func NewTestServer(t *testing.T, opts ...Option) *TestServer {
	t.Helper()

	// 1. Open a temp-file SQLite database.
	// We cannot use ":memory:" because Go's sql.DB connection pool opens
	// multiple connections, and each in-memory connection gets its own DB.
	// The session manager's background goroutines would see empty tables.
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Suppress noisy log output during tests
	_ = os.Setenv("LAUNCHPAD_LOG_LEVEL", "error")

	// 2. Build test config
	cfg := &config.Config{
		Port:               0,
		DB:                 ":memory:",
		JWTSecret:          TestJWTSecret,
		JWTAccessExpiry:    15 * time.Minute,
		JWTRefreshExpiry:   24 * time.Hour,
		AdminUsername:      TestAdminUsername,
		AdminPassword:      TestAdminPassword,
		AllowRegistration:  true,
		MaxSessionsPerUser: 10,
		MaxGlobalSessions:  100,
		PodReadyTimeout:    30 * time.Second,
		SessionTimeout:     1 * time.Hour,
		MaxUploadSize:      10 * 1024 * 1024,
	}

	// Apply custom options before building the server
	for _, opt := range opts {
		opt(cfg)
	}

	// 3. Initialize JWT auth provider
	jwtAuth := auth.NewJWTAuthProvider()
	authConfig := map[string]string{
		"jwt_secret":     cfg.JWTSecret,
		"access_expiry":  cfg.JWTAccessExpiry.String(),
		"refresh_expiry": cfg.JWTRefreshExpiry.String(),
	}
	if err := jwtAuth.Initialize(context.Background(), authConfig); err != nil {
		t.Fatalf("failed to initialize JWT auth: %v", err)
	}
	jwtAuth.SetDatabase(database)

	// 4. Seed admin user
	passwordHash, err := auth.HashPassword(TestAdminPassword)
	if err != nil {
		t.Fatalf("failed to hash admin password: %v", err)
	}
	if err := database.SeedAdminUser(TestAdminUsername, passwordHash); err != nil {
		t.Fatalf("failed to seed admin user: %v", err)
	}

	// 5. Create mock runner + session manager
	mockRunner := NewMockRunner()
	sm := sessions.NewManagerWithConfig(database, sessions.ManagerConfig{
		SessionTimeout:     cfg.SessionTimeout,
		CleanupInterval:    5 * time.Minute,
		PodReadyTimeout:    cfg.PodReadyTimeout,
		MaxSessionsPerUser: cfg.MaxSessionsPerUser,
		MaxGlobalSessions:  cfg.MaxGlobalSessions,
		Runner:             mockRunner,
	})
	sm.Start()

	// 6. Create backpressure handler
	bp := sessions.NewBackpressureHandler(sm, sm.Queue(), 0)

	// 7. Create file handler
	fh := files.NewHandler(sm, database, cfg.MaxUploadSize)

	// 8. Create diagnostics collector
	dc := diagnostics.NewCollector(database, cfg, plugins.Global(), time.Now())

	// 9. Build server.App and handler
	app := &server.App{
		DB:                  database,
		SessionManager:      sm,
		JWTAuth:             jwtAuth,
		OIDCAuth:            nil,
		GatewayHandler:      nil, // No WebSocket gateway in integration tests
		BackpressureHandler: bp,
		FileHandler:         fh,
		DiagCollector:       dc,
		Config:              cfg,
		StaticFS:            nil, // No static files in integration tests
	}

	ts := httptest.NewServer(app.Handler())

	// 10. Pre-generate admin token
	adminToken := LoginAs(t, ts.URL, TestAdminUsername, TestAdminPassword)

	// 11. Register cleanup
	t.Cleanup(func() {
		ts.Close()
		sm.Stop()
		database.Close()
	})

	return &TestServer{
		Server:         ts,
		URL:            ts.URL,
		AdminToken:     adminToken,
		DB:             database,
		Runner:         mockRunner,
		SessionManager: sm,
		Config:         cfg,
	}
}
