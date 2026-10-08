package rhokp

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
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

		It("should create the RHOKP NetworkPolicy with narrow Prometheus ingress", func() {
			np := &networkingv1.NetworkPolicy{}
			key := types.NamespacedName{Name: utils.RHOKPNetworkPolicyName, Namespace: utils.OLSNamespaceDefault}
			Expect(k8sClient.Get(ctx, key, np)).To(Succeed())
			expectOwnedByOLSConfig(np)
			Expect(np.Spec.PolicyTypes).To(Equal([]networkingv1.PolicyType{networkingv1.PolicyTypeIngress}))
			Expect(np.Spec.Egress).To(BeEmpty())
			Expect(np.Spec.Ingress).To(HaveLen(1))
			Expect(np.Spec.Ingress[0].From).To(ConsistOf(
				networkingv1.NetworkPolicyPeer{PodSelector: &metav1.LabelSelector{}},
				networkingv1.NetworkPolicyPeer{
					PodSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
						{Key: "app.kubernetes.io/name", Operator: metav1.LabelSelectorOpIn, Values: []string{"prometheus"}},
						{Key: "prometheus", Operator: metav1.LabelSelectorOpIn, Values: []string{"k8s"}},
					}},
					NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
						"kubernetes.io/metadata.name": "openshift-monitoring",
					}},
				},
			))
			tcp := corev1.ProtocolTCP
			port := intstr.FromInt(8443)
			Expect(np.Spec.Ingress[0].Ports).To(ConsistOf(networkingv1.NetworkPolicyPort{
				Protocol: &tcp,
				Port:     &port,
			}))
		})

		It("should create a separate egress-only policy selecting RHOKP pods", func() {
			np := &networkingv1.NetworkPolicy{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: utils.RHOKPNetworkPolicyName + "-egress", Namespace: utils.OLSNamespaceDefault,
			}, np)).To(Succeed())
			Expect(np.Spec.PodSelector.MatchLabels).To(Equal(selectorLabels()))
			Expect(np.Spec.PolicyTypes).To(Equal([]networkingv1.PolicyType{networkingv1.PolicyTypeEgress}))
			Expect(np.Spec.Egress).To(BeEmpty())
			Expect(np.Spec.Ingress).To(BeEmpty())
		})

		It("should restore RHOKP Prometheus ingress on reconciliation", func() {
			key := types.NamespacedName{Name: utils.RHOKPNetworkPolicyName, Namespace: utils.OLSNamespaceDefault}
			np := &networkingv1.NetworkPolicy{}
			Expect(k8sClient.Get(ctx, key, np)).To(Succeed())
			np.Spec.Ingress[0].From = []networkingv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{}}}
			Expect(k8sClient.Update(ctx, np)).To(Succeed())

			Expect(ReconcileResources(testReconcilerInstance, ctx, testCR)).To(Succeed())
			Expect(k8sClient.Get(ctx, key, np)).To(Succeed())
			Expect(np.Spec.Ingress[0].From).To(HaveLen(2))
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

		It("should create the RHOKP ServiceMonitor", func() {
			sm := &monv1.ServiceMonitor{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.RHOKPServiceMonitorName,
				Namespace: utils.OLSNamespaceDefault,
			}, sm)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(sm)
			Expect(sm.Spec.Endpoints).To(HaveLen(1))
			ep := sm.Spec.Endpoints[0]
			Expect(ep.Port).To(Equal("https"))
			Expect(ep.Path).To(Equal(utils.RHOKPMetricsPath))
			Expect(string(*ep.Scheme)).To(Equal("https"))
			Expect(ep.TLSConfig).NotTo(BeNil())
			Expect(*ep.TLSConfig.InsecureSkipVerify).To(BeFalse())
			Expect(ep.TLSConfig.CAFile).To(Equal("/etc/prometheus/configmaps/serving-certs-ca-bundle/service-ca.crt"))
			expectedServerName := utils.RHOKPServiceName + "." + utils.OLSNamespaceDefault + ".svc"
			Expect(*ep.TLSConfig.ServerName).To(Equal(expectedServerName))
		})

		It("should probe Solr for startup and liveness, and both Solr and Apache for readiness", func() {
			dep := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: utils.RHOKPDeploymentName, Namespace: utils.OLSNamespaceDefault,
			}, dep)).To(Succeed())

			container := dep.Spec.Template.Spec.Containers[0]
			for _, probe := range []*corev1.Probe{container.StartupProbe, container.LivenessProbe} {
				Expect(probe).NotTo(BeNil())
				Expect(probe.HTTPGet).To(BeNil())
				Expect(probe.Exec).NotTo(BeNil())
				Expect(probe.Exec.Command).To(Equal([]string{
					"/usr/bin/curl", "--fail", "--silent", "--show-error",
					"--max-time", "3", "--output", "/dev/null",
					"http://127.0.0.1:8983/solr/portal-rag/admin/ping",
				}))
				Expect(probe.TimeoutSeconds).To(Equal(int32(5)))
			}

			readiness := container.ReadinessProbe
			Expect(readiness).NotTo(BeNil())
			Expect(readiness.HTTPGet).To(BeNil())
			Expect(readiness.Exec).NotTo(BeNil())
			Expect(readiness.Exec.Command).To(Equal([]string{
				"/bin/sh", "-ec",
				"/usr/bin/curl --fail --silent --show-error --max-time 3 --output /dev/null " +
					"http://127.0.0.1:8983/solr/portal-rag/admin/ping && " +
					"apache_status=$(/usr/bin/curl --fail --silent --show-error --insecure --max-time 3 " +
					"--output /dev/null --write-out '%{http_code}' " +
					"'https://127.0.0.1:8443/solr/portal-rag/select?q=%2A%3A%2A&rows=0') && " +
					"test \"$apache_status\" = 200",
			}))
			Expect(readiness.TimeoutSeconds).To(Equal(int32(7)))
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
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name: utils.RHOKPNetworkPolicyName + "-egress", Namespace: utils.OLSNamespaceDefault,
			}, np)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "egress policy should be deleted")

			tlsSecret := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.RHOKPCertsSecretName,
				Namespace: utils.OLSNamespaceDefault,
			}, tlsSecret)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "TLS secret should be deleted")

			sm := &monv1.ServiceMonitor{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.RHOKPServiceMonitorName,
				Namespace: utils.OLSNamespaceDefault,
			}, sm)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "ServiceMonitor should be deleted")
		})
	})
})
