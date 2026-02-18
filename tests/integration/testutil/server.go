package testutil

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/rjsadow/sortie/internal/config"
	"github.com/rjsadow/sortie/internal/db"
	"github.com/rjsadow/sortie/internal/db/dbtest"
	"github.com/rjsadow/sortie/internal/diagnostics"
	"github.com/rjsadow/sortie/internal/files"
	"github.com/rjsadow/sortie/internal/plugins"
	"github.com/rjsadow/sortie/internal/plugins/auth"
	"github.com/rjsadow/sortie/internal/plugins/storage"
	"github.com/rjsadow/sortie/internal/recordings"
	"github.com/rjsadow/sortie/internal/server"
	"github.com/rjsadow/sortie/internal/sessions"
	"github.com/rjsadow/sortie/internal/sse"
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

// WithRecordingEnabled enables the video recording handler with local storage.
func WithRecordingEnabled() Option {
	return func(c *config.Config) {
		c.VideoRecordingEnabled = true
		c.RecordingStorageBackend = "local"
		c.RecordingMaxSizeMB = 10
	}
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

	// Use fast bcrypt for tests (DefaultCost is ~100x slower under -race).
	auth.BcryptCost = bcrypt.MinCost

	// 1. Open a test database via dbtest (supports sqlite or postgres).
	tmpDir := t.TempDir()
	database := dbtest.NewTestDB(t)

	// Wire shared DB into the storage plugin
	storage.SetDB(database)

	// Suppress noisy log output during tests
	_ = os.Setenv("SORTIE_LOG_LEVEL", "error")

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

	// 5. Create SSE hub + mock runner + session manager
	// Use MultiRecorder to match main.go's composition pattern
	sseHub := sse.NewHub(jwtAuth)
	recorder := sessions.NewMultiRecorder(nil, sseHub) // nil simulates the no-billing case
	mockRunner := NewMockRunner()
	sm := sessions.NewManagerWithConfig(database, sessions.ManagerConfig{
		SessionTimeout:     cfg.SessionTimeout,
		CleanupInterval:    5 * time.Minute,
		PodReadyTimeout:    cfg.PodReadyTimeout,
		MaxSessionsPerUser: cfg.MaxSessionsPerUser,
		MaxGlobalSessions:  cfg.MaxGlobalSessions,
		Recorder:           recorder,
		Runner:             mockRunner,
	})
	sm.Start()

	// 6. Create backpressure handler
	bp := sessions.NewBackpressureHandler(sm, sm.Queue(), 0)

	// 7. Create file handler
	fh := files.NewHandler(sm, database, cfg.MaxUploadSize)

	// 8. Create diagnostics collector
	dc := diagnostics.NewCollector(database, cfg, plugins.Global(), time.Now())

	// 9. Optionally create recording handler
	var recordingHandler *recordings.Handler
	if cfg.VideoRecordingEnabled {
		recDir := filepath.Join(tmpDir, "recordings")
		os.MkdirAll(recDir, 0o755)
		recStore := recordings.NewLocalStore(recDir)
		recordingHandler = recordings.NewHandler(database, recStore, cfg)
	}

	// 10. Build server.App and handler
	app := &server.App{
		DB:                  database,
		SessionManager:      sm,
		JWTAuth:             jwtAuth,
		OIDCAuth:            nil,
		GatewayHandler:      nil, // No WebSocket gateway in integration tests
		SSEHub:              sseHub,
		BackpressureHandler: bp,
		FileHandler:         fh,
		RecordingHandler:    recordingHandler,
		DiagCollector:       dc,
		Config:              cfg,
		StaticFS:            nil, // No static files in integration tests
	}

	ts := httptest.NewServer(app.Handler())

	// 10. Pre-generate admin token
	adminToken := LoginAs(t, ts.URL, TestAdminUsername, TestAdminPassword)

	// 11. Register cleanup (database.Close() is handled by dbtest)
	t.Cleanup(func() {
		ts.Close()
		sm.Stop()
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
