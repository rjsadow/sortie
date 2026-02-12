package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func createE2EUser(username, password string, roles []string) string {
	body, _ := json.Marshal(map[string]interface{}{
		"username": username,
		"password": password,
		"email":    username + "@e2e.test",
		"roles":    roles,
	})
	resp := authPost(baseURL+"/api/admin/users", adminToken, body)
	defer resp.Body.Close()
	Expect(resp.StatusCode).To(Equal(http.StatusCreated), "create user %s", username)

	var result struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.ID
}

func deleteE2EUser(id string) {
	resp := authDelete(baseURL+"/api/admin/users/"+id, adminToken)
	resp.Body.Close()
}

func createE2ECategory(id, name string) {
	body := []byte(fmt.Sprintf(`{"id":%q,"name":%q}`, id, name))
	resp := authPost(baseURL+"/api/categories", adminToken, body)
	resp.Body.Close()
	Expect(resp.StatusCode).To(SatisfyAny(
		Equal(http.StatusCreated),
		Equal(http.StatusConflict),
	), "create category %s", name)
}

func deleteE2ECategory(id string) {
	resp := authDelete(baseURL+"/api/categories/"+id, adminToken)
	resp.Body.Close()
}

var _ = Describe("Category Access Control", func() {

	Describe("Category visibility for app access", func() {
		It("enforces app-level visibility for app access", func() {
			// 1. Admin creates categories
			createE2ECategory("e2e-cat-pub", "E2E Public")
			DeferCleanup(deleteE2ECategory, "e2e-cat-pub")

			createE2ECategory("e2e-cat-appr", "E2E Approved")
			DeferCleanup(deleteE2ECategory, "e2e-cat-appr")

			createE2ECategory("e2e-cat-admin", "E2E AdminOnly")
			DeferCleanup(deleteE2ECategory, "e2e-cat-admin")

			// 2. Admin creates apps with visibility on the app
			for _, c := range []struct{ id, cat, visibility string }{
				{"e2e-vis-pub", "E2E Public", "public"},
				{"e2e-vis-appr", "E2E Approved", "approved"},
				{"e2e-vis-admin", "E2E AdminOnly", "admin_only"},
			} {
				body := []byte(fmt.Sprintf(`{
					"id": %q,
					"name": "App %s",
					"url": "https://example.com",
					"launch_type": "url",
					"category": %q,
					"visibility": %q
				}`, c.id, c.id, c.cat, c.visibility))
				resp := authPost(baseURL+"/api/apps", adminToken, body)
				resp.Body.Close()
				Expect(resp.StatusCode).To(SatisfyAny(
					Equal(http.StatusCreated),
					Equal(http.StatusConflict),
				))
				DeferCleanup(deleteE2EApp, c.id)
			}

			// 3. Create regular user, login
			userID := createE2EUser("e2e-catuser", "pass1234", []string{"user"})
			DeferCleanup(deleteE2EUser, userID)

			userToken, err := login(baseURL, "e2e-catuser", "pass1234")
			Expect(err).NotTo(HaveOccurred())

			// 4. Verify user only sees apps in public category
			resp := authGet(baseURL+"/api/apps", userToken)
			var apps []map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&apps)
			resp.Body.Close()

			appIDs := []string{}
			for _, a := range apps {
				appIDs = append(appIDs, a["id"].(string))
			}
			Expect(appIDs).To(ContainElement("e2e-vis-pub"))
			Expect(appIDs).NotTo(ContainElement("e2e-vis-appr"))
			Expect(appIDs).NotTo(ContainElement("e2e-vis-admin"))

			// 5. Admin approves user for the approved category
			addBody := []byte(fmt.Sprintf(`{"user_id":%q}`, userID))
			resp = authPost(baseURL+"/api/categories/e2e-cat-appr/approved-users", adminToken, addBody)
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			// 6. Verify user now sees apps in public + approved categories
			resp = authGet(baseURL+"/api/apps", userToken)
			json.NewDecoder(resp.Body).Decode(&apps)
			resp.Body.Close()

			appIDs = []string{}
			for _, a := range apps {
				appIDs = append(appIDs, a["id"].(string))
			}
			Expect(appIDs).To(ContainElement("e2e-vis-pub"))
			Expect(appIDs).To(ContainElement("e2e-vis-appr"))
			Expect(appIDs).NotTo(ContainElement("e2e-vis-admin"))
		})
	})

	Describe("Category admin app management", func() {
		It("allows category admins to manage apps in their categories", func() {
			// 1. Admin creates category, assigns user as category admin
			createE2ECategory("e2e-cat-mgmt", "E2E Managed")
			DeferCleanup(deleteE2ECategory, "e2e-cat-mgmt")

			createE2ECategory("e2e-cat-other", "E2E Other")
			DeferCleanup(deleteE2ECategory, "e2e-cat-other")

			userID := createE2EUser("e2e-catadmin", "pass1234", []string{"user"})
			DeferCleanup(deleteE2EUser, userID)

			addBody := []byte(fmt.Sprintf(`{"user_id":%q}`, userID))
			resp := authPost(baseURL+"/api/categories/e2e-cat-mgmt/admins", adminToken, addBody)
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			userToken, err := login(baseURL, "e2e-catadmin", "pass1234")
			Expect(err).NotTo(HaveOccurred())

			// 2. Category admin creates an app in their category (should succeed)
			appBody := []byte(`{
				"id": "e2e-catadmin-app",
				"name": "CatAdmin App",
				"url": "https://example.com",
				"launch_type": "url",
				"category": "E2E Managed"
			}`)
			resp = authPost(baseURL+"/api/apps", userToken, appBody)
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))
			DeferCleanup(deleteE2EApp, "e2e-catadmin-app")

			// 3. Category admin tries to create app in another category (should fail)
			otherBody := []byte(`{
				"id": "e2e-catadmin-other",
				"name": "Other App",
				"url": "https://example.com",
				"launch_type": "url",
				"category": "E2E Other"
			}`)
			resp = authPost(baseURL+"/api/apps", userToken, otherBody)
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
		})
	})
})
