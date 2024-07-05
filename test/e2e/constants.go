package e2e

const (
	// OLSNameSpace is the namespace where the operator is deployed
	OLSNameSpace = "openshift-lightspeed"
	// OperatorDeploymentName is the name of the operator deployment
	OperatorDeploymentName = "lightspeed-operator-controller-manager"
	// LLMTokenEnvVar is the environment variable containing the LLM API token
	LLMTokenEnvVar = "LLM_TOKEN"
	// LLMTokenFirstSecretName is the name of the first secret containing the LLM API token
	LLMTokenFirstSecretName = "llm-token-first" // #nosec G101
	// LLMTokenSecondSecretName is the name of the second secret containing the LLM API token
	LLMTokenSecondSecretName = "llm-token-second" // #nosec G101
	// LLMApiTokenFileName
	LLMApiTokenFileName = "apitoken"
	// LLMDefaultProvider
	LLMDefaultProvider = "openai"
	// LLMProviderEnvVar is the environment variable containing the LLM provider
	LLMProviderEnvVar = "LLM_PROVIDER"
	// LLMTypeEnvVar is the environment variable containing the LLM type
	LLMTypeEnvVar = "LLM_TYPE"
	// LLMDefaultType is the default LLM type
	LLMDefaultType = "openai"
	// OpenAIDefaultModel is the default model to use
	OpenAIDefaultModel = "gpt-3.5-turbo"
	// OpenAIAlternativeModel is the alternative model to test model change
	OpenAIAlternativeModel = "gpt-4-1106-preview"
	// LLMModelEnvVar is the environment variable containing the LLM model
	LLMModelEnvVar = "LLM_MODEL"
	// OLSCRName is the name of the OLSConfig CR
	OLSCRName = "cluster"
	// AppServerDeploymentName is the name of the OLS application server deployment
	AppServerDeploymentName = "lightspeed-app-server"
	// AppServerServiceName is the name of the OLS application server service
	AppServerServiceName = "lightspeed-app-server"
	// AppServerServiceHTTPSPort is the port number of the OLS application server service
	AppServerServiceHTTPSPort = 8443
	// ConsolePluginServiceName is the name of the OLS console plugin deployment
	ConsolePluginDeploymentName = "lightspeed-console-plugin"
	// ConsolePluginServiceName is the name of the OLS console plugin service
	ConsolePluginServiceName = "lightspeed-console-plugin"
	// ConsoleUIPluginName is the name of the OLS console plugin
	ConsoleUIPluginName = "lightspeed-console-plugin"
	// ConsoleUIConfigMapName is the name of the console UI nginx configmap
	ConsoleUIConfigMapName = "lightspeed-console-plugin"

	// OLSConsolePluginServiceHTTPSPort is the port number of the OLS console plugin service
	OLSConsolePluginServiceHTTPSPort = 9443
	// AppServerConfigMapName is the name of the OLS application server config map
	AppServerConfigMapName = "olsconfig"
	// AppServerConfigMapKey is the key of config file in the OLS application server config map
	AppServerConfigMapKey = "olsconfig.yaml"
	// AppServerTLSSecretName is the name of the OLS application server TLS secret
	AppServerTLSSecretName = "lightspeed-tls" // #nosec G101
	// ConditionTimeoutEnvVar is the environment variable containing the condition check timeout in seconds
	ConditionTimeoutEnvVar = "CONDITION_TIMEOUT"

	// Test no Konflux build is triggered
	NoKonfluxBuild = "no-konflux-build"
)
