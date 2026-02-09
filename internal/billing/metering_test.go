package billing

import (
	"context"
	"testing"
	"time"

	"github.com/rjsadow/sortie/internal/sessions"
)

func TestCollectorImplementsSessionRecorder(t *testing.T) {
	var _ sessions.SessionRecorder = (*Collector)(nil)
}

func TestCollectorActiveUserTracking(t *testing.T) {
	c := NewCollector()
	now := time.Now()

	// User starts first session
	c.OnEvent(context.Background(), sessions.SessionEventData{
		SessionID: "s1",
		UserID:    "user1",
		AppID:     "app1",
		Event:     sessions.EventSessionReady,
		Timestamp: now,
	})

	if c.ActiveUserCount() != 1 {
		t.Errorf("expected 1 active user, got %d", c.ActiveUserCount())
	}
	if c.ActiveSessionCount() != 1 {
		t.Errorf("expected 1 active session, got %d", c.ActiveSessionCount())
	}

	// Same user starts second session
	c.OnEvent(context.Background(), sessions.SessionEventData{
		SessionID: "s2",
		UserID:    "user1",
		AppID:     "app2",
		Event:     sessions.EventSessionReady,
		Timestamp: now.Add(time.Minute),
	})

	if c.ActiveUserCount() != 1 {
		t.Errorf("expected 1 active user (same user), got %d", c.ActiveUserCount())
	}
	if c.ActiveSessionCount() != 2 {
		t.Errorf("expected 2 active sessions, got %d", c.ActiveSessionCount())
	}

	// Different user starts session
	c.OnEvent(context.Background(), sessions.SessionEventData{
		SessionID: "s3",
		UserID:    "user2",
		AppID:     "app1",
		Event:     sessions.EventSessionReady,
		Timestamp: now.Add(2 * time.Minute),
	})

	if c.ActiveUserCount() != 2 {
		t.Errorf("expected 2 active users, got %d", c.ActiveUserCount())
	}

	// User1 ends first session
	c.OnEvent(context.Background(), sessions.SessionEventData{
		SessionID: "s1",
		UserID:    "user1",
		AppID:     "app1",
		Event:     sessions.EventSessionStopped,
		Timestamp: now.Add(3 * time.Minute),
	})

	if c.ActiveUserCount() != 2 {
		t.Errorf("expected 2 active users (user1 still has s2), got %d", c.ActiveUserCount())
	}

	// User1 ends second session
	c.OnEvent(context.Background(), sessions.SessionEventData{
		SessionID: "s2",
		UserID:    "user1",
		AppID:     "app2",
		Event:     sessions.EventSessionExpired,
		Timestamp: now.Add(4 * time.Minute),
	})

	if c.ActiveUserCount() != 1 {
		t.Errorf("expected 1 active user (only user2), got %d", c.ActiveUserCount())
	}
}

func TestCollectorSessionHourEvents(t *testing.T) {
	c := NewCollector()
	start := time.Now()
	end := start.Add(2 * time.Hour)

	// Start session
	c.OnEvent(context.Background(), sessions.SessionEventData{
		SessionID: "s1",
		UserID:    "user1",
		AppID:     "app1",
		Event:     sessions.EventSessionReady,
		Timestamp: start,
	})

	// End session after 2 hours
	c.OnEvent(context.Background(), sessions.SessionEventData{
		SessionID: "s1",
		UserID:    "user1",
		AppID:     "app1",
		Event:     sessions.EventSessionStopped,
		Timestamp: end,
		Reason:    "user stopped",
	})

	events := c.DrainEvents()

	// Should have: active_user start, session_hour, active_user stop
	var sessionHourEvent *MeteringEvent
	for i, e := range events {
		if e.Type == EventSessionHour {
			sessionHourEvent = &events[i]
			break
		}
	}

	if sessionHourEvent == nil {
		t.Fatal("expected session_hour event")
	}

	if sessionHourEvent.Quantity != 2.0 {
		t.Errorf("expected 2.0 hours, got %f", sessionHourEvent.Quantity)
	}
	if sessionHourEvent.Unit != "hours" {
		t.Errorf("expected unit 'hours', got %q", sessionHourEvent.Unit)
	}
	if sessionHourEvent.UserID != "user1" {
		t.Errorf("expected user1, got %q", sessionHourEvent.UserID)
	}
}

func TestCollectorResourceUsageEvents(t *testing.T) {
	c := NewCollector()
	start := time.Now()

	// Start session with resource metadata
	c.OnEvent(context.Background(), sessions.SessionEventData{
		SessionID: "s1",
		UserID:    "user1",
		AppID:     "app1",
		Event:     sessions.EventSessionReady,
		Timestamp: start,
		Metadata: map[string]string{
			"cpu_limit": "2",
			"mem_limit": "4Gi",
		},
	})

	// End session
	c.OnEvent(context.Background(), sessions.SessionEventData{
		SessionID: "s1",
		UserID:    "user1",
		AppID:     "app1",
		Event:     sessions.EventSessionStopped,
		Timestamp: start.Add(30 * time.Minute),
	})

	events := c.DrainEvents()

	var resourceEvent *MeteringEvent
	for i, e := range events {
		if e.Type == EventResourceUsage {
			resourceEvent = &events[i]
			break
		}
	}

	if resourceEvent == nil {
		t.Fatal("expected resource_usage event")
	}

	if resourceEvent.Unit != "cpu_seconds" {
		t.Errorf("expected unit 'cpu_seconds', got %q", resourceEvent.Unit)
	}

	expectedSeconds := (30 * time.Minute).Seconds()
	if resourceEvent.Quantity != expectedSeconds {
		t.Errorf("expected %f seconds, got %f", expectedSeconds, resourceEvent.Quantity)
	}

	if resourceEvent.Metadata["cpu_limit"] != "2" {
		t.Errorf("expected cpu_limit '2', got %q", resourceEvent.Metadata["cpu_limit"])
	}
	if resourceEvent.Metadata["mem_limit"] != "4Gi" {
		t.Errorf("expected mem_limit '4Gi', got %q", resourceEvent.Metadata["mem_limit"])
	}
}

func TestCollectorNoResourceEventWithoutLimits(t *testing.T) {
	c := NewCollector()
	start := time.Now()

	// Start session without resource metadata
	c.OnEvent(context.Background(), sessions.SessionEventData{
		SessionID: "s1",
		UserID:    "user1",
		AppID:     "app1",
		Event:     sessions.EventSessionReady,
		Timestamp: start,
	})

	// End session
	c.OnEvent(context.Background(), sessions.SessionEventData{
		SessionID: "s1",
		UserID:    "user1",
		AppID:     "app1",
		Event:     sessions.EventSessionStopped,
		Timestamp: start.Add(time.Hour),
	})

	events := c.DrainEvents()

	for _, e := range events {
		if e.Type == EventResourceUsage {
			t.Error("did not expect resource_usage event without resource metadata")
		}
	}
}

func TestCollectorSessionFailedCleanup(t *testing.T) {
	c := NewCollector()

	// Start session
	c.OnEvent(context.Background(), sessions.SessionEventData{
		SessionID: "s1",
		UserID:    "user1",
		AppID:     "app1",
		Event:     sessions.EventSessionReady,
		Timestamp: time.Now(),
	})

	if c.ActiveSessionCount() != 1 {
		t.Errorf("expected 1 active session, got %d", c.ActiveSessionCount())
	}

	// Session fails
	c.OnEvent(context.Background(), sessions.SessionEventData{
		SessionID: "s1",
		UserID:    "user1",
		AppID:     "app1",
		Event:     sessions.EventSessionFailed,
		Timestamp: time.Now(),
		Reason:    "pod crash",
	})

	if c.ActiveSessionCount() != 0 {
		t.Errorf("expected 0 active sessions after failure, got %d", c.ActiveSessionCount())
	}
	if c.ActiveUserCount() != 0 {
		t.Errorf("expected 0 active users after failure, got %d", c.ActiveUserCount())
	}
}

func TestCollectorDrainEvents(t *testing.T) {
	c := NewCollector()

	c.OnEvent(context.Background(), sessions.SessionEventData{
		SessionID: "s1",
		UserID:    "user1",
		AppID:     "app1",
		Event:     sessions.EventSessionReady,
		Timestamp: time.Now(),
	})

	events := c.DrainEvents()
	if len(events) == 0 {
		t.Fatal("expected events after drain")
	}

	// Second drain should be empty
	events2 := c.DrainEvents()
	if len(events2) != 0 {
		t.Errorf("expected empty after second drain, got %d events", len(events2))
	}
}

func TestCollectorPeekEvents(t *testing.T) {
	c := NewCollector()

	c.OnEvent(context.Background(), sessions.SessionEventData{
		SessionID: "s1",
		UserID:    "user1",
		AppID:     "app1",
		Event:     sessions.EventSessionReady,
		Timestamp: time.Now(),
	})

	events1 := c.PeekEvents()
	events2 := c.PeekEvents()

	if len(events1) != len(events2) {
		t.Errorf("peek should not consume events: got %d and %d", len(events1), len(events2))
	}
}

func TestCollectorIgnoresUnknownSession(t *testing.T) {
	c := NewCollector()

	// End event for session we never tracked should not panic
	c.OnEvent(context.Background(), sessions.SessionEventData{
		SessionID: "unknown",
		UserID:    "user1",
		AppID:     "app1",
		Event:     sessions.EventSessionStopped,
		Timestamp: time.Now(),
	})

	if c.ActiveSessionCount() != 0 {
		t.Errorf("expected 0 sessions, got %d", c.ActiveSessionCount())
	}
}

func TestCollectorActiveUserEvents(t *testing.T) {
	c := NewCollector()
	now := time.Now()

	// Start session
	c.OnEvent(context.Background(), sessions.SessionEventData{
		SessionID: "s1",
		UserID:    "user1",
		AppID:     "app1",
		Event:     sessions.EventSessionReady,
		Timestamp: now,
	})

	// End session
	c.OnEvent(context.Background(), sessions.SessionEventData{
		SessionID: "s1",
		UserID:    "user1",
		AppID:     "app1",
		Event:     sessions.EventSessionStopped,
		Timestamp: now.Add(time.Hour),
	})

	events := c.DrainEvents()

	var startEvents, stopEvents int
	for _, e := range events {
		if e.Type == EventActiveUser {
			if e.Metadata["action"] == "start" {
				startEvents++
			}
			if e.Metadata["action"] == "stop" {
				stopEvents++
			}
		}
	}

	if startEvents != 1 {
		t.Errorf("expected 1 active_user start event, got %d", startEvents)
	}
	if stopEvents != 1 {
		t.Errorf("expected 1 active_user stop event, got %d", stopEvents)
	}
}
