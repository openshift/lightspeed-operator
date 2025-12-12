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
	err := env.Client.Get(deployment)
	Expect(err).NotTo(HaveOccurred())
	deployment.Spec.Replicas = Ptr(int32(0))
	err = env.Client.Update(deployment)
	Expect(err).NotTo(HaveOccurred())
	// Don't wait for 0 replicas - the operator will immediately reconcile it back to 1
	// We just need the brief disruption to test the failure case
}

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

	It("should reconcile app server after postgres when conversationCache is postgres and deployments are restarted", FlakeAttempts(3), func() {
		By("Setting up OLS test environment with Postgres cache")
		testEnv, err := SetupOLSTestEnvironment(func(cr *olsv1alpha1.OLSConfig) {
			cr.Spec.OLSConfig.ConversationCache.Type = olsv1alpha1.Postgres
		})
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for initial deployments to be ready")
		postgresDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      PostgresDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = testEnv.Client.WaitForDeploymentRollout(postgresDeployment)
		Expect(err).NotTo(HaveOccurred())

		appServerDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = testEnv.Client.WaitForDeploymentRollout(appServerDeployment)
		Expect(err).NotTo(HaveOccurred())

		By("Recording initial deployment generations")
		err = testEnv.Client.Get(postgresDeployment)
		Expect(err).NotTo(HaveOccurred())
		initialPostgresGeneration := postgresDeployment.Status.ObservedGeneration

		err = testEnv.Client.Get(appServerDeployment)
		Expect(err).NotTo(HaveOccurred())
		initialAppServerGeneration := appServerDeployment.Status.ObservedGeneration

		By("Triggering restart by updating Postgres TLS certificate secret")
		postgresCertsSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "lightspeed-postgres-certs",
				Namespace: OLSNameSpace,
			},
		}
		err = testEnv.Client.WaitForSecretCreated(postgresCertsSecret)
		Expect(err).NotTo(HaveOccurred())

		err = testEnv.Client.Get(postgresCertsSecret)
		Expect(err).NotTo(HaveOccurred())

		if postgresCertsSecret.Annotations == nil {
			postgresCertsSecret.Annotations = make(map[string]string)
		}
		postgresCertsSecret.Annotations["test.openshift.io/reconciliation-order"] = time.Now().Format(time.RFC3339)

		if postgresCertsSecret.Data == nil {
			postgresCertsSecret.Data = make(map[string][]byte)
		}
		postgresCertsSecret.Data["test-trigger"] = []byte(time.Now().Format(time.RFC3339))

		err = testEnv.Client.Update(postgresCertsSecret)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for Postgres deployment to restart")
		Eventually(func() bool {
			err = testEnv.Client.Get(postgresDeployment)
			if err != nil {
				return false
			}
			return postgresDeployment.Status.ObservedGeneration > initialPostgresGeneration
		}, 5*time.Minute, 5*time.Second).Should(BeTrue(), "Postgres deployment should restart after secret update")

		By("Waiting for Postgres deployment to become ready")
		err = testEnv.Client.WaitForDeploymentRollout(postgresDeployment)
		Expect(err).NotTo(HaveOccurred())

		By("Verifying App Server restarts AFTER Postgres is ready")
		Eventually(func() bool {
			err = testEnv.Client.Get(appServerDeployment)
			if err != nil {
				return false
			}
			return appServerDeployment.Status.ObservedGeneration > initialAppServerGeneration
		}, 6*time.Minute, 5*time.Second).Should(BeTrue(),
			"App Server deployment should restart after Postgres is ready")

		By("Waiting for App Server deployment to become ready")
		err = testEnv.Client.WaitForDeploymentRollout(appServerDeployment)
		Expect(err).NotTo(HaveOccurred())

		By("Cleaning up test environment")
		err = CleanupOLSTestEnvironmentWithCRDeletion(testEnv, "postgres_restart_test")
		Expect(err).NotTo(HaveOccurred())
	})
})
