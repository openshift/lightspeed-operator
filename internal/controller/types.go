package controller

import (
	"context"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

/*** controller inernal ***/
type ReconcileFunc func(context.Context, *olsv1alpha1.OLSConfig) error
type ReconcileTask struct {
	Name string
	Task ReconcileFunc
}

type DeleteFunc func(context.Context) error
type DeleteTask struct {
	Name string
	Task DeleteFunc
}

/*** application server configuration file ***/
// root of the app server configuration file
type AppSrvConfigFile struct {
	LLMProviders []ProviderConfig `json:"llm_providers"`
	OLSConfig    OLSConfig        `json:"ols_config,omitempty"`
	DevConfig    DevConfig        `json:"dev_config,omitempty"`
}

type ProviderConfig struct {
	// Provider name
	Name string `json:"name"`
	// Provider API URL
	URL string `json:"url,omitempty"`
	// Path to the file containing API provider credentials in the app server container.
	// default to "bam_api_key.txt"
	CredentialsPath string `json:"credentials_path" default:"bam_api_key.txt"`
	// List of models from the provider
	Models []ModelConfig `json:"models,omitempty"`
	// Provider type
	Type string `json:"type,omitempty"`
	// Azure deployment name
	AzureDeploymentName string `json:"deployment_name,omitempty"`
	// Watsonx Project ID
	WatsonProjectID string `json:"project_id,omitempty"`
}

// ModelSpec defines the desired state of in-memory cache.
type ModelConfig struct {
	// Model name
	Name string `json:"name"`
	// Model API URL
	URL string `json:"url,omitempty"`
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

type DevConfig struct {
	// User Authorization enable/disable
	DisableAuth bool `json:"disable_auth" default:"false"`
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

type ReferenceContent struct {
	// Path to the file containing the product docs index in the app server container.
	ProductDocsIndexPath string `json:"product_docs_index_path,omitempty"`
	// Name of the index to load.
	ProductDocsIndexId string `json:"product_docs_index_id,omitempty"`
	// Path to the file containing the product docs embeddings model in the app server container.
	EmbeddingsModelPath string `json:"embeddings_model_path,omitempty"`
}

type UserDataCollectionConfig struct {
	FeedbackDisabled    bool   `json:"feedback_disabled" default:"false"`
	FeedbackStorage     string `json:"feedback_storage,omitempty"`
	TranscriptsDisabled bool   `json:"transcripts_disabled" default:"false"`
	TranscriptsStorage  string `json:"transcripts_storage,omitempty"`
}
