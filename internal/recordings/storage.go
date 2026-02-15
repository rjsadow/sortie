package recordings

import "io"

// RecordingStore abstracts video recording file storage.
type RecordingStore interface {
	// Save writes a recording file from the reader and returns the storage path.
	Save(id string, r io.Reader) (storagePath string, err error)

	// Get returns a ReadCloser for the recording file at the given storage path.
	Get(storagePath string) (io.ReadCloser, error)

	// Delete removes the recording file at the given storage path.
	Delete(storagePath string) error
}
