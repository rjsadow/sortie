package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/rjsadow/sortie/tests/integration/testutil"
)

func TestRecording_FullLifecycle(t *testing.T) {
	ts := testutil.NewTestServer(t, testutil.WithRecordingEnabled())
	sessionID := createRunningSession(t, ts, "rec-lifecycle-app", ts.AdminToken, "admin-admin")

	// 1. Start recording
	resp := testutil.AuthPost(t, ts.URL+"/api/sessions/"+sessionID+"/recording/start", ts.AdminToken, nil)
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("start recording: expected 201, got %d: %s", resp.StatusCode, string(b))
	}
	var startResult map[string]string
	json.NewDecoder(resp.Body).Decode(&startResult)
	resp.Body.Close()

	recordingID := startResult["recording_id"]
	if recordingID == "" {
		t.Fatal("expected recording_id in start response")
	}
	if startResult["status"] != "recording" {
		t.Errorf("expected status 'recording', got %q", startResult["status"])
	}

	// 2. Stop recording
	stopBody, _ := json.Marshal(map[string]string{"recording_id": recordingID})
	resp = testutil.AuthPost(t, ts.URL+"/api/sessions/"+sessionID+"/recording/stop", ts.AdminToken, stopBody)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("stop recording: expected 200, got %d: %s", resp.StatusCode, string(b))
	}
	resp.Body.Close()

	// 3. Upload recording
	dummyContent := []byte("fake-webm-video-content-for-testing")
	fields := map[string]string{
		"recording_id": recordingID,
		"duration":     "12.5",
	}
	resp = testutil.AuthPostMultipart(t, ts.URL+"/api/sessions/"+sessionID+"/recording/upload", ts.AdminToken, fields, "test.webm", dummyContent)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("upload recording: expected 200, got %d: %s", resp.StatusCode, string(b))
	}
	resp.Body.Close()

	// 4. List recordings - verify it's present and ready
	resp = testutil.AuthGet(t, ts.URL+"/api/recordings", ts.AdminToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list recordings: expected 200, got %d", resp.StatusCode)
	}
	var recs []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&recs)
	resp.Body.Close()

	found := false
	for _, rec := range recs {
		if rec["id"] == recordingID {
			found = true
			if rec["status"] != "ready" {
				t.Errorf("expected status 'ready', got %v", rec["status"])
			}
			break
		}
	}
	if !found {
		t.Fatalf("recording %s not found in list", recordingID)
	}

	// 5. Download recording - verify content matches
	resp = testutil.AuthGet(t, ts.URL+"/api/recordings/"+recordingID+"/download", ts.AdminToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download recording: expected 200, got %d", resp.StatusCode)
	}
	downloadedContent, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if string(downloadedContent) != string(dummyContent) {
		t.Errorf("downloaded content mismatch: got %d bytes, want %d bytes", len(downloadedContent), len(dummyContent))
	}

	// 6. Delete recording
	resp = testutil.AuthDelete(t, ts.URL+"/api/recordings/"+recordingID, ts.AdminToken)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete recording: expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 7. Verify it's gone
	resp = testutil.AuthGet(t, ts.URL+"/api/recordings", ts.AdminToken)
	json.NewDecoder(resp.Body).Decode(&recs)
	resp.Body.Close()

	for _, rec := range recs {
		if rec["id"] == recordingID {
			t.Fatalf("recording %s should have been deleted", recordingID)
		}
	}
}

func TestRecording_DisabledReturns404(t *testing.T) {
	ts := testutil.NewTestServer(t) // No WithRecordingEnabled
	sessionID := createRunningSession(t, ts, "rec-disabled-app", ts.AdminToken, "admin-admin")

	resp := testutil.AuthPost(t, ts.URL+"/api/sessions/"+sessionID+"/recording/start", ts.AdminToken, nil)
	body := testutil.ReadBody(t, resp)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 when recording disabled, got %d: %s", resp.StatusCode, body)
	}
}

func TestRecording_AdminListShowsAllUsers(t *testing.T) {
	ts := testutil.NewTestServer(t, testutil.WithRecordingEnabled())

	// Create two regular users
	userAID := testutil.CreateUser(t, ts.URL, ts.AdminToken, "rec-user-a", "pass123", []string{"user"})
	userBID := testutil.CreateUser(t, ts.URL, ts.AdminToken, "rec-user-b", "pass123", []string{"user"})
	tokenA := testutil.LoginAs(t, ts.URL, "rec-user-a", "pass123")
	tokenB := testutil.LoginAs(t, ts.URL, "rec-user-b", "pass123")

	// Each user creates a session and records
	recIDA := recStartAndUpload(t, ts, "rec-multi-a", tokenA, userAID)
	recIDB := recStartAndUpload(t, ts, "rec-multi-b", tokenB, userBID)

	// User A sees only their recording
	resp := testutil.AuthGet(t, ts.URL+"/api/recordings", tokenA)
	var userARecs []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&userARecs)
	resp.Body.Close()

	if len(userARecs) != 1 {
		t.Fatalf("user-a expected 1 recording, got %d", len(userARecs))
	}
	if userARecs[0]["id"] != recIDA {
		t.Errorf("user-a expected recording %s, got %v", recIDA, userARecs[0]["id"])
	}

	// Admin sees both recordings
	resp = testutil.AuthGet(t, ts.URL+"/api/admin/recordings", ts.AdminToken)
	var adminRecs []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&adminRecs)
	resp.Body.Close()

	foundA, foundB := false, false
	for _, rec := range adminRecs {
		if rec["id"] == recIDA {
			foundA = true
		}
		if rec["id"] == recIDB {
			foundB = true
		}
	}
	if !foundA || !foundB {
		t.Errorf("admin should see both recordings: foundA=%v foundB=%v (total=%d)", foundA, foundB, len(adminRecs))
	}
}

func TestRecording_NonOwnerCannotDelete(t *testing.T) {
	ts := testutil.NewTestServer(t, testutil.WithRecordingEnabled())

	// Create two users
	ownerID := testutil.CreateUser(t, ts.URL, ts.AdminToken, "rec-owner", "pass123", []string{"user"})
	testutil.CreateUser(t, ts.URL, ts.AdminToken, "rec-other", "pass123", []string{"user"})
	ownerToken := testutil.LoginAs(t, ts.URL, "rec-owner", "pass123")
	otherToken := testutil.LoginAs(t, ts.URL, "rec-other", "pass123")

	// Owner creates and uploads a recording
	recID := recStartAndUpload(t, ts, "rec-acl-app", ownerToken, ownerID)

	// Other user tries to delete it
	resp := testutil.AuthDelete(t, ts.URL+"/api/recordings/"+recID, otherToken)
	resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-owner delete: expected 403, got %d", resp.StatusCode)
	}

	// Verify recording still exists (owner can list it)
	resp = testutil.AuthGet(t, ts.URL+"/api/recordings", ownerToken)
	var recs []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&recs)
	resp.Body.Close()

	found := false
	for _, rec := range recs {
		if rec["id"] == recID {
			found = true
		}
	}
	if !found {
		t.Error("recording should still exist after failed delete by non-owner")
	}
}

func TestRecording_StartOnNonRunningSession(t *testing.T) {
	ts := testutil.NewTestServer(t, testutil.WithRecordingEnabled())
	sessionID := createRunningSession(t, ts, "rec-stopped-app", ts.AdminToken, "admin-admin")

	// Stop the session
	resp := testutil.AuthPost(t, ts.URL+"/api/sessions/"+sessionID+"/stop", ts.AdminToken, nil)
	resp.Body.Close()

	// Try to start recording on stopped session
	resp = testutil.AuthPost(t, ts.URL+"/api/sessions/"+sessionID+"/recording/start", ts.AdminToken, nil)
	body := testutil.ReadBody(t, resp)

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("start on non-running session: expected 409, got %d: %s", resp.StatusCode, body)
	}
}

// recStartAndUpload is a helper that creates a session, starts a recording,
// stops it, uploads a dummy file, and returns the recording ID.
func recStartAndUpload(t *testing.T, ts *testutil.TestServer, appID, token, userID string) string {
	t.Helper()

	sessionID := createRunningSession(t, ts, appID, token, userID)

	// Start
	resp := testutil.AuthPost(t, ts.URL+"/api/sessions/"+sessionID+"/recording/start", token, nil)
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("start recording failed: %d: %s", resp.StatusCode, string(b))
	}
	var startResult map[string]string
	json.NewDecoder(resp.Body).Decode(&startResult)
	resp.Body.Close()
	recordingID := startResult["recording_id"]

	// Stop
	stopBody, _ := json.Marshal(map[string]string{"recording_id": recordingID})
	resp = testutil.AuthPost(t, ts.URL+"/api/sessions/"+sessionID+"/recording/stop", token, stopBody)
	resp.Body.Close()

	// Upload
	fields := map[string]string{
		"recording_id": recordingID,
		"duration":     "5.0",
	}
	resp = testutil.AuthPostMultipart(t, ts.URL+"/api/sessions/"+sessionID+"/recording/upload", token, fields, "test.webm", []byte("dummy-video"))
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("upload recording failed: %d: %s", resp.StatusCode, string(b))
	}
	resp.Body.Close()

	return recordingID
}
