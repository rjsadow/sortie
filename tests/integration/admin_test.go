package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/rjsadow/sortie/tests/integration/testutil"
)

func TestAdmin_GetSettings(t *testing.T) {
	ts := testutil.NewTestServer(t)

	resp := testutil.AuthGet(t, ts.URL+"/api/admin/settings", ts.AdminToken)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var settings map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&settings)

	if _, ok := settings["allow_registration"]; !ok {
		t.Error("expected allow_registration in settings")
	}
}

func TestAdmin_UpdateSettings(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := []byte(`{"test_setting":"test_value"}`)
	resp := testutil.AuthPut(t, ts.URL+"/api/admin/settings", ts.AdminToken, body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
}

func TestAdmin_ListUsers(t *testing.T) {
	ts := testutil.NewTestServer(t)

	resp := testutil.AuthGet(t, ts.URL+"/api/admin/users", ts.AdminToken)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var users []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&users)

	// Should include at least the admin user
	found := false
	for _, u := range users {
		if u["username"] == "admin" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected admin user in user list")
	}
}

func TestAdmin_CreateUser(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := []byte(`{"username":"newadminuser","password":"password123","email":"new@test.local","roles":["user"]}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/admin/users", ts.AdminToken, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(b))
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)

	if result["id"] == "" {
		t.Error("expected user ID in response")
	}
}

func TestAdmin_DeleteUser(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create user first
	userID := testutil.CreateUser(t, ts.URL, ts.AdminToken, "deleteuser", "password123", []string{"user"})

	// Delete user
	resp := testutil.AuthDelete(t, ts.URL+"/api/admin/users/"+userID, ts.AdminToken)
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
}

func TestAdmin_ListAllSessions(t *testing.T) {
	ts := testutil.NewTestServer(t)

	resp := testutil.AuthGet(t, ts.URL+"/api/admin/sessions", ts.AdminToken)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestAdmin_HealthCheck(t *testing.T) {
	ts := testutil.NewTestServer(t)

	resp := testutil.AuthGet(t, ts.URL+"/api/admin/health", ts.AdminToken)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var health map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&health)

	if health["status"] != "healthy" {
		t.Errorf("expected healthy status, got %v", health["status"])
	}
}

func TestAdmin_AutoRecordSetting(t *testing.T) {
	ts := testutil.NewTestServer(t)

	t.Run("default recording_auto_record is absent or false", func(t *testing.T) {
		resp := testutil.AuthGet(t, ts.URL+"/api/admin/settings", ts.AdminToken)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var settings map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&settings)

		// Default should not be "true"
		val, ok := settings["recording_auto_record"]
		if ok && val == "true" {
			t.Errorf("expected recording_auto_record to not be 'true' by default, got %v", val)
		}
	})

	t.Run("set recording_auto_record to true", func(t *testing.T) {
		body := []byte(`{"recording_auto_record":"true"}`)
		resp := testutil.AuthPut(t, ts.URL+"/api/admin/settings", ts.AdminToken, body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", resp.StatusCode)
		}

		// Verify it persisted
		resp = testutil.AuthGet(t, ts.URL+"/api/admin/settings", ts.AdminToken)
		defer resp.Body.Close()

		var settings map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&settings)

		if settings["recording_auto_record"] != "true" {
			t.Errorf("expected recording_auto_record = 'true', got %v", settings["recording_auto_record"])
		}
	})
}

func TestAdmin_SessionRecordingPolicy(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create a container app for session tests
	appBody := []byte(`{"id":"rec-pol-app","name":"Rec Policy App","launch_type":"container","container_image":"nginx:latest"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken, appBody)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("failed to create app: status %d", resp.StatusCode)
	}

	t.Run("session has no recording_policy by default", func(t *testing.T) {
		body := []byte(`{"app_id":"rec-pol-app"}`)
		resp := testutil.AuthPost(t, ts.URL+"/api/sessions", ts.AdminToken, body)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(b))
		}

		var session map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&session)

		// recording_policy should be empty/absent (omitempty)
		if val, ok := session["recording_policy"]; ok && val != "" {
			t.Errorf("expected recording_policy to be absent or empty, got %v", val)
		}
	})

	t.Run("session has auto recording_policy when setting enabled", func(t *testing.T) {
		// Enable auto-record
		settingsBody := []byte(`{"recording_auto_record":"true"}`)
		resp := testutil.AuthPut(t, ts.URL+"/api/admin/settings", ts.AdminToken, settingsBody)
		resp.Body.Close()

		// Create a new session
		body := []byte(`{"app_id":"rec-pol-app"}`)
		resp = testutil.AuthPost(t, ts.URL+"/api/sessions", ts.AdminToken, body)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(b))
		}

		var session map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&session)

		if session["recording_policy"] != "auto" {
			t.Errorf("expected recording_policy = 'auto', got %v", session["recording_policy"])
		}
	})

	t.Run("session list includes recording_policy", func(t *testing.T) {
		resp := testutil.AuthGet(t, ts.URL+"/api/sessions", ts.AdminToken)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var sessions []map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&sessions)

		if len(sessions) == 0 {
			t.Fatal("expected at least one session")
		}

		// All sessions should have recording_policy = "auto" since we enabled it above
		for _, s := range sessions {
			if s["recording_policy"] != "auto" {
				t.Errorf("session %v: expected recording_policy = 'auto', got %v", s["id"], s["recording_policy"])
			}
		}
	})

	t.Run("disabling auto-record removes policy from sessions", func(t *testing.T) {
		// Disable auto-record
		settingsBody := []byte(`{"recording_auto_record":"false"}`)
		resp := testutil.AuthPut(t, ts.URL+"/api/admin/settings", ts.AdminToken, settingsBody)
		resp.Body.Close()

		// List sessions again
		resp = testutil.AuthGet(t, ts.URL+"/api/sessions", ts.AdminToken)
		defer resp.Body.Close()

		var sessions []map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&sessions)

		for _, s := range sessions {
			if val, ok := s["recording_policy"]; ok && val != "" {
				t.Errorf("session %v: expected no recording_policy, got %v", s["id"], val)
			}
		}
	})
}

func TestAdmin_DiagnosticsBundle(t *testing.T) {
	ts := testutil.NewTestServer(t)

	resp := testutil.AuthGet(t, ts.URL+"/api/admin/diagnostics", ts.AdminToken)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}
}
