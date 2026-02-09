package e2e

import (
	"net/http"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	defaultBaseURL = "http://localhost:18080"
	adminUsername  = "admin"
	adminPassword  = "admin123"
)

var (
	baseURL    string
	adminToken string
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2E Suite")
}

var _ = BeforeSuite(func() {
	baseURL = os.Getenv("E2E_BASE_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	// Wait for readyz
	Eventually(func() int {
		resp, err := http.Get(baseURL + "/readyz")
		if err != nil {
			return 0
		}
		resp.Body.Close()
		return resp.StatusCode
	}).WithTimeout(60 * time.Second).WithPolling(2 * time.Second).Should(Equal(http.StatusOK))

	// Login as admin
	var err error
	adminToken, err = login(baseURL, adminUsername, adminPassword)
	Expect(err).NotTo(HaveOccurred())
	Expect(adminToken).NotTo(BeEmpty())
})
