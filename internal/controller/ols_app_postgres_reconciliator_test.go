package controller

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Postgres server reconciliator", Ordered, func() {

	Context("Creation logic", Ordered, func() {
		var secret, bootstrapSecret *corev1.Secret
		var sc *storagev1.StorageClass
		BeforeEach(func() {
			By("create the provider secret")
			secret, _ = generateRandomSecret()
			secret.Name = "lightspeed-postgres-secret"
			secret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID1",
					Name:       "lightspeed-postgres-secret",
				},
			})
			secretCreationErr := reconciler.Create(ctx, secret)
			Expect(secretCreationErr).NotTo(HaveOccurred())

			By("create the tls secret")
			tlsSecret, _ = generateRandomSecret()
			tlsSecret.Name = OLSCertsSecretName
			tlsSecret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       OLSCertsSecretName,
				},
			})
			secretCreationErr = reconciler.Create(ctx, tlsSecret)
			Expect(secretCreationErr).NotTo(HaveOccurred())

			By("create the bootstrap secret")
			bootstrapSecret, _ = generateRandomSecret()
			bootstrapSecret.Name = "lightspeed-bootstrap-secret"
			bootstrapSecret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID2",
					Name:       "lightspeed-bootstrap-secret",
				},
			})
			bootstrapSecretCreationErr := reconciler.Create(ctx, bootstrapSecret)
			Expect(bootstrapSecretCreationErr).NotTo(HaveOccurred())

			By("Creating default StorageClass")
			sc = buildDefaultStorageClass()
			storageClassCreationErr := reconciler.Create(ctx, sc)
			Expect(storageClassCreationErr).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			By("Deleting default StorageClass")
			storageClassDeletionErr := reconciler.Delete(ctx, sc)
			Expect(storageClassDeletionErr).NotTo(HaveOccurred())

			By("Delete the provider secret")
			secretDeletionErr := reconciler.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Delete the tls secret")
			secretDeletionErr = reconciler.Delete(ctx, tlsSecret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Delete the bootstrap secret")
			bootstrapSecretDeletionErr := reconciler.Delete(ctx, bootstrapSecret)
			Expect(bootstrapSecretDeletionErr).NotTo(HaveOccurred())
		})

		It("should reconcile from OLSConfig custom resource", func() {
			By("Reconcile the OLSConfig custom resource")
			cr.Spec.OLSConfig.ConversationCache.Postgres.CredentialsSecret = PostgresSecretName
			err := reconciler.reconcilePostgresServer(ctx, cr)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a service for lightspeed-postgres-server", func() {

			By("Get postgres service")
			svc := &corev1.Service{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: PostgresServiceName, Namespace: OLSNamespaceDefault}, svc)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a deployment lightspeed-postgres-server", func() {

			By("Get postgres deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: PostgresDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a postgres configmap", func() {

			By("Get the postgres config")
			configMap := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: PostgresConfigMap, Namespace: OLSNamespaceDefault}, configMap)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a postgres bootstrap secret", func() {

			By("Get the bootstrap secret")
			secret := &corev1.Secret{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: PostgresBootstrapSecretName, Namespace: OLSNamespaceDefault}, secret)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a postgres secret", func() {

			By("Get the postgres secret")
			secret := &corev1.Secret{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: PostgresSecretName, Namespace: OLSNamespaceDefault}, secret)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a postgres network policy", func() {
			By("Get the postgres network policy")
			networkPolicy := &networkingv1.NetworkPolicy{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: PostgresNetworkPolicyName, Namespace: OLSNamespaceDefault}, networkPolicy)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should trigger a rolling deployment when there is an update in secret name", func() {

			By("create the test secret")
			secret, _ = generateRandomSecret()
			secret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID1",
					Name:       "test-secret",
				},
			})
			secretCreationErr := reconciler.Create(ctx, secret)
			Expect(secretCreationErr).NotTo(HaveOccurred())

			By("Get the postgres deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: PostgresDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			oldHash := dep.Spec.Template.Annotations[PostgresConfigHashKey]

			By("Update the OLSConfig custom resource")
			olsConfig := &olsv1alpha1.OLSConfig{}
			err = k8sClient.Get(ctx, crNamespacedName, olsConfig)
			Expect(err).NotTo(HaveOccurred())
			olsConfig.Spec.OLSConfig.ConversationCache.Postgres.CredentialsSecret = "dummy-secret"

			By("Reconcile the app server")
			err = reconciler.reconcileAppServer(ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())
			By("Reconcile the postgres server")
			err = reconciler.reconcilePostgresServer(ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Get the postgres deployment")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: PostgresDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			fmt.Printf("%v", dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			Expect(dep.Annotations[PostgresConfigHashKey]).NotTo(Equal(oldHash))
		})
	})
})

func buildDefaultStorageClass() *storagev1.StorageClass {
	trueVal := true
	immediate := storagev1.VolumeBindingImmediate

	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "standard",
			Annotations: map[string]string{
				"storageclass.kubernetes.io/is-default-class": "true",
			},
		},
		Provisioner:          "kubernetes.io/no-provisioner",
		AllowVolumeExpansion: &trueVal,
		VolumeBindingMode:    &immediate,
	}
}
