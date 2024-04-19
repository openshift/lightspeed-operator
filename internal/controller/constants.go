package controller

const (
	/*** Operator Settings ***/
	// OLSConfigName is the name of the OLSConfig Custom Resource
	OLSConfigName = "cluster"

	/*** application server configuration file ***/
	// OLSConfigName is the name of the OLSConfig configmap
	OLSConfigCmName = "olsconfig"
	// OLSCAConfigMap is the name of the OLS TLS ca certificate configmap
	OLSCAConfigMap = "openshift-service-ca.crt"
	// PostgresCAVolume is the name of the OLS postgres TLS ca certificate volume name
	PostgresCAVolume = "cm-olspostgresca"
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
	// PostgresDeploymentName is the name of OLS application postgres deployment
	PostgresDeploymentName = "lightspeed-postgres-server"
	// APIKeyMountRoot is the directory hosting the API key file in the container
	APIKeyMountRoot = "/etc/apikeys" // #nosec G101
	// CredentialsMountRoot is the directory hosting the credential files in the container
	CredentialsMountRoot = "/etc/credentials"
	// OLSAppCertsMountRoot is the directory hosting the cert files in the container
	OLSAppCertsMountRoot = "/etc/certs"
	// LLMApiTokenFileName is the name of the file containing the API token to access LLM in the secret referenced by the OLSConfig
	LLMApiTokenFileName = "apitoken"
	// OLSComponentPasswordFileName is the generic name of the password file for each of its components
	OLSComponentPasswordFileName = "password"
	// OLSConfigFilename is the name of the application server configuration file
	OLSConfigFilename = "olsconfig.yaml"
	// PostgresSecretKeyName is the name of the key holding postgres server secret
	PostgresSecretKeyName = "password"
	// Image of the OLS application server
	// todo: image vesion should synchronize with the release version of the lightspeed-service-api image.
	OLSAppServerImageDefault = "quay.io/openshift/lightspeed-service-api:latest"
	// Image of the OLS application postgres server
	PostgresServerImageDefault = "quay.io/openshift/lightspeed-service-postgres:latest"
	// AppServerServiceMonitorName is the name of the service monitor for the OLS application server
	AppServerServiceMonitorName = "lightspeed-app-server-monitor"
	// AppServerMetricsPath is the path of the metrics endpoint of the OLS application server
	AppServerMetricsPath = "/metrics"
	// OLSConfigHashKey is the key of the hash value of the OLSConfig configmap
	OLSConfigHashKey = "hash/olsconfig"
	// LLMProviderHashKey is the key of the hash value of OLS LLM provider credentials consolidated
	// #nosec G101
	LLMProviderHashKey = "hash/llmprovider"
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
	PostgresBootstrapSecretName = "postgres-bootstrap"
	// PostgresBootstrapVolumeMount is the name of bootstrap volume mount
	PostgresBootstrapVolumeMount = "/usr/share/container-scripts/postgresql/start/create-extensions.sh"
	// PostgresExtensionScript is the name of the postgres extensions script
	PostgresExtensionScript = "create-extensions.sh"
	// PostgresConfigMap is the name of the postgres config map
	PostgresConfigMap = "postgres-conf"
	// PostgresConfigVolumeMount is the name of postgres configuration volume mount
	PostgresConfigVolumeMount = "/usr/share/pgsql/postgresql.conf.sample"
	// PostgresConfig is the name of postgres configuration used to start the server
	PostgresConfig = "postgresql.conf.sample"
	// PostgresDataVolume is the name of postgres data volume
	PostgresDataVolume = "postgres-data"
	// PostgresDataVolumeMount is the name of postgres data volume mount
	PostgresDataVolumeMount = "/var/lib/pgsql/data"
	// OLSAppServerContainerPort is the port number of the lightspeed-service-api container exposes
	OLSAppServerContainerPort = 8443
	// OLSAppServerServicePort is the port number for OLS application server service.
	OLSAppServerServicePort = 8443
	// OLSAppServerServiceName is the name of the OLS application server service
	OLSAppServerServiceName = "lightspeed-app-server"
	// OLSCertsSecretName is the name of the TLS secret for OLS.
	OLSCertsSecretName = "lightspeed-tls" // #nosec G101
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
	
	echo "attempting to create pg_trgm extension if it does not exist"
	
	_psql () { psql --set ON_ERROR_STOP=1 "$@" ; }
	
	echo "CREATE EXTENSION IF NOT EXISTS pg_trgm;" | _psql -d $POSTGRESQL_DATABASE
	`
	// PostgresConfigMapContent is the postgres's config content
	PostgresConfigMapContent = `huge_pages = off
	ssl = on
	ssl_cert_file = '/etc/certs/tls.crt'
	ssl_key_file = '/etc/certs/tls.key'
	ssl_ca_file = '/etc/certs/cm-olspostgresca/service-ca.crt'
	logging_collector = on
	log_filename = 'postgresql-%a.log'
	log_truncate_on_rotation = on
	log_rotation_age = 1d
	log_rotation_size = 0`
	// OLSDefaultCacheType is the default cache type for OLS
	OLSDefaultCacheType = "postgres"
	// Annotation key for serving certificate secret name
	// #nosec G101
	ServingCertSecretAnnotationKey = "service.beta.openshift.io/serving-cert-secret-name"
	/*** state cache keys ***/
	OLSConfigHashStateCacheKey      = "olsconfigmap-hash"
	LLMProviderHashStateCacheKey    = "llmprovider-hash"
	PostgresConfigHashStateCacheKey = "olspostgresconfig-hash"
	// #nosec G101
	PostgresSecretHashStateCacheKey = "olspostgressecret-hash"

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

	/*** watchers ***/
	WatcherAnnotationKey = "ols.openshift.io/watcher"
)
