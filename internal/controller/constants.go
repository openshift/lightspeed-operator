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
	// AdditionalCAVolumeName is the name of the volume for additional CA certificates provided by the user
	AdditionalCAVolumeName = "additional-ca"
	// CertBundleVolumeName is the name of the volume for the certificate bundle
	CertBundleVolumeName = "cert-bundle"
	// CertBundleDir is the path of the volume for the certificate bundle
	CertBundleDir = "cert-bundle"

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
	ConsoleUIImageDefault = "quay.io/openshift/lightspeed-console-plugin:latest"
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

	/*** watchers ***/
	WatcherAnnotationKey = "ols.openshift.io/watcher"

	/*** Postgres Constants ***/
	// PostgresCAVolume is the name of the OLS postgres TLS ca certificate volume name
	PostgresCAVolume = "cm-olspostgresca"
	// PostgresDeploymentName is the name of OLS application postgres deployment
	PostgresDeploymentName = "lightspeed-postgres-server"
	// PostgresSecretKeyName is the name of the key holding postgres server secret
	PostgresSecretKeyName = "password"
	// Image of the OLS application postgres server
	PostgresServerImageDefault = "registry.redhat.io/rhel9/postgresql-16:9.5-1732622748"
	// PostgresDefaultUser is the default user name for postgres
	PostgresDefaultUser = "postgres"
	// PostgresDefaultDbName is the default db name for postgres
	PostgresDefaultDbName = "postgres"
	// PostgresConfigHashKey is the key of the hash value of the OLS's postgres config
	PostgresConfigHashKey = "hash/olspostgresconfig"
	// PostgresSecretHashKey is the key of the hash value of OLS Postgres secret
	// #nosec G101
	PostgresSecretHashKey = "hash/postgres-secret"
	// PostgresServiceName is the name of OLS application postgres server service
	PostgresServiceName = "lightspeed-postgres-server"
	// PostgresSecretName is the name of OLS application postgres secret
	PostgresSecretName = "lightspeed-postgres-secret"
	// PostgresCertsSecretName is the name of the postgres certs secret
	PostgresCertsSecretName = "lightspeed-postgres-certs"
	// PostgresBootstrapSecretName is the name of the postgres bootstrap secret
	// #nosec G101
	PostgresBootstrapSecretName = "lightspeed-postgres-bootstrap"
	// PostgresBootstrapVolumeMountPath is the path of bootstrap volume mount
	PostgresBootstrapVolumeMountPath = "/usr/share/container-scripts/postgresql/start/create-extensions.sh"
	// PostgresExtensionScript is the name of the postgres extensions script
	PostgresExtensionScript = "create-extensions.sh"
	// PostgresConfigMap is the name of the postgres config map
	PostgresConfigMap = "lightspeed-postgres-conf"
	// PostgresConfigVolumeMountPath is the path of postgres configuration volume mount
	PostgresConfigVolumeMountPath = "/usr/share/pgsql/postgresql.conf.sample"
	// PostgresConfig is the name of postgres configuration used to start the server
	PostgresConfig = "postgresql.conf.sample"
	// PostgresDataVolume is the name of postgres data volume
	PostgresDataVolume = "postgres-data"
	// PostgresDataVolumeMountPath is the path of postgres data volume mount
	PostgresDataVolumeMountPath = "/var/lib/pgsql/data"
	// PostgresServicePort is the port number of the OLS postgres server service
	PostgresServicePort = 5432
	// PostgresSharedBuffers is the share buffers value for postgres cache
	PostgresSharedBuffers = "256MB"
	// PostgresMaxConnections is the max connections values for postgres cache
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
	/*** state cache keys ***/
	// OLSAppTLSHashStateCacheKey is the key of the hash value of the OLS App TLS certificates
	OLSAppTLSHashStateCacheKey = "olsapptls-hash"
	// OLSConfigHashStateCacheKey is the key of the hash value of the OLSConfig configmap
	// TelemetryPullSecretNamespace "openshift-config" contains the telemetry pull secret to determine the enablement of telemetry
	// #nosec G101
	TelemetryPullSecretNamespace = "openshift-config"
	// TelemetryPullSecretName is the name of the secret containing the telemetry pull secret
	TelemetryPullSecretName         = "pull-secret"
	OLSDefaultCacheType             = "postgres"
	PostgresConfigHashStateCacheKey = "olspostgresconfig-hash"
	// #nosec G101
	PostgresSecretHashStateCacheKey = "olspostgressecret-hash"
)
