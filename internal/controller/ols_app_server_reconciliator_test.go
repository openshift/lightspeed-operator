package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var secret *corev1.Secret
var _ = Describe("App server reconciliator", Ordered, func() {
	Context("Creation logic", Ordered, func() {

		BeforeEach(func() {
			By("create the provider secret")
			secret, _ = generateRandomSecret()
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
		})

		AfterEach(func() {
			By("Delete the provider secret")
			secretDeletionErr := reconciler.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())
		})

		It("should reconcile from OLSConfig custom resource", func() {
			By("Reconcile the OLSConfig custom resource")
			err := reconciler.reconcileAppServer(ctx, cr)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a service account lightspeed-app-server", func() {
			By("Get the service account")
			sa := &corev1.ServiceAccount{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerServiceAccountName, Namespace: OLSNamespaceDefault}, sa)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a SAR cluster role lightspeed-app-server-sar-role", func() {
			By("Get the SAR cluster role")
			role := &rbacv1.ClusterRole{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: OLSAppServerSARRoleName}, role)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a SAR cluster role binding lightspeed-app-server-sar-role-binding", func() {
			By("Get the SAR cluster role binding")
			rb := &rbacv1.ClusterRoleBinding{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: OLSAppServerSARRoleBindingName}, rb)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a service lightspeed-app-server", func() {
			By("Get the service")
			svc := &corev1.Service{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerServiceName, Namespace: OLSNamespaceDefault}, svc)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a config map olsconfig", func() {
			By("Get the config map")
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: OLSConfigCmName, Namespace: OLSNamespaceDefault}, cm)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a deployment lightspeed-app-server", func() {
			By("Get the deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should trigger rolling update of the deployment when changing the generated config", func() {
			By("Get the deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			oldHash := dep.Spec.Template.Annotations[OLSConfigHashKey]
			Expect(oldHash).NotTo(BeEmpty())

			By("Update the OLSConfig custom resource")
			olsConfig := &olsv1alpha1.OLSConfig{}
			err = k8sClient.Get(ctx, crNamespacedName, olsConfig)
			Expect(err).NotTo(HaveOccurred())
			olsConfig.Spec.OLSConfig.LogLevel = "ERROR"

			By("Reconcile the app server")
			err = reconciler.reconcileAppServer(ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Get the deployment")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			Expect(dep.Annotations[OLSConfigHashKey]).NotTo(Equal(oldHash))
			Expect(dep.Annotations[OLSConfigHashKey]).NotTo(Equal(oldHash))
		})

		It("should trigger rolling update of the deployment when changing LLM secret content", func() {

			By("Reconcile for LLM Provider Secrets")
			olsConfig := &olsv1alpha1.OLSConfig{}
			err := reconciler.reconcileLLMSecrets(ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Get the deployment")
			dep := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			oldHash := dep.Spec.Template.Annotations[LLMProviderHashKey]

			By("Update the provider secret content")
			secret.Data[LLMApiTokenFileName] = []byte("new-value")
			err = k8sClient.Update(ctx, secret)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile the app server
			err = k8sClient.Get(ctx, crNamespacedName, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile the app server")
			err = reconciler.reconcileAppServer(ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Get the updated deployment")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())

			// Verify that the hash in deployment annotations has been updated
			Expect(dep.Annotations[LLMProviderHashKey]).NotTo(Equal(oldHash))
		})

		It("should trigger rolling update of the deployment when recreating secret", func() {

			By("Get the deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			oldHash := dep.Spec.Template.Annotations[LLMProviderHashKey]
			Expect(oldHash).NotTo(BeEmpty())

			By("Delete the provider secret")
			secretDeletionErr := reconciler.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Recreate the provider secret")
			secret, _ = generateRandomSecret()
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
			olsConfig := &olsv1alpha1.OLSConfig{}
			err = k8sClient.Get(ctx, crNamespacedName, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile for LLM Provider Secrets Again")
			err = reconciler.reconcileLLMSecrets(ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile the app server")
			err = reconciler.reconcileAppServer(ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Get the deployment")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			Expect(dep.Annotations[LLMProviderHashKey]).NotTo(Equal(oldHash))
		})
	})

	Context("Creation logic", Ordered, func() {

		BeforeEach(func() {
			By("create the provider secret")
			secret, _ = generateRandomSecret()
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
		})

		AfterEach(func() {
			By("Delete the provider secret")
			secretDeletionErr := reconciler.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())
		})

		It("should reconcile from OLSConfig custom resource", func() {
			By("Reconcile the OLSConfig custom resource")
			err := reconciler.reconcileAppServer(ctx, cr)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should update deployment volumes when changing the token secret", func() {
			By("create the provider secret")
			secret, _ := generateRandomSecret()
			secret.Name = "new-token-secret"
			secret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       "new-token-secret",
				},
			})
			secretCreationErr := reconciler.Create(ctx, secret)
			Expect(secretCreationErr).NotTo(HaveOccurred())

			By("Reconcile after modifying the token secret")
			cr.Spec.LLMConfig.Providers[0].CredentialsSecretRef = corev1.LocalObjectReference{Name: "new-token-secret"}
			err := reconciler.reconcileAppServer(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Get the deployment and check the new volume")
			dep := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			defaultSecretMode := int32(420)
			Expect(dep.Spec.Template.Spec.Volumes).To(ContainElement(corev1.Volume{
				Name: "secret-new-token-secret",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  "new-token-secret",
						DefaultMode: &defaultSecretMode,
					},
				},
			}))

			By("Delete the provider secret")
			secretDeletionErr := reconciler.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())
		})
	})
})
