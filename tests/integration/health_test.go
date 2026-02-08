package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/rjsadow/launchpad/tests/integration/testutil"
)

func TestHealth_Liveness(t *testing.T) {
	ts := testutil.NewTestServer(t)

	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %q", body["status"])
	}
}

func TestHealth_Readiness(t *testing.T) {
	ts := testutil.NewTestServer(t)

	resp, err := http.Get(ts.URL + "/readyz")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ready" {
		t.Errorf("expected status ready, got %v", body["status"])
	}

	// Database should be healthy
	db, ok := body["database"].(map[string]interface{})
	if ok && db["status"] != "healthy" {
		t.Errorf("expected database healthy, got %v", db["status"])
	}
}

func TestHealth_LoadStatus(t *testing.T) {
	ts := testutil.NewTestServer(t)

	resp, err := http.Get(ts.URL + "/api/load")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)

	// Should have load_factor field
	if _, ok := body["load_factor"]; !ok {
		t.Error("expected load_factor in response")
	}
}

func TestHealth_NoAuthRequired(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// All health endpoints should work without auth
	endpoints := []string{"/healthz", "/readyz", "/api/load"}
	for _, ep := range endpoints {
		resp, err := http.Get(ts.URL + ep)
		if err != nil {
			t.Fatalf("request to %s failed: %v", ep, err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s: expected 200, got %d", ep, resp.StatusCode)
		}
	}
}
