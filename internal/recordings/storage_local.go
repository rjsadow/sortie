package recordings

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// LocalStore implements RecordingStore using the local filesystem.
// Files are stored at {baseDir}/{year}/{month}/{id}.webm.
type LocalStore struct {
	baseDir string
}

// NewLocalStore creates a LocalStore that writes to the given base directory.
func NewLocalStore(baseDir string) *LocalStore {
	return &LocalStore{baseDir: baseDir}
}

// Save writes a recording file to disk and returns the relative storage path.
func (s *LocalStore) Save(id string, r io.Reader) (string, error) {
	now := time.Now()
	dir := filepath.Join(s.baseDir, fmt.Sprintf("%d", now.Year()), fmt.Sprintf("%02d", now.Month()))

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	relPath := filepath.Join(fmt.Sprintf("%d", now.Year()), fmt.Sprintf("%02d", now.Month()), id+".webm")
	fullPath := filepath.Join(s.baseDir, relPath)

	f, err := os.Create(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file %s: %w", fullPath, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		os.Remove(fullPath)
		return "", fmt.Errorf("failed to write recording: %w", err)
	}

	return relPath, nil
}

// Get opens the recording file at the given storage path for reading.
func (s *LocalStore) Get(storagePath string) (io.ReadCloser, error) {
	fullPath := filepath.Join(s.baseDir, storagePath)
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open recording: %w", err)
	}
	return f, nil
}

// Delete removes the recording file at the given storage path.
func (s *LocalStore) Delete(storagePath string) error {
	fullPath := filepath.Join(s.baseDir, storagePath)
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete recording: %w", err)
	}
	return nil
}
