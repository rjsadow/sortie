package sessions

import (
	"context"
	"time"
)

// SessionEvent represents a lifecycle event type for session recording.
type SessionEvent string

const (
	// EventSessionCreated is emitted when a session is created and pod provisioning begins.
	EventSessionCreated SessionEvent = "session.created"

	// EventSessionReady is emitted when the session's pod is ready and the session is running.
	EventSessionReady SessionEvent = "session.ready"

	// EventSessionFailed is emitted when a session fails (pod failed to start, IP lookup failed, etc.).
	EventSessionFailed SessionEvent = "session.failed"

	// EventSessionStopped is emitted when a user stops a session (pod deleted, session record kept).
	EventSessionStopped SessionEvent = "session.stopped"

	// EventSessionRestarted is emitted when a stopped session is restarted with a new pod.
	EventSessionRestarted SessionEvent = "session.restarted"

	// EventSessionExpired is emitted when a session is expired by the cleanup goroutine.
	EventSessionExpired SessionEvent = "session.expired"

	// EventSessionTerminated is emitted when a session is terminated by user action (DELETE).
	EventSessionTerminated SessionEvent = "session.terminated"
)

// SessionEventData holds data associated with a session lifecycle event.
type SessionEventData struct {
	SessionID string            `json:"session_id"`
	UserID    string            `json:"user_id"`
	AppID     string            `json:"app_id"`
	Event     SessionEvent      `json:"event"`
	Timestamp time.Time         `json:"timestamp"`
	Reason    string            `json:"reason,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// SessionRecorder is the interface for recording session lifecycle events.
// Implementations should be non-blocking; long-running operations should
// be handled asynchronously to avoid slowing down session lifecycle methods.
type SessionRecorder interface {
	// OnEvent is called when a session lifecycle event occurs.
	OnEvent(ctx context.Context, event SessionEventData)
}

// RecordingConfig holds configuration for session recording.
type RecordingConfig struct {
	// Enabled controls whether session recording is active.
	Enabled bool

	// Endpoint is an optional URL for sending recording events (for future use).
	Endpoint string

	// BufferSize is the channel buffer size for async event processing (for future use).
	BufferSize int
}
