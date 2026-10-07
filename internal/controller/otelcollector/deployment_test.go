package otelcollector

import (
	"context"
	"errors"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

var _ = Describe("OTEL Collector deployment", func() {
	var testCR *olsv1alpha1.OLSConfig

	BeforeEach(func() {
		testCR = utils.GetDefaultOLSConfigCR()
		ensurePostgresSecret()
		setTelemetryPullSecretForTest(telemetryPullSecretWithAuthForTest, corev1.SecretTypeDockerConfigJson)
		ensureOtelDataverseExporterConfigMap(testCR)
		ensureCollectorConfigMap(testCR)
	})

	It("should generate the collector deployment with Postgres wiring and writable trace collection storage", func() {
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
		generatedConfigMap, err := GenerateOtelCollectorConfigMap(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())
		var generatedConfig struct {
			Exporters map[string]struct {
				Path string `json:"path"`
			} `json:"exporters"`
		}
		Expect(yaml.Unmarshal([]byte(generatedConfigMap.Data[utils.OtelCollectorConfigMapDataKey]), &generatedConfig)).To(Succeed())
		fileExporter, found := generatedConfig.Exporters["file/data_collection"]
		Expect(found).To(BeTrue())
		Expect(fileExporter.Path).To(Equal("/var/lib/lightspeed-data/otel/traces.jsonl"))

		fileStorageVolume, found := findVolume(spec.Volumes, otelCollectorFileStorageVolumeName)
		Expect(found).To(BeTrue())
		Expect(fileStorageVolume.EmptyDir).NotTo(BeNil())

		dataCollectionVolume, found := findVolume(spec.Volumes, "data-collection")
		Expect(found).To(BeTrue())
		Expect(dataCollectionVolume.EmptyDir).NotTo(BeNil())
		Expect(dataCollectionVolume.EmptyDir.SizeLimit.String()).To(Equal("500Mi"))

		dataCollectionMount, found := findVolumeMount(container.VolumeMounts, "data-collection")
		Expect(found).To(BeTrue())
		Expect(dataCollectionMount).To(Equal(corev1.VolumeMount{
			Name:      utils.OtelCollectorDataCollectionVolumeName,
			MountPath: filepath.Dir(fileExporter.Path),
		}))
		Expect(dataCollectionMount.ReadOnly).To(BeFalse())
		fileRelativePath, err := filepath.Rel(dataCollectionMount.MountPath, fileExporter.Path)
		Expect(err).NotTo(HaveOccurred())
		Expect(fileRelativePath).To(Equal("traces.jsonl"))
		Expect(spec.Containers).To(HaveLen(2))
		exporter := spec.Containers[1]
		Expect(exporter.Name).To(Equal(utils.DataverseExporterContainerName))
		Expect(exporter.Image).To(Equal("quay.io/test/dataverse-exporter:test"))
		Expect(exporter.Args).To(Equal([]string{"--mode", "openshift", "--config", "/etc/config/config.yaml"}))
		Expect(exporter.Env).To(Equal(utils.GetProxyEnvVars()))
		Expect(exporter.Resources.Requests.Cpu().String()).To(Equal("50m"))
		Expect(exporter.Resources.Requests.Memory().String()).To(Equal("64Mi"))

		Expect(container.SecurityContext).NotTo(BeNil())
		Expect(*container.SecurityContext.RunAsNonRoot).To(BeTrue())
		Expect(*container.SecurityContext.AllowPrivilegeEscalation).To(BeFalse())
		Expect(*container.SecurityContext.ReadOnlyRootFilesystem).To(BeTrue())
		Expect(container.SecurityContext.Capabilities.Drop).To(ContainElement(corev1.Capability("ALL")))
		Expect(container.SecurityContext.SeccompProfile.Type).To(Equal(corev1.SeccompProfileTypeRuntimeDefault))
	})
	It("should omit exporter-only resources when transcripts are disabled", func() {
		testCR.Spec.OLSConfig.UserDataCollection.TranscriptsDisabled = true
		ensureCollectorConfigMap(testCR)

		dep, err := GenerateOtelCollectorDeployment(testReconcilerInstance, ctx, testCR)
		Expect(err).NotTo(HaveOccurred())

		spec := dep.Spec.Template.Spec
		Expect(spec.Containers).To(HaveLen(1))
		for _, name := range []string{
			utils.OtelCollectorDataCollectionVolumeName,
			utils.OtelDataverseExporterConfigVolumeName,
			utils.OtelDataverseExporterStateVolumeName,
			utils.OtelDataverseExporterAuthVolumeName,
		} {
			_, found := findVolume(spec.Volumes, name)
			Expect(found).To(BeFalse())
		}
		_, found := findVolumeMount(spec.Containers[0].VolumeMounts, utils.OtelCollectorDataCollectionVolumeName)
		Expect(found).To(BeFalse())
	})

	It("should keep native trace storage when telemetry auth is absent", func() {
		setTelemetryPullSecretForTest(telemetryPullSecretWithoutAuthTest, corev1.SecretTypeDockerConfigJson)

		dep, err := GenerateOtelCollectorDeployment(testReconcilerInstance, ctx, testCR)
		Expect(err).NotTo(HaveOccurred())

		spec := dep.Spec.Template.Spec
		Expect(spec.Containers).To(HaveLen(1))
		sourceVolume, found := findVolume(spec.Volumes, utils.OtelCollectorDataCollectionVolumeName)
		Expect(found).To(BeTrue())
		Expect(sourceVolume.EmptyDir.SizeLimit.String()).To(Equal("500Mi"))
		sourceMount, found := findVolumeMount(spec.Containers[0].VolumeMounts, utils.OtelCollectorDataCollectionVolumeName)
		Expect(found).To(BeTrue())
		Expect(sourceMount.ReadOnly).To(BeFalse())
		for _, name := range []string{
			utils.OtelDataverseExporterConfigVolumeName,
			utils.OtelDataverseExporterStateVolumeName,
			utils.OtelDataverseExporterAuthVolumeName,
		} {
			_, found := findVolume(spec.Volumes, name)
			Expect(found).To(BeFalse())
		}

		collectorConfig, err := GenerateOtelCollectorConfigMap(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())
		var generatedConfig struct {
			Exporters map[string]struct {
				Path string `json:"path"`
			} `json:"exporters"`
		}
		Expect(yaml.Unmarshal([]byte(collectorConfig.Data[utils.OtelCollectorConfigMapDataKey]), &generatedConfig)).To(Succeed())
		Expect(generatedConfig.Exporters["file/data_collection"].Path).To(Equal("/var/lib/lightspeed-data/otel/traces.jsonl"))
	})

	It("should omit the exporter for feedback-only collection", func() {
		testCR.Spec.OLSConfig.UserDataCollection.FeedbackDisabled = false
		testCR.Spec.OLSConfig.UserDataCollection.TranscriptsDisabled = true
		ensureCollectorConfigMap(testCR)

		dep, err := GenerateOtelCollectorDeployment(testReconcilerInstance, ctx, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(1))
		_, found := findVolume(dep.Spec.Template.Spec.Volumes, utils.OtelCollectorDataCollectionVolumeName)
		Expect(found).To(BeFalse())
	})

	It("should omit the exporter when the telemetry pull secret is not found", func() {
		utils.DeleteTelemetryPullSecret(ctx, k8sClient)

		dep, err := GenerateOtelCollectorDeployment(testReconcilerInstance, ctx, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(1))
		_, found := findVolume(dep.Spec.Template.Spec.Volumes, utils.OtelCollectorDataCollectionVolumeName)
		Expect(found).To(BeTrue())
	})

	It("should return errors for malformed telemetry auth configuration", func() {
		setTelemetryPullSecretForTest("not-json", corev1.SecretTypeOpaque)

		_, err := GenerateOtelCollectorDeployment(testReconcilerInstance, ctx, testCR)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to decode telemetry pull secret"))
	})

	It("should return unexpected telemetry pull-secret API errors", func() {
		base := testReconcilerInstance.(*utils.TestReconciler)
		failingReconciler := *base
		failingReconciler.Client = telemetryPullSecretErrorClient{
			Client: base.Client,
			err:    errors.New("API unavailable"),
		}

		_, err := GenerateOtelCollectorDeployment(&failingReconciler, ctx, testCR)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to read telemetry pull secret"))
		Expect(err.Error()).To(ContainSubstring("API unavailable"))
	})

	It("should mount source read-only and keep state, config, and auth isolated to the exporter", func() {
		dep, err := GenerateOtelCollectorDeployment(testReconcilerInstance, ctx, testCR)
		Expect(err).NotTo(HaveOccurred())
		spec := dep.Spec.Template.Spec
		Expect(spec.AutomountServiceAccountToken).NotTo(BeNil())
		Expect(*spec.AutomountServiceAccountToken).To(BeFalse())
		Expect(dep.Annotations).To(HaveKey(utils.OtelDataverseExporterConfigMapResourceVersionAnnotation))

		exporter := spec.Containers[1]
		Expect(exporter.SecurityContext).NotTo(BeNil())
		Expect(exporter.SecurityContext.ReadOnlyRootFilesystem).NotTo(BeNil())
		Expect(*exporter.SecurityContext.ReadOnlyRootFilesystem).To(BeTrue())
		Expect(exporter.SecurityContext.AllowPrivilegeEscalation).NotTo(BeNil())
		Expect(*exporter.SecurityContext.AllowPrivilegeEscalation).To(BeFalse())
		Expect(exporter.SecurityContext.Capabilities.Drop).To(ContainElement(corev1.Capability("ALL")))

		sourceMount, found := findVolumeMount(exporter.VolumeMounts, utils.OtelCollectorDataCollectionVolumeName)
		Expect(found).To(BeTrue())
		Expect(sourceMount.MountPath).To(Equal("/input"))
		Expect(sourceMount.ReadOnly).To(BeTrue())
		stateMount, found := findVolumeMount(exporter.VolumeMounts, utils.OtelDataverseExporterStateVolumeName)
		Expect(found).To(BeTrue())
		Expect(stateMount.MountPath).To(Equal("/state"))
		Expect(stateMount.ReadOnly).To(BeFalse())
		configMount, found := findVolumeMount(exporter.VolumeMounts, utils.OtelDataverseExporterConfigVolumeName)
		Expect(found).To(BeTrue())
		Expect(configMount.ReadOnly).To(BeTrue())
		Expect(configMount.MountPath).To(Equal("/etc/config"))
		configVolume, found := findVolume(spec.Volumes, utils.OtelDataverseExporterConfigVolumeName)
		Expect(found).To(BeTrue())
		Expect(configVolume.ConfigMap.Name).To(Equal(utils.OtelDataverseExporterConfigMapName))
		authMount, found := findVolumeMount(exporter.VolumeMounts, utils.OtelDataverseExporterAuthVolumeName)
		Expect(found).To(BeTrue())
		Expect(authMount.MountPath).To(Equal("/var/run/secrets/kubernetes.io/serviceaccount"))
		Expect(authMount.ReadOnly).To(BeTrue())

		stateVolume, found := findVolume(spec.Volumes, utils.OtelDataverseExporterStateVolumeName)
		Expect(found).To(BeTrue())
		Expect(stateVolume.EmptyDir).NotTo(BeNil())
		authVolume, found := findVolume(spec.Volumes, utils.OtelDataverseExporterAuthVolumeName)
		Expect(found).To(BeTrue())
		Expect(authVolume.Projected).NotTo(BeNil())
		Expect(authVolume.Projected.Sources[0].ServiceAccountToken.Path).To(Equal("token"))
		Expect(authVolume.Projected.Sources[1].ConfigMap.Name).To(Equal("kube-root-ca.crt"))
		Expect(authVolume.Projected.Sources[1].ConfigMap.Items).To(ContainElement(corev1.KeyToPath{Key: "ca.crt", Path: "ca.crt"}))
		Expect(authVolume.Projected.Sources[2].DownwardAPI.Items[0].Path).To(Equal("namespace"))
		Expect(authVolume.Projected.Sources[2].DownwardAPI.Items[0].FieldRef.FieldPath).To(Equal("metadata.namespace"))

		collectorMountNames := make([]string, 0, len(spec.Containers[0].VolumeMounts))
		for _, mount := range spec.Containers[0].VolumeMounts {
			collectorMountNames = append(collectorMountNames, mount.Name)
		}
		for _, name := range []string{
			utils.OtelDataverseExporterConfigVolumeName,
			utils.OtelDataverseExporterStateVolumeName,
			utils.OtelDataverseExporterAuthVolumeName,
		} {
			Expect(collectorMountNames).NotTo(ContainElement(name))
		}
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

		_, found := findVolume(spec.Volumes, "data-collection")
		Expect(found).To(BeTrue())
		_, found = findVolumeMount(container.VolumeMounts, "data-collection")
		Expect(found).To(BeTrue())
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

type telemetryPullSecretErrorClient struct {
	client.Client
	err error
}

func (c telemetryPullSecretErrorClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if key.Namespace == utils.TelemetryPullSecretNamespace && key.Name == utils.TelemetryPullSecretName {
		return c.err
	}
	return c.Client.Get(ctx, key, obj, opts...)
}
