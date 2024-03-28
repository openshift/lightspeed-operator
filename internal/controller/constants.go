package controller

const (
	/*** Operator Settings ***/
	// OLSConfigName is the name of the OLSConfig Custom Resource
	OLSConfigName = "cluster"

	/*** application server configuration file ***/
	// OLSConfigName is the name of the OLSConfig configmap
	OLSConfigCmName = "olsconfig"
	// RedisCAConfigMap is the name of the OLS redis server TLS ca certificate configmap
	RedisCAConfigMap = "openshift-service-ca.crt"
	// RedisCAVolume is the name of the OLS redis TLS ca certificate volume name
	RedisCAVolume = "cm-olsredisca"
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
	// RedisDeploymentName is the name of OLS application redis deployment
	RedisDeploymentName = "lightspeed-redis-server"
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
	// RedisSecretKeyName is the name of the key holding redis server secret
	RedisSecretKeyName = "password"
	// Image of the OLS application server
	// todo: image vesion should synchronize with the release version of the lightspeed-service-api image.
	OLSAppServerImageDefault = "quay.io/openshift/lightspeed-service-api:latest"
	// AppServerServiceMonitorName is the name of the service monitor for the OLS application server
	AppServerServiceMonitorName = "lightspeed-app-server-monitor"
	// AppServerMetricsPath is the path of the metrics endpoint of the OLS application server
	AppServerMetricsPath = "/metrics"

	// Image of the OLS application redis server
	//RedisServerImageDefault = "quay.io/openshift/lightspeed-service-redis:latest"
	// OLSConfigHashKey is the key of the hash value of the OLSConfig configmap
	OLSConfigHashKey = "hash/olsconfig"
	// RedisConfigHashKey is the key of the hash value of the OLS's redis config
	RedisConfigHashKey = "hash/olsredisconfig"
	// RedisSecretHashKey is the key of the hash value of OLS Redis secret
	// #nosec G101
	RedisSecretHashKey = "hash/redis-secret"
	// RedisServiceName is the name of OLS application redis server service
	RedisServiceName = "lightspeed-redis-server"
	// RedisSecretName is the name of OLS application redis secret
	RedisSecretName = "lightspeed-redis-secret"
	// OLSAppRedisCertsName is the name of the OLS application redis certs secret
	RedisCertsSecretName = "lightspeed-redis-certs"
	// OLSAppServerContainerPort is the port number of the lightspeed-service-api container exposes
	OLSAppServerContainerPort = 8443
	// OLSAppServerServicePort is the port number for OLS application server service.
	OLSAppServerServicePort = 8443
	// OLSAppServerServiceName is the name of the OLS application server service
	OLSAppServerServiceName = "lightspeed-app-server"
	// OLSCertsSecretName is the name of the TLS secret for OLS.
	OLSCertsSecretName = "lightspeed-tls" // #nosec G101
	// RedisServicePort is the port number of the OLS redis server service
	RedisServicePort = 6379
	// RedisMaxMemory is the max memory of the OLS redis cache
	RedisMaxMemory = "1024mb"
	// RedisMaxMemoryPolicy is the max memory policy of the OLS redis cache
	RedisMaxMemoryPolicy = "allkeys-lru"
	// OLSDefaultCacheType is the default cache type for OLS
	OLSDefaultCacheType = "redis"
	// Annotation key for serving certificate secret name
	// #nosec G101
	ServingCertSecretAnnotationKey = "service.beta.openshift.io/serving-cert-secret-name"
	/*** state cache keys ***/
	OLSConfigHashStateCacheKey   = "olsconfigmap-hash"
	RedisConfigHashStateCacheKey = "olsredisconfig-hash"
	// #nosec G101
	RedisSecretHashStateCacheKey = "olsredissecret-hash"

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
)
