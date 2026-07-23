package utils

import (
	"time"

	"github.com/openshift/lightspeed-operator/internal/relatedimages"
)

const (
	/*** Volume Permissions ***/
	// VolumeDefaultMode is the default permission for mounted volumes (0644 - read/write for owner, read-only for group/others)
	VolumeDefaultMode = int32(420)
	// VolumeRestrictedMode is for sensitive volumes like secrets (0600 - read/write for owner only)
	VolumeRestrictedMode = int32(0600)

	/*** Operator Settings ***/
	// OLSConfigName is the name of the OLSConfig Custom Resource
	OLSConfigName = "cluster"
	// OLSConfigKind is the Kind of the OLSConfig Custom Resource
	OLSConfigKind = "OLSConfig"
	// OLSConfigAPIVersion is the APIVersion of the OLSConfig Custom Resource
	OLSConfigAPIVersion = "ols.openshift.io/v1alpha1"
	// OLSConfigFinalizer is the finalizer for OLSConfig CR to ensure proper cleanup
	OLSConfigFinalizer = "ols.openshift.io/finalizer"
	// OperatorCertDirDefault is the default directory for storing the operator certificate
	OperatorCertDirDefault = "/etc/tls/private"
	// OperatorCertNameDefault is the default name of the operator certificate
	OperatorCertNameDefault = "tls.crt"
	// OperatorKeyNameDefault is the default name of the operator key
	OperatorKeyNameDefault = "tls.key"
	// OperatorCACertPathDefault is the default path to the CA certificate
	OperatorCACertPathDefault = "/etc/tls/private/ca.crt"
	// ClientCACmName is the name of the client CA configmap
	ClientCACmName = "metrics-client-ca"
	// ClientCACmNamespace is the namespace of the client CA configmap
	ClientCACmNamespace = "openshift-monitoring"
	// ClientCACertKey is the key of the client CA certificate in the configmap
	ClientCACertKey = "client-ca.crt"
	// ResourceCreationTimeout is the maximum time in seconds operator waiting for creating resources
	ResourceCreationTimeout = 60 * time.Second
	// EnvTestCRDInstallMaxTime is the envtest CRD install wait budget for unit-test suites.
	// CI runs package tests in parallel; the default envtest deadline is too tight.
	EnvTestCRDInstallMaxTime = 3 * time.Minute

	/*** application server configuration file ***/
	// OLSConfigName is the name of the OLSConfig configmap
	OLSConfigCmName = "olsconfig"
	// OLSCAConfigMap is the name of the OLS TLS ca certificate configmap
	OLSCAConfigMap = "openshift-service-ca.crt"
	// OLSNamespaceDefault is the default namespace for OLS
	OLSNamespaceDefault = "openshift-lightspeed"
	// OLSAppServerServiceAccountName is the name of service account running the application server
	OLSAppServerServiceAccountName = "lightspeed-app-server"
	// OLSAppServerSARRoleName is the name of the SAR role for the service account running the application server
	OLSAppServerSARRoleName = OLSAppServerServiceAccountName + "-sar-role"
	// OLSAppServerSARRoleBindingName is the name of the SAR role binding for the service account running the application server
	OLSAppServerSARRoleBindingName = OLSAppServerSARRoleName + "-binding"
	// OLSAppServerDeploymentName is the name of the OLS application server deployment
	OLSAppServerDeploymentName = "lightspeed-app-server"
	// APIKeyMountRoot is the directory hosting the API key file in the container
	APIKeyMountRoot = "/etc/apikeys" // #nosec G101
	// CredentialsMountRoot is the directory hosting the credential files in the container
	CredentialsMountRoot = "/etc/credentials"
	// OLSAppCertsMountRoot is the directory hosting the cert files in the container
	OLSAppCertsMountRoot = "/etc/certs"
	// OLSConfigMountRoot is the directory hosting the OLS configuration files in the container
	OLSConfigMountRoot = "/etc/ols"
	// OLSComponentPasswordFileName is the generic name of the password file for each of its components
	OLSComponentPasswordFileName = "password"
	// OLSConfigFilename is the name of the application server configuration file
	OLSConfigFilename = "olsconfig.yaml"
	// AppServerServiceMonitorName is the name of the service monitor for the OLS application server
	AppServerServiceMonitorName = "lightspeed-app-server-monitor"
	// AppServerPrometheusRuleName is the name of the prometheus rules for the OLS application server
	AppServerPrometheusRuleName = "lightspeed-app-server-prometheus-rule"
	// AppServerMetricsPath is the path of the metrics endpoint of the OLS application server
	AppServerMetricsPath = "/metrics"
	// AppAdditionalCACertDir is the directory for storing additional CA certificates in the app server container under OLSAppCertsMountRoot
	AppAdditionalCACertDir = "ols-additional-ca"
	// UserCACertDir is the directory for storing additional CA certificates in the app server container under OLSAppCertsMountRoot
	UserCACertDir = "ols-user-ca"
	// OpenShiftCAVolumeName is the name of the volume for OpenShift CA certificates
	OpenShiftCAVolumeName = "openshift-ca"
	// AdditionalCAVolumeName is the name of the volume for additional CA certificates provided by the user
	AdditionalCAVolumeName = "additional-ca"
	// CertBundleVolumeName is the name of the volume for the certificate bundle
	CertBundleVolumeName = "cert-bundle"
	// ProxyCACertFileName is the name of the proxy CA certificate file
	ProxyCACertFileName = "proxy-ca.crt"
	// ProxyCACertVolumeName is the name of the volume for the proxy CA certificate
	ProxyCACertVolumeName = "proxy-ca"
	// RAGVolumeName is the name of the volume hosting customized RAG content
	RAGVolumeName = "rag"
	// RAGVolumeMountPath is the path of the volume hosting customized RAG content
	RAGVolumeMountPath = "/rag-data"
	// OLSAppServerNetworkPolicyName is the name of the network policy for the OLS application server
	OLSAppServerNetworkPolicyName = "lightspeed-app-server"
	// FeatureGateMCPServer is the feature gate flag activating the MCP server
	FeatureGateMCPServer = "MCPServer"
	// FeatureGateToolFiltering is the feature gate flag activating tool filtering
	FeatureGateToolFiltering = "ToolFiltering"
	// OLSConfigHashKey is the key of the hash value of the OLSConfig configmap
	OLSConfigHashKey = "hash/olsconfig"
	// LLMProviderHashKey is the key of the hash value of OLS LLM provider credentials consolidated
	// #nosec G101
	LLMProviderHashKey = "hash/llmprovider"
	// OLSAppTLSHashKey is the key of the hash value of the OLS App TLS certificates
	OLSAppTLSHashKey = "hash/olstls"
	// OLSConsoleTLSHashKey is the key of the hash value of the OLS Console TLS certificates
	OLSConsoleTLSHashKey = "hash/olsconsoletls"
	// AdditionalCAHashKey is the key of the hash value of the additional CA certificates in the deployment annotations
	AdditionalCAHashKey = "hash/additionalca"
	// ProxyCACertHashAnnotation is the annotation key for tracking Proxy CA certificate content hash.
	ProxyCACertHashAnnotation = "ols.openshift.io/proxy-ca-configmap-hash"
	// OLSAppServerContainerPort is the port number of the lightspeed-service-api container exposes
	OLSAppServerContainerPort = 8443
	// OLSAppServerServicePort is the port number for OLS application server service.
	OLSAppServerServicePort = 8443
	// OLSAppServerServiceName is the name of the OLS application server service
	OLSAppServerServiceName = "lightspeed-app-server"
	// OLSCertsSecretName is the name of the TLS secret for OLS.
	OLSCertsSecretName = "lightspeed-tls" // #nosec G101
	// Annotation key for serving certificate secret name
	// #nosec G101
	ServingCertSecretAnnotationKey = "service.beta.openshift.io/serving-cert-secret-name"
	// DefaultCredentialKey is the default secret key name for provider credentials
	DefaultCredentialKey = "apitoken"
	// BedrockAccessKeyIDKey is the secret key for AWS access key ID (Bedrock IAM auth)
	BedrockAccessKeyIDKey = "aws_access_key_id"
	// BedrockSecretAccessKeyKey is the secret key for AWS secret access key (Bedrock IAM auth)
	BedrockSecretAccessKeyKey = "aws_secret_access_key" // #nosec G101
	// BedrockRoleARNKey is the optional secret key for STS role ARN (Bedrock IAM auth)
	BedrockRoleARNKey = "role_arn"
	// AzureOpenAIType is the name of the Azure OpenAI provider type
	AzureOpenAIType = "azure_openai"
	// FakeProviderType is the name of the fake provider type used for testing
	FakeProviderType = "fake_provider"
	// GoogleVertexType is the name of the Google Vertex generic provider type
	GoogleVertexType = "google_vertex"
	// GoogleVertexAnthropicType is the name of the Google Vertex Anthropic provider type
	GoogleVertexAnthropicType = "google_vertex_anthropic"
	// BedrockType is the name of the AWS Bedrock provider type
	BedrockType = "bedrock"
	// DeploymentInProgress message
	DeploymentInProgress = "In Progress"
	// OLSSystemPromptFileName is the filename for the system prompt
	OLSSystemPromptFileName = "system_prompt"
	// BYOK image stream annotation on the app server deployment
	OLSAppServerImageStreamTriggerAnnotation = "image.openshift.io/triggers"

	/*** console UI plugin ***/
	// ConsoleUIConfigMapName is the name of the console UI nginx configmap
	ConsoleUIConfigMapName = "lightspeed-console-plugin"
	// ConsoleUIServiceCertSecretName is the name of the console UI service certificate secret
	ConsoleUIServiceCertSecretName = "lightspeed-console-plugin-cert"
	// ConsoleUIServiceName is the name of the console UI service
	ConsoleUIServiceName = "lightspeed-console-plugin"
	// ConsoleUIDeploymentName is the name of the console UI deployment
	ConsoleUIDeploymentName = "lightspeed-console-plugin"
	// ConsoleUIHTTPSPort is the port number of the console UI service
	ConsoleUIHTTPSPort = 9443
	// ConsoleUIPluginName is the name of the console UI plugin
	ConsoleUIPluginName = "lightspeed-console-plugin"
	// ConsoleUIPluginDisplayName is the display name of the console UI plugin
	ConsoleUIPluginDisplayName = "Lightspeed Console"
	// ConsoleUIServiceAccountName is the name of the service account for the console UI plugin
	ConsoleUIServiceAccountName = "lightspeed-console-plugin"
	// ConsoleCRName is the name of the console custom resource
	ConsoleCRName = "cluster"
	// ConsoleProxyAlias is the alias of the console proxy
	// The console backend exposes following proxy endpoint: /api/proxy/plugin/<plugin-name>/<proxy-alias>/<request-path>?<optional-query-parameters>
	ConsoleProxyAlias = "ols"
	// ConsoleUINetworkPolicyName is the name of the network policy for the console UI plugin
	ConsoleUINetworkPolicyName = "lightspeed-console-plugin"

	/*** agentic console UI plugin ***/
	// AgenticConsoleUIConfigMapName is the name of the agentic console UI nginx configmap
	AgenticConsoleUIConfigMapName = "lightspeed-agentic-console-plugin"
	// AgenticConsoleUIServiceCertSecretName is the name of the agentic console UI service certificate secret
	AgenticConsoleUIServiceCertSecretName = "lightspeed-agentic-console-plugin-cert"
	// AgenticConsoleUIServiceName is the name of the agentic console UI service
	AgenticConsoleUIServiceName = "lightspeed-agentic-console-plugin"
	// AgenticConsoleUIDeploymentName is the name of the agentic console UI deployment
	AgenticConsoleUIDeploymentName = "lightspeed-agentic-console-plugin"
	// AgenticConsoleUIHTTPSPort is the port number of the agentic console UI service
	AgenticConsoleUIHTTPSPort = 9443
	// AgenticConsoleUIPluginName is the name of the agentic console UI plugin
	AgenticConsoleUIPluginName = "lightspeed-agentic-console-plugin"
	// AgenticConsoleUIPluginDisplayName is the display name of the agentic console UI plugin
	AgenticConsoleUIPluginDisplayName = "OpenShift Lightspeed Agentic Console Plugin"
	// AgenticConsoleUIServiceAccountName is the name of the service account for the agentic console UI plugin
	AgenticConsoleUIServiceAccountName = "lightspeed-agentic-console-plugin"
	// AgenticConsoleUINetworkPolicyName is the name of the network policy for the agentic console UI plugin
	AgenticConsoleUINetworkPolicyName = "lightspeed-agentic-console-plugin"
	// AgenticConsoleUIContainerName is the name of the agentic console UI container
	AgenticConsoleUIContainerName = "console"
	/*** watchers ***/
	// Watcher Annotation key
	WatcherAnnotationKey = "ols.openshift.io/watcher"
	// ConfigMap with default openshift certificates
	DefaultOpenShiftCerts = "kube-root-ca.crt"
	// Force reload annotation key
	ForceReloadAnnotationKey = "ols.openshift.io/force-reload"
	/*** Postgres Constants ***/
	// PostgresCAVolume is the name of the OLS Postgres TLS ca certificate volume name
	PostgresCAVolume = "cm-olspostgresca"
	// PostgresDeploymentName is the name of OLS application Postgres deployment
	PostgresDeploymentName = "lightspeed-postgres-server"
	// PostgresWaitInitContainerName is the name of the init container that waits for Postgres to accept connections
	PostgresWaitInitContainerName = "wait-for-postgres"
	// PostgresSecretKeyName is the name of the key holding Postgres server secret
	PostgresSecretKeyName = "password"
	// PostgresDefaultUser is the default user name for postgres
	PostgresDefaultUser = "postgres"
	// PostgresDefaultDbName is the default db name for Postgres
	PostgresDefaultDbName = "postgres"
	// PostgresConfigHashKey is the key of the hash value of the OLS's Postgres config
	PostgresConfigHashKey = "hash/olspostgresconfig"
	// PostgresSecretHashKey is the key of the hash value of OLS Postgres secret
	// #nosec G101
	PostgresSecretHashKey = "hash/postgres-secret"
	// PostgresConfigMapResourceVersionAnnotation is the annotation key for tracking ConfigMap ResourceVersion
	PostgresConfigMapResourceVersionAnnotation = "ols.openshift.io/postgres-configmap-version"
	// PostgresSecretResourceVersionAnnotation is the annotation key for tracking Secret ResourceVersion
	//nolint:gosec // G101: This is an annotation key name, not a credential
	PostgresSecretResourceVersionAnnotation = "ols.openshift.io/postgres-secret-version"
	// PostgresServiceName is the name of OLS application Postgres server service
	PostgresServiceName = "lightspeed-postgres-server"
	// OtelCollectorServiceName is the in-cluster OTEL Collector Service for OTLP from OLS.
	OtelCollectorServiceName = "lightspeed-otel-collector"
	// OtelCollectorGRPCPort is the OTLP gRPC port exposed by the OTEL Collector Service.
	OtelCollectorGRPCPort = 4317
	// OtelCollectorHTTPPort is the OTLP HTTP port exposed by the OTEL Collector.
	OtelCollectorHTTPPort = 4318
	// OtelCollectorHealthCheckPort is the health check extension port.
	OtelCollectorHealthCheckPort = 13133
	// OtelCollectorAdminPort is the postgres_admin HTTPS port.
	OtelCollectorAdminPort = 8080
	// OtelCollectorMetricsPort is the cluster-facing HTTPS Prometheus metrics port
	// (https_metrics extension; OLS-3656).
	OtelCollectorMetricsPort = 8888
	// OtelCollectorMetricsInternalPort is the localhost-only HTTP Prometheus pull port
	// used as upstream by the https_metrics extension.
	OtelCollectorMetricsInternalPort = 18888
	// OtelCollectorMetricsPath is the Prometheus metrics scrape path.
	OtelCollectorMetricsPath = "/metrics"
	// OtelCollectorMetricsUpstreamURL is the https_metrics extension upstream
	// (stock telemetry pull on localhost).
	OtelCollectorMetricsUpstreamURL = "http://127.0.0.1:18888/metrics"
	// OtelCollectorHTTPSMetricsExtension is the collector extension type name for HTTPS metrics.
	OtelCollectorHTTPSMetricsExtension = "https_metrics"
	// OtelCollectorServiceMonitorName is the ServiceMonitor for collector metrics scraping.
	OtelCollectorServiceMonitorName = "lightspeed-otel-collector-monitor"
	// OtelCollectorDeploymentName is the name of the OTEL Collector deployment.
	OtelCollectorDeploymentName = "lightspeed-otel-collector"
	// OtelCollectorConfigMapName is the ConfigMap holding collector runtime YAML.
	OtelCollectorConfigMapName = "lightspeed-otel-collector-config"
	// OtelCollectorConfigMapDataKey is the key within OtelCollectorConfigMapName for collector YAML.
	OtelCollectorConfigMapDataKey = "config.yaml"
	// OtelCollectorClientConfigMapName is the ConfigMap publishing Collector connectivity for clients
	// (agentic-operator OTLP export and admin API). Distinct from the collector runtime ConfigMap.
	OtelCollectorClientConfigMapName = "lightspeed-otel-collector-client"
	// OtelCollectorClientCollectorEndpointKey is the OTLP gRPC endpoint (host:port).
	OtelCollectorClientCollectorEndpointKey = "collector-endpoint"
	// OtelCollectorClientAdminEndpointKey is the HTTPS admin API base URL.
	OtelCollectorClientAdminEndpointKey = "admin-endpoint"
	// OtelCollectorClientCACertKey is the PEM CA used to verify Collector TLS.
	OtelCollectorClientCACertKey = "ca.crt"
	// OtelCollectorClientCredentialsSecretKey is an optional Secret name for mTLS client credentials.
	OtelCollectorClientCredentialsSecretKey = "credentials-secret"
	// OtelCollectorConfigMapResourceVersionAnnotation tracks collector ConfigMap changes for rollout.
	OtelCollectorConfigMapResourceVersionAnnotation = "ols.openshift.io/otel-collector-configmap-version"
	// OtelCollectorConfigVolumeName is the pod volume name for the mounted collector config.
	OtelCollectorConfigVolumeName = "config"
	// OtelCollectorConfigVolumeMountPath is where the collector reads config.yaml.
	OtelCollectorConfigVolumeMountPath = "/etc/otelcol"
	// OtelCollectorCertsSecretName is the service-ca TLS secret for OTLP, admin API, and metrics.
	OtelCollectorCertsSecretName = "lightspeed-otel-collector-cert" // #nosec G101
	// OtelCollectorServingCertVolumeName is the pod volume name for the serving cert Secret.
	OtelCollectorServingCertVolumeName = "serving-cert"
	// OtelCollectorServingCertMountPath is where the collector expects service-ca TLS material.
	OtelCollectorServingCertMountPath = "/var/run/secrets/serving-cert"
	// OtelCollectorPostgresConnectionStringEnvVar is the env var for the collector Postgres DSN.
	OtelCollectorPostgresConnectionStringEnvVar = "POSTGRES_CONNECTION_STRING"
	// OtelCollectorPostgresDSNSecretName stores the collector Postgres DSN.
	OtelCollectorPostgresDSNSecretName = "lightspeed-otel-collector-postgres" // #nosec G101
	// OtelCollectorPostgresConnectionStringSecretKey is the key for the collector Postgres DSN.
	OtelCollectorPostgresConnectionStringSecretKey = "connection-string"
	// OtelCollectorTracesBackendEndpointEnvVar is the env var for external trace export.
	OtelCollectorTracesBackendEndpointEnvVar = "TRACES_BACKEND_ENDPOINT"
	// OtelCollectorNetworkPolicyName is the network policy for the OTEL Collector.
	OtelCollectorNetworkPolicyName = "lightspeed-otel-collector"
	// OtelCollectorServiceAccountName is the service account for the OTEL Collector pod.
	OtelCollectorServiceAccountName = "lightspeed-otel-collector"
	// OtelCollectorContainerName is the collector container name.
	OtelCollectorContainerName = "collector"
	// OtelCollectorComponentLabel is the app.kubernetes.io/component label value.
	OtelCollectorComponentLabel = "otel-collector"
	// OtelCollectorFileStorageMountPath is the file_storage extension directory.
	OtelCollectorFileStorageMountPath = "/var/lib/otelcol/file_storage"
	// OtelSandboxServiceName is the OTLP service.name for agentic sandbox audit logs routed to Postgres.
	OtelSandboxServiceName = "lightspeed-agentic-sandbox"
	// OtelCollectorServingCertTLSFile is the serving certificate path in collector YAML.
	OtelCollectorServingCertTLSFile = "/var/run/secrets/serving-cert/tls.crt"
	// OtelCollectorServingCertTLSKeyFile is the serving certificate key path in collector YAML.
	OtelCollectorServingCertTLSKeyFile = "/var/run/secrets/serving-cert/tls.key"
	// AppOtelCollectorCACertDir is the app-server mount directory for the collector serving cert.
	AppOtelCollectorCACertDir = "otel-collector-ca"
	// AppOtelCollectorCACertVolumeName is the app-server volume name for the collector TLS secret.
	AppOtelCollectorCACertVolumeName = "otel-collector-cert"
	// AppOtelCollectorCACertFile is the serving cert filename within AppOtelCollectorCACertDir.
	AppOtelCollectorCACertFile = "tls.crt"
	// PostgresSecretName is the name of OLS application Postgres secret
	PostgresSecretName = "lightspeed-postgres-secret"
	// PostgresCertsSecretName is the name of the Postgres certs secret
	PostgresCertsSecretName = "lightspeed-postgres-certs"
	// PostgresBootstrapSecretName is the name of the Postgres bootstrap secret
	// #nosec G101
	PostgresBootstrapSecretName = "lightspeed-postgres-bootstrap"
	// PostgresBootstrapVolumeMountPath is the path of bootstrap volume mount
	PostgresBootstrapVolumeMountPath = "/usr/share/container-scripts/postgresql/start/create-extensions.sh"
	// PostgresExtensionScript is the name of the Postgres extensions script
	PostgresExtensionScript = "create-extensions.sh"
	// PostgresConfigMap is the name of the Postgres config map
	PostgresConfigMap = "lightspeed-postgres-conf"
	// PostgresConfigVolumeMountPath is the path of Postgres configuration volume mount
	PostgresConfigVolumeMountPath = "/usr/share/pgsql/postgresql.conf.sample"
	// PostgresConfig is the name of Postgres configuration used to start the server
	PostgresConfig = "postgresql.conf.sample"
	// PostgresDataVolume is the name of Postgres data volume
	PostgresDataVolume = "postgres-data"
	// PostgresDataVolumeMountPath is the path of Postgres data volume mount
	PostgresDataVolumeMountPath = "/var/lib/pgsql"
	// PostgreVarRunVolumeName is the data volume name for the /var/run/postgresql writable mount
	PostgresVarRunVolumeName = "lightspeed-postgres-var-run"
	// PostgresVarRunVolumeMountPath is the path of Postgres data volume mount
	PostgresVarRunVolumeMountPath = "/var/run/postgresql"
	// PostgresServicePort is the port number of the OLS Postgres server service
	PostgresServicePort = 5432
	// PostgresSharedBuffers is the share buffers value for Postgres cache
	PostgresSharedBuffers = "256MB"
	// PostgresMaxConnections is the max connections values for Postgres cache
	PostgresMaxConnections = 2000
	// PostgresDefaultSSLMode is the default ssl mode for postgres
	PostgresDefaultSSLMode = "require"
	// PostgresBootStrapScriptContent is the postgres's bootstrap script content
	PostgresBootStrapScriptContent = `
#!/bin/bash

cat /var/lib/pgsql/data/userdata/postgresql.conf

echo "attempting to create extensions and schemas if they do not exist"

_psql () { psql --set ON_ERROR_STOP=1 "$@" ; }

# Create pg_trgm extension in default database (for OLS conversation cache)
echo "CREATE EXTENSION IF NOT EXISTS pg_trgm;" | _psql -d $POSTGRESQL_DATABASE

# quota schema: token quota tracking and limits
echo "CREATE SCHEMA IF NOT EXISTS quota;" | _psql -d $POSTGRESQL_DATABASE

# conversation_cache schema: conversation history storage
echo "CREATE SCHEMA IF NOT EXISTS conversation_cache;" | _psql -d $POSTGRESQL_DATABASE
`
	// PostgresConfigMapContent is the postgres's config content
	PostgresConfigMapContent = `
huge_pages = off
ssl = on
ssl_cert_file = '/etc/certs/tls.crt'
ssl_key_file = '/etc/certs/tls.key'
ssl_ca_file = '/etc/certs/cm-olspostgresca/service-ca.crt'
`
	// PostgresNetworkPolicyName is the name of the network policy for the OLS postgres server
	PostgresNetworkPolicyName = "lightspeed-postgres-server"

	/*** Alerts Adapter Constants ***/
	// AlertsAdapterDeploymentName is the name of the agentic alerts adapter deployment
	AlertsAdapterDeploymentName = "lightspeed-agentic-alerts-adapter"
	// AlertsAdapterServiceAccountName is the name of the alerts adapter service account
	AlertsAdapterServiceAccountName = "lightspeed-agentic-alerts-adapter"
	// AlertsAdapterContainerName is the name of the alerts adapter container
	AlertsAdapterContainerName = "adapter"
	// AlertsAdapterNetworkPolicyName is the name of the network policy for the alerts adapter
	AlertsAdapterNetworkPolicyName = "lightspeed-agentic-alerts-adapter"
	// AlertsAdapterAgenticRunsClusterRoleName is the cluster role granting AgenticRun create/list/get
	AlertsAdapterAgenticRunsClusterRoleName = "lightspeed-agentic-alerts-adapter-agenticruns"
	// AlertsAdapterAgenticRunsClusterRoleBindingName binds the AgenticRun ClusterRole to the alerts adapter SA
	AlertsAdapterAgenticRunsClusterRoleBindingName = "lightspeed-agentic-alerts-adapter-agenticruns"
	// AlertsAdapterLegacyProposalsClusterRoleName is the pre-OLS-3475 ClusterRole name removed on reconcile
	AlertsAdapterLegacyProposalsClusterRoleName = "lightspeed-agentic-alerts-adapter-proposals"
	// AlertsAdapterAlertmanagerRoleBindingName is the RoleBinding in openshift-monitoring for Alertmanager read access
	AlertsAdapterAlertmanagerRoleBindingName = "lightspeed-agentic-alerts-adapter-alertmanager"
	// AlertsAdapterConfigMapName is the ConfigMap holding runtime adapter settings (poll interval, cooldown, tools)
	AlertsAdapterConfigMapName = "alerts-adapter-config"
	// AlertsAdapterConfigMapDataKey is the key within AlertsAdapterConfigMapName for adapter YAML config
	AlertsAdapterConfigMapDataKey = "config.yaml"
	// AlertsAdapterConfigVolumeName is the pod volume name for the mounted runtime config ConfigMap
	AlertsAdapterConfigVolumeName = "config"
	// AlertsAdapterConfigVolumeMountPath is where the adapter reads mounted config.yaml
	AlertsAdapterConfigVolumeMountPath = "/etc/alerts-adapter"
	// AlertsAdapterConfigRoleName is the legacy Role name removed after config moved to a volume mount
	AlertsAdapterConfigRoleName = "lightspeed-agentic-alerts-adapter-config"
	// AlertsAdapterConfigRoleBindingName is the legacy RoleBinding name removed after config moved to a volume mount
	AlertsAdapterConfigRoleBindingName = "lightspeed-agentic-alerts-adapter-config"
	// MonitoringAlertmanagerViewRoleName is the OpenShift monitoring Role for read-only Alertmanager API access
	MonitoringAlertmanagerViewRoleName = "monitoring-alertmanager-view"
	// OpenShiftMonitoringNamespace is the namespace for platform monitoring RBAC and services
	OpenShiftMonitoringNamespace = "openshift-monitoring"
	// AlertsAdapterAlertmanagerURL is the in-cluster Alertmanager API base URL
	AlertsAdapterAlertmanagerURL = "https://alertmanager-main.openshift-monitoring.svc:9094"
	// AlertsAdapterAlertmanagerURLEnvVar is the deployment env var for the Alertmanager URL
	AlertsAdapterAlertmanagerURLEnvVar = "ALERTMANAGER_URL"
	// AlertsAdapterComponentLabel is the app.kubernetes.io/component label value for alerts adapter resources
	AlertsAdapterComponentLabel = "alerts-adapter"

	// PostgresPVCName is the name of the PVC for the OLS Postgres server
	PostgresPVCName = "lightspeed-postgres-pvc"

	// PostgresDefaultPVCSize is the default size of the PVC for the OLS Postgres server
	PostgresDefaultPVCSize = "1Gi"

	// PostgreServiceAccountName is the name of the service account for the OLS Postgres server
	PostgreServiceAccountName = "lightspeed-postgres-server"

	// TmpVolume is the data volume name for the /tmp writable mount
	TmpVolumeName = "tmp-writable-volume"
	// TmpVolumeMountPath is the path of the /tmp writable mount
	TmpVolumeMountPath = "/tmp"

	// TelemetryPullSecretNamespace "openshift-config" contains the telemetry pull secret to determine the enablement of telemetry
	// #nosec G101
	TelemetryPullSecretNamespace = "openshift-config"
	// TelemetryPullSecretName is the name of the secret containing the telemetry pull secret
	TelemetryPullSecretName = "pull-secret"

	/*** operator resources ***/
	// OperatorServiceMonitorName is the name of the service monitor for scraping the operator metrics
	OperatorServiceMonitorName = "controller-manager-metrics-monitor"
	// OperatorDeploymentName is the name of the operator deployment
	OperatorDeploymentName = "lightspeed-operator-controller-manager"
	OLSDefaultCacheType    = "postgres"
	// OperatorNetworkPolicyName is the name of the network policy for the operator
	OperatorNetworkPolicyName = "lightspeed-operator"
	// OperatorMetricsPort is the port number of the operator metrics endpoint
	OperatorMetricsPort = 8443
	// MetricsReaderServiceAccountTokenSecretName is the name of the secret containing the service account token for the metrics reader
	MetricsReaderServiceAccountTokenSecretName = "metrics-reader-token" // #nosec G101
	// MetricsReaderServiceAccountName is the name of the service account for the metrics reader
	MetricsReaderServiceAccountName = "lightspeed-operator-metrics-reader"
	// MetricsReaderClusterRoleName is the name of the ClusterRole granting metrics read access
	MetricsReaderClusterRoleName = "lightspeed-operator-ols-metrics-reader"
	// MetricsReaderClusterRoleBindingName is the name of the ClusterRoleBinding for the metrics reader
	MetricsReaderClusterRoleBindingName = "lightspeed-operator-ols-metrics-reader"
	// MCP server URL
	OpenShiftMCPServerURL = "http://localhost:%d/mcp"
	// MCP server port.
	OpenShiftMCPServerPort = 8080
	// RHOOKPHTTPPort is the Solr HTTP proxy port for the RH OKP sidecar (remapped from image default 8080).
	RHOOKPHTTPPort = 9080
	// RHOOKPHTTPSPort is the RH OKP Apache HTTPS port (remapped from image default 8443; OLS uses 8443).
	RHOOKPHTTPSPort = 9443
	// OCPClusterVersionEnvVar is read by lightspeed-service to resolve Solr chunk_filter_query at startup.
	OCPClusterVersionEnvVar = "OCP_CLUSTER_VERSION"
	// OLSRosaProductEnvVar is read by lightspeed-service to scope OKP retrieval on ROSA clusters.
	OLSRosaProductEnvVar = "OLS_ROSA_PRODUCT"
	// RosaOKPProductHCP is the OKP product identifier for ROSA hosted control planes.
	RosaOKPProductHCP = "red_hat_openshift_service_on_aws"
	// RosaOKPProductClassic is the OKP product identifier for ROSA classic architecture.
	RosaOKPProductClassic = "red_hat_openshift_service_on_aws_classic_architecture"
	// RHOOKPSolrCollection is the Solr core served by RHOKP (matches lightspeed-service portal-rag client).
	RHOOKPSolrCollection = "portal-rag"
	// RHOOKPReadinessHTTPPath hits the portal-rag core admin ping (Apache / alone is not Solr-ready).
	RHOOKPReadinessHTTPPath = "/solr/" + RHOOKPSolrCollection + "/admin/ping"
	// RHOKP startup probe: Solr can take several minutes on cold start (portal-rag core load).
	RHOOKPStartupProbeInitialDelaySeconds = 20
	RHOOKPStartupProbePeriodSeconds       = 10
	RHOOKPStartupProbeFailureThreshold    = 34 // 20s delay + 34*10s = 6 min before startup fails
	RHOOKPProbePeriodSeconds              = 10
	RHOOKPProbeTimeoutSeconds             = 5
	RHOOKPProbeFailureThreshold           = 3
	RHOOKPAccessKeySecretName             = "rhokp-access-key" // #nosec G101 -- user-created secret for RHOKP portal access
	RHOOKPAccessKeySecretKey              = "ACCESS_KEY"
	// RHOKP image Apache config paths (Listen directives remapped before mel start).
	RHOOKPHTTPDConfPath       = "/etc/httpd/conf/httpd.conf"
	RHOOKPHTTPDSSLConfPath    = "/etc/httpd/conf.d/ssl.conf"
	RHOOKPImageHTTPPort       = 8080
	RHOOKPImageHTTPSPort      = 8443
	RHOOKPContainerEntrypoint = "/usr/bin/container-entrypoint"
	RHOOKPMainCommand         = "/usr/local/bin/mel"
	// Solr hybrid defaults written to the OLS config file (not exposed on OLSConfig CR).
	SolrHybridMaxResultsDefault         = 5
	SolrHybridVectorBoostDefault        = 8.0
	SolrHybridPoolDocsDefault           = 100
	SolrHybridScoreThresholdDefault     = 0.0
	SolrHybridSolrTimeoutSecondsDefault = 60.0
	// MCP server timeout, sec
	OpenShiftMCPServerTimeout = 60
	// MCP server SSE read timeout, sec
	OpenShiftMCPServerHTTPReadTimeout = 30
	// Tools approval timeout default, sec (must match +kubebuilder:default in ToolsApprovalConfig)
	ToolsApprovalDefaultTimeout = 600
	// Authorization header for OpenShift MCP server
	K8S_AUTH_HEADER = "Authorization"
	// Constant, defining usage of kubernetes token
	KUBERNETES_PLACEHOLDER = "kubernetes"
	// Constant, defining usage of client token passthrough
	CLIENT_PLACEHOLDER = "client"
	// MCPHeadersMountRoot is the directory hosting MCP headers in the container
	MCPHeadersMountRoot = "/etc/mcp/headers"
	// OpenShiftMCPServerConfigCmName is the name of the ConfigMap for openshift-mcp-server configuration
	OpenShiftMCPServerConfigCmName = "openshift-mcp-server-config"
	// OpenShiftMCPServerConfigFilename is the filename for the openshift-mcp-server TOML config
	OpenShiftMCPServerConfigFilename = "config.toml"
	// OpenShiftMCPServerConfigMountPath is the directory where the MCP server config is mounted
	OpenShiftMCPServerConfigMountPath = "/etc/mcp-server"
	// OpenShiftMCPServerConfigVolumeName is the volume name for the MCP server config
	OpenShiftMCPServerConfigVolumeName = "mcp-server-config"
	// Header Secret Data Path
	MCPSECRETDATAPATH = "header"
	/*** Data Exporter Constants ***/
	// ExporterConfigCmName is the name of the exporter configmap
	ExporterConfigCmName = "lightspeed-exporter-config"
	// ExporterConfigVolumeName is the name of the volume for exporter configuration
	ExporterConfigVolumeName = "exporter-config"
	// ExporterConfigMountPath is the path where exporter config is mounted
	ExporterConfigMountPath = "/etc/config"
	// ExporterConfigFilename is the name of the exporter configuration file
	ExporterConfigFilename = "config.yaml"
	// OLSUserDataMountPath is the path where user data is mounted in the app server container
	OLSUserDataMountPath = "/app-root/ols-user-data"
	// ServiceIDOLS is the service ID used by the data exporter
	ServiceIDOLS = "ols"
	// RHOSOLightspeedOwnerIDLabel is the label used to identify RHOSO Lightspeed deployment
	RHOSOLightspeedOwnerIDLabel = "openstack.org/lightspeed-owner-id"
	// ServiceIDRHOSO is the service ID used by the data exporter when RHOSO Lightspeed is deployed
	ServiceIDRHOSO = "rhos-lightspeed"

	/*** Container Names (used for testing) ***/
	// OLSAppServerContainerName is the name of the OLS application server container
	OLSAppServerContainerName = "lightspeed-service-api"
	// DataverseExporterContainerName is the name of the dataverse exporter container
	DataverseExporterContainerName = "lightspeed-to-dataverse-exporter"
	// ConsoleUIContainerName is the name of the console UI container
	ConsoleUIContainerName = "lightspeed-console-plugin"
	// PostgresContainerName is the name of the postgres container
	PostgresContainerName = "lightspeed-postgres-server"
	// OpenShiftMCPServerContainerName is the name of the OpenShift MCP server container
	OpenShiftMCPServerContainerName = "openshift-mcp-server"
	// RHOOKPContainerName is the RH Offline Knowledge Portal (Solr) sidecar container name.
	RHOOKPContainerName = "rhokp"
	// OLSConfigMapResourceVersionAnnotation is the annotation key for tracking OLS ConfigMap ResourceVersion
	OLSConfigMapResourceVersionAnnotation = "ols.openshift.io/olsconfig-configmap-version"
	// OpenShiftMCPServerConfigMapResourceVersionAnnotation is the annotation key for tracking MCP Server ConfigMap ResourceVersion
	OpenShiftMCPServerConfigMapResourceVersionAnnotation = "ols.openshift.io/mcp-server-configmap-version"

	/*** Environment Variable Suffixes ***/
	// EnvVarSuffixAPIKey is the environment variable suffix for API key credentials
	EnvVarSuffixAPIKey = "_API_KEY"
	// EnvVarSuffixClientID is the environment variable suffix for client ID credentials
	EnvVarSuffixClientID = "_CLIENT_ID"
	// EnvVarSuffixTenantID is the environment variable suffix for tenant ID credentials
	EnvVarSuffixTenantID = "_TENANT_ID"
	// EnvVarSuffixClientSecret is the environment variable suffix for client secret credentials
	EnvVarSuffixClientSecret = "_CLIENT_SECRET"
)

// Default images from related_images.json (see internal/relatedimages). Used for flags and tests.
var (
	OLSAppServerImageDefault       = relatedimages.GetDefaultImage("lightspeed-service-api")
	ConsoleUIImageDefault          = relatedimages.GetDefaultImage("lightspeed-console-plugin")
	PostgresServerImageDefault     = relatedimages.GetDefaultImage("lightspeed-postgresql")
	OpenShiftMCPServerImageDefault = relatedimages.GetDefaultImage("openshift-mcp-server")
	DataverseExporterImageDefault  = relatedimages.GetDefaultImage("lightspeed-to-dataverse-exporter")
	AgenticConsoleUIImageDefault   = imageDefaultOr("lightspeed-agentic-console-plugin", agenticConsoleUIImageFallback)
	AlertsAdapterImageDefault      = imageDefaultOr("lightspeed-agentic-alerts-adapter", alertsAdapterImageFallback)
	OtelCollectorImageDefault      = imageDefaultOr("lightspeed-otel-collector", otelCollectorImageFallback)
	RHOOKPImageDefault             = imageDefaultOr("rhokp", rhokpImageFallback)
)

const (
	// Fallbacks when related_images.json is unavailable (e.g. local run outside repo root).
	agenticConsoleUIImageFallback = "quay.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/lightspeed-agentic-console:main"
	alertsAdapterImageFallback    = "quay.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/lightspeed-agentic-alerts-adapter:main"
	otelCollectorImageFallback    = "quay.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/lightspeed-otel-collector:main"
	rhokpImageFallback            = "registry.redhat.io/offline-knowledge-portal/rhokp-rhel9:latest"
)

func imageDefaultOr(name, fallback string) string {
	if img := relatedimages.GetDefaultImage(name); img != "" {
		return img
	}
	return fallback
}
