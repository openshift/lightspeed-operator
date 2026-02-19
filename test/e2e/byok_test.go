// byok_test.go contains end-to-end tests for the BYOK (Bring Your Own Knowledge) flow.
// It verifies that OLS can use customer-provided RAG images, that queries hit the BYOK
// index only when ByokRAGOnly is set, and that image updates (e.g. new tag/digest)
// propagate correctly to the app server deployment.

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	origImage1              = "docker://quay.io/openshift-lightspeed-test/assisted-installer-guide:2025-1"
	origImage2              = "docker://quay.io/openshift-lightspeed-test/assisted-installer-guide:2025-2"
	imageName               = "assisted-installer-guide"
	internalRegistyHostName = "image-registry.openshift-image-registry.svc:5000"
	imageTag                = "latest"
	imageNameAndTag         = imageName + ":" + imageTag
	dstUserName             = "builder"
	latestImageName         = internalRegistyHostName + "/" + utils.OLSNamespaceDefault + "/" + imageNameAndTag
)

// Test Design Notes:
// - Uses Ordered to ensure serial execution (critical for test isolation)
// - Tests Bring-Your-Own-Knowledge (BYOK) RAG functionality with custom vector database
// - Uses DeleteAndWait in cleanup to prevent resource pollution between test suites
// - FlakeAttempts(5) handles transient query timing and LLM response issues
var _ = Describe("BYOK", Ordered, Label("BYOK"), FlakeAttempts(5), func() {
	var env *OLSTestEnvironment
	var err error
	var registryDefaultRoute string
	var dstToken string

	// BeforeAll enables the internal image registry route, copies a RAG image into
	// the cluster registry, and sets up the OLS test environment with RAG and
	// ByokRAGOnly enabled so subsequent tests run against the BYOK index.
	BeforeAll(func() {
		By("Setting up OLS test environment with RAG configuration")
		ctx := context.Background()
		client, err := GetClient(nil)
		Expect(err).NotTo(HaveOccurred())

		err = EnableInternalImageRegistryRoute(client)
		Expect(err).NotTo(HaveOccurred())

		registryDefaultRoute, err = GetInternalImageRegistryRoute(client)
		Expect(err).NotTo(HaveOccurred())

		err = AddImageBuilderRole(client, utils.OLSNamespaceDefault, dstUserName)
		Expect(err).NotTo(HaveOccurred())

		dstToken, err = GetBuilderToken(client, utils.OLSNamespaceDefault, dstUserName)
		Expect(err).NotTo(HaveOccurred())

		_, err = CopyImageToRegistry(
			ctx,
			origImage1,
			registryDefaultRoute,
			utils.OLSNamespaceDefault,
			imageNameAndTag,
			"",
			"",
			dstUserName,
			dstToken,
			false,
			true,
			os.Stdout,
			15*time.Minute,
		)
		Expect(err).NotTo(HaveOccurred())

		env, err = SetupOLSTestEnvironment(func(cr *olsv1alpha1.OLSConfig) {
			cr.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{
				{
					Image: latestImageName,
				},
			}
			cr.Spec.OLSConfig.ByokRAGOnly = true
		}, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	// AfterAll tears down the OLS test environment and deletes the OLSConfig CR
	// so the cluster is left clean after the BYOK test run.
	AfterAll(func() {
		By("Cleaning up OLS test environment with CR deletion")
		err = CleanupOLSTestEnvironmentWithCRDeletion(env, "byok_test")
		Expect(err).NotTo(HaveOccurred())
	})

	// Verifies that when RAG is configured by image only (no IndexID), the CR
	// spec keeps IndexID empty so the OLS uses the only database in the BYOK image.
	It("should check that the default index ID is empty", FlakeAttempts(5), func() {
		olsConfig := &olsv1alpha1.OLSConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: OLSCRName,
			}}
		err := env.Client.Get(olsConfig)
		Expect(err).NotTo(HaveOccurred())
		Expect(olsConfig.Spec.OLSConfig.RAG[0].IndexID).To(BeEmpty())
	})

	// Ensures the OLS /v1/query endpoint answers using the BYOK RAG index by asking
	// a question whose answer is in the assisted-installer-guide content (CPU
	// architectures) and asserting the response includes the expected architectures.
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

	// With ByokRAGOnly true, OLS must not fall back to the default documentation
	// index. A query that would trigger "Related documentation" from the default
	// index must not contain that phrase when only the BYOK database is used.
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

	// Pushes a new BYOK image (different tag) to the internal registry and triggers
	// an ImageStream update; then checks that the app server deployment's init
	// container image is updated to the new digest, confirming BYOK image change
	// propagated and the OLS deployment is restarted.
	It("should check that BYOK image update propagates to the OLS", func() {
		appServerDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err := env.Client.Get(appServerDeployment)
		Expect(err).NotTo(HaveOccurred())
		oldGeneration := appServerDeployment.Generation

		By("Copying a BYOK image to the internal image registry")
		ctx := context.Background()
		digest, err := CopyImageToRegistry(
			ctx,
			origImage2,
			registryDefaultRoute,
			utils.OLSNamespaceDefault,
			imageNameAndTag,
			"",
			"",
			dstUserName,
			dstToken,
			false,
			true,
			os.Stdout,
			15*time.Minute,
		)
		Expect(err).NotTo(HaveOccurred())
		err = AddImageStreamImport(env.Client, utils.OLSNamespaceDefault, imageTag, latestImageName)
		Expect(err).NotTo(HaveOccurred())
		err = env.Client.WaitForDeploymentNextGeneration(appServerDeployment, oldGeneration)
		Expect(err).NotTo(HaveOccurred())

		err = env.Client.WaitForDeploymentRollout(appServerDeployment)
		Expect(err).NotTo(HaveOccurred())

		appServerDeployment = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = env.Client.Get(appServerDeployment)
		Expect(err).NotTo(HaveOccurred())
		Expect(appServerDeployment.Spec.Template.Spec.InitContainers[0].Image).To(
			Equal(internalRegistyHostName + "/" + utils.OLSNamespaceDefault + "/" + imageName + "@" + digest),
		)
	})
})
