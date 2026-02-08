// Package billing provides metering event collection and billing export
// for tracking active users, session-hours, and resource usage.
package billing

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/rjsadow/launchpad/internal/sessions"
)

// MeteringEventType identifies the kind of metering event.
type MeteringEventType string

const (
	// EventActiveUser is emitted when a user starts or ends a session.
	EventActiveUser MeteringEventType = "active_user"

	// EventSessionHour is emitted to record session duration for billing.
	EventSessionHour MeteringEventType = "session_hour"

	// EventResourceUsage is emitted to record CPU/memory resource consumption.
	EventResourceUsage MeteringEventType = "resource_usage"

	// EventQuotaExceeded is emitted when a user hits a quota limit.
	EventQuotaExceeded MeteringEventType = "quota_exceeded"
)

// MeteringEvent represents a single billable event.
type MeteringEvent struct {
	ID        string            `json:"id"`
	Type      MeteringEventType `json:"type"`
	UserID    string            `json:"user_id"`
	SessionID string            `json:"session_id,omitempty"`
	AppID     string            `json:"app_id,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Quantity  float64           `json:"quantity"`  // e.g., hours, CPU-seconds, bytes
	Unit      string            `json:"unit"`      // e.g., "hours", "cpu_seconds", "bytes"
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// activeSession tracks a running session for duration calculation.
type activeSession struct {
	UserID    string
	AppID     string
	StartedAt time.Time
	CPULimit  string
	MemLimit  string
}

// Collector implements sessions.SessionRecorder to collect metering events
// from session lifecycle hooks. It tracks active users and session durations,
// and emits metering events that can be exported for billing.
type Collector struct {
	mu             sync.Mutex
	events         []MeteringEvent
	activeSessions map[string]*activeSession // sessionID -> activeSession
	activeUsers    map[string]int            // userID -> active session count
	eventCounter   uint64
}

// NewCollector creates a new metering event collector.
func NewCollector() *Collector {
	return &Collector{
		events:         make([]MeteringEvent, 0),
		activeSessions: make(map[string]*activeSession),
		activeUsers:    make(map[string]int),
	}
}

// OnEvent implements sessions.SessionRecorder. It processes session lifecycle
// events and generates corresponding metering events.
func (c *Collector) OnEvent(_ context.Context, event sessions.SessionEventData) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch event.Event {
	case sessions.EventSessionReady:
		c.handleSessionReady(event)
	case sessions.EventSessionStopped, sessions.EventSessionExpired, sessions.EventSessionTerminated:
		c.handleSessionEnd(event)
	case sessions.EventSessionFailed:
		c.handleSessionFailed(event)
	}
}

// handleSessionReady records the start of a billable session.
func (c *Collector) handleSessionReady(event sessions.SessionEventData) {
	c.activeSessions[event.SessionID] = &activeSession{
		UserID:    event.UserID,
		AppID:     event.AppID,
		StartedAt: event.Timestamp,
		CPULimit:  event.Metadata["cpu_limit"],
		MemLimit:  event.Metadata["mem_limit"],
	}

	// Track active user count
	prev := c.activeUsers[event.UserID]
	c.activeUsers[event.UserID] = prev + 1

	// Emit active_user event on first session for this user
	if prev == 0 {
		c.appendEvent(MeteringEvent{
			ID:        c.nextEventID(),
			Type:      EventActiveUser,
			UserID:    event.UserID,
			SessionID: event.SessionID,
			AppID:     event.AppID,
			Timestamp: event.Timestamp,
			Quantity:  1,
			Unit:      "active",
			Metadata:  map[string]string{"action": "start"},
		})
	}
}

// handleSessionEnd records session duration and resource usage.
func (c *Collector) handleSessionEnd(event sessions.SessionEventData) {
	active, ok := c.activeSessions[event.SessionID]
	if !ok {
		log.Printf("billing: session end event for unknown session %s", event.SessionID)
		return
	}

	duration := event.Timestamp.Sub(active.StartedAt)
	hours := duration.Hours()

	// Emit session_hour event
	c.appendEvent(MeteringEvent{
		ID:        c.nextEventID(),
		Type:      EventSessionHour,
		UserID:    active.UserID,
		SessionID: event.SessionID,
		AppID:     active.AppID,
		Timestamp: event.Timestamp,
		Quantity:  hours,
		Unit:      "hours",
		Metadata: map[string]string{
			"started_at": active.StartedAt.Format(time.RFC3339),
			"ended_at":   event.Timestamp.Format(time.RFC3339),
			"reason":     event.Reason,
		},
	})

	// Emit resource_usage event if resource info is available
	if active.CPULimit != "" || active.MemLimit != "" {
		meta := map[string]string{
			"duration_seconds": formatFloat(duration.Seconds()),
		}
		if active.CPULimit != "" {
			meta["cpu_limit"] = active.CPULimit
		}
		if active.MemLimit != "" {
			meta["mem_limit"] = active.MemLimit
		}
		c.appendEvent(MeteringEvent{
			ID:        c.nextEventID(),
			Type:      EventResourceUsage,
			UserID:    active.UserID,
			SessionID: event.SessionID,
			AppID:     active.AppID,
			Timestamp: event.Timestamp,
			Quantity:  duration.Seconds(),
			Unit:      "cpu_seconds",
			Metadata:  meta,
		})
	}

	// Update active user tracking
	c.activeUsers[active.UserID]--
	if c.activeUsers[active.UserID] <= 0 {
		delete(c.activeUsers, active.UserID)
		c.appendEvent(MeteringEvent{
			ID:        c.nextEventID(),
			Type:      EventActiveUser,
			UserID:    active.UserID,
			SessionID: event.SessionID,
			AppID:     active.AppID,
			Timestamp: event.Timestamp,
			Quantity:  0,
			Unit:      "active",
			Metadata:  map[string]string{"action": "stop"},
		})
	}

	delete(c.activeSessions, event.SessionID)
}

// handleSessionFailed records a failed session (no billing, but track the event).
func (c *Collector) handleSessionFailed(event sessions.SessionEventData) {
	// Clean up if we were tracking this session
	if active, ok := c.activeSessions[event.SessionID]; ok {
		c.activeUsers[active.UserID]--
		if c.activeUsers[active.UserID] <= 0 {
			delete(c.activeUsers, active.UserID)
		}
		delete(c.activeSessions, event.SessionID)
	}
}

// DrainEvents returns all collected events and clears the buffer.
// This is used by exporters to retrieve events for billing.
func (c *Collector) DrainEvents() []MeteringEvent {
	c.mu.Lock()
	defer c.mu.Unlock()

	events := c.events
	c.events = make([]MeteringEvent, 0)
	return events
}

// PeekEvents returns a copy of collected events without clearing the buffer.
func (c *Collector) PeekEvents() []MeteringEvent {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := make([]MeteringEvent, len(c.events))
	copy(result, c.events)
	return result
}

// ActiveUserCount returns the current number of active users.
func (c *Collector) ActiveUserCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.activeUsers)
}

// ActiveSessionCount returns the current number of active sessions.
func (c *Collector) ActiveSessionCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.activeSessions)
}

func (c *Collector) appendEvent(event MeteringEvent) {
	c.events = append(c.events, event)
}

func (c *Collector) nextEventID() string {
	c.eventCounter++
	return formatUint(c.eventCounter)
}

func formatFloat(f float64) string {
	return time.Duration(int64(f * float64(time.Second))).String()
}

func formatUint(n uint64) string {
	// Simple uint-to-string without importing strconv in hot path
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
