package recordings

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LocalStore implements RecordingStore using the local filesystem.
// Files are stored at {baseDir}/{year}/{month}/{id}.vncrec.
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
	cleanID := filepath.Base(id) // strip any directory components
	relPath := filepath.Join(fmt.Sprintf("%d", now.Year()), fmt.Sprintf("%02d", now.Month()), cleanID+".vncrec")

	// Validate path stays within baseDir
	fullPath := filepath.Clean(filepath.Join(s.baseDir, relPath))
	absBase, err := filepath.Abs(s.baseDir)
	if err != nil {
		return "", fmt.Errorf("invalid base dir: %w", err)
	}
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	if !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid recording id: path traversal detected")
	}

	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	f, err := os.Create(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file %s: %w", absPath, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		os.Remove(absPath)
		return "", fmt.Errorf("failed to write recording: %w", err)
	}

	return relPath, nil
}

// Get opens the recording file at the given storage path for reading.
func (s *LocalStore) Get(storagePath string) (io.ReadCloser, error) {
	// Validate path stays within baseDir
	fullPath := filepath.Clean(filepath.Join(s.baseDir, storagePath))
	absBase, err := filepath.Abs(s.baseDir)
	if err != nil {
		return nil, fmt.Errorf("invalid base dir: %w", err)
	}
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}
	if !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) && absPath != absBase {
		return nil, fmt.Errorf("path traversal detected: %s", storagePath)
	}

	f, err := os.Open(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open recording: %w", err)
	}
	return f, nil
}

// Delete removes the recording file at the given storage path.
func (s *LocalStore) Delete(storagePath string) error {
	// Validate path stays within baseDir
	fullPath := filepath.Clean(filepath.Join(s.baseDir, storagePath))
	absBase, err := filepath.Abs(s.baseDir)
	if err != nil {
		return fmt.Errorf("invalid base dir: %w", err)
	}
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	if !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) && absPath != absBase {
		return fmt.Errorf("path traversal detected: %s", storagePath)
	}

	if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete recording: %w", err)
	}
	return nil
}
