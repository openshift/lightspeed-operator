package controller

const (
	/*** Operator environment variables ***/
	// WatchNamespaceEnvVar is the environment variable to specify the namespace to watch
	WatchNamespaceEnvVar = "WATCH_NAMESPACE"

	/*** Operator Settings ***/
	// OLSConfigName is the name of the OLSConfig Custom Resource
	OLSConfigName = "cluster"

	/*** application server configuration file ***/
	// OLSConfigName is the name of the OLSConfig configmap
	OLSConfigCmName = "olsconfig"
	// OLSRedisCACmName is the name of the OLS redis server TLS ca certificate configmap
	OLSRedisCACmName = "openshift-service-ca.crt"
	// OLSRedisCAVolumeName is the name of the OLS redis TLS ca certificate volume name
	OLSRedisCAVolumeName = "cm-olsredisca"
	// OLSNamespaceDefault is the default namespace for OLS
	OLSNamespaceDefault = "openshift-lightspeed"
	// OLSAppServerServiceAccountName is the name of service account running the application server
	OLSAppServerServiceAccountName = "lightspeed-app-server"
	// OLSAppServerDeploymentName is the name of the OLS application server deployment
	OLSAppServerDeploymentName = "lightspeed-app-server"
	// OLSAppRedisDeploymentName is the name of OLS application redis deployment
	OLSAppRedisDeploymentName = "lightspeed-redis-server"
	// APIKeyMountRoot is the directory hosting the API key file in the container
	APIKeyMountRoot = "/etc/apikeys"
	// CredentialsMountRoot is the directory hosting the credential files in the container
	CredentialsMountRoot = "/etc/credentials"
	// OLSAppCertsMountRoot is the directory hosting the cert files in the container
	OLSAppCertsMountRoot = "/etc/certs"
	// LLMApiTokenFileName is the name of the file containing the API token to access LLM in the secret referenced by the OLSConfig
	LLMApiTokenFileName = "apitoken"
	// OLSPasswordFileName is the name of the file containing password for its infra
	OLSPasswordFileName = "password"
	// OLSConfigFilename is the name of the application server configuration file
	OLSConfigFilename = "olsconfig.yaml"
	// OLSRedisSecretKeyName is the name of the key holding redis server secret
	OLSRedisSecretKeyName = "password"
	// Image of the OLS application server
	// todo: image vesion should synchronize with the release version of the lightspeed-service-api image.
	OLSAppServerImageDefault = "quay.io/openshift/lightspeed-service:latest"
	// Image of the OLS application redis server
	OLSAppRedisServerImageDefault = "quay.io/openshift/lightspeed-service-redis:latest"
	// OLSConfigHashKey is the key of the hash value of the OLSConfig configmap
	OLSConfigHashKey = "hash/olsconfig"
	// OLSRedisSecretHashKey is the key of the hash value of OLS Redis secret
	OLSRedisSecretHashKey = "hash/redis-secret"
	// OLSAppServerServiceName is the name of the OLS application server service
	OLSAppServerServiceName = "lightspeed-app-server"
	// OLSAppRedisServiceName is the name of OLS application redis server service
	OLSAppRedisServiceName = "lightspeed-redis-server"
	// OLSAppRedisSecretName is the name of OLS application redis secret
	OLSAppRedisSecretName = "lightspeed-redis-secret"
	// OLSAppRedisCertsName is the name of the OLS application redis certs secret
	OLSAppRedisCertsSecretName = "lightspeed-redis-certs"
	// OLSAppServerContainerPort is the port number of the lightspeed-service-api container exposes
	OLSAppServerContainerPort = 8080
	// OLSAppServerServicePort is the port number of the OLS application server service
	OLSAppServerServicePort = 8080
	// OLSAppRedisServicePort is the port number of the OLS redis server service
	OLSAppRedisServicePort = 6379
	// OLSAppRedisMaxMemory is the max memory of the OLS redis cache
	OLSAppRedisMaxMemory = "1024mb"
	// OLSAppRedisMaxMemoryPolicy is the max memory policy of the OLS redis cache
	OLSAppRedisMaxMemoryPolicy = "allkeys-lru"
	// OLSDefaultCacheType is the default cache type for OLS
	OLSDefaultCacheType = "redis"

	/*** state cache keys ***/
	OLSConfigHashStateCacheKey      = "olsconfigmap-hash"
	OLSRedisSecretHashStateCacheKey = "olsredissecret-hash"
)
