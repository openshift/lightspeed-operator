package agenticintegration

import (
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

var _ = Describe("Agentic integration assets", func() {
	var testCR *olsv1alpha1.OLSConfig
	labels := utils.GenerateAgenticIntegrationSelectorLabels()

	BeforeEach(func() {
		testCR = utils.GetDefaultOLSConfigCR()
	})

	It("should default sandbox mode to bare-pod", func() {
		Expect(SandboxModeFromCR(testCR)).To(Equal(olsv1alpha1.SandboxModeBarePod))
		testCR.Spec.AgenticOLS = &olsv1alpha1.AgenticOLSSpec{SandboxMode: olsv1alpha1.SandboxModeSandboxClaim}
		Expect(SandboxModeFromCR(testCR)).To(Equal(olsv1alpha1.SandboxModeSandboxClaim))
	})

	It("should generate a PodSpec with image, default resources, and writable emptyDirs", func() {
		spec := GenerateSandboxPodSpec(testReconcilerInstance, testCR)
		Expect(spec.Containers).To(HaveLen(1))
		c := spec.Containers[0]
		Expect(c.Name).To(Equal(utils.AgenticSandboxContainerName))
		Expect(c.Image).To(Equal(utils.AgenticSandboxImageDefault))
		Expect(c.Resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("500m")))
		Expect(c.Resources.Requests[corev1.ResourceMemory]).To(Equal(resource.MustParse("128Mi")))
		Expect(c.Resources.Limits).To(BeEmpty())
		Expect(c.VolumeMounts).To(ConsistOf(
			corev1.VolumeMount{Name: sandboxHomeVolumeName, MountPath: sandboxHomeMountPath},
			corev1.VolumeMount{Name: sandboxSkillsWorkdirVolumeName, MountPath: sandboxSkillsWorkdirMountPath},
		))
		Expect(spec.Volumes).To(HaveLen(2))
		Expect(spec.Volumes[0].EmptyDir).NotTo(BeNil())
		Expect(spec.Volumes[1].EmptyDir).NotTo(BeNil())
	})

	It("should apply AgenticSandboxConfig resources, tolerations, and nodeSelector", func() {
		testCR.Spec.AgenticOLS = &olsv1alpha1.AgenticOLSSpec{
			AgenticSandboxConfig: olsv1alpha1.Config{
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
				NodeSelector: map[string]string{"node-role.kubernetes.io/worker": ""},
				Tolerations: []corev1.Toleration{{
					Key:      "key",
					Operator: corev1.TolerationOpExists,
					Effect:   corev1.TaintEffectNoSchedule,
				}},
			},
		}
		spec := GenerateSandboxPodSpec(testReconcilerInstance, testCR)
		Expect(spec.Containers[0].Resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("1")))
		Expect(spec.NodeSelector).To(HaveKey("node-role.kubernetes.io/worker"))
		Expect(spec.Tolerations).To(HaveLen(1))
	})

	It("should generate the handoff ConfigMap without MCP keys when introspection is off", func() {
		testCR.Spec.OLSConfig.IntrospectionEnabled = utils.BoolPtr(false)
		cm, err := GenerateAgenticConfigurationConfigMap(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(cm.Name).To(Equal(utils.AgenticConfigurationConfigMapName))
		Expect(cm.Labels).To(Equal(labels))
		Expect(cm.Data[utils.AgenticConfigurationSandboxModeKey]).To(Equal(string(olsv1alpha1.SandboxModeBarePod)))
		Expect(cm.Data[utils.AgenticConfigurationOtelCASecretKey]).To(Equal(utils.AgenticOtelCASecretName))
		Expect(cm.Data).NotTo(HaveKey(utils.AgenticConfigurationMCPEndpointKey))
		Expect(cm.Data).NotTo(HaveKey(utils.AgenticConfigurationMCPCASecretKey))

		var podSpec corev1.PodSpec
		Expect(json.Unmarshal([]byte(cm.Data[utils.AgenticConfigurationSandboxPodSpecKey]), &podSpec)).To(Succeed())
		Expect(podSpec.Containers[0].Image).To(Equal(utils.AgenticSandboxImageDefault))

		host := fmt.Sprintf("%s.%s.svc", utils.OtelCollectorServiceName, utils.OLSNamespaceDefault)
		Expect(cm.Data[utils.AgenticConfigurationOtelCollectorEndpointKey]).To(Equal(
			fmt.Sprintf("%s:%d", host, utils.OtelCollectorGRPCPort),
		))
		Expect(cm.Data[utils.AgenticConfigurationOtelAdminEndpointKey]).To(Equal(
			fmt.Sprintf("https://%s:%d", host, utils.OtelCollectorAdminPort),
		))
	})

	It("should include MCP keys when introspection is enabled", func() {
		testCR.Spec.OLSConfig.IntrospectionEnabled = utils.BoolPtr(true)
		cm, err := GenerateAgenticConfigurationConfigMap(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(cm.Data[utils.AgenticConfigurationMCPEndpointKey]).To(Equal(
			utils.OpenShiftMCPServerServiceURL(utils.OLSNamespaceDefault),
		))
		Expect(cm.Data[utils.AgenticConfigurationMCPCASecretKey]).To(Equal(utils.AgenticMCPCASecretName))
	})

	It("should touch the ConfigMap annotation to bump resourceVersion", func() {
		testCR.Spec.OLSConfig.IntrospectionEnabled = utils.BoolPtr(false)
		ensureHandoffCreatePrerequisites(false)
		Expect(ReconcileAgenticIntegrationResources(testReconcilerInstance, ctx, testCR)).To(Succeed())

		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      utils.AgenticConfigurationConfigMapName,
			Namespace: utils.OLSNamespaceDefault,
		}, cm)).To(Succeed())
		oldRV := cm.ResourceVersion

		Expect(TouchAgenticConfiguration(testReconcilerInstance, ctx)).To(Succeed())

		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      utils.AgenticConfigurationConfigMapName,
			Namespace: utils.OLSNamespaceDefault,
		}, cm)).To(Succeed())
		Expect(cm.ResourceVersion).NotTo(Equal(oldRV))
		Expect(cm.Annotations).To(HaveKey(utils.AgenticConfigurationCertReloadAnnotation))
	})
})
