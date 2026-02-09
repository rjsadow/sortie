package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func jsonReader(s string) io.Reader {
	return strings.NewReader(s)
}

func authGet(url, token string) *http.Response {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	return resp
}

func authPost(url, token string, body []byte) *http.Response {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	return resp
}

func authPut(url, token string, body []byte) *http.Response {
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	return resp
}

func authDelete(url, token string) *http.Response {
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	return resp
}

func waitForSessionRunning(sessionID string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp := authGet(baseURL+"/api/sessions/"+sessionID, adminToken)
		var session map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&session)
		resp.Body.Close()

		status, _ := session["status"].(string)
		if status == "running" {
			return
		}
		if status == "failed" {
			Fail(fmt.Sprintf("session %s failed", sessionID))
		}
		time.Sleep(2 * time.Second)
	}
	Fail(fmt.Sprintf("timeout waiting for session %s to reach running", sessionID))
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
func waitForPodDeletion(sessionID string, timeout time.Duration) {
	podName := "sortie-session-" + sessionID
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)

		resp := authGet(baseURL+"/api/sessions/"+sessionID, adminToken)
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
	GinkgoWriter.Printf("warning: pod %s may not be fully deleted after %v\n", podName, timeout)
}

func createE2EApp(id, launchType string) {
	body := []byte(fmt.Sprintf(`{
		"id": %q,
		"name": "E2E App %s",
		"launch_type": %q,
		"container_image": "nginx:latest"
	}`, id, id, launchType))

	resp := authPost(baseURL+"/api/apps", adminToken, body)
	resp.Body.Close()

	Expect(resp.StatusCode).To(SatisfyAny(
		Equal(http.StatusCreated),
		Equal(http.StatusConflict),
	), "failed to create app %s: %d", id, resp.StatusCode)
}

func deleteE2EApp(id string) {
	resp := authDelete(baseURL+"/api/apps/"+id, adminToken)
	resp.Body.Close()
}

func login(base, username, password string) (string, error) {
	body := fmt.Sprintf(`{"username":%q,"password":%q}`, username, password)
	resp, err := http.Post(base+"/api/auth/login", "application/json", jsonReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login returned %d", resp.StatusCode)
	}

	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.AccessToken, nil
}
