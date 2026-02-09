package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/rjsadow/sortie/tests/integration/testutil"
)

func TestAudit_LoginCreatesEntry(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// The admin login during server setup already created an audit entry.
	// Let's check it.
	resp := testutil.AuthGet(t, ts.URL+"/api/audit", ts.AdminToken)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var page struct {
		Logs []struct {
			Action string `json:"action"`
			User   string `json:"user"`
		} `json:"logs"`
	}
	json.NewDecoder(resp.Body).Decode(&page)

	found := false
	for _, log := range page.Logs {
		if log.Action == "LOGIN" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected LOGIN audit entry")
	}
}

func TestAudit_AppCreationLogged(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create an app
	body := []byte(`{"id":"audit-app","name":"Audit App","url":"https://example.com","launch_type":"url"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken, body)
	resp.Body.Close()

	// Check audit log
	resp = testutil.AuthGet(t, ts.URL+"/api/audit?action=CREATE_APP", ts.AdminToken)
	defer resp.Body.Close()

	var page struct {
		Logs []struct {
			Action string `json:"action"`
		} `json:"logs"`
	}
	json.NewDecoder(resp.Body).Decode(&page)

	found := false
	for _, log := range page.Logs {
		if log.Action == "CREATE_APP" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected CREATE_APP audit entry")
	}
}

func TestAudit_SessionCreationLogged(t *testing.T) {
	ts := testutil.NewTestServer(t)
	createContainerApp(t, ts, "audit-sess-app")

	// Create session
	body := []byte(`{"app_id":"audit-sess-app","user_id":"audituser"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/sessions", ts.AdminToken, body)
	resp.Body.Close()

	// Check audit log
	resp = testutil.AuthGet(t, ts.URL+"/api/audit?action=CREATE_SESSION", ts.AdminToken)
	defer resp.Body.Close()

	var page struct {
		Logs []struct {
			Action string `json:"action"`
		} `json:"logs"`
	}
	json.NewDecoder(resp.Body).Decode(&page)

	found := false
	for _, log := range page.Logs {
		if log.Action == "CREATE_SESSION" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected CREATE_SESSION audit entry")
	}
}

func TestAudit_ExportJSON(t *testing.T) {
	ts := testutil.NewTestServer(t)

	resp := testutil.AuthGet(t, ts.URL+"/api/audit/export?format=json", ts.AdminToken)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("expected JSON content type, got %s", contentType)
	}
}

func TestAudit_ExportCSV(t *testing.T) {
	ts := testutil.NewTestServer(t)

	resp := testutil.AuthGet(t, ts.URL+"/api/audit/export?format=csv", ts.AdminToken)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/csv") {
		t.Errorf("expected CSV content type, got %s", contentType)
	}
}

func TestAudit_Filters(t *testing.T) {
	ts := testutil.NewTestServer(t)

	resp := testutil.AuthGet(t, ts.URL+"/api/audit/filters", ts.AdminToken)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var filters map[string][]string
	json.NewDecoder(resp.Body).Decode(&filters)

	if _, ok := filters["actions"]; !ok {
		t.Error("expected actions in filters")
	}
	if _, ok := filters["users"]; !ok {
		t.Error("expected users in filters")
	}
}
