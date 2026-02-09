package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/rjsadow/sortie/tests/integration/testutil"
)

func TestQuota_PerUserQuotaEnforced(t *testing.T) {
	ts := testutil.NewTestServer(t, testutil.WithMaxSessionsPerUser(2))

	createContainerApp(t, ts, "quota-app")

	// Create max sessions
	for i := 0; i < 2; i++ {
		body := []byte(`{"app_id":"quota-app","user_id":"quotauser"}`)
		resp := testutil.AuthPost(t, ts.URL+"/api/sessions", ts.AdminToken, body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("session %d creation failed: %d", i, resp.StatusCode)
		}
	}

	// Next one should fail with 429
	body := []byte(`{"app_id":"quota-app","user_id":"quotauser"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/sessions", ts.AdminToken, body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", resp.StatusCode)
	}
}

func TestQuota_GlobalQuotaEnforced(t *testing.T) {
	ts := testutil.NewTestServer(t, testutil.WithMaxGlobalSessions(2))

	createContainerApp(t, ts, "gquota-app")

	// Create max sessions with different users
	for i := 0; i < 2; i++ {
		body := []byte(fmt.Sprintf(`{"app_id":"gquota-app","user_id":"user%d"}`, i))
		resp := testutil.AuthPost(t, ts.URL+"/api/sessions", ts.AdminToken, body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("session %d creation failed: %d", i, resp.StatusCode)
		}
	}

	// Next one should fail
	body := []byte(`{"app_id":"gquota-app","user_id":"user99"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/sessions", ts.AdminToken, body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", resp.StatusCode)
	}
}

func TestQuota_StatusEndpoint(t *testing.T) {
	ts := testutil.NewTestServer(t)

	resp := testutil.AuthGet(t, ts.URL+"/api/quotas", ts.AdminToken)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var status map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&status)

	if _, ok := status["max_sessions_per_user"]; !ok {
		t.Error("expected max_sessions_per_user in quota status")
	}
}

func TestQuota_StoppingSessionFreesQuota(t *testing.T) {
	ts := testutil.NewTestServer(t, testutil.WithMaxSessionsPerUser(1))

	createContainerApp(t, ts, "free-quota-app")

	// Create a session
	body := []byte(`{"app_id":"free-quota-app","user_id":"freeuser"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/sessions", ts.AdminToken, body)
	var session map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&session)
	resp.Body.Close()

	sessionID := session["id"].(string)
	waitForRunning(t, ts, sessionID)

	// Stop the session
	resp = testutil.AuthPost(t, ts.URL+"/api/sessions/"+sessionID+"/stop", ts.AdminToken, nil)
	resp.Body.Close()

	// Should be able to create another session now
	resp = testutil.AuthPost(t, ts.URL+"/api/sessions", ts.AdminToken, body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201 after freeing quota, got %d", resp.StatusCode)
	}
}
