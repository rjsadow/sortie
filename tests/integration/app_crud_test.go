package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/rjsadow/sortie/tests/integration/testutil"
)

func TestAppCRUD_ListEmpty(t *testing.T) {
	ts := testutil.NewTestServer(t)

	resp := testutil.AuthGet(t, ts.URL+"/api/apps", ts.AdminToken)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var apps []interface{}
	json.NewDecoder(resp.Body).Decode(&apps)

	if len(apps) != 0 {
		t.Errorf("expected empty list, got %d apps", len(apps))
	}
}

func TestAppCRUD_CreateURLApp(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := []byte(`{"id":"google","name":"Google","url":"https://google.com","launch_type":"url"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(b))
	}

	var app map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&app)

	if app["id"] != "google" {
		t.Errorf("expected id google, got %v", app["id"])
	}
}

func TestAppCRUD_CreateContainerApp(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := []byte(`{"id":"vscode","name":"VS Code","launch_type":"container","container_image":"codercom/code-server:latest"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(b))
	}

	var app map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&app)

	if app["container_image"] != "codercom/code-server:latest" {
		t.Errorf("expected container_image, got %v", app["container_image"])
	}
}

func TestAppCRUD_CreateWebProxyApp(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := []byte(`{"id":"jupyter","name":"Jupyter","launch_type":"web_proxy","container_image":"jupyter/base-notebook:latest"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(b))
	}
}

func TestAppCRUD_CreateWindowsApp(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := []byte(`{"id":"windesktop","name":"Windows Desktop","launch_type":"container","os_type":"windows","container_image":"mcr.microsoft.com/windows:latest"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(b))
	}

	var app map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&app)

	if app["os_type"] != "windows" {
		t.Errorf("expected os_type windows, got %v", app["os_type"])
	}
}

func TestAppCRUD_MissingRequiredFields(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Missing name
	body := []byte(`{"id":"test","url":"https://example.com"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken, body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestAppCRUD_DuplicateAppID(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := []byte(`{"id":"dup","name":"First","url":"https://example.com","launch_type":"url"}`)

	// Create first
	resp := testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken, body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("first create failed: %d", resp.StatusCode)
	}

	// Try duplicate
	resp = testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken, body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Errorf("expected 409, got %d", resp.StatusCode)
	}
}

func TestAppCRUD_GetByID(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create
	body := []byte(`{"id":"getme","name":"Get Me","url":"https://example.com","launch_type":"url"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken, body)
	resp.Body.Close()

	// Get
	resp = testutil.AuthGet(t, ts.URL+"/api/apps/getme", ts.AdminToken)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var app map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&app)

	if app["name"] != "Get Me" {
		t.Errorf("expected name 'Get Me', got %v", app["name"])
	}
}

func TestAppCRUD_GetNonExistent(t *testing.T) {
	ts := testutil.NewTestServer(t)

	resp := testutil.AuthGet(t, ts.URL+"/api/apps/nonexistent", ts.AdminToken)
	resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestAppCRUD_UpdateApp(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create
	createBody := []byte(`{"id":"updateme","name":"Original","url":"https://example.com","launch_type":"url"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken, createBody)
	resp.Body.Close()

	// Update
	updateBody := []byte(`{"name":"Updated","url":"https://updated.com","launch_type":"url"}`)
	resp = testutil.AuthPut(t, ts.URL+"/api/apps/updateme", ts.AdminToken, updateBody)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}

	var app map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&app)

	if app["name"] != "Updated" {
		t.Errorf("expected name 'Updated', got %v", app["name"])
	}
}

func TestAppCRUD_DeleteApp(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create
	body := []byte(`{"id":"deleteme","name":"Delete Me","url":"https://example.com","launch_type":"url"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken, body)
	resp.Body.Close()

	// Delete
	resp = testutil.AuthDelete(t, ts.URL+"/api/apps/deleteme", ts.AdminToken)
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}

	// Verify deleted
	resp = testutil.AuthGet(t, ts.URL+"/api/apps/deleteme", ts.AdminToken)
	resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", resp.StatusCode)
	}
}
