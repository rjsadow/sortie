package recordings

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rjsadow/sortie/internal/config"
	"github.com/rjsadow/sortie/internal/db"
	"github.com/rjsadow/sortie/internal/middleware"
	"github.com/rjsadow/sortie/internal/plugins"
)

func setupTestDB(t *testing.T) *db.DB {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "rec-handler-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	database, err := db.Open(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	return database
}

func setupTestHandler(t *testing.T) (*Handler, *db.DB, *LocalStore) {
	t.Helper()
	database := setupTestDB(t)
	storeDir := t.TempDir()
	store := NewLocalStore(storeDir)
	cfg := &config.Config{
		RecordingStorageBackend: "local",
		RecordingStoragePath:    storeDir,
		RecordingMaxSizeMB:      10,
	}
	handler := NewHandler(database, store, cfg)

	// Create a prerequisite app and running session
	app := db.Application{
		ID: "test-app", Name: "Test App", Description: "d",
		URL: "http://x", Icon: "i", Category: "c",
		LaunchType: db.LaunchTypeContainer,
	}
	if err := database.CreateApp(app); err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}

	now := time.Now().Truncate(time.Second)
	session := db.Session{
		ID: "test-sess", UserID: "user-1", AppID: "test-app",
		PodName: "pod-1", Status: db.SessionStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := database.CreateSession(session); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	return handler, database, store
}

func reqWithUser(r *http.Request, user *plugins.User) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.UserContextKey, user)
	return r.WithContext(ctx)
}

func ownerUser() *plugins.User {
	return &plugins.User{ID: "user-1", Username: "owner", Roles: []string{"user"}}
}

func adminUser() *plugins.User {
	return &plugins.User{ID: "admin-1", Username: "admin", Roles: []string{"admin", "user"}}
}

func otherUser() *plugins.User {
	return &plugins.User{ID: "user-2", Username: "other", Roles: []string{"user"}}
}

func TestHandler_StartRecording(t *testing.T) {
	handler, _, _ := setupTestHandler(t)

	t.Run("start recording success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/sessions/test-sess/recording/start", nil)
		req = reqWithUser(req, ownerUser())
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d (body: %s)", rr.Code, http.StatusCreated, rr.Body.String())
		}

		var resp map[string]string
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp["recording_id"] == "" {
			t.Error("recording_id should not be empty")
		}
		if resp["status"] != "recording" {
			t.Errorf("status = %s, want recording", resp["status"])
		}
	})

	t.Run("start recording wrong method", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/sessions/test-sess/recording/start", nil)
		req = reqWithUser(req, ownerUser())
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("start recording nonexistent session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/sessions/nonexistent/recording/start", nil)
		req = reqWithUser(req, ownerUser())
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusNotFound)
		}
	})

	t.Run("start recording access denied for non-owner", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/sessions/test-sess/recording/start", nil)
		req = reqWithUser(req, otherUser())
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusForbidden)
		}
	})

	t.Run("start recording allowed for admin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/sessions/test-sess/recording/start", nil)
		req = reqWithUser(req, adminUser())
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Errorf("status = %d, want %d (body: %s)", rr.Code, http.StatusCreated, rr.Body.String())
		}
	})
}

func TestHandler_StopRecording(t *testing.T) {
	handler, database, _ := setupTestHandler(t)

	// Create a recording to stop
	now := time.Now()
	rec := db.Recording{
		ID: "stop-rec", SessionID: "test-sess", UserID: "user-1",
		Filename: "stop.webm", Format: "webm", StorageBackend: "local",
		Status: db.RecordingStatusRecording, CreatedAt: now,
	}
	if err := database.CreateRecording(rec); err != nil {
		t.Fatalf("CreateRecording() error = %v", err)
	}

	t.Run("stop recording success", func(t *testing.T) {
		body := `{"recording_id":"stop-rec"}`
		req := httptest.NewRequest(http.MethodPost, "/api/sessions/test-sess/recording/stop", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = reqWithUser(req, ownerUser())
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body: %s)", rr.Code, http.StatusOK, rr.Body.String())
		}

		// Verify status was updated
		got, _ := database.GetRecording("stop-rec")
		if got.Status != db.RecordingStatusUploading {
			t.Errorf("status = %s, want uploading", got.Status)
		}
	})

	t.Run("stop recording missing recording_id", func(t *testing.T) {
		body := `{}`
		req := httptest.NewRequest(http.MethodPost, "/api/sessions/test-sess/recording/stop", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = reqWithUser(req, ownerUser())
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
		}
	})
}

func TestHandler_UploadRecording(t *testing.T) {
	handler, database, _ := setupTestHandler(t)

	// Create a recording to upload to
	now := time.Now()
	rec := db.Recording{
		ID: "upload-rec", SessionID: "test-sess", UserID: "user-1",
		Filename: "upload.webm", Format: "webm", StorageBackend: "local",
		Status: db.RecordingStatusUploading, CreatedAt: now,
	}
	if err := database.CreateRecording(rec); err != nil {
		t.Fatalf("CreateRecording() error = %v", err)
	}

	t.Run("upload recording success", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		writer.WriteField("recording_id", "upload-rec")
		writer.WriteField("duration", "30.5")
		part, err := writer.CreateFormFile("file", "upload-rec.webm")
		if err != nil {
			t.Fatalf("CreateFormFile() error = %v", err)
		}
		part.Write([]byte("fake video content"))
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/api/sessions/test-sess/recording/upload", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req = reqWithUser(req, ownerUser())
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body: %s)", rr.Code, http.StatusOK, rr.Body.String())
		}

		// Verify recording was marked as ready
		got, _ := database.GetRecording("upload-rec")
		if got.Status != db.RecordingStatusReady {
			t.Errorf("status = %s, want ready", got.Status)
		}
		if got.SizeBytes == 0 {
			t.Error("SizeBytes should not be 0")
		}
		if got.StoragePath == "" {
			t.Error("StoragePath should not be empty")
		}
		if got.CompletedAt == nil {
			t.Error("CompletedAt should not be nil")
		}
	})

	t.Run("upload recording missing file", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		writer.WriteField("recording_id", "upload-rec")
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/api/sessions/test-sess/recording/upload", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req = reqWithUser(req, ownerUser())
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("upload recording missing recording_id", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, _ := writer.CreateFormFile("file", "test.webm")
		part.Write([]byte("data"))
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/api/sessions/test-sess/recording/upload", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req = reqWithUser(req, ownerUser())
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
		}
	})
}

func TestHandler_ListRecordings(t *testing.T) {
	handler, database, _ := setupTestHandler(t)

	now := time.Now()
	rec := db.Recording{
		ID: "list-rec", SessionID: "test-sess", UserID: "user-1",
		Filename: "list.webm", Format: "webm", StorageBackend: "local",
		Status: db.RecordingStatusReady, CreatedAt: now,
	}
	if err := database.CreateRecording(rec); err != nil {
		t.Fatalf("CreateRecording() error = %v", err)
	}

	t.Run("list user recordings", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/recordings", nil)
		req = reqWithUser(req, ownerUser())
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body: %s)", rr.Code, http.StatusOK, rr.Body.String())
		}

		var recs []db.Recording
		if err := json.NewDecoder(rr.Body).Decode(&recs); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if len(recs) != 1 {
			t.Fatalf("expected 1 recording, got %d", len(recs))
		}
		if recs[0].ID != "list-rec" {
			t.Errorf("recording ID = %s, want list-rec", recs[0].ID)
		}
	})

	t.Run("list user recordings empty for other user", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/recordings", nil)
		req = reqWithUser(req, otherUser())
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
		}

		var recs []db.Recording
		json.NewDecoder(rr.Body).Decode(&recs)
		if len(recs) != 0 {
			t.Errorf("expected 0 recordings for other user, got %d", len(recs))
		}
	})

	t.Run("list user recordings no auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/recordings", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
		}
	})

	t.Run("admin list all recordings", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/admin/recordings", nil)
		req = reqWithUser(req, adminUser())
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body: %s)", rr.Code, http.StatusOK, rr.Body.String())
		}

		var recs []db.Recording
		json.NewDecoder(rr.Body).Decode(&recs)
		if len(recs) != 1 {
			t.Errorf("expected 1 recording, got %d", len(recs))
		}
	})
}

func TestHandler_DownloadRecording(t *testing.T) {
	handler, database, store := setupTestHandler(t)

	// Save a file and create a recording record
	content := []byte("video data for download test")
	storagePath, err := store.Save("dl-rec", bytes.NewReader(content))
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	now := time.Now()
	rec := db.Recording{
		ID: "dl-rec", SessionID: "test-sess", UserID: "user-1",
		Filename: "download.webm", Format: "webm", StorageBackend: "local",
		StoragePath: storagePath, Status: db.RecordingStatusReady,
		SizeBytes: int64(len(content)), CreatedAt: now,
	}
	if err := database.CreateRecording(rec); err != nil {
		t.Fatalf("CreateRecording() error = %v", err)
	}

	t.Run("download recording success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/recordings/dl-rec/download", nil)
		req = reqWithUser(req, ownerUser())
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body: %s)", rr.Code, http.StatusOK, rr.Body.String())
		}

		body, _ := io.ReadAll(rr.Body)
		if !bytes.Equal(body, content) {
			t.Errorf("body length = %d, want %d", len(body), len(content))
		}

		ct := rr.Header().Get("Content-Type")
		if ct != "video/webm" {
			t.Errorf("Content-Type = %s, want video/webm", ct)
		}

		cd := rr.Header().Get("Content-Disposition")
		if !strings.Contains(cd, "download.webm") {
			t.Errorf("Content-Disposition = %s, should contain filename", cd)
		}
	})

	t.Run("download recording access denied", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/recordings/dl-rec/download", nil)
		req = reqWithUser(req, otherUser())
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusForbidden)
		}
	})

	t.Run("download recording admin access", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/recordings/dl-rec/download", nil)
		req = reqWithUser(req, adminUser())
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
		}
	})

	t.Run("download nonexistent recording", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/recordings/nonexistent/download", nil)
		req = reqWithUser(req, ownerUser())
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusNotFound)
		}
	})
}

func TestHandler_DownloadNotReady(t *testing.T) {
	handler, database, _ := setupTestHandler(t)

	now := time.Now()
	rec := db.Recording{
		ID: "notready-rec", SessionID: "test-sess", UserID: "user-1",
		Filename: "notready.webm", Format: "webm", StorageBackend: "local",
		Status: db.RecordingStatusRecording, CreatedAt: now,
	}
	if err := database.CreateRecording(rec); err != nil {
		t.Fatalf("CreateRecording() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/recordings/notready-rec/download", nil)
	req = reqWithUser(req, ownerUser())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d (body: %s)", rr.Code, http.StatusConflict, rr.Body.String())
	}
}

func TestHandler_DeleteRecording(t *testing.T) {
	handler, database, store := setupTestHandler(t)

	// Save a file
	content := []byte("video to delete")
	storagePath, _ := store.Save("del-rec", bytes.NewReader(content))

	now := time.Now()
	rec := db.Recording{
		ID: "del-rec", SessionID: "test-sess", UserID: "user-1",
		Filename: "delete.webm", Format: "webm", StorageBackend: "local",
		StoragePath: storagePath, Status: db.RecordingStatusReady,
		CreatedAt: now,
	}
	if err := database.CreateRecording(rec); err != nil {
		t.Fatalf("CreateRecording() error = %v", err)
	}

	t.Run("delete recording access denied", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/recordings/del-rec", nil)
		req = reqWithUser(req, otherUser())
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusForbidden)
		}
	})

	t.Run("delete recording success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/recordings/del-rec", nil)
		req = reqWithUser(req, ownerUser())
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d (body: %s)", rr.Code, http.StatusNoContent, rr.Body.String())
		}

		// Verify DB record is gone
		got, _ := database.GetRecording("del-rec")
		if got != nil {
			t.Error("recording should be deleted from database")
		}

		// Verify file is gone
		_, err := store.Get(storagePath)
		if err == nil {
			t.Error("storage file should be deleted")
		}
	})

	t.Run("delete nonexistent recording", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/recordings/nonexistent", nil)
		req = reqWithUser(req, ownerUser())
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusNotFound)
		}
	})
}

func TestHandler_SessionNotRunning(t *testing.T) {
	handler, database, _ := setupTestHandler(t)

	// Stop the session
	database.UpdateSessionStatus("test-sess", db.SessionStatusStopped)

	req := httptest.NewRequest(http.MethodPost, "/api/sessions/test-sess/recording/start", nil)
	req = reqWithUser(req, ownerUser())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d (body: %s)", rr.Code, http.StatusConflict, rr.Body.String())
	}
}

func TestHandler_UnknownPaths(t *testing.T) {
	handler, _, _ := setupTestHandler(t)

	tests := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{"unknown session action", http.MethodPost, "/api/sessions/test-sess/recording/unknown", http.StatusNotFound},
		{"unknown top-level path", http.MethodGet, "/api/something", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req = reqWithUser(req, ownerUser())
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.want {
				t.Errorf("status = %d, want %d (body: %s)", rr.Code, tt.want, rr.Body.String())
			}
		})
	}
}
