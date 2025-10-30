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

	})

	Context("LLM Credentials Validation", func() {
		It("should validate LLM credentials exist", func() {
			By("Create a test CR with LLM provider")
			testCR := utils.GetDefaultOLSConfigCR()

			By("Check LLM credentials - should succeed as the secret exists from BeforeAll")
			err := checkLLMCredentials(testReconcilerInstance, ctx, testCR)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail when LLM credentials secret is missing", func() {
			By("Create a test CR with non-existent secret")
			testCR := utils.GetDefaultOLSConfigCR()
			testCR.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name = "non-existent-secret"

			By("Check LLM credentials - should fail")
			err := checkLLMCredentials(testReconcilerInstance, ctx, testCR)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("credential secret non-existent-secret not found"))
		})

		It("should fail when provider is missing credentials secret name", func() {
			By("Create a test CR with empty secret name")
			testCR := utils.GetDefaultOLSConfigCR()
			testCR.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name = ""

			By("Check LLM credentials - should fail")
			err := checkLLMCredentials(testReconcilerInstance, ctx, testCR)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing credentials secret"))
		})
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

		It("should annotate additional CA configmap with watcher annotation", func() {
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

			By("Check the additional CA configmap has watcher annotation")
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: additionalCACMName, Namespace: utils.OLSNamespaceDefault}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Annotations).To(HaveKeyWithValue(utils.WatcherAnnotationKey, utils.OLSConfigName))
		})

		It("should skip reconciliation when additional CA is not configured", func() {
			By("Remove additional CA from CR")
			err := k8sClient.Get(ctx, crNamespacedName, cr)
			Expect(err).NotTo(HaveOccurred())
			cr.Spec.OLSConfig.AdditionalCAConfigMapRef = nil
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
				ProxyCACertificateRef: &corev1.LocalObjectReference{
					Name: proxyCACMName,
				},
			}
			// LCore requires supported Llama Stack provider types
			cr.Spec.LLMConfig.Providers[0].Type = "openai"
			err = k8sClient.Update(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile the proxy CA ConfigMap")
			err = ReconcileLCore(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Check the proxy CA configmap has watcher annotation")
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: proxyCACMName, Namespace: utils.OLSNamespaceDefault}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Annotations).To(HaveKeyWithValue(utils.WatcherAnnotationKey, utils.OLSConfigName))
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
})
