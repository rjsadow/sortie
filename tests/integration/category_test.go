package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/rjsadow/sortie/internal/db"
	"github.com/rjsadow/sortie/tests/integration/testutil"
)

// --- Category API CRUD ---

func TestCategory_AdminCanCreateCategory(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := []byte(`{"id":"cat-dev","name":"Development","description":"Dev tools"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/categories", ts.AdminToken, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(b))
	}

	var cat db.Category
	json.NewDecoder(resp.Body).Decode(&cat)
	if cat.Name != "Development" {
		t.Errorf("expected name Development, got %s", cat.Name)
	}
}

func TestCategory_RegularUserCannotCreate(t *testing.T) {
	ts := testutil.NewTestServer(t)

	testutil.CreateUser(t, ts.URL, ts.AdminToken, "user1", "pass123", []string{"user"})
	userToken := testutil.LoginAs(t, ts.URL, "user1", "pass123")

	body := []byte(`{"id":"cat-test","name":"Test"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/categories", userToken, body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

func TestCategory_GetByID(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create
	body := []byte(`{"id":"cat-get","name":"GetMe"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/categories", ts.AdminToken, body)
	resp.Body.Close()

	// Get
	resp = testutil.AuthGet(t, ts.URL+"/api/categories/cat-get", ts.AdminToken)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var cat db.Category
	json.NewDecoder(resp.Body).Decode(&cat)
	if cat.Name != "GetMe" {
		t.Errorf("expected name GetMe, got %s", cat.Name)
	}
}

func TestCategory_ListCategories(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create two categories
	body1 := []byte(`{"id":"cat-a","name":"Alpha"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/categories", ts.AdminToken, body1)
	resp.Body.Close()

	body2 := []byte(`{"id":"cat-b","name":"Beta"}`)
	resp = testutil.AuthPost(t, ts.URL+"/api/categories", ts.AdminToken, body2)
	resp.Body.Close()

	// List as admin (should see both)
	resp = testutil.AuthGet(t, ts.URL+"/api/categories", ts.AdminToken)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var cats []db.Category
	json.NewDecoder(resp.Body).Decode(&cats)
	if len(cats) < 2 {
		t.Errorf("expected at least 2 categories, got %d", len(cats))
	}
}

func TestCategory_UpdateCategory(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create
	body := []byte(`{"id":"cat-upd","name":"Original"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/categories", ts.AdminToken, body)
	resp.Body.Close()

	// Update
	updateBody := []byte(`{"name":"Updated","description":"New desc"}`)
	resp = testutil.AuthPut(t, ts.URL+"/api/categories/cat-upd", ts.AdminToken, updateBody)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}

	var cat db.Category
	json.NewDecoder(resp.Body).Decode(&cat)
	if cat.Name != "Updated" {
		t.Errorf("expected name Updated, got %s", cat.Name)
	}
}

func TestCategory_DeleteCategory(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := []byte(`{"id":"cat-del","name":"DeleteMe"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/categories", ts.AdminToken, body)
	resp.Body.Close()

	resp = testutil.AuthDelete(t, ts.URL+"/api/categories/cat-del", ts.AdminToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}

	// Verify deleted
	resp = testutil.AuthGet(t, ts.URL+"/api/categories/cat-del", ts.AdminToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", resp.StatusCode)
	}
}

func TestCategory_RegularUserCannotDelete(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := []byte(`{"id":"cat-nodel","name":"NoDel"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/categories", ts.AdminToken, body)
	resp.Body.Close()

	testutil.CreateUser(t, ts.URL, ts.AdminToken, "user2", "pass123", []string{"user"})
	userToken := testutil.LoginAs(t, ts.URL, "user2", "pass123")

	resp = testutil.AuthDelete(t, ts.URL+"/api/categories/cat-nodel", userToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

// --- Category Admin Management ---

func TestCategory_AdminCanManageCategoryAdmins(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create category
	body := []byte(`{"id":"cat-adm","name":"AdminCat"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/categories", ts.AdminToken, body)
	resp.Body.Close()

	// Create user
	userID := testutil.CreateUser(t, ts.URL, ts.AdminToken, "catadmin1", "pass123", []string{"user"})

	// Add as category admin
	addBody := []byte(fmt.Sprintf(`{"user_id":%q}`, userID))
	resp = testutil.AuthPost(t, ts.URL+"/api/categories/cat-adm/admins", ts.AdminToken, addBody)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(b))
	}

	// List admins
	resp = testutil.AuthGet(t, ts.URL+"/api/categories/cat-adm/admins", ts.AdminToken)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var admins []string
	json.NewDecoder(resp.Body).Decode(&admins)
	if len(admins) != 1 || admins[0] != userID {
		t.Errorf("expected [%s], got %v", userID, admins)
	}
}

func TestCategory_CategoryAdminCanManageAdmins(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create category
	body := []byte(`{"id":"cat-ca","name":"CatAdminManage"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/categories", ts.AdminToken, body)
	resp.Body.Close()

	// Create two users
	user1ID := testutil.CreateUser(t, ts.URL, ts.AdminToken, "cadmin1", "pass123", []string{"user"})
	user2ID := testutil.CreateUser(t, ts.URL, ts.AdminToken, "cadmin2", "pass123", []string{"user"})

	// Make user1 a category admin
	addBody := []byte(fmt.Sprintf(`{"user_id":%q}`, user1ID))
	resp = testutil.AuthPost(t, ts.URL+"/api/categories/cat-ca/admins", ts.AdminToken, addBody)
	resp.Body.Close()

	// user1 adds user2 as category admin
	user1Token := testutil.LoginAs(t, ts.URL, "cadmin1", "pass123")
	addBody2 := []byte(fmt.Sprintf(`{"user_id":%q}`, user2ID))
	resp = testutil.AuthPost(t, ts.URL+"/api/categories/cat-ca/admins", user1Token, addBody2)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("category admin add failed: %d: %s", resp.StatusCode, string(b))
	}
}

func TestCategory_RegularUserCannotManageAdmins(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := []byte(`{"id":"cat-noadm","name":"NoAdmManage"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/categories", ts.AdminToken, body)
	resp.Body.Close()

	userID := testutil.CreateUser(t, ts.URL, ts.AdminToken, "nonadmin1", "pass123", []string{"user"})
	userToken := testutil.LoginAs(t, ts.URL, "nonadmin1", "pass123")

	addBody := []byte(fmt.Sprintf(`{"user_id":%q}`, userID))
	resp = testutil.AuthPost(t, ts.URL+"/api/categories/cat-noadm/admins", userToken, addBody)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

// --- Approved User Management ---

func TestCategory_ManageApprovedUsers(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create approved category
	body := []byte(`{"id":"cat-apr","name":"ApprovedCat"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/categories", ts.AdminToken, body)
	resp.Body.Close()

	// Create user
	userID := testutil.CreateUser(t, ts.URL, ts.AdminToken, "approveduser1", "pass123", []string{"user"})

	// Add as approved user
	addBody := []byte(fmt.Sprintf(`{"user_id":%q}`, userID))
	resp = testutil.AuthPost(t, ts.URL+"/api/categories/cat-apr/approved-users", ts.AdminToken, addBody)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(b))
	}

	// List approved users
	resp = testutil.AuthGet(t, ts.URL+"/api/categories/cat-apr/approved-users", ts.AdminToken)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var users []string
	json.NewDecoder(resp.Body).Decode(&users)
	if len(users) != 1 || users[0] != userID {
		t.Errorf("expected [%s], got %v", userID, users)
	}
}

func TestCategory_RemoveApprovedUser(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := []byte(`{"id":"cat-rapr","name":"RemoveApproved"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/categories", ts.AdminToken, body)
	resp.Body.Close()

	userID := testutil.CreateUser(t, ts.URL, ts.AdminToken, "removeme1", "pass123", []string{"user"})

	// Add
	addBody := []byte(fmt.Sprintf(`{"user_id":%q}`, userID))
	resp = testutil.AuthPost(t, ts.URL+"/api/categories/cat-rapr/approved-users", ts.AdminToken, addBody)
	resp.Body.Close()

	// Remove
	resp = testutil.AuthDelete(t, ts.URL+"/api/categories/cat-rapr/approved-users/"+userID, ts.AdminToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}

	// Verify removed
	resp = testutil.AuthGet(t, ts.URL+"/api/categories/cat-rapr/approved-users", ts.AdminToken)
	defer resp.Body.Close()

	var users []string
	json.NewDecoder(resp.Body).Decode(&users)
	if len(users) != 0 {
		t.Errorf("expected empty, got %v", users)
	}
}

// --- App Listing with Category Visibility ---

func TestCategory_AppVisibilityPublicOnly(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create categories
	resp := testutil.AuthPost(t, ts.URL+"/api/categories", ts.AdminToken,
		[]byte(`{"id":"cat-pub","name":"Public"}`))
	resp.Body.Close()

	resp = testutil.AuthPost(t, ts.URL+"/api/categories", ts.AdminToken,
		[]byte(`{"id":"cat-priv","name":"Private"}`))
	resp.Body.Close()

	// Create apps with visibility on the app
	resp = testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken,
		[]byte(`{"id":"app-pub","name":"Public App","url":"https://pub.com","launch_type":"url","category":"Public","visibility":"public"}`))
	resp.Body.Close()

	resp = testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken,
		[]byte(`{"id":"app-priv","name":"Private App","url":"https://priv.com","launch_type":"url","category":"Private","visibility":"admin_only"}`))
	resp.Body.Close()

	// Regular user should only see public app
	testutil.CreateUser(t, ts.URL, ts.AdminToken, "viewer1", "pass123", []string{"user"})
	viewerToken := testutil.LoginAs(t, ts.URL, "viewer1", "pass123")

	resp = testutil.AuthGet(t, ts.URL+"/api/apps", viewerToken)
	defer resp.Body.Close()

	var apps []db.Application
	json.NewDecoder(resp.Body).Decode(&apps)

	if len(apps) != 1 {
		t.Fatalf("expected 1 app, got %d", len(apps))
	}
	if apps[0].ID != "app-pub" {
		t.Errorf("expected app-pub, got %s", apps[0].ID)
	}
}

func TestCategory_AppVisibilityApprovedUser(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create categories
	resp := testutil.AuthPost(t, ts.URL+"/api/categories", ts.AdminToken,
		[]byte(`{"id":"cat-pub2","name":"Public2"}`))
	resp.Body.Close()

	resp = testutil.AuthPost(t, ts.URL+"/api/categories", ts.AdminToken,
		[]byte(`{"id":"cat-appr","name":"Approved"}`))
	resp.Body.Close()

	// Create apps with visibility on the app
	resp = testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken,
		[]byte(`{"id":"app-pub2","name":"Public App 2","url":"https://pub2.com","launch_type":"url","category":"Public2","visibility":"public"}`))
	resp.Body.Close()

	resp = testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken,
		[]byte(`{"id":"app-appr","name":"Approved App","url":"https://appr.com","launch_type":"url","category":"Approved","visibility":"approved"}`))
	resp.Body.Close()

	// Create user — initially sees only public
	userID := testutil.CreateUser(t, ts.URL, ts.AdminToken, "approvedviewer", "pass123", []string{"user"})
	userToken := testutil.LoginAs(t, ts.URL, "approvedviewer", "pass123")

	resp = testutil.AuthGet(t, ts.URL+"/api/apps", userToken)
	var apps []db.Application
	json.NewDecoder(resp.Body).Decode(&apps)
	resp.Body.Close()
	if len(apps) != 1 {
		t.Fatalf("expected 1 app before approval, got %d", len(apps))
	}

	// Approve user for the category
	addBody := []byte(fmt.Sprintf(`{"user_id":%q}`, userID))
	resp = testutil.AuthPost(t, ts.URL+"/api/categories/cat-appr/approved-users", ts.AdminToken, addBody)
	resp.Body.Close()

	// Now user sees both
	resp = testutil.AuthGet(t, ts.URL+"/api/apps", userToken)
	defer resp.Body.Close()
	json.NewDecoder(resp.Body).Decode(&apps)
	if len(apps) != 2 {
		t.Fatalf("expected 2 apps after approval, got %d", len(apps))
	}
}

func TestCategory_AppVisibilityCategoryAdmin(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create category
	resp := testutil.AuthPost(t, ts.URL+"/api/categories", ts.AdminToken,
		[]byte(`{"id":"cat-ao","name":"AdminOnly"}`))
	resp.Body.Close()

	// Create app with admin_only visibility
	resp = testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken,
		[]byte(`{"id":"app-ao","name":"Admin Only App","url":"https://ao.com","launch_type":"url","category":"AdminOnly","visibility":"admin_only"}`))
	resp.Body.Close()

	// Create user and make them category admin
	userID := testutil.CreateUser(t, ts.URL, ts.AdminToken, "catadm", "pass123", []string{"user"})
	userToken := testutil.LoginAs(t, ts.URL, "catadm", "pass123")

	// Before: user sees 0 apps
	resp = testutil.AuthGet(t, ts.URL+"/api/apps", userToken)
	var apps []db.Application
	json.NewDecoder(resp.Body).Decode(&apps)
	resp.Body.Close()
	if len(apps) != 0 {
		t.Fatalf("expected 0 apps before cat admin, got %d", len(apps))
	}

	// Make user a category admin
	addBody := []byte(fmt.Sprintf(`{"user_id":%q}`, userID))
	resp = testutil.AuthPost(t, ts.URL+"/api/categories/cat-ao/admins", ts.AdminToken, addBody)
	resp.Body.Close()

	// After: user sees the app
	resp = testutil.AuthGet(t, ts.URL+"/api/apps", userToken)
	defer resp.Body.Close()
	json.NewDecoder(resp.Body).Decode(&apps)
	if len(apps) != 1 {
		t.Fatalf("expected 1 app after cat admin, got %d", len(apps))
	}
	if apps[0].ID != "app-ao" {
		t.Errorf("expected app-ao, got %s", apps[0].ID)
	}
}

func TestCategory_AdminSeesAllApps(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create categories
	resp := testutil.AuthPost(t, ts.URL+"/api/categories", ts.AdminToken,
		[]byte(`{"id":"cat-all1","name":"AllPub"}`))
	resp.Body.Close()

	resp = testutil.AuthPost(t, ts.URL+"/api/categories", ts.AdminToken,
		[]byte(`{"id":"cat-all2","name":"AllAppr"}`))
	resp.Body.Close()

	resp = testutil.AuthPost(t, ts.URL+"/api/categories", ts.AdminToken,
		[]byte(`{"id":"cat-all3","name":"AllAdmin"}`))
	resp.Body.Close()

	// Create apps with different visibilities
	visibilities := []string{"public", "approved", "admin_only"}
	for i, cat := range []string{"AllPub", "AllAppr", "AllAdmin"} {
		body := fmt.Sprintf(`{"id":"app-all%d","name":"App %d","url":"https://all%d.com","launch_type":"url","category":"%s","visibility":"%s"}`, i, i, i, cat, visibilities[i])
		resp = testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken, []byte(body))
		resp.Body.Close()
	}

	// Admin sees all 3
	resp = testutil.AuthGet(t, ts.URL+"/api/apps", ts.AdminToken)
	defer resp.Body.Close()

	var apps []db.Application
	json.NewDecoder(resp.Body).Decode(&apps)
	if len(apps) != 3 {
		t.Errorf("expected 3 apps for admin, got %d", len(apps))
	}
}

// --- App Management with Category Admin Perms ---

func TestCategory_CategoryAdminCanCreateApp(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create category
	resp := testutil.AuthPost(t, ts.URL+"/api/categories", ts.AdminToken,
		[]byte(`{"id":"cat-mgmt","name":"Managed"}`))
	resp.Body.Close()

	// Create user and make them category admin
	userID := testutil.CreateUser(t, ts.URL, ts.AdminToken, "appcreator", "pass123", []string{"user"})
	addBody := []byte(fmt.Sprintf(`{"user_id":%q}`, userID))
	resp = testutil.AuthPost(t, ts.URL+"/api/categories/cat-mgmt/admins", ts.AdminToken, addBody)
	resp.Body.Close()

	// Category admin creates an app
	userToken := testutil.LoginAs(t, ts.URL, "appcreator", "pass123")
	appBody := []byte(`{"id":"app-mgmt","name":"Managed App","url":"https://mgmt.com","launch_type":"url","category":"Managed"}`)
	resp = testutil.AuthPost(t, ts.URL+"/api/apps", userToken, appBody)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(b))
	}
}

func TestCategory_CategoryAdminCannotCreateAppInOtherCategory(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create two categories
	resp := testutil.AuthPost(t, ts.URL+"/api/categories", ts.AdminToken,
		[]byte(`{"id":"cat-mine","name":"Mine"}`))
	resp.Body.Close()

	resp = testutil.AuthPost(t, ts.URL+"/api/categories", ts.AdminToken,
		[]byte(`{"id":"cat-other","name":"Other"}`))
	resp.Body.Close()

	// Make user admin of "Mine" only
	userID := testutil.CreateUser(t, ts.URL, ts.AdminToken, "limitedadmin", "pass123", []string{"user"})
	addBody := []byte(fmt.Sprintf(`{"user_id":%q}`, userID))
	resp = testutil.AuthPost(t, ts.URL+"/api/categories/cat-mine/admins", ts.AdminToken, addBody)
	resp.Body.Close()

	// Try to create app in "Other" — should fail
	userToken := testutil.LoginAs(t, ts.URL, "limitedadmin", "pass123")
	appBody := []byte(`{"id":"app-other","name":"Other App","url":"https://other.com","launch_type":"url","category":"Other"}`)
	resp = testutil.AuthPost(t, ts.URL+"/api/apps", userToken, appBody)
	resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

func TestCategory_CategoryAdminCanDeleteAppInCategory(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create category and app
	resp := testutil.AuthPost(t, ts.URL+"/api/categories", ts.AdminToken,
		[]byte(`{"id":"cat-delmgmt","name":"DelManaged"}`))
	resp.Body.Close()

	resp = testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken,
		[]byte(`{"id":"app-delmgmt","name":"Del App","url":"https://del.com","launch_type":"url","category":"DelManaged"}`))
	resp.Body.Close()

	// Make user category admin
	userID := testutil.CreateUser(t, ts.URL, ts.AdminToken, "deleter1", "pass123", []string{"user"})
	addBody := []byte(fmt.Sprintf(`{"user_id":%q}`, userID))
	resp = testutil.AuthPost(t, ts.URL+"/api/categories/cat-delmgmt/admins", ts.AdminToken, addBody)
	resp.Body.Close()

	// Category admin deletes app
	userToken := testutil.LoginAs(t, ts.URL, "deleter1", "pass123")
	resp = testutil.AuthDelete(t, ts.URL+"/api/apps/app-delmgmt", userToken)
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
}

func TestCategory_RegularUserCannotCreateApp(t *testing.T) {
	ts := testutil.NewTestServer(t)

	testutil.CreateUser(t, ts.URL, ts.AdminToken, "noappuser", "pass123", []string{"user"})
	userToken := testutil.LoginAs(t, ts.URL, "noappuser", "pass123")

	appBody := []byte(`{"id":"app-nope","name":"Nope","url":"https://nope.com","launch_type":"url"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/apps", userToken, appBody)
	resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

// --- Category Update by Category Admin ---

func TestCategory_CategoryAdminCanUpdateTheirCategory(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create category
	resp := testutil.AuthPost(t, ts.URL+"/api/categories", ts.AdminToken,
		[]byte(`{"id":"cat-upca","name":"UpdByCatAdmin"}`))
	resp.Body.Close()

	// Make user category admin
	userID := testutil.CreateUser(t, ts.URL, ts.AdminToken, "catupdater", "pass123", []string{"user"})
	addBody := []byte(fmt.Sprintf(`{"user_id":%q}`, userID))
	resp = testutil.AuthPost(t, ts.URL+"/api/categories/cat-upca/admins", ts.AdminToken, addBody)
	resp.Body.Close()

	// Category admin updates category
	userToken := testutil.LoginAs(t, ts.URL, "catupdater", "pass123")
	updateBody := []byte(`{"name":"UpdByCatAdmin","description":"Updated by cat admin"}`)
	resp = testutil.AuthPut(t, ts.URL+"/api/categories/cat-upca", userToken, updateBody)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}
}

// --- Auto-create category on app create ---

func TestCategory_AutoCreateOnAppCreate(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create app with unknown category — should auto-create
	appBody := []byte(`{"id":"app-auto","name":"Auto","url":"https://auto.com","launch_type":"url","category":"NewCat"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/apps", ts.AdminToken, appBody)
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(b))
	}

	// Verify category was created
	resp = testutil.AuthGet(t, ts.URL+"/api/categories", ts.AdminToken)
	defer resp.Body.Close()

	var cats []db.Category
	json.NewDecoder(resp.Body).Decode(&cats)

	found := false
	for _, c := range cats {
		if c.Name == "NewCat" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected auto-created category 'NewCat' not found")
	}
}
