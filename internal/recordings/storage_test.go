package recordings

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalStore_SaveGetDelete(t *testing.T) {
	baseDir := t.TempDir()
	store := NewLocalStore(baseDir)

	content := []byte("fake webm video data for testing")

	t.Run("save recording", func(t *testing.T) {
		path, err := store.Save("test-rec-1", bytes.NewReader(content))
		if err != nil {
			t.Fatalf("Save() error = %v", err)
		}
		if path == "" {
			t.Fatal("Save() returned empty path")
		}
		if !strings.HasSuffix(path, "test-rec-1.vncrec") {
			t.Errorf("path = %s, want suffix test-rec-1.vncrec", path)
		}

		// Verify file exists on disk
		fullPath := filepath.Join(baseDir, path)
		info, err := os.Stat(fullPath)
		if err != nil {
			t.Fatalf("file not found at %s: %v", fullPath, err)
		}
		if info.Size() != int64(len(content)) {
			t.Errorf("file size = %d, want %d", info.Size(), len(content))
		}
	})

	t.Run("get recording", func(t *testing.T) {
		// First save one
		path, err := store.Save("test-rec-2", bytes.NewReader(content))
		if err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		reader, err := store.Get(path)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		defer reader.Close()

		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		if !bytes.Equal(data, content) {
			t.Errorf("content mismatch: got %d bytes, want %d bytes", len(data), len(content))
		}
	})

	t.Run("get nonexistent recording", func(t *testing.T) {
		_, err := store.Get("nonexistent/path.vncrec")
		if err == nil {
			t.Fatal("expected error for nonexistent path")
		}
	})

	t.Run("delete recording", func(t *testing.T) {
		path, err := store.Save("test-rec-3", bytes.NewReader(content))
		if err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		if err := store.Delete(path); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		// Verify file no longer exists
		fullPath := filepath.Join(baseDir, path)
		if _, err := os.Stat(fullPath); !os.IsNotExist(err) {
			t.Errorf("file should not exist after delete: %v", err)
		}
	})

	t.Run("delete nonexistent is not error", func(t *testing.T) {
		if err := store.Delete("nonexistent/path.vncrec"); err != nil {
			t.Errorf("Delete() should not error for nonexistent file, got %v", err)
		}
	})
}

func TestLocalStore_SaveCreatesDirectories(t *testing.T) {
	baseDir := t.TempDir()
	store := NewLocalStore(baseDir)

	content := []byte("test data")
	path, err := store.Save("dir-test", bytes.NewReader(content))
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Path should have year/month structure
	parts := strings.Split(path, string(filepath.Separator))
	if len(parts) < 3 {
		t.Fatalf("path should have at least 3 segments (year/month/file), got %s", path)
	}
}

func TestLocalStore_SaveLargeFile(t *testing.T) {
	baseDir := t.TempDir()
	store := NewLocalStore(baseDir)

	// 1MB of data
	content := make([]byte, 1024*1024)
	for i := range content {
		content[i] = byte(i % 256)
	}

	path, err := store.Save("large-rec", bytes.NewReader(content))
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	reader, err := store.Get(path)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(data) != len(content) {
		t.Errorf("read %d bytes, want %d", len(data), len(content))
	}
}

func TestLocalStore_PathTraversal(t *testing.T) {
	baseDir := t.TempDir()
	store := NewLocalStore(baseDir)
	content := []byte("test data")

	t.Run("save sanitizes path traversal in id", func(t *testing.T) {
		// filepath.Base strips traversal, so "../../etc/passwd" becomes "passwd"
		path, err := store.Save("../../etc/passwd", bytes.NewReader(content))
		if err != nil {
			t.Fatalf("Save() error = %v", err)
		}
		if !strings.HasSuffix(path, "passwd.vncrec") {
			t.Errorf("path = %s, want suffix passwd.vncrec", path)
		}
		// Verify file is within baseDir
		fullPath, _ := filepath.Abs(filepath.Join(baseDir, path))
		absBase, _ := filepath.Abs(baseDir)
		if !strings.HasPrefix(fullPath, absBase) {
			t.Errorf("file %s escaped baseDir %s", fullPath, absBase)
		}
	})

	t.Run("get rejects path traversal", func(t *testing.T) {
		_, err := store.Get("../../etc/passwd")
		if err == nil {
			t.Fatal("Get() should reject path traversal")
		}
	})

	t.Run("delete rejects path traversal", func(t *testing.T) {
		err := store.Delete("../../../etc/important")
		if err == nil {
			t.Fatal("Delete() should reject path traversal")
		}
	})
}

func TestNewLocalStore(t *testing.T) {
	store := NewLocalStore("/tmp/test-recordings")
	if store == nil {
		t.Fatal("NewLocalStore() returned nil")
		return
	}
	if store.baseDir != "/tmp/test-recordings" {
		t.Errorf("baseDir = %s, want /tmp/test-recordings", store.baseDir)
	}
}
