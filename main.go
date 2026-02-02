package main

import (
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

	"github.com/rjsadow/launchpad/internal/config"
	"github.com/rjsadow/launchpad/internal/db"
	"github.com/rjsadow/launchpad/internal/k8s"
	"github.com/rjsadow/launchpad/internal/middleware"
	"github.com/rjsadow/launchpad/internal/sessions"
	"github.com/rjsadow/launchpad/internal/websocket"
)

//go:embed all:web/dist
var embeddedFiles embed.FS

var database *db.DB
var sessionManager *sessions.Manager
var appConfig *config.Config

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

	// API routes
	http.HandleFunc("/api/apps", handleApps)
	http.HandleFunc("/api/apps/", handleAppByID)
	http.HandleFunc("/api/audit", handleAuditLogs)
	http.HandleFunc("/api/analytics/launch", handleAnalyticsLaunch)
	http.HandleFunc("/api/analytics/stats", handleAnalyticsStats)
	http.HandleFunc("/api/config", handleConfig)

	// Session API routes
	http.HandleFunc("/api/sessions", handleSessions)
	http.HandleFunc("/api/sessions/", handleSessionByID)

	// WebSocket route for session VNC streams
	http.Handle("/ws/sessions/", wsHandler)

	// Serve apps.json from database (for frontend compatibility)
	http.HandleFunc("/apps.json", handleAppsJSON)

	// Handle static files and SPA routing
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the file directly
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		// Check if file exists
		if _, err := fs.Stat(distFS, path[1:]); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		// For SPA routing, serve index.html for non-existent paths
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	addr := fmt.Sprintf(":%d", appConfig.Port)
	log.Printf("Launchpad server starting on http://localhost%s", addr)

	// Wrap default mux with security headers middleware
	handler := middleware.SecurityHeaders(http.DefaultServeMux)

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

		// URL is required for non-container apps, container_image is required for container apps
		if app.LaunchType == db.LaunchTypeContainer {
			if app.ContainerImage == "" {
				http.Error(w, "Missing required field for container app: container_image", http.StatusBadRequest)
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

		// URL is required for non-container apps, container_image is required for container apps
		if app.LaunchType == db.LaunchTypeContainer {
			if app.ContainerImage == "" {
				http.Error(w, "Missing required field for container app: container_image", http.StatusBadRequest)
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
	LogoURL        string `json:"logo_url"`
	PrimaryColor   string `json:"primary_color"`
	SecondaryColor string `json:"secondary_color"`
	TenantName     string `json:"tenant_name"`
}

// handleConfig returns tenant-specific branding configuration
func handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Use centralized config for branding
	brandingCfg := BrandingConfig{
		LogoURL:        appConfig.LogoURL,
		PrimaryColor:   appConfig.PrimaryColor,
		SecondaryColor: appConfig.SecondaryColor,
		TenantName:     appConfig.TenantName,
	}

	// Try to load overrides from config file if it exists
	if data, err := os.ReadFile(appConfig.BrandingConfigPath); err == nil {
		json.Unmarshal(data, &brandingCfg)
	}

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

		// Convert to response format with WebSocket URLs
		responses := make([]sessions.SessionResponse, len(sessionList))
		for i, s := range sessionList {
			app, _ := database.GetApp(s.AppID)
			appName := ""
			if app != nil {
				appName = app.Name
			}
			responses[i] = *sessions.SessionFromDB(&s, appName, sessionManager.GetSessionWebSocketURL(&s))
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

		// Get app name for response
		app, _ := database.GetApp(session.AppID)
		appName := ""
		if app != nil {
			appName = app.Name
		}

		response := sessions.SessionFromDB(session, appName, sessionManager.GetSessionWebSocketURL(session))

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

		// Get app name for response
		app, _ := database.GetApp(session.AppID)
		appName := ""
		if app != nil {
			appName = app.Name
		}

		response := sessions.SessionFromDB(session, appName, sessionManager.GetSessionWebSocketURL(session))

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
