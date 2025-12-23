package utils

import (
	"context"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
)

// Definitions to manage status conditions
const (
	TypeApiReady           = "ApiReady"
	TypeCacheReady         = "CacheReady"
	TypeConsolePluginReady = "ConsolePluginReady"
	TypeCRReconciled       = "Reconciled"
)

type OLSConfigReconcilerOptions struct {
	OpenShiftMajor                 string
	OpenshiftMinor                 string
	LightspeedServiceImage         string
	LightspeedServicePostgresImage string
	ConsoleUIImage                 string
	DataverseExporterImage         string
	OpenShiftMCPServerImage        string
	LightspeedCoreImage            string
	OcpRagImage                    string
	UseLCore                       bool
	Namespace                      string
	PrometheusAvailable            bool
}

// SystemSecret represents a secret managed by Kubernetes or other applications
// that the operator needs to watch for changes
type SystemSecret struct {
	Name                string
	Namespace           string
	Description         string
	AffectedDeployments []string
}

// SystemConfigMap represents a configmap managed by Kubernetes or other applications
// that the operator needs to watch for changes
type SystemConfigMap struct {
	Name                string
	Namespace           string
	Description         string
	AffectedDeployments []string
}

// SecretWatcherConfig contains configuration for watching secrets
type SecretWatcherConfig struct {
	SystemResources []SystemSecret
}

// ConfigMapWatcherConfig contains configuration for watching configmaps
type ConfigMapWatcherConfig struct {
	SystemResources []SystemConfigMap
}

// WatcherConfig contains all watcher configuration
type WatcherConfig struct {
	Secrets                   SecretWatcherConfig
	ConfigMaps                ConfigMapWatcherConfig
	AnnotatedSecretMapping    map[string][]string
	AnnotatedConfigMapMapping map[string][]string
}

/*** controller internal ***/
type ReconcileFunc func(reconciler.Reconciler, context.Context, *olsv1alpha1.OLSConfig) error
type ReconcileTask struct {
	Name string
	Task ReconcileFunc
}

type DeleteFunc func(reconciler.Reconciler, context.Context) error
type DeleteTask struct {
	Name string
	Task DeleteFunc
}

/*** application server configuration file ***/
// root of the app server configuration file
type AppSrvConfigFile struct {
	LLMProviders            []ProviderConfig        `json:"llm_providers"`
	OLSConfig               OLSConfig               `json:"ols_config,omitempty"`
	UserDataCollectorConfig UserDataCollectorConfig `json:"user_data_collector_config,omitempty"`
	MCPServers              []MCPServerConfig       `json:"mcp_servers,omitempty"`
}

type ProviderConfig struct {
	// Provider name
	Name string `json:"name"`
	// Provider API URL
	URL string `json:"url,omitempty"`
	// Path to the file containing API provider credentials in the app server container.
	// default to "bam_api_key.txt"
	CredentialsPath string `json:"credentials_path,omitempty" default:"bam_api_key.txt"`
	// List of models from the provider
	Models []ModelConfig `json:"models,omitempty"`
	// Provider type
	Type string `json:"type,omitempty"`
	// Watsonx Project ID
	WatsonProjectID string `json:"project_id,omitempty"`
	// API Version for Azure OpenAI provider
	APIVersion string `json:"api_version,omitempty"`
	// Azure OpenAI Config
	AzureOpenAIConfig *AzureOpenAIConfig `json:"azure_openai_config,omitempty"`
}

type AzureOpenAIConfig struct {
	// Azure OpenAI API URL
	URL string `json:"url,omitempty"`
	// Path where Azure OpenAI accesstoken or credentials are stored
	CredentialsPath string `json:"credentials_path"`
	// Azure deployment name
	AzureDeploymentName string `json:"deployment_name,omitempty"`
}

// ModelParameters defines the parameters for a model.
type ModelParameters struct {
	// Maximum number of tokens for the input text. Default: 1024
	MaxTokensForResponse int `json:"max_tokens_for_response,omitempty"`
}

// ModelSpec defines the desired state of in-memory cache.
type ModelConfig struct {
	// Model name
	Name string `json:"name"`
	// Model API URL
	URL string `json:"url,omitempty"`
	// Model context window size
	ContextWindowSize uint `json:"context_window_size,omitempty"`
	// Model parameters
	Parameters ModelParameters `json:"parameters,omitempty"`
}

type OLSConfig struct {
	// Default model for usage
	DefaultModel string `json:"default_model,omitempty"`
	// Default provider for usage
	DefaultProvider string `json:"default_provider,omitempty"`
	// Logging config
	Logging LoggingConfig `json:"logging_config,omitempty"`
	// Conversation cache
	ConversationCache ConversationCacheConfig `json:"conversation_cache,omitempty"`
	// TLS configuration
	TLSConfig TLSConfig `json:"tls_config,omitempty"`
	// Query filters
	QueryFilters []QueryFilters `json:"query_filters,omitempty"`
	// Reference content for RAG
	ReferenceContent ReferenceContent `json:"reference_content,omitempty"`
	// User data collection configuration
	UserDataCollection UserDataCollectionConfig `json:"user_data_collection,omitempty"`
	// List of Paths to files containing additional CA certificates in the app server container.
	ExtraCAs []string `json:"extra_ca,omitempty"`
	// Path to the directory containing the certificates bundle in the app server container.
	CertificateDirectory string `json:"certificate_directory,omitempty"`
	// Proxy settings
	ProxyConfig *ProxyConfig `json:"proxy_config,omitempty"`
	// LLM Token Quota Configuration
	QuotaHandlersConfig *QuotaHandlersConfig `json:"quota_handlers,omitempty"`
}

// QuotaHandlersConfig defines the token quota configuration
type QuotaHandlersConfig struct {
	// Postgres connection details
	Storage PostgresCacheConfig `json:"storage,omitempty"`
	// Quota scheduler settings
	Scheduler SchedulerConfig `json:"scheduler,omitempty"`
	// Token quota limiters
	LimitersConfig []LimiterConfig `json:"limiters,omitempty"`
	// Enable token history
	EnableTokenHistory bool `json:"enable_token_history,omitempty"`
}

// LimiterConfig defines settings for a token quota limiter
type LimiterConfig struct {
	// Name of the limiter
	Name string `json:"name"`
	// Type of the limiter
	Type string `json:"type"`
	// Initial value of the token quota
	InitialQuota int `json:"initial_quota"`
	// Token quota increase step
	QuotaIncrease int `json:"quota_increase"`
	// Period of time the token quota is for
	Period string `json:"period"`
}

// Scheduler configuration
type SchedulerConfig struct {
	// How often token quota is checked, sec
	Period int `json:"period,omitempty"`
}

type LoggingConfig struct {
	// Application log level
	AppLogLevel string `json:"app_log_level" default:"info"`
	// Library log level
	LibLogLevel string `json:"lib_log_level" default:"warning"`
	// Uvicorn log level
	UvicornLogLevel string `json:"uvicorn_log_level" default:"info"`
}

type ConversationCacheConfig struct {
	// Type of cache to use. Default: "postgres"
	Type string `json:"type" default:"postgres"`
	// Postgres cache configuration
	Postgres PostgresCacheConfig `json:"postgres,omitempty"`
}

type MemoryCacheConfig struct {
	// Maximum number of cache entries. Default: "1000"
	MaxEntries int `json:"max_entries,omitempty" default:"1000"`
}

type PostgresCacheConfig struct {
	// Postgres host
	Host string `json:"host,omitempty" default:"lightspeed-postgres-server.openshift-lightspeed.svc"`
	// Postgres port
	Port int `json:"port,omitempty" default:"5432"`
	// Postgres user
	User string `json:"user,omitempty" default:"postgres"`
	// Postgres dbname
	DbName string `json:"dbname,omitempty" default:"postgres"`
	// Path to the file containing postgres credentials in the app server container
	PasswordPath string `json:"password_path,omitempty"`
	// SSLMode is the preferred ssl mode to connect with postgres
	SSLMode string `json:"ssl_mode,omitempty" default:"require"`
	// Postgres CA certificate path
	CACertPath string `json:"ca_cert_path,omitempty"`
}

type TLSConfig struct {
	TLSCertificatePath string `json:"tls_certificate_path,omitempty"`
	TLSKeyPath         string `json:"tls_key_path,omitempty"`
}

type QueryFilters struct {
	// Filter name.
	Name string `json:"name,omitempty"`
	// Filter pattern.
	Pattern string `json:"pattern,omitempty"`
	// Replacement for the matched pattern.
	ReplaceWith string `json:"replace_with,omitempty"`
}

type ReferenceIndex struct {
	// Path to the file containing the product docs index in the app server container.
	ProductDocsIndexPath string `json:"product_docs_index_path,omitempty"`
	// Name of the index to load.
	ProductDocsIndexId string `json:"product_docs_index_id,omitempty"`
	// Where the database was copied from, i.e. BYOK image name.
	ProductDocsOrigin string `json:"product_docs_origin,omitempty"`
}

type ReferenceContent struct {
	// Path to the file containing the product docs embeddings model in the app server container.
	EmbeddingsModelPath string `json:"embeddings_model_path,omitempty"`
	// List of reference indexes.
	Indexes []ReferenceIndex `json:"indexes,omitempty"`
}

type UserDataCollectionConfig struct {
	FeedbackDisabled    bool   `json:"feedback_disabled" default:"false"`
	FeedbackStorage     string `json:"feedback_storage,omitempty"`
	TranscriptsDisabled bool   `json:"transcripts_disabled" default:"false"`
	TranscriptsStorage  string `json:"transcripts_storage,omitempty"`
}

type UserDataCollectorConfig struct {
	// Path to dir where ols user data (feedback and transcripts) are stored
	DataStorage string `json:"data_storage,omitempty"`
	// Collector logging level
	LogLevel string `json:"log_level,omitempty"`
}

type MCPTransport string

const (
	SSE            MCPTransport = "sse"
	Stdio          MCPTransport = "stdio"
	StreamableHTTP MCPTransport = "streamable_http"
)

type MCPServerConfig struct {
	// MCP server name
	Name string `json:"name"`
	// MCP server transport - stdio or sse
	Transport MCPTransport `json:"transport"`
	// Transport settings if the transport is stdio
	Stdio *StdioTransportConfig `json:"stdio,omitempty"`
	// Transport settings if the transport is sse
	SSE *StreamableHTTPTransportConfig `json:"sse,omitempty"`
	// Transport settings if the transport is streamable_http
	StreamableHTTP *StreamableHTTPTransportConfig `json:"streamable_http,omitempty"`
}

type StdioTransportConfig struct {
	// Command to run
	Command string `json:"command,omitempty"`
	// Command-line parameters for the command
	Args []string `json:"args,omitempty"`
	// Environment variables for the command
	Env map[string]string `json:"env,omitempty"`
	// The working directory for the command
	Cwd string `json:"cwd,omitempty"`
	// Encoding for the text exchanged with the command
	Encoding string `json:"encoding,omitempty"`
}

type StreamableHTTPTransportConfig struct {
	// URL of the MCP server
	URL string `json:"url,omitempty"`
	// Overall timeout for the MCP server
	Timeout int `json:"timeout,omitempty"`
	// SSE read timeout for the MCP server
	SSEReadTimeout int `json:"sse_read_timeout,omitempty"`
	// Headers to send to the MCP server
	Headers map[string]string `json:"headers,omitempty"`
}

type ProxyConfig struct {
	// Proxy URL
	ProxyURL string `json:"proxy_url,omitempty"`
	// ProxyCACertPath is the path to the CA certificate for the proxy server
	ProxyCACertPath string `json:"proxy_ca_cert_path,omitempty"`
}

type OperatorReconcileFuncs struct {
	Name string
	Fn   func(context.Context) error
}

type ReconcileSteps struct {
	Name          string
	Fn            func(context.Context, *olsv1alpha1.OLSConfig) error
	ConditionType string
	Deployment    string
}
