package e2e

import (
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Test Design Notes:
// - Uses Ordered to ensure serial execution (critical for test isolation)
// - Tests Bring-Your-Own-Knowledge (BYOK) RAG functionality with custom vector database
// - Uses DeleteAndWait in cleanup to prevent resource pollution between test suites
// - FlakeAttempts(5) handles transient query timing and LLM response issues
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

	It("should check that the default index ID is empty", FlakeAttempts(5), func() {
		olsConfig := &olsv1alpha1.OLSConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: OLSCRName,
			}}
		err := env.Client.Get(olsConfig)
		Expect(err).NotTo(HaveOccurred())
		Expect(olsConfig.Spec.OLSConfig.RAG[0].IndexID).To(BeEmpty())
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
