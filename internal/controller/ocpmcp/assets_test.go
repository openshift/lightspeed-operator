package ocpmcp

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

var _ = Describe("OpenShift MCP Server assets", func() {
	var testCR *olsv1alpha1.OLSConfig
	labels := selectorLabels()

	BeforeEach(func() {
		testCR = utils.GetDefaultOLSConfigCR()
		testCR.Spec.OLSConfig.IntrospectionEnabled = utils.BoolPtr(true)
	})

	It("should generate the TOML ConfigMap with denied Secret and RBAC resources", func() {
		cm, err := GenerateConfigMap(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(cm.Name).To(Equal(utils.OpenShiftMCPServerConfigCmName))
		Expect(cm.Namespace).To(Equal(utils.OLSNamespaceDefault))
		Expect(cm.Labels).To(Equal(labels))

		toml := cm.Data[utils.OpenShiftMCPServerConfigFilename]
		Expect(toml).To(ContainSubstring(`toolsets = ["core", "config", "helm", "metrics"]`))
		Expect(toml).To(ContainSubstring(`kind = "Secret"`))
		Expect(toml).To(ContainSubstring(`group = ""`))
		Expect(toml).To(ContainSubstring(`group = "rbac.authorization.k8s.io"`))
		Expect(toml).To(ContainSubstring("[[denied_resources]]"))
		Expect(toml).To(ContainSubstring("thanos-querier.openshift-monitoring"))
		Expect(toml).To(ContainSubstring("alertmanager-main.openshift-monitoring"))
		Expect(toml).To(ContainSubstring(`guardrails = "!tsdb"`))
		Expect(strings.Count(toml, "[[denied_resources]]")).To(Equal(2))
	})

	It("should generate the Service with HTTPS port and serving-cert annotation", func() {
		svc, err := GenerateService(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(svc.Name).To(Equal(utils.OpenShiftMCPServerServiceName))
		Expect(svc.Labels).To(Equal(labels))
		Expect(svc.Annotations[utils.ServingCertSecretAnnotationKey]).To(Equal(utils.OpenShiftMCPServerCertsSecretName))
		Expect(svc.Spec.Selector).To(Equal(labels))
		Expect(svc.Spec.Ports).To(HaveLen(1))
		Expect(svc.Spec.Ports[0].Name).To(Equal("https"))
		Expect(svc.Spec.Ports[0].Port).To(Equal(int32(utils.OpenShiftMCPServerHTTPSPort)))
		Expect(svc.Spec.Ports[0].TargetPort).To(Equal(intstr.FromString("https")))
	})

	It("should generate the NetworkPolicy for in-namespace HTTPS ingress", func() {
		np, err := GenerateNetworkPolicy(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(np.Name).To(Equal(utils.OpenShiftMCPServerNetworkPolicyName))
		Expect(np.Labels).To(Equal(labels))
		Expect(np.Spec.PodSelector.MatchLabels).To(Equal(labels))

		tcp := corev1.ProtocolTCP
		httpsPort := intstr.FromInt32(utils.OpenShiftMCPServerHTTPSPort)
		Expect(np.Spec.Ingress).To(ConsistOf(networkingv1.NetworkPolicyIngressRule{
			From: []networkingv1.NetworkPolicyPeer{
				{PodSelector: &metav1.LabelSelector{}},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				{Protocol: &tcp, Port: &httpsPort},
			},
		}))
	})

	It("should generate the ServiceAccount", func() {
		sa, err := GenerateServiceAccount(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(sa.Name).To(Equal(utils.OpenShiftMCPServerServiceAccountName))
		Expect(sa.Namespace).To(Equal(utils.OLSNamespaceDefault))
	})

	It("should return config volume and mount for --config", func() {
		volume, mount := GetConfigVolumeAndMount()
		Expect(volume.Name).To(Equal(utils.OpenShiftMCPServerConfigVolumeName))
		Expect(volume.ConfigMap).NotTo(BeNil())
		Expect(volume.ConfigMap.Name).To(Equal(utils.OpenShiftMCPServerConfigCmName))
		Expect(volume.ConfigMap.DefaultMode).NotTo(BeNil())
		Expect(*volume.ConfigMap.DefaultMode).To(Equal(utils.VolumeDefaultMode))
		Expect(mount.Name).To(Equal(utils.OpenShiftMCPServerConfigVolumeName))
		Expect(mount.MountPath).To(Equal(GetConfigPath()))
		Expect(mount.SubPath).To(Equal(utils.OpenShiftMCPServerConfigFilename))
		Expect(mount.ReadOnly).To(BeTrue())
		Expect(GetConfigPath()).To(Equal("/etc/mcp-server/config.toml"))
	})
})
