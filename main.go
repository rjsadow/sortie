package main

import (
	"context"
	"embed"
	"expvar"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/rjsadow/sortie/internal/billing"
	"github.com/rjsadow/sortie/internal/config"
	"github.com/rjsadow/sortie/internal/db"
	"github.com/rjsadow/sortie/internal/diagnostics"
	"github.com/rjsadow/sortie/internal/files"
	"github.com/rjsadow/sortie/internal/gateway"
	"github.com/rjsadow/sortie/internal/recordings"
	"github.com/rjsadow/sortie/internal/k8s"
	"github.com/rjsadow/sortie/internal/plugins"
	"github.com/rjsadow/sortie/internal/plugins/auth"
	"github.com/rjsadow/sortie/internal/plugins/storage"
	"github.com/rjsadow/sortie/internal/runner"
	"github.com/rjsadow/sortie/internal/server"
	"github.com/rjsadow/sortie/internal/sessions"
	"github.com/rjsadow/sortie/internal/sse"

	"golang.org/x/time/rate"
)

//go:embed all:web/dist
var embeddedFiles embed.FS

//go:embed all:docs-site/dist
var embeddedDocs embed.FS

//go:embed web/src/data/templates.json
var embeddedTemplates []byte

func main() {
	// Initialize structured logging with JSON handler for production
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Parse command-line flags (can override env vars)
	port := flag.Int("port", config.DefaultPort, "Port to listen on")
	dbPath := flag.String("db", config.DefaultDBPath, "Path to SQLite database")
	seedPath := flag.String("seed", "", "Path to apps.json for initial seeding")
	mockRunnerFlag := flag.Bool("mock-runner", false, "Use in-memory mock runner (no Kubernetes required)")
	flag.Parse()

	// Load configuration (env vars + flag overrides)
	appConfig, err := config.LoadWithFlags(*port, *dbPath, *seedPath)
	if err != nil {
		slog.Error("configuration error", "error", err, "hint", "See .env.example for configuration options.")
		os.Exit(1)
	}

	// Initialize Kubernetes configuration (skip when using mock runner)
	if !*mockRunnerFlag {
		k8s.Configure(appConfig.Namespace, appConfig.Kubeconfig, appConfig.VNCSidecarImage)
		k8s.ConfigureBrowserSidecar(appConfig.BrowserSidecarImage)
		k8s.ConfigureGuacdSidecar(appConfig.GuacdSidecarImage)
	}

	// Initialize database
	database, err := db.OpenDB(appConfig.DBType, appConfig.DSN())
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	// Wire shared DB into the storage plugin before any plugin initialization
	storage.SetDB(database)

	// Seed from JSON if provided and database is empty
	if appConfig.Seed != "" {
		if err := database.SeedFromJSON(appConfig.Seed); err != nil {
			slog.Warn("failed to seed from JSON", "error", err)
		}
	}

	// Seed templates from embedded templates.json if templates table is empty
	if err := database.SeedTemplatesFromData(embeddedTemplates); err != nil {
		slog.Warn("failed to seed templates", "error", err)
	}

	// Initialize JWT auth provider
	var jwtAuthProvider *auth.JWTAuthProvider
	if appConfig.JWTSecret != "" {
		jwtAuthProvider = auth.NewJWTAuthProvider()
		authConfig := map[string]string{
			"jwt_secret":     appConfig.JWTSecret,
			"access_expiry":  appConfig.JWTAccessExpiry.String(),
			"refresh_expiry": appConfig.JWTRefreshExpiry.String(),
		}
		if err := jwtAuthProvider.Initialize(context.Background(), authConfig); err != nil {
			slog.Error("failed to initialize JWT auth provider", "error", err)
			os.Exit(1)
		}
		jwtAuthProvider.SetDatabase(database)

		// Seed admin user if password is configured
		if appConfig.AdminPassword != "" {
			passwordHash, err := auth.HashPassword(appConfig.AdminPassword)
			if err != nil {
				slog.Error("failed to hash admin password", "error", err)
				os.Exit(1)
			}
			if err := database.SeedAdminUser(appConfig.AdminUsername, passwordHash); err != nil {
				slog.Warn("failed to seed admin user", "error", err)
			} else {
				slog.Info("admin user ready", "username", appConfig.AdminUsername)
			}
		}
	} else {
		slog.Warn("SORTIE_JWT_SECRET not set - authentication disabled")
	}

	// Initialize OIDC auth provider if configured
	var oidcAuthProvider *auth.OIDCAuthProvider
	if appConfig.OIDCEnabled() && appConfig.JWTSecret != "" {
		oidcAuthProvider = auth.NewOIDCAuthProvider()
		oidcScopes := appConfig.OIDCScopes
		oidcConfig := map[string]string{
			"issuer":         appConfig.OIDCIssuer,
			"client_id":      appConfig.OIDCClientID,
			"client_secret":  appConfig.OIDCClientSecret,
			"redirect_url":   appConfig.OIDCRedirectURL,
			"jwt_secret":     appConfig.JWTSecret,
			"access_expiry":  appConfig.JWTAccessExpiry.String(),
			"refresh_expiry": appConfig.JWTRefreshExpiry.String(),
		}
		if oidcScopes != "" {
			oidcConfig["scopes"] = oidcScopes
		}
		if err := oidcAuthProvider.Initialize(context.Background(), oidcConfig); err != nil {
			slog.Error("failed to initialize OIDC auth provider", "error", err)
			// Non-fatal: SSO won't be available but local login still works
			oidcAuthProvider = nil
		} else {
			oidcAuthProvider.SetDatabase(database)
			slog.Info("OIDC SSO enabled", "issuer", appConfig.OIDCIssuer)
		}
	}

	// Initialize session recorder (noop by default, enabled via config)
	var sessionRecorder sessions.SessionRecorder
	if appConfig.RecordingEnabled {
		slog.Info("Session recording enabled")
		sessionRecorder = &sessions.NoopRecorder{}
	}

	// Initialize billing metering collector (replaces noop recorder when enabled)
	var billingCollector *billing.Collector
	if appConfig.BillingEnabled {
		billingCollector = billing.NewCollector()
		sessionRecorder = billingCollector

		// Select billing exporter
		var exporter billing.Exporter
		switch appConfig.BillingExporter {
		case "webhook":
			exporter = &billing.WebhookExporter{Endpoint: appConfig.BillingWebhookURL}
		default:
			exporter = &billing.LogExporter{}
		}

		// Start export loop in background
		billingCtx, billingCancel := context.WithCancel(context.Background())
		defer billingCancel()
		go billing.ExportLoop(billingCtx, billingCollector, exporter, appConfig.BillingExportInterval)

		slog.Info("Billing metering enabled",
			"exporter", exporter.Name(),
			"interval", appConfig.BillingExportInterval)
	}

	// Initialize SSE hub for real-time session event push
	var sseHub *sse.Hub
	if jwtAuthProvider != nil {
		sseHub = sse.NewHub(jwtAuthProvider)
	}

	// Compose multi-recorder: chain existing recorder (billing/noop) with SSE hub
	sessionRecorder = sessions.NewMultiRecorder(sessionRecorder, sseHub)

	// Initialize workload runner
	var workloadRunner runner.Runner
	if *mockRunnerFlag {
		workloadRunner = runner.NewMockRunner()
	} else {
		workloadRunner = runner.NewKubernetesRunner()
	}
	slog.Info("Workload runner initialized", "type", workloadRunner.Type())

	// Initialize session manager with config
	sessionManager := sessions.NewManagerWithConfig(database, sessions.ManagerConfig{
		SessionTimeout:     appConfig.SessionTimeout,
		CleanupInterval:    appConfig.SessionCleanupInterval,
		PodReadyTimeout:    appConfig.PodReadyTimeout,
		MaxSessionsPerUser: appConfig.MaxSessionsPerUser,
		MaxGlobalSessions:  appConfig.MaxGlobalSessions,
		DefaultCPURequest:  appConfig.DefaultCPURequest,
		DefaultCPULimit:    appConfig.DefaultCPULimit,
		DefaultMemRequest:  appConfig.DefaultMemRequest,
		DefaultMemLimit:    appConfig.DefaultMemLimit,
		Recorder:           sessionRecorder,
		QueueMaxSize:       appConfig.QueueMaxSize,
		QueueTimeout:       appConfig.QueueTimeout,
		Runner:             workloadRunner,
	})
	sessionManager.Start()
	defer sessionManager.Stop()

	// Initialize backpressure handler for load monitoring and admission control
	backpressureHandler := sessions.NewBackpressureHandler(
		sessionManager,
		sessionManager.Queue(),
		appConfig.QueueMaxSize,
	)
	if appConfig.QueueMaxSize > 0 {
		slog.Info("Session queueing enabled",
			"max_size", appConfig.QueueMaxSize,
			"timeout", appConfig.QueueTimeout)
	}

	// Initialize gateway handler (WebSocket proxy with auth + rate limiting)
	var gwHandler *gateway.Handler
	if appConfig.JWTSecret != "" {
		var rl *gateway.RateLimiter
		if appConfig.GatewayRateLimit > 0 {
			rl = gateway.NewRateLimiter(rate.Limit(appConfig.GatewayRateLimit), appConfig.GatewayBurst)
			slog.Info("Gateway rate limiter enabled",
				"rate", appConfig.GatewayRateLimit,
				"burst", appConfig.GatewayBurst)
		}
		gwHandler = gateway.NewHandler(gateway.Config{
			SessionManager: sessionManager,
			AuthProvider:   jwtAuthProvider,
			Database:       database,
			RateLimiter:    rl,
		})
		slog.Info("Gateway service initialized with auth and rate limiting")
	} else {
		slog.Warn("Gateway disabled: SORTIE_JWT_SECRET not set, WebSocket endpoints unprotected")
	}

	// Initialize file transfer handler
	fileHandler := files.NewHandler(sessionManager, database, appConfig.MaxUploadSize)

	// Initialize video recording handler
	var recordingHandler *recordings.Handler
	if appConfig.VideoRecordingEnabled {
		var recordingStore recordings.RecordingStore
		switch appConfig.RecordingStorageBackend {
		case "s3":
			s3Store, err := recordings.NewS3Store(
				appConfig.RecordingS3Bucket,
				appConfig.RecordingS3Region,
				appConfig.RecordingS3Endpoint,
				appConfig.RecordingS3Prefix,
				appConfig.RecordingS3AccessKeyID,
				appConfig.RecordingS3SecretAccessKey,
			)
			if err != nil {
				slog.Error("failed to initialize S3 recording store", "error", err)
				os.Exit(1)
			}
			recordingStore = s3Store
			slog.Info("Video recording enabled",
				"storage_backend", "s3",
				"bucket", appConfig.RecordingS3Bucket,
				"region", appConfig.RecordingS3Region,
				"max_size_mb", appConfig.RecordingMaxSizeMB)
		default:
			recordingStore = recordings.NewLocalStore(appConfig.RecordingStoragePath)
			slog.Info("Video recording enabled",
				"storage_backend", "local",
				"storage_path", appConfig.RecordingStoragePath,
				"max_size_mb", appConfig.RecordingMaxSizeMB)
		}

		recordingHandler = recordings.NewHandler(database, recordingStore, appConfig)

		if appConfig.RecordingRetentionDays > 0 {
			cleaner := recordings.NewCleaner(database, recordingStore, appConfig.RecordingRetentionDays)
			cleaner.Start()
			defer cleaner.Stop()
			slog.Info("Recording retention cleanup enabled", "retention_days", appConfig.RecordingRetentionDays)
		}
	}

	// Get the subdirectory from the embedded filesystem
	distFS, err := fs.Sub(embeddedFiles, "web/dist")
	if err != nil {
		slog.Error("failed to access embedded files", "error", err)
		os.Exit(1)
	}

	// Get the docs subdirectory from the embedded filesystem
	docsFS, err := fs.Sub(embeddedDocs, "docs-site/dist")
	if err != nil {
		slog.Warn("failed to access embedded docs", "error", err)
	}

	// Publish basic application metrics via expvar
	serverStartTime := time.Now()
	expvar.NewString("app.name").Set("sortie")
	expvar.NewString("app.start_time").Set(serverStartTime.UTC().Format(time.RFC3339))

	// Initialize diagnostics collector for enterprise support bundles
	diagCollector := diagnostics.NewCollector(database, appConfig, plugins.Global(), serverStartTime)

	// Publish session metrics for HPA custom metrics adapter
	expvar.Publish("sortie_active_sessions", expvar.Func(func() any {
		status := backpressureHandler.GetLoadStatus()
		return status.ActiveSessions
	}))
	expvar.Publish("sortie_queue_depth", expvar.Func(func() any {
		status := backpressureHandler.GetLoadStatus()
		return status.QueueDepth
	}))
	expvar.Publish("sortie_load_factor", expvar.Func(func() any {
		status := backpressureHandler.GetLoadStatus()
		return status.LoadFactor
	}))

	// Build the application handler using the server package
	app := &server.App{
		DB:                  database,
		SessionManager:      sessionManager,
		JWTAuth:             jwtAuthProvider,
		OIDCAuth:            oidcAuthProvider,
		GatewayHandler:      gwHandler,
		BackpressureHandler: backpressureHandler,
		FileHandler:         fileHandler,
		RecordingHandler:    recordingHandler,
		SSEHub:              sseHub,
		DiagCollector:       diagCollector,
		Config:              appConfig,
		StaticFS:            distFS,
		DocsFS:              docsFS,
	}

	handler := app.Handler()

	addr := fmt.Sprintf(":%d", appConfig.Port)
	slog.Info("Sortie server starting", "addr", "http://localhost"+addr)

	if err := http.ListenAndServe(addr, handler); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
