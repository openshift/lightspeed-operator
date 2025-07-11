package e2e

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Rapidast", Ordered, Label("Rapidast"), func() {
	var env *OLSTestEnvironment
	var err error

	BeforeAll(func() {
		By("Setting up OLS test environment")
		env, err = SetupOLSTestEnvironment(nil)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		By("Cleaning up OLS test environment with CR deletion")
		err = CleanupOLSTestEnvironmentWithCRDeletion(env, "rapidast_test")
		Expect(err).NotTo(HaveOccurred())
	})

	It("should activate OLS on service HTTPS port", func() {
		By("Testing OLS service activation")
		secret, err := TestOLSServiceActivation(env)
		Expect(err).NotTo(HaveOccurred())

		By("Testing HTTPS POST on /v1/query endpoint by OLS user")
		reqBody := []byte(`{"query": "write a deployment yaml for the mongodb image"}`)
		resp, body, err := TestHTTPSQueryEndpoint(env, secret, reqBody)
		fmt.Println("httpsClient.PostJson", map[string]string{"Authorization": "Bearer " + env.SAToken})
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		fmt.Println(string(body))
		Expect(body).NotTo(BeEmpty())

		By("Creating a route for the OLS application")
		err = CreateOLSRoute(env.Client)
		Expect(err).NotTo(HaveOccurred())

		By("Updating ols-rapidast-config-updated.yaml with host and token")
		// Get console URL from Environment and create host URL
		consoleURL := os.Getenv("CONSOLE_URL")
		Expect(consoleURL).To(ContainSubstring(".apps"))
		index := strings.Index(consoleURL, ".apps")
		hostURL := OLSRouteName + consoleURL[index:]

		err = UpdateRapidastConfig(hostURL, env.SAToken)
		Expect(err).NotTo(HaveOccurred())
	})
})
