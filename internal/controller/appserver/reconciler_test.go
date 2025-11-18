package appserver

import (
	"fmt"
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

var tlsSecret *corev1.Secret
var _ = Describe("App server reconciliator", Ordered, func() {
	Context("Creation logic", Ordered, func() {
		var secret *corev1.Secret
		var configmap *corev1.ConfigMap
		var tlsSecret *corev1.Secret
		var tlsUserSecret *corev1.Secret
		const tlsUserSecretName = "tls-user-secret"
		BeforeEach(func() {
			By("create the provider secret")
			secret, _ = utils.GenerateRandomSecret()
			secret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       "test-secret",
				},
			})
			secretCreationErr := testReconcilerInstance.Create(ctx, secret)
			Expect(secretCreationErr).NotTo(HaveOccurred())

			By("create the default tls secret")
			tlsSecret, _ = utils.GenerateRandomTLSSecret()
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

			By("create user provided tls secret")
			tlsUserSecret, _ = utils.GenerateRandomTLSSecret()
			tlsUserSecret.Name = tlsUserSecretName
			secretCreationErr = testReconcilerInstance.Create(ctx, tlsUserSecret)
			Expect(secretCreationErr).NotTo(HaveOccurred())

			By("Set OLSConfig CR to default")
			err := k8sClient.Get(ctx, crNamespacedName, cr)
			Expect(err).NotTo(HaveOccurred())
			crDefault := utils.GetDefaultOLSConfigCR()
			cr.Spec = crDefault.Spec

			By("create the OpenShift certificates config map")
			configmap, _ = utils.GenerateRandomConfigMap()
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
			By("Delete the provider secret")
			secretDeletionErr := testReconcilerInstance.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Delete the tls secret")
			secretDeletionErr = testReconcilerInstance.Delete(ctx, tlsSecret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Delete the user provided tls secret")
			secretDeletionErr = testReconcilerInstance.Delete(ctx, tlsUserSecret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Delete OpenShift certificates config map")
			configMapDeletionErr := testReconcilerInstance.Delete(ctx, configmap)
			Expect(configMapDeletionErr).NotTo(HaveOccurred())
		})

		It("should reconcile from OLSConfig custom resource", func() {
			By("Reconcile the OLSConfig custom resource")
			err := ReconcileAppServer(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())
			// Note: Status conditions are managed by the main controller, not component reconcilers
		})

		It("should create a service account lightspeed-app-server", func() {
			By("Get the service account")
			sa := &corev1.ServiceAccount{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerServiceAccountName, Namespace: utils.OLSNamespaceDefault}, sa)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a SAR cluster role lightspeed-app-server-sar-role", func() {
			By("Get the SAR cluster role")
			role := &rbacv1.ClusterRole{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: utils.OLSAppServerSARRoleName}, role)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a SAR cluster role binding lightspeed-app-server-sar-role-binding", func() {
			By("Get the SAR cluster role binding")
			rb := &rbacv1.ClusterRoleBinding{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: utils.OLSAppServerSARRoleBindingName}, rb)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a service lightspeed-app-server", func() {
			By("Get the service")
			svc := &corev1.Service{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerServiceName, Namespace: utils.OLSNamespaceDefault}, svc)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a config map olsconfig", func() {
			By("Get the config map")
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSConfigCmName, Namespace: utils.OLSNamespaceDefault}, cm)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a deployment lightspeed-app-server", func() {
			By("Get the deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a network policy lightspeed-app-server", func() {
			By("Get the network policy")
			np := &networkingv1.NetworkPolicy{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerNetworkPolicyName, Namespace: utils.OLSNamespaceDefault}, np)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should trigger rolling update of the deployment when changing the generated config", func() {
			By("Get the deployment before update")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Annotations).NotTo(BeNil())
			oldConfigMapVersion := dep.Annotations[utils.OLSConfigMapResourceVersionAnnotation]
			Expect(oldConfigMapVersion).NotTo(BeEmpty())

			By("Update the OLSConfig custom resource")
			olsConfig := &olsv1alpha1.OLSConfig{}
			err = k8sClient.Get(ctx, crNamespacedName, olsConfig)
			Expect(err).NotTo(HaveOccurred())
			olsConfig.Spec.OLSConfig.LogLevel = utils.LogLevelError

			By("Reconcile the app server")
			err = ReconcileAppServer(testReconcilerInstance, ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Get the deployment after update")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Annotations).NotTo(BeNil())

			// Verify that the ConfigMap ResourceVersion annotation has been updated
			newConfigMapVersion := dep.Annotations[utils.OLSConfigMapResourceVersionAnnotation]
			Expect(newConfigMapVersion).NotTo(Equal(oldConfigMapVersion))
			Expect(newConfigMapVersion).NotTo(BeEmpty())
		})

		It("should trigger rolling update of the deployment when updating the tolerations", func() {
			By("Get the deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
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
			err = ReconcileAppServer(testReconcilerInstance, ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Get the deployment")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.Tolerations).NotTo(BeNil())
			Expect(dep.Spec.Template.Spec.Tolerations).To(Equal(olsConfig.Spec.OLSConfig.DeploymentConfig.APIContainer.Tolerations))
		})

		It("should trigger rolling update of the deployment when updating the nodeselector ", func() {
			By("Get the deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())

			By("Update the OLSConfig custom resource")
			olsConfig := &olsv1alpha1.OLSConfig{}
			err = k8sClient.Get(ctx, crNamespacedName, olsConfig)
			Expect(err).NotTo(HaveOccurred())
			olsConfig.Spec.OLSConfig.DeploymentConfig.APIContainer.NodeSelector = map[string]string{
				"key": "value",
			}

			By("Reconcile the app server")
			err = ReconcileAppServer(testReconcilerInstance, ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Get the deployment")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.NodeSelector).NotTo(BeNil())
			Expect(dep.Spec.Template.Spec.NodeSelector).To(Equal(olsConfig.Spec.OLSConfig.DeploymentConfig.APIContainer.NodeSelector))
		})

		// This is specific for hash based implementation. Now done by watcher
		XIt("should trigger rolling update of the deployment when changing tls secret content", func() {

			By("Get the deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			oldHash := dep.Spec.Template.Annotations[utils.OLSAppTLSHashKey]
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
			err = ReconcileAppServer(testReconcilerInstance, ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Get the updated deployment")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())

			// Verify that the hash in deployment annotations has been updated
			Expect(dep.Annotations[utils.OLSAppTLSHashKey]).NotTo(Equal(oldHash))
		})

		// This is specific for hash based implementation. Now done by watcher
		XIt("should trigger rolling update of the deployment when recreating tls secret", func() {

			By("Get the deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			oldHash := dep.Spec.Template.Annotations[utils.OLSAppTLSHashKey]
			Expect(oldHash).NotTo(BeEmpty())

			By("Delete the tls secret")
			secretDeletionErr := testReconcilerInstance.Delete(ctx, tlsSecret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Recreate the tls secret")
			tlsSecret, _ = utils.GenerateRandomTLSSecret()
			tlsSecret.Name = utils.OLSCertsSecretName
			tlsSecret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       utils.OLSCertsSecretName,
				},
			})

			secretCreationErr := testReconcilerInstance.Create(ctx, tlsSecret)
			Expect(secretCreationErr).NotTo(HaveOccurred())
			olsConfig := &olsv1alpha1.OLSConfig{}
			err = k8sClient.Get(ctx, crNamespacedName, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile the app server")
			err = ReconcileAppServer(testReconcilerInstance, ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Get the deployment")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			Expect(dep.Annotations[utils.OLSAppTLSHashKey]).NotTo(Equal(oldHash))
		})

		// This is specific for hash based implementation. Now done by watcher
		XIt("should update the deployment when switching to user provided tls secret", func() {
			By("Get the old hash")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			oldHash := dep.Spec.Template.Annotations[utils.OLSAppTLSHashKey]
			Expect(oldHash).NotTo(BeEmpty())

			By("Change OLSConfig to use user provided tls secret and reconcile")
			olsConfig := cr.DeepCopy()
			olsConfig.Spec.OLSConfig.TLSConfig = &olsv1alpha1.TLSConfig{
				KeyCertSecretRef: corev1.LocalObjectReference{
					Name: tlsUserSecretName,
				},
			}
			err = ReconcileAppServer(testReconcilerInstance, ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Check new hash is updated")
			dep = &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			// Hash-based validation removed - now handled by watchers
			// Expect(dep.Spec.Template.Annotations[utils.OLSAppTLSHashKey]).To(Equal(newHash))

		})

		// TODO: Re-enable after annotation consolidation (Phase 2)
		// This test relies on hash-based change detection which will be replaced by watcher annotations
		// It("should trigger rolling update of the deployment when changing LLM secret content", func() {
		// 	var err error

		// 	By("Validate external secrets (LLM Provider)")
		// 	olsConfig := &olsv1alpha1.OLSConfig{}
		// 	err = utils.ValidateExternalSecrets(testReconcilerInstance, ctx, olsConfig)
		// 	Expect(err).NotTo(HaveOccurred())

		// 	By("Get the deployment")
		// 	dep := &appsv1.Deployment{}
		// 	err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
		// 	oldHash := dep.Spec.Template.Annotations[utils.LLMProviderHashKey]
		// 	By("Update the provider secret content")
		// 	secret.Data["apitoken2"] = []byte("new-value")
		// 	err = k8sClient.Update(ctx, secret)
		// 	Expect(err).NotTo(HaveOccurred())

		// 	By("Validate external secrets again (LLM Provider)")
		// 	// Validate external secrets before testing
		// 	err = utils.ValidateExternalSecrets(testReconcilerInstance, ctx, olsConfig)
		// 	Expect(err).NotTo(HaveOccurred())

		// 	// Reconcile the app server
		// 	err = k8sClient.Get(ctx, crNamespacedName, olsConfig)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	By("Reconcile the app server")
		// 	err = ReconcileAppServer(testReconcilerInstance, ctx, olsConfig)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	By("Get the updated deployment")
		// 	err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
		// 	// Verify that the hash in deployment annotations has been updated
		// 	Expect(dep.Spec.Template.Annotations[utils.LLMProviderHashKey]).NotTo(Equal(oldHash))
		// })

		// TODO: Re-enable after annotation consolidation (Phase 2)
		// This test relies on hash-based change detection which will be replaced by watcher annotations
		// It("should trigger rolling update of the deployment when recreating provider secret", func() {
		// 	By("Get the deployment")
		// 	dep := &appsv1.Deployment{}
		// 	err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
		// 	oldHash := dep.Spec.Template.Annotations[utils.LLMProviderHashKey]
		// 	Expect(oldHash).NotTo(BeEmpty())
		// 	By("Delete the provider secret")
		// 	secretDeletionErr := testReconcilerInstance.Delete(ctx, secret)
		// 	Expect(secretDeletionErr).NotTo(HaveOccurred())
		// 	By("Recreate the provider secret")
		// 	secret, _ = utils.GenerateRandomSecret()
		// 	secret.SetOwnerReferences([]metav1.OwnerReference{
		// 		{
		// 			Kind:       "Secret",
		// 			APIVersion: "v1",
		// 			UID:        "ownerUID",
		// 			Name:       "test-secret",
		// 		},
		// 	})

		// 	secretCreationErr := testReconcilerInstance.Create(ctx, secret)
		// 	Expect(secretCreationErr).NotTo(HaveOccurred())

		// 	olsConfig := &olsv1alpha1.OLSConfig{}
		// 	err = k8sClient.Get(ctx, crNamespacedName, olsConfig)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	By("Validate external secrets again (LLM Provider)")
		// 	// Validate external secrets before testing
		// 	err = utils.ValidateExternalSecrets(testReconcilerInstance, ctx, olsConfig)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	By("Reconcile the app server")
		// 	err = ReconcileAppServer(testReconcilerInstance, ctx, olsConfig)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	By("Get the deployment")
		// 	err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
		// 	Expect(dep.Spec.Template.Annotations[utils.LLMProviderHashKey]).NotTo(Equal(oldHash))
		// })

		It("should create a service monitor lightspeed-app-server-monitor", func() {
			By("Get the service monitor")
			sm := &monv1.ServiceMonitor{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.AppServerServiceMonitorName, Namespace: utils.OLSNamespaceDefault}, sm)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a metrics reader secret", func() {
			By("Get the metrics reader secret")
			secret := &corev1.Secret{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.MetricsReaderServiceAccountTokenSecretName, Namespace: utils.OLSNamespaceDefault}, secret)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a prometheus rule", func() {
			By("Get the prometheus rule")
			pr := &monv1.PrometheusRule{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.AppServerPrometheusRuleName, Namespace: utils.OLSNamespaceDefault}, pr)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create exporter configmap when data collector is enabled", func() {
			By("Enable telemetry via pull secret and reconcile")
			// Ensure exporter container has a valid image when enabled
			if tr, ok := testReconcilerInstance.(*utils.TestReconciler); ok {
				tr.DataverseExporter = utils.DataverseExporterImageDefault
				tr.McpServerImage = utils.OpenShiftMCPServerImageDefault
			}
			utils.CreateTelemetryPullSecret(ctx, k8sClient, true)
			defer utils.DeleteTelemetryPullSecret(ctx, k8sClient)
			err := ReconcileAppServer(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Verify exporter configmap exists")
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.ExporterConfigCmName, Namespace: utils.OLSNamespaceDefault}, cm)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should delete exporter configmap when data collector is disabled", func() {
			By("Ensure exporter configmap exists by enabling telemetry and reconciling")
			// Ensure exporter container has a valid image when enabled
			if tr, ok := testReconcilerInstance.(*utils.TestReconciler); ok {
				tr.DataverseExporter = utils.DataverseExporterImageDefault
				tr.McpServerImage = utils.OpenShiftMCPServerImageDefault
			}
			utils.CreateTelemetryPullSecret(ctx, k8sClient, true)
			err := ReconcileAppServer(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Verify exporter configmap exists")
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.ExporterConfigCmName, Namespace: utils.OLSNamespaceDefault}, cm)
			Expect(err).NotTo(HaveOccurred())

			By("Disable telemetry and reconcile to trigger deletion")
			utils.DeleteTelemetryPullSecret(ctx, k8sClient)
			err = ReconcileAppServer(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Verify exporter configmap has been deleted")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.ExporterConfigCmName, Namespace: utils.OLSNamespaceDefault}, &corev1.ConfigMap{})
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("should return error when the LLM provider token secret does not have required keys", func() {
			By("General provider: the token secret miss 'apitoken' key")
			secret, _ := utils.GenerateRandomSecret()
			// delete the required key "apitoken"
			delete(secret.Data, "apitoken")
			err := k8sClient.Update(ctx, secret)
			Expect(err).NotTo(HaveOccurred())
			err = ReconcileAppServer(testReconcilerInstance, ctx, cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("reconcile OLSConfigMap"))

			By("AzureOpenAI provider: the token secret miss 'client_id', 'tenant_id', 'client_secret' key")
			secret, _ = utils.GenerateRandomSecret()
			delete(secret.Data, "client_id")
			delete(secret.Data, "tenant_id")
			delete(secret.Data, "client_secret")
			delete(secret.Data, "apitoken")
			err = k8sClient.Update(ctx, secret)
			Expect(err).NotTo(HaveOccurred())
			crAzure := cr.DeepCopy()
			crAzure.Spec.LLMConfig.Providers[0].Type = utils.AzureOpenAIType
			err = ReconcileAppServer(testReconcilerInstance, ctx, crAzure)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("reconcile OLSConfigMap"))
		})

	})

	Context("Referred Secrets", Ordered, func() {
		var secret *corev1.Secret
		var tlsSecret *corev1.Secret
		var configmap *corev1.ConfigMap
		BeforeEach(func() {
			By("create the provider secret")
			secret, _ = utils.GenerateRandomSecret()
			secret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       "test-secret",
				},
			})
			secretCreationErr := testReconcilerInstance.Create(ctx, secret)
			Expect(secretCreationErr).NotTo(HaveOccurred())
			By("create the tls secret")
			tlsSecret, _ = utils.GenerateRandomTLSSecret()
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

			By("create the OpenShift certificates config map")
			configmap, _ = utils.GenerateRandomConfigMap()
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
			By("Delete the provider secret")
			secretDeletionErr := testReconcilerInstance.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())
			By("Delete the tls secret")
			secretDeletionErr = testReconcilerInstance.Delete(ctx, tlsSecret)
			if secretDeletionErr != nil {
				Expect(errors.IsNotFound(secretDeletionErr)).To(BeTrue())
			} else {
				Expect(secretDeletionErr).NotTo(HaveOccurred())
			}
			By("Delete OpenShift certificates config map")
			configMapDeletionErr := testReconcilerInstance.Delete(ctx, configmap)
			Expect(configMapDeletionErr).NotTo(HaveOccurred())
		})

		It("should reconcile from OLSConfig custom resource", func() {
			By("Reconcile the OLSConfig custom resource")
			err := ReconcileAppServer(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should update deployment volumes when changing the token secret", func() {
			By("create the provider secret")
			secret, _ := utils.GenerateRandomSecret()
			secret.Name = "new-token-secret"
			secret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       "new-token-secret",
				},
			})
			secretCreationErr := testReconcilerInstance.Create(ctx, secret)
			Expect(secretCreationErr).NotTo(HaveOccurred())

			By("Reconcile after modifying the token secret")
			cr.Spec.LLMConfig.Providers[0].CredentialsSecretRef = corev1.LocalObjectReference{Name: "new-token-secret"}
			err := ReconcileAppServer(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Get the deployment and check the new volume")
			dep := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
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
			secretDeletionErr := testReconcilerInstance.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())
		})

		It("should return error when the LLM provider token secret is not found", func() {
			By("Validate external secrets after modifying the token secret")
			originalSecretName := cr.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name
			cr.Spec.LLMConfig.Providers[0].CredentialsSecretRef = corev1.LocalObjectReference{Name: "non-existing-secret"}
			err := utils.ValidateExternalSecrets(testReconcilerInstance, ctx, cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("secret not found: non-existing-secret"))
			// Note: Status condition management is the responsibility of the main controller, not component reconcilers
			cr.Spec.LLMConfig.Providers[0].CredentialsSecretRef = corev1.LocalObjectReference{Name: originalSecretName}
		})

		It("should return error when the TLS secret is not found", func() {
			By("reconcile TLS secret")
			err := ReconcileTLSSecret(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Delete the tls secret and reconcile again")
			err = testReconcilerInstance.Delete(ctx, tlsSecret)
			Expect(err).NotTo(HaveOccurred())
			err = ReconcileTLSSecret(testReconcilerInstance, ctx, cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get TLS secret"))
		})

	})

	Context("User CA Certs", Ordered, func() {
		var secret *corev1.Secret
		var volumeDefaultMode = int32(420)
		var cmCACert1 *corev1.ConfigMap
		var cmCACert2 *corev1.ConfigMap
		var configmap *corev1.ConfigMap
		const cmCACert1Name = "ca-cert-1"
		const cmCACert2Name = "ca-cert-2"
		const caCert1FileName = "ca-cert-1.crt"
		const caCert2FileName = "ca-cert-2.crt"
		BeforeEach(func() {
			By("create the provider secret")
			secret, _ = utils.GenerateRandomSecret()
			secret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       "test-secret",
				},
			})
			secretCreationErr := testReconcilerInstance.Create(ctx, secret)
			Expect(secretCreationErr).NotTo(HaveOccurred())

			By("create the tls secret")
			tlsSecret, _ = utils.GenerateRandomTLSSecret()
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

			By("create the config map for CA cert 1")
			cmCACert1 = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cmCACert1Name,
					Namespace: utils.OLSNamespaceDefault,
				},
				Data: map[string]string{
					caCert1FileName: utils.TestCACert,
				},
			}
			err := testReconcilerInstance.Create(ctx, cmCACert1)
			Expect(err).NotTo(HaveOccurred())

			By("create the config map for CA cert 2")
			cmCACert2 = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cmCACert2Name,
					Namespace: utils.OLSNamespaceDefault,
				},
				Data: map[string]string{
					caCert2FileName: utils.TestCACert,
				},
			}
			err = testReconcilerInstance.Create(ctx, cmCACert2)
			Expect(err).NotTo(HaveOccurred())

			By("create the OpenShift certificates config map")
			configmap, _ = utils.GenerateRandomConfigMap()
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

			By("Generate default CR")
			cr = utils.GetDefaultOLSConfigCR()
		})

		AfterEach(func() {
			By("Delete the provider secret")
			secretDeletionErr := testReconcilerInstance.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Delete the tls secret")
			secretDeletionErr = testReconcilerInstance.Delete(ctx, tlsSecret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Delete the config map for CA cert 1")
			err := testReconcilerInstance.Delete(ctx, cmCACert1)
			Expect(err).NotTo(HaveOccurred())

			By("Delete the config map for CA cert 2")
			err = testReconcilerInstance.Delete(ctx, cmCACert2)
			Expect(err).NotTo(HaveOccurred())

			By("Delete OpenShift certificates config map")
			configMapDeletionErr := testReconcilerInstance.Delete(ctx, configmap)
			Expect(configMapDeletionErr).NotTo(HaveOccurred())
		})

		It("should update the configmap and deployment when changing the additional CA cert", func() {
			By("Set up an additional CA cert")
			cr.Spec.OLSConfig.AdditionalCAConfigMapRef = &corev1.LocalObjectReference{
				Name: cmCACert1Name,
			}
			err := ReconcileAppServer(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("check OLS configmap has extra_ca section")
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSConfigCmName, Namespace: utils.OLSNamespaceDefault}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Data).To(HaveKey(utils.OLSConfigFilename))
			Expect(cm.Data[utils.OLSConfigFilename]).To(ContainSubstring("extra_ca:\n  - /etc/certs/ols-additional-ca/service-ca.crt\n  - /etc/certs/ols-user-ca/ca-cert-1.crt"))
			Expect(cm.Data[utils.OLSConfigFilename]).To(ContainSubstring("certificate_directory: /etc/certs/cert-bundle"))

			By("check the additional CA configmap has watcher annotation")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: cmCACert1Name, Namespace: utils.OLSNamespaceDefault}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Annotations).To(HaveKeyWithValue(utils.WatcherAnnotationKey, utils.OLSConfigName))

			By("Get app deployment and check the volume mount")
			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerDeploymentName, Namespace: utils.OLSNamespaceDefault}, deployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Spec.Template.Spec.Volumes).To(ContainElements(
				corev1.Volume{
					Name: utils.AdditionalCAVolumeName,
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
					Name: utils.CertBundleVolumeName,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			))
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).To(ContainElement(corev1.VolumeMount{
				Name:      utils.AdditionalCAVolumeName,
				MountPath: path.Join(utils.OLSAppCertsMountRoot, utils.UserCACertDir),
				ReadOnly:  true,
			}))
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).To(ContainElement(corev1.VolumeMount{
				Name:      utils.CertBundleVolumeName,
				MountPath: path.Join(utils.OLSAppCertsMountRoot, utils.CertBundleVolumeName),
			}))
		})

		It("should not generate additional CA related settings if additional CA is not defined", func() {
			By("Set no additional CA cert")
			cr.Spec.OLSConfig.AdditionalCAConfigMapRef = nil
			err := ReconcileAppServer(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Check app deployment does not have additional CA volumes and volume mounts")
			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerDeploymentName, Namespace: utils.OLSNamespaceDefault}, deployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Spec.Template.Spec.Volumes).NotTo(ContainElement(corev1.Volume{
				Name: utils.AdditionalCAVolumeName,
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
				Name:      utils.AdditionalCAVolumeName,
				MountPath: path.Join(utils.OLSAppCertsMountRoot, utils.AppAdditionalCACertDir),
				ReadOnly:  true,
			}))
		})

	})

	Context("RAG extension", Ordered, func() {
		var secret *corev1.Secret
		var tlsSecret *corev1.Secret
		var tlsUserSecret *corev1.Secret
		var configmap *corev1.ConfigMap
		const tlsUserSecretName = "tls-user-secret"
		BeforeEach(func() {
			By("create the provider secret")
			secret, _ = utils.GenerateRandomSecret()
			secret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       "test-secret",
				},
			})
			secretCreationErr := testReconcilerInstance.Create(ctx, secret)
			Expect(secretCreationErr).NotTo(HaveOccurred())

			By("create the default tls secret")
			tlsSecret, _ = utils.GenerateRandomTLSSecret()
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

			By("create user provided tls secret")
			tlsUserSecret, _ = utils.GenerateRandomTLSSecret()
			tlsUserSecret.Name = tlsUserSecretName
			secretCreationErr = testReconcilerInstance.Create(ctx, tlsUserSecret)
			Expect(secretCreationErr).NotTo(HaveOccurred())

			By("Set OLSConfig CR to default")
			err := k8sClient.Get(ctx, crNamespacedName, cr)
			Expect(err).NotTo(HaveOccurred())
			crDefault := utils.GetDefaultOLSConfigCR()
			cr.Spec = crDefault.Spec

			By("create the OpenShift certificates config map")
			configmap, _ = utils.GenerateRandomConfigMap()
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
			By("Delete the provider secret")
			secretDeletionErr := testReconcilerInstance.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Delete the tls secret")
			secretDeletionErr = testReconcilerInstance.Delete(ctx, tlsSecret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Delete the user provided tls secret")
			secretDeletionErr = testReconcilerInstance.Delete(ctx, tlsUserSecret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Delete OpenShift certificates config map")
			configMapDeletionErr := testReconcilerInstance.Delete(ctx, configmap)
			Expect(configMapDeletionErr).NotTo(HaveOccurred())
		})

		It("should generate RAG volumes and initContainers when RAG is defined, remove them when RAG is not defined", func() {
			By("Reconcile with RAG defined")
			cr.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{
				{
					IndexPath: "/rag/vector_db/ocp_product_docs/4.19",
					IndexID:   "ocp-product-docs-4_19",
					Image:     "rag-ocp-product-docs:4.19",
				},
				{
					IndexPath: "/rag/vector_db/ansible_docs/2.18",
					IndexID:   "ansible-docs-2_18",
					Image:     "rag-ansible-docs:2.18",
				},
			}
			err := ReconcileAppServer(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())
			By("Check deployment have RAG volumes and initContainers")
			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerDeploymentName, Namespace: utils.OLSNamespaceDefault}, deployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Spec.Template.Spec.Volumes).To(ContainElement(corev1.Volume{
				Name: utils.RAGVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			}))
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).NotTo(ContainElement(corev1.VolumeMount{
				Name:      utils.RAGVolumeName,
				MountPath: utils.RAGVolumeMountPath,
				ReadOnly:  true,
			}))

			By("Reconcile without RAG defined")
			cr.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{}
			err = ReconcileAppServer(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())
			By("Check deployment does not have RAG volumes and initContainers")
			deployment = &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerDeploymentName, Namespace: utils.OLSNamespaceDefault}, deployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Spec.Template.Spec.Volumes).NotTo(ContainElement(corev1.Volume{
				Name: utils.RAGVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			}))
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).NotTo(ContainElement(corev1.VolumeMount{
				Name:      utils.RAGVolumeName,
				MountPath: utils.RAGVolumeMountPath,
				ReadOnly:  true,
			}))

		})

		It("should add RAG indexes into the configmap when RAG is defined", func() {
			By("Reconcile with RAG defined")
			cr.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{
				{
					IndexPath: "/rag/vector_db/ocp_product_docs/4.19",
					IndexID:   "ocp-product-docs-4_19",
					Image:     "rag-ocp-product-docs:4.19",
				},
				{
					IndexPath: "/rag/vector_db/ansible_docs/2.18",
					IndexID:   "ansible-docs-2_18",
					Image:     "rag-ansible-docs:2.18",
				},
			}
			err := ReconcileAppServer(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())
			By("Check configmap has RAG indexes")
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSConfigCmName, Namespace: utils.OLSNamespaceDefault}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Data).To(HaveKey(utils.OLSConfigFilename))
			major, minor, err := utils.GetOpenshiftVersion(k8sClient, ctx)
			Expect(err).NotTo(HaveOccurred())
			// OCP document is always there
			Expect(cm.Data[utils.OLSConfigFilename]).To(ContainSubstring("indexes:"))
			Expect(cm.Data[utils.OLSConfigFilename]).To(ContainSubstring("  - product_docs_index_id: " + "ocp-product-docs-" + major + "_" + minor))
			Expect(cm.Data[utils.OLSConfigFilename]).To(ContainSubstring("    product_docs_index_path: " + "/app-root/vector_db/ocp_product_docs/" + major + "." + minor))
			Expect(cm.Data[utils.OLSConfigFilename]).To(ContainSubstring("  - product_docs_index_id: ocp-product-docs-4_19"))
			Expect(cm.Data[utils.OLSConfigFilename]).To(ContainSubstring("    product_docs_index_path: " + utils.RAGVolumeMountPath + "/rag-0"))
			Expect(cm.Data[utils.OLSConfigFilename]).To(ContainSubstring("  - product_docs_index_id: ansible-docs-2_18"))
			Expect(cm.Data[utils.OLSConfigFilename]).To(ContainSubstring("    product_docs_index_path: " + utils.RAGVolumeMountPath + "/rag-1"))
		})

	})

	Context("Proxy Settings", Ordered, func() {
		var secret *corev1.Secret
		var volumeDefaultMode = int32(420)
		var cmCACert *corev1.ConfigMap
		var configmap *corev1.ConfigMap
		const cmCACertName = "proxy-ca-cert"
		BeforeEach(func() {
			By("create the provider secret")
			secret, _ = utils.GenerateRandomSecret()
			secret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       "test-secret",
				},
			})
			secretCreationErr := testReconcilerInstance.Create(ctx, secret)
			Expect(secretCreationErr).NotTo(HaveOccurred())

			By("create the tls secret")
			tlsSecret, _ = utils.GenerateRandomTLSSecret()
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

			By("create the config map for proxy CA cert")
			cmCACert = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cmCACertName,
					Namespace: utils.OLSNamespaceDefault,
				},
				Data: map[string]string{
					utils.ProxyCACertFileName: utils.TestCACert,
				},
			}
			err := testReconcilerInstance.Create(ctx, cmCACert)
			Expect(err).NotTo(HaveOccurred())

			By("create the OpenShift certificates config map")
			configmap, _ = utils.GenerateRandomConfigMap()
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

			By("Generate default CR")
			cr = utils.GetDefaultOLSConfigCR()
		})

		AfterEach(func() {
			By("Delete the provider secret")
			secretDeletionErr := testReconcilerInstance.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Delete the tls secret")
			secretDeletionErr = testReconcilerInstance.Delete(ctx, tlsSecret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Delete the config map for CA cert")
			err := testReconcilerInstance.Delete(ctx, cmCACert)
			Expect(err).NotTo(HaveOccurred())

			By("Delete OpenShift certificates config map")
			configMapDeletionErr := testReconcilerInstance.Delete(ctx, configmap)
			Expect(configMapDeletionErr).NotTo(HaveOccurred())
		})

		It("should update the configmap and deployment when changing the proxy CA cert", func() {

			By("Set up a proxy CA cert")
			cr.Spec.OLSConfig.ProxyConfig = &olsv1alpha1.ProxyConfig{
				ProxyURL: "https://proxy.example.com:8080",
				ProxyCACertificateRef: &corev1.LocalObjectReference{
					Name: cmCACertName,
				},
			}
			err := ReconcileAppServer(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("check OLS configmap has proxy_ca section")
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSConfigCmName, Namespace: utils.OLSNamespaceDefault}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Data).To(HaveKey(utils.OLSConfigFilename))
			Expect(cm.Data[utils.OLSConfigFilename]).To(ContainSubstring(fmt.Sprintf("proxy_ca_cert_path: %s", path.Join(utils.OLSAppCertsMountRoot, utils.ProxyCACertVolumeName, utils.ProxyCACertFileName))))

			By("check the proxy CA configmap has watcher annotation")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: cmCACertName, Namespace: utils.OLSNamespaceDefault}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Annotations).To(HaveKeyWithValue(utils.WatcherAnnotationKey, utils.OLSConfigName))

			By("Get app deployment and check the volume mount")
			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerDeploymentName, Namespace: utils.OLSNamespaceDefault}, deployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Spec.Template.Spec.Volumes).To(ContainElements(
				corev1.Volume{
					Name: utils.ProxyCACertVolumeName,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: cmCACertName,
							},
							DefaultMode: &volumeDefaultMode,
						},
					},
				},
			))
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).To(ContainElement(corev1.VolumeMount{
				Name:      utils.ProxyCACertVolumeName,
				MountPath: path.Join(utils.OLSAppCertsMountRoot, utils.ProxyCACertVolumeName),
				ReadOnly:  true,
			}))

		})

	})

	Context("MCP Headers", Ordered, func() {
		var volumeDefaultMode = int32(420)
		BeforeEach(func() {
			By("Set OLSConfig CR to default")
			err := k8sClient.Get(ctx, crNamespacedName, cr)
			Expect(err).NotTo(HaveOccurred())
			crDefault := utils.GetDefaultOLSConfigCR()
			cr.Spec = crDefault.Spec
		})

		It("should create additional volumes and volume mounts when MCP headers are defined", func() {
			cr.Spec.FeatureGates = []olsv1alpha1.FeatureGate{utils.FeatureGateMCPServer}
			cr.Spec.MCPServers = []olsv1alpha1.MCPServer{
				{
					Name: "testMCP",
					StreamableHTTP: &olsv1alpha1.MCPServerStreamableHTTPTransport{
						URL:            "https://testMCP.com",
						Timeout:        10,
						SSEReadTimeout: 10,
						Headers: map[string]string{
							"header1": "value1",
						},
					},
				},
				{
					Name: "testMCP2",
					StreamableHTTP: &olsv1alpha1.MCPServerStreamableHTTPTransport{
						URL:            "https://testMCP2.com",
						Timeout:        10,
						SSEReadTimeout: 10,
						Headers: map[string]string{
							"header2": "value2",
						},
						EnableSSE: true,
					},
				},
			}
			deployment, err := GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Spec.Template.Spec.Volumes).To(ContainElement(corev1.Volume{
				Name: "header-value1",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  "value1",
						DefaultMode: &volumeDefaultMode,
					},
				},
			}))
			Expect(deployment.Spec.Template.Spec.Volumes).To(ContainElement(corev1.Volume{
				Name: "header-value2",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  "value2",
						DefaultMode: &volumeDefaultMode,
					},
				},
			}))
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).To(ContainElement(corev1.VolumeMount{
				Name:      "header-value1",
				MountPath: "/etc/mcp/headers/value1",
				ReadOnly:  true,
			}))
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).To(ContainElement(corev1.VolumeMount{
				Name:      "header-value2",
				MountPath: "/etc/mcp/headers/value2",
				ReadOnly:  true,
			}))
		})
	})

})
