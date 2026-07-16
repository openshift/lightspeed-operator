package otelcollector

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
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

		spec := dep.Spec.Template.Spec
		Expect(spec.ServiceAccountName).To(Equal(utils.OtelCollectorServiceAccountName))
		Expect(spec.InitContainers).To(HaveLen(1))
		Expect(spec.InitContainers[0].Name).To(Equal(utils.PostgresWaitInitContainerName))

		container := spec.Containers[0]
		Expect(container.Name).To(Equal(utils.OtelCollectorContainerName))
		Expect(container.Image).To(Equal(utils.OtelCollectorImageDefault))
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
		Expect(portNames).To(ConsistOf("otlp-grpc", "otlp-http", "admin"))
		Expect(container.ReadinessProbe.HTTPGet.Port.IntValue()).To(Equal(int(utils.OtelCollectorHealthCheckPort)))
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
		Expect(portNames).To(ConsistOf("otlp-grpc", "otlp-http", "admin"))
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
