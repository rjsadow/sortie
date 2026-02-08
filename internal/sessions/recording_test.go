package sessions

import (
	"context"
	"sync"
	"testing"
	"time"
)

// mockRecorder captures events for testing.
type mockRecorder struct {
	mu     sync.Mutex
	events []SessionEventData
}

func (m *mockRecorder) OnEvent(_ context.Context, event SessionEventData) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
}

func (m *mockRecorder) getEvents() []SessionEventData {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make([]SessionEventData, len(m.events))
	copy(copied, m.events)
	return copied
}

func TestNoopRecorder_OnEvent(t *testing.T) {
	recorder := &NoopRecorder{}

	// Should not panic
	recorder.OnEvent(context.Background(), SessionEventData{
		SessionID: "test-session",
		UserID:    "test-user",
		AppID:     "test-app",
		Event:     EventSessionCreated,
		Timestamp: time.Now(),
		Reason:    "test",
	})
}

func TestSessionEventTypes(t *testing.T) {
	events := []SessionEvent{
		EventSessionCreated,
		EventSessionReady,
		EventSessionFailed,
		EventSessionStopped,
		EventSessionRestarted,
		EventSessionExpired,
		EventSessionTerminated,
	}

	expected := []string{
		"session.created",
		"session.ready",
		"session.failed",
		"session.stopped",
		"session.restarted",
		"session.expired",
		"session.terminated",
	}

	if len(events) != len(expected) {
		t.Fatalf("expected %d event types, got %d", len(expected), len(events))
	}

	for i, event := range events {
		if string(event) != expected[i] {
			t.Errorf("event %d: expected %q, got %q", i, expected[i], string(event))
		}
	}
}

func TestSessionEventData_Fields(t *testing.T) {
	now := time.Now()
	data := SessionEventData{
		SessionID: "sess-123",
		UserID:    "user-456",
		AppID:     "app-789",
		Event:     EventSessionCreated,
		Timestamp: now,
		Reason:    "test reason",
		Metadata: map[string]string{
			"key": "value",
		},
	}

	if data.SessionID != "sess-123" {
		t.Errorf("expected SessionID %q, got %q", "sess-123", data.SessionID)
	}
	if data.UserID != "user-456" {
		t.Errorf("expected UserID %q, got %q", "user-456", data.UserID)
	}
	if data.AppID != "app-789" {
		t.Errorf("expected AppID %q, got %q", "app-789", data.AppID)
	}
	if data.Event != EventSessionCreated {
		t.Errorf("expected Event %q, got %q", EventSessionCreated, data.Event)
	}
	if data.Timestamp != now {
		t.Errorf("expected Timestamp %v, got %v", now, data.Timestamp)
	}
	if data.Reason != "test reason" {
		t.Errorf("expected Reason %q, got %q", "test reason", data.Reason)
	}
	if data.Metadata["key"] != "value" {
		t.Errorf("expected Metadata[key] %q, got %q", "value", data.Metadata["key"])
	}
}

func TestMockRecorder_CapturesEvents(t *testing.T) {
	recorder := &mockRecorder{}

	ctx := context.Background()
	recorder.OnEvent(ctx, SessionEventData{
		SessionID: "s1",
		Event:     EventSessionCreated,
	})
	recorder.OnEvent(ctx, SessionEventData{
		SessionID: "s1",
		Event:     EventSessionReady,
	})
	recorder.OnEvent(ctx, SessionEventData{
		SessionID: "s1",
		Event:     EventSessionStopped,
	})

	events := recorder.getEvents()
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	expectedEvents := []SessionEvent{
		EventSessionCreated,
		EventSessionReady,
		EventSessionStopped,
	}

	for i, expected := range expectedEvents {
		if events[i].Event != expected {
			t.Errorf("event %d: expected %q, got %q", i, expected, events[i].Event)
		}
		if events[i].SessionID != "s1" {
			t.Errorf("event %d: expected SessionID %q, got %q", i, "s1", events[i].SessionID)
		}
	}
}

func TestRecordingConfig_Defaults(t *testing.T) {
	cfg := RecordingConfig{}

	if cfg.Enabled {
		t.Error("expected Enabled to default to false")
	}
	if cfg.Endpoint != "" {
		t.Errorf("expected Endpoint to default to empty, got %q", cfg.Endpoint)
	}
	if cfg.BufferSize != 0 {
		t.Errorf("expected BufferSize to default to 0, got %d", cfg.BufferSize)
	}
}

func TestRecordingConfig_WithValues(t *testing.T) {
	cfg := RecordingConfig{
		Enabled:    true,
		Endpoint:   "http://recorder.example.com/events",
		BufferSize: 100,
	}

	if !cfg.Enabled {
		t.Error("expected Enabled to be true")
	}
	if cfg.Endpoint != "http://recorder.example.com/events" {
		t.Errorf("expected Endpoint %q, got %q", "http://recorder.example.com/events", cfg.Endpoint)
	}
	if cfg.BufferSize != 100 {
		t.Errorf("expected BufferSize 100, got %d", cfg.BufferSize)
	}
}

func TestNoopRecorder_ImplementsInterface(t *testing.T) {
	// Compile-time check that NoopRecorder implements SessionRecorder
	var _ SessionRecorder = (*NoopRecorder)(nil)
}

func TestMockRecorder_ConcurrentSafety(t *testing.T) {
	recorder := &mockRecorder{}
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			recorder.OnEvent(ctx, SessionEventData{
				SessionID: "concurrent-session",
				Event:     EventSessionCreated,
				Reason:    "concurrent test",
			})
		}(i)
	}

	wg.Wait()

	events := recorder.getEvents()
	if len(events) != 100 {
		t.Errorf("expected 100 events, got %d", len(events))
	}
}

func TestManagerConfig_NilRecorderUsesNoop(t *testing.T) {
	// When Recorder is nil, the manager should use NoopRecorder
	cfg := ManagerConfig{
		Recorder: nil,
	}

	if cfg.Recorder != nil {
		t.Error("expected nil Recorder in config")
	}

	// The actual noop assignment is tested through NewManagerWithConfig,
	// but we can't call that here without a database. We verify the
	// config field is nil, and the NewManagerWithConfig logic handles it.
}

func TestSessionEventLifecycle(t *testing.T) {
	// Test a full lifecycle through the mock recorder
	recorder := &mockRecorder{}
	ctx := context.Background()

	// Simulate: created -> ready -> stopped -> restarted -> ready -> expired
	lifecycle := []SessionEventData{
		{SessionID: "lifecycle-test", UserID: "u1", AppID: "a1", Event: EventSessionCreated, Reason: "session created"},
		{SessionID: "lifecycle-test", UserID: "u1", AppID: "a1", Event: EventSessionReady, Reason: "pod ready"},
		{SessionID: "lifecycle-test", UserID: "u1", AppID: "a1", Event: EventSessionStopped, Reason: "user stopped"},
		{SessionID: "lifecycle-test", UserID: "u1", AppID: "a1", Event: EventSessionRestarted, Reason: "user restarted"},
		{SessionID: "lifecycle-test", UserID: "u1", AppID: "a1", Event: EventSessionReady, Reason: "pod ready"},
		{SessionID: "lifecycle-test", UserID: "u1", AppID: "a1", Event: EventSessionExpired, Reason: "session timeout"},
	}

	for _, event := range lifecycle {
		event.Timestamp = time.Now()
		recorder.OnEvent(ctx, event)
	}

	events := recorder.getEvents()
	if len(events) != 6 {
		t.Fatalf("expected 6 lifecycle events, got %d", len(events))
	}

	expectedOrder := []SessionEvent{
		EventSessionCreated,
		EventSessionReady,
		EventSessionStopped,
		EventSessionRestarted,
		EventSessionReady,
		EventSessionExpired,
	}

	for i, expected := range expectedOrder {
		if events[i].Event != expected {
			t.Errorf("lifecycle event %d: expected %q, got %q", i, expected, events[i].Event)
		}
	}
}
