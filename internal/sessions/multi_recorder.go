package sessions

import "context"

// MultiRecorder is a composite SessionRecorder that delegates OnEvent to
// multiple child recorders. This allows chaining the SSE hub with the
// billing collector (or any other recorder) without replacing either one.
type MultiRecorder struct {
	recorders []SessionRecorder
}

// NewMultiRecorder creates a MultiRecorder from the given recorders.
// Nil entries are silently skipped.
func NewMultiRecorder(recorders ...SessionRecorder) *MultiRecorder {
	filtered := make([]SessionRecorder, 0, len(recorders))
	for _, r := range recorders {
		if r != nil {
			filtered = append(filtered, r)
		}
	}
	return &MultiRecorder{recorders: filtered}
}

// OnEvent fans out the event to every child recorder.
func (m *MultiRecorder) OnEvent(ctx context.Context, event SessionEventData) {
	for _, r := range m.recorders {
		r.OnEvent(ctx, event)
	}
}
