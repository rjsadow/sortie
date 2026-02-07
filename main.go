package main

import (
	"context"
	"embed"
	"encoding/json"
	"expvar"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rjsadow/launchpad/internal/config"
	"github.com/rjsadow/launchpad/internal/db"
	"github.com/rjsadow/launchpad/internal/files"
	"github.com/rjsadow/launchpad/internal/guacamole"
	"github.com/rjsadow/launchpad/internal/k8s"
	"github.com/rjsadow/launchpad/internal/middleware"
	"github.com/rjsadow/launchpad/internal/plugins"
	"github.com/rjsadow/launchpad/internal/plugins/auth"
	"github.com/rjsadow/launchpad/internal/sessions"
	"github.com/rjsadow/launchpad/internal/websocket"
)

//go:embed all:web/dist
var embeddedFiles embed.FS

//go:embed web/src/data/templates.json
var embeddedTemplates []byte

var database *db.DB
var sessionManager *sessions.Manager
var appConfig *config.Config
var jwtAuthProvider *auth.JWTAuthProvider
var fileHandler *files.Handler

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
	flag.Parse()

	// Load configuration (env vars + flag overrides)
	var err error
	appConfig, err = config.LoadWithFlags(*port, *dbPath, *seedPath)
	if err != nil {
		slog.Error("configuration error", "error", err, "hint", "See .env.example for configuration options.")
		os.Exit(1)
	}

	// Initialize Kubernetes configuration
	k8s.Configure(appConfig.Namespace, appConfig.Kubeconfig, appConfig.VNCSidecarImage)
	k8s.ConfigureGuacdSidecar(appConfig.GuacdSidecarImage)

	// Initialize database
	database, err = db.Open(appConfig.DB)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

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
	jwtAuthProvider = auth.NewJWTAuthProvider()
	if appConfig.JWTSecret != "" {
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
		slog.Warn("LAUNCHPAD_JWT_SECRET not set - authentication disabled")
	}

	// Initialize session manager with config
	sessionManager = sessions.NewManagerWithConfig(database, sessions.ManagerConfig{
		SessionTimeout:  appConfig.SessionTimeout,
		CleanupInterval: appConfig.SessionCleanupInterval,
		PodReadyTimeout: appConfig.PodReadyTimeout,
	})
	sessionManager.Start()
	defer sessionManager.Stop()

	// Initialize WebSocket handler
	wsHandler := websocket.NewHandler(sessionManager)

	// Initialize file transfer handler
	fileHandler = files.NewHandler(sessionManager, database, appConfig.MaxUploadSize)

	// Get the subdirectory from the embedded filesystem
	distFS, err := fs.Sub(embeddedFiles, "web/dist")
	if err != nil {
		slog.Error("failed to access embedded files", "error", err)
		os.Exit(1)
	}

	// Create file server handler
	fileServer := http.FileServer(http.FS(distFS))

	// Publish basic application metrics via expvar
	expvar.NewString("app.name").Set("sortie")
	expvar.NewString("app.start_time").Set(time.Now().UTC().Format(time.RFC3339))

	// Create a custom mux for better control
	mux := http.NewServeMux()

	// Observability endpoints (public, no auth required)
	mux.Handle("/metrics", expvar.Handler())
	mux.HandleFunc("/healthz", handleHealthz)
	mux.HandleFunc("/readyz", handleReadyz)

	// Auth routes (public - no authentication required)
	mux.HandleFunc("/api/auth/login", handleLogin)
	mux.HandleFunc("/api/auth/logout", handleLogout)
	mux.HandleFunc("/api/auth/refresh", handleRefreshToken)
	mux.HandleFunc("/api/auth/me", handleAuthMe)
	mux.HandleFunc("/api/auth/register", handleRegister)

	// Config route (public - needed before login for branding)
	mux.HandleFunc("/api/config", handleConfig)

	// Protected API routes - wrapped with auth middleware
	authMiddleware := middleware.AuthMiddleware(jwtAuthProvider)
	requireAdmin := middleware.RequireRole(middleware.RoleAdmin)
	// Admin routes (protected, admin-only via RBAC middleware)
	mux.Handle("/api/admin/settings", authMiddleware(requireAdmin(http.HandlerFunc(handleAdminSettings))))
	mux.Handle("/api/admin/users", authMiddleware(requireAdmin(http.HandlerFunc(handleAdminUsers))))
	mux.Handle("/api/admin/users/", authMiddleware(requireAdmin(http.HandlerFunc(handleAdminUserByID))))
	mux.Handle("/api/admin/sessions", authMiddleware(requireAdmin(http.HandlerFunc(handleAdminSessions))))
	mux.Handle("/api/admin/templates", authMiddleware(requireAdmin(http.HandlerFunc(handleAdminTemplates))))
	mux.Handle("/api/admin/templates/", authMiddleware(requireAdmin(http.HandlerFunc(handleAdminTemplateByID))))

	// Public template endpoints (for template marketplace)
	mux.HandleFunc("/api/templates", handleTemplates)
	mux.HandleFunc("/api/templates/", handleTemplateByID)

	// App and AppSpec routes: GET is any authenticated user, mutations require app-author or admin
	mux.Handle("/api/appspecs", authMiddleware(http.HandlerFunc(handleAppSpecs)))
	mux.Handle("/api/appspecs/", authMiddleware(http.HandlerFunc(handleAppSpecByID)))

	mux.Handle("/api/apps", authMiddleware(http.HandlerFunc(handleApps)))
	mux.Handle("/api/apps/", authMiddleware(http.HandlerFunc(handleAppByID)))

	// Audit logs: admin only
	mux.Handle("/api/audit", authMiddleware(requireAdmin(http.HandlerFunc(handleAuditLogs))))
	mux.Handle("/api/audit/export", authMiddleware(requireAdmin(http.HandlerFunc(handleAuditExport))))
	mux.Handle("/api/audit/filters", authMiddleware(requireAdmin(http.HandlerFunc(handleAuditFilters))))

	// Analytics: stats admin-only, launch recording any authenticated user
	mux.Handle("/api/analytics/launch", authMiddleware(http.HandlerFunc(handleAnalyticsLaunch)))
	mux.Handle("/api/analytics/stats", authMiddleware(requireAdmin(http.HandlerFunc(handleAnalyticsStats))))

	// Session API routes (any authenticated user)
	mux.Handle("/api/sessions", authMiddleware(http.HandlerFunc(handleSessions)))
	mux.Handle("/api/sessions/", authMiddleware(http.HandlerFunc(handleSessionByID)))

	// WebSocket route for session VNC streams
	mux.Handle("/ws/sessions/", wsHandler)

	// WebSocket route for Guacamole (Windows RDP) streams
	guacHandler := guacamole.NewHandler(sessionManager)
	mux.Handle("/ws/guac/sessions/", guacHandler)

	// Serve apps.json from database (for frontend compatibility)
	mux.HandleFunc("/apps.json", handleAppsJSON)

	// Handle static files and SPA routing
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Try to serve the file directly
		if path == "/" {
			path = "/index.html"
		}

		// Set cache headers based on file type
		// HTML files: no-cache to ensure fresh content
		// Hashed assets (JS, CSS): cache for 1 year (immutable via filename hash)
		if strings.HasSuffix(path, ".html") || path == "/" {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		} else if strings.HasPrefix(path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}

		// Check if file exists
		if _, err := fs.Stat(distFS, path[1:]); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		// For SPA routing, serve index.html for non-existent paths
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	addr := fmt.Sprintf(":%d", appConfig.Port)
	slog.Info("Sortie server starting", "addr", "http://localhost"+addr)

	// Wrap mux with request ID and security headers middleware
	handler := middleware.SecurityHeaders(middleware.RequestID(mux))

	if err := http.ListenAndServe(addr, handler); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

// handleAppsJSON serves apps in the legacy JSON format for frontend compatibility
func handleAppsJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	apps, err := database.ListApps()
	if err != nil {
		slog.Error("error listing apps", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Return in the format the frontend expects
	response := db.AppConfig{Applications: apps}
	if response.Applications == nil {
		response.Applications = []db.Application{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleApps handles GET (list) and POST (create) for /api/apps
func handleApps(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		apps, err := database.ListApps()
		if err != nil {
			slog.Error("error listing apps", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if apps == nil {
			apps = []db.Application{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(apps)

	case http.MethodPost:
		user := middleware.GetUserFromContext(r.Context())
		if !middleware.HasRole(user.Roles, middleware.RoleAdmin, middleware.RoleAppAuthor) {
			http.Error(w, "Insufficient permissions", http.StatusForbidden)
			return
		}

		var app db.Application
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		if err := json.Unmarshal(body, &app); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if app.ID == "" || app.Name == "" {
			http.Error(w, "Missing required fields: id, name", http.StatusBadRequest)
			return
		}

		// URL is required for url apps, container_image is required for container/web_proxy apps
		if app.LaunchType == db.LaunchTypeContainer || app.LaunchType == db.LaunchTypeWebProxy {
			if app.ContainerImage == "" {
				http.Error(w, "Missing required field for container/web_proxy app: container_image", http.StatusBadRequest)
				return
			}
		} else if app.URL == "" {
			http.Error(w, "Missing required field: url", http.StatusBadRequest)
			return
		}

		if err := database.CreateApp(app); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				http.Error(w, "Application with this ID already exists", http.StatusConflict)
				return
			}
			slog.Error("error creating app", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Log the action
		details := fmt.Sprintf("Created app: %s (%s)", app.Name, app.ID)
		database.LogAudit(user.Username, "CREATE_APP", details)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(app)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAppByID handles GET, PUT, DELETE for /api/apps/{id}
func handleAppByID(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path
	id := strings.TrimPrefix(r.URL.Path, "/api/apps/")
	if id == "" {
		http.Error(w, "Missing app ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		app, err := database.GetApp(id)
		if err != nil {
			slog.Error("error getting app", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if app == nil {
			http.Error(w, "Application not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(app)

	case http.MethodPut:
		user := middleware.GetUserFromContext(r.Context())
		if !middleware.HasRole(user.Roles, middleware.RoleAdmin, middleware.RoleAppAuthor) {
			http.Error(w, "Insufficient permissions", http.StatusForbidden)
			return
		}

		var app db.Application
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		if err := json.Unmarshal(body, &app); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Use ID from URL path
		app.ID = id

		if app.Name == "" {
			http.Error(w, "Missing required field: name", http.StatusBadRequest)
			return
		}

		// URL is required for url apps, container_image is required for container/web_proxy apps
		if app.LaunchType == db.LaunchTypeContainer || app.LaunchType == db.LaunchTypeWebProxy {
			if app.ContainerImage == "" {
				http.Error(w, "Missing required field for container/web_proxy app: container_image", http.StatusBadRequest)
				return
			}
		} else if app.URL == "" {
			http.Error(w, "Missing required field: url", http.StatusBadRequest)
			return
		}

		if err := database.UpdateApp(app); err != nil {
			if err.Error() == "sql: no rows in result set" {
				http.Error(w, "Application not found", http.StatusNotFound)
				return
			}
			slog.Error("error updating app", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Log the action
		details := fmt.Sprintf("Updated app: %s (%s)", app.Name, app.ID)
		database.LogAudit(user.Username, "UPDATE_APP", details)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(app)

	case http.MethodDelete:
		user := middleware.GetUserFromContext(r.Context())
		if !middleware.HasRole(user.Roles, middleware.RoleAdmin, middleware.RoleAppAuthor) {
			http.Error(w, "Insufficient permissions", http.StatusForbidden)
			return
		}

		// Get app name before deleting for audit log
		app, err := database.GetApp(id)
		if err != nil {
			slog.Error("error getting app", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if app == nil {
			http.Error(w, "Application not found", http.StatusNotFound)
			return
		}

		if err := database.DeleteApp(id); err != nil {
			slog.Error("error deleting app", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Log the action
		details := fmt.Sprintf("Deleted app: %s (%s)", app.Name, id)
		database.LogAudit(user.Username, "DELETE_APP", details)

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAppSpecs handles GET (list) and POST (create) for /api/appspecs
func handleAppSpecs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		specs, err := database.ListAppSpecs()
		if err != nil {
			slog.Error("error listing app specs", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if specs == nil {
			specs = []db.AppSpec{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(specs)

	case http.MethodPost:
		user := middleware.GetUserFromContext(r.Context())
		if !middleware.HasRole(user.Roles, middleware.RoleAdmin, middleware.RoleAppAuthor) {
			http.Error(w, "Insufficient permissions", http.StatusForbidden)
			return
		}

		var spec db.AppSpec
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		if err := json.Unmarshal(body, &spec); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if spec.ID == "" || spec.Name == "" || spec.Image == "" {
			http.Error(w, "Missing required fields: id, name, image", http.StatusBadRequest)
			return
		}

		if err := database.CreateAppSpec(spec); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				http.Error(w, "AppSpec with this ID already exists", http.StatusConflict)
				return
			}
			slog.Error("error creating app spec", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		database.LogAudit(user.Username, "CREATE_APPSPEC", fmt.Sprintf("Created app spec: %s (%s)", spec.Name, spec.ID))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(spec)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAppSpecByID handles GET, PUT, DELETE for /api/appspecs/{id}
func handleAppSpecByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/appspecs/")
	if id == "" {
		http.Error(w, "Missing app spec ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		spec, err := database.GetAppSpec(id)
		if err != nil {
			slog.Error("error getting app spec", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if spec == nil {
			http.Error(w, "AppSpec not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(spec)

	case http.MethodPut:
		user := middleware.GetUserFromContext(r.Context())
		if !middleware.HasRole(user.Roles, middleware.RoleAdmin, middleware.RoleAppAuthor) {
			http.Error(w, "Insufficient permissions", http.StatusForbidden)
			return
		}

		var spec db.AppSpec
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		if err := json.Unmarshal(body, &spec); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		spec.ID = id

		if spec.Name == "" || spec.Image == "" {
			http.Error(w, "Missing required fields: name, image", http.StatusBadRequest)
			return
		}

		if err := database.UpdateAppSpec(spec); err != nil {
			if err.Error() == "sql: no rows in result set" {
				http.Error(w, "AppSpec not found", http.StatusNotFound)
				return
			}
			slog.Error("error updating app spec", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		database.LogAudit(user.Username, "UPDATE_APPSPEC", fmt.Sprintf("Updated app spec: %s (%s)", spec.Name, spec.ID))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(spec)

	case http.MethodDelete:
		user := middleware.GetUserFromContext(r.Context())
		if !middleware.HasRole(user.Roles, middleware.RoleAdmin, middleware.RoleAppAuthor) {
			http.Error(w, "Insufficient permissions", http.StatusForbidden)
			return
		}

		spec, err := database.GetAppSpec(id)
		if err != nil {
			slog.Error("error getting app spec", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if spec == nil {
			http.Error(w, "AppSpec not found", http.StatusNotFound)
			return
		}

		if err := database.DeleteAppSpec(id); err != nil {
			slog.Error("error deleting app spec", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		database.LogAudit(user.Username, "DELETE_APPSPEC", fmt.Sprintf("Deleted app spec: %s (%s)", spec.Name, id))

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAuditLogs returns audit log entries with optional filtering and pagination.
// Query params: user, action, from, to (RFC3339), limit, offset
func handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filter, err := parseAuditFilter(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	page, err := database.QueryAuditLogs(filter)
	if err != nil {
		slog.Error("error querying audit logs", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if page.Logs == nil {
		page.Logs = []db.AuditLog{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(page)
}

// handleAuditExport exports audit logs as JSON or CSV.
// Query params: same as handleAuditLogs plus format=json|csv
func handleAuditExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filter, err := parseAuditFilter(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// For export, allow up to 10000 rows
	if filter.Limit <= 0 || filter.Limit > 10000 {
		filter.Limit = 10000
	}
	filter.Offset = 0

	page, err := database.QueryAuditLogs(filter)
	if err != nil {
		slog.Error("error exporting audit logs", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if page.Logs == nil {
		page.Logs = []db.AuditLog{}
	}

	format := r.URL.Query().Get("format")
	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=audit_log.csv")
		writeAuditCSV(w, page.Logs)
		return
	}

	// Default: JSON
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=audit_log.json")
	json.NewEncoder(w).Encode(page.Logs)
}

// handleAuditFilters returns distinct users and actions for filter dropdowns
func handleAuditFilters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	actions, err := database.GetAuditLogActions()
	if err != nil {
		slog.Error("error getting audit actions", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	users, err := database.GetAuditLogUsers()
	if err != nil {
		slog.Error("error getting audit users", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if actions == nil {
		actions = []string{}
	}
	if users == nil {
		users = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string][]string{
		"actions": actions,
		"users":   users,
	})
}

// parseAuditFilter extracts audit log filter parameters from the request
func parseAuditFilter(r *http.Request) (db.AuditLogFilter, error) {
	q := r.URL.Query()
	filter := db.AuditLogFilter{
		User:   q.Get("user"),
		Action: q.Get("action"),
	}

	if from := q.Get("from"); from != "" {
		t, err := time.Parse(time.RFC3339, from)
		if err != nil {
			return filter, fmt.Errorf("invalid 'from' date: %w", err)
		}
		filter.From = t
	}
	if to := q.Get("to"); to != "" {
		t, err := time.Parse(time.RFC3339, to)
		if err != nil {
			return filter, fmt.Errorf("invalid 'to' date: %w", err)
		}
		filter.To = t
	}
	if limitStr := q.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return filter, fmt.Errorf("invalid 'limit': %w", err)
		}
		filter.Limit = limit
	}
	if offsetStr := q.Get("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil {
			return filter, fmt.Errorf("invalid 'offset': %w", err)
		}
		filter.Offset = offset
	}

	return filter, nil
}

// writeAuditCSV writes audit log entries as CSV to the writer
func writeAuditCSV(w io.Writer, logs []db.AuditLog) {
	fmt.Fprintf(w, "ID,Timestamp,User,Action,Details\n")
	for _, log := range logs {
		// Escape CSV fields that may contain commas or quotes
		details := strings.ReplaceAll(log.Details, "\"", "\"\"")
		fmt.Fprintf(w, "%d,%s,%s,%s,\"%s\"\n",
			log.ID,
			log.Timestamp.Format(time.RFC3339),
			log.User,
			log.Action,
			details,
		)
	}
}

// handleAnalyticsLaunch records an app launch
func handleAnalyticsLaunch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		AppID string `json:"app_id"`
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.AppID == "" {
		http.Error(w, "Missing required field: app_id", http.StatusBadRequest)
		return
	}

	if err := database.RecordLaunch(req.AppID); err != nil {
		slog.Error("error recording launch", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "recorded"})
}

// handleAnalyticsStats returns analytics statistics
func handleAnalyticsStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats, err := database.GetAnalyticsStats()
	if err != nil {
		slog.Error("error getting analytics stats", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// BrandingConfig represents tenant branding configuration
type BrandingConfig struct {
	LogoURL           string `json:"logo_url"`
	PrimaryColor      string `json:"primary_color"`
	SecondaryColor    string `json:"secondary_color"`
	TenantName        string `json:"tenant_name"`
	AllowRegistration bool   `json:"allow_registration"`
}

// handleConfig returns tenant-specific branding configuration
func handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Use centralized config for branding
	brandingCfg := BrandingConfig{
		LogoURL:           appConfig.LogoURL,
		PrimaryColor:      appConfig.PrimaryColor,
		SecondaryColor:    appConfig.SecondaryColor,
		TenantName:        appConfig.TenantName,
		AllowRegistration: isRegistrationAllowed(),
	}

	// Try to load overrides from config file if it exists
	if data, err := os.ReadFile(appConfig.BrandingConfigPath); err == nil {
		json.Unmarshal(data, &brandingCfg)
	}

	// Always use dynamic registration check (may be overridden in DB)
	brandingCfg.AllowRegistration = isRegistrationAllowed()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(brandingCfg)
}

// handleSessions handles GET (list) and POST (create) for /api/sessions
func handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Optional filter by user_id
		userID := r.URL.Query().Get("user_id")
		var sessionList []db.Session
		var err error

		if userID != "" {
			sessionList, err = sessionManager.ListSessionsByUser(r.Context(), userID)
		} else {
			sessionList, err = sessionManager.ListSessions(r.Context())
		}

		if err != nil {
			slog.Error("error listing sessions", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if sessionList == nil {
			sessionList = []db.Session{}
		}

		// Convert to response format with WebSocket/Proxy URLs
		responses := make([]sessions.SessionResponse, len(sessionList))
		for i, s := range sessionList {
			app, _ := database.GetApp(s.AppID)
			appName := ""
			wsURL := ""
			guacURL := ""
			proxyURL := ""
			if app != nil {
				appName = app.Name
				if app.LaunchType == db.LaunchTypeContainer || app.LaunchType == db.LaunchTypeWebProxy {
					if app.OsType == "windows" {
						guacURL = sessionManager.GetSessionGuacWebSocketURL(&s)
					} else {
						wsURL = sessionManager.GetSessionWebSocketURL(&s)
					}
				}
			}
			responses[i] = *sessions.SessionFromDB(&s, appName, wsURL, guacURL, proxyURL)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(responses)

	case http.MethodPost:
		var req sessions.CreateSessionRequest
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if req.AppID == "" {
			http.Error(w, "Missing required field: app_id", http.StatusBadRequest)
			return
		}

		// Default user ID if not provided
		if req.UserID == "" {
			req.UserID = "anonymous"
		}

		session, err := sessionManager.CreateSession(r.Context(), &req)
		if err != nil {
			slog.Error("error creating session", "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Get app name and URL for response
		app, _ := database.GetApp(session.AppID)
		appName := ""
		wsURL := ""
		guacURL := ""
		proxyURL := ""
		if app != nil {
			appName = app.Name
			if app.LaunchType == db.LaunchTypeContainer || app.LaunchType == db.LaunchTypeWebProxy {
				if app.OsType == "windows" {
					guacURL = sessionManager.GetSessionGuacWebSocketURL(session)
				} else {
					wsURL = sessionManager.GetSessionWebSocketURL(session)
				}
			}
		}

		response := sessions.SessionFromDB(session, appName, wsURL, guacURL, proxyURL)

		// Log the action
		details := fmt.Sprintf("Created session %s for app %s", session.ID, session.AppID)
		database.LogAudit(req.UserID, "CREATE_SESSION", details)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSessionByID handles GET and DELETE for /api/sessions/{id}
// and routes to sub-handlers for /api/sessions/{id}/stop and /api/sessions/{id}/restart
func handleSessionByID(w http.ResponseWriter, r *http.Request) {
	// Extract the path after /api/sessions/
	remainder := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	if remainder == "" {
		http.Error(w, "Missing session ID", http.StatusBadRequest)
		return
	}

	// Check for sub-path actions: {id}/stop, {id}/restart
	parts := strings.SplitN(remainder, "/", 2)
	id := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch {
	case action == "stop":
		handleSessionStop(w, r, id)
		return
	case action == "restart":
		handleSessionRestart(w, r, id)
		return
	case action == "files" || strings.HasPrefix(action, "files/"):
		// Delegate to file transfer handler (handles upload, download, list, delete)
		fileHandler.ServeHTTP(w, r)
		return
	case action == "":
		// Fall through to standard GET/DELETE handling
	default:
		http.Error(w, "Unknown session action", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		session, err := sessionManager.GetSession(r.Context(), id)
		if err != nil {
			slog.Error("error getting session", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if session == nil {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}

		// Get app name and URL for response
		app, _ := database.GetApp(session.AppID)
		appName := ""
		wsURL := ""
		guacURL := ""
		proxyURL := ""
		if app != nil {
			appName = app.Name
			if app.LaunchType == db.LaunchTypeContainer || app.LaunchType == db.LaunchTypeWebProxy {
				if app.OsType == "windows" {
					guacURL = sessionManager.GetSessionGuacWebSocketURL(session)
				} else {
					wsURL = sessionManager.GetSessionWebSocketURL(session)
				}
			}
		}

		response := sessions.SessionFromDB(session, appName, wsURL, guacURL, proxyURL)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)

	case http.MethodDelete:
		session, err := sessionManager.GetSession(r.Context(), id)
		if err != nil {
			slog.Error("error getting session", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if session == nil {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}

		if err := sessionManager.TerminateSession(r.Context(), id); err != nil {
			slog.Error("error terminating session", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Log the action
		details := fmt.Sprintf("Terminated session %s", id)
		database.LogAudit("admin", "TERMINATE_SESSION", details)

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSessionStop handles POST /api/sessions/{id}/stop
func handleSessionStop(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := sessionManager.StopSession(r.Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}
		if strings.Contains(err.Error(), "invalid session state") {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		slog.Error("error stopping session", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Fetch the updated session for the response
	session, err := sessionManager.GetSession(r.Context(), id)
	if err != nil {
		slog.Error("error getting session after stop", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	app, _ := database.GetApp(session.AppID)
	appName := ""
	if app != nil {
		appName = app.Name
	}
	response := sessions.SessionFromDB(session, appName, "", "", "")

	// Log the action
	database.LogAudit("user", "STOP_SESSION", fmt.Sprintf("Stopped session %s", id))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleSessionRestart handles POST /api/sessions/{id}/restart
func handleSessionRestart(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session, err := sessionManager.RestartSession(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}
		if strings.Contains(err.Error(), "must be stopped") {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		slog.Error("error restarting session", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Get app name and URL for response
	app, _ := database.GetApp(session.AppID)
	appName := ""
	wsURL := ""
	guacURL := ""
	proxyURL := ""
	if app != nil {
		appName = app.Name
		if app.LaunchType == db.LaunchTypeContainer || app.LaunchType == db.LaunchTypeWebProxy {
			if app.OsType == "windows" {
				guacURL = sessionManager.GetSessionGuacWebSocketURL(session)
			} else {
				wsURL = sessionManager.GetSessionWebSocketURL(session)
			}
		}
	}

	response := sessions.SessionFromDB(session, appName, wsURL, guacURL, proxyURL)

	// Log the action
	database.LogAudit("user", "RESTART_SESSION", fmt.Sprintf("Restarted session %s", id))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleLogin handles POST /api/auth/login
func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if jwtAuthProvider == nil || appConfig.JWTSecret == "" {
		http.Error(w, "Authentication not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		http.Error(w, "Username and password are required", http.StatusBadRequest)
		return
	}

	result, err := jwtAuthProvider.LoginWithCredentials(r.Context(), req.Username, req.Password)
	if err != nil {
		slog.Warn("login failed", "username", req.Username, "error", err)
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Log the login
	database.LogAudit(req.Username, "LOGIN", "User logged in")

	// Set access token cookie for iframe/proxy requests
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.AccessTokenCookieName,
		Value:    result.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(result.ExpiresIn),
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleLogout handles POST /api/auth/logout
func handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Clear the access token cookie
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.AccessTokenCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1, // Delete the cookie
	})

	// For JWT, logout is client-side (discard tokens)
	// We just return success
	w.WriteHeader(http.StatusNoContent)
}

// handleRefreshToken handles POST /api/auth/refresh
func handleRefreshToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if jwtAuthProvider == nil || appConfig.JWTSecret == "" {
		http.Error(w, "Authentication not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		RefreshToken string `json:"refresh_token"`
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.RefreshToken == "" {
		http.Error(w, "Refresh token is required", http.StatusBadRequest)
		return
	}

	result, err := jwtAuthProvider.RefreshAccessToken(r.Context(), req.RefreshToken)
	if err != nil {
		slog.Warn("token refresh failed", "error", err)
		http.Error(w, "Invalid refresh token", http.StatusUnauthorized)
		return
	}

	// Update the access token cookie
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.AccessTokenCookieName,
		Value:    result.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(result.ExpiresIn),
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleAuthMe handles GET /api/auth/me
func handleAuthMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if jwtAuthProvider == nil || appConfig.JWTSecret == "" {
		http.Error(w, "Authentication not configured", http.StatusServiceUnavailable)
		return
	}

	// Extract and validate token
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Authorization header required", http.StatusUnauthorized)
		return
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
		return
	}

	token := parts[1]
	result, err := jwtAuthProvider.Authenticate(r.Context(), token)
	if err != nil || !result.Authenticated {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result.User)
}

// isRegistrationAllowed checks if user registration is currently allowed
func isRegistrationAllowed() bool {
	// Check database setting first (admin override)
	if dbSetting, err := database.GetSetting("allow_registration"); err == nil && dbSetting != "" {
		return strings.EqualFold(dbSetting, "true") || dbSetting == "1"
	}
	// Fall back to config
	return appConfig.AllowRegistration
}

// handleRegister handles POST /api/auth/register
func handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if jwtAuthProvider == nil || appConfig.JWTSecret == "" {
		http.Error(w, "Authentication not configured", http.StatusServiceUnavailable)
		return
	}

	if !isRegistrationAllowed() {
		http.Error(w, "Registration is not enabled", http.StatusForbidden)
		return
	}

	var req struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		http.Error(w, "Username and password are required", http.StatusBadRequest)
		return
	}

	if req.Email == "" {
		http.Error(w, "Email is required", http.StatusBadRequest)
		return
	}

	if len(req.Password) < 6 {
		http.Error(w, "Password must be at least 6 characters", http.StatusBadRequest)
		return
	}

	// Check if username already exists
	existing, err := database.GetUserByUsername(req.Username)
	if err != nil {
		slog.Error("error checking username", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if existing != nil {
		http.Error(w, "Username already taken", http.StatusConflict)
		return
	}

	// Hash password
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		slog.Error("error hashing password", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Create user
	user := db.User{
		ID:           fmt.Sprintf("user-%s-%d", req.Username, time.Now().UnixNano()),
		Username:     req.Username,
		Email:        req.Email,
		DisplayName:  req.DisplayName,
		PasswordHash: passwordHash,
		Roles:        []string{"user"},
	}

	if err := database.CreateUser(user); err != nil {
		slog.Error("error creating user", "error", err)
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	// Log the registration
	database.LogAudit(req.Username, "REGISTER", "User registered")

	// Auto-login: generate tokens
	result, err := jwtAuthProvider.LoginWithCredentials(r.Context(), req.Username, req.Password)
	if err != nil {
		slog.Error("error generating tokens after registration", "error", err)
		// Registration succeeded, but login failed - still return success
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"message": "Registration successful, please login"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}

// handleAdminSettings handles GET/PUT /api/admin/settings
func handleAdminSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := database.GetAllSettings()
		if err != nil {
			slog.Error("error getting settings", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Include default values for settings not in DB
		response := map[string]interface{}{
			"allow_registration": isRegistrationAllowed(),
		}

		// Override with DB settings
		for k, v := range settings {
			response[k] = v
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)

	case http.MethodPut:
		var req map[string]string
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Update each setting
		for key, value := range req {
			if err := database.SetSetting(key, value); err != nil {
				slog.Error("error updating setting", "key", key, "error", err)
				http.Error(w, "Failed to update settings", http.StatusInternalServerError)
				return
			}
		}

		// Log the action
		user := middleware.GetUserFromContext(r.Context())
		database.LogAudit(user.Username, "UPDATE_SETTINGS", fmt.Sprintf("Updated settings: %v", req))

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAdminSessions handles GET /api/admin/sessions (admin only)
func handleAdminSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionList, err := sessionManager.ListSessions(r.Context())
	if err != nil {
		slog.Error("error listing sessions", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if sessionList == nil {
		sessionList = []db.Session{}
	}

	// Convert to response format with WebSocket/Proxy URLs
	responses := make([]sessions.SessionResponse, len(sessionList))
	for i, s := range sessionList {
		app, _ := database.GetApp(s.AppID)
		appName := ""
		wsURL := ""
		guacURL := ""
		proxyURL := ""
		if app != nil {
			appName = app.Name
			if app.LaunchType == db.LaunchTypeContainer || app.LaunchType == db.LaunchTypeWebProxy {
				if app.OsType == "windows" {
					guacURL = sessionManager.GetSessionGuacWebSocketURL(&s)
				} else {
					wsURL = sessionManager.GetSessionWebSocketURL(&s)
				}
			}
		}
		responses[i] = *sessions.SessionFromDB(&s, appName, wsURL, guacURL, proxyURL)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responses)
}

// handleAdminUsers handles GET/POST /api/admin/users
func handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		users, err := database.ListUsers()
		if err != nil {
			slog.Error("error listing users", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if users == nil {
			users = []db.User{}
		}

		// Remove password hashes from response
		type userResponse struct {
			ID          string    `json:"id"`
			Username    string    `json:"username"`
			Email       string    `json:"email,omitempty"`
			DisplayName string    `json:"display_name,omitempty"`
			Roles       []string  `json:"roles"`
			CreatedAt   time.Time `json:"created_at"`
		}

		response := make([]userResponse, len(users))
		for i, u := range users {
			response[i] = userResponse{
				ID:          u.ID,
				Username:    u.Username,
				Email:       u.Email,
				DisplayName: u.DisplayName,
				Roles:       u.Roles,
				CreatedAt:   u.CreatedAt,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)

	case http.MethodPost:
		var req struct {
			Username    string   `json:"username"`
			Password    string   `json:"password"`
			Email       string   `json:"email"`
			DisplayName string   `json:"display_name"`
			Roles       []string `json:"roles"`
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if req.Username == "" || req.Password == "" {
			http.Error(w, "Username and password are required", http.StatusBadRequest)
			return
		}

		// Check if username already exists
		existing, err := database.GetUserByUsername(req.Username)
		if err != nil {
			slog.Error("error checking username", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if existing != nil {
			http.Error(w, "Username already exists", http.StatusConflict)
			return
		}

		// Hash password
		passwordHash, err := auth.HashPassword(req.Password)
		if err != nil {
			slog.Error("error hashing password", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Default roles
		roles := req.Roles
		if len(roles) == 0 {
			roles = []string{"user"}
		}

		// Create user
		user := db.User{
			ID:           fmt.Sprintf("user-%s-%d", req.Username, time.Now().UnixNano()),
			Username:     req.Username,
			Email:        req.Email,
			DisplayName:  req.DisplayName,
			PasswordHash: passwordHash,
			Roles:        roles,
		}

		if err := database.CreateUser(user); err != nil {
			slog.Error("error creating user", "error", err)
			http.Error(w, "Failed to create user", http.StatusInternalServerError)
			return
		}

		// Log the action
		adminUser := middleware.GetUserFromContext(r.Context())
		database.LogAudit(adminUser.Username, "CREATE_USER", fmt.Sprintf("Created user: %s", req.Username))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"id":       user.ID,
			"username": user.Username,
			"message":  "User created successfully",
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAdminUserByID handles DELETE /api/admin/users/{id}
func handleAdminUserByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/admin/users/")
	if id == "" {
		http.Error(w, "User ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		// Prevent deleting self
		currentUser := middleware.GetUserFromContext(r.Context())
		if currentUser != nil && currentUser.ID == id {
			http.Error(w, "Cannot delete your own account", http.StatusBadRequest)
			return
		}

		// Get user info for audit log
		user, err := database.GetUserByID(id)
		if err != nil {
			slog.Error("error getting user", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if user == nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		if err := database.DeleteUser(id); err != nil {
			slog.Error("error deleting user", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Log the action
		database.LogAudit(currentUser.Username, "DELETE_USER", fmt.Sprintf("Deleted user: %s (%s)", user.Username, id))

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleTemplates handles GET /api/templates (public)
func handleTemplates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	templates, err := database.ListTemplates()
	if err != nil {
		slog.Error("error listing templates", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if templates == nil {
		templates = []db.Template{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(templates)
}

// handleTemplateByID handles GET /api/templates/{id} (public)
func handleTemplateByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	templateID := strings.TrimPrefix(r.URL.Path, "/api/templates/")
	if templateID == "" {
		http.Error(w, "Template ID required", http.StatusBadRequest)
		return
	}

	template, err := database.GetTemplate(templateID)
	if err != nil {
		slog.Error("error getting template", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if template == nil {
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(template)
}

// handleAdminTemplates handles GET/POST /api/admin/templates (admin only)
func handleAdminTemplates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		templates, err := database.ListTemplates()
		if err != nil {
			slog.Error("error listing templates", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if templates == nil {
			templates = []db.Template{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(templates)

	case http.MethodPost:
		var template db.Template
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		if err := json.Unmarshal(body, &template); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Validate required fields
		if template.TemplateID == "" {
			http.Error(w, "Missing required field: template_id", http.StatusBadRequest)
			return
		}
		if template.Name == "" {
			http.Error(w, "Missing required field: name", http.StatusBadRequest)
			return
		}
		if template.TemplateCategory == "" {
			http.Error(w, "Missing required field: template_category", http.StatusBadRequest)
			return
		}
		if template.Category == "" {
			http.Error(w, "Missing required field: category", http.StatusBadRequest)
			return
		}

		// Set defaults
		if template.TemplateVersion == "" {
			template.TemplateVersion = "1.0.0"
		}
		if template.LaunchType == "" {
			template.LaunchType = "container"
		}

		if err := database.CreateTemplate(template); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				http.Error(w, "Template with this ID already exists", http.StatusConflict)
				return
			}
			slog.Error("error creating template", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Log the action
		user := middleware.GetUserFromContext(r.Context())
		database.LogAudit(user.Username, "CREATE_TEMPLATE", fmt.Sprintf("Created template: %s (%s)", template.Name, template.TemplateID))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(template)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAdminTemplateByID handles PUT/DELETE /api/admin/templates/{id} (admin only)
func handleAdminTemplateByID(w http.ResponseWriter, r *http.Request) {
	templateID := strings.TrimPrefix(r.URL.Path, "/api/admin/templates/")
	if templateID == "" {
		http.Error(w, "Template ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		template, err := database.GetTemplate(templateID)
		if err != nil {
			slog.Error("error getting template", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if template == nil {
			http.Error(w, "Template not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(template)

	case http.MethodPut:
		var template db.Template
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		if err := json.Unmarshal(body, &template); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Use ID from URL path
		template.TemplateID = templateID

		// Validate required fields
		if template.Name == "" {
			http.Error(w, "Missing required field: name", http.StatusBadRequest)
			return
		}

		if err := database.UpdateTemplate(template); err != nil {
			if err.Error() == "sql: no rows in result set" {
				http.Error(w, "Template not found", http.StatusNotFound)
				return
			}
			slog.Error("error updating template", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Log the action
		user := middleware.GetUserFromContext(r.Context())
		database.LogAudit(user.Username, "UPDATE_TEMPLATE", fmt.Sprintf("Updated template: %s (%s)", template.Name, templateID))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(template)

	case http.MethodDelete:
		// Get template info for audit log
		template, err := database.GetTemplate(templateID)
		if err != nil {
			slog.Error("error getting template", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if template == nil {
			http.Error(w, "Template not found", http.StatusNotFound)
			return
		}

		if err := database.DeleteTemplate(templateID); err != nil {
			slog.Error("error deleting template", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Log the action
		user := middleware.GetUserFromContext(r.Context())
		database.LogAudit(user.Username, "DELETE_TEMPLATE", fmt.Sprintf("Deleted template: %s (%s)", template.Name, templateID))

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleHealthz is a liveness probe that returns 200 if the server is running.
func handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleReadyz is a readiness probe that checks database connectivity and plugin health.
func handleReadyz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ready := true
	checks := make(map[string]interface{})

	// Check database connectivity
	if err := database.Ping(); err != nil {
		ready = false
		checks["database"] = map[string]string{"status": "unhealthy", "error": err.Error()}
	} else {
		checks["database"] = map[string]string{"status": "healthy"}
	}

	// Check plugin health via the global registry
	pluginStatuses := plugins.Global().HealthCheck(r.Context())
	pluginChecks := make([]map[string]interface{}, 0, len(pluginStatuses))
	for _, ps := range pluginStatuses {
		entry := map[string]interface{}{
			"name":    ps.PluginName,
			"type":    ps.PluginType,
			"healthy": ps.Healthy,
			"message": ps.Message,
		}
		if !ps.Healthy {
			ready = false
		}
		pluginChecks = append(pluginChecks, entry)
	}
	checks["plugins"] = pluginChecks

	w.Header().Set("Content-Type", "application/json")
	if ready {
		checks["status"] = "ready"
		w.WriteHeader(http.StatusOK)
	} else {
		checks["status"] = "not_ready"
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(checks)
}
