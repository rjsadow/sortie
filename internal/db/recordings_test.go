package db

import (
	"database/sql"
	"testing"
	"time"
)

func TestRecordingCRUD(t *testing.T) {
	db := setupTestDB(t)

	// Create prerequisite app and session
	app := Application{
		ID: "rec-app", Name: "Rec App", Description: "d",
		URL: "http://x", Icon: "i", Category: "c",
		LaunchType: LaunchTypeContainer,
	}
	if err := db.CreateApp(app); err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}

	now := time.Now().Truncate(time.Second)
	session := Session{
		ID: "rec-sess-1", UserID: "user-1", AppID: "rec-app",
		PodName: "pod-1", Status: SessionStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.CreateSession(session); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	t.Run("create and get recording", func(t *testing.T) {
		rec := Recording{
			ID:             "rec-1",
			SessionID:      "rec-sess-1",
			UserID:         "user-1",
			Filename:       "test-recording.webm",
			Format:         "webm",
			StorageBackend: "local",
			Status:         RecordingStatusRecording,
			CreatedAt:      now,
		}
		if err := db.CreateRecording(rec); err != nil {
			t.Fatalf("CreateRecording() error = %v", err)
		}

		got, err := db.GetRecording("rec-1")
		if err != nil {
			t.Fatalf("GetRecording() error = %v", err)
		}
		if got == nil {
			t.Fatal("GetRecording() returned nil")
		}
		if got.ID != "rec-1" {
			t.Errorf("ID = %s, want rec-1", got.ID)
		}
		if got.SessionID != "rec-sess-1" {
			t.Errorf("SessionID = %s, want rec-sess-1", got.SessionID)
		}
		if got.UserID != "user-1" {
			t.Errorf("UserID = %s, want user-1", got.UserID)
		}
		if got.Filename != "test-recording.webm" {
			t.Errorf("Filename = %s, want test-recording.webm", got.Filename)
		}
		if got.Format != "webm" {
			t.Errorf("Format = %s, want webm", got.Format)
		}
		if got.Status != RecordingStatusRecording {
			t.Errorf("Status = %s, want recording", got.Status)
		}
		if got.CompletedAt != nil {
			t.Errorf("CompletedAt should be nil, got %v", got.CompletedAt)
		}
		if got.TenantID != DefaultTenantID {
			t.Errorf("TenantID = %s, want %s", got.TenantID, DefaultTenantID)
		}
	})

	t.Run("get nonexistent recording", func(t *testing.T) {
		got, err := db.GetRecording("nonexistent")
		if err != nil {
			t.Fatalf("GetRecording() error = %v", err)
		}
		if got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("update recording status", func(t *testing.T) {
		if err := db.UpdateRecordingStatus("rec-1", RecordingStatusUploading); err != nil {
			t.Fatalf("UpdateRecordingStatus() error = %v", err)
		}

		got, err := db.GetRecording("rec-1")
		if err != nil {
			t.Fatalf("GetRecording() error = %v", err)
		}
		if got.Status != RecordingStatusUploading {
			t.Errorf("Status = %s, want uploading", got.Status)
		}
	})

	t.Run("update recording status nonexistent", func(t *testing.T) {
		err := db.UpdateRecordingStatus("nonexistent", RecordingStatusReady)
		if err != sql.ErrNoRows {
			t.Errorf("expected sql.ErrNoRows, got %v", err)
		}
	})

	t.Run("update recording complete", func(t *testing.T) {
		if err := db.UpdateRecordingComplete("rec-1", "2026/02/rec-1.webm", 1024000, 65.5); err != nil {
			t.Fatalf("UpdateRecordingComplete() error = %v", err)
		}

		got, err := db.GetRecording("rec-1")
		if err != nil {
			t.Fatalf("GetRecording() error = %v", err)
		}
		if got.Status != RecordingStatusReady {
			t.Errorf("Status = %s, want ready", got.Status)
		}
		if got.StoragePath != "2026/02/rec-1.webm" {
			t.Errorf("StoragePath = %s, want 2026/02/rec-1.webm", got.StoragePath)
		}
		if got.SizeBytes != 1024000 {
			t.Errorf("SizeBytes = %d, want 1024000", got.SizeBytes)
		}
		if got.DurationSeconds != 65.5 {
			t.Errorf("DurationSeconds = %f, want 65.5", got.DurationSeconds)
		}
		if got.CompletedAt == nil {
			t.Error("CompletedAt should not be nil after completion")
		}
	})

	t.Run("update recording complete nonexistent", func(t *testing.T) {
		err := db.UpdateRecordingComplete("nonexistent", "path", 100, 1.0)
		if err != sql.ErrNoRows {
			t.Errorf("expected sql.ErrNoRows, got %v", err)
		}
	})
}

func TestRecordingListMethods(t *testing.T) {
	db := setupTestDB(t)

	app := Application{
		ID: "list-app", Name: "List App", Description: "d",
		URL: "http://x", Icon: "i", Category: "c",
		LaunchType: LaunchTypeContainer,
	}
	if err := db.CreateApp(app); err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}

	now := time.Now().Truncate(time.Second)
	for _, sid := range []string{"list-sess-1", "list-sess-2"} {
		s := Session{
			ID: sid, UserID: "user-1", AppID: "list-app",
			PodName: "pod-" + sid, Status: SessionStatusRunning,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := db.CreateSession(s); err != nil {
			t.Fatalf("CreateSession(%s) error = %v", sid, err)
		}
	}

	// Create recordings for different users and sessions
	recordings := []Recording{
		{ID: "lr-1", SessionID: "list-sess-1", UserID: "user-1", Filename: "a.webm", Format: "webm", StorageBackend: "local", Status: RecordingStatusReady, CreatedAt: now},
		{ID: "lr-2", SessionID: "list-sess-1", UserID: "user-1", Filename: "b.webm", Format: "webm", StorageBackend: "local", Status: RecordingStatusReady, CreatedAt: now.Add(time.Second)},
		{ID: "lr-3", SessionID: "list-sess-2", UserID: "user-2", Filename: "c.webm", Format: "webm", StorageBackend: "local", Status: RecordingStatusRecording, CreatedAt: now.Add(2 * time.Second)},
	}
	for _, rec := range recordings {
		if err := db.CreateRecording(rec); err != nil {
			t.Fatalf("CreateRecording(%s) error = %v", rec.ID, err)
		}
	}

	t.Run("list by user", func(t *testing.T) {
		recs, err := db.ListRecordingsByUser("user-1")
		if err != nil {
			t.Fatalf("ListRecordingsByUser() error = %v", err)
		}
		if len(recs) != 2 {
			t.Fatalf("expected 2 recordings, got %d", len(recs))
		}
		// Should be ordered by created_at DESC
		if recs[0].ID != "lr-2" {
			t.Errorf("first recording ID = %s, want lr-2 (most recent)", recs[0].ID)
		}
	})

	t.Run("list by user no results", func(t *testing.T) {
		recs, err := db.ListRecordingsByUser("nobody")
		if err != nil {
			t.Fatalf("ListRecordingsByUser() error = %v", err)
		}
		if len(recs) != 0 {
			t.Errorf("expected 0 recordings, got %d", len(recs))
		}
	})

	t.Run("list by session", func(t *testing.T) {
		recs, err := db.ListRecordingsBySession("list-sess-1")
		if err != nil {
			t.Fatalf("ListRecordingsBySession() error = %v", err)
		}
		if len(recs) != 2 {
			t.Fatalf("expected 2 recordings for session list-sess-1, got %d", len(recs))
		}
	})

	t.Run("list by session no results", func(t *testing.T) {
		recs, err := db.ListRecordingsBySession("nonexistent")
		if err != nil {
			t.Fatalf("ListRecordingsBySession() error = %v", err)
		}
		if len(recs) != 0 {
			t.Errorf("expected 0 recordings, got %d", len(recs))
		}
	})

	t.Run("list all recordings", func(t *testing.T) {
		recs, err := db.ListAllRecordings()
		if err != nil {
			t.Fatalf("ListAllRecordings() error = %v", err)
		}
		if len(recs) != 3 {
			t.Fatalf("expected 3 recordings, got %d", len(recs))
		}
		// Most recent first
		if recs[0].ID != "lr-3" {
			t.Errorf("first recording ID = %s, want lr-3", recs[0].ID)
		}
	})
}

func TestRecordingDelete(t *testing.T) {
	db := setupTestDB(t)

	app := Application{
		ID: "del-app", Name: "Del App", Description: "d",
		URL: "http://x", Icon: "i", Category: "c",
	}
	if err := db.CreateApp(app); err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}

	now := time.Now().Truncate(time.Second)
	session := Session{
		ID: "del-sess", UserID: "user-1", AppID: "del-app",
		PodName: "pod-1", Status: SessionStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.CreateSession(session); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	rec := Recording{
		ID: "del-rec", SessionID: "del-sess", UserID: "user-1",
		Filename: "del.webm", Format: "webm", StorageBackend: "local",
		Status: RecordingStatusReady, CreatedAt: now,
	}
	if err := db.CreateRecording(rec); err != nil {
		t.Fatalf("CreateRecording() error = %v", err)
	}

	t.Run("delete existing recording", func(t *testing.T) {
		if err := db.DeleteRecording("del-rec"); err != nil {
			t.Fatalf("DeleteRecording() error = %v", err)
		}

		got, err := db.GetRecording("del-rec")
		if err != nil {
			t.Fatalf("GetRecording() error = %v", err)
		}
		if got != nil {
			t.Error("expected nil after deletion")
		}
	})

	t.Run("delete nonexistent recording", func(t *testing.T) {
		err := db.DeleteRecording("nonexistent")
		if err != sql.ErrNoRows {
			t.Errorf("expected sql.ErrNoRows, got %v", err)
		}
	})
}

func TestRecordingStatusConstants(t *testing.T) {
	statuses := []struct {
		status RecordingStatus
		want   string
	}{
		{RecordingStatusRecording, "recording"},
		{RecordingStatusUploading, "uploading"},
		{RecordingStatusReady, "ready"},
		{RecordingStatusFailed, "failed"},
	}

	for _, tt := range statuses {
		if string(tt.status) != tt.want {
			t.Errorf("RecordingStatus %q != %q", tt.status, tt.want)
		}
	}
}

func TestListExpiredRecordings(t *testing.T) {
	db := setupTestDB(t)

	app := Application{
		ID: "exp-app", Name: "E App", Description: "d",
		URL: "http://x", Icon: "i", Category: "c",
		LaunchType: LaunchTypeContainer,
	}
	if err := db.CreateApp(app); err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}

	now := time.Now().Truncate(time.Second)
	session := Session{
		ID: "exp-sess", UserID: "user-1", AppID: "exp-app",
		PodName: "pod-1", Status: SessionStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.CreateSession(session); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	// Create two ready recordings with different completed_at times
	for _, id := range []string{"exp-old", "exp-new"} {
		rec := Recording{
			ID: id, SessionID: "exp-sess", UserID: "user-1",
			Filename: id + ".webm", Format: "webm", StorageBackend: "local",
			Status: RecordingStatusRecording, CreatedAt: now,
		}
		if err := db.CreateRecording(rec); err != nil {
			t.Fatalf("CreateRecording(%s): %v", id, err)
		}
		if err := db.UpdateRecordingComplete(id, id+".webm", 1024, 60.0); err != nil {
			t.Fatalf("UpdateRecordingComplete(%s): %v", id, err)
		}
	}

	// Backdate exp-old to 31 days ago
	past := now.Add(-31 * 24 * time.Hour)
	if _, err := db.bun.DB.Exec("UPDATE recordings SET completed_at = ? WHERE id = ?", past, "exp-old"); err != nil {
		t.Fatalf("backdate completed_at: %v", err)
	}

	// Query with 30 day cutoff
	cutoff := now.Add(-30 * 24 * time.Hour)
	expired, err := db.ListExpiredRecordings(cutoff)
	if err != nil {
		t.Fatalf("ListExpiredRecordings() error = %v", err)
	}

	if len(expired) != 1 {
		t.Fatalf("expected 1 expired recording, got %d", len(expired))
	}
	if expired[0].ID != "exp-old" {
		t.Errorf("expected exp-old, got %s", expired[0].ID)
	}

	// Non-ready recordings should not appear
	rec := Recording{
		ID: "exp-failed", SessionID: "exp-sess", UserID: "user-1",
		Filename: "f.webm", Format: "webm", StorageBackend: "local",
		Status: RecordingStatusFailed, CreatedAt: now,
	}
	if err := db.CreateRecording(rec); err != nil {
		t.Fatalf("CreateRecording: %v", err)
	}
	expired, err = db.ListExpiredRecordings(cutoff)
	if err != nil {
		t.Fatalf("ListExpiredRecordings() error = %v", err)
	}
	if len(expired) != 1 {
		t.Errorf("expected 1 expired (failed should be excluded), got %d", len(expired))
	}
}

func TestListExpiredRecordings_EmptyResult(t *testing.T) {
	db := setupTestDB(t)

	app := Application{
		ID: "empty-app", Name: "E App", Description: "d",
		URL: "http://x", Icon: "i", Category: "c",
		LaunchType: LaunchTypeContainer,
	}
	if err := db.CreateApp(app); err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}

	now := time.Now().Truncate(time.Second)
	session := Session{
		ID: "empty-sess", UserID: "user-1", AppID: "empty-app",
		PodName: "pod-1", Status: SessionStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.CreateSession(session); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	// Create a fresh ready recording (not expired)
	rec := Recording{
		ID: "fresh-rec", SessionID: "empty-sess", UserID: "user-1",
		Filename: "f.webm", Format: "webm", StorageBackend: "local",
		Status: RecordingStatusRecording, CreatedAt: now,
	}
	if err := db.CreateRecording(rec); err != nil {
		t.Fatalf("CreateRecording: %v", err)
	}
	if err := db.UpdateRecordingComplete("fresh-rec", "fresh-rec.webm", 1024, 60.0); err != nil {
		t.Fatalf("UpdateRecordingComplete: %v", err)
	}

	// Query with cutoff far in the past - nothing should match
	cutoff := now.Add(-30 * 24 * time.Hour)
	expired, err := db.ListExpiredRecordings(cutoff)
	if err != nil {
		t.Fatalf("ListExpiredRecordings() error = %v", err)
	}
	if len(expired) != 0 {
		t.Errorf("expected 0 expired recordings, got %d", len(expired))
	}
}

func TestListExpiredRecordings_NullCompletedAtExcluded(t *testing.T) {
	db := setupTestDB(t)

	app := Application{
		ID: "null-app", Name: "N App", Description: "d",
		URL: "http://x", Icon: "i", Category: "c",
		LaunchType: LaunchTypeContainer,
	}
	if err := db.CreateApp(app); err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}

	now := time.Now().Truncate(time.Second)
	session := Session{
		ID: "null-sess", UserID: "user-1", AppID: "null-app",
		PodName: "pod-1", Status: SessionStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.CreateSession(session); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	// Create a recording that is still "recording" (no completed_at, NULL)
	rec := Recording{
		ID: "incomplete-rec", SessionID: "null-sess", UserID: "user-1",
		Filename: "i.webm", Format: "webm", StorageBackend: "local",
		Status: RecordingStatusRecording, CreatedAt: now,
	}
	if err := db.CreateRecording(rec); err != nil {
		t.Fatalf("CreateRecording: %v", err)
	}

	// Even with a very generous cutoff, NULL completed_at should be excluded
	cutoff := now.Add(24 * time.Hour) // future cutoff
	expired, err := db.ListExpiredRecordings(cutoff)
	if err != nil {
		t.Fatalf("ListExpiredRecordings() error = %v", err)
	}
	if len(expired) != 0 {
		t.Errorf("expected 0 expired (incomplete should be excluded), got %d", len(expired))
	}
}

func TestRecordingTenantDefault(t *testing.T) {
	db := setupTestDB(t)

	app := Application{
		ID: "tenant-app", Name: "T App", Description: "d",
		URL: "http://x", Icon: "i", Category: "c",
	}
	if err := db.CreateApp(app); err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}

	now := time.Now().Truncate(time.Second)
	session := Session{
		ID: "tenant-sess", UserID: "user-1", AppID: "tenant-app",
		PodName: "pod-1", Status: SessionStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.CreateSession(session); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	// Create recording without setting TenantID - should default to "default"
	rec := Recording{
		ID: "tenant-rec", SessionID: "tenant-sess", UserID: "user-1",
		Filename: "t.webm", Format: "webm", StorageBackend: "local",
		Status: RecordingStatusRecording, CreatedAt: now,
	}
	if err := db.CreateRecording(rec); err != nil {
		t.Fatalf("CreateRecording() error = %v", err)
	}

	got, err := db.GetRecording("tenant-rec")
	if err != nil {
		t.Fatalf("GetRecording() error = %v", err)
	}
	if got.TenantID != DefaultTenantID {
		t.Errorf("TenantID = %s, want %s", got.TenantID, DefaultTenantID)
	}
}
