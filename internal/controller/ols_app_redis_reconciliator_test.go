package controller

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
			err := k8sClient.Get(ctx, types.NamespacedName{Name: RedisServiceName, Namespace: OLSNamespaceDefault}, svc)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a deployment lightspeed-redis-server", func() {

			By("Get redis deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: RedisDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a redis secret", func() {

			By("Get the secret")
			secret := &corev1.Secret{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: RedisSecretName, Namespace: OLSNamespaceDefault}, secret)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should trigger a rolling deployment when there is an update in secret name", func() {

			By("create the provider secret")
			secret, _ := generateRandomSecret()
			secret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       "test-secret",
				},
			})
			secretCreationErr := reconciler.Create(ctx, secret)
			Expect(secretCreationErr).NotTo(HaveOccurred())

			By("Get the redis deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: RedisDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			oldHash := dep.Spec.Template.Annotations[RedisConfigHashKey]

			By("Update the OLSConfig custom resource")
			olsConfig := &olsv1alpha1.OLSConfig{}
			err = k8sClient.Get(ctx, crNamespacedName, olsConfig)
			Expect(err).NotTo(HaveOccurred())
			olsConfig.Spec.OLSConfig.ConversationCache.Redis.CredentialsSecret = "dummy-secret"

			By("Reconcile the app server")
			err = reconciler.reconcileAppServer(ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())
			By("Reconcile the redis server")
			err = reconciler.reconcileRedisServer(ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Get the redis deployment")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: RedisDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			fmt.Printf("%v", dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			Expect(dep.Annotations[RedisConfigHashKey]).NotTo(Equal(oldHash))

			By("Delete the provider secret")
			secretDeletionErr := reconciler.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())
		})
	})
})
