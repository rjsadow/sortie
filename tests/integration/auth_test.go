package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/rjsadow/sortie/tests/integration/testutil"
)

func TestAuth_LoginValidCredentials(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := `{"username":"admin","password":"admin123"}`
	resp, err := http.Post(ts.URL+"/api/auth/login", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if result["access_token"] == nil || result["access_token"] == "" {
		t.Error("expected access_token in response")
	}
	if result["refresh_token"] == nil || result["refresh_token"] == "" {
		t.Error("expected refresh_token in response")
	}
}

func TestAuth_LoginInvalidPassword(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := `{"username":"admin","password":"wrongpassword"}`
	resp, err := http.Post(ts.URL+"/api/auth/login", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAuth_LoginMissingFields(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := `{"username":"admin"}`
	resp, err := http.Post(ts.URL+"/api/auth/login", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestAuth_ProtectedRouteWithoutToken(t *testing.T) {
	ts := testutil.NewTestServer(t)

	resp, err := http.Get(ts.URL + "/api/apps")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAuth_ProtectedRouteWithValidToken(t *testing.T) {
	ts := testutil.NewTestServer(t)

	resp := testutil.AuthGet(t, ts.URL+"/api/apps", ts.AdminToken)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestAuth_TokenRefreshValid(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// First login to get refresh token
	loginBody := `{"username":"admin","password":"admin123"}`
	loginResp, err := http.Post(ts.URL+"/api/auth/login", "application/json", bytes.NewBufferString(loginBody))
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	defer loginResp.Body.Close()

	var loginResult struct {
		RefreshToken string `json:"refresh_token"`
	}
	json.NewDecoder(loginResp.Body).Decode(&loginResult)

	// Now refresh
	refreshBody, _ := json.Marshal(map[string]string{"refresh_token": loginResult.RefreshToken})
	resp, err := http.Post(ts.URL+"/api/auth/refresh", "application/json", bytes.NewReader(refreshBody))
	if err != nil {
		t.Fatalf("refresh failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}

	var refreshResult map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&refreshResult)

	if refreshResult["access_token"] == nil || refreshResult["access_token"] == "" {
		t.Error("expected new access_token in refresh response")
	}
}

func TestAuth_TokenRefreshInvalid(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := `{"refresh_token":"invalid-token"}`
	resp, err := http.Post(ts.URL+"/api/auth/refresh", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAuth_Logout(t *testing.T) {
	ts := testutil.NewTestServer(t)

	resp, err := http.Post(ts.URL+"/api/auth/logout", "application/json", nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}

	// Check that cookie is cleared
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "sortie_access_token" {
			if cookie.MaxAge >= 0 {
				t.Error("expected cookie MaxAge < 0 (cleared)")
			}
		}
	}
}

func TestAuth_GetCurrentUser(t *testing.T) {
	ts := testutil.NewTestServer(t)

	resp := testutil.AuthGet(t, ts.URL+"/api/auth/me", ts.AdminToken)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}

	var user map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&user)

	if user["username"] != "admin" {
		t.Errorf("expected username admin, got %v", user["username"])
	}
}

func TestAuth_RegisterEnabled(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := `{"username":"newuser","password":"password123","email":"new@test.local"}`
	resp, err := http.Post(ts.URL+"/api/auth/register", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(b))
	}
}

func TestAuth_RegisterDisabled(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Disable registration via settings
	ts.Config.AllowRegistration = false

	body := `{"username":"newuser2","password":"password123","email":"new2@test.local"}`
	resp, err := http.Post(ts.URL+"/api/auth/register", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}
