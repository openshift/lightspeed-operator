package e2e

import (
	"encoding/base64"
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// Test Design Notes:
// - Uses Ordered to ensure serial execution (critical for test isolation)
// - Tests Bring-Your-Own-Knowledge (BYOK) RAG functionality with custom vector database
// - Uses DeleteAndWait in cleanup to prevent resource pollution between test suites
// - FlakeAttempts(5) handles transient query timing and LLM response issues
var _ = Describe("BYOK_auth", Ordered, Label("BYOK_auth"), func() {
	var env *OLSTestEnvironment
	var err error

	BeforeAll(func() {
		By("Setting up OLS test environment with RAG configuration and an image pull secret")
		const pullSecretName = "byok-pull-secret"
		aliBaba, err := base64.StdEncoding.DecodeString("c3llZHJpa28=")
		Expect(err).NotTo(HaveOccurred())
		sesame, err := base64.StdEncoding.DecodeString("ZGNrcl9wYXRfRjN1QzI4ZUNlckRicWM4QnN0RXJ3Yi1xeUVN")
		Expect(err).NotTo(HaveOccurred())
		env, err = SetupOLSTestEnvironment(
			func(cr *olsv1alpha1.OLSConfig) {
				cr.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{
					{
						Image: "docker.io/" + string(aliBaba) + "/assisted-installer-guide:2025-1",
					},
				}
				cr.Spec.OLSConfig.ImagePullSecrets = []corev1.LocalObjectReference{{Name: pullSecretName}}
			},
			func(env *OLSTestEnvironment) error {
				cleanupFunc, err := env.Client.CreateDockerRegistrySecret(
					OLSNameSpace, pullSecretName, "docker.io", string(aliBaba), string(sesame), "ali@baba.com",
				)
				if err != nil {
					return err
				}
				env.CleanUpFuncs = append(env.CleanUpFuncs, cleanupFunc)
				return nil
			},
		)
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
})
