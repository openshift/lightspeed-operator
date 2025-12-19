package e2e

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Test Design Notes:
// - Uses Ordered to ensure serial execution (critical for test isolation)
// - Tests operator upgrade scenario - CR persists across operator version changes
// - Uses DeleteAndWait in AfterAll to prevent resource pollution between test suites
// - CR is created before upgrade and should remain functional after upgrade
var _ = Describe("Upgrade operator tests", Ordered, Label("Upgrade"), func() {
	const testSAName = "test-sa"
	const queryAccessClusterRole = "lightspeed-operator-query-access"
	var cr *olsv1alpha1.OLSConfig
	var err error
	var client *Client
	var cleanUpFuncs []func()

	BeforeAll(func() {
		client, err = GetClient(nil)
		Expect(err).NotTo(HaveOccurred())
		By("Creating a OLSConfig CR")
		cr, err = generateOLSConfig()
		Expect(err).NotTo(HaveOccurred())
		err = client.Create(cr)
		Expect(err).NotTo(HaveOccurred())

		var cleanUp func()

		By("create a service account for OLS user")
		cleanUp, err := client.CreateServiceAccount(OLSNameSpace, testSAName)
		Expect(err).NotTo(HaveOccurred())
		cleanUpFuncs = append(cleanUpFuncs, cleanUp)

		By("create a role binding for OLS user accessing query API")
		cleanUp, err = client.CreateClusterRoleBinding(OLSNameSpace, testSAName, queryAccessClusterRole)
		Expect(err).NotTo(HaveOccurred())
		cleanUpFuncs = append(cleanUpFuncs, cleanUp)

		By("fetch the service account tokens")
		_, err = client.GetServiceAccountToken(OLSNameSpace, testSAName)
		Expect(err).NotTo(HaveOccurred())

		By("wait for application server deployment rollout")
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForDeploymentRollout(deployment)
		Expect(err).NotTo(HaveOccurred())

		cleanUpFuncs = append(cleanUpFuncs, cleanUp)
	})

	AfterAll(func() {
		err = mustGather("upgrade_test")
		Expect(err).NotTo(HaveOccurred())

		By("Deleting the OLSConfig CR and waiting for cleanup")
		if cr != nil {
			err = client.DeleteAndWait(cr, 2*time.Minute)
			Expect(err).NotTo(HaveOccurred())
		}

		for _, cleanUpFunc := range cleanUpFuncs {
			cleanUpFunc()
		}
	})

	It("should continue working after operator upgrade", func() {
		By("Wait for the application service created")
		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerServiceName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForServiceCreated(service)
		Expect(err).NotTo(HaveOccurred())

		err = client.UpgradeOperator(OLSNameSpace)
		Expect(err).NotTo(HaveOccurred())

		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}

		err = client.WaitForDeploymentRollout(deployment)
		Expect(err).NotTo(HaveOccurred())

	})
})
