package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/rjsadow/sortie/internal/plugins"
	"github.com/rjsadow/sortie/tests/integration/testutil"
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

func TestHealth_StoragePluginRegistered(t *testing.T) {
	// NewTestServer calls storage.SetDB(database) which wires the shared DB
	// into the storage plugin factory. Verify the factory is registered and
	// produces a healthy plugin in the integration test environment.
	_ = testutil.NewTestServer(t)

	storagePlugins := plugins.Global().ListPluginsByType(plugins.PluginTypeStorage)

	// The sqlite and memory factories should both be registered via init()
	found := false
	for _, p := range storagePlugins {
		if p.Name == "sqlite" && p.Type == plugins.PluginTypeStorage {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'sqlite' storage plugin to be registered in global registry")
	}

	// Verify the factory produces a plugin with correct metadata.
	// ListPluginsByType calls factory() internally, exercising the SetDB wiring.
	for _, p := range storagePlugins {
		if p.Name == "sqlite" {
			if p.Version == "" {
				t.Error("expected sqlite plugin to have a version")
			}
			if p.Description == "" {
				t.Error("expected sqlite plugin to have a description")
			}
		}
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
