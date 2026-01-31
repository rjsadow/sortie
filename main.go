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

	"github.com/rjsadow/launchpad/internal/db"
)

//go:embed web/dist/*
var embeddedFiles embed.FS

var database *db.DB

func main() {
	port := flag.Int("port", 8080, "Port to listen on")
	dbPath := flag.String("db", "launchpad.db", "Path to SQLite database")
	seedPath := flag.String("seed", "", "Path to apps.json for initial seeding")
	flag.Parse()

	// Initialize database
	var err error
	database, err = db.Open(*dbPath)
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}
	defer database.Close()

	// Seed from JSON if provided and database is empty
	if *seedPath != "" {
		if err := database.SeedFromJSON(*seedPath); err != nil {
			log.Printf("Warning: failed to seed from JSON: %v", err)
		}
	}

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

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Launchpad server starting on http://localhost%s", addr)

	if err := http.ListenAndServe(addr, nil); err != nil {
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

		if app.ID == "" || app.Name == "" || app.URL == "" {
			http.Error(w, "Missing required fields: id, name, url", http.StatusBadRequest)
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

		if app.Name == "" || app.URL == "" {
			http.Error(w, "Missing required fields: name, url", http.StatusBadRequest)
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

	// Default branding - can be overridden via config file
	config := BrandingConfig{
		LogoURL:        "",
		PrimaryColor:   "#398D9B",
		SecondaryColor: "#4AB7C3",
		TenantName:     "Launchpad",
	}

	// Try to load from config file if it exists
	configPath := os.Getenv("LAUNCHPAD_CONFIG")
	if configPath == "" {
		configPath = "branding.json"
	}

	if data, err := os.ReadFile(configPath); err == nil {
		json.Unmarshal(data, &config)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}
