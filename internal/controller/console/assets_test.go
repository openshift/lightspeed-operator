package console

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	consolev1 "github.com/openshift/api/console/v1"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

var _ = Describe("Console UI assets", func() {
	var cr *olsv1alpha1.OLSConfig
	labels := map[string]string{
		"app.kubernetes.io/component":  "console-plugin",
		"app.kubernetes.io/managed-by": "lightspeed-operator",
		"app.kubernetes.io/name":       "lightspeed-console-plugin",
		"app.kubernetes.io/part-of":    "openshift-lightspeed",
	}

	Context("complete custom resource", func() {
		BeforeEach(func() {
			cr = utils.GetDefaultOLSConfigCR()
		})

		It("should generate the nginx config map", func() {
			cm, err := GenerateConsoleUIConfigMap(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Name).To(Equal(utils.ConsoleUIConfigMapName))
			Expect(cm.Namespace).To(Equal(utils.OLSNamespaceDefault))
			Expect(cm.Labels).To(Equal(labels))

			// todo: check the nginx config
		})

		It("should generate the console UI service", func() {
			svc, err := GenerateConsoleUIService(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(svc.Name).To(Equal(utils.ConsoleUIServiceName))
			Expect(svc.Namespace).To(Equal(utils.OLSNamespaceDefault))
			Expect(svc.Labels).To(Equal(labels))
			Expect(svc.ObjectMeta.Annotations["service.beta.openshift.io/serving-cert-secret-name"]).To(Equal(utils.ConsoleUIServiceCertSecretName))
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(utils.ConsoleUIHTTPSPort)))
			Expect(svc.Spec.Ports[0].TargetPort.StrVal).To(Equal("https"))
			Expect(svc.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP))
		})

		It("should generate the console UI deployment", func() {
			dep, err := GenerateConsoleUIDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Name).To(Equal(utils.ConsoleUIDeploymentName))
			Expect(dep.Namespace).To(Equal(utils.OLSNamespaceDefault))
			Expect(dep.Labels).To(Equal(labels))
			Expect(dep.Spec.Template.Labels).To(Equal(labels))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal(utils.ConsoleUIContainerName))
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal(utils.ConsoleUIImageDefault))
			Expect(dep.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort).To(Equal(int32(utils.ConsoleUIHTTPSPort)))
			Expect(dep.Spec.Template.Spec.Containers[0].Ports[0].Name).To(Equal("https"))
			Expect(dep.Spec.Template.Spec.Containers[0].Ports[0].Protocol).To(Equal(corev1.ProtocolTCP))
			Expect(dep.Spec.Template.Spec.Containers[0].Resources).To(Equal(corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10m"), corev1.ResourceMemory: resource.MustParse("50Mi")},
				Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("100Mi")},
				Claims:   []corev1.ResourceClaim{},
			}))
			Expect(dep.Spec.Template.Spec.Tolerations).To(BeNil())
			Expect(dep.Spec.Template.Spec.NodeSelector).To(BeNil())
			// Console always runs 1 replica (not configurable)
			Expect(*dep.Spec.Replicas).To(Equal(int32(1)))
		})

		It("should generate the console UI plugin", func() {
			plugin, err := GenerateConsoleUIPlugin(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(plugin.Name).To(Equal(utils.ConsoleUIPluginName))
			Expect(plugin.Labels).To(Equal(labels))
			Expect(plugin.Spec.Backend.Service.Name).To(Equal(utils.ConsoleUIServiceName))
			Expect(plugin.Spec.Backend.Service.Namespace).To(Equal(utils.OLSNamespaceDefault))
			Expect(plugin.Spec.Backend.Service.Port).To(Equal(int32(utils.ConsoleUIHTTPSPort)))
			Expect(plugin.Spec.Backend.Service.BasePath).To(Equal("/"))
			Expect(plugin.Spec.Backend.Type).To(Equal(consolev1.Service))

			Expect(plugin.Spec.Proxy).To(HaveLen(1))
			Expect(plugin.Spec.Proxy[0].Endpoint.Service.Name).To(Equal(utils.OLSAppServerServiceName))
			Expect(plugin.Spec.Proxy[0].Endpoint.Service.Port).To(Equal(int32(utils.OLSAppServerServicePort)))
			Expect(plugin.Spec.Proxy[0].Endpoint.Service.Namespace).To(Equal(utils.OLSNamespaceDefault))
			Expect(plugin.Spec.Proxy[0].Endpoint.Type).To(Equal(consolev1.ProxyTypeService))
		})

		It("should generate the console UI plugin NetworkPolicy", func() {
			np, err := GenerateConsoleUINetworkPolicy(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(np.Name).To(Equal(utils.ConsoleUINetworkPolicyName))
			Expect(np.Namespace).To(Equal(utils.OLSNamespaceDefault))
			Expect(np.Labels).To(Equal(labels))
			Expect(np.Spec.PolicyTypes).To(Equal([]networkingv1.PolicyType{networkingv1.PolicyTypeIngress}))
			Expect(np.Spec.Ingress).To(HaveLen(1))
			Expect(np.Spec.Ingress).To(ConsistOf([]networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": "openshift-console",
								},
							},
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "console",
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
							Port:     &[]intstr.IntOrString{intstr.FromInt(utils.ConsoleUIHTTPSPort)}[0],
						},
					},
				},
			}))
			Expect(np.Spec.PodSelector.MatchLabels).To(Equal(labels))

		})
	})
})
