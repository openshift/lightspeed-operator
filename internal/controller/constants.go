package controller

import "time"

const (
	/*** Operator Settings ***/
	// OLSConfigName is the name of the OLSConfig Custom Resource
	OLSConfigName = "cluster"
	// DefaultReconcileInterval is the default interval for reconciliation
	DefaultReconcileInterval = 120
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
	// OLSComponentPasswordFileName is the generic name of the password file for each of its components
	OLSComponentPasswordFileName = "password"
	// OLSConfigFilename is the name of the application server configuration file
	OLSConfigFilename = "olsconfig.yaml"
	// Image of the OLS application server
	// todo: image vesion should synchronize with the release version of the lightspeed-service-api image.
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
	// CertBundleDir is the path of the volume for the certificate bundle
	CertBundleDir = "cert-bundle"
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
	/*** state cache keys ***/
	OLSConfigHashStateCacheKey   = "olsconfigmap-hash"
	LLMProviderHashStateCacheKey = "llmprovider-hash"
	// AzureOpenAIType is the name of the Azure OpenAI provider type
	AzureOpenAIType = "azure_openai"
	// AdditionalCAHashStateCacheKey is the key of the hash value of the additional CA certificates in the state cache
	AdditionalCAHashStateCacheKey = "additionalca-hash"
	// DeploymentInProgress message
	DeploymentInProgress = "In Progress"

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
	// OLSConsoleTLSHashStateCacheKey is the key of the hash value of the OLS Console TLS certificates
	OLSConsoleTLSHashStateCacheKey = "olsconsoletls-hash"
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
	PostgresBootStrapScriptContent = `
#!/bin/bash

cat /var/lib/pgsql/data/userdata/postgresql.conf

echo "attempting to create pg_trgm extension if it does not exist"

_psql () { psql --set ON_ERROR_STOP=1 "$@" ; }

echo "CREATE EXTENSION IF NOT EXISTS pg_trgm;" | _psql -d $POSTGRESQL_DATABASE
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

	/*** state cache keys ***/
	// OLSAppTLSHashStateCacheKey is the key of the hash value of the OLS App TLS certificates
	OLSAppTLSHashStateCacheKey = "olsapptls-hash"
	// OLSConfigHashStateCacheKey is the key of the hash value of the OLSConfig configmap
	// TelemetryPullSecretNamespace "openshift-config" contains the telemetry pull secret to determine the enablement of telemetry
	// #nosec G101
	TelemetryPullSecretNamespace = "openshift-config"
	// TelemetryPullSecretName is the name of the secret containing the telemetry pull secret
	TelemetryPullSecretName = "pull-secret"

	/*** operator resources ***/
	// OperatorServiceMonitorName is the name of the service monitor for scraping the operator metrics
	OperatorServiceMonitorName = "controller-manager-metrics-monitor"
	// OperatorDeploymentName is the name of the operator deployment
	OperatorDeploymentName          = "lightspeed-operator-controller-manager"
	OLSDefaultCacheType             = "postgres"
	PostgresConfigHashStateCacheKey = "olspostgresconfig-hash"
	// #nosec G101
	PostgresSecretHashStateCacheKey = "olspostgressecret-hash"
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
	// Dataverse exporter image
	DataverseExporterImageDefault = "quay.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/lightspeed-to-dataverse-exporter@sha256:ccb6705a5e7ff0c4d371dc72dc8cf319574a2d64bcc0a89ccc7130f626656722"
	// MCP server URL
	OpenShiftMCPServerURL = "http://localhost:%d/mcp"
	// MCP server port
	OpenShiftMCPServerPort = 8080
	// MCP server timeout, sec
	OpenShiftMCPServerTimeout = 60
	// MCP server SSE read timeout, sec
	OpenShiftMCPServerHTTPReadTimeout = 30
)
