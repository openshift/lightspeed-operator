package e2e

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func invokeOLS(env *OLSTestEnvironment, secret *corev1.Secret, query string, expected_success bool) {
	reqBody := []byte(`{"query": "` + query + `"}`)
	Eventually(func() bool {
		resp, body, err := TestHTTPSQueryEndpoint(env, secret, reqBody)
		CheckErrorAndRestartPortForwardingTestEnvironment(env, err)
		if err != nil && strings.Contains(err.Error(), "EOF") {
			// retry in next iteration after port forwarding restarts
			return !expected_success
		}
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		fmt.Println(GinkgoWriter, string(body))
		return resp.StatusCode == http.StatusOK
	}, 5*time.Minute, 5*time.Second).Should(Equal(expected_success))
}

func shutdownPostgres(env *OLSTestEnvironment) {
	// Scale Postgres to 0 to simulate a brief outage
	// The operator will quickly reconcile it back to 1, but there's a brief window
	// where queries will fail, which is what we're testing
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PostgresDeploymentName,
			Namespace: OLSNameSpace,
		},
	}
	err := env.Client.Update(deployment, func(obj ctrlclient.Object) error {
		dep := obj.(*appsv1.Deployment)
		dep.Spec.Replicas = Ptr(int32(0))
		return nil
	})
	Expect(err).NotTo(HaveOccurred())
	// Don't wait for 0 replicas - the operator will immediately reconcile it back to 1
	// We just need the brief disruption to test the failure case
}

// Test Design Notes:
// - Uses Ordered to ensure serial execution (critical for test isolation)
// - Tests that OLS gracefully handles Postgres outages and automatic recovery
// - Uses DeleteAndWait in cleanup to prevent resource pollution between test suites
// - FlakeAttempts(5) handles timing issues with pod restarts
var _ = Describe("Postgres restart", Ordered, Label("Postgres restart"), func() {
	var env *OLSTestEnvironment
	var err error

	BeforeAll(func() {
		By("Setting up OLS test environment")
		env, err = SetupOLSTestEnvironment(nil)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		By("Cleaning up OLS test environment with CR deletion")
		err = CleanupOLSTestEnvironmentWithCRDeletion(env, "postgres_restart_test")
		Expect(err).NotTo(HaveOccurred())
	})

	It("should automatically recover when Postgres is scaled down", FlakeAttempts(5), func() {
		By("Testing OLS service activation")
		secret, err := TestOLSServiceActivation(env)
		Expect(err).NotTo(HaveOccurred())

		By("Testing HTTPS POST on /v1/query endpoint by OLS user - baseline")
		invokeOLS(env, secret, "how do I stop a VM?", true)

		By("Scaling down Postgres to simulate a crash")
		shutdownPostgres(env)

		By("Waiting for the operator to automatically bring Postgres back up")
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      PostgresDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = env.Client.WaitForDeploymentRollout(deployment)
		Expect(err).NotTo(HaveOccurred())

		By("Testing HTTPS POST on /v1/query endpoint by OLS user - should work after recovery")
		invokeOLS(env, secret, "how do I stop a VM?", true)
	})
})
