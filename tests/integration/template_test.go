package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/rjsadow/sortie/tests/integration/testutil"
)

func TestTemplate_ListPublicNoAuth(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Public endpoint: no auth required
	resp, err := http.Get(ts.URL + "/api/templates")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var templates []interface{}
	json.NewDecoder(resp.Body).Decode(&templates)

	// Should return an array (possibly with seeded templates)
	if templates == nil {
		t.Error("expected non-nil templates array")
	}
}

func TestTemplate_AdminCreateTemplate(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := []byte(`{
		"template_id": "test-tmpl",
		"name": "Test Template",
		"template_category": "development",
		"category": "IDE",
		"description": "A test template",
		"launch_type": "container",
		"container_image": "nginx:latest"
	}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/admin/templates", ts.AdminToken, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(b))
	}

	var tmpl map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&tmpl)

	if tmpl["template_id"] != "test-tmpl" {
		t.Errorf("expected template_id test-tmpl, got %v", tmpl["template_id"])
	}
}

func TestTemplate_AdminUpdateTemplate(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create first
	createBody := []byte(`{
		"template_id": "update-tmpl",
		"name": "Original",
		"template_category": "tools",
		"category": "Utility"
	}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/admin/templates", ts.AdminToken, createBody)
	resp.Body.Close()

	// Update
	updateBody := []byte(`{"name": "Updated Template", "template_category": "tools", "category": "Utility"}`)
	resp = testutil.AuthPut(t, ts.URL+"/api/admin/templates/update-tmpl", ts.AdminToken, updateBody)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}

	var tmpl map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&tmpl)

	if tmpl["name"] != "Updated Template" {
		t.Errorf("expected Updated Template, got %v", tmpl["name"])
	}
}

func TestTemplate_AdminDeleteTemplate(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create
	body := []byte(`{
		"template_id": "delete-tmpl",
		"name": "Delete Me",
		"template_category": "tools",
		"category": "Utility"
	}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/admin/templates", ts.AdminToken, body)
	resp.Body.Close()

	// Delete
	resp = testutil.AuthDelete(t, ts.URL+"/api/admin/templates/delete-tmpl", ts.AdminToken)
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
}

func TestTemplate_NonAdminCannotManage(t *testing.T) {
	ts := testutil.NewTestServer(t)

	testutil.CreateUser(t, ts.URL, ts.AdminToken, "tmpluser", "password123", []string{"user"})
	userToken := testutil.LoginAs(t, ts.URL, "tmpluser", "password123")

	body := []byte(`{
		"template_id": "blocked",
		"name": "Blocked",
		"template_category": "test",
		"category": "test"
	}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/admin/templates", userToken, body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}
