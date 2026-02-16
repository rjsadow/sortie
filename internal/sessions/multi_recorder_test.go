package sessions

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMultiRecorder_FanOut(t *testing.T) {
	r1 := &mockRecorder{}
	r2 := &mockRecorder{}

	multi := NewMultiRecorder(r1, r2)

	event := SessionEventData{
		SessionID: "s1",
		UserID:    "u1",
		AppID:     "a1",
		Event:     EventSessionCreated,
		Timestamp: time.Now(),
	}
	multi.OnEvent(context.Background(), event)

	if got := len(r1.getEvents()); got != 1 {
		t.Errorf("r1: expected 1 event, got %d", got)
	}
	if got := len(r2.getEvents()); got != 1 {
		t.Errorf("r2: expected 1 event, got %d", got)
	}
}

func TestMultiRecorder_NilsFiltered(t *testing.T) {
	r1 := &mockRecorder{}

	// nil entries should be silently skipped
	multi := NewMultiRecorder(nil, r1, nil)

	multi.OnEvent(context.Background(), SessionEventData{
		SessionID: "s1",
		Event:     EventSessionReady,
	})

	if got := len(r1.getEvents()); got != 1 {
		t.Errorf("expected 1 event, got %d", got)
	}
}

func TestMultiRecorder_EmptyList(t *testing.T) {
	multi := NewMultiRecorder()

	// Should not panic
	multi.OnEvent(context.Background(), SessionEventData{
		SessionID: "s1",
		Event:     EventSessionStopped,
	})
}

func TestMultiRecorder_ConcurrentSafe(t *testing.T) {
	r1 := &mockRecorder{}
	r2 := &mockRecorder{}
	multi := NewMultiRecorder(r1, r2)

	const n = 100
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			multi.OnEvent(context.Background(), SessionEventData{
				SessionID: "concurrent",
				Event:     EventSessionCreated,
			})
		}()
	}
	wg.Wait()

	if got := len(r1.getEvents()); got != n {
		t.Errorf("r1: expected %d events, got %d", n, got)
	}
	if got := len(r2.getEvents()); got != n {
		t.Errorf("r2: expected %d events, got %d", n, got)
	}
}

func TestMultiRecorder_ImplementsInterface(t *testing.T) {
	var _ SessionRecorder = (*MultiRecorder)(nil)
}
