package otelcollector

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/yaml"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

type collectorExporterTLSForTest struct {
	Insecure                 bool   `json:"insecure"`
	CAFile                   string `json:"ca_file"`
	IncludeSystemCACertsPool bool   `json:"include_system_ca_certs_pool"`
}

type collectorExporterRotationForTest struct {
	MaxMegabytes int `json:"max_megabytes"`
	MaxBackups   int `json:"max_backups"`
	MaxDays      int `json:"max_days"`
}

type collectorExporterForTest struct {
	Endpoint         string                           `json:"endpoint"`
	ConnectionString string                           `json:"connection_string"`
	Schema           string                           `json:"schema"`
	LogsTable        string                           `json:"logs_table"`
	Path             string                           `json:"path"`
	Format           string                           `json:"format"`
	CreateDirectory  bool                             `json:"create_directory"`
	Rotation         collectorExporterRotationForTest `json:"rotation"`
	TLS              collectorExporterTLSForTest      `json:"tls"`
}

type otelDataverseExporterConfigForTest struct {
	DataMode           string `json:"data_mode"`
	DataDir            string `json:"data_dir"`
	OtelActiveFile     string `json:"otel_active_file"`
	LedgerFile         string `json:"ledger_file"`
	ArchivePathPrefix  string `json:"archive_path_prefix"`
	CollectionInterval int    `json:"collection_interval"`
	CleanupAfterSend   *bool  `json:"cleanup_after_send"`
	ServiceID          string `json:"service_id"`
	IngressServerURL   string `json:"ingress_server_url"`
	IngressAuthToken   string `json:"ingress_server_auth_token"`
	ClusterID          string `json:"cluster_id"`
}

type collectorRoutingRuleForTest struct {
	Context   string   `json:"context"`
	Condition string   `json:"condition"`
	Pipelines []string `json:"pipelines"`
}

type collectorConnectorForTest struct {
	DefaultPipelines []string                      `json:"default_pipelines"`
	Table            []collectorRoutingRuleForTest `json:"table"`
}

type collectorPipelineForTest struct {
	Receivers  []string `json:"receivers"`
	Processors []string `json:"processors"`
	Exporters  []string `json:"exporters"`
}

type collectorConfigForTest struct {
	Exporters  map[string]collectorExporterForTest  `json:"exporters"`
	Connectors map[string]collectorConnectorForTest `json:"connectors"`
	Service    struct {
		Pipelines map[string]collectorPipelineForTest `json:"pipelines"`
	} `json:"service"`
}

func decodeCollectorConfigForTest(configYAML string) collectorConfigForTest {
	var config collectorConfigForTest
	Expect(yaml.Unmarshal([]byte(configYAML), &config)).To(Succeed())
	return config
}

var _ = Describe("OTEL Collector assets", func() {
	var testCR *olsv1alpha1.OLSConfig
	labels := utils.GenerateOtelCollectorSelectorLabels()

	BeforeEach(func() {
		testCR = utils.GetDefaultOLSConfigCR()
	})

	It("should generate the collector ConfigMap with logging and trace collection defaults", func() {
		cm, err := GenerateOtelCollectorConfigMap(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(cm.Name).To(Equal(utils.OtelCollectorConfigMapName))
		Expect(cm.Namespace).To(Equal(utils.OLSNamespaceDefault))
		Expect(cm.Labels).To(Equal(labels))

		configYAML := cm.Data[utils.OtelCollectorConfigMapDataKey]
		Expect(configYAML).To(ContainSubstring("cert_file: " + utils.OtelCollectorServingCertTLSFile))
		Expect(configYAML).To(ContainSubstring("key_file: " + utils.OtelCollectorServingCertTLSKeyFile))
		Expect(configYAML).To(ContainSubstring(fmt.Sprintf("max_recv_msg_size_mib: %d", utils.OtelCollectorGRPCMaxRecvMsgSizeMiB)))
		Expect(configYAML).To(ContainSubstring(fmt.Sprintf("max_request_body_size: %d", utils.OtelCollectorHTTPMaxRequestBodySize)))
		Expect(configYAML).To(ContainSubstring(utils.OtelCollectorHTTPSMetricsExtension + ":"))
		Expect(configYAML).To(ContainSubstring("upstream: " + utils.OtelCollectorMetricsUpstreamURL))
		Expect(configYAML).To(ContainSubstring("host: 127.0.0.1"))
		Expect(configYAML).To(ContainSubstring(fmt.Sprintf("port: %d", utils.OtelCollectorMetricsInternalPort)))
		Expect(configYAML).To(ContainSubstring("without_type_suffix: true"))
		Expect(configYAML).To(ContainSubstring("without_units: true"))

		decoded := decodeCollectorConfigForTest(configYAML)
		Expect(decoded.Service.Pipelines).To(HaveKey("traces"))
		Expect(decoded.Exporters).To(HaveKey("file/data_collection"))
		Expect(decoded.Exporters).NotTo(HaveKey("otlp/tracing"))
		Expect(decoded.Exporters["postgres"].ConnectionString).To(Equal("${env:POSTGRES_CONNECTION_STRING}"))
		Expect(decoded.Exporters["postgres"].Schema).To(Equal("templogs"))
		Expect(decoded.Exporters["postgres"].LogsTable).To(Equal("logs"))

		fileExporter := decoded.Exporters["file/data_collection"]
		Expect(fileExporter.Path).To(Equal("/var/lib/lightspeed-data/otel/traces.jsonl"))
		Expect(fileExporter.Format).To(Equal("json"))
		Expect(fileExporter.CreateDirectory).To(BeTrue())

		Expect(fileExporter.Rotation.MaxMegabytes).To(Equal(10))
		Expect(fileExporter.Rotation.MaxBackups).To(Equal(40))
		Expect(fileExporter.Rotation.MaxDays).To(Equal(1))

		dataCollectionRoute := decoded.Connectors["routing/data_collection"]
		Expect(dataCollectionRoute.DefaultPipelines).To(BeEmpty())
		Expect(dataCollectionRoute.Table).To(ConsistOf(collectorRoutingRuleForTest{
			Context:   "resource",
			Condition: `attributes["service.name"] == "lightspeed-agentic-operator" or attributes["service.name"] == "lightspeed-agentic-sandbox"`,
			Pipelines: []string{"traces/data_collection"},
		}))
		logsRoute := decoded.Connectors["routing/logs"]
		Expect(logsRoute.DefaultPipelines).To(Equal([]string{"logs/unmatched"}))
		Expect(logsRoute.Table).To(ConsistOf(collectorRoutingRuleForTest{
			Condition: fmt.Sprintf(`resource.attributes["service.name"] == %q`, utils.OtelSandboxServiceName),
			Pipelines: []string{"logs/postgres"},
		}))

		tracePipeline := decoded.Service.Pipelines["traces"]
		Expect(tracePipeline.Receivers).To(Equal([]string{"otlp"}))
		Expect(tracePipeline.Processors).To(BeEmpty())
		Expect(tracePipeline.Exporters).To(Equal([]string{"nop", "routing/data_collection"}))
		Expect(decoded.Service.Pipelines["traces/data_collection"]).To(Equal(collectorPipelineForTest{
			Receivers: []string{"routing/data_collection"},
			Exporters: []string{"file/data_collection"},
		}))
		Expect(decoded.Service.Pipelines["logs"].Processors).To(Equal([]string{"batch"}))
		Expect(decoded.Service.Pipelines["logs"].Exporters).To(Equal([]string{"routing/logs"}))
		Expect(decoded.Service.Pipelines["logs/postgres"].Exporters).To(Equal([]string{"postgres"}))
	})
	It("should omit trace collection when transcripts are disabled", func() {
		testCR.Spec.OLSConfig.UserDataCollection.TranscriptsDisabled = true
		cm, err := GenerateOtelCollectorConfigMap(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())

		decoded := decodeCollectorConfigForTest(cm.Data[utils.OtelCollectorConfigMapDataKey])
		Expect(decoded.Exporters).NotTo(HaveKey("file/data_collection"))
		Expect(decoded.Connectors).NotTo(HaveKey("routing/data_collection"))
		Expect(decoded.Service.Pipelines).NotTo(HaveKey("traces/data_collection"))
		Expect(decoded.Service.Pipelines).NotTo(HaveKey("traces/lightspeed"))
		Expect(decoded.Service.Pipelines["traces"].Processors).To(BeEmpty())
		Expect(decoded.Service.Pipelines["traces"].Exporters).To(Equal([]string{"nop"}))
	})

	It("should keep trace collection, forwarding, and log routing independent across settings", func() {
		cases := []struct {
			name              string
			collectionEnabled bool
			backendEnabled    bool
			loggingEnabled    bool
		}{
			{name: "collection and logs without backend", collectionEnabled: true, loggingEnabled: true},
			{name: "collection, logs, and backend", collectionEnabled: true, backendEnabled: true, loggingEnabled: true},
			{name: "collection without logs or backend", collectionEnabled: true},
			{name: "collection and backend without logs", collectionEnabled: true, backendEnabled: true},
			{name: "logs without collection or backend", loggingEnabled: true},
			{name: "logs and backend without collection", backendEnabled: true, loggingEnabled: true},
			{name: "neither collection, logs, nor backend"},
			{name: "backend without collection or logs", backendEnabled: true},
		}
		for _, testCase := range cases {
			By(testCase.name)
			scenario := utils.GetDefaultOLSConfigCR()
			scenario.Spec.OLSConfig.UserDataCollection.TranscriptsDisabled = !testCase.collectionEnabled
			scenario.Spec.Audit.Logging = boolPtr(testCase.loggingEnabled)
			if testCase.backendEnabled {
				scenario.Spec.Audit.TracingEndpoint = "traces.example:4317"
			}
			cm, err := GenerateOtelCollectorConfigMap(testReconcilerInstance, scenario)
			Expect(err).NotTo(HaveOccurred())
			decoded := decodeCollectorConfigForTest(cm.Data[utils.OtelCollectorConfigMapDataKey])

			_, hasFileExporter := decoded.Exporters["file/data_collection"]
			Expect(hasFileExporter).To(Equal(testCase.collectionEnabled))
			_, hasDataCollectionRoute := decoded.Connectors["routing/data_collection"]
			Expect(hasDataCollectionRoute).To(Equal(testCase.collectionEnabled))
			_, hasDataCollectionPipeline := decoded.Service.Pipelines["traces/data_collection"]
			Expect(hasDataCollectionPipeline).To(Equal(testCase.collectionEnabled))
			_, hasBackendExporter := decoded.Exporters["otlp/tracing"]
			Expect(hasBackendExporter).To(Equal(testCase.backendEnabled))
			_, hasTraceRouting := decoded.Connectors["routing/traces"]
			Expect(hasTraceRouting).To(Equal(testCase.collectionEnabled && testCase.backendEnabled))

			tracePipeline, found := decoded.Service.Pipelines["traces"]
			Expect(found).To(BeTrue())
			Expect(tracePipeline.Receivers).To(Equal([]string{"otlp"}))
			switch {
			case testCase.collectionEnabled && testCase.backendEnabled:
				Expect(tracePipeline.Processors).To(BeEmpty())
				Expect(tracePipeline.Exporters).To(Equal([]string{"routing/traces", "routing/data_collection"}))
				Expect(decoded.Service.Pipelines["traces/lightspeed"].Processors).To(Equal([]string{"batch"}))
				Expect(decoded.Service.Pipelines["traces/lightspeed"].Exporters).To(Equal([]string{"otlp/tracing"}))
				Expect(decoded.Service.Pipelines["traces/data_collection"].Processors).To(BeEmpty())
			case testCase.collectionEnabled:
				Expect(tracePipeline.Processors).To(BeEmpty())
				Expect(tracePipeline.Exporters).To(Equal([]string{"nop", "routing/data_collection"}))
				Expect(decoded.Service.Pipelines).NotTo(HaveKey("traces/lightspeed"))
			case testCase.backendEnabled:
				Expect(tracePipeline.Processors).To(Equal([]string{"batch"}))
				Expect(tracePipeline.Exporters).To(Equal([]string{"otlp/tracing"}))
				Expect(decoded.Service.Pipelines).NotTo(HaveKey("traces/lightspeed"))
			default:
				Expect(tracePipeline.Processors).To(BeEmpty())
				Expect(tracePipeline.Exporters).To(Equal([]string{"nop"}))
				Expect(decoded.Service.Pipelines).NotTo(HaveKey("traces/lightspeed"))
			}

			logPipeline, hasLogPipeline := decoded.Service.Pipelines["logs"]
			Expect(hasLogPipeline).To(BeTrue())
			if testCase.loggingEnabled {
				Expect(decoded.Connectors).To(HaveKey("routing/logs"))
				Expect(decoded.Exporters).To(HaveKey("postgres"))
				Expect(decoded.Service.Pipelines).To(HaveKey("logs/postgres"))
				Expect(logPipeline.Exporters).To(Equal([]string{"routing/logs"}))
			} else {
				Expect(decoded.Connectors).NotTo(HaveKey("routing/logs"))
				Expect(decoded.Exporters).NotTo(HaveKey("postgres"))
				Expect(decoded.Service.Pipelines).NotTo(HaveKey("logs/postgres"))
				Expect(decoded.Service.Pipelines).NotTo(HaveKey("logs/unmatched"))
				Expect(logPipeline.Exporters).To(Equal([]string{"nop"}))
			}
		}
	})

	It("should omit postgres pipelines when audit logging is disabled", func() {
		testCR.Spec.Audit.Logging = boolPtr(false)
		cm, err := GenerateOtelCollectorConfigMap(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())

		configYAML := cm.Data[utils.OtelCollectorConfigMapDataKey]
		decoded := decodeCollectorConfigForTest(configYAML)
		Expect(decoded.Exporters).NotTo(HaveKey("postgres"))
		Expect(decoded.Connectors).NotTo(HaveKey("routing/logs"))
		Expect(decoded.Service.Pipelines["logs"].Exporters).To(Equal([]string{"nop"}))
		Expect(decoded.Exporters).To(HaveKey("file/data_collection"))
		Expect(decoded.Service.Pipelines).To(HaveKey("traces/data_collection"))
		Expect(configYAML).To(ContainSubstring("postgres_admin"))
		Expect(configYAML).To(ContainSubstring("health_check"))
		Expect(configYAML).To(ContainSubstring("file_storage"))
		Expect(configYAML).To(ContainSubstring(utils.OtelCollectorHTTPSMetricsExtension + ":"))
	})
	It("should batch trace forwarding separately from the collection pipeline", func() {
		testCR.Spec.Audit.TracingEndpoint = "jaeger-collector:4317"
		cm, err := GenerateOtelCollectorConfigMap(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())

		decoded := decodeCollectorConfigForTest(cm.Data[utils.OtelCollectorConfigMapDataKey])
		traceExporter := decoded.Exporters["otlp/tracing"]
		Expect(traceExporter.Endpoint).To(Equal("${env:" + utils.OtelCollectorTracesBackendEndpointEnvVar + "}"))
		Expect(traceExporter.TLS.Insecure).To(BeFalse())
		Expect(traceExporter.TLS.CAFile).To(Equal(utils.OtelCollectorServiceCAFile))
		Expect(traceExporter.TLS.IncludeSystemCACertsPool).To(BeTrue())

		tracePipeline := decoded.Service.Pipelines["traces"]
		Expect(tracePipeline.Processors).To(BeEmpty())
		Expect(tracePipeline.Exporters).To(Equal([]string{"routing/traces", "routing/data_collection"}))
		Expect(decoded.Connectors["routing/traces"].DefaultPipelines).To(BeEmpty())
		Expect(decoded.Connectors["routing/traces"].Table).To(ConsistOf(collectorRoutingRuleForTest{
			Context:   "resource",
			Condition: "true",
			Pipelines: []string{"traces/lightspeed"},
		}))
		Expect(decoded.Service.Pipelines["traces/lightspeed"]).To(Equal(collectorPipelineForTest{
			Receivers:  []string{"routing/traces"},
			Processors: []string{"batch"},
			Exporters:  []string{"otlp/tracing"},
		}))
		Expect(decoded.Service.Pipelines["traces/data_collection"].Processors).To(BeEmpty())
		Expect(decoded.Service.Pipelines["traces/data_collection"].Exporters).To(Equal([]string{"file/data_collection"}))
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
		sm, err := generateOtelCollectorServiceMonitor(testReconcilerInstance, testCR)
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
	It("should generate a separate OTEL-mode Dataverse exporter config", func() {
		cm, err := GenerateOtelDataverseExporterConfigMap(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(cm.Name).To(Equal("lightspeed-otel-dataverse-exporter-config"))
		Expect(cm.Namespace).To(Equal(utils.OLSNamespaceDefault))
		Expect(cm.Data).To(HaveKey("config.yaml"))

		var config otelDataverseExporterConfigForTest
		Expect(yaml.Unmarshal([]byte(cm.Data["config.yaml"]), &config)).To(Succeed())
		cleanupAfterSend := false
		Expect(config).To(Equal(otelDataverseExporterConfigForTest{
			DataMode:           "otel",
			DataDir:            "/input",
			OtelActiveFile:     "traces.jsonl",
			LedgerFile:         "/state/ledger.json",
			ArchivePathPrefix:  "v1/",
			CollectionInterval: 300,
			CleanupAfterSend:   &cleanupAfterSend,
			ServiceID:          "ols",
			IngressServerURL:   "https://console.redhat.com/api/ingress/v1/upload",
		}))
		Expect(cm.Data["config.yaml"]).NotTo(ContainSubstring("ingress_server_auth_token"))
		Expect(cm.Data["config.yaml"]).NotTo(ContainSubstring("cluster_id"))

		testCR.Labels = map[string]string{utils.RHOSOLightspeedOwnerIDLabel: "rhoso"}
		cm, err = GenerateOtelDataverseExporterConfigMap(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(yaml.Unmarshal([]byte(cm.Data["config.yaml"]), &config)).To(Succeed())
		Expect(config.ServiceID).To(Equal("rhos-lightspeed"))
	})

	It("should generate least-privilege OpenShift auth RBAC", func() {
		clusterRole, err := GenerateOtelDataverseExporterClusterRole(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(clusterRole.Name).To(Equal(utils.OtelDataverseExporterClusterRoleName))
		Expect(clusterRole.Rules).To(ConsistOf(rbacv1.PolicyRule{
			APIGroups:     []string{"config.openshift.io"},
			Resources:     []string{"clusterversions"},
			ResourceNames: []string{"version"},
			Verbs:         []string{"get"},
		}))

		clusterRoleBinding, err := GenerateOtelDataverseExporterClusterRoleBinding(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(clusterRoleBinding.Name).To(Equal(utils.OtelDataverseExporterClusterRoleBindingName))
		Expect(clusterRoleBinding.RoleRef).To(Equal(rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     utils.OtelDataverseExporterClusterRoleName,
		}))
		Expect(clusterRoleBinding.Subjects).To(ConsistOf(rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      utils.OtelCollectorServiceAccountName,
			Namespace: utils.OLSNamespaceDefault,
		}))

		pullSecretClusterRole, err := GenerateOtelDataverseExporterPullSecretClusterRole(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(pullSecretClusterRole.Name).To(Equal(utils.OtelDataverseExporterPullSecretClusterRoleName))
		Expect(pullSecretClusterRole.Rules).To(ConsistOf(rbacv1.PolicyRule{
			APIGroups:     []string{""},
			Resources:     []string{"secrets"},
			ResourceNames: []string{utils.TelemetryPullSecretName},
			Verbs:         []string{"get"},
		}))

		pullSecretRoleBinding, err := GenerateOtelDataverseExporterPullSecretRoleBinding(testReconcilerInstance, testCR)
		Expect(err).NotTo(HaveOccurred())
		Expect(pullSecretRoleBinding.Name).To(Equal(utils.OtelDataverseExporterPullSecretRoleBindingName))
		Expect(pullSecretRoleBinding.Name).To(Equal(pullSecretClusterRole.Name))
		Expect(pullSecretRoleBinding.Namespace).To(Equal(utils.TelemetryPullSecretNamespace))
		Expect(pullSecretRoleBinding.RoleRef).To(Equal(rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     utils.OtelDataverseExporterPullSecretClusterRoleName,
		}))
		Expect(pullSecretRoleBinding.Subjects).To(ConsistOf(rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      utils.OtelCollectorServiceAccountName,
			Namespace: utils.OLSNamespaceDefault,
		}))
	})

})

func protocolTCP() *corev1.Protocol {
	p := corev1.ProtocolTCP
	return &p
}

func intstrPtr(v intstr.IntOrString) *intstr.IntOrString {
	return &v
}
