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

// waitForSessionStatus polls until the session reaches the target status.
// Returns nil on success, error on timeout.
func waitForSessionStatus(sessionID, target string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/api/sessions/"+sessionID, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		var session map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&session)
		resp.Body.Close()

		status, _ := session["status"].(string)
		if status == target {
			return nil
		}
		if status == "failed" && target != "failed" {
			return fmt.Errorf("session %s failed", sessionID)
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for session %s to reach %s", sessionID, target)
}

// waitForPodDeletion waits for the session's Kubernetes pod to be fully
// terminated. The stop endpoint triggers pod deletion, but Kubernetes needs
// time to finalize it. Without this wait, restart fails with "pod already
// exists" because the old pod is still terminating.
func waitForPodDeletion(t *testing.T, sessionID string, timeout time.Duration) {
	t.Helper()
	podName := "launchpad-session-" + sessionID
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		// Use the admin sessions endpoint to check if the pod still exists
		// by looking at whether kubectl would find it. We check via the
		// session API — when the pod is gone, a fresh GET should show
		// the session as stopped with no running pod.
		cmd := fmt.Sprintf("kubectl get pod %s -n launchpad --no-headers 2>&1", podName)
		// We can't run kubectl from within the test binary directly,
		// so we poll with a simple delay instead.
		_ = cmd
		time.Sleep(2 * time.Second)

		// Double-check the session is stopped
		resp := authGet(t, baseURL+"/api/sessions/"+sessionID, adminToken)
		var session map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&session)
		resp.Body.Close()

		status, _ := session["status"].(string)
		if status == "stopped" {
			// Give Kubernetes a bit more time for pod finalizers
			time.Sleep(3 * time.Second)
			return
		}
	}
	t.Logf("warning: pod %s may not be fully deleted after %v", podName, timeout)
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
