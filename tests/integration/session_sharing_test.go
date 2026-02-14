package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/rjsadow/sortie/tests/integration/testutil"
)

// createRunningSession creates a container app and session, waits for running, returns the session ID.
// userID must match the ID of the user whose token is passed.
func createRunningSession(t *testing.T, ts *testutil.TestServer, appID, userToken, userID string) string {
	t.Helper()
	createContainerApp(t, ts, appID)
	body, _ := json.Marshal(map[string]string{"app_id": appID, "user_id": userID})
	resp := testutil.AuthPost(t, ts.URL+"/api/sessions", userToken, body)
	var session map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&session)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	sessionID := session["id"].(string)
	waitForRunning(t, ts, sessionID)
	return sessionID
}

func TestSessionSharing_InviteByUsername(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create two users: owner and viewer
	ownerID := testutil.CreateUser(t, ts.URL, ts.AdminToken, "owner", "pass123", []string{"user"})
	testutil.CreateUser(t, ts.URL, ts.AdminToken, "viewer", "pass123", []string{"user"})
	ownerToken := testutil.LoginAs(t, ts.URL, "owner", "pass123")
	viewerToken := testutil.LoginAs(t, ts.URL, "viewer", "pass123")

	// Owner creates a session
	sessionID := createRunningSession(t, ts, "share-invite-app", ownerToken, ownerID)

	// Owner shares with viewer (read_only)
	shareBody := []byte(`{"username":"viewer","permission":"read_only"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/sessions/"+sessionID+"/shares", ownerToken, shareBody)
	if resp.StatusCode != http.StatusCreated {
		body := testutil.ReadBody(t, resp)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}
	var shareResp map[string]interface{}
	testutil.ReadJSON(t, resp, &shareResp)

	if shareResp["id"] == nil || shareResp["id"] == "" {
		t.Error("expected share ID")
	}
	if shareResp["permission"] != "read_only" {
		t.Errorf("expected read_only permission, got %v", shareResp["permission"])
	}

	// Viewer sees the shared session in /api/sessions/shared
	resp = testutil.AuthGet(t, ts.URL+"/api/sessions/shared", viewerToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var sharedSessions []map[string]interface{}
	testutil.ReadJSON(t, resp, &sharedSessions)

	if len(sharedSessions) != 1 {
		t.Fatalf("expected 1 shared session, got %d", len(sharedSessions))
	}
	if sharedSessions[0]["is_shared"] != true {
		t.Error("expected is_shared=true")
	}
	if sharedSessions[0]["owner_username"] != "owner" {
		t.Errorf("expected owner_username=owner, got %v", sharedSessions[0]["owner_username"])
	}
	if sharedSessions[0]["share_permission"] != "read_only" {
		t.Errorf("expected share_permission=read_only, got %v", sharedSessions[0]["share_permission"])
	}
}

func TestSessionSharing_ListShares(t *testing.T) {
	ts := testutil.NewTestServer(t)

	listerID := testutil.CreateUser(t, ts.URL, ts.AdminToken, "lister", "pass123", []string{"user"})
	testutil.CreateUser(t, ts.URL, ts.AdminToken, "invitee", "pass123", []string{"user"})
	listerToken := testutil.LoginAs(t, ts.URL, "lister", "pass123")

	sessionID := createRunningSession(t, ts, "share-list-app", listerToken, listerID)

	// Create two shares
	testutil.AuthPost(t, ts.URL+"/api/sessions/"+sessionID+"/shares", listerToken,
		[]byte(`{"username":"invitee","permission":"read_only"}`)).Body.Close()
	testutil.AuthPost(t, ts.URL+"/api/sessions/"+sessionID+"/shares", listerToken,
		[]byte(`{"link_share":true,"permission":"read_write"}`)).Body.Close()

	// List shares
	resp := testutil.AuthGet(t, ts.URL+"/api/sessions/"+sessionID+"/shares", listerToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var shares []map[string]interface{}
	testutil.ReadJSON(t, resp, &shares)

	if len(shares) != 2 {
		t.Fatalf("expected 2 shares, got %d", len(shares))
	}
}

func TestSessionSharing_RevokeShare(t *testing.T) {
	ts := testutil.NewTestServer(t)

	revokerID := testutil.CreateUser(t, ts.URL, ts.AdminToken, "revoker", "pass123", []string{"user"})
	testutil.CreateUser(t, ts.URL, ts.AdminToken, "revokee", "pass123", []string{"user"})
	revokerToken := testutil.LoginAs(t, ts.URL, "revoker", "pass123")
	revokeeToken := testutil.LoginAs(t, ts.URL, "revokee", "pass123")

	sessionID := createRunningSession(t, ts, "share-revoke-app", revokerToken, revokerID)

	// Create a share
	resp := testutil.AuthPost(t, ts.URL+"/api/sessions/"+sessionID+"/shares", revokerToken,
		[]byte(`{"username":"revokee","permission":"read_write"}`))
	var shareResp map[string]interface{}
	testutil.ReadJSON(t, resp, &shareResp)
	shareID := shareResp["id"].(string)

	// Verify revokee sees it
	resp = testutil.AuthGet(t, ts.URL+"/api/sessions/shared", revokeeToken)
	var sessions []map[string]interface{}
	testutil.ReadJSON(t, resp, &sessions)
	if len(sessions) != 1 {
		t.Fatalf("expected 1 shared session before revoke, got %d", len(sessions))
	}

	// Revoke
	resp = testutil.AuthDelete(t, ts.URL+"/api/sessions/"+sessionID+"/shares/"+shareID, revokerToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}

	// Verify revokee no longer sees it
	resp = testutil.AuthGet(t, ts.URL+"/api/sessions/shared", revokeeToken)
	testutil.ReadJSON(t, resp, &sessions)
	if len(sessions) != 0 {
		t.Errorf("expected 0 shared sessions after revoke, got %d", len(sessions))
	}
}

func TestSessionSharing_LinkShare(t *testing.T) {
	ts := testutil.NewTestServer(t)

	linkerID := testutil.CreateUser(t, ts.URL, ts.AdminToken, "linker", "pass123", []string{"user"})
	testutil.CreateUser(t, ts.URL, ts.AdminToken, "joiner", "pass123", []string{"user"})
	linkerToken := testutil.LoginAs(t, ts.URL, "linker", "pass123")
	joinerToken := testutil.LoginAs(t, ts.URL, "joiner", "pass123")

	sessionID := createRunningSession(t, ts, "share-link-app", linkerToken, linkerID)

	// Generate a link share
	resp := testutil.AuthPost(t, ts.URL+"/api/sessions/"+sessionID+"/shares", linkerToken,
		[]byte(`{"link_share":true,"permission":"read_only"}`))
	if resp.StatusCode != http.StatusCreated {
		body := testutil.ReadBody(t, resp)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}
	var shareResp map[string]interface{}
	testutil.ReadJSON(t, resp, &shareResp)

	shareURL := shareResp["share_url"].(string)
	if shareURL == "" {
		t.Fatal("expected non-empty share_url for link share")
	}

	// Extract token from URL (format: /session/{id}?share_token={token})
	// We need the token value
	// Parse manually since it's a path, not a full URL
	// Find share_token= in the URL
	tokenStart := len("/session/" + sessionID + "?share_token=")
	if len(shareURL) <= tokenStart {
		t.Fatalf("unexpected share_url format: %s", shareURL)
	}
	token := shareURL[tokenStart:]

	// Joiner uses the token to join
	joinBody, _ := json.Marshal(map[string]string{"token": token})
	resp = testutil.AuthPost(t, ts.URL+"/api/sessions/shares/join", joinerToken, joinBody)
	if resp.StatusCode != http.StatusOK {
		body := testutil.ReadBody(t, resp)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var joinResp map[string]interface{}
	testutil.ReadJSON(t, resp, &joinResp)

	if joinResp["id"] != sessionID {
		t.Errorf("expected session ID %s, got %v", sessionID, joinResp["id"])
	}
	if joinResp["is_shared"] != true {
		t.Error("expected is_shared=true in join response")
	}
	if joinResp["share_permission"] != "read_only" {
		t.Errorf("expected share_permission=read_only, got %v", joinResp["share_permission"])
	}
}

func TestSessionSharing_InvalidToken(t *testing.T) {
	ts := testutil.NewTestServer(t)

	testutil.CreateUser(t, ts.URL, ts.AdminToken, "badjoiner", "pass123", []string{"user"})
	joinerToken := testutil.LoginAs(t, ts.URL, "badjoiner", "pass123")

	joinBody := []byte(`{"token":"nonexistent-token"}`)
	resp := testutil.AuthPost(t, ts.URL+"/api/sessions/shares/join", joinerToken, joinBody)
	resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for invalid token, got %d", resp.StatusCode)
	}
}

func TestSessionSharing_OnlyOwnerCanShare(t *testing.T) {
	ts := testutil.NewTestServer(t)

	ownerID := testutil.CreateUser(t, ts.URL, ts.AdminToken, "realowner", "pass123", []string{"user"})
	testutil.CreateUser(t, ts.URL, ts.AdminToken, "notowner", "pass123", []string{"user"})
	ownerToken := testutil.LoginAs(t, ts.URL, "realowner", "pass123")
	notOwnerToken := testutil.LoginAs(t, ts.URL, "notowner", "pass123")

	sessionID := createRunningSession(t, ts, "share-owner-app", ownerToken, ownerID)

	// Non-owner tries to share
	resp := testutil.AuthPost(t, ts.URL+"/api/sessions/"+sessionID+"/shares", notOwnerToken,
		[]byte(`{"username":"realowner","permission":"read_only"}`))
	resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for non-owner sharing, got %d", resp.StatusCode)
	}

	// Non-owner tries to list shares
	resp = testutil.AuthGet(t, ts.URL+"/api/sessions/"+sessionID+"/shares", notOwnerToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for non-owner listing shares, got %d", resp.StatusCode)
	}
}

func TestSessionSharing_CannotShareWithSelf(t *testing.T) {
	ts := testutil.NewTestServer(t)

	userID := testutil.CreateUser(t, ts.URL, ts.AdminToken, "selfsharer", "pass123", []string{"user"})
	token := testutil.LoginAs(t, ts.URL, "selfsharer", "pass123")

	sessionID := createRunningSession(t, ts, "share-self-app", token, userID)

	resp := testutil.AuthPost(t, ts.URL+"/api/sessions/"+sessionID+"/shares", token,
		[]byte(`{"username":"selfsharer","permission":"read_only"}`))
	resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for sharing with self, got %d", resp.StatusCode)
	}
}

func TestSessionSharing_NonexistentUser(t *testing.T) {
	ts := testutil.NewTestServer(t)

	userID := testutil.CreateUser(t, ts.URL, ts.AdminToken, "sharer2", "pass123", []string{"user"})
	token := testutil.LoginAs(t, ts.URL, "sharer2", "pass123")

	sessionID := createRunningSession(t, ts, "share-nouser-app", token, userID)

	resp := testutil.AuthPost(t, ts.URL+"/api/sessions/"+sessionID+"/shares", token,
		[]byte(`{"username":"ghost","permission":"read_only"}`))
	resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent user, got %d", resp.StatusCode)
	}
}

func TestSessionSharing_SharedSessionsEmpty(t *testing.T) {
	ts := testutil.NewTestServer(t)

	testutil.CreateUser(t, ts.URL, ts.AdminToken, "lonely", "pass123", []string{"user"})
	token := testutil.LoginAs(t, ts.URL, "lonely", "pass123")

	resp := testutil.AuthGet(t, ts.URL+"/api/sessions/shared", token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var sessions []map[string]interface{}
	testutil.ReadJSON(t, resp, &sessions)

	if len(sessions) != 0 {
		t.Errorf("expected 0 shared sessions, got %d", len(sessions))
	}
}
