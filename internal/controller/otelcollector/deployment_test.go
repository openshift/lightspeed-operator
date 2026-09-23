package otelcollector

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("OTEL Collector deployment", func() {
	var testCR *olsv1alpha1.OLSConfig

	BeforeEach(func() {
		testCR = utils.GetDefaultOLSConfigCR()
		ensurePostgresSecret()
		ensureCollectorConfigMap(testCR)
	})

	It("should generate the collector deployment with postgres env, admin port, and init container", func() {
		dep, err := GenerateOtelCollectorDeployment(testReconcilerInstance, ctx, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(dep.Name).To(Equal(utils.OtelCollectorDeploymentName))
		Expect(dep.Labels).To(Equal(utils.GenerateOtelCollectorSelectorLabels()))
		Expect(dep.Annotations).To(HaveKey(utils.OtelCollectorConfigMapResourceVersionAnnotation))
		Expect(dep.OwnerReferences).To(HaveLen(1))
		Expect(dep.OwnerReferences[0].Name).To(Equal(testCR.Name))
		Expect(dep.OwnerReferences[0].UID).To(Equal(testCR.UID))

		spec := dep.Spec.Template.Spec
		Expect(spec.ServiceAccountName).To(Equal(utils.OtelCollectorServiceAccountName))
		Expect(spec.InitContainers).To(HaveLen(1))
		Expect(spec.InitContainers[0].Name).To(Equal(utils.PostgresWaitInitContainerName))

		container := spec.Containers[0]
		Expect(container.Resources.Requests.Cpu().String()).To(Equal("100m"))
		Expect(container.Resources.Requests.Memory().String()).To(Equal("128Mi"))

		Expect(container.Name).To(Equal(utils.OtelCollectorContainerName))
		Expect(container.Image).To(Equal(testOtelCollectorImage))
		Expect(container.Args).To(ConsistOf("--config=/etc/otelcol/config.yaml"))

		postgresEnv, ok := containerEnvNamed(container, utils.OtelCollectorPostgresConnectionStringEnvVar)
		Expect(ok).To(BeTrue())
		Expect(postgresEnv.Value).To(BeEmpty())
		Expect(postgresEnv.ValueFrom).NotTo(BeNil())
		Expect(postgresEnv.ValueFrom.SecretKeyRef).NotTo(BeNil())
		Expect(postgresEnv.ValueFrom.SecretKeyRef.Name).To(Equal(utils.OtelCollectorPostgresDSNSecretName))
		Expect(postgresEnv.ValueFrom.SecretKeyRef.Key).To(Equal(utils.OtelCollectorPostgresConnectionStringSecretKey))

		_, ok = containerEnvNamed(container, utils.OtelCollectorTracesBackendEndpointEnvVar)
		Expect(ok).To(BeFalse())

		portNames := make([]string, 0, len(container.Ports))
		for _, p := range container.Ports {
			portNames = append(portNames, p.Name)
		}
		Expect(portNames).To(ConsistOf("otlp-grpc", "otlp-http", "admin", "metrics"))
		Expect(container.ReadinessProbe.HTTPGet.Port.IntValue()).To(Equal(int(utils.OtelCollectorHealthCheckPort)))

		volumeNames := make([]string, 0, len(spec.Volumes))
		for _, v := range spec.Volumes {
			volumeNames = append(volumeNames, v.Name)
		}
		Expect(volumeNames).To(ContainElement(utils.OtelCollectorServiceCAVolumeName))

		mountNames := make([]string, 0, len(container.VolumeMounts))
		for _, m := range container.VolumeMounts {
			mountNames = append(mountNames, m.Name)
		}
		Expect(mountNames).To(ContainElement(utils.OtelCollectorServiceCAVolumeName))
		agenticVolume, found := findVolume(spec.Volumes, utils.OtelCollectorAgenticDataVolumeName)
		Expect(found).To(BeTrue())
		Expect(agenticVolume.EmptyDir).NotTo(BeNil())
		Expect(agenticVolume.EmptyDir.SizeLimit.String()).To(Equal(utils.OtelCollectorAgenticDataSizeLimitDefault))

		agenticMount, found := findVolumeMount(container.VolumeMounts, utils.OtelCollectorAgenticDataVolumeName)
		Expect(found).To(BeTrue())
		Expect(agenticMount.MountPath).To(Equal(utils.OtelCollectorAgenticDataMountPath))
		Expect(agenticMount.ReadOnly).To(BeFalse())
		Expect(spec.Containers).To(HaveLen(1))

		Expect(container.SecurityContext).NotTo(BeNil())
		Expect(*container.SecurityContext.RunAsNonRoot).To(BeTrue())
		Expect(*container.SecurityContext.AllowPrivilegeEscalation).To(BeFalse())
		Expect(*container.SecurityContext.ReadOnlyRootFilesystem).To(BeTrue())
		Expect(container.SecurityContext.Capabilities.Drop).To(ContainElement(corev1.Capability("ALL")))
		Expect(container.SecurityContext.SeccompProfile.Type).To(Equal(corev1.SeccompProfileTypeRuntimeDefault))
	})
	It("should omit the Agentic spool when transcripts are disabled", func() {
		testCR.Spec.OLSConfig.UserDataCollection.TranscriptsDisabled = true
		ensureCollectorConfigMap(testCR)

		dep, err := GenerateOtelCollectorDeployment(testReconcilerInstance, ctx, testCR)
		Expect(err).NotTo(HaveOccurred())

		spec := dep.Spec.Template.Spec
		_, found := findVolume(spec.Volumes, utils.OtelCollectorAgenticDataVolumeName)
		Expect(found).To(BeFalse())
		_, found = findVolumeMount(spec.Containers[0].VolumeMounts, utils.OtelCollectorAgenticDataVolumeName)
		Expect(found).To(BeFalse())
		Expect(spec.Containers).To(HaveLen(1))
	})

	It("should keep postgres wiring when audit logging is disabled", func() {
		testCR.Spec.Audit.Logging = boolPtr(false)
		ensureCollectorConfigMap(testCR)

		dep, err := GenerateOtelCollectorDeployment(testReconcilerInstance, ctx, testCR)
		Expect(err).NotTo(HaveOccurred())

		spec := dep.Spec.Template.Spec
		Expect(spec.InitContainers).To(HaveLen(1))
		Expect(spec.InitContainers[0].Name).To(Equal(utils.PostgresWaitInitContainerName))

		container := spec.Containers[0]
		_, ok := containerEnvNamed(container, utils.OtelCollectorPostgresConnectionStringEnvVar)
		Expect(ok).To(BeTrue())

		portNames := make([]string, 0, len(container.Ports))
		for _, p := range container.Ports {
			portNames = append(portNames, p.Name)
		}
		Expect(portNames).To(ConsistOf("otlp-grpc", "otlp-http", "admin", "metrics"))
	})

	It("should set TRACES_BACKEND_ENDPOINT when tracingEndpoint is configured", func() {
		testCR.Spec.Audit.TracingEndpoint = "jaeger-collector:4317"
		ensureCollectorConfigMap(testCR)

		dep, err := GenerateOtelCollectorDeployment(testReconcilerInstance, ctx, testCR)
		Expect(err).NotTo(HaveOccurred())

		tracesEnv, ok := containerEnvNamed(dep.Spec.Template.Spec.Containers[0], utils.OtelCollectorTracesBackendEndpointEnvVar)
		Expect(ok).To(BeTrue())
		Expect(tracesEnv.Value).To(Equal("jaeger-collector:4317"))
	})

})

func findVolume(volumes []corev1.Volume, name string) (corev1.Volume, bool) {
	for _, volume := range volumes {
		if volume.Name == name {
			return volume, true
		}
	}
	return corev1.Volume{}, false
}

func findVolumeMount(volumeMounts []corev1.VolumeMount, name string) (corev1.VolumeMount, bool) {
	for _, volumeMount := range volumeMounts {
		if volumeMount.Name == name {
			return volumeMount, true
		}
	}
	return corev1.VolumeMount{}, false
}
