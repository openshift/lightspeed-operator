package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Redis server reconciliator", Ordered, func() {

	Context("Creation logic", Ordered, func() {

		It("should reconcile from OLSConfig custom resource", func() {
			By("Reconcile the OLSConfig custom resource")
			err := reconciler.reconcileRedisServer(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

		})

		It("should create a service for lightspeed-redis-server", func() {

			By("Get redis service")
			svc := &corev1.Service{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppRedisServiceName, Namespace: cr.Namespace}, svc)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a deployment lightspeed-redis-server", func() {

			By("Get redis deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppRedisDeploymentName, Namespace: cr.Namespace}, dep)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
