package recordings

import (
	"log/slog"
	"time"

	"github.com/rjsadow/sortie/internal/db"
)

// Cleaner periodically removes expired recordings from storage and the database.
type Cleaner struct {
	db            *db.DB
	store         RecordingStore
	retentionDays int
	interval      time.Duration
	stopCh        chan struct{}
}

// NewCleaner creates a Cleaner that deletes recordings older than retentionDays.
// If retentionDays is 0 the cleaner does nothing when started.
func NewCleaner(database *db.DB, store RecordingStore, retentionDays int) *Cleaner {
	return &Cleaner{
		db:            database,
		store:         store,
		retentionDays: retentionDays,
		interval:      1 * time.Hour,
		stopCh:        make(chan struct{}),
	}
}

// Start launches the cleanup goroutine. It returns immediately.
func (c *Cleaner) Start() {
	if c.retentionDays <= 0 {
		return
	}
	go c.loop()
}

// Stop signals the cleanup goroutine to exit.
func (c *Cleaner) Stop() {
	close(c.stopCh)
}

func (c *Cleaner) loop() {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.run()
		case <-c.stopCh:
			return
		}
	}
}

func (c *Cleaner) run() {
	if c.retentionDays <= 0 {
		return
	}
	cutoff := time.Now().Add(-time.Duration(c.retentionDays) * 24 * time.Hour)
	expired, err := c.db.ListExpiredRecordings(cutoff)
	if err != nil {
		slog.Warn("Recording cleanup: failed to list expired recordings", "error", err)
		return
	}

	for _, rec := range expired {
		if rec.StoragePath != "" {
			if err := c.store.Delete(rec.StoragePath); err != nil {
				slog.Warn("Recording cleanup: failed to delete file",
					"recording_id", rec.ID,
					"storage_path", rec.StoragePath,
					"error", err)
			}
		}
		if rec.VideoPath != "" {
			if err := c.store.Delete(rec.VideoPath); err != nil {
				slog.Warn("Recording cleanup: failed to delete video file",
					"recording_id", rec.ID,
					"video_path", rec.VideoPath,
					"error", err)
			}
		}

		if err := c.db.DeleteRecording(rec.ID); err != nil {
			slog.Warn("Recording cleanup: failed to delete DB record",
				"recording_id", rec.ID,
				"error", err)
			continue
		}

		slog.Info("Recording cleanup: deleted expired recording",
			"recording_id", rec.ID,
			"completed_at", rec.CompletedAt)
	}
}
