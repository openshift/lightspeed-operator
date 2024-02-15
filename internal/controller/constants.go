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
	// OLSNamespaceDefault is the default namespace for OLS
	OLSNamespaceDefault = "openshift-lightspeed"
	// OLSAppServerServiceAccountName is the name of service account running the application server
	OLSAppServerServiceAccountName = "lightspeed-app-server"
	// OLSAppServerDeploymentName is the name of the OLS application server deployment
	OLSAppServerDeploymentName = "lightspeed-app-server"
	// APIKeyMountRoot is the directory hosting the API key file in the container
	APIKeyMountRoot = "/etc/apikeys"
	// LLMApiTokenFileName is the name of the file containing the API token to access LLM in the secret referenced by the OLSConfig
	LLMApiTokenFileName = "apitoken"
	// OLSConfigFilename is the name of the application server configuration file
	OLSConfigFilename = "olsconfig.yaml"
	// Image of the OLS application server
	// todo: image vesion should synchronize with the release version of the lightspeed-service-api image.
	OLSAppServerImageDefault = "quay.io/openshift/lightspeed-service-api:latest"
	// OLSConfigHashKey is the key of the hash value of the OLSConfig configmap
	OLSConfigHashKey = "hash/olsconfig"
	// OLSAppServerServiceName is the name of the OLS application server service
	OLSAppServerServiceName = "lightspeed-app-server"
	// OLSAppServerContainerPort is the port number of the lightspeed-service-api container exposes
	OLSAppServerContainerPort = 8080
	// OLSAppServerServicePort is the port number of the OLS application server service
	OLSAppServerServicePort = 8080

	/*** state cache keys ***/
	OLSConfigHashStateCacheKey = "olsconfigmap-hash"
)
