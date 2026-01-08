package utils

import "time"

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
	// Image of the OLS application server (default for testing only)
	// TODO: Tests should use a pinned version instead of :latest for reproducibility.
	// Production image is configured via environment variables in cmd/main.go.
	OLSAppServerImageDefault = "quay.io/openshift-lightspeed/lightspeed-service-api:latest"
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
	// AzureOpenAIType is the name of the Azure OpenAI provider type
	AzureOpenAIType = "azure_openai"
	// DeploymentInProgress message
	DeploymentInProgress = "In Progress"
	// OLSSystemPromptFileName is the filename for the system prompt
	OLSSystemPromptFileName = "system_prompt"

	/*** console UI plugin ***/
	// ConsoleUIConfigMapName is the name of the console UI nginx configmap
	ConsoleUIConfigMapName = "lightspeed-console-plugin"
	// ConsoleUIServiceCertSecretName is the name of the console UI service certificate secret
	ConsoleUIServiceCertSecretName = "lightspeed-console-plugin-cert"
	// ConsoleUIServiceName is the name of the console UI service
	ConsoleUIServiceName = "lightspeed-console-plugin"
	// ConsoleUIDeploymentName is the name of the console UI deployment
	ConsoleUIDeploymentName = "lightspeed-console-plugin"
	// ConsoleUIImage is the image of the console UI plugin
	ConsoleUIImageDefault = "quay.io/openshift-lightspeed/lightspeed-console-plugin:latest"
	// ConsoleUIImage is the image of the console UI plugin
	ConsoleUIImagePF5Default = "quay.io/openshift-lightspeed/lightspeed-console-plugin-pf5:latest"
	// ConsoleUIHTTPSPort is the port number of the console UI service
	ConsoleUIHTTPSPort = 9443
	// ConsoleUIPluginName is the name of the console UI plugin
	ConsoleUIPluginName = "lightspeed-console-plugin"
	// ConsoleUIPluginDisplayName is the display name of the console UI plugin
	ConsoleUIPluginDisplayName = "Lightspeed Console"
	// ConsoleCRName is the name of the console custom resource
	ConsoleCRName = "cluster"
	// ConsoleProxyAlias is the alias of the console proxy
	// The console backend exposes following proxy endpoint: /api/proxy/plugin/<plugin-name>/<proxy-alias>/<request-path>?<optional-query-parameters>
	ConsoleProxyAlias = "ols"
	// ConsoleUINetworkPolicyName is the name of the network policy for the console UI plugin
	ConsoleUINetworkPolicyName = "lightspeed-console-plugin"

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
	// PostgresSecretKeyName is the name of the key holding Postgres server secret
	PostgresSecretKeyName = "password"
	// Image of the OLS application postgres server
	PostgresServerImageDefault = "registry.redhat.io/rhel9/postgresql-16@sha256:42f385ac3c9b8913426da7c57e70bc6617cd237aaf697c667f6385a8c0b0118b"
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
	// NOTE: Database name must match LlamaStackDatabaseName constant (hardcoded by llama-stack)
	PostgresBootStrapScriptContent = `
#!/bin/bash

cat /var/lib/pgsql/data/userdata/postgresql.conf

echo "attempting to create llama-stack database and pg_trgm extension if they do not exist"

_psql () { psql --set ON_ERROR_STOP=1 "$@" ; }

# Create database for llama-stack conversation storage
# Database name is hardcoded by llama-stack internally (value from LlamaStackDatabaseName: ` + LlamaStackDatabaseName + `)
DB_NAME="` + LlamaStackDatabaseName + `"

echo "SELECT 'CREATE DATABASE $DB_NAME' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '$DB_NAME')\gexec" | _psql -d $POSTGRESQL_DATABASE

# Create pg_trgm extension in default database (for OLS conversation cache)
echo "CREATE EXTENSION IF NOT EXISTS pg_trgm;" | _psql -d $POSTGRESQL_DATABASE

# Create pg_trgm extension in llama-stack database (for text search if needed)
echo "CREATE EXTENSION IF NOT EXISTS pg_trgm;" | _psql -d $DB_NAME

# Create schemas for isolating different components' data
# lcore schema: main lightspeed-stack data (general database operations)
echo "CREATE SCHEMA IF NOT EXISTS lcore;" | _psql -d $POSTGRESQL_DATABASE

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

	// PostgresPVCName is the name of the PVC for the OLS Postgres server
	PostgresPVCName = "lightspeed-postgres-pvc"

	// PostgresDefaultPVCSize is the default size of the PVC for the OLS Postgres server
	PostgresDefaultPVCSize = "1Gi"

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
	// MCP server image
	OpenShiftMCPServerImageDefault = "quay.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/openshift-mcp-server@sha256:3a035744b772104c6c592acf8a813daced19362667ed6dab73a00d17eb9c3a43"
	// MCP server URL
	OpenShiftMCPServerURL = "http://localhost:%d/mcp"
	// MCP server port
	OpenShiftMCPServerPort = 8080
	// MCP server timeout, sec
	OpenShiftMCPServerTimeout = 60
	// MCP server SSE read timeout, sec
	OpenShiftMCPServerHTTPReadTimeout = 30
	// Authorization header for OpenShift MCP server
	K8S_AUTH_HEADER = "Authorization"
	// Constant, defining usage of kubernetes token
	KUBERNETES_PLACEHOLDER = "kubernetes"
	// MCPHeadersMountRoot is the directory hosting MCP headers in the container
	MCPHeadersMountRoot = "/etc/mcp/headers"
	// Header Secret Data Path
	MCPSECRETDATAPATH = "header"
	// OCP RAG image
	OcpRagImageDefault = "quay.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/lightspeed-rag-content-lsc@sha256:edf031376f6ad3a06d3ad1b2e3b06ed6139a03f5c32f01ffee012240e9169639"

	/*** Data Exporter Constants ***/
	// Dataverse exporter image
	DataverseExporterImageDefault = "quay.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/lightspeed-to-dataverse-exporter@sha256:ccb6705a5e7ff0c4d371dc72dc8cf319574a2d64bcc0a89ccc7130f626656722"
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

	/*** LCore specific Settings ***/
	// LlamaStackConfigCmName name for the Llama stack config map
	LlamaStackConfigCmName = "llama-stack-config"
	// LCoreConfigCmName name for the LCore config map
	LCoreConfigCmName = "lightspeed-stack-config"
	// LlamaStackImageDefault default image for Llama Stack
	LlamaStackImageDefault = "quay.io/lightspeed-core/lightspeed-stack:dev-latest"
	// LlamaStackConfigHashKey is the key of the hash value of the Llama Stack configmap
	LlamaStackConfigHashKey = "hash/llamastackconfig"
	// LCoreDeploymentName is the name of the LCore deployment (used for testing)
	LCoreDeploymentName = "lightspeed-stack-deployment"
	// LCoreAppLabel is the app label for LCore resources (used for testing)
	LCoreAppLabel = "lightspeed-stack"
	// LlamaStackContainerName is the name of the Llama Stack container (used for testing)
	LlamaStackContainerName = "llama-stack"
	// LCoreContainerName is the name of the LCore container (used for testing)
	LCoreContainerName = "lightspeed-stack"
	// LlamaStackContainerPort is the port for the Llama Stack container (used for testing)
	LlamaStackContainerPort = 8321
	// LlamaCacheVolumeName is the name of the Llama cache volume (used for testing)
	LlamaCacheVolumeName = "llama-cache"
	// LlamaStackConfigFilename is the filename for Llama Stack config (used for testing)
	LlamaStackConfigFilename = "run.yaml"
	// LCoreConfigFilename is the filename for LCore config (used for testing)
	LCoreConfigFilename = "lightspeed-stack.yaml"
	// KubeRootCAMountPath is the mount path for kube-root-ca.crt (used for testing)
	KubeRootCAMountPath = "/etc/pki/ca-trust/extracted/pem"
	// AdditionalCAMountPath is the mount path for additional CA certificates (used for testing)
	AdditionalCAMountPath = "/etc/pki/ca-trust/source/anchors"
	// LlamaStackHealthPath is the health check path for Llama Stack (used for testing)
	LlamaStackHealthPath = "/v1/health"
	// OLSConfigMapResourceVersionAnnotation is the annotation key for tracking OLS ConfigMap ResourceVersion
	OLSConfigMapResourceVersionAnnotation = "ols.openshift.io/olsconfig-configmap-version"
	// LlamaStackConfigMapResourceVersionAnnotation is the annotation key for tracking Llama Stack ConfigMap ResourceVersion
	LlamaStackConfigMapResourceVersionAnnotation = "ols.openshift.io/llamastack-configmap-version"
	// LCoreConfigMapResourceVersionAnnotation is the annotation key for tracking LCore ConfigMap ResourceVersion
	LCoreConfigMapResourceVersionAnnotation = "ols.openshift.io/lcore-configmap-version"
	// LlamaStackDatabaseName is the PostgreSQL database name for llama-stack conversation storage.
	// CRITICAL: This value is HARDCODED in llama-stack's internal PostgreSQL adapter.
	// DO NOT CHANGE THIS VALUE UNDER ANY CIRCUMSTANCES - llama-stack expects exactly "llamastack".
	// Changing this will break llama-stack's database connectivity.
	// This database is created in PostgresBootStrapScriptContent.
	LlamaStackDatabaseName = "llamastack"
)
