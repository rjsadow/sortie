package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func jsonReader(s string) io.Reader {
	return strings.NewReader(s)
}

func authGet(t *testing.T, url, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func authPost(t *testing.T, url, token string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func authPut(t *testing.T, url, token string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func authDelete(t *testing.T, url, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func waitForSessionRunning(t *testing.T, sessionID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp := authGet(t, baseURL+"/api/sessions/"+sessionID, adminToken)
		var session map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&session)
		resp.Body.Close()

		status, _ := session["status"].(string)
		if status == "running" {
			return
		}
		if status == "failed" {
			t.Fatalf("session %s failed", sessionID)
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("timeout waiting for session %s to reach running", sessionID)
}

func createE2EApp(t *testing.T, id, launchType string) {
	t.Helper()
	body := []byte(fmt.Sprintf(`{
		"id": %q,
		"name": "E2E App %s",
		"launch_type": %q,
		"container_image": "nginx:latest"
	}`, id, id, launchType))

	resp := authPost(t, baseURL+"/api/apps", adminToken, body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
		t.Fatalf("failed to create app %s: %d", id, resp.StatusCode)
	}
}

func deleteE2EApp(t *testing.T, id string) {
	t.Helper()
	resp := authDelete(t, baseURL+"/api/apps/"+id, adminToken)
	resp.Body.Close()
}
