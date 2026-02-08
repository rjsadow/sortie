package sessions

import "context"

// NoopRecorder is a no-op implementation of SessionRecorder.
// It discards all events and is used as the default when recording is disabled.
type NoopRecorder struct{}

// OnEvent implements SessionRecorder. It does nothing.
func (n *NoopRecorder) OnEvent(_ context.Context, _ SessionEventData) {}
