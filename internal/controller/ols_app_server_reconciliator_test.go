package controller

import (
	"fmt"
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var tlsSecret *corev1.Secret
var _ = Describe("App server reconciliator", Ordered, func() {
	Context("Creation logic", Ordered, func() {
		var secret *corev1.Secret
		var tlsSecret *corev1.Secret
		var tlsUserSecret *corev1.Secret
		const tlsUserSecretName = "tls-user-secret"
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

			By("create the default tls secret")
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

			By("create user provided tls secret")
			tlsUserSecret, _ = generateRandomSecret()
			tlsUserSecret.Name = tlsUserSecretName
			secretCreationErr = reconciler.Create(ctx, tlsUserSecret)
			Expect(secretCreationErr).NotTo(HaveOccurred())

			By("Set OLSConfig CR to default")
			err := k8sClient.Get(ctx, crNamespacedName, cr)
			Expect(err).NotTo(HaveOccurred())
			crDefault := getDefaultOLSConfigCR()
			cr.Spec = crDefault.Spec
		})

		AfterEach(func() {
			By("Delete the provider secret")
			secretDeletionErr := reconciler.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Delete the tls secret")
			secretDeletionErr = reconciler.Delete(ctx, tlsSecret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Delete the user provided tls secret")
			secretDeletionErr = reconciler.Delete(ctx, tlsUserSecret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())
		})

		It("should reconcile from OLSConfig custom resource", func() {
			By("Reconcile the OLSConfig custom resource")
			err := reconciler.reconcileAppServer(ctx, cr)
			Expect(err).NotTo(HaveOccurred())
			reconciler.updateStatusCondition(ctx, cr, typeApiReady, true, "All components are successfully deployed", nil)
			expectedCondition := metav1.Condition{
				Type:   typeApiReady,
				Status: metav1.ConditionTrue,
			}
			Expect(cr.Status.Conditions).To(ContainElement(HaveField("Type", expectedCondition.Type)))
			Expect(cr.Status.Conditions).To(ContainElement(HaveField("Status", expectedCondition.Status)))
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

		It("should trigger rolling update of the deployment when updating the tolerations", func() {
			By("Get the deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())

			By("Update the OLSConfig custom resource")
			olsConfig := &olsv1alpha1.OLSConfig{}
			err = k8sClient.Get(ctx, crNamespacedName, olsConfig)
			Expect(err).NotTo(HaveOccurred())
			olsConfig.Spec.OLSConfig.DeploymentConfig.APIContainer.Tolerations = []corev1.Toleration{
				{
					Key:      "key",
					Operator: corev1.TolerationOpEqual,
					Value:    "value",
					Effect:   corev1.TaintEffectNoSchedule,
				},
			}

			By("Reconcile the app server")
			err = reconciler.reconcileAppServer(ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Get the deployment")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.Tolerations).NotTo(BeNil())
			Expect(dep.Spec.Template.Spec.Tolerations).To(Equal(olsConfig.Spec.OLSConfig.DeploymentConfig.APIContainer.Tolerations))
		})

		It("should trigger rolling update of the deployment when updating the nodeselector ", func() {
			By("Get the deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())

			By("Update the OLSConfig custom resource")
			olsConfig := &olsv1alpha1.OLSConfig{}
			err = k8sClient.Get(ctx, crNamespacedName, olsConfig)
			Expect(err).NotTo(HaveOccurred())
			olsConfig.Spec.OLSConfig.DeploymentConfig.APIContainer.NodeSelector = map[string]string{
				"key": "value",
			}

			By("Reconcile the app server")
			err = reconciler.reconcileAppServer(ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Get the deployment")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.NodeSelector).NotTo(BeNil())
			Expect(dep.Spec.Template.Spec.NodeSelector).To(Equal(olsConfig.Spec.OLSConfig.DeploymentConfig.APIContainer.NodeSelector))
		})

		It("should trigger rolling update of the deployment when changing tls secret content", func() {

			By("Get the deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			oldHash := dep.Spec.Template.Annotations[OLSAppTLSHashKey]
			Expect(oldHash).NotTo(BeEmpty())

			By("Update the tls secret content")
			tlsSecret.Data["tls.key"] = []byte("new-value")
			err = k8sClient.Update(ctx, tlsSecret)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile the app server
			olsConfig := &olsv1alpha1.OLSConfig{}
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
			Expect(dep.Annotations[OLSAppTLSHashKey]).NotTo(Equal(oldHash))
		})

		It("should trigger rolling update of the deployment when recreating tls secret", func() {

			By("Get the deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			oldHash := dep.Spec.Template.Annotations[OLSAppTLSHashKey]
			Expect(oldHash).NotTo(BeEmpty())

			By("Delete the tls secret")
			secretDeletionErr := reconciler.Delete(ctx, tlsSecret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Recreate the tls secret")
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

			secretCreationErr := reconciler.Create(ctx, tlsSecret)
			Expect(secretCreationErr).NotTo(HaveOccurred())
			olsConfig := &olsv1alpha1.OLSConfig{}
			err = k8sClient.Get(ctx, crNamespacedName, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile the app server")
			err = reconciler.reconcileAppServer(ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Get the deployment")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			Expect(dep.Annotations[OLSAppTLSHashKey]).NotTo(Equal(oldHash))
		})

		It("should update the deployment when switching to user provided tls secret", func() {
			By("Get the old hash")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			oldHash := dep.Spec.Template.Annotations[OLSAppTLSHashKey]
			Expect(oldHash).NotTo(BeEmpty())

			By("Change OLSConfig to use user provided tls secret and reconcile")
			olsConfig := cr.DeepCopy()
			olsConfig.Spec.OLSConfig.TLSConfig = &olsv1alpha1.TLSConfig{
				KeyCertSecretRef: corev1.LocalObjectReference{
					Name: tlsUserSecretName,
				},
			}
			err = reconciler.reconcileAppServer(ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Check new hash is updated")
			dep = &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			bytesArr := make([]byte, len(tlsUserSecret.Data["tls.key"])+len(tlsUserSecret.Data["tls.crt"]))
			copy(bytesArr, tlsUserSecret.Data["tls.key"])
			copy(bytesArr[len(tlsUserSecret.Data["tls.key"]):], tlsUserSecret.Data["tls.crt"])
			newHash, err := hashBytes(bytesArr)
			Expect(err).NotTo(HaveOccurred())
			Expect(newHash).NotTo(Equal(oldHash))
			Expect(dep.Spec.Template.Annotations[OLSAppTLSHashKey]).To(Equal(newHash))

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
			secret.Data["apitoken2"] = []byte("new-value")
			err = k8sClient.Update(ctx, secret)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile for LLM Provider Secrets Again")
			err = reconciler.reconcileLLMSecrets(ctx, olsConfig)
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

		It("should trigger rolling update of the deployment when recreating provider secret", func() {
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

		It("should create a service monitor lightspeed-app-server-monitor", func() {
			By("Get the service monitor")
			sm := &monv1.ServiceMonitor{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: AppServerServiceMonitorName, Namespace: OLSNamespaceDefault}, sm)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a prometheus rule", func() {
			By("Get the prometheus rule")
			pr := &monv1.PrometheusRule{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: AppServerPrometheusRuleName, Namespace: OLSNamespaceDefault}, pr)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error when the LLM provider token secret does not have required keys", func() {
			By("General provider: the token secret miss 'apitoken' key")
			secret, _ := generateRandomSecret()
			// delete the required key "apitoken"
			delete(secret.Data, "apitoken")
			err := k8sClient.Update(ctx, secret)
			Expect(err).NotTo(HaveOccurred())
			err = reconciler.reconcileAppServer(ctx, cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing key 'apitoken'"))

			By("AzureOpenAI provider: the token secret miss 'clientid', 'tenantid', 'client_secret' key")
			secret, _ = generateRandomSecret()
			delete(secret.Data, "client_id")
			delete(secret.Data, "tenant_id")
			delete(secret.Data, "client_secret")
			err = k8sClient.Update(ctx, secret)
			Expect(err).NotTo(HaveOccurred())
			crAzure := cr.DeepCopy()
			crAzure.Spec.LLMConfig.Providers[0].Type = AzureOpenAIType
			err = reconciler.reconcileAppServer(ctx, crAzure)
			Expect(err).NotTo(HaveOccurred())
			delete(secret.Data, "apitoken")
			err = k8sClient.Update(ctx, secret)
			Expect(err).NotTo(HaveOccurred())
			err = reconciler.reconcileAppServer(ctx, crAzure)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing key 'client_id'"))
			secret.Data["client_id"] = []byte("test-client-id")
			err = k8sClient.Update(ctx, secret)
			Expect(err).NotTo(HaveOccurred())
			err = reconciler.reconcileAppServer(ctx, crAzure)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing key 'tenant_id'"))
			secret.Data["tenant_id"] = []byte("test-tenant-id")
			err = k8sClient.Update(ctx, secret)
			Expect(err).NotTo(HaveOccurred())
			err = reconciler.reconcileAppServer(ctx, crAzure)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing key 'client_secret'"))
			secret.Data["client_secret"] = []byte("test-client-secret")
			err = k8sClient.Update(ctx, secret)
			Expect(err).NotTo(HaveOccurred())
			err = reconciler.reconcileAppServer(ctx, crAzure)
			Expect(err).NotTo(HaveOccurred())
		})

	})

	Context("Referred Secrets", Ordered, func() {
		var secret *corev1.Secret
		var tlsSecret *corev1.Secret
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
		})

		AfterEach(func() {
			By("Delete the provider secret")
			secretDeletionErr := reconciler.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())
			By("Delete the tls secret")
			secretDeletionErr = reconciler.Delete(ctx, tlsSecret)
			if secretDeletionErr != nil {
				Expect(errors.IsNotFound(secretDeletionErr)).To(BeTrue())
			} else {
				Expect(secretDeletionErr).NotTo(HaveOccurred())
			}
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

		It("should return error when the LLM provider token secret is not found", func() {
			By("Reconcile after modifying the token secret")
			originalSecretName := cr.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name
			cr.Spec.LLMConfig.Providers[0].CredentialsSecretRef = corev1.LocalObjectReference{Name: "non-existing-secret"}
			err := reconciler.reconcileLLMSecrets(ctx, cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("secret not found: non-existing-secret"))
			Expect(statusHasCondition(cr.Status, metav1.Condition{
				Type:    typeApiReady,
				Status:  metav1.ConditionFalse,
				Reason:  "Reconciling",
				Message: "failed to get LLM provider secret: secret not found: non-existing-secret. error: secrets \"non-existing-secret\" not found",
			})).To(BeTrue())
			cr.Spec.LLMConfig.Providers[0].CredentialsSecretRef = corev1.LocalObjectReference{Name: originalSecretName}
		})

		It("should return error when the TLS secret is not found", func() {
			By("reconcile TLS secret")
			err := reconciler.reconcileTLSSecret(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Delete the tls secret and reconcile again")
			err = reconciler.Delete(ctx, tlsSecret)
			Expect(err).NotTo(HaveOccurred())
			err = reconciler.reconcileTLSSecret(ctx, cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get TLS secret"))
			Expect(statusHasCondition(cr.Status, metav1.Condition{
				Type:    typeApiReady,
				Status:  metav1.ConditionFalse,
				Reason:  "Reconciling",
				Message: "failed to get TLS secret - lightspeed-tls: context deadline exceeded",
			})).To(BeTrue())
		})

	})

	Context("User CA Certs", Ordered, func() {
		var secret *corev1.Secret
		var volumeDefaultMode = int32(420)
		var cmCACert1 *corev1.ConfigMap
		var cmCACert2 *corev1.ConfigMap
		const cmCACert1Name = "ca-cert-1"
		const cmCACert2Name = "ca-cert-2"
		const caCert1FileName = "ca-cert-1.crt"
		const caCert2FileName = "ca-cert-2.crt"
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

			By("create the config map for CA cert 1")
			cmCACert1 = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cmCACert1Name,
					Namespace: OLSNamespaceDefault,
				},
				Data: map[string]string{
					caCert1FileName: testCACert,
				},
			}
			err := reconciler.Create(ctx, cmCACert1)
			Expect(err).NotTo(HaveOccurred())

			By("create the config map for CA cert 2")
			cmCACert2 = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cmCACert2Name,
					Namespace: OLSNamespaceDefault,
				},
				Data: map[string]string{
					caCert2FileName: testCACert,
				},
			}
			err = reconciler.Create(ctx, cmCACert2)
			Expect(err).NotTo(HaveOccurred())

			By("Generate default CR")
			cr = getDefaultOLSConfigCR()
		})

		AfterEach(func() {
			By("Delete the provider secret")
			secretDeletionErr := reconciler.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Delete the tls secret")
			secretDeletionErr = reconciler.Delete(ctx, tlsSecret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Delete the config map for CA cert 1")
			err := reconciler.Delete(ctx, cmCACert1)
			Expect(err).NotTo(HaveOccurred())

			By("Delete the config map for CA cert 2")
			err = reconciler.Delete(ctx, cmCACert2)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should update the configmap and deployment when changing the additional CA cert", func() {
			By("Set up an additional CA cert")
			cr.Spec.OLSConfig.AdditionalCAConfigMapRef = &corev1.LocalObjectReference{
				Name: cmCACert1Name,
			}
			err := reconciler.reconcileAppServer(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("check OLS configmap has extra_ca section")
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: OLSConfigCmName, Namespace: OLSNamespaceDefault}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Data).To(HaveKey(OLSConfigFilename))
			Expect(cm.Data[OLSConfigFilename]).To(ContainSubstring(fmt.Sprintf("extra_ca:\n  - %s", path.Join(OLSAppCertsMountRoot, AppAdditionalCACertDir, caCert1FileName))))
			Expect(cm.Data[OLSConfigFilename]).To(ContainSubstring("certificate_directory: /etc/certs/cert-bundle"))

			By("check the additional CA configmap has watcher annotation")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: cmCACert1Name, Namespace: OLSNamespaceDefault}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Annotations).To(HaveKeyWithValue(WatcherAnnotationKey, OLSConfigName))

			By("Get app deployment and check the volume mount")
			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerDeploymentName, Namespace: OLSNamespaceDefault}, deployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Spec.Template.Spec.Volumes).To(ContainElements(
				corev1.Volume{
					Name: AdditionalCAVolumeName,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: cmCACert1Name,
							},
							DefaultMode: &volumeDefaultMode,
						},
					},
				},
				corev1.Volume{
					Name: CertBundleVolumeName,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			))
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).To(ContainElement(corev1.VolumeMount{
				Name:      AdditionalCAVolumeName,
				MountPath: path.Join(OLSAppCertsMountRoot, AppAdditionalCACertDir),
				ReadOnly:  true,
			}))
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).To(ContainElement(corev1.VolumeMount{
				Name:      CertBundleVolumeName,
				MountPath: path.Join(OLSAppCertsMountRoot, CertBundleDir),
			}))
		})

		It("should not generate additional CA related settings if additional CA is not defined", func() {
			By("Set no additional CA cert")
			cr.Spec.OLSConfig.AdditionalCAConfigMapRef = nil
			err := reconciler.reconcileAppServer(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Check app deployment does not have additional CA volumes and volume mounts")
			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerDeploymentName, Namespace: OLSNamespaceDefault}, deployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Spec.Template.Spec.Volumes).NotTo(ContainElement(corev1.Volume{
				Name: AdditionalCAVolumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: cmCACert1Name,
						},
						DefaultMode: &volumeDefaultMode,
					},
				},
			}))

			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).NotTo(ContainElement(corev1.VolumeMount{
				Name:      AdditionalCAVolumeName,
				MountPath: path.Join(OLSAppCertsMountRoot, AppAdditionalCACertDir),
				ReadOnly:  true,
			}))

			By("Check OLS configmap does not have extra_ca section")
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: OLSConfigCmName, Namespace: OLSNamespaceDefault}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Data).To(HaveKey(OLSConfigFilename))
			Expect(cm.Data[OLSConfigFilename]).NotTo(ContainSubstring("extra_ca:"))

		})

	})

	Context("RAG extension", Ordered, func() {
		var secret *corev1.Secret
		var tlsSecret *corev1.Secret
		var tlsUserSecret *corev1.Secret
		const tlsUserSecretName = "tls-user-secret"
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

			By("create the default tls secret")
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

			By("create user provided tls secret")
			tlsUserSecret, _ = generateRandomSecret()
			tlsUserSecret.Name = tlsUserSecretName
			secretCreationErr = reconciler.Create(ctx, tlsUserSecret)
			Expect(secretCreationErr).NotTo(HaveOccurred())

			By("Set OLSConfig CR to default")
			err := k8sClient.Get(ctx, crNamespacedName, cr)
			Expect(err).NotTo(HaveOccurred())
			crDefault := getDefaultOLSConfigCR()
			cr.Spec = crDefault.Spec
		})

		AfterEach(func() {
			By("Delete the provider secret")
			secretDeletionErr := reconciler.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Delete the tls secret")
			secretDeletionErr = reconciler.Delete(ctx, tlsSecret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Delete the user provided tls secret")
			secretDeletionErr = reconciler.Delete(ctx, tlsUserSecret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())
		})

		It("should generate RAG volumes and initContainers when RAG is defined, remove them when RAG is not defined", func() {
			By("Reconcile with RAG defined")
			cr.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{
				{
					IndexPath: "/rag/vector_db/ocp_product_docs/4.15",
					IndexID:   "ocp-product-docs-4_15",
					Image:     "rag-ocp-product-docs:4.15",
				},
				{
					IndexPath: "/rag/vector_db/ansible_docs/2.18",
					IndexID:   "ansible-docs-2_18",
					Image:     "rag-ansible-docs:2.18",
				},
			}
			err := reconciler.reconcileAppServer(ctx, cr)
			Expect(err).NotTo(HaveOccurred())
			By("Check deployment have RAG volumes and initContainers")
			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerDeploymentName, Namespace: OLSNamespaceDefault}, deployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Spec.Template.Spec.Volumes).To(ContainElement(corev1.Volume{
				Name: RAGVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			}))
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).NotTo(ContainElement(corev1.VolumeMount{
				Name:      RAGVolumeName,
				MountPath: RAGVolumeMountPath,
				ReadOnly:  true,
			}))

			By("Reconcile without RAG defined")
			cr.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{}
			err = reconciler.reconcileAppServer(ctx, cr)
			Expect(err).NotTo(HaveOccurred())
			By("Check deployment does not have RAG volumes and initContainers")
			deployment = &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerDeploymentName, Namespace: OLSNamespaceDefault}, deployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Spec.Template.Spec.Volumes).NotTo(ContainElement(corev1.Volume{
				Name: RAGVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			}))
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).NotTo(ContainElement(corev1.VolumeMount{
				Name:      RAGVolumeName,
				MountPath: RAGVolumeMountPath,
				ReadOnly:  true,
			}))

		})

		It("should add RAG indexes into the configmap when RAG is defined", func() {
			By("Reconcile with RAG defined")
			cr.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{
				{
					IndexPath: "/rag/vector_db/ocp_product_docs/4.15",
					IndexID:   "ocp-product-docs-4_15",
					Image:     "rag-ocp-product-docs:4.15",
				},
				{
					IndexPath: "/rag/vector_db/ansible_docs/2.18",
					IndexID:   "ansible-docs-2_18",
					Image:     "rag-ansible-docs:2.18",
				},
			}
			err := reconciler.reconcileAppServer(ctx, cr)
			Expect(err).NotTo(HaveOccurred())
			By("Check configmap has RAG indexes")
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: OLSConfigCmName, Namespace: OLSNamespaceDefault}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Data).To(HaveKey(OLSConfigFilename))
			major, minor, err := reconciler.getClusterVersion(ctx)
			Expect(err).NotTo(HaveOccurred())
			// OCP document is always there
			Expect(cm.Data[OLSConfigFilename]).To(ContainSubstring("indexes:"))
			Expect(cm.Data[OLSConfigFilename]).To(ContainSubstring("  - product_docs_index_id: " + "ocp-product-docs-" + major + "_" + minor))
			Expect(cm.Data[OLSConfigFilename]).To(ContainSubstring("    product_docs_index_path: " + "/app-root/vector_db/ocp_product_docs/" + major + "." + minor))
			Expect(cm.Data[OLSConfigFilename]).To(ContainSubstring("  - product_docs_index_id: ocp-product-docs-4_15"))
			Expect(cm.Data[OLSConfigFilename]).To(ContainSubstring("    product_docs_index_path: " + RAGVolumeMountPath + "/rag-0"))
			Expect(cm.Data[OLSConfigFilename]).To(ContainSubstring("  - product_docs_index_id: ansible-docs-2_18"))
			Expect(cm.Data[OLSConfigFilename]).To(ContainSubstring("    product_docs_index_path: " + RAGVolumeMountPath + "/rag-1"))
		})

	})

})
