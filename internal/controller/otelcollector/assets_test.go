package otelcollector

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

var _ = Describe("OTEL Collector assets", func() {
	var testCR *olsv1alpha1.OLSConfig
	labels := utils.GenerateOtelCollectorSelectorLabels()

	BeforeEach(func() {
		testCR = utils.GetDefaultOLSConfigCR()
	})

	It("should generate the collector ConfigMap with logging and routing defaults", func() {
		cm, err := GenerateOtelCollectorConfigMap(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(cm.Name).To(Equal(utils.OtelCollectorConfigMapName))
		Expect(cm.Namespace).To(Equal(utils.OLSNamespaceDefault))
		Expect(cm.Labels).To(Equal(labels))

		configYAML := cm.Data[utils.OtelCollectorConfigMapDataKey]
		Expect(configYAML).To(ContainSubstring("cert_file: " + utils.OtelCollectorServingCertTLSFile))
		Expect(configYAML).To(ContainSubstring("key_file: " + utils.OtelCollectorServingCertTLSKeyFile))
		Expect(configYAML).To(ContainSubstring(`attributes["service.name"] == "` + utils.OtelSandboxServiceName + `"`))
		Expect(configYAML).To(ContainSubstring("connection_string: ${env:POSTGRES_CONNECTION_STRING}"))
		Expect(configYAML).To(ContainSubstring("schema: templogs"))
		Expect(configYAML).To(ContainSubstring("logs/unmatched"))
		Expect(configYAML).To(ContainSubstring("- logs/unmatched"))
		Expect(configYAML).NotTo(ContainSubstring("TRACES_BACKEND_ENDPOINT"))
		Expect(configYAML).To(ContainSubstring("nop:"))
		Expect(configYAML).To(ContainSubstring("- nop"))
		Expect(configYAML).To(ContainSubstring(utils.OtelCollectorHTTPSMetricsExtension + ":"))
		Expect(configYAML).To(ContainSubstring("upstream: " + utils.OtelCollectorMetricsUpstreamURL))
		Expect(configYAML).To(ContainSubstring("host: 127.0.0.1"))
		Expect(configYAML).To(ContainSubstring(fmt.Sprintf("port: %d", utils.OtelCollectorMetricsInternalPort)))
		Expect(configYAML).To(ContainSubstring("without_type_suffix: true"))
		Expect(configYAML).To(ContainSubstring("without_units: true"))
	})

	It("should omit postgres pipelines when audit logging is disabled", func() {
		testCR.Spec.Audit.Logging = boolPtr(false)
		cm, err := GenerateOtelCollectorConfigMap(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())

		configYAML := cm.Data[utils.OtelCollectorConfigMapDataKey]
		Expect(configYAML).To(ContainSubstring("nop:"))
		Expect(configYAML).NotTo(ContainSubstring("routing/logs"))
		Expect(configYAML).NotTo(ContainSubstring("postgres:"))
		Expect(configYAML).To(ContainSubstring("postgres_admin"))
		Expect(configYAML).To(ContainSubstring("health_check"))
		Expect(configYAML).To(ContainSubstring("file_storage"))
		Expect(configYAML).To(ContainSubstring(utils.OtelCollectorHTTPSMetricsExtension + ":"))
	})

	It("should add trace export when tracingEndpoint is set", func() {
		testCR.Spec.Audit.TracingEndpoint = "jaeger-collector:4317"
		cm, err := GenerateOtelCollectorConfigMap(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())

		configYAML := cm.Data[utils.OtelCollectorConfigMapDataKey]
		Expect(configYAML).To(ContainSubstring("otlp/tracing"))
		Expect(configYAML).To(ContainSubstring("${env:TRACES_BACKEND_ENDPOINT}"))
		Expect(configYAML).NotTo(ContainSubstring("routing/traces"))
	})

	It("should generate the collector Service with serving-cert annotation", func() {
		svc, err := GenerateOtelCollectorService(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(svc.Name).To(Equal(utils.OtelCollectorServiceName))
		Expect(svc.Labels).To(Equal(labels))
		Expect(svc.Annotations[utils.ServingCertSecretAnnotationKey]).To(Equal(utils.OtelCollectorCertsSecretName))
		Expect(svc.Spec.Ports).To(HaveLen(4))
		Expect(svc.Spec.Ports[0].Port).To(Equal(int32(utils.OtelCollectorGRPCPort)))
		Expect(svc.Spec.Ports[1].Port).To(Equal(int32(utils.OtelCollectorHTTPPort)))
		Expect(svc.Spec.Ports[2].Name).To(Equal("admin"))
		Expect(svc.Spec.Ports[2].Port).To(Equal(int32(utils.OtelCollectorAdminPort)))
		Expect(svc.Spec.Ports[3].Name).To(Equal("metrics"))
		Expect(svc.Spec.Ports[3].Port).To(Equal(int32(utils.OtelCollectorMetricsPort)))
	})

	It("should generate the collector NetworkPolicy for in-namespace and Prometheus metrics ingress", func() {
		np, err := GenerateOtelCollectorNetworkPolicy(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(np.Name).To(Equal(utils.OtelCollectorNetworkPolicyName))
		Expect(np.Labels).To(Equal(labels))
		Expect(np.Spec.Ingress).To(ConsistOf(
			networkingv1.NetworkPolicyIngressRule{
				From: []networkingv1.NetworkPolicyPeer{
					{
						PodSelector: &metav1.LabelSelector{},
					},
				},
				Ports: []networkingv1.NetworkPolicyPort{
					{
						Protocol: protocolTCP(),
						Port:     intstrPtr(intstr.FromInt32(utils.OtelCollectorGRPCPort)),
					},
					{
						Protocol: protocolTCP(),
						Port:     intstrPtr(intstr.FromInt32(utils.OtelCollectorAdminPort)),
					},
				},
			},
			networkingv1.NetworkPolicyIngressRule{
				From: []networkingv1.NetworkPolicyPeer{
					{
						PodSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "app.kubernetes.io/name",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"prometheus"},
								},
								{
									Key:      "prometheus",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"k8s"},
								},
							},
						},
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"kubernetes.io/metadata.name": utils.ClientCACmNamespace,
							},
						},
					},
				},
				Ports: []networkingv1.NetworkPolicyPort{
					{
						Protocol: protocolTCP(),
						Port:     intstrPtr(intstr.FromInt32(utils.OtelCollectorMetricsPort)),
					},
				},
			},
		))
	})

	It("should generate the collector ServiceMonitor for HTTPS metrics scraping", func() {
		sm, err := GenerateOtelCollectorServiceMonitor(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(sm.Name).To(Equal(utils.OtelCollectorServiceMonitorName))
		Expect(sm.Namespace).To(Equal(utils.OLSNamespaceDefault))
		Expect(sm.Labels).To(HaveKeyWithValue("openshift.io/user-monitoring", "false"))
		Expect(sm.Labels).To(HaveKeyWithValue("monitoring.openshift.io/collection-profile", "full"))

		valFalse := false
		serverName := fmt.Sprintf("%s.%s.svc", utils.OtelCollectorServiceName, utils.OLSNamespaceDefault)
		var schemeHTTPS monv1.Scheme = "https"
		Expect(sm.Spec.Endpoints).To(ConsistOf(monv1.Endpoint{
			Port:     "metrics",
			Path:     utils.OtelCollectorMetricsPath,
			Interval: "30s",
			Scheme:   &schemeHTTPS,
			HTTPConfigWithProxyAndTLSFiles: monv1.HTTPConfigWithProxyAndTLSFiles{
				HTTPConfigWithTLSFiles: monv1.HTTPConfigWithTLSFiles{
					TLSConfig: &monv1.TLSConfig{
						TLSFilesConfig: monv1.TLSFilesConfig{
							CAFile: "/etc/prometheus/configmaps/serving-certs-ca-bundle/service-ca.crt",
						},
						SafeTLSConfig: monv1.SafeTLSConfig{
							InsecureSkipVerify: &valFalse,
							ServerName:         &serverName,
						},
					},
				},
			},
		}))
		Expect(sm.Spec.Selector.MatchLabels).To(Equal(labels))
		Expect(sm.Spec.Endpoints[0].Authorization).To(BeNil())
		Expect(sm.Spec.Endpoints[0].TLSConfig.CertFile).To(BeEmpty())
		Expect(sm.Spec.Endpoints[0].TLSConfig.KeyFile).To(BeEmpty())
	})

	It("should generate the collector ServiceAccount", func() {
		sa, err := GenerateOtelCollectorServiceAccount(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(sa.Name).To(Equal(utils.OtelCollectorServiceAccountName))
		Expect(sa.Namespace).To(Equal(utils.OLSNamespaceDefault))
	})

	It("should generate collector Postgres DSN Secret from lightspeed-postgres-secret", func() {
		ensurePostgresSecret()
		secret, err := GenerateOtelCollectorPostgresSecret(testReconcilerInstance, ctx, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(secret.Name).To(Equal(utils.OtelCollectorPostgresDSNSecretName))
		Expect(secret.Namespace).To(Equal(utils.OLSNamespaceDefault))
		dsn := string(secret.Data[utils.OtelCollectorPostgresConnectionStringSecretKey])
		Expect(dsn).To(ContainSubstring(utils.PostgresDefaultUser))
		Expect(dsn).To(ContainSubstring(utils.PostgresDefaultDbName))
		Expect(dsn).To(ContainSubstring("sslmode=" + utils.PostgresDefaultSSLMode))
		Expect(dsn).To(ContainSubstring(utils.PostgresServiceName))
	})
})

func protocolTCP() *corev1.Protocol {
	p := corev1.ProtocolTCP
	return &p
}

func intstrPtr(v intstr.IntOrString) *intstr.IntOrString {
	return &v
}
