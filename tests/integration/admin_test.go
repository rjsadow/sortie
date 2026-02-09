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

func TestAdmin_DiagnosticsBundle(t *testing.T) {
	ts := testutil.NewTestServer(t)

	resp := testutil.AuthGet(t, ts.URL+"/api/admin/diagnostics", ts.AdminToken)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}
}
