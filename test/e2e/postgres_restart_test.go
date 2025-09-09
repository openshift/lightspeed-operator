package e2e

import (
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func invokeOLS(env *OLSTestEnvironment, secret *corev1.Secret, query string, expected_success bool) {
	reqBody := []byte(`{"query": "` + query + `"}`)
	Eventually(func() bool {
		resp, body, err := TestHTTPSQueryEndpoint(env, secret, reqBody)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		fmt.Println(GinkgoWriter, string(body))
		return resp.StatusCode == http.StatusOK
	}, 5*time.Minute, 5*time.Second).Should(Equal(expected_success))
}

func waitForPodsToDisappear(env *OLSTestEnvironment, namespace, labelKey, labelValue string) {
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels{labelKey: labelValue},
	}
	Eventually(func() error {
		var podList corev1.PodList
		if err := env.Client.List(&podList, listOpts...); err != nil {
			return err
		}
		if len(podList.Items) == 0 {
			return nil
		}
		for _, pod := range podList.Items {
			fmt.Fprintf(GinkgoWriter, "Pod %s phase: %s\n", pod.Name, pod.Status.Phase)
		}
		return fmt.Errorf(
			"waiting for deletion of the pods with label key %s and value %s in namespace %s",
			labelKey, labelValue, namespace)
	}, 5*time.Minute, 5*time.Second).Should(BeNil())
}

func shutdownPostgres(env *OLSTestEnvironment) {
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
	err = env.Client.WaitForDeploymentCondition(deployment, func(dep *appsv1.Deployment) (bool, error) {
		fmt.Println(GinkgoWriter, dep.Status)
		if dep.Status.AvailableReplicas > 0 {
			return false, fmt.Errorf("got %d available replicas", dep.Status.AvailableReplicas)
		}
		return true, nil
	})
	Expect(err).NotTo(HaveOccurred())

	waitForPodsToDisappear(env, OLSNameSpace, "app.kubernetes.io/component", "postgres-server")
}

func startPostgres(env *OLSTestEnvironment) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PostgresDeploymentName,
			Namespace: OLSNameSpace,
		},
	}
	err := env.Client.Get(deployment)
	Expect(err).NotTo(HaveOccurred())
	deployment.Spec.Replicas = Ptr(int32(1))
	err = env.Client.Update(deployment)
	Expect(err).NotTo(HaveOccurred())
	err = env.Client.WaitForDeploymentRollout(deployment)
	Expect(err).NotTo(HaveOccurred())
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

	It("should bounce Postgres and reestablish connection with it", func() {
		By("Testing OLS service activation")
		secret, err := TestOLSServiceActivation(env)
		Expect(err).NotTo(HaveOccurred())

		By("Testing HTTPS POST on /v1/query endpoint by OLS user - should pass")
		invokeOLS(env, secret, "how do I stop a VM?", true)

		By("shut down Postgres")
		shutdownPostgres(env)

		By("Testing HTTPS POST on /v1/query endpoint by OLS user - should fail")
		invokeOLS(env, secret, "how do I stop a VM?", false)

		By("bring Postgres back up")
		startPostgres(env)

		By("Testing HTTPS POST on /v1/query endpoint by OLS user - should pass")
		invokeOLS(env, secret, "how do I stop a VM?", true)
	})
})
