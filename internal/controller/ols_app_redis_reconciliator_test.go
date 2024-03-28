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

var caConfigMap *corev1.ConfigMap
var apptlsSecret *corev1.Secret
var _ = Describe("Redis server reconciliator", Ordered, func() {

	Context("Creation logic", Ordered, func() {

		BeforeEach(func() {
			By("create the CA configmap")
			caConfigMap, _ = generateRandomConfigMap()
			caConfigMap.Name = RedisCAConfigMap
			caConfigMap.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "ConfigMap",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       RedisCAConfigMap,
				},
			})
			configMapCreationErr := reconciler.Create(ctx, caConfigMap)
			Expect(configMapCreationErr).NotTo(HaveOccurred())
			By("create the tls secret")
			tlsSecret, _ = generateRandomSecret()
			tlsSecret.Name = RedisCertsSecretName
			tlsSecret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       RedisCertsSecretName,
				},
			})
			secretCreationErr := reconciler.Create(ctx, tlsSecret)
			Expect(secretCreationErr).NotTo(HaveOccurred())
			By("create the app tls secret")
			apptlsSecret, _ = generateRandomSecret()
			apptlsSecret.Name = OLSCertsSecretName
			apptlsSecret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       OLSCertsSecretName,
				},
			})
			appsecretCreationErr := reconciler.Create(ctx, apptlsSecret)
			Expect(appsecretCreationErr).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			By("Delete the CA configmap")
			configMapDeletionErr := reconciler.Delete(ctx, caConfigMap)
			Expect(configMapDeletionErr).NotTo(HaveOccurred())
			By("Delete the tls secret")
			secretDeletionErr := reconciler.Delete(ctx, tlsSecret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())
			By("Delete the app tls secret")
			appsecretDeletionErr := reconciler.Delete(ctx, apptlsSecret)
			Expect(appsecretDeletionErr).NotTo(HaveOccurred())
		})

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
		})

		It("should trigger a rolling deployment when there is an update in tls secret content", func() {

			By("Get the redis deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: RedisDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			oldHash := dep.Spec.Template.Annotations[RedisTLSHashKey]

			By("Update the redis tls secret content")
			tlsSecret.Data["tls.key"] = []byte("new-value")
			err = k8sClient.Update(ctx, tlsSecret)
			Expect(err).NotTo(HaveOccurred())

			By("Update the OLSConfig custom resource")
			olsConfig := &olsv1alpha1.OLSConfig{}
			err = k8sClient.Get(ctx, crNamespacedName, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile the app server")
			err = reconciler.reconcileAppServer(ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())
			By("Reconcile the redis server")
			err = reconciler.reconcileRedisServer(ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Get the redis deployment")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: RedisDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			Expect(dep.Annotations[RedisTLSHashKey]).NotTo(Equal(oldHash))
		})

		It("should trigger a rolling deployment when tls secret is recreated", func() {

			By("Get the redis deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: RedisDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			oldHash := dep.Spec.Template.Annotations[RedisTLSHashKey]

			By("Delete the tls secret")
			secretDeletionErr := reconciler.Delete(ctx, tlsSecret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Recreate the tls secret")
			tlsSecret, _ = generateRandomSecret()
			tlsSecret.Name = RedisCertsSecretName
			tlsSecret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       RedisCertsSecretName,
				},
			})
			secretCreationErr := reconciler.Create(ctx, tlsSecret)
			Expect(secretCreationErr).NotTo(HaveOccurred())

			By("Update the OLSConfig custom resource")
			olsConfig := &olsv1alpha1.OLSConfig{}
			err = k8sClient.Get(ctx, crNamespacedName, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile the app server")
			err = reconciler.reconcileAppServer(ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())
			By("Reconcile the redis server")
			err = reconciler.reconcileRedisServer(ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Get the redis deployment")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: RedisDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			Expect(dep.Annotations[RedisTLSHashKey]).NotTo(Equal(oldHash))
		})

		It("should trigger a rolling deployment when there is an update in the openshift CA certs", func() {

			By("Get the redis deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: RedisDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			oldHash := dep.Spec.Template.Annotations[RedisCAHashKey]

			By("Update the redis CA configmap content")
			caConfigMap.Data["service-ca.crt"] = "new-value"
			err = k8sClient.Update(ctx, caConfigMap)
			Expect(err).NotTo(HaveOccurred())

			By("Update the OLSConfig custom resource")
			olsConfig := &olsv1alpha1.OLSConfig{}
			err = k8sClient.Get(ctx, crNamespacedName, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile the app server")
			err = reconciler.reconcileAppServer(ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())
			By("Reconcile the redis server")
			err = reconciler.reconcileRedisServer(ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Get the redis deployment")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: RedisDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			Expect(dep.Annotations[RedisCAHashKey]).NotTo(Equal(oldHash))
		})

		It("should trigger a rolling deployment when openshift CA certs are regenerated", func() {

			By("Get the redis deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: RedisDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			oldHash := dep.Spec.Template.Annotations[RedisCAHashKey]

			By("Delete the CA configmap")
			caConfigMapDeletionErr := reconciler.Delete(ctx, caConfigMap)
			Expect(caConfigMapDeletionErr).NotTo(HaveOccurred())

			By("Recreate the CA configmap")
			caConfigMap, _ = generateRandomConfigMap()
			caConfigMap.Name = RedisCAConfigMap
			caConfigMap.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "ConfigMap",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       RedisCAConfigMap,
				},
			})
			configMapCreationErr := reconciler.Create(ctx, caConfigMap)
			Expect(configMapCreationErr).NotTo(HaveOccurred())

			By("Update the OLSConfig custom resource")
			olsConfig := &olsv1alpha1.OLSConfig{}
			err = k8sClient.Get(ctx, crNamespacedName, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile the app server")
			err = reconciler.reconcileAppServer(ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())
			By("Reconcile the redis server")
			err = reconciler.reconcileRedisServer(ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Get the redis deployment")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: RedisDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			Expect(dep.Annotations[RedisCAHashKey]).NotTo(Equal(oldHash))
		})
	})
})
