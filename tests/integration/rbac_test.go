package integration

import (
	"net/http"
	"testing"

	"github.com/rjsadow/sortie/tests/integration/testutil"
)

func TestRBAC_UserCanListApps(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create a regular user
	testutil.CreateUser(t, ts.URL, ts.AdminToken, "regularuser", "password123", []string{"user"})
	userToken := testutil.LoginAs(t, ts.URL, "regularuser", "password123")

	resp := testutil.AuthGet(t, ts.URL+"/api/apps", userToken)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("user should be able to list apps, got %d", resp.StatusCode)
	}
}

func TestRBAC_UserCannotCreateApps(t *testing.T) {
	ts := testutil.NewTestServer(t)

	testutil.CreateUser(t, ts.URL, ts.AdminToken, "regularuser2", "password123", []string{"user"})
	userToken := testutil.LoginAs(t, ts.URL, "regularuser2", "password123")

	body := []byte(`{"id":"test","name":"Test","url":"https://example.com","launch_type":"url"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/apps", userToken, body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("regular user should not create apps, got %d", resp.StatusCode)
	}
}

func TestRBAC_AppAuthorCanCreateApps(t *testing.T) {
	ts := testutil.NewTestServer(t)

	testutil.CreateUser(t, ts.URL, ts.AdminToken, "author", "password123", []string{"app-author"})
	authorToken := testutil.LoginAs(t, ts.URL, "author", "password123")

	body := []byte(`{"id":"authored","name":"Authored App","url":"https://example.com","launch_type":"url"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/apps", authorToken, body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("app-author should create apps, got %d", resp.StatusCode)
	}
}

func TestRBAC_AdminAccessAdminRoutes(t *testing.T) {
	ts := testutil.NewTestServer(t)

	endpoints := []string{
		"/api/admin/settings",
		"/api/admin/users",
		"/api/admin/sessions",
		"/api/admin/health",
		"/api/admin/tenants",
	}

	for _, ep := range endpoints {
		resp := testutil.AuthGet(t, ts.URL+ep, ts.AdminToken)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("admin should access %s, got %d", ep, resp.StatusCode)
		}
	}
}

func TestRBAC_UserCannotAccessAdminRoutes(t *testing.T) {
	ts := testutil.NewTestServer(t)

	testutil.CreateUser(t, ts.URL, ts.AdminToken, "regularuser3", "password123", []string{"user"})
	userToken := testutil.LoginAs(t, ts.URL, "regularuser3", "password123")

	endpoints := []string{
		"/api/admin/settings",
		"/api/admin/users",
		"/api/admin/sessions",
		"/api/admin/health",
		"/api/admin/tenants",
	}

	for _, ep := range endpoints {
		resp := testutil.AuthGet(t, ts.URL+ep, userToken)
		resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("regular user should not access %s, got %d", ep, resp.StatusCode)
		}
	}
}

func TestRBAC_NonAdminCannotManageTemplates(t *testing.T) {
	ts := testutil.NewTestServer(t)

	testutil.CreateUser(t, ts.URL, ts.AdminToken, "regularuser4", "password123", []string{"user"})
	userToken := testutil.LoginAs(t, ts.URL, "regularuser4", "password123")

	body := []byte(`{"template_id":"t1","name":"Test","template_category":"test","category":"test"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/admin/templates", userToken, body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("non-admin should not manage templates, got %d", resp.StatusCode)
	}
}
