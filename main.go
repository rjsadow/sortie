package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rjsadow/launchpad/internal/config"
	"github.com/rjsadow/launchpad/internal/db"
	"github.com/rjsadow/launchpad/internal/k8s"
	"github.com/rjsadow/launchpad/internal/middleware"
	"github.com/rjsadow/launchpad/internal/plugins/auth"
	"github.com/rjsadow/launchpad/internal/sessions"
	"github.com/rjsadow/launchpad/internal/websocket"
)

//go:embed all:web/dist
var embeddedFiles embed.FS

var database *db.DB
var sessionManager *sessions.Manager
var appConfig *config.Config
var jwtAuthProvider *auth.JWTAuthProvider

func main() {
	// Parse command-line flags (can override env vars)
	port := flag.Int("port", config.DefaultPort, "Port to listen on")
	dbPath := flag.String("db", config.DefaultDBPath, "Path to SQLite database")
	seedPath := flag.String("seed", "", "Path to apps.json for initial seeding")
	flag.Parse()

	// Load configuration (env vars + flag overrides)
	var err error
	appConfig, err = config.LoadWithFlags(*port, *dbPath, *seedPath)
	if err != nil {
		log.Fatalf("Configuration error:\n%v\n\nSee .env.example for configuration options.", err)
	}

	// Initialize Kubernetes configuration
	k8s.Configure(appConfig.Namespace, appConfig.Kubeconfig, appConfig.VNCSidecarImage)

	// Initialize database
	database, err = db.Open(appConfig.DB)
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}
	defer database.Close()

	// Seed from JSON if provided and database is empty
	if appConfig.Seed != "" {
		if err := database.SeedFromJSON(appConfig.Seed); err != nil {
			log.Printf("Warning: failed to seed from JSON: %v", err)
		}
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
			log.Fatalf("Failed to initialize JWT auth provider: %v", err)
		}
		jwtAuthProvider.SetDatabase(database)

		// Seed admin user if password is configured
		if appConfig.AdminPassword != "" {
			passwordHash, err := auth.HashPassword(appConfig.AdminPassword)
			if err != nil {
				log.Fatalf("Failed to hash admin password: %v", err)
			}
			if err := database.SeedAdminUser(appConfig.AdminUsername, passwordHash); err != nil {
				log.Printf("Warning: failed to seed admin user: %v", err)
			} else {
				log.Printf("Admin user '%s' ready", appConfig.AdminUsername)
			}
		}
	} else {
		log.Println("Warning: LAUNCHPAD_JWT_SECRET not set - authentication disabled")
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

	// Get the subdirectory from the embedded filesystem
	distFS, err := fs.Sub(embeddedFiles, "web/dist")
	if err != nil {
		log.Fatal("Failed to access embedded files:", err)
	}

	// Create file server handler
	fileServer := http.FileServer(http.FS(distFS))

	// Create a custom mux for better control
	mux := http.NewServeMux()

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

	// Admin routes (protected, admin-only checked in handlers)
	mux.Handle("/api/admin/settings", authMiddleware(http.HandlerFunc(handleAdminSettings)))
	mux.Handle("/api/admin/users", authMiddleware(http.HandlerFunc(handleAdminUsers)))
	mux.Handle("/api/admin/users/", authMiddleware(http.HandlerFunc(handleAdminUserByID)))

	mux.Handle("/api/apps", authMiddleware(http.HandlerFunc(handleApps)))
	mux.Handle("/api/apps/", authMiddleware(http.HandlerFunc(handleAppByID)))
	mux.Handle("/api/audit", authMiddleware(http.HandlerFunc(handleAuditLogs)))
	mux.Handle("/api/analytics/launch", authMiddleware(http.HandlerFunc(handleAnalyticsLaunch)))
	mux.Handle("/api/analytics/stats", authMiddleware(http.HandlerFunc(handleAnalyticsStats)))

	// Session API routes (protected)
	mux.Handle("/api/sessions", authMiddleware(http.HandlerFunc(handleSessions)))
	mux.Handle("/api/sessions/", authMiddleware(http.HandlerFunc(handleSessionByID)))

	// WebSocket route for session VNC streams
	mux.Handle("/ws/sessions/", wsHandler)

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
	log.Printf("Launchpad server starting on http://localhost%s", addr)

	// Wrap mux with security headers middleware
	handler := middleware.SecurityHeaders(mux)

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal("Server error:", err)
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
		log.Printf("Error listing apps: %v", err)
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
			log.Printf("Error listing apps: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if apps == nil {
			apps = []db.Application{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(apps)

	case http.MethodPost:
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
			log.Printf("Error creating app: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Log the action
		details := fmt.Sprintf("Created app: %s (%s)", app.Name, app.ID)
		database.LogAudit("admin", "CREATE_APP", details)

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
			log.Printf("Error getting app: %v", err)
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
			log.Printf("Error updating app: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Log the action
		details := fmt.Sprintf("Updated app: %s (%s)", app.Name, app.ID)
		database.LogAudit("admin", "UPDATE_APP", details)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(app)

	case http.MethodDelete:
		// Get app name before deleting for audit log
		app, err := database.GetApp(id)
		if err != nil {
			log.Printf("Error getting app: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if app == nil {
			http.Error(w, "Application not found", http.StatusNotFound)
			return
		}

		if err := database.DeleteApp(id); err != nil {
			log.Printf("Error deleting app: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Log the action
		details := fmt.Sprintf("Deleted app: %s (%s)", app.Name, id)
		database.LogAudit("admin", "DELETE_APP", details)

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAuditLogs returns recent audit log entries
func handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	logs, err := database.GetAuditLogs(100)
	if err != nil {
		log.Printf("Error getting audit logs: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if logs == nil {
		logs = []db.AuditLog{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
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
		log.Printf("Error recording launch: %v", err)
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
		log.Printf("Error getting analytics stats: %v", err)
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
			log.Printf("Error listing sessions: %v", err)
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
			proxyURL := ""
			if app != nil {
				appName = app.Name
				if app.LaunchType == db.LaunchTypeContainer || app.LaunchType == db.LaunchTypeWebProxy {
					// Both container and web_proxy apps use VNC streaming
					wsURL = sessionManager.GetSessionWebSocketURL(&s)
				}
			}
			responses[i] = *sessions.SessionFromDB(&s, appName, wsURL, proxyURL)
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
			log.Printf("Error creating session: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Get app name and URL for response
		app, _ := database.GetApp(session.AppID)
		appName := ""
		wsURL := ""
		proxyURL := ""
		if app != nil {
			appName = app.Name
			if app.LaunchType == db.LaunchTypeContainer || app.LaunchType == db.LaunchTypeWebProxy {
				// Both container and web_proxy apps use VNC streaming
				wsURL = sessionManager.GetSessionWebSocketURL(session)
			}
		}

		response := sessions.SessionFromDB(session, appName, wsURL, proxyURL)

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
func handleSessionByID(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path
	id := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	if id == "" {
		http.Error(w, "Missing session ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		session, err := sessionManager.GetSession(r.Context(), id)
		if err != nil {
			log.Printf("Error getting session: %v", err)
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
		proxyURL := ""
		if app != nil {
			appName = app.Name
			if app.LaunchType == db.LaunchTypeContainer || app.LaunchType == db.LaunchTypeWebProxy {
				// Both container and web_proxy apps use VNC streaming
				wsURL = sessionManager.GetSessionWebSocketURL(session)
			}
		}

		response := sessions.SessionFromDB(session, appName, wsURL, proxyURL)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)

	case http.MethodDelete:
		session, err := sessionManager.GetSession(r.Context(), id)
		if err != nil {
			log.Printf("Error getting session: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if session == nil {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}

		if err := sessionManager.TerminateSession(r.Context(), id); err != nil {
			log.Printf("Error terminating session: %v", err)
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
		log.Printf("Login failed for user %s: %v", req.Username, err)
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
		log.Printf("Token refresh failed: %v", err)
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
		log.Printf("Error checking username: %v", err)
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
		log.Printf("Error hashing password: %v", err)
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
		log.Printf("Error creating user: %v", err)
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	// Log the registration
	database.LogAudit(req.Username, "REGISTER", "User registered")

	// Auto-login: generate tokens
	result, err := jwtAuthProvider.LoginWithCredentials(r.Context(), req.Username, req.Password)
	if err != nil {
		log.Printf("Error generating tokens after registration: %v", err)
		// Registration succeeded, but login failed - still return success
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"message": "Registration successful, please login"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}

// isAdmin checks if the user from context has admin role
func isAdmin(r *http.Request) bool {
	user := middleware.GetUserFromContext(r.Context())
	if user == nil {
		return false
	}
	for _, role := range user.Roles {
		if role == "admin" {
			return true
		}
	}
	return false
}

// handleAdminSettings handles GET/PUT /api/admin/settings
func handleAdminSettings(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}

	switch r.Method {
	case http.MethodGet:
		settings, err := database.GetAllSettings()
		if err != nil {
			log.Printf("Error getting settings: %v", err)
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
				log.Printf("Error setting %s: %v", key, err)
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

// handleAdminUsers handles GET/POST /api/admin/users
func handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}

	switch r.Method {
	case http.MethodGet:
		users, err := database.ListUsers()
		if err != nil {
			log.Printf("Error listing users: %v", err)
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
			log.Printf("Error checking username: %v", err)
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
			log.Printf("Error hashing password: %v", err)
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
			log.Printf("Error creating user: %v", err)
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
	if !isAdmin(r) {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}

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
			log.Printf("Error getting user: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if user == nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		if err := database.DeleteUser(id); err != nil {
			log.Printf("Error deleting user: %v", err)
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
