package files

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/rjsadow/launchpad/internal/db"
	"github.com/rjsadow/launchpad/internal/middleware"
	"github.com/rjsadow/launchpad/internal/sessions"
)

// Handler handles file transfer HTTP requests for session workspaces.
type Handler struct {
	sessionManager *sessions.Manager
	database       *db.DB
	maxUploadSize  int64
}

// NewHandler creates a new file transfer handler.
func NewHandler(sm *sessions.Manager, database *db.DB, maxUploadSize int64) *Handler {
	return &Handler{
		sessionManager: sm,
		database:       database,
		maxUploadSize:  maxUploadSize,
	}
}

// ServeHTTP routes file transfer requests.
// Expected paths:
//   - POST   /api/sessions/{id}/files/upload
//   - GET    /api/sessions/{id}/files/download?path=<path>
//   - GET    /api/sessions/{id}/files
//   - DELETE /api/sessions/{id}/files?path=<path>
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract session ID and sub-action from the path
	// Path format: /api/sessions/{id}/files[/upload|/download]
	remainder := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	parts := strings.SplitN(remainder, "/", 3) // [id, "files", action?]

	if len(parts) < 2 || parts[1] != "files" {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	sessionID := parts[0]
	action := ""
	if len(parts) > 2 {
		action = parts[2]
	}

	// Validate session exists and is running
	session, err := h.sessionManager.GetSession(r.Context(), sessionID)
	if err != nil {
		slog.Error("error getting session", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if session == nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	if session.Status != db.SessionStatusRunning {
		http.Error(w, "Session is not running", http.StatusConflict)
		return
	}

	// Validate user owns the session
	user := middleware.GetUserFromContext(r.Context())
	if user != nil && session.UserID != user.ID && session.UserID != user.Username {
		// Check if admin
		isAdmin := false
		for _, role := range user.Roles {
			if role == "admin" {
				isAdmin = true
				break
			}
		}
		if !isAdmin {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}
	}

	switch action {
	case "upload":
		h.handleUpload(w, r, session)
	case "download":
		h.handleDownload(w, r, session)
	case "":
		switch r.Method {
		case http.MethodGet:
			h.handleList(w, r, session)
		case http.MethodDelete:
			h.handleDelete(w, r, session)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	default:
		http.Error(w, "Unknown action", http.StatusNotFound)
	}
}

// handleUpload handles POST /api/sessions/{id}/files/upload
func (h *Handler) handleUpload(w http.ResponseWriter, r *http.Request, session *db.Session) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadSize)

	// Parse multipart form
	if err := r.ParseMultipartForm(h.maxUploadSize); err != nil {
		if strings.Contains(err.Error(), "http: request body too large") {
			http.Error(w, fmt.Sprintf("File too large (max %d bytes)", h.maxUploadSize), http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "Failed to parse upload", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Missing 'file' field in upload", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Optional target path (subdirectory within workspace)
	targetPath := r.FormValue("path")
	filename := header.Filename
	if targetPath != "" {
		filename = strings.TrimPrefix(targetPath, "/") + "/" + filename
	}

	if err := UploadFile(r.Context(), session.PodName, filename, file, header.Size); err != nil {
		slog.Error("file upload failed", "session", session.ID, "filename", filename, "error", err)
		http.Error(w, "Upload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Audit log
	h.database.LogAudit(session.UserID, "FILE_UPLOAD", fmt.Sprintf("Uploaded %s to session %s", filename, session.ID))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"status":   "uploaded",
		"filename": filename,
	})
}

// handleDownload handles GET /api/sessions/{id}/files/download?path=<path>
func (h *Handler) handleDownload(w http.ResponseWriter, r *http.Request, session *db.Session) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "Missing 'path' query parameter", http.StatusBadRequest)
		return
	}

	// Set content disposition for download
	parts := strings.Split(filePath, "/")
	filename := parts[len(parts)-1]
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Type", "application/octet-stream")

	if err := DownloadFile(r.Context(), session.PodName, filePath, w); err != nil {
		// Can't set error headers after starting to write body,
		// but if we haven't written anything yet (error happened early), reset headers
		if strings.Contains(err.Error(), "file not found") {
			w.Header().Del("Content-Disposition")
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		slog.Error("file download failed", "session", session.ID, "path", filePath, "error", err)
		http.Error(w, "Download failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Audit log
	h.database.LogAudit(session.UserID, "FILE_DOWNLOAD", fmt.Sprintf("Downloaded %s from session %s", filePath, session.ID))
}

// handleList handles GET /api/sessions/{id}/files?path=<path>
func (h *Handler) handleList(w http.ResponseWriter, r *http.Request, session *db.Session) {
	dirPath := r.URL.Query().Get("path")
	if dirPath == "" {
		dirPath = "."
	}

	files, err := ListFiles(r.Context(), session.PodName, dirPath)
	if err != nil {
		if strings.Contains(err.Error(), "directory not found") {
			http.Error(w, "Directory not found", http.StatusNotFound)
			return
		}
		slog.Error("file listing failed", "session", session.ID, "path", dirPath, "error", err)
		http.Error(w, "Failed to list files: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

// handleDelete handles DELETE /api/sessions/{id}/files?path=<path>
func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request, session *db.Session) {
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "Missing 'path' query parameter", http.StatusBadRequest)
		return
	}

	if err := DeleteFile(r.Context(), session.PodName, filePath); err != nil {
		slog.Error("file deletion failed", "session", session.ID, "path", filePath, "error", err)
		http.Error(w, "Delete failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Audit log
	h.database.LogAudit(session.UserID, "FILE_DELETE", fmt.Sprintf("Deleted %s from session %s", filePath, session.ID))

	w.WriteHeader(http.StatusNoContent)
}
