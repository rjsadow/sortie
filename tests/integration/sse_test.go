package integration

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rjsadow/sortie/tests/integration/testutil"
)

// sseEvent represents a parsed SSE data payload.
type sseEvent struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
}

// connectSSE opens an EventSource connection and reads past the initial "connected" event.
// Returns the response and a scanner positioned after the connected event block.
func connectSSE(t *testing.T, ts *testutil.TestServer, token string, ctx context.Context) (*http.Response, *bufio.Scanner) {
	t.Helper()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		ts.URL+"/api/sessions/events?token="+token, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}

	scanner := bufio.NewScanner(resp.Body)
	// Read past "event: connected\ndata: {}\n\n"
	for scanner.Scan() {
		if scanner.Text() == "" {
			break
		}
	}

	return resp, scanner
}

// readNextEvent reads SSE lines until it finds a data payload, parses it, and returns it.
// Returns nil if the context expires or the stream ends before an event arrives.
func readNextEvent(scanner *bufio.Scanner) *sseEvent {
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			raw := strings.TrimPrefix(line, "data:")
			raw = strings.TrimSpace(raw)
			var evt sseEvent
			if err := json.Unmarshal([]byte(raw), &evt); err != nil {
				continue
			}
			return &evt
		}
	}
	return nil
}

// collectEvents reads SSE events until timeout, returning all parsed events.
// The caller must pass the response body so it can be closed on timeout to
// unblock the scanner goroutine and prevent goroutine leaks.
func collectEvents(scanner *bufio.Scanner, body io.Closer, timeout time.Duration) []sseEvent {
	var events []sseEvent
	done := make(chan struct{})
	go func() {
		defer close(done)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data:") {
				raw := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				var evt sseEvent
				if json.Unmarshal([]byte(raw), &evt) == nil {
					events = append(events, evt)
				}
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		body.Close() // Unblock scanner.Scan()
		<-done        // Wait for goroutine to exit
	}
	return events
}

func TestSSE_ConnectedEvent(t *testing.T) {
	ts := testutil.NewTestServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		ts.URL+"/api/sessions/events?token="+ts.AdminToken, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %s", ct)
	}

	// Read lines until we see the connected event
	scanner := bufio.NewScanner(resp.Body)
	found := false
	for scanner.Scan() {
		if scanner.Text() == "event: connected" {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not receive 'event: connected'")
	}
}

func TestSSE_Unauthenticated(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// No token at all
	resp, err := http.Get(ts.URL + "/api/sessions/events")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no token: expected 401, got %d", resp.StatusCode)
	}

	// Invalid token
	resp, err = http.Get(ts.URL + "/api/sessions/events?token=bogus-invalid-token")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("invalid token: expected 401, got %d", resp.StatusCode)
	}
}

func TestSSE_EventPayloadStructure(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create app
	appBody := []byte(`{"id":"sse-payload-app","name":"SSE Payload App","launch_type":"container","container_image":"nginx:latest"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken, appBody)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("failed to create app: status %d", resp.StatusCode)
	}

	// Connect SSE
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sseResp, scanner := connectSSE(t, ts, ts.AdminToken, ctx)
	defer sseResp.Body.Close()

	// Create session
	adminID := getAdminUserID(t, ts)
	sessBody := []byte(fmt.Sprintf(`{"app_id":"sse-payload-app","user_id":"%s"}`, adminID))
	resp = testutil.AuthPost(t, ts.URL+"/api/sessions", ts.AdminToken, sessBody)
	var sess map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&sess)
	resp.Body.Close()
	sessionID := sess["id"].(string)

	// Read the first event and verify JSON payload structure
	evt := readNextEvent(scanner)
	if evt == nil {
		t.Fatal("did not receive any SSE event")
	}

	if evt.SessionID != sessionID {
		t.Errorf("expected session_id %q, got %q", sessionID, evt.SessionID)
	}
	if evt.Type == "" {
		t.Error("expected non-empty type field")
	}
	if evt.Status == "" {
		t.Error("expected non-empty status field")
	}
	// First event should be session.created with status "creating"
	if evt.Type != "session.created" {
		t.Errorf("expected first event type session.created, got %q", evt.Type)
	}
	if evt.Status != "creating" {
		t.Errorf("expected first event status creating, got %q", evt.Status)
	}
}

func TestSSE_LifecycleEvents(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create app
	appBody := []byte(`{"id":"sse-lifecycle","name":"SSE Lifecycle","launch_type":"container","container_image":"nginx:latest"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken, appBody)
	resp.Body.Close()

	// Connect SSE
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	sseResp, scanner := connectSSE(t, ts, ts.AdminToken, ctx)
	defer sseResp.Body.Close()

	// Create session
	adminID := getAdminUserID(t, ts)
	sessBody := []byte(fmt.Sprintf(`{"app_id":"sse-lifecycle","user_id":"%s"}`, adminID))
	resp = testutil.AuthPost(t, ts.URL+"/api/sessions", ts.AdminToken, sessBody)
	var sess map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&sess)
	resp.Body.Close()
	sessionID := sess["id"].(string)

	// Collect session.created event
	evt := readNextEvent(scanner)
	if evt == nil || evt.Type != "session.created" {
		t.Fatalf("expected session.created, got %+v", evt)
	}

	// Wait for session to become running (mock runner is near-instant)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r := testutil.AuthGet(t, ts.URL+"/api/sessions/"+sessionID, ts.AdminToken)
		var s map[string]interface{}
		json.NewDecoder(r.Body).Decode(&s)
		r.Body.Close()
		if s["status"] == "running" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Should have received session.ready event
	evt = readNextEvent(scanner)
	if evt == nil {
		t.Fatal("did not receive session.ready event")
	}
	if evt.Type != "session.ready" {
		t.Errorf("expected session.ready, got %q", evt.Type)
	}
	if evt.Status != "running" {
		t.Errorf("expected status running, got %q", evt.Status)
	}

	// Terminate the session
	resp = testutil.AuthDelete(t, ts.URL+"/api/sessions/"+sessionID, ts.AdminToken)
	resp.Body.Close()

	// Should receive session.terminated event
	evt = readNextEvent(scanner)
	if evt == nil {
		t.Fatal("did not receive session.terminated event")
	}
	if evt.Type != "session.terminated" {
		t.Errorf("expected session.terminated, got %q", evt.Type)
	}
	if evt.Status != "stopped" {
		t.Errorf("expected status stopped, got %q", evt.Status)
	}
}

func TestSSE_MultiUserIsolation(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create a second user
	testutil.CreateUser(t, ts.URL, ts.AdminToken, "alice", "alice123", []string{"user"})
	aliceToken := testutil.LoginAs(t, ts.URL, "alice", "alice123")

	// Create app
	appBody := []byte(`{"id":"sse-iso-app","name":"SSE Isolation","launch_type":"container","container_image":"nginx:latest"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken, appBody)
	resp.Body.Close()

	// Alice connects to SSE
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	aliceSSE, aliceScanner := connectSSE(t, ts, aliceToken, ctx)
	defer aliceSSE.Body.Close()

	// Admin creates a session (as admin, not alice)
	adminID := getAdminUserID(t, ts)
	sessBody := []byte(fmt.Sprintf(`{"app_id":"sse-iso-app","user_id":"%s"}`, adminID))
	resp = testutil.AuthPost(t, ts.URL+"/api/sessions", ts.AdminToken, sessBody)
	resp.Body.Close()

	// Alice should NOT receive admin's session events.
	// Pass the response body so collectEvents can close it on timeout.
	events := collectEvents(aliceScanner, aliceSSE.Body, 1*time.Second)
	for _, evt := range events {
		t.Errorf("alice received event that should have gone to admin: %+v", evt)
	}
}

func TestSSE_StopAndRestartEvents(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create app
	appBody := []byte(`{"id":"sse-stop-restart","name":"SSE StopRestart","launch_type":"container","container_image":"nginx:latest"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken, appBody)
	resp.Body.Close()

	// Connect SSE
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	sseResp, scanner := connectSSE(t, ts, ts.AdminToken, ctx)
	defer sseResp.Body.Close()

	// Create session and wait for running
	adminID := getAdminUserID(t, ts)
	sessBody := []byte(fmt.Sprintf(`{"app_id":"sse-stop-restart","user_id":"%s"}`, adminID))
	resp = testutil.AuthPost(t, ts.URL+"/api/sessions", ts.AdminToken, sessBody)
	var sess map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&sess)
	resp.Body.Close()
	sessionID := sess["id"].(string)

	// Drain created + ready events
	readNextEvent(scanner) // session.created
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r := testutil.AuthGet(t, ts.URL+"/api/sessions/"+sessionID, ts.AdminToken)
		var s map[string]interface{}
		json.NewDecoder(r.Body).Decode(&s)
		r.Body.Close()
		if s["status"] == "running" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	readNextEvent(scanner) // session.ready

	// Stop the session
	resp = testutil.AuthPost(t, ts.URL+"/api/sessions/"+sessionID+"/stop", ts.AdminToken, nil)
	resp.Body.Close()

	evt := readNextEvent(scanner)
	if evt == nil {
		t.Fatal("did not receive session.stopped event")
	}
	if evt.Type != "session.stopped" {
		t.Errorf("expected session.stopped, got %q", evt.Type)
	}

	// Restart the session
	resp = testutil.AuthPost(t, ts.URL+"/api/sessions/"+sessionID+"/restart", ts.AdminToken, nil)
	resp.Body.Close()

	evt = readNextEvent(scanner)
	if evt == nil {
		t.Fatal("did not receive session.restarted event")
	}
	if evt.Type != "session.restarted" {
		t.Errorf("expected session.restarted, got %q", evt.Type)
	}
	if evt.Status != "creating" {
		t.Errorf("expected status creating, got %q", evt.Status)
	}
}

func TestSSE_TerminateOneOfMultipleSessions(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create app
	appBody := []byte(`{"id":"sse-multi-sess","name":"SSE MultiSess","launch_type":"container","container_image":"nginx:latest"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken, appBody)
	resp.Body.Close()

	// Connect SSE
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	sseResp, scanner := connectSSE(t, ts, ts.AdminToken, ctx)
	defer sseResp.Body.Close()

	adminID := getAdminUserID(t, ts)

	// Create two sessions
	sessBody := []byte(fmt.Sprintf(`{"app_id":"sse-multi-sess","user_id":"%s"}`, adminID))
	resp = testutil.AuthPost(t, ts.URL+"/api/sessions", ts.AdminToken, sessBody)
	var sess1 map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&sess1)
	resp.Body.Close()
	sess1ID := sess1["id"].(string)

	// Drain session1 created event
	evt := readNextEvent(scanner)
	if evt == nil || evt.Type != "session.created" {
		t.Fatalf("expected session.created for sess1, got %+v", evt)
	}

	resp = testutil.AuthPost(t, ts.URL+"/api/sessions", ts.AdminToken, sessBody)
	var sess2 map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&sess2)
	resp.Body.Close()
	sess2ID := sess2["id"].(string)

	// Drain session2 created event
	evt = readNextEvent(scanner)
	if evt == nil || evt.Type != "session.created" {
		t.Fatalf("expected session.created for sess2, got %+v", evt)
	}

	// Wait for both sessions to be running
	for _, sid := range []string{sess1ID, sess2ID} {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			r := testutil.AuthGet(t, ts.URL+"/api/sessions/"+sid, ts.AdminToken)
			var s map[string]interface{}
			json.NewDecoder(r.Body).Decode(&s)
			r.Body.Close()
			if s["status"] == "running" {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	// Drain ready events for both sessions
	readNextEvent(scanner) // session.ready for sess1
	readNextEvent(scanner) // session.ready for sess2

	// Verify both sessions are running via API
	listResp := testutil.AuthGet(t, ts.URL+"/api/sessions", ts.AdminToken)
	var sessions []map[string]interface{}
	json.NewDecoder(listResp.Body).Decode(&sessions)
	listResp.Body.Close()
	runningCount := 0
	for _, s := range sessions {
		if s["status"] == "running" {
			runningCount++
		}
	}
	if runningCount != 2 {
		t.Fatalf("expected 2 running sessions, got %d", runningCount)
	}

	// Terminate session 1 — should get terminated event, session 2 stays running
	resp = testutil.AuthDelete(t, ts.URL+"/api/sessions/"+sess1ID, ts.AdminToken)
	resp.Body.Close()

	evt = readNextEvent(scanner)
	if evt == nil {
		t.Fatal("did not receive session.terminated event")
	}
	if evt.Type != "session.terminated" {
		t.Errorf("expected session.terminated, got %q", evt.Type)
	}
	if evt.SessionID != sess1ID {
		t.Errorf("expected terminated session_id %q, got %q", sess1ID, evt.SessionID)
	}

	// Verify only one session remains running
	listResp = testutil.AuthGet(t, ts.URL+"/api/sessions", ts.AdminToken)
	json.NewDecoder(listResp.Body).Decode(&sessions)
	listResp.Body.Close()
	runningCount = 0
	for _, s := range sessions {
		if s["status"] == "running" {
			runningCount++
		}
	}
	if runningCount != 1 {
		t.Errorf("expected 1 running session after termination, got %d", runningCount)
	}
}

// getAdminUserID fetches the admin user ID via the /api/auth/me endpoint.
func getAdminUserID(t *testing.T, ts *testutil.TestServer) string {
	t.Helper()
	resp := testutil.AuthGet(t, ts.URL+"/api/auth/me", ts.AdminToken)
	defer resp.Body.Close()
	var user struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		t.Fatalf("failed to decode /api/auth/me: %v", err)
	}
	return user.ID
}
