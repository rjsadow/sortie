package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

const (
	defaultBaseURL = "http://localhost:18080"
	adminUsername  = "admin"
	adminPassword  = "admin123"
)

var (
	baseURL    string
	adminToken string
)

func TestMain(m *testing.M) {
	baseURL = os.Getenv("E2E_BASE_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	// Wait for readyz
	if err := waitForReady(baseURL, 60*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "Launchpad not ready: %v\n", err)
		os.Exit(1)
	}

	// Login as admin
	var err error
	adminToken, err = login(baseURL, adminUsername, adminPassword)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to login: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func waitForReady(base string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/readyz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for %s/readyz", base)
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
