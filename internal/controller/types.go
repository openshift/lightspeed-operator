package controller

import (
	"context"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

/*** controller inernal ***/
type ReconcileFunc func(context.Context, *olsv1alpha1.OLSConfig) error
type ReconcileTask struct {
	Name string
	Task ReconcileFunc
}

/*** application server configuration file ***/
// root of the app server configuration file
type AppSrvConfigFile struct {
	LLMProviders []ProviderConfig `json:"llm_providers"`
	OLSConfig    OLSConfig        `json:"ols_config,omitempty"`
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
	Models []ModelConfig `json:"models,omitempty" `
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
}

type LoggingConfig struct {
	// Application log level
	AppLogLevel string `json:"app_log_level" default:"info"`
	// Library log level
	LibLogLevel string `json:"lib_log_level" default:"warning"`
}

type ConversationCacheConfig struct {
	// Type of cache to use. Default: "redis"
	Type string `json:"type" default:"redis"`
	// Redis cache configuration
	Redis RedisCacheConfig `json:"redis,omitempty"`
}

type RedisCacheConfig struct {
	// Redis maxmemory
	MaxMemory *intstr.IntOrString `json:"max_memory,omitempty" default:"1024mb"`
	// Redis maxmemory policy
	MaxMemoryPolicy string `json:"max_memory_policy,omitempty" default:"allkeys-lru"`
	// Path to the file containing redis credentials in the app server container.
	// default to "lightspeed_redis_secret.txt"
	CredentialsPath string `json:"credentials_path,omitempty"`
}
