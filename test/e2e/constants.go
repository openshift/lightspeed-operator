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
	OpenAIDefaultModel = "gpt-4o-mini"
	// OpenAIAlternativeModel is the alternative model to test model change
	OpenAIAlternativeModel = "gpt-4-1106-preview"
	// LLMModelEnvVar is the environment variable containing the LLM model
	LLMModelEnvVar = "LLM_MODEL"
	// AzureTenantID is the environment variable containing the tenant id for azure openai authentication
	AzureTenantID = "AZUREOPENAI_ENTRA_ID_TENANT_ID"
	// AzureClientID is the environment variable containing the client id for azure openai authentication
	AzureClientID = "AZUREOPENAI_ENTRA_ID_CLIENT_ID"
	// AzureClientSecret is the environment variable containing the client secret for azure openai authentication
	AzureClientSecret = "AZUREOPENAI_ENTRA_ID_CLIENT_SECRET"
	// AzureOpenaiTenantID
	AzureOpenaiTenantID = "tenant_id"
	// AzureOpenaiClientID
	AzureOpenaiClientID = "client_id"
	// AzureOpenaiClientSecret
	AzureOpenaiClientSecret = "client_secret"
	// AzureURL
	AzureURL = "https://ols-test.openai.azure.com/"
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
	// ArtifactDir is the relative path to where the artifacts will be exported to
	ArtifactDir = "ARTIFACT_DIR"
	// serverContainerName is the name of the app-server container
	ServerContainerName = "lightspeed-service-api"
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

	// ServiceAnnotationKeyTLSSecret is the annotation key for TLS secret
	ServiceAnnotationKeyTLSSecret = "service.beta.openshift.io/serving-cert-secret-name"
	// TestSAName is the name of the test service account
	TestSAName = "test-sa"
	// TestSAOutsiderName is the name of the test service account for outsider tests
	TestSAOutsiderName = "test-sa-outsider"
	// QueryAccessClusterRole is the cluster role for query access
	QueryAccessClusterRole = "lightspeed-operator-query-access"
	// AppMetricsAccessClusterRole is the cluster role for app metrics access
	AppMetricsAccessClusterRole = "lightspeed-operator-ols-metrics-reader"
	// OLSRouteName is the name of the OLS route
	OLSRouteName = "ols-route"
	// InClusterHost is the in-cluster host for the lightspeed app server
	InClusterHost = "lightspeed-app-server.openshift-lightspeed.svc.cluster.local"

	// TestCACert is for testing additional CA certificate
	TestCACert = `-----BEGIN CERTIFICATE-----
MIIEMDCCAxigAwIBAgIJANqb7HHzA7AZMA0GCSqGSIb3DQEBCwUAMIGkMQswCQYD
VQQGEwJQQTEPMA0GA1UECAwGUGFuYW1hMRQwEgYDVQQHDAtQYW5hbWEgQ2l0eTEk
MCIGA1UECgwbVHJ1c3RDb3IgU3lzdGVtcyBTLiBkZSBSLkwuMScwJQYDVQQLDB5U
cnVzdENvciBDZXJ0aWZpY2F0ZSBBdXRob3JpdHkxHzAdBgNVBAMMFlRydXN0Q29y
IFJvb3RDZXJ0IENBLTEwHhcNMTYwMjA0MTIzMjE2WhcNMjkxMjMxMTcyMzE2WjCB
pDELMAkGA1UEBhMCUEExDzANBgNVBAgMBlBhbmFtYTEUMBIGA1UEBwwLUGFuYW1h
IENpdHkxJDAiBgNVBAoMG1RydXN0Q29yIFN5c3RlbXMgUy4gZGUgUi5MLjEnMCUG
A1UECwweVHJ1c3RDb3IgQ2VydGlmaWNhdGUgQXV0aG9yaXR5MR8wHQYDVQQDDBZU
cnVzdENvciBSb290Q2VydCBDQS0xMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIB
CgKCAQEAv463leLCJhJrMxnHQFgKq1mqjQCj/IDHUHuO1CAmujIS2CNUSSUQIpid
RtLByZ5OGy4sDjjzGiVoHKZaBeYei0i/mJZ0PmnK6bV4pQa81QBeCQryJ3pS/C3V
seq0iWEk8xoT26nPUu0MJLq5nux+AHT6k61sKZKuUbS701e/s/OojZz0JEsq1pme
9J7+wH5COucLlVPat2gOkEz7cD+PSiyU8ybdY2mplNgQTsVHCJCZGxdNuWxu72CV
EY4hgLW9oHPY0LJ3xEXqWib7ZnZ2+AYfYW0PVcWDtxBWcgYHpfOxGgMFZA6dWorW
hnAbJN7+KIor0Gqw/Hqi3LJ5DotlDwIDAQABo2MwYTAdBgNVHQ4EFgQU7mtJPHo/
DeOxCbeKyKsZn3MzUOcwHwYDVR0jBBgwFoAU7mtJPHo/DeOxCbeKyKsZn3MzUOcw
DwYDVR0TAQH/BAUwAwEB/zAOBgNVHQ8BAf8EBAMCAYYwDQYJKoZIhvcNAQELBQAD
ggEBACUY1JGPE+6PHh0RU9otRCkZoB5rMZ5NDp6tPVxBb5UrJKF5mDo4Nvu7Zp5I
/5CQ7z3UuJu0h3U/IJvOcs+hVcFNZKIZBqEHMwwLKeXx6quj7LUKdJDHfXLy11yf
ke+Ri7fc7Waiz45mO7yfOgLgJ90WmMCV1Aqk5IGadZQ1nJBfiDcGrVmVCrDRZ9MZ
yonnMlo2HD6CqFqTvsbQZJG2z9m2GM/bftJlo6bEjhcxwft+dtvTheNYsnd6djts
L1Ac59v2Z3kf9YKVmgenFK+P3CghZwnS1k1aHBkcjndcw5QkPTJrS37UeJSDvjdN
zl/HHk484IkzlQsPpTLWPFp5LBk=
-----END CERTIFICATE-----
`
	// OLSAppCertsMountRoot is the directory hosting the cert files in the container
	OLSAppCertsMountRoot = "/etc/certs"
	// AdditionalCAVolumeName is the name of the additional CA volume in the app server container
	AdditionalCAVolumeName = "additional-ca"
	// UserCACertDir is the directory for storing additional CA certificates in the app server container under OLSAppCertsMountRoot
	UserCACertDir = "ols-user-ca"
	// AdditionalCAHashKey is the key of the hash value of the additional CA certificates
	AdditionalCAHashKey = "hash/additionalca"
	// CertBundleVolumeName is the name of the volume for the certificate bundle
	CertBundleVolumeName = "cert-bundle"
	// CertBundleDir is the path of the volume for the certificate bundle
	CertBundleDir = "cert-bundle"

	// PostgresDeploymentName is the name of OLS application Postgres deployment
	PostgresDeploymentName = "lightspeed-postgres-server"
)
