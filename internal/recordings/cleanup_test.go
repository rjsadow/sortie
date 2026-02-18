package recordings

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/rjsadow/sortie/internal/db"
	"github.com/rjsadow/sortie/internal/db/dbtest"
)

// memoryStore is a simple in-memory RecordingStore for testing cleanup.
type memoryStore struct {
	files     map[string]bool
	deleteErr error
}

func newMemoryStore() *memoryStore {
	return &memoryStore{files: make(map[string]bool)}
}

func (m *memoryStore) Save(id string, _ io.Reader) (string, error) {
	key := id + ".webm"
	m.files[key] = true
	return key, nil
}

func (m *memoryStore) Get(_ string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}

func (m *memoryStore) Delete(storagePath string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.files, storagePath)
	return nil
}

func openTestDB(t *testing.T) *db.DB {
	t.Helper()

	database := dbtest.NewTestDB(t)

	// Create prerequisite app and session
	app := db.Application{
		ID: "cleanup-app", Name: "App", Description: "d",
		URL: "http://x", Icon: "i", Category: "c",
		LaunchType: db.LaunchTypeContainer,
	}
	if err := database.CreateApp(app); err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	now := time.Now().Truncate(time.Second)
	session := db.Session{
		ID: "cleanup-sess", UserID: "user-1", AppID: "cleanup-app",
		PodName: "pod-1", Status: db.SessionStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := database.CreateSession(session); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	return database
}

func createRecording(t *testing.T, database *db.DB, id string, completedDaysAgo int) {
	t.Helper()
	rec := db.Recording{
		ID:             id,
		SessionID:      "cleanup-sess",
		UserID:         "user-1",
		Filename:       id + ".webm",
		Format:         "webm",
		StorageBackend: "local",
		Status:         db.RecordingStatusRecording,
		CreatedAt:      time.Now(),
	}
	if err := database.CreateRecording(rec); err != nil {
		t.Fatalf("CreateRecording(%s): %v", id, err)
	}
	// Mark as ready with a completed_at in the past
	if err := database.UpdateRecordingComplete(id, id+".webm", 1024, 60.0); err != nil {
		t.Fatalf("UpdateRecordingComplete(%s): %v", id, err)
	}
	// Backdate completed_at using ExecRaw
	if completedDaysAgo > 0 {
		past := time.Now().Add(-time.Duration(completedDaysAgo) * 24 * time.Hour)
		if _, err := database.ExecRaw("UPDATE recordings SET completed_at = ? WHERE id = ?", past, id); err != nil {
			t.Fatalf("backdate completed_at: %v", err)
		}
	}
}

func TestCleaner_DeletesExpiredRecordings(t *testing.T) {
	tdb := openTestDB(t)
	store := newMemoryStore()

	// Create an expired recording (31 days old) and a fresh one
	createRecording(t, tdb, "rec-old", 31)
	store.files["rec-old.webm"] = true

	createRecording(t, tdb, "rec-new", 1)
	store.files["rec-new.webm"] = true

	cleaner := NewCleaner(tdb, store, 30)
	cleaner.run()

	// Old recording should be deleted from both store and DB
	if store.files["rec-old.webm"] {
		t.Error("expected expired recording to be deleted from store")
	}
	rec, err := tdb.GetRecording("rec-old")
	if err != nil {
		t.Fatalf("GetRecording error: %v", err)
	}
	if rec != nil {
		t.Error("expected expired recording to be deleted from DB")
	}

	// New recording should still exist
	if !store.files["rec-new.webm"] {
		t.Error("expected non-expired recording to remain in store")
	}
	rec, err = tdb.GetRecording("rec-new")
	if err != nil {
		t.Fatalf("GetRecording error: %v", err)
	}
	if rec == nil {
		t.Error("expected non-expired recording to remain in DB")
	}
}

func TestCleaner_ZeroRetentionSkipsCleanup(t *testing.T) {
	tdb := openTestDB(t)
	store := newMemoryStore()

	createRecording(t, tdb, "rec-forever", 365)
	store.files["rec-forever.webm"] = true

	cleaner := NewCleaner(tdb, store, 0)
	// Start should be a no-op with retentionDays=0
	cleaner.Start()
	// run explicitly to verify it's harmless
	cleaner.run()
	cleaner.Stop()

	// Nothing should be deleted
	if !store.files["rec-forever.webm"] {
		t.Error("expected recording to remain with retention=0")
	}
	rec, _ := tdb.GetRecording("rec-forever")
	if rec == nil {
		t.Error("expected recording to remain in DB with retention=0")
	}
}

func TestCleaner_StoreDeleteFailureStillDeletesDB(t *testing.T) {
	tdb := openTestDB(t)
	store := newMemoryStore()
	store.deleteErr = io.ErrUnexpectedEOF // simulate store failure

	createRecording(t, tdb, "rec-fail", 31)
	store.files["rec-fail.webm"] = true

	cleaner := NewCleaner(tdb, store, 30)
	cleaner.run()

	// Store file should still be there (delete failed)
	if !store.files["rec-fail.webm"] {
		t.Error("expected file to remain after store delete failure")
	}

	// DB record should still be deleted
	rec, _ := tdb.GetRecording("rec-fail")
	if rec != nil {
		t.Error("expected DB record to be deleted even after store failure")
	}
}

func TestCleaner_NoExpiredRecordings(t *testing.T) {
	tdb := openTestDB(t)
	store := newMemoryStore()

	// Create only fresh recordings
	createRecording(t, tdb, "rec-fresh", 1)
	store.files["rec-fresh.webm"] = true

	cleaner := NewCleaner(tdb, store, 30)
	cleaner.run()

	// Nothing should be deleted
	if !store.files["rec-fresh.webm"] {
		t.Error("expected fresh recording to remain in store")
	}
	rec, _ := tdb.GetRecording("rec-fresh")
	if rec == nil {
		t.Error("expected fresh recording to remain in DB")
	}
}

func TestCleaner_StopTerminatesGoroutine(t *testing.T) {
	tdb := openTestDB(t)
	store := newMemoryStore()

	cleaner := NewCleaner(tdb, store, 30)
	cleaner.Start()

	// Stop should not block or panic
	cleaner.Stop()

	// Verify the stop channel is closed (second close would panic if not already closed)
	select {
	case <-cleaner.stopCh:
		// expected: channel is closed
	default:
		t.Error("expected stopCh to be closed after Stop()")
	}
}

func TestCleaner_EmptyStoragePath(t *testing.T) {
	tdb := openTestDB(t)
	store := newMemoryStore()

	// Create an expired recording with no storage path
	rec := db.Recording{
		ID:             "rec-nopath",
		SessionID:      "cleanup-sess",
		UserID:         "user-1",
		Filename:       "rec-nopath.webm",
		Format:         "webm",
		StorageBackend: "local",
		Status:         db.RecordingStatusRecording,
		CreatedAt:      time.Now(),
	}
	if err := tdb.CreateRecording(rec); err != nil {
		t.Fatalf("CreateRecording: %v", err)
	}
	if err := tdb.UpdateRecordingComplete("rec-nopath", "", 0, 0); err != nil {
		t.Fatalf("UpdateRecordingComplete: %v", err)
	}
	// Backdate to 31 days ago
	past := time.Now().Add(-31 * 24 * time.Hour)
	if _, err := tdb.ExecRaw("UPDATE recordings SET completed_at = ? WHERE id = ?", past, "rec-nopath"); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	cleaner := NewCleaner(tdb, store, 30)
	cleaner.run()

	// DB record should still be deleted even with empty storage path
	got, _ := tdb.GetRecording("rec-nopath")
	if got != nil {
		t.Error("expected recording with empty storage path to be deleted from DB")
	}
}
