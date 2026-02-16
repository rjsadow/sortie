package recordings

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// mockS3Client implements S3API for testing.
type mockS3Client struct {
	objects   map[string][]byte
	putErr    error
	getErr    error
	deleteErr error
}

func newMockS3Client() *mockS3Client {
	return &mockS3Client{objects: make(map[string][]byte)}
}

func (m *mockS3Client) PutObject(_ context.Context, input *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if m.putErr != nil {
		return nil, m.putErr
	}
	data, err := io.ReadAll(input.Body)
	if err != nil {
		return nil, err
	}
	key := *input.Key
	m.objects[key] = data
	return &s3.PutObjectOutput{}, nil
}

func (m *mockS3Client) GetObject(_ context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	key := *input.Key
	data, ok := m.objects[key]
	if !ok {
		return nil, fmt.Errorf("NoSuchKey: %s", key)
	}
	return &s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader(data)),
	}, nil
}

func (m *mockS3Client) DeleteObject(_ context.Context, input *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	if m.deleteErr != nil {
		return nil, m.deleteErr
	}
	delete(m.objects, *input.Key)
	return &s3.DeleteObjectOutput{}, nil
}

func TestS3Store_SaveGetDelete(t *testing.T) {
	mock := newMockS3Client()
	store := NewS3StoreWithClient(mock, "test-bucket", "recordings/")

	content := "test recording data"
	storagePath, err := store.Save("rec-123", strings.NewReader(content))
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify key format: prefix + year/month/id.vncrec
	now := time.Now()
	expectedPrefix := fmt.Sprintf("recordings/%d/%02d/rec-123.vncrec", now.Year(), now.Month())
	if storagePath != expectedPrefix {
		t.Errorf("unexpected storage path: got %q, want %q", storagePath, expectedPrefix)
	}

	// Get
	reader, err := store.Get(storagePath)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer reader.Close()

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if string(got) != content {
		t.Errorf("content mismatch: got %q, want %q", string(got), content)
	}

	// Delete
	if err := store.Delete(storagePath); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	if _, err := store.Get(storagePath); err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestS3Store_KeyConstruction(t *testing.T) {
	mock := newMockS3Client()
	now := time.Now()

	tests := []struct {
		name     string
		prefix   string
		id       string
		wantKey  string
	}{
		{
			name:    "standard prefix",
			prefix:  "recordings/",
			id:      "rec-abc",
			wantKey: fmt.Sprintf("recordings/%d/%02d/rec-abc.vncrec", now.Year(), now.Month()),
		},
		{
			name:    "empty prefix",
			prefix:  "",
			id:      "rec-xyz",
			wantKey: fmt.Sprintf("%d/%02d/rec-xyz.vncrec", now.Year(), now.Month()),
		},
		{
			name:    "custom prefix",
			prefix:  "tenant1/vids/",
			id:      "rec-001",
			wantKey: fmt.Sprintf("tenant1/vids/%d/%02d/rec-001.vncrec", now.Year(), now.Month()),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewS3StoreWithClient(mock, "bucket", tt.prefix)
			key, err := store.Save(tt.id, strings.NewReader("data"))
			if err != nil {
				t.Fatalf("Save failed: %v", err)
			}
			if key != tt.wantKey {
				t.Errorf("key = %q, want %q", key, tt.wantKey)
			}
		})
	}
}

func TestS3Store_SaveError(t *testing.T) {
	mock := newMockS3Client()
	mock.putErr = fmt.Errorf("access denied")
	store := NewS3StoreWithClient(mock, "bucket", "prefix/")

	_, err := store.Save("rec-fail", strings.NewReader("data"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestS3Store_GetError(t *testing.T) {
	mock := newMockS3Client()
	mock.getErr = fmt.Errorf("no such key")
	store := NewS3StoreWithClient(mock, "bucket", "prefix/")

	_, err := store.Get("nonexistent.vncrec")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no such key") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestS3Store_DeleteError(t *testing.T) {
	mock := newMockS3Client()
	mock.deleteErr = fmt.Errorf("permission denied")
	store := NewS3StoreWithClient(mock, "bucket", "prefix/")

	err := store.Delete("some-key.vncrec")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("unexpected error: %v", err)
	}
}
