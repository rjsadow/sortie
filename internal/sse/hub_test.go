package sse

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rjsadow/sortie/internal/plugins"
	"github.com/rjsadow/sortie/internal/sessions"
)

// fakeAuth implements sse.Authenticator for testing.
type fakeAuth struct {
	user *plugins.User
}

func (f *fakeAuth) Authenticate(_ context.Context, token string) (*plugins.AuthResult, error) {
	if token == "valid" {
		return &plugins.AuthResult{Authenticated: true, User: f.user}, nil
	}
	return &plugins.AuthResult{Authenticated: false}, nil
}

func newTestHub() (*Hub, *fakeAuth) {
	auth := &fakeAuth{user: &plugins.User{ID: "user1", Username: "testuser"}}
	hub := NewHub(auth)
	return hub, auth
}

func TestHub_Unauthenticated_Returns401(t *testing.T) {
	hub, _ := newTestHub()

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/events?token=invalid", nil)
	rec := httptest.NewRecorder()
	hub.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestHub_NoToken_Returns401(t *testing.T) {
	hub, _ := newTestHub()

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/events", nil)
	rec := httptest.NewRecorder()
	hub.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestHub_MethodNotAllowed(t *testing.T) {
	hub, _ := newTestHub()

	req := httptest.NewRequest(http.MethodPost, "/api/sessions/events?token=valid", nil)
	rec := httptest.NewRecorder()
	hub.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestHub_ConnectedEventSent(t *testing.T) {
	hub, _ := newTestHub()

	// Use a real server so we get proper streaming
	ts := httptest.NewServer(hub)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/sessions/events?token=valid", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %s", ct)
	}

	// Read the first event (should be "connected")
	scanner := bufio.NewScanner(resp.Body)
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		// The connected event is: "event: connected\ndata: {}\n\n"
		if line == "" && len(lines) > 1 {
			break
		}
	}

	found := false
	for _, l := range lines {
		if l == "event: connected" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'event: connected' in initial output, got: %v", lines)
	}
}

func TestHub_EventRouting(t *testing.T) {
	hub, _ := newTestHub()

	ts := httptest.NewServer(hub)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/sessions/events?token=valid", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// Wait for the client to register
	deadline := time.Now().Add(1 * time.Second)
	for hub.ClientCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if hub.ClientCount() == 0 {
		t.Fatal("client did not register")
	}

	// Send a session event for user1 (matching the connected user)
	hub.OnEvent(context.Background(), sessions.SessionEventData{
		SessionID: "sess-1",
		UserID:    "user1",
		Event:     sessions.EventSessionReady,
	})

	// Read lines until we see the session event
	scanner := bufio.NewScanner(resp.Body)
	foundSession := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") && strings.Contains(line, "sess-1") {
			foundSession = true
			if !strings.Contains(line, `"status":"running"`) {
				t.Errorf("expected status running in event, got: %s", line)
			}
			break
		}
	}
	if !foundSession {
		t.Error("did not receive session event for matching user")
	}
}

func TestHub_EventNotRoutedToOtherUser(t *testing.T) {
	hub, _ := newTestHub()

	ts := httptest.NewServer(hub)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/sessions/events?token=valid", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// Wait for registration
	deadline := time.Now().Add(1 * time.Second)
	for hub.ClientCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	// Send event for a different user
	hub.OnEvent(context.Background(), sessions.SessionEventData{
		SessionID: "other-sess",
		UserID:    "other-user",
		Event:     sessions.EventSessionReady,
	})

	// Read with short timeout — we should NOT see the event
	scanner := bufio.NewScanner(resp.Body)
	gotOtherEvent := false
	done := make(chan struct{})
	go func() {
		defer close(done)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "other-sess") {
				gotOtherEvent = true
				return
			}
		}
	}()

	select {
	case <-done:
		if gotOtherEvent {
			t.Error("received event meant for a different user")
		}
	case <-time.After(500 * time.Millisecond):
		// Expected: no event for other user
	}
}

func TestHub_FullBufferNonBlocking(t *testing.T) {
	hub, _ := newTestHub()

	// Manually register a client with a tiny buffer
	c := &client{
		userID: "user1",
		ch:     make(chan sseEvent, 1),
	}
	hub.mu.Lock()
	hub.clients[c] = struct{}{}
	hub.mu.Unlock()

	// Send more events than the buffer can hold — should not block
	for i := 0; i < 100; i++ {
		hub.OnEvent(context.Background(), sessions.SessionEventData{
			SessionID: "flood",
			UserID:    "user1",
			Event:     sessions.EventSessionCreated,
		})
	}

	// If we got here without blocking, the test passes
	hub.mu.Lock()
	delete(hub.clients, c)
	hub.mu.Unlock()
}

func TestEventToStatus(t *testing.T) {
	tests := []struct {
		event  sessions.SessionEvent
		status string
	}{
		{sessions.EventSessionCreated, "creating"},
		{sessions.EventSessionReady, "running"},
		{sessions.EventSessionFailed, "failed"},
		{sessions.EventSessionStopped, "stopped"},
		{sessions.EventSessionRestarted, "creating"},
		{sessions.EventSessionExpired, "expired"},
		{sessions.EventSessionTerminated, "stopped"},
		{sessions.SessionEvent("unknown.event"), "unknown"},
	}

	for _, tt := range tests {
		got := eventToStatus(tt.event)
		if got != tt.status {
			t.Errorf("eventToStatus(%q) = %q, want %q", tt.event, got, tt.status)
		}
	}
}

func TestHub_ImplementsSessionRecorder(t *testing.T) {
	var _ sessions.SessionRecorder = (*Hub)(nil)
}

func TestHub_ClientCount(t *testing.T) {
	hub, _ := newTestHub()

	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients, got %d", hub.ClientCount())
	}

	c := &client{userID: "u1", ch: make(chan sseEvent, 1)}
	hub.mu.Lock()
	hub.clients[c] = struct{}{}
	hub.mu.Unlock()

	if hub.ClientCount() != 1 {
		t.Errorf("expected 1 client, got %d", hub.ClientCount())
	}

	hub.mu.Lock()
	delete(hub.clients, c)
	hub.mu.Unlock()

	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients after removal, got %d", hub.ClientCount())
	}
}
