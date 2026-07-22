package ocpmcp

import (
	"fmt"
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

var _ = Describe("OpenShift MCP Server deployment", func() {
	var testCR *olsv1alpha1.OLSConfig

	BeforeEach(func() {
		testCR = utils.GetDefaultOLSConfigCR()
		testCR.Spec.OLSConfig.IntrospectionEnabled = utils.BoolPtr(true)
		ensureMCPConfigMap(testCR)
		ensureMCPTLSSecret()
	})

	It("should generate HTTPS deployment with TLS args, probes, and version annotations", func() {
		dep, err := GenerateDeployment(testReconcilerInstance, ctx, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(dep.Name).To(Equal(utils.OpenShiftMCPServerDeploymentName))
		Expect(dep.Labels).To(Equal(selectorLabels()))
		Expect(dep.Annotations).To(HaveKey(utils.OpenShiftMCPServerConfigMapResourceVersionAnnotation))
		Expect(dep.Annotations).To(HaveKey(utils.OpenShiftMCPServerTLSSecretResourceVersionAnnotation))

		spec := dep.Spec.Template.Spec
		Expect(spec.ServiceAccountName).To(Equal(utils.OpenShiftMCPServerServiceAccountName))
		Expect(spec.Containers).To(HaveLen(1))

		container := spec.Containers[0]
		Expect(container.Name).To(Equal(utils.OpenShiftMCPServerContainerName))
		Expect(container.Image).To(Equal(utils.OpenShiftMCPServerImageDefault))
		Expect(container.ImagePullPolicy).To(Equal(corev1.PullIfNotPresent))
		Expect(container.Command).To(Equal([]string{
			"/openshift-mcp-server",
			"--config", utils.GetOpenShiftMCPServerConfigPath(),
			"--port", fmt.Sprintf("%d", utils.OpenShiftMCPServerHTTPSPort),
			"--tls-cert=" + path.Join(utils.OpenShiftMCPServerTLSMountPath, "tls.crt"),
			"--tls-key=" + path.Join(utils.OpenShiftMCPServerTLSMountPath, "tls.key"),
		}))
		Expect(container.Ports).To(HaveLen(1))
		Expect(container.Ports[0].Name).To(Equal("https"))
		Expect(container.Ports[0].ContainerPort).To(Equal(int32(utils.OpenShiftMCPServerHTTPSPort)))
		Expect(container.LivenessProbe.HTTPGet.Path).To(Equal("/healthz"))
		Expect(container.LivenessProbe.HTTPGet.Scheme).To(Equal(corev1.URISchemeHTTPS))
		Expect(container.ReadinessProbe.HTTPGet.Path).To(Equal("/healthz"))
		Expect(container.Resources).To(Equal(corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("64Mi")},
			Claims:   []corev1.ResourceClaim{},
		}))

		_, expectedConfigMount := utils.GetOpenShiftMCPServerConfigVolumeAndMount()
		Expect(container.VolumeMounts).To(ContainElement(expectedConfigMount))
		Expect(container.VolumeMounts).To(ContainElement(corev1.VolumeMount{
			Name:      utils.OpenShiftMCPServerTLSVolumeName,
			MountPath: utils.OpenShiftMCPServerTLSMountPath,
			ReadOnly:  true,
		}))
	})

	It("should apply MCP server resource overrides from the CR", func() {
		testCR.Spec.OLSConfig.DeploymentConfig.MCPServerContainer.Resources = &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
		}

		dep, err := GenerateDeployment(testReconcilerInstance, ctx, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(dep.Spec.Template.Spec.Containers[0].Resources.Requests).To(Equal(corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		}))
	})

	It("should fail GenerateDeployment when the TLS secret is missing", func() {
		Expect(k8sClient.Delete(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      utils.OpenShiftMCPServerCertsSecretName,
				Namespace: utils.OLSNamespaceDefault,
			},
		})).To(Succeed())

		_, err := GenerateDeployment(testReconcilerInstance, ctx, testCR)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(utils.ErrGetOpenShiftMCPServerTLSSecret))
	})
})
