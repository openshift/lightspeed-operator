package rhokp

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

var _ = Describe("RHOKP reconciler", Ordered, func() {
	var testCR *olsv1alpha1.OLSConfig

	BeforeAll(func() {
		testCR = cr.DeepCopy()
		testCR.Spec.OLSConfig.ByokRAGOnly = false
	})

	Context("Phase 1 resources", func() {
		BeforeAll(func() {
			err := ReconcileResources(testReconcilerInstance, ctx, testCR)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create the RHOKP NetworkPolicy", func() {
			np := &networkingv1.NetworkPolicy{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.RHOKPNetworkPolicyName,
				Namespace: utils.OLSNamespaceDefault,
			}, np)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(np)
		})

		It("should skip NetworkPolicy update when spec is unchanged", func() {
			np := &networkingv1.NetworkPolicy{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.RHOKPNetworkPolicyName,
				Namespace: utils.OLSNamespaceDefault,
			}, np)
			Expect(err).NotTo(HaveOccurred())
			oldRV := np.ResourceVersion

			err = ReconcileResources(testReconcilerInstance, ctx, testCR)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.RHOKPNetworkPolicyName,
				Namespace: utils.OLSNamespaceDefault,
			}, np)
			Expect(err).NotTo(HaveOccurred())
			Expect(np.ResourceVersion).To(Equal(oldRV))
		})
	})

	Context("Phase 2 deployment", func() {
		BeforeAll(func() {
			ensureRHOKPTLSSecret()
			err := ReconcileDeployment(testReconcilerInstance, ctx, testCR)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create the RHOKP Service with serving-cert annotation", func() {
			svc := &corev1.Service{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.RHOKPServiceName,
				Namespace: utils.OLSNamespaceDefault,
			}, svc)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(svc)
			Expect(svc.Annotations[utils.ServingCertSecretAnnotationKey]).To(Equal(utils.RHOKPCertsSecretName))
		})

		It("should create the RHOKP Deployment", func() {
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.RHOKPDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, dep)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(dep)
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal(utils.RHOOKPImageDefault))
			Expect(dep.Annotations).To(HaveKey(utils.RHOKPTLSSecretResourceVersionAnnotation))
		})

		It("should mount TLS as localhost.crt/localhost.key for Apache httpd", func() {
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.RHOKPDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, dep)
			Expect(err).NotTo(HaveOccurred())

			var tlsVolume *corev1.Volume
			for i := range dep.Spec.Template.Spec.Volumes {
				if dep.Spec.Template.Spec.Volumes[i].Name == utils.RHOKPTLSVolumeName {
					tlsVolume = &dep.Spec.Template.Spec.Volumes[i]
					break
				}
			}
			Expect(tlsVolume).NotTo(BeNil())
			Expect(tlsVolume.Secret.SecretName).To(Equal(utils.RHOKPCertsSecretName))
			Expect(tlsVolume.Secret.Items).To(ConsistOf(
				corev1.KeyToPath{Key: "tls.crt", Path: "localhost.crt"},
				corev1.KeyToPath{Key: "tls.key", Path: "localhost.key"},
			))
		})

		It("should have an EmptyDir volume for Solr data with 75Gi sizeLimit", func() {
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.RHOKPDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, dep)
			Expect(err).NotTo(HaveOccurred())

			var solrVolume *corev1.Volume
			for i := range dep.Spec.Template.Spec.Volumes {
				if dep.Spec.Template.Spec.Volumes[i].Name == utils.RHOKPSolrDataVolumeName {
					solrVolume = &dep.Spec.Template.Spec.Volumes[i]
					break
				}
			}
			Expect(solrVolume).NotTo(BeNil())
			Expect(solrVolume.EmptyDir).NotTo(BeNil())
			Expect(solrVolume.EmptyDir.SizeLimit.String()).To(Equal(utils.RHOKPSolrDataSizeLimitDefault))
		})

		It("should skip Deployment update when spec and versions are unchanged", func() {
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.RHOKPDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, dep)
			Expect(err).NotTo(HaveOccurred())
			oldRV := dep.ResourceVersion

			err = ReconcileDeployment(testReconcilerInstance, ctx, testCR)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.RHOKPDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.ResourceVersion).To(Equal(oldRV))
		})

		It("should trigger a rolling restart via Restart", func() {
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.RHOKPDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, dep)
			Expect(err).NotTo(HaveOccurred())

			err = Restart(testReconcilerInstance, ctx, dep)
			Expect(err).NotTo(HaveOccurred())

			updated := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.RHOKPDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, updated)
			Expect(err).NotTo(HaveOccurred())
			Expect(updated.Spec.Template.Annotations).To(HaveKey(utils.ForceReloadAnnotationKey))
		})

		It("should skip Restart when the Deployment is missing", func() {
			Expect(k8sClient.Delete(ctx, &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      utils.RHOKPDeploymentName,
					Namespace: utils.OLSNamespaceDefault,
				},
			})).To(Succeed())

			err := Restart(testReconcilerInstance, ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should remove all RHOKP resources via Remove", func() {
			ensureRHOKPTLSSecret()
			Expect(ReconcileDeployment(testReconcilerInstance, ctx, testCR)).To(Succeed())

			err := Remove(testReconcilerInstance, ctx)
			Expect(err).NotTo(HaveOccurred())

			dep := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.RHOKPDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, dep)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "deployment should be deleted")

			svc := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.RHOKPServiceName,
				Namespace: utils.OLSNamespaceDefault,
			}, svc)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "service should be deleted")

			np := &networkingv1.NetworkPolicy{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.RHOKPNetworkPolicyName,
				Namespace: utils.OLSNamespaceDefault,
			}, np)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "network policy should be deleted")

			tlsSecret := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.RHOKPCertsSecretName,
				Namespace: utils.OLSNamespaceDefault,
			}, tlsSecret)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "TLS secret should be deleted")
		})
	})
})
