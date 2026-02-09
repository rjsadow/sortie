package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/rjsadow/sortie/tests/integration/testutil"
)

func createContainerApp(t *testing.T, ts *testutil.TestServer, id string) {
	t.Helper()
	body := []byte(fmt.Sprintf(`{"id":%q,"name":"Test App %s","launch_type":"container","container_image":"nginx:latest"}`, id, id))
	resp := testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken, body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("failed to create app %s: status %d", id, resp.StatusCode)
	}
}

func createWindowsApp(t *testing.T, ts *testutil.TestServer, id string) {
	t.Helper()
	body := []byte(fmt.Sprintf(`{"id":%q,"name":"Win App %s","launch_type":"container","os_type":"windows","container_image":"mcr.microsoft.com/windows:latest"}`, id, id))
	resp := testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken, body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("failed to create windows app %s: status %d", id, resp.StatusCode)
	}
}

func TestSession_CreateSession(t *testing.T) {
	ts := testutil.NewTestServer(t)
	createContainerApp(t, ts, "sess-app")

	body := []byte(`{"app_id":"sess-app","user_id":"testuser"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/sessions", ts.AdminToken, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(b))
	}

	var session map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&session)

	if session["id"] == nil || session["id"] == "" {
		t.Error("expected session ID")
	}
	if session["status"] == nil {
		t.Error("expected session status")
	}
}

func TestSession_BecomesRunning(t *testing.T) {
	ts := testutil.NewTestServer(t)
	createContainerApp(t, ts, "run-app")

	// Create session
	body := []byte(`{"app_id":"run-app","user_id":"testuser"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/sessions", ts.AdminToken, body)
	var session map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&session)
	resp.Body.Close()

	sessionID := session["id"].(string)

	// Poll for running status (mock runner makes it instant)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp = testutil.AuthGet(t, ts.URL+"/api/sessions/"+sessionID, ts.AdminToken)
		json.NewDecoder(resp.Body).Decode(&session)
		resp.Body.Close()

		if session["status"] == "running" {
			// Verify websocket_url is populated for container apps
			if session["websocket_url"] == nil || session["websocket_url"] == "" {
				t.Error("expected websocket_url for container session")
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Errorf("session did not reach running status, last status: %v", session["status"])
}

func TestSession_WindowsApp(t *testing.T) {
	ts := testutil.NewTestServer(t)
	createWindowsApp(t, ts, "win-sess-app")

	body := []byte(`{"app_id":"win-sess-app","user_id":"testuser"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/sessions", ts.AdminToken, body)
	var session map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&session)
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	sessionID := session["id"].(string)

	// Poll for running
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp = testutil.AuthGet(t, ts.URL+"/api/sessions/"+sessionID, ts.AdminToken)
		json.NewDecoder(resp.Body).Decode(&session)
		resp.Body.Close()

		if session["status"] == "running" {
			if session["guacamole_url"] == nil || session["guacamole_url"] == "" {
				t.Error("expected guacamole_url for windows session")
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Errorf("windows session did not reach running, last status: %v", session["status"])
}

func TestSession_StopRunningSession(t *testing.T) {
	ts := testutil.NewTestServer(t)
	createContainerApp(t, ts, "stop-app")

	// Create and wait for running
	body := []byte(`{"app_id":"stop-app","user_id":"testuser"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/sessions", ts.AdminToken, body)
	var session map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&session)
	resp.Body.Close()

	sessionID := session["id"].(string)
	waitForRunning(t, ts, sessionID)

	// Stop
	resp = testutil.AuthPost(t, ts.URL+"/api/sessions/"+sessionID+"/stop", ts.AdminToken, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}

	json.NewDecoder(resp.Body).Decode(&session)
	if session["status"] != "stopped" {
		t.Errorf("expected stopped, got %v", session["status"])
	}
}

func TestSession_RestartStoppedSession(t *testing.T) {
	ts := testutil.NewTestServer(t)
	createContainerApp(t, ts, "restart-app")

	// Create and wait for running
	body := []byte(`{"app_id":"restart-app","user_id":"testuser"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/sessions", ts.AdminToken, body)
	var session map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&session)
	resp.Body.Close()

	sessionID := session["id"].(string)
	waitForRunning(t, ts, sessionID)

	// Stop
	resp = testutil.AuthPost(t, ts.URL+"/api/sessions/"+sessionID+"/stop", ts.AdminToken, nil)
	resp.Body.Close()

	// Restart
	resp = testutil.AuthPost(t, ts.URL+"/api/sessions/"+sessionID+"/restart", ts.AdminToken, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}

	json.NewDecoder(resp.Body).Decode(&session)
	// After restart, status should be creating or running
	status := session["status"].(string)
	if status != "creating" && status != "running" {
		t.Errorf("expected creating or running after restart, got %v", status)
	}
}

func TestSession_TerminateSession(t *testing.T) {
	ts := testutil.NewTestServer(t)
	createContainerApp(t, ts, "term-app")

	body := []byte(`{"app_id":"term-app","user_id":"testuser"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/sessions", ts.AdminToken, body)
	var session map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&session)
	resp.Body.Close()

	sessionID := session["id"].(string)
	waitForRunning(t, ts, sessionID)

	// Terminate (DELETE)
	resp = testutil.AuthDelete(t, ts.URL+"/api/sessions/"+sessionID, ts.AdminToken)
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
}

func TestSession_NonExistentApp(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := []byte(`{"app_id":"nonexistent","user_id":"testuser"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/sessions", ts.AdminToken, body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 400 or 404, got %d", resp.StatusCode)
	}
}

func TestSession_RunnerCreateError(t *testing.T) {
	ts := testutil.NewTestServer(t)
	createContainerApp(t, ts, "err-app")

	ts.Runner.CreateError = fmt.Errorf("simulated runner failure")
	defer func() { ts.Runner.CreateError = nil }()

	body := []byte(`{"app_id":"err-app","user_id":"testuser"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/sessions", ts.AdminToken, body)
	resp.Body.Close()

	// Session creation should still succeed (pod creation happens async)
	// but the session will eventually fail. Check it was created.
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 201 or 400, got %d", resp.StatusCode)
	}
}

func TestSession_ListUserSessions(t *testing.T) {
	ts := testutil.NewTestServer(t)
	createContainerApp(t, ts, "list-app")

	// Create a session
	body := []byte(`{"app_id":"list-app","user_id":"testuser"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/sessions", ts.AdminToken, body)
	resp.Body.Close()

	// List sessions
	resp = testutil.AuthGet(t, ts.URL+"/api/sessions", ts.AdminToken)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var sessions []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&sessions)

	if len(sessions) < 1 {
		t.Error("expected at least 1 session in list")
	}
}

// waitForRunning polls until the session reaches running status.
func waitForRunning(t *testing.T, ts *testutil.TestServer, sessionID string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp := testutil.AuthGet(t, ts.URL+"/api/sessions/"+sessionID, ts.AdminToken)
		var session map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&session)
		resp.Body.Close()

		if session["status"] == "running" {
			return
		}
		if session["status"] == "failed" {
			t.Fatalf("session %s failed", sessionID)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for session %s to reach running", sessionID)
}
