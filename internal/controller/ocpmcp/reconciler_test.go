package ocpmcp

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func expectOwnedByOLSConfig(obj metav1.Object) {
	olsConfig := &olsv1alpha1.OLSConfig{}
	Expect(k8sClient.Get(ctx, crNamespacedName, olsConfig)).To(Succeed())

	var ownerRef *metav1.OwnerReference
	for i := range obj.GetOwnerReferences() {
		ref := &obj.GetOwnerReferences()[i]
		if ref.APIVersion == utils.OLSConfigAPIVersion &&
			ref.Kind == utils.OLSConfigKind &&
			ref.Name == olsConfig.Name {
			ownerRef = ref
			break
		}
	}
	Expect(ownerRef).NotTo(BeNil(), "expected %T %s to be owned by OLSConfig", obj, obj.GetName())
	Expect(ownerRef.Name).To(Equal(olsConfig.Name))
}

var _ = Describe("OpenShift MCP Server reconciler", Ordered, func() {
	var testCR *olsv1alpha1.OLSConfig

	BeforeAll(func() {
		testCR = cr.DeepCopy()
		testCR.Spec.OLSConfig.IntrospectionEnabled = utils.BoolPtr(true)
	})

	Context("Phase 1 resources", func() {
		BeforeAll(func() {
			err := ReconcileResources(testReconcilerInstance, ctx, testCR)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create the MCP ConfigMap", func() {
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OpenShiftMCPServerConfigCmName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(cm)
			Expect(cm.Data[utils.OpenShiftMCPServerConfigFilename]).To(ContainSubstring(`kind = "Secret"`))
		})

		It("should create the MCP CA ConfigMap with inject-cabundle", func() {
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OpenShiftMCPServerCAConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(cm)
			Expect(cm.Annotations[utils.InjectCABundleAnnotationKey]).To(Equal("true"))
		})

		It("should create the MCP ServiceAccount", func() {
			sa := &corev1.ServiceAccount{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OpenShiftMCPServerServiceAccountName,
				Namespace: utils.OLSNamespaceDefault,
			}, sa)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(sa)
		})

		It("should create the MCP NetworkPolicy", func() {
			np := &networkingv1.NetworkPolicy{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OpenShiftMCPServerNetworkPolicyName,
				Namespace: utils.OLSNamespaceDefault,
			}, np)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(np)
		})

		It("should skip ConfigMap update when data is unchanged", func() {
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OpenShiftMCPServerConfigCmName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			Expect(err).NotTo(HaveOccurred())
			oldRV := cm.ResourceVersion

			err = ReconcileResources(testReconcilerInstance, ctx, testCR)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OpenShiftMCPServerConfigCmName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.ResourceVersion).To(Equal(oldRV))
		})

		It("should not overwrite CA ConfigMap Data on reconcile", func() {
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OpenShiftMCPServerCAConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			Expect(err).NotTo(HaveOccurred())
			cm.Data = map[string]string{utils.OpenShiftMCPServerCACertKey: "injected-by-service-ca"}
			Expect(k8sClient.Update(ctx, cm)).To(Succeed())

			err = ReconcileResources(testReconcilerInstance, ctx, testCR)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OpenShiftMCPServerCAConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Data[utils.OpenShiftMCPServerCACertKey]).To(Equal("injected-by-service-ca"))
		})

		It("should preserve foreign CA ConfigMap annotations on reconcile", func() {
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OpenShiftMCPServerCAConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			Expect(err).NotTo(HaveOccurred())
			if cm.Annotations == nil {
				cm.Annotations = map[string]string{}
			}
			cm.Annotations["service.beta.openshift.io/inject-cabundle-status"] = "injected"
			Expect(k8sClient.Update(ctx, cm)).To(Succeed())

			err = ReconcileResources(testReconcilerInstance, ctx, testCR)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OpenShiftMCPServerCAConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Annotations[utils.InjectCABundleAnnotationKey]).To(Equal("true"))
			Expect(cm.Annotations["service.beta.openshift.io/inject-cabundle-status"]).To(Equal("injected"))
		})
	})

	Context("Phase 2 deployment", func() {
		BeforeAll(func() {
			ensureMCPTLSSecret()
			err := ReconcileDeployment(testReconcilerInstance, ctx, testCR)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create the MCP Service", func() {
			svc := &corev1.Service{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OpenShiftMCPServerServiceName,
				Namespace: utils.OLSNamespaceDefault,
			}, svc)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(svc)
			Expect(svc.Annotations[utils.ServingCertSecretAnnotationKey]).To(Equal(utils.OpenShiftMCPServerCertsSecretName))
		})

		It("should create the MCP Deployment", func() {
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OpenShiftMCPServerDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, dep)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(dep)
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal(utils.OpenShiftMCPServerImageDefault))
			Expect(dep.Annotations).To(HaveKey(utils.OpenShiftMCPServerConfigMapResourceVersionAnnotation))
			Expect(dep.Annotations).To(HaveKey(utils.OpenShiftMCPServerTLSSecretResourceVersionAnnotation))
		})

		It("should skip Deployment update when spec and versions are unchanged", func() {
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OpenShiftMCPServerDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, dep)
			Expect(err).NotTo(HaveOccurred())
			oldRV := dep.ResourceVersion
			oldForceReload := dep.Spec.Template.Annotations[utils.ForceReloadAnnotationKey]

			err = ReconcileDeployment(testReconcilerInstance, ctx, testCR)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OpenShiftMCPServerDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.ResourceVersion).To(Equal(oldRV))
			Expect(dep.Spec.Template.Annotations[utils.ForceReloadAnnotationKey]).To(Equal(oldForceReload))
		})

		It("should trigger a rolling restart via Restart", func() {
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OpenShiftMCPServerDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, dep)
			Expect(err).NotTo(HaveOccurred())

			err = Restart(testReconcilerInstance, ctx, dep)
			Expect(err).NotTo(HaveOccurred())

			updated := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OpenShiftMCPServerDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, updated)
			Expect(err).NotTo(HaveOccurred())
			Expect(updated.Spec.Template.Annotations).To(HaveKey(utils.ForceReloadAnnotationKey))
		})

		It("should skip Restart when the Deployment is missing", func() {
			Expect(k8sClient.Delete(ctx, &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      utils.OpenShiftMCPServerDeploymentName,
					Namespace: utils.OLSNamespaceDefault,
				},
			})).To(Succeed())

			err := Restart(testReconcilerInstance, ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should remove MCP resources when introspection is disabled", func() {
			// Prior tests may have deleted the Deployment; recreate so cleanup is exercised.
			ensureMCPTLSSecret()
			Expect(ReconcileDeployment(testReconcilerInstance, ctx, testCR)).To(Succeed())

			disabledCR := testCR.DeepCopy()
			disabledCR.Spec.OLSConfig.IntrospectionEnabled = utils.BoolPtr(false)

			err := ReconcileResources(testReconcilerInstance, ctx, disabledCR)
			Expect(err).NotTo(HaveOccurred())

			err = ReconcileDeployment(testReconcilerInstance, ctx, disabledCR)
			Expect(err).NotTo(HaveOccurred())

			dep := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OpenShiftMCPServerDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, dep)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "deployment should be deleted")

			svc := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OpenShiftMCPServerServiceName,
				Namespace: utils.OLSNamespaceDefault,
			}, svc)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "service should be deleted")

			for _, name := range []string{
				utils.OpenShiftMCPServerConfigCmName,
				utils.OpenShiftMCPServerCAConfigMapName,
			} {
				cm := &corev1.ConfigMap{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: utils.OLSNamespaceDefault}, cm)
				Expect(apierrors.IsNotFound(err)).To(BeTrue(), "configmap %s should be deleted", name)
			}

			sa := &corev1.ServiceAccount{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OpenShiftMCPServerServiceAccountName,
				Namespace: utils.OLSNamespaceDefault,
			}, sa)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "service account should be deleted")

			np := &networkingv1.NetworkPolicy{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OpenShiftMCPServerNetworkPolicyName,
				Namespace: utils.OLSNamespaceDefault,
			}, np)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "network policy should be deleted")

			tlsSecret := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OpenShiftMCPServerCertsSecretName,
				Namespace: utils.OLSNamespaceDefault,
			}, tlsSecret)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "TLS secret should be deleted")
		})
	})
})
