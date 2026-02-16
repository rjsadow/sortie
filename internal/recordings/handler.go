package recordings

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/rjsadow/sortie/internal/config"
	"github.com/rjsadow/sortie/internal/db"
	"github.com/rjsadow/sortie/internal/middleware"
)

// Handler handles video recording HTTP requests.
type Handler struct {
	database *db.DB
	store    RecordingStore
	config   *config.Config
}

// NewHandler creates a new recording handler.
func NewHandler(database *db.DB, store RecordingStore, cfg *config.Config) *Handler {
	return &Handler{
		database: database,
		store:    store,
		config:   cfg,
	}
}

// ServeHTTP routes recording requests.
// Expected paths:
//   - POST   /api/sessions/{id}/recording/start
//   - POST   /api/sessions/{id}/recording/stop
//   - POST   /api/sessions/{id}/recording/upload
//   - GET    /api/recordings
//   - GET    /api/recordings/{id}/download
//   - DELETE /api/recordings/{id}
//   - GET    /api/admin/recordings
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Admin recordings list
	if path == "/api/admin/recordings" {
		h.handleAdminRecordings(w, r)
		return
	}

	// User recordings list
	if path == "/api/recordings" {
		h.handleUserRecordings(w, r)
		return
	}

	// Recording by ID routes: /api/recordings/{id}/...
	if remainder, ok := strings.CutPrefix(path, "/api/recordings/"); ok {
		parts := strings.SplitN(remainder, "/", 2)
		recordingID := parts[0]
		action := ""
		if len(parts) > 1 {
			action = parts[1]
		}

		switch {
		case action == "download" && r.Method == http.MethodGet:
			h.handleDownload(w, r, recordingID)
		case action == "" && r.Method == http.MethodDelete:
			h.handleDelete(w, r, recordingID)
		default:
			http.Error(w, "Not found", http.StatusNotFound)
		}
		return
	}

	// Session recording routes: /api/sessions/{id}/recording/...
	if strings.HasPrefix(path, "/api/sessions/") && strings.Contains(path, "/recording/") {
		remainder := strings.TrimPrefix(path, "/api/sessions/")
		parts := strings.SplitN(remainder, "/recording/", 2)
		if len(parts) != 2 {
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}
		sessionID := parts[0]
		action := parts[1]

		// Validate session exists and is running
		session, err := h.database.GetSession(sessionID)
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
			if !slices.Contains(user.Roles, "admin") {
				http.Error(w, "Access denied", http.StatusForbidden)
				return
			}
		}

		switch action {
		case "start":
			h.handleStart(w, r, session)
		case "stop":
			h.handleStop(w, r)
		case "upload":
			h.handleUpload(w, r, session)
		default:
			http.Error(w, "Unknown action", http.StatusNotFound)
		}
		return
	}

	http.Error(w, "Not found", http.StatusNotFound)
}

func (h *Handler) handleStart(w http.ResponseWriter, r *http.Request, session *db.Session) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUserFromContext(r.Context())
	userID := ""
	if user != nil {
		userID = user.ID
	}

	id := fmt.Sprintf("rec-%d", time.Now().UnixNano())
	now := time.Now()

	rec := db.Recording{
		ID:             id,
		SessionID:      session.ID,
		UserID:         userID,
		Filename:       fmt.Sprintf("%s-%s.vncrec", session.AppID, now.Format("20060102-150405")),
		Format:         "vncrec",
		StorageBackend: h.config.RecordingStorageBackend,
		Status:         db.RecordingStatusRecording,
		TenantID:       session.TenantID,
		CreatedAt:      now,
	}

	if err := h.database.CreateRecording(rec); err != nil {
		slog.Error("failed to create recording", "error", err)
		http.Error(w, "Failed to create recording", http.StatusInternalServerError)
		return
	}

	h.database.LogAudit(userID, "RECORDING_START", fmt.Sprintf("Started recording %s for session %s", id, session.ID))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"recording_id": id,
		"status":       "recording",
	})
}

func (h *Handler) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		RecordingID string `json:"recording_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RecordingID == "" {
		http.Error(w, "Missing recording_id", http.StatusBadRequest)
		return
	}

	if err := h.database.UpdateRecordingStatus(body.RecordingID, db.RecordingStatusUploading); err != nil {
		slog.Error("failed to update recording status", "error", err)
		http.Error(w, "Failed to update recording", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "uploading"})
}

func (h *Handler) handleUpload(w http.ResponseWriter, r *http.Request, session *db.Session) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	maxSize := int64(h.config.RecordingMaxSizeMB) * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, maxSize)

	if err := r.ParseMultipartForm(maxSize); err != nil {
		if strings.Contains(err.Error(), "http: request body too large") {
			http.Error(w, fmt.Sprintf("Recording too large (max %d MB)", h.config.RecordingMaxSizeMB), http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "Failed to parse upload", http.StatusBadRequest)
		return
	}

	recordingID := r.FormValue("recording_id")
	if recordingID == "" {
		http.Error(w, "Missing recording_id", http.StatusBadRequest)
		return
	}

	durationStr := r.FormValue("duration")
	var duration float64
	if durationStr != "" {
		fmt.Sscanf(durationStr, "%f", &duration)
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Missing 'file' field in upload", http.StatusBadRequest)
		return
	}
	defer file.Close()

	storagePath, err := h.store.Save(recordingID, file)
	if err != nil {
		slog.Error("failed to save recording", "error", err)
		if uerr := h.database.UpdateRecordingStatus(recordingID, db.RecordingStatusFailed); uerr != nil {
			slog.Error("failed to mark recording as failed", "error", uerr)
		}
		http.Error(w, "Failed to save recording", http.StatusInternalServerError)
		return
	}

	if err := h.database.UpdateRecordingComplete(recordingID, storagePath, header.Size, duration); err != nil {
		slog.Error("failed to update recording", "error", err)
		http.Error(w, "Failed to finalize recording", http.StatusInternalServerError)
		return
	}

	user := middleware.GetUserFromContext(r.Context())
	userID := ""
	if user != nil {
		userID = user.ID
	}
	h.database.LogAudit(userID, "RECORDING_UPLOAD", fmt.Sprintf("Uploaded recording %s for session %s (%d bytes)", recordingID, session.ID, header.Size))

	// Trigger background video conversion for local storage
	responseStatus := db.RecordingStatusReady
	if h.config.RecordingStorageBackend == "local" {
		responseStatus = db.RecordingStatusProcessing
		if err := h.database.UpdateRecordingStatus(recordingID, db.RecordingStatusProcessing); err != nil {
			slog.Error("failed to set processing status", "error", err)
		}
		go h.convertToVideo(recordingID, storagePath)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": string(responseStatus)})
}

// convertToVideo converts a .vncrec recording to MP4 in the background.
func (h *Handler) convertToVideo(recordingID, storagePath string) {
	baseDir := filepath.Clean(h.config.RecordingStoragePath)
	inputPath := filepath.Clean(filepath.Join(baseDir, storagePath))
	videoRelPath := strings.TrimSuffix(storagePath, filepath.Ext(storagePath)) + ".mp4"
	outputPath := filepath.Clean(filepath.Join(baseDir, videoRelPath))

	// Validate paths stay within the storage directory
	if !strings.HasPrefix(inputPath, baseDir+string(filepath.Separator)) ||
		!strings.HasPrefix(outputPath, baseDir+string(filepath.Separator)) {
		slog.Error("Video conversion: path traversal detected",
			"recording_id", recordingID, "storage_path", storagePath)
		return
	}

	slog.Info("Starting video conversion", "recording_id", recordingID, "input", inputPath)

	if err := ConvertToMP4(inputPath, outputPath); err != nil {
		slog.Error("Video conversion failed", "recording_id", recordingID, "error", err)
		if uerr := h.database.UpdateRecordingStatus(recordingID, db.RecordingStatusFailed); uerr != nil {
			slog.Error("failed to mark recording as failed after conversion error", "error", uerr)
		}
		return
	}

	if err := h.database.UpdateRecordingVideoPath(recordingID, videoRelPath); err != nil {
		slog.Error("Failed to update video path in DB", "recording_id", recordingID, "error", err)
		if uerr := h.database.UpdateRecordingStatus(recordingID, db.RecordingStatusFailed); uerr != nil {
			slog.Error("failed to mark recording as failed after DB error", "error", uerr)
		}
		return
	}

	if err := h.database.UpdateRecordingStatus(recordingID, db.RecordingStatusReady); err != nil {
		slog.Error("failed to set ready status after conversion", "recording_id", recordingID, "error", err)
	}

	slog.Info("Video conversion complete", "recording_id", recordingID, "video_path", videoRelPath)
}

func (h *Handler) handleUserRecordings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	recs, err := h.database.ListRecordingsByUser(user.ID)
	if err != nil {
		slog.Error("failed to list recordings", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if recs == nil {
		recs = []db.Recording{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(recs)
}

func (h *Handler) handleAdminRecordings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	recs, err := h.database.ListAllRecordings()
	if err != nil {
		slog.Error("failed to list recordings", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if recs == nil {
		recs = []db.Recording{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(recs)
}

func (h *Handler) handleDownload(w http.ResponseWriter, r *http.Request, recordingID string) {
	rec, err := h.database.GetRecording(recordingID)
	if err != nil {
		slog.Error("failed to get recording", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if rec == nil {
		http.Error(w, "Recording not found", http.StatusNotFound)
		return
	}

	// Check access: owner or admin
	user := middleware.GetUserFromContext(r.Context())
	if user != nil && rec.UserID != user.ID {
		if !slices.Contains(user.Roles, "admin") {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}
	}

	if rec.Status != db.RecordingStatusReady {
		http.Error(w, "Recording not ready", http.StatusConflict)
		return
	}

	// Prefer serving the converted MP4 video if available
	servePath := rec.StoragePath
	filename := rec.Filename
	contentType := "application/octet-stream"

	if rec.VideoPath != "" {
		// Try to serve the MP4 video
		reader, verr := h.store.Get(rec.VideoPath)
		if verr == nil {
			defer reader.Close()
			filename = strings.TrimSuffix(rec.Filename, filepath.Ext(rec.Filename)) + ".mp4"
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
			w.Header().Set("Content-Type", "video/mp4")
			io.Copy(w, reader)
			return
		}
		slog.Warn("MP4 video not available, falling back to vncrec", "recording_id", recordingID, "error", verr)
	}

	// Fall back to original vncrec
	reader, err := h.store.Get(servePath)
	if err != nil {
		slog.Error("failed to open recording file", "error", err)
		http.Error(w, "Failed to read recording", http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	if rec.Format == "webm" {
		contentType = "video/webm"
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Type", contentType)
	io.Copy(w, reader)
}

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request, recordingID string) {
	rec, err := h.database.GetRecording(recordingID)
	if err != nil {
		slog.Error("failed to get recording", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if rec == nil {
		http.Error(w, "Recording not found", http.StatusNotFound)
		return
	}

	// Check access: owner or admin
	user := middleware.GetUserFromContext(r.Context())
	if user != nil && rec.UserID != user.ID {
		if !slices.Contains(user.Roles, "admin") {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}
	}

	// Delete storage files if they exist
	if rec.StoragePath != "" {
		if err := h.store.Delete(rec.StoragePath); err != nil {
			slog.Warn("failed to delete recording file", "error", err, "path", rec.StoragePath)
		}
	}
	if rec.VideoPath != "" {
		if err := h.store.Delete(rec.VideoPath); err != nil {
			slog.Warn("failed to delete video file", "error", err, "path", rec.VideoPath)
		}
	}

	if err := h.database.DeleteRecording(recordingID); err != nil {
		slog.Error("failed to delete recording record", "error", err)
		http.Error(w, "Failed to delete recording", http.StatusInternalServerError)
		return
	}

	userID := ""
	if user != nil {
		userID = user.ID
	}
	h.database.LogAudit(userID, "RECORDING_DELETE", fmt.Sprintf("Deleted recording %s", recordingID))

	w.WriteHeader(http.StatusNoContent)
}
