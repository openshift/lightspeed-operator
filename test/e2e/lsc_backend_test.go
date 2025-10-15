package e2e

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Reconciliation From OLSConfig CR", Label("LSCBackend"), Ordered, func() {
	var cr *olsv1alpha1.OLSConfig
	var err error
	var client *Client

	BeforeAll(func() {
		client, err = GetClient(nil)
		Expect(err).NotTo(HaveOccurred())
		By("Creating a OLSConfig CR")
		cr, err = generateOLSConfig()
		Expect(err).NotTo(HaveOccurred())
		err = client.Create(cr)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		client, err = GetClient(nil)
		Expect(err).NotTo(HaveOccurred())
		err = mustGather("reconciliation_test")
		Expect(err).NotTo(HaveOccurred())
		By("Deleting the OLSConfig CR")
		Expect(cr).NotTo(BeNil())
		err = client.Delete(cr)
		Expect(err).NotTo(HaveOccurred())

	})

	It("should setup LSC backend", func() {
		By("Verify the LSC backend deployment running")
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForDeploymentRollout(deployment)
		Expect(err).NotTo(HaveOccurred())

		By("Verify the LSC backend service created")
		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerServiceName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForServiceCreated(service)
		Expect(err).NotTo(HaveOccurred())
	})

	// todo: add test according to the LSC backend implementation details
})
