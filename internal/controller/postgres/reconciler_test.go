package postgres

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"

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
		var configmap *corev1.ConfigMap
		BeforeEach(func() {
			By("create the provider secret")
			secret, _ = utils.GenerateRandomSecret()
			secret.Name = utils.PostgresSecretName
			secret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID1",
					Name:       utils.PostgresSecretName,
				},
			})
			secretCreationErr := testReconcilerInstance.Create(ctx, secret)
			Expect(secretCreationErr).NotTo(HaveOccurred())

			By("create the tls secret")
			tlsSecret, _ = utils.GenerateRandomSecret()
			tlsSecret.Name = utils.OLSCertsSecretName
			tlsSecret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       utils.OLSCertsSecretName,
				},
			})
			secretCreationErr = testReconcilerInstance.Create(ctx, tlsSecret)
			Expect(secretCreationErr).NotTo(HaveOccurred())

			By("create the bootstrap secret")
			bootstrapSecret, _ = utils.GenerateRandomSecret()
			bootstrapSecret.Name = "lightspeed-bootstrap-secret"
			bootstrapSecret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID2",
					Name:       "lightspeed-bootstrap-secret",
				},
			})
			bootstrapSecretCreationErr := testReconcilerInstance.Create(ctx, bootstrapSecret)
			Expect(bootstrapSecretCreationErr).NotTo(HaveOccurred())

			By("Creating default StorageClass")
			sc = utils.BuildDefaultStorageClass()
			storageClassCreationErr := testReconcilerInstance.Create(ctx, sc)
			Expect(storageClassCreationErr).NotTo(HaveOccurred())

			By("create the OpenShift certificates config map")
			configmap, _ = utils.GenerateRandomConfigMap()
			configmap.Name = utils.DefaultOpenShiftCerts
			configmap.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Configmap",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       utils.DefaultOpenShiftCerts,
				},
			})
			configMapCreationErr := testReconcilerInstance.Create(ctx, configmap)
			Expect(configMapCreationErr).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			By("Deleting default StorageClass")
			storageClassDeletionErr := testReconcilerInstance.Delete(ctx, sc)
			Expect(storageClassDeletionErr).NotTo(HaveOccurred())

			By("Delete the provider secret")
			secretDeletionErr := testReconcilerInstance.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Delete the tls secret")
			secretDeletionErr = testReconcilerInstance.Delete(ctx, tlsSecret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Delete the bootstrap secret")
			bootstrapSecretDeletionErr := testReconcilerInstance.Delete(ctx, bootstrapSecret)
			Expect(bootstrapSecretDeletionErr).NotTo(HaveOccurred())

			By("Delete OpenShift certificates config map")
			configMapDeletionErr := testReconcilerInstance.Delete(ctx, configmap)
			Expect(configMapDeletionErr).NotTo(HaveOccurred())
		})

		It("should reconcile from OLSConfig custom resource", func() {
			By("Reconcile the OLSConfig custom resource")
			cr.Spec.OLSConfig.ConversationCache.Postgres.CredentialsSecret = utils.PostgresSecretName
			err := ReconcilePostgres(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a service for lightspeed-postgres-server", func() {

			By("Get postgres service")
			svc := &corev1.Service{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.PostgresServiceName, Namespace: utils.OLSNamespaceDefault}, svc)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a deployment lightspeed-postgres-server", func() {

			By("Get postgres deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.PostgresDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a postgres configmap", func() {

			By("Get the postgres config")
			configMap := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.PostgresConfigMap, Namespace: utils.OLSNamespaceDefault}, configMap)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a postgres bootstrap secret", func() {

			By("Get the bootstrap secret")
			secret := &corev1.Secret{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.PostgresBootstrapSecretName, Namespace: utils.OLSNamespaceDefault}, secret)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a postgres secret", func() {

			By("Get the postgres secret")
			secret := &corev1.Secret{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.PostgresSecretName, Namespace: utils.OLSNamespaceDefault}, secret)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a postgres network policy", func() {
			By("Get the postgres network policy")
			networkPolicy := &networkingv1.NetworkPolicy{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.PostgresNetworkPolicyName, Namespace: utils.OLSNamespaceDefault}, networkPolicy)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should trigger a rolling deployment when there is an update in secret name", func() {

			By("create the test secret with new name")
			dummySecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dummy-secret",
					Namespace: utils.OLSNamespaceDefault,
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind:       "Secret",
							APIVersion: "v1",
							UID:        "ownerUID1",
							Name:       "dummy-secret",
						},
					},
				},
				Data: map[string][]byte{
					utils.OLSComponentPasswordFileName: []byte("test-password-123"),
				},
			}
			secretCreationErr := testReconcilerInstance.Create(ctx, dummySecret)
			Expect(secretCreationErr).NotTo(HaveOccurred())

			By("Get the postgres deployment before update")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.PostgresDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Annotations).NotTo(BeNil())
			oldSecretVersion := dep.Annotations[utils.PostgresSecretResourceVersionAnnotation]

			By("Update the OLSConfig custom resource to use new secret")
			olsConfig := &olsv1alpha1.OLSConfig{}
			err = k8sClient.Get(ctx, crNamespacedName, olsConfig)
			Expect(err).NotTo(HaveOccurred())
			olsConfig.Spec.OLSConfig.ConversationCache.Postgres.CredentialsSecret = "dummy-secret"

			By("Reconcile the postgres server")
			err = ReconcilePostgres(testReconcilerInstance, ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Get the postgres deployment after update")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.PostgresDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Annotations).NotTo(BeNil())

			// Verify that the secret ResourceVersion annotation has been updated
			newSecretVersion := dep.Annotations[utils.PostgresSecretResourceVersionAnnotation]
			Expect(newSecretVersion).NotTo(Equal(oldSecretVersion))
			Expect(newSecretVersion).NotTo(BeEmpty())
		})
	})
})
