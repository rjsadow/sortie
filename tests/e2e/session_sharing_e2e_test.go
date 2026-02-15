package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// createE2ESession creates a session for the given app and user, waits for running,
// and returns the session ID. The token must belong to the user whose ID is userID.
func createE2ESession(appID, token, userID string) string {
	body := []byte(fmt.Sprintf(`{"app_id":%q,"user_id":%q}`, appID, userID))
	resp := authPost(baseURL+"/api/sessions", token, body)
	var session map[string]any
	json.NewDecoder(resp.Body).Decode(&session)
	resp.Body.Close()
	Expect(resp.StatusCode).To(Equal(http.StatusCreated), "create session for app %s", appID)

	sessionID := session["id"].(string)
	waitForSessionRunning(sessionID, 3*time.Minute)
	return sessionID
}

// cleanupSession stops and deletes a session, ignoring errors.
func cleanupSession(sessionID string) {
	resp := authPost(baseURL+"/api/sessions/"+sessionID+"/stop", adminToken, nil)
	resp.Body.Close()
	resp = authDelete(baseURL+"/api/sessions/"+sessionID, adminToken)
	resp.Body.Close()
}

var _ = Describe("Session Sharing", func() {

	Describe("Invite by username", func() {
		It("allows owner to share a session and viewer to see it", func() {
			// Setup: app + two users
			appID := "e2e-share-invite"
			createE2EApp(appID, "container")
			DeferCleanup(deleteE2EApp, appID)

			ownerID := createE2EUser("e2e-share-owner", "pass1234", []string{"user"})
			DeferCleanup(deleteE2EUser, ownerID)
			viewerID := createE2EUser("e2e-share-viewer", "pass1234", []string{"user"})
			DeferCleanup(deleteE2EUser, viewerID)

			ownerToken, err := login(baseURL, "e2e-share-owner", "pass1234")
			Expect(err).NotTo(HaveOccurred())
			viewerToken, err := login(baseURL, "e2e-share-viewer", "pass1234")
			Expect(err).NotTo(HaveOccurred())

			// Owner creates a session
			sessionID := createE2ESession(appID, ownerToken, ownerID)
			DeferCleanup(cleanupSession, sessionID)

			// Owner shares with viewer (read_only)
			shareBody := []byte(`{"username":"e2e-share-viewer","permission":"read_only"}`)
			resp := authPost(baseURL+"/api/sessions/"+sessionID+"/shares", ownerToken, shareBody)
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))
			var shareResp map[string]any
			json.NewDecoder(resp.Body).Decode(&shareResp)
			resp.Body.Close()

			Expect(shareResp["id"]).NotTo(BeEmpty())
			Expect(shareResp["permission"]).To(Equal("read_only"))

			// Viewer sees the shared session
			resp = authGet(baseURL+"/api/sessions/shared", viewerToken)
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			var sharedSessions []map[string]any
			json.NewDecoder(resp.Body).Decode(&sharedSessions)
			resp.Body.Close()

			Expect(sharedSessions).To(HaveLen(1))
			Expect(sharedSessions[0]["is_shared"]).To(BeTrue())
			Expect(sharedSessions[0]["owner_username"]).To(Equal("e2e-share-owner"))
			Expect(sharedSessions[0]["share_permission"]).To(Equal("read_only"))
		})
	})

	Describe("List and revoke shares", func() {
		It("lists shares for a session and revokes them", func() {
			appID := "e2e-share-list"
			createE2EApp(appID, "container")
			DeferCleanup(deleteE2EApp, appID)

			ownerID := createE2EUser("e2e-share-lister", "pass1234", []string{"user"})
			DeferCleanup(deleteE2EUser, ownerID)
			inviteeID := createE2EUser("e2e-share-invitee", "pass1234", []string{"user"})
			DeferCleanup(deleteE2EUser, inviteeID)

			ownerToken, err := login(baseURL, "e2e-share-lister", "pass1234")
			Expect(err).NotTo(HaveOccurred())
			inviteeToken, err := login(baseURL, "e2e-share-invitee", "pass1234")
			Expect(err).NotTo(HaveOccurred())

			sessionID := createE2ESession(appID, ownerToken, ownerID)
			DeferCleanup(cleanupSession, sessionID)

			// Create two shares: one by username, one link share
			resp := authPost(baseURL+"/api/sessions/"+sessionID+"/shares", ownerToken,
				[]byte(`{"username":"e2e-share-invitee","permission":"read_only"}`))
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))
			var share1 map[string]any
			json.NewDecoder(resp.Body).Decode(&share1)
			resp.Body.Close()
			shareID := share1["id"].(string)

			resp = authPost(baseURL+"/api/sessions/"+sessionID+"/shares", ownerToken,
				[]byte(`{"link_share":true,"permission":"read_write"}`))
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))
			resp.Body.Close()

			// List shares — should see 2
			resp = authGet(baseURL+"/api/sessions/"+sessionID+"/shares", ownerToken)
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			var shares []map[string]any
			json.NewDecoder(resp.Body).Decode(&shares)
			resp.Body.Close()
			Expect(shares).To(HaveLen(2))

			// Invitee sees 1 shared session
			resp = authGet(baseURL+"/api/sessions/shared", inviteeToken)
			var sessions []map[string]any
			json.NewDecoder(resp.Body).Decode(&sessions)
			resp.Body.Close()
			Expect(sessions).To(HaveLen(1))

			// Revoke the username share
			resp = authDelete(baseURL+"/api/sessions/"+sessionID+"/shares/"+shareID, ownerToken)
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

			// Invitee no longer sees shared session
			resp = authGet(baseURL+"/api/sessions/shared", inviteeToken)
			json.NewDecoder(resp.Body).Decode(&sessions)
			resp.Body.Close()
			Expect(sessions).To(HaveLen(0))
		})
	})

	Describe("Link sharing", func() {
		It("generates a share link and allows joining via token", func() {
			appID := "e2e-share-link"
			createE2EApp(appID, "container")
			DeferCleanup(deleteE2EApp, appID)

			ownerID := createE2EUser("e2e-link-owner", "pass1234", []string{"user"})
			DeferCleanup(deleteE2EUser, ownerID)
			joinerID := createE2EUser("e2e-link-joiner", "pass1234", []string{"user"})
			DeferCleanup(deleteE2EUser, joinerID)

			ownerToken, err := login(baseURL, "e2e-link-owner", "pass1234")
			Expect(err).NotTo(HaveOccurred())
			joinerToken, err := login(baseURL, "e2e-link-joiner", "pass1234")
			Expect(err).NotTo(HaveOccurred())

			sessionID := createE2ESession(appID, ownerToken, ownerID)
			DeferCleanup(cleanupSession, sessionID)

			// Generate link share
			resp := authPost(baseURL+"/api/sessions/"+sessionID+"/shares", ownerToken,
				[]byte(`{"link_share":true,"permission":"read_only"}`))
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))
			var shareResp map[string]any
			json.NewDecoder(resp.Body).Decode(&shareResp)
			resp.Body.Close()

			shareURL, ok := shareResp["share_url"].(string)
			Expect(ok).To(BeTrue())
			Expect(shareURL).NotTo(BeEmpty())

			// Extract token from URL: /session/{id}?share_token={token}
			prefix := fmt.Sprintf("/session/%s?share_token=", sessionID)
			Expect(shareURL).To(HavePrefix(prefix))
			token := shareURL[len(prefix):]

			// Joiner joins via token
			joinBody := []byte(fmt.Sprintf(`{"token":%q}`, token))
			resp = authPost(baseURL+"/api/sessions/shares/join", joinerToken, joinBody)
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			var joinResp map[string]any
			json.NewDecoder(resp.Body).Decode(&joinResp)
			resp.Body.Close()

			Expect(joinResp["id"]).To(Equal(sessionID))
			Expect(joinResp["is_shared"]).To(BeTrue())
			Expect(joinResp["share_permission"]).To(Equal("read_only"))
		})
	})

	Describe("Access control", func() {
		It("prevents non-owners from sharing or listing shares", func() {
			appID := "e2e-share-acl"
			createE2EApp(appID, "container")
			DeferCleanup(deleteE2EApp, appID)

			ownerID := createE2EUser("e2e-acl-owner", "pass1234", []string{"user"})
			DeferCleanup(deleteE2EUser, ownerID)
			otherID := createE2EUser("e2e-acl-other", "pass1234", []string{"user"})
			DeferCleanup(deleteE2EUser, otherID)

			ownerToken, err := login(baseURL, "e2e-acl-owner", "pass1234")
			Expect(err).NotTo(HaveOccurred())
			otherToken, err := login(baseURL, "e2e-acl-other", "pass1234")
			Expect(err).NotTo(HaveOccurred())

			sessionID := createE2ESession(appID, ownerToken, ownerID)
			DeferCleanup(cleanupSession, sessionID)

			// Non-owner tries to create a share → 403
			resp := authPost(baseURL+"/api/sessions/"+sessionID+"/shares", otherToken,
				[]byte(`{"username":"e2e-acl-owner","permission":"read_only"}`))
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusForbidden))

			// Non-owner tries to list shares → 403
			resp = authGet(baseURL+"/api/sessions/"+sessionID+"/shares", otherToken)
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
		})

		It("prevents sharing with yourself", func() {
			appID := "e2e-share-self"
			createE2EApp(appID, "container")
			DeferCleanup(deleteE2EApp, appID)

			userID := createE2EUser("e2e-selfshare", "pass1234", []string{"user"})
			DeferCleanup(deleteE2EUser, userID)

			token, err := login(baseURL, "e2e-selfshare", "pass1234")
			Expect(err).NotTo(HaveOccurred())

			sessionID := createE2ESession(appID, token, userID)
			DeferCleanup(cleanupSession, sessionID)

			resp := authPost(baseURL+"/api/sessions/"+sessionID+"/shares", token,
				[]byte(`{"username":"e2e-selfshare","permission":"read_only"}`))
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})

		It("returns 404 for nonexistent user", func() {
			appID := "e2e-share-nouser"
			createE2EApp(appID, "container")
			DeferCleanup(deleteE2EApp, appID)

			userID := createE2EUser("e2e-nouser-sharer", "pass1234", []string{"user"})
			DeferCleanup(deleteE2EUser, userID)

			token, err := login(baseURL, "e2e-nouser-sharer", "pass1234")
			Expect(err).NotTo(HaveOccurred())

			sessionID := createE2ESession(appID, token, userID)
			DeferCleanup(cleanupSession, sessionID)

			resp := authPost(baseURL+"/api/sessions/"+sessionID+"/shares", token,
				[]byte(`{"username":"ghost-user","permission":"read_only"}`))
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		})

		It("returns 404 for invalid share token", func() {
			userID := createE2EUser("e2e-badtoken", "pass1234", []string{"user"})
			DeferCleanup(deleteE2EUser, userID)

			token, err := login(baseURL, "e2e-badtoken", "pass1234")
			Expect(err).NotTo(HaveOccurred())

			resp := authPost(baseURL+"/api/sessions/shares/join", token,
				[]byte(`{"token":"nonexistent-token"}`))
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		})
	})

	Describe("Empty shared sessions", func() {
		It("returns empty list when no sessions are shared with user", func() {
			userID := createE2EUser("e2e-lonely", "pass1234", []string{"user"})
			DeferCleanup(deleteE2EUser, userID)

			token, err := login(baseURL, "e2e-lonely", "pass1234")
			Expect(err).NotTo(HaveOccurred())

			resp := authGet(baseURL+"/api/sessions/shared", token)
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			var sessions []map[string]any
			json.NewDecoder(resp.Body).Decode(&sessions)
			resp.Body.Close()
			Expect(sessions).To(HaveLen(0))
		})
	})
})
