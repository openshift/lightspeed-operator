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

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
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
		env, err = SetupOLSTestEnvironment(nil, nil)
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

	It("should wait for postgres to be ready before starting app server when conversationCache is postgres", FlakeAttempts(3), func() {
		By("Verifying conversation cache type is Postgres")
		olsConfig := &olsv1alpha1.OLSConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: OLSCRName,
			},
		}
		err = env.Client.Get(olsConfig)
		Expect(err).NotTo(HaveOccurred())
		if olsConfig.Spec.OLSConfig.ConversationCache.Type != olsv1alpha1.Postgres {
			Skip("Skipping test - conversation cache type is not Postgres")
		}

		postgresDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      PostgresDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		appServerDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}

		By("Recording initial app server generation")
		err = env.Client.Get(appServerDeployment)
		Expect(err).NotTo(HaveOccurred())
		initialAppServerGeneration := appServerDeployment.Generation

		By("Scaling down Postgres to trigger a restart")
		shutdownPostgres(env)

		By("Waiting for Postgres to reconcile back to ready state")
		Eventually(func() bool {
			err = env.Client.Get(postgresDeployment)
			if err != nil {
				return false
			}
			return postgresDeployment.Status.ReadyReplicas == *postgresDeployment.Spec.Replicas &&
				postgresDeployment.Status.UpdatedReplicas == *postgresDeployment.Spec.Replicas
		}, 5*time.Minute, 5*time.Second).Should(BeTrue(), "Postgres should become ready")

		By("Verifying App Server waits for Postgres before becoming ready")
		// App Server should only become ready after Postgres is ready
		Eventually(func() bool {
			err = env.Client.Get(appServerDeployment)
			if err != nil {
				return false
			}
			// Check if app server has been updated/restarted
			appServerUpdated := appServerDeployment.Generation > initialAppServerGeneration ||
				appServerDeployment.Status.UpdatedReplicas == *appServerDeployment.Spec.Replicas

			// Verify postgres is ready
			err = env.Client.Get(postgresDeployment)
			if err != nil {
				return false
			}
			postgresReady := postgresDeployment.Status.ReadyReplicas == *postgresDeployment.Spec.Replicas

			// App server should only be ready if postgres is ready
			if appServerUpdated {
				Expect(postgresReady).To(BeTrue(), "Postgres must be ready before App Server becomes ready")
			}

			return appServerDeployment.Status.ReadyReplicas == *appServerDeployment.Spec.Replicas
		}, 8*time.Minute, 5*time.Second).Should(BeTrue(), "App Server should become ready after Postgres is ready")

		By("Verifying both deployments are fully operational")
		err = env.Client.Get(postgresDeployment)
		Expect(err).NotTo(HaveOccurred())
		Expect(postgresDeployment.Status.ReadyReplicas).To(Equal(*postgresDeployment.Spec.Replicas),
			"Postgres should have all replicas ready")

		err = env.Client.Get(appServerDeployment)
		Expect(err).NotTo(HaveOccurred())
		Expect(appServerDeployment.Status.ReadyReplicas).To(Equal(*appServerDeployment.Spec.Replicas),
			"App Server should have all replicas ready")
	})
})
