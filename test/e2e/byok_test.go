package e2e

import (
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

var _ = Describe("BYOK", Ordered, Label("BYOK"), func() {
	var env *OLSTestEnvironment
	var err error

	BeforeAll(func() {
		By("Setting up OLS test environment with RAG configuration")
		env, err = SetupOLSTestEnvironment(func(cr *olsv1alpha1.OLSConfig) {
			cr.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{
				{
					Image: "quay.io/openshift-lightspeed-test/assisted-installer-guide:2025-1",
				},
			}
			cr.Spec.OLSConfig.ByokRAGOnly = true
		})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		By("Cleaning up OLS test environment with CR deletion")
		err = CleanupOLSTestEnvironmentWithCRDeletion(env, "byok_test")
		Expect(err).NotTo(HaveOccurred())
	})

	It("should query the BYOK database", FlakeAttempts(5), func() {
		By("Testing OLS service activation")
		secret, err := TestOLSServiceActivation(env)
		Expect(err).NotTo(HaveOccurred())

		By("Testing HTTPS POST on /v1/query endpoint by OLS user")
		reqBody := []byte(`{"query": "what CPU architectures does the assisted installer support?"}`)
		resp, body, err := TestHTTPSQueryEndpoint(env, secret, reqBody)
		CheckErrorAndRestartPortForwardingTestEnvironment(env, err)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		fmt.Println(string(body))

		Expect(string(body)).To(
			And(
				ContainSubstring("x86_64"),
				ContainSubstring("arm64"),
				ContainSubstring("ppc64le"),
				ContainSubstring("s390x"),
			),
		)
	})

	It("should only query the BYOK database", func() {
		By("Testing OLS service activation")
		secret, err := TestOLSServiceActivation(env)
		Expect(err).NotTo(HaveOccurred())

		By("Testing HTTPS POST on /v1/query endpoint by OLS user")
		reqBody := []byte(`{"query": "how do I stop a VM?"}`)
		resp, body, err := TestHTTPSQueryEndpoint(env, secret, reqBody)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		fmt.Println(string(body))

		Expect(string(body)).NotTo(ContainSubstring("Related documentation"))
	})
})
