package lcore

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("LCore reconciliator", Ordered, func() {

	Context("Creation logic", Ordered, func() {
		BeforeAll(func() {
			By("set the OLSConfig custom resource to default")
			err := k8sClient.Get(ctx, crNamespacedName, cr)
			Expect(err).NotTo(HaveOccurred())
			crDefault := utils.GetDefaultOLSConfigCR()
			cr.Spec = crDefault.Spec
			// LCore requires supported Llama Stack provider types
			cr.Spec.LLMConfig.Providers[0].Type = "openai"
		})

		It("should reconcile from OLSConfig custom resource", func() {
			By("Reconcile the OLSConfig custom resource")
			err := ReconcileLCore(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())
			// Note: Status conditions are managed by the main OLSConfigReconciler,
			// not by the component-specific reconcilers
		})

		It("should create a service account lightspeed-app-server", func() {
			By("Get the service account")
			sa := &corev1.ServiceAccount{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerServiceAccountName, Namespace: utils.OLSNamespaceDefault}, sa)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a cluster role lightspeed-app-server-sar", func() {
			By("Get the cluster role")
			cr := &rbacv1.ClusterRole{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerSARRoleName}, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(cr.Rules).NotTo(BeEmpty())
		})

		It("should create a cluster role binding lightspeed-app-server-sar", func() {
			By("Get the cluster role binding")
			crb := &rbacv1.ClusterRoleBinding{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerSARRoleBindingName}, crb)
			Expect(err).NotTo(HaveOccurred())
			Expect(crb.Subjects).NotTo(BeEmpty())
			Expect(crb.RoleRef.Name).To(Equal(utils.OLSAppServerSARRoleName))
		})

		It("should create a config map llama-stack-config", func() {
			By("Get the config map")
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.LlamaStackConfigCmName, Namespace: utils.OLSNamespaceDefault}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Data).To(HaveKey(utils.LlamaStackConfigFilename))
		})

		It("should create a config map lightspeed-stack-config", func() {
			By("Get the config map")
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.LCoreConfigCmName, Namespace: utils.OLSNamespaceDefault}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Data).To(HaveKey(utils.LCoreConfigFilename))
		})

		It("should create a service lightspeed-app-server", func() {
			By("Get the service")
			svc := &corev1.Service{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerServiceName, Namespace: utils.OLSNamespaceDefault}, svc)
			Expect(err).NotTo(HaveOccurred())
			Expect(svc.Spec.Ports).NotTo(BeEmpty())
		})

		It("should create a deployment lightspeed-stack-deployment", func() {
			By("Get the deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.LCoreDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal(utils.LlamaStackContainerName))
			Expect(dep.Spec.Template.Spec.Containers[1].Name).To(Equal(utils.LCoreContainerName))
		})

		It("should create a metrics reader secret", func() {
			By("Get the metrics reader secret")
			secret := &corev1.Secret{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.MetricsReaderServiceAccountTokenSecretName, Namespace: utils.OLSNamespaceDefault}, secret)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a service monitor lightspeed-app-server-monitor", func() {
			By("Get the service monitor")
			sm := &monv1.ServiceMonitor{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.AppServerServiceMonitorName, Namespace: utils.OLSNamespaceDefault}, sm)
			Expect(err).NotTo(HaveOccurred())
			Expect(sm.Spec.Endpoints).NotTo(BeEmpty())
		})

		It("should create a prometheus rule lightspeed-app-server-prometheus-rule", func() {
			By("Get the prometheus rule")
			pr := &monv1.PrometheusRule{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.AppServerPrometheusRuleName, Namespace: utils.OLSNamespaceDefault}, pr)
			Expect(err).NotTo(HaveOccurred())
			Expect(pr.Spec.Groups).NotTo(BeEmpty())
		})

		It("should create a network policy lightspeed-app-server", func() {
			By("Get the network policy")
			np := &networkingv1.NetworkPolicy{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSAppServerNetworkPolicyName, Namespace: utils.OLSNamespaceDefault}, np)
			Expect(err).NotTo(HaveOccurred())
		})

		// Note: LLM credential validation is now done in annotateExternalResources
		// and tested in utils_misc_test.go and olsconfig_helpers_test.go

	})

	Context("Additional CA ConfigMap reconciliation", Ordered, func() {
		const additionalCACMName = "additional-ca-test"

		BeforeAll(func() {
			By("Create an additional CA ConfigMap")
			cm := &corev1.ConfigMap{}
			cm.Name = additionalCACMName
			cm.Namespace = utils.OLSNamespaceDefault
			cm.Data = map[string]string{
				"ca-cert.crt": "test-ca-cert-content",
			}
			err := k8sClient.Create(ctx, cm)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterAll(func() {
			By("Delete the additional CA ConfigMap")
			cm := &corev1.ConfigMap{}
			cm.Name = additionalCACMName
			cm.Namespace = utils.OLSNamespaceDefault
			_ = k8sClient.Delete(ctx, cm)
		})

		It("should reconcile additional CA configmap", func() {
			By("Set up an additional CA cert in CR")
			err := k8sClient.Get(ctx, crNamespacedName, cr)
			Expect(err).NotTo(HaveOccurred())
			cr.Spec.OLSConfig.AdditionalCAConfigMapRef = &corev1.LocalObjectReference{
				Name: additionalCACMName,
			}
			// LCore requires supported Llama Stack provider types
			cr.Spec.LLMConfig.Providers[0].Type = "openai"
			err = k8sClient.Update(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile the additional CA ConfigMap")
			err = ReconcileLCore(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Verify the additional CA configmap exists")
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: additionalCACMName, Namespace: utils.OLSNamespaceDefault}, cm)
			Expect(err).NotTo(HaveOccurred())
			// Note: Annotation is now handled by main controller, not component reconciler
		})

		It("should skip reconciliation when additional CA is not configured", func() {
			By("Remove additional CA from CR")
			err := k8sClient.Get(ctx, crNamespacedName, cr)
			Expect(err).NotTo(HaveOccurred())
			cr.Spec.OLSConfig.AdditionalCAConfigMapRef = nil
			// LCore requires supported Llama Stack provider types
			cr.Spec.LLMConfig.Providers[0].Type = "openai"
			err = k8sClient.Update(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile should succeed and skip")
			err = ReconcileLCore(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reconcile successfully with additional CA configured", func() {
			By("Set up an additional CA cert in CR")
			err := k8sClient.Get(ctx, crNamespacedName, cr)
			Expect(err).NotTo(HaveOccurred())
			cr.Spec.OLSConfig.AdditionalCAConfigMapRef = &corev1.LocalObjectReference{
				Name: additionalCACMName,
			}
			// LCore requires supported Llama Stack provider types
			cr.Spec.LLMConfig.Providers[0].Type = "openai"
			err = k8sClient.Update(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile should succeed")
			err = ReconcileLCore(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Second reconciliation should also succeed")
			err = ReconcileLCore(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Proxy CA ConfigMap reconciliation", Ordered, func() {
		const proxyCACMName = "proxy-ca-test"

		BeforeAll(func() {
			By("Reset the CR to default state")
			err := k8sClient.Get(ctx, crNamespacedName, cr)
			Expect(err).NotTo(HaveOccurred())
			crDefault := utils.GetDefaultOLSConfigCR()
			cr.Spec = crDefault.Spec
			err = k8sClient.Update(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Create a proxy CA ConfigMap")
			cm := &corev1.ConfigMap{}
			cm.Name = proxyCACMName
			cm.Namespace = utils.OLSNamespaceDefault
			cm.Data = map[string]string{
				"ca-bundle.crt": "test-proxy-ca-cert-content",
			}
			err = k8sClient.Create(ctx, cm)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterAll(func() {
			By("Delete the proxy CA ConfigMap")
			cm := &corev1.ConfigMap{}
			cm.Name = proxyCACMName
			cm.Namespace = utils.OLSNamespaceDefault
			_ = k8sClient.Delete(ctx, cm)
		})

		It("should annotate proxy CA configmap with watcher annotation", func() {
			By("Set up a proxy CA cert in CR")
			err := k8sClient.Get(ctx, crNamespacedName, cr)
			Expect(err).NotTo(HaveOccurred())
			cr.Spec.OLSConfig.ProxyConfig = &olsv1alpha1.ProxyConfig{
				ProxyURL: "https://proxy.example.com:8443",
				ProxyCACertificateRef: &olsv1alpha1.ProxyCACertConfigMapRef{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: proxyCACMName,
					},
					// Key is omitted - will default to "proxy-ca.crt" for backward compatibility
				},
			}
			// LCore requires supported Llama Stack provider types
			cr.Spec.LLMConfig.Providers[0].Type = "openai"
			err = k8sClient.Update(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile the proxy CA ConfigMap")
			err = ReconcileLCore(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Verify the proxy CA configmap exists")
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: proxyCACMName, Namespace: utils.OLSNamespaceDefault}, cm)
			Expect(err).NotTo(HaveOccurred())
			// Note: Annotation is now handled by main controller, not component reconciler
		})

		It("should skip reconciliation when proxy CA is not configured", func() {
			By("Remove proxy CA from CR")
			err := k8sClient.Get(ctx, crNamespacedName, cr)
			Expect(err).NotTo(HaveOccurred())
			cr.Spec.OLSConfig.ProxyConfig = nil
			err = k8sClient.Update(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile should succeed and skip")
			err = ReconcileLCore(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should skip reconciliation when proxy config exists but CA ref is nil", func() {
			By("Set up proxy config without CA ref")
			err := k8sClient.Get(ctx, crNamespacedName, cr)
			Expect(err).NotTo(HaveOccurred())
			cr.Spec.OLSConfig.ProxyConfig = &olsv1alpha1.ProxyConfig{
				ProxyURL:              "http://proxy.example.com:8080",
				ProxyCACertificateRef: nil,
			}
			err = k8sClient.Update(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile should succeed and skip")
			err = ReconcileLCore(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Data collector exporter ConfigMap reconciliation", Ordered, func() {
		BeforeAll(func() {
			By("Create telemetry pull secret")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      utils.TelemetryPullSecretName,
					Namespace: utils.TelemetryPullSecretNamespace,
				},
				Data: map[string][]byte{
					".dockerconfigjson": []byte(`{"auths":{"cloud.openshift.com":{}}}`),
				},
			}
			err := k8sClient.Create(ctx, secret)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterAll(func() {
			By("Delete telemetry pull secret")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      utils.TelemetryPullSecretName,
					Namespace: utils.TelemetryPullSecretNamespace,
				},
			}
			_ = k8sClient.Delete(ctx, secret)
		})

		It("should skip exporter ConfigMap creation when data collection is disabled", func() {
			By("Create CR with data collection disabled")
			err := k8sClient.Get(ctx, crNamespacedName, cr)
			Expect(err).NotTo(HaveOccurred())
			cr.Spec.OLSConfig.UserDataCollection = olsv1alpha1.UserDataCollectionSpec{
				FeedbackDisabled:    true,
				TranscriptsDisabled: true,
			}
			// LCore requires supported Llama Stack provider types
			cr.Spec.LLMConfig.Providers[0].Type = "openai"
			err = k8sClient.Update(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile should succeed")
			err = ReconcileLCore(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Exporter ConfigMap should not exist")
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.ExporterConfigCmName, Namespace: utils.OLSNamespaceDefault}, cm)
			Expect(err).To(HaveOccurred())
		})

		It("should create exporter ConfigMap when data collection is enabled and telemetry is available", func() {
			By("Create CR with data collection enabled")
			err := k8sClient.Get(ctx, crNamespacedName, cr)
			Expect(err).NotTo(HaveOccurred())
			cr.Spec.OLSConfig.UserDataCollection = olsv1alpha1.UserDataCollectionSpec{
				FeedbackDisabled:    false,
				TranscriptsDisabled: false,
			}
			// LCore requires supported Llama Stack provider types
			cr.Spec.LLMConfig.Providers[0].Type = "openai"
			err = k8sClient.Update(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile should succeed")
			err = ReconcileLCore(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Exporter ConfigMap should exist")
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.ExporterConfigCmName, Namespace: utils.OLSNamespaceDefault}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Data).To(HaveKey(utils.ExporterConfigFilename))
			Expect(cm.Data[utils.ExporterConfigFilename]).To(ContainSubstring("service_id"))
			Expect(cm.Data[utils.ExporterConfigFilename]).To(ContainSubstring("ingress_server_url"))
		})

		It("should delete exporter ConfigMap when data collection is disabled after being enabled", func() {
			By("Update CR to disable data collection")
			err := k8sClient.Get(ctx, crNamespacedName, cr)
			Expect(err).NotTo(HaveOccurred())
			cr.Spec.OLSConfig.UserDataCollection = olsv1alpha1.UserDataCollectionSpec{
				FeedbackDisabled:    true,
				TranscriptsDisabled: true,
			}
			err = k8sClient.Update(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile should succeed")
			err = ReconcileLCore(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Exporter ConfigMap should be deleted")
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.ExporterConfigCmName, Namespace: utils.OLSNamespaceDefault}, cm)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("MCP Server ConfigMap ResourceVersion tracking", Ordered, func() {
		BeforeAll(func() {
			By("Reset the CR to default state with supported provider")
			err := k8sClient.Get(ctx, crNamespacedName, cr)
			Expect(err).NotTo(HaveOccurred())
			crDefault := utils.GetDefaultOLSConfigCR()
			cr.Spec = crDefault.Spec
			// LCore requires supported Llama Stack provider types
			cr.Spec.LLMConfig.Providers[0].Type = "openai"
			err = k8sClient.Update(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile should succeed")
			err = ReconcileLCore(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should track MCP Server ConfigMap ResourceVersion and trigger restart on introspection toggle", func() {
			By("Disable introspection initially")
			err := k8sClient.Get(ctx, crNamespacedName, cr)
			Expect(err).NotTo(HaveOccurred())
			cr.Spec.OLSConfig.IntrospectionEnabled = false
			err = k8sClient.Update(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile with introspection disabled")
			err = ReconcileLCore(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Verify MCP Server ConfigMap does not exist")
			mcpCm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.OpenShiftMCPServerConfigCmName, Namespace: utils.OLSNamespaceDefault}, mcpCm)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "MCP ConfigMap should not exist when introspection is disabled")

			By("Get LCore deployment and verify MCP annotation is empty")
			dep := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.LCoreDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			mcpAnnotation := dep.Annotations[utils.OpenShiftMCPServerConfigMapResourceVersionAnnotation]
			Expect(mcpAnnotation).To(BeEmpty(), "MCP annotation should be empty when ConfigMap doesn't exist")

			By("Enable introspection")
			err = k8sClient.Get(ctx, crNamespacedName, cr)
			Expect(err).NotTo(HaveOccurred())
			cr.Spec.OLSConfig.IntrospectionEnabled = true
			err = k8sClient.Update(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile with introspection enabled")
			err = ReconcileLCore(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Verify MCP Server ConfigMap was created")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.OpenShiftMCPServerConfigCmName, Namespace: utils.OLSNamespaceDefault}, mcpCm)
			Expect(err).NotTo(HaveOccurred(), "MCP ConfigMap should exist after enabling introspection")
			firstResourceVersion := mcpCm.ResourceVersion
			Expect(firstResourceVersion).NotTo(BeEmpty())

			By("Verify deployment annotation tracks MCP ConfigMap ResourceVersion")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.LCoreDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			mcpAnnotation = dep.Annotations[utils.OpenShiftMCPServerConfigMapResourceVersionAnnotation]
			Expect(mcpAnnotation).To(Equal(firstResourceVersion), "Deployment annotation should match ConfigMap ResourceVersion")

			By("Disable introspection again")
			err = k8sClient.Get(ctx, crNamespacedName, cr)
			Expect(err).NotTo(HaveOccurred())
			cr.Spec.OLSConfig.IntrospectionEnabled = false
			err = k8sClient.Update(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile with introspection disabled again")
			err = ReconcileLCore(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Verify MCP Server ConfigMap was deleted")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.OpenShiftMCPServerConfigCmName, Namespace: utils.OLSNamespaceDefault}, mcpCm)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "MCP ConfigMap should be deleted when introspection is disabled")

			By("Verify deployment annotation changed (triggering restart)")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.LCoreDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			newMcpAnnotation := dep.Annotations[utils.OpenShiftMCPServerConfigMapResourceVersionAnnotation]
			Expect(newMcpAnnotation).NotTo(Equal(firstResourceVersion), "Annotation should change when ConfigMap is deleted to trigger pod restart")

			By("Verify pod template restart annotation is set")
			forceReloadValue := dep.Spec.Template.Annotations[utils.ForceReloadAnnotationKey]
			Expect(forceReloadValue).NotTo(BeEmpty(), "Pod template restart annotation should be set to trigger rolling update")
		})

		It("should trigger rolling update when MCP ConfigMap is modified externally", func() {
			By("Enable introspection")
			err := k8sClient.Get(ctx, crNamespacedName, cr)
			Expect(err).NotTo(HaveOccurred())
			cr.Spec.OLSConfig.IntrospectionEnabled = true
			err = k8sClient.Update(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile to create MCP ConfigMap")
			err = ReconcileLCore(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Get the MCP ConfigMap and capture initial ResourceVersion")
			mcpCm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.OpenShiftMCPServerConfigCmName, Namespace: utils.OLSNamespaceDefault}, mcpCm)
			Expect(err).NotTo(HaveOccurred())
			initialResourceVersion := mcpCm.ResourceVersion

			By("Get deployment and capture initial annotation")
			dep := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.LCoreDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			initialAnnotation := dep.Annotations[utils.OpenShiftMCPServerConfigMapResourceVersionAnnotation]
			Expect(initialAnnotation).To(Equal(initialResourceVersion))

			By("Manually modify MCP ConfigMap data (simulating external change)")
			mcpCm.Data["mcp-server-config.toml"] = "# Modified externally"
			err = k8sClient.Update(ctx, mcpCm)
			Expect(err).NotTo(HaveOccurred())

			By("Get updated ResourceVersion after manual modification")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.OpenShiftMCPServerConfigCmName, Namespace: utils.OLSNamespaceDefault}, mcpCm)
			Expect(err).NotTo(HaveOccurred())
			modifiedResourceVersion := mcpCm.ResourceVersion
			Expect(modifiedResourceVersion).NotTo(Equal(initialResourceVersion), "ResourceVersion should change after update")

			By("Reconcile again to correct the ConfigMap")
			err = ReconcileLCore(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Verify ConfigMap was corrected and has new ResourceVersion")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.OpenShiftMCPServerConfigCmName, Namespace: utils.OLSNamespaceDefault}, mcpCm)
			Expect(err).NotTo(HaveOccurred())
			correctedResourceVersion := mcpCm.ResourceVersion
			Expect(correctedResourceVersion).NotTo(Equal(modifiedResourceVersion), "ResourceVersion should change after reconciler correction")
			Expect(mcpCm.Data["mcp-server-config.toml"]).NotTo(ContainSubstring("Modified externally"), "ConfigMap should be corrected to proper content")

			By("Verify deployment annotation updated to new ResourceVersion")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: utils.LCoreDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			newAnnotation := dep.Annotations[utils.OpenShiftMCPServerConfigMapResourceVersionAnnotation]
			Expect(newAnnotation).To(Equal(correctedResourceVersion), "Deployment annotation should track corrected ConfigMap ResourceVersion")
			Expect(newAnnotation).NotTo(Equal(initialAnnotation), "Deployment annotation should change to trigger restart")

			By("Verify pod template restart annotation is set")
			forceReloadValue := dep.Spec.Template.Annotations[utils.ForceReloadAnnotationKey]
			Expect(forceReloadValue).NotTo(BeEmpty(), "Pod template restart annotation should be set to trigger rolling update")
		})
	})
})
