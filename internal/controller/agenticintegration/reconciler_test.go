package agenticintegration

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

var _ = Describe("Agentic integration reconciler", Ordered, func() {
	var testCR *olsv1alpha1.OLSConfig

	BeforeAll(func() {
		testCR = cr.DeepCopy()
		testCR.Spec.OLSConfig.IntrospectionEnabled = utils.BoolPtr(false)
	})

	Context("create gating", func() {
		It("should not create the handoff ConfigMap when OTEL Service is missing", func() {
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AgenticConfigurationConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			if err == nil {
				Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
			} else {
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}

			// Other specs may have created prerequisites; remove OTEL Service for this gate.
			otelSvc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
				Name: utils.OtelCollectorServiceName, Namespace: utils.OLSNamespaceDefault,
			}}
			_ = k8sClient.Delete(ctx, otelSvc)
			defer ensureHandoffCreatePrerequisites(false)

			gated := testCR.DeepCopy()
			gated.Spec.OLSConfig.IntrospectionEnabled = utils.BoolPtr(false)
			err = ReconcileAgenticIntegrationResources(testReconcilerInstance, ctx, gated)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(utils.ErrAgenticConfigurationPrerequisitesNotReady))

			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AgenticConfigurationConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})
	})

	Context("with introspection disabled", func() {
		BeforeAll(func() {
			ensureHandoffCreatePrerequisites(false)
			err := ReconcileAgenticIntegrationResources(testReconcilerInstance, ctx, testCR)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create the handoff ConfigMap without MCP keys", func() {
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AgenticConfigurationConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(cm)
			Expect(cm.Data[utils.AgenticConfigurationSandboxModeKey]).To(Equal(string(olsv1alpha1.SandboxModeBarePod)))
			Expect(cm.Data).To(HaveKey(utils.AgenticConfigurationSandboxPodSpecKey))
			Expect(cm.Data[utils.AgenticConfigurationOtelCASecretKey]).To(Equal(utils.AgenticOtelCASecretName))
			Expect(cm.Data).NotTo(HaveKey(utils.AgenticConfigurationMCPEndpointKey))
		})

		It("should skip ConfigMap update when unchanged", func() {
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AgenticConfigurationConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			Expect(err).NotTo(HaveOccurred())
			oldRV := cm.ResourceVersion

			err = ReconcileAgenticIntegrationResources(testReconcilerInstance, ctx, testCR)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AgenticConfigurationConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.ResourceVersion).To(Equal(oldRV))
		})

		It("should preserve cert-reload annotation across reconcile", func() {
			Expect(TouchAgenticConfiguration(testReconcilerInstance, ctx)).To(Succeed())

			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AgenticConfigurationConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)).To(Succeed())
			annotation := cm.Annotations[utils.AgenticConfigurationCertReloadAnnotation]
			Expect(annotation).NotTo(BeEmpty())

			Expect(ReconcileAgenticIntegrationResources(testReconcilerInstance, ctx, testCR)).To(Succeed())

			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AgenticConfigurationConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)).To(Succeed())
			Expect(cm.Annotations[utils.AgenticConfigurationCertReloadAnnotation]).To(Equal(annotation))
		})

		It("should update sandbox-mode when ConfigMap exists even if OTEL Service is gone", func() {
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      utils.OtelCollectorServiceName,
					Namespace: utils.OLSNamespaceDefault,
				},
			}
			Expect(k8sClient.Delete(ctx, svc)).To(Succeed())
			defer ensureHandoffCreatePrerequisites(false)

			updated := testCR.DeepCopy()
			updated.Spec.AgenticOLS = &olsv1alpha1.AgenticOLSSpec{SandboxMode: olsv1alpha1.SandboxModeSandboxClaim}
			Expect(ReconcileAgenticIntegrationResources(testReconcilerInstance, ctx, updated)).To(Succeed())

			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AgenticConfigurationConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)).To(Succeed())
			Expect(cm.Data[utils.AgenticConfigurationSandboxModeKey]).To(Equal(string(olsv1alpha1.SandboxModeSandboxClaim)))
		})
	})

	Context("with introspection enabled", func() {
		BeforeAll(func() {
			ensureHandoffCreatePrerequisites(true)
			testCR.Spec.OLSConfig.IntrospectionEnabled = utils.BoolPtr(true)
			testCR.Spec.AgenticOLS = nil
			err := ReconcileAgenticIntegrationResources(testReconcilerInstance, ctx, testCR)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should include MCP keys in the ConfigMap", func() {
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AgenticConfigurationConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Data).To(HaveKey(utils.AgenticConfigurationMCPEndpointKey))
			Expect(cm.Data[utils.AgenticConfigurationMCPCASecretKey]).To(Equal(utils.AgenticMCPCASecretName))
		})

		It("should omit MCP keys when introspection is disabled", func() {
			disabled := testCR.DeepCopy()
			disabled.Spec.OLSConfig.IntrospectionEnabled = utils.BoolPtr(false)
			err := ReconcileAgenticIntegrationResources(testReconcilerInstance, ctx, disabled)
			Expect(err).NotTo(HaveOccurred())

			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AgenticConfigurationConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Data).NotTo(HaveKey(utils.AgenticConfigurationMCPEndpointKey))
		})
	})
})
