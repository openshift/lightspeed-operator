package otelcollector

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
		Expect(configYAML).NotTo(ContainSubstring("TRACES_BACKEND_ENDPOINT"))
	})

	It("should generate the client connectivity ConfigMap for agentic-operator", func() {
		ensureCollectorTLSSecret()

		cm, err := GenerateOtelCollectorClientConfigMap(testReconcilerInstance, ctx, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(cm.Name).To(Equal(utils.OtelCollectorClientConfigMapName))
		Expect(cm.Namespace).To(Equal(utils.OLSNamespaceDefault))
		Expect(cm.Labels).To(Equal(labels))

		secret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      utils.OtelCollectorCertsSecretName,
			Namespace: utils.OLSNamespaceDefault,
		}, secret)).To(Succeed())

		host := utils.OtelCollectorServiceName + "." + utils.OLSNamespaceDefault + ".svc"
		Expect(cm.Data[utils.OtelCollectorClientCollectorEndpointKey]).To(Equal(
			fmt.Sprintf("%s:%d", host, utils.OtelCollectorGRPCPort),
		))
		Expect(cm.Data[utils.OtelCollectorClientAdminEndpointKey]).To(Equal(
			fmt.Sprintf("https://%s:%d", host, utils.OtelCollectorAdminPort),
		))
		Expect(cm.Data[utils.OtelCollectorClientCACertKey]).To(Equal(string(secret.Data["tls.crt"])))
		Expect(cm.Data).NotTo(HaveKey(utils.OtelCollectorClientCredentialsSecretKey))
	})

	It("should omit postgres pipelines when audit logging is disabled", func() {
		testCR.Spec.Audit.Logging = boolPtr(false)
		cm, err := GenerateOtelCollectorConfigMap(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())

		configYAML := cm.Data[utils.OtelCollectorConfigMapDataKey]
		Expect(configYAML).NotTo(ContainSubstring("exporters:"))
		Expect(configYAML).NotTo(ContainSubstring("routing/logs"))
		Expect(configYAML).To(ContainSubstring("postgres_admin"))
		Expect(configYAML).To(ContainSubstring("health_check"))
		Expect(configYAML).To(ContainSubstring("file_storage"))
	})

	It("should add trace export when tracingEndpoint is set", func() {
		testCR.Spec.Audit.TracingEndpoint = "jaeger-collector:4317"
		cm, err := GenerateOtelCollectorConfigMap(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())

		configYAML := cm.Data[utils.OtelCollectorConfigMapDataKey]
		Expect(configYAML).To(ContainSubstring("otlp/tracing"))
		Expect(configYAML).To(ContainSubstring("${env:TRACES_BACKEND_ENDPOINT}"))
		Expect(configYAML).To(ContainSubstring("traces/lightspeed"))
	})

	It("should generate the collector Service with serving-cert annotation", func() {
		svc, err := GenerateOtelCollectorService(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(svc.Name).To(Equal(utils.OtelCollectorServiceName))
		Expect(svc.Labels).To(Equal(labels))
		Expect(svc.Annotations[utils.ServingCertSecretAnnotationKey]).To(Equal(utils.OtelCollectorCertsSecretName))
		Expect(svc.Spec.Ports).To(HaveLen(3))
		Expect(svc.Spec.Ports[0].Port).To(Equal(int32(utils.OtelCollectorGRPCPort)))
		Expect(svc.Spec.Ports[1].Port).To(Equal(int32(utils.OtelCollectorHTTPPort)))
		Expect(svc.Spec.Ports[2].Name).To(Equal("admin"))
		Expect(svc.Spec.Ports[2].Port).To(Equal(int32(utils.OtelCollectorAdminPort)))
	})

	It("should generate the collector NetworkPolicy for in-namespace ingress on OTLP gRPC and admin HTTPS", func() {
		np, err := GenerateOtelCollectorNetworkPolicy(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(np.Name).To(Equal(utils.OtelCollectorNetworkPolicyName))
		Expect(np.Labels).To(Equal(labels))
		Expect(np.Spec.Ingress).To(ConsistOf(networkingv1.NetworkPolicyIngressRule{
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
		}))
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
