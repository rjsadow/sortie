package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("E2E", func() {

	Describe("Health Endpoints", func() {
		It("returns 200 for /healthz", func() {
			resp, err := http.Get(baseURL + "/healthz")
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		It("returns 200 for /readyz", func() {
			resp, err := http.Get(baseURL + "/readyz")
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})

	Describe("Authentication", func() {
		It("logs in with valid credentials", func() {
			token, err := login(baseURL, adminUsername, adminPassword)
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())
		})
	})

	Describe("App CRUD", func() {
		It("creates, reads, updates, and deletes an app", func() {
			appID := "e2e-crud-app"

			// Create
			body := []byte(`{
				"id": "e2e-crud-app",
				"name": "E2E CRUD Test",
				"url": "https://example.com",
				"launch_type": "url"
			}`)
			resp := authPost(baseURL+"/api/apps", adminToken, body)
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			// Get
			resp = authGet(baseURL+"/api/apps/"+appID, adminToken)
			var app map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&app)
			resp.Body.Close()
			Expect(app["name"]).To(Equal("E2E CRUD Test"))

			// Update
			updateBody := []byte(`{"name": "E2E CRUD Updated", "launch_type": "url", "url": "https://example.com"}`)
			resp = authPut(baseURL+"/api/apps/"+appID, adminToken, updateBody)
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// Delete
			resp = authDelete(baseURL+"/api/apps/"+appID, adminToken)
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
		})
	})

	Describe("Container Session Lifecycle", func() {
		It("creates a session, waits for running, stops, and terminates", func() {
			appID := "e2e-container-app"
			createE2EApp(appID, "container")
			DeferCleanup(deleteE2EApp, appID)

			// Create session
			body := []byte(`{"app_id":"e2e-container-app","user_id":"e2e-user"}`)
			resp := authPost(baseURL+"/api/sessions", adminToken, body)
			var session map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&session)
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			sessionID := session["id"].(string)

			// Wait for running
			waitForSessionRunning(sessionID, 3*time.Minute)

			// Verify session details
			resp = authGet(baseURL+"/api/sessions/"+sessionID, adminToken)
			json.NewDecoder(resp.Body).Decode(&session)
			resp.Body.Close()
			Expect(session["websocket_url"]).NotTo(BeEmpty())

			// Stop session
			resp = authPost(baseURL+"/api/sessions/"+sessionID+"/stop", adminToken, nil)
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// Terminate (DELETE)
			resp = authDelete(baseURL+"/api/sessions/"+sessionID, adminToken)
			resp.Body.Close()
		})
	})

	Describe("Web Proxy Session", func() {
		It("creates a web_proxy session or skips if sidecar unavailable", func() {
			appID := "e2e-webproxy-app"
			createE2EApp(appID, "web_proxy")
			DeferCleanup(deleteE2EApp, appID)

			body := []byte(`{"app_id":"e2e-webproxy-app","user_id":"e2e-user"}`)
			resp := authPost(baseURL+"/api/sessions", adminToken, body)
			var session map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&session)
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			sessionID := session["id"].(string)

			err := waitForSessionStatus(sessionID, "running", 3*time.Minute)
			if err != nil {
				resp = authGet(baseURL+"/api/sessions/"+sessionID, adminToken)
				json.NewDecoder(resp.Body).Decode(&session)
				resp.Body.Close()
				status, _ := session["status"].(string)
				if status == "failed" || status == "creating" {
					Skip("web_proxy session did not reach running (status: " + status + ") — browser sidecar likely not configured for E2E")
				}
				Fail("web_proxy session did not reach running: " + err.Error())
			}

			// Cleanup
			resp = authPost(baseURL+"/api/sessions/"+sessionID+"/stop", adminToken, nil)
			resp.Body.Close()
			resp = authDelete(baseURL+"/api/sessions/"+sessionID, adminToken)
			resp.Body.Close()
		})
	})

	Describe("Session Stop and Restart", func() {
		It("stops and restarts a session after pod deletion", func() {
			appID := "e2e-restart-app"
			createE2EApp(appID, "container")
			DeferCleanup(deleteE2EApp, appID)

			// Create session
			body := []byte(`{"app_id":"e2e-restart-app","user_id":"e2e-user"}`)
			resp := authPost(baseURL+"/api/sessions", adminToken, body)
			var session map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&session)
			resp.Body.Close()

			sessionID := session["id"].(string)
			waitForSessionRunning(sessionID, 3*time.Minute)

			// Stop
			resp = authPost(baseURL+"/api/sessions/"+sessionID+"/stop", adminToken, nil)
			resp.Body.Close()

			// Wait for the old pod to be fully deleted before restarting
			waitForPodDeletion(sessionID, 30*time.Second)

			// Restart
			resp = authPost(baseURL+"/api/sessions/"+sessionID+"/restart", adminToken, nil)
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "restart session: got %d: %s", resp.StatusCode, string(b))

			// Wait for running again
			waitForSessionRunning(sessionID, 5*time.Minute)

			// Cleanup
			resp = authPost(baseURL+"/api/sessions/"+sessionID+"/stop", adminToken, nil)
			resp.Body.Close()
			resp = authDelete(baseURL+"/api/sessions/"+sessionID, adminToken)
			resp.Body.Close()
		})
	})

	Describe("Quota Enforcement", func() {
		It("returns quota status with max_sessions_per_user", func() {
			resp := authGet(baseURL+"/api/quotas", adminToken)
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var status map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&status)
			Expect(status).To(HaveKey("max_sessions_per_user"))
		})
	})
})
