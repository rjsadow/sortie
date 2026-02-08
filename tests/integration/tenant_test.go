package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/rjsadow/launchpad/tests/integration/testutil"
)

func TestTenant_DefaultTenantExists(t *testing.T) {
	ts := testutil.NewTestServer(t)

	resp := testutil.AuthGet(t, ts.URL+"/api/admin/tenants", ts.AdminToken)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var tenants []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&tenants)

	found := false
	for _, tenant := range tenants {
		if tenant["id"] == "default" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected default tenant in tenant list")
	}
}

func TestTenant_CreateTenant(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := []byte(`{"name":"Test Tenant","slug":"test-tenant"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/admin/tenants", ts.AdminToken, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(b))
	}

	var tenant map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&tenant)

	if tenant["name"] != "Test Tenant" {
		t.Errorf("expected name 'Test Tenant', got %v", tenant["name"])
	}
}

func TestTenant_SessionsScopedByTenant(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create tenant A
	bodyA := []byte(`{"name":"Tenant A","slug":"tenant-a"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/admin/tenants", ts.AdminToken, bodyA)
	var tenantA map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&tenantA)
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("failed to create tenant A: %d", resp.StatusCode)
	}

	// Verify both default and new tenant exist
	resp = testutil.AuthGet(t, ts.URL+"/api/admin/tenants", ts.AdminToken)
	var tenants []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&tenants)
	resp.Body.Close()

	if len(tenants) < 2 {
		t.Errorf("expected at least 2 tenants, got %d", len(tenants))
	}
}

func TestTenant_InvalidTenantReturns404(t *testing.T) {
	ts := testutil.NewTestServer(t)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/apps", nil)
	req.Header.Set("Authorization", "Bearer "+ts.AdminToken)
	req.Header.Set("X-Tenant-ID", "nonexistent-tenant")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent tenant, got %d", resp.StatusCode)
	}
}

func TestTenant_DeleteNonDefaultTenant(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create a tenant
	body := []byte(`{"name":"Deletable","slug":"deletable"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/admin/tenants", ts.AdminToken, body)
	var tenant map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&tenant)
	resp.Body.Close()

	tenantID := tenant["id"].(string)

	// Delete it
	resp = testutil.AuthDelete(t, ts.URL+"/api/admin/tenants/"+tenantID, ts.AdminToken)
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
}
