/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// OLSConfigSpec defines the desired state of OLSConfig
type OLSConfigSpec struct {
	// +kubebuilder:validation:Required
	// +required
	LLMConfig LLMSpec `json:"llm"`
	OLSConfig OLSSpec `json:"ols,omitempty"`
}

// OLSConfigStatus defines the observed state of OLS deployment.
type OLSConfigStatus struct {
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Conditions []metav1.Condition `json:"conditions"`
}

// LLMSpec defines the desired state of the large language model (LLM).
type LLMSpec struct {
	// +kubebuilder:validation:Required
	// +required
	Providers []ProviderSpec `json:"providers"`
}

// OLSSpec defines the desired state of OLS deployment.
type OLSSpec struct {
	// Conversation cache settings
	ConversationCache ConversationCacheSpec `json:"conversationCache,omitempty"`
	// OLS deployment settings
	DeploymentConfig DeploymentConfig `json:"deployment,omitempty"`
	// Log level. Default: "INFO". Valid options are DEBUG, INFO, WARNING, ERROR and CRITICAL.
	// +kubebuilder:validation:Enum=DEBUG;INFO;WARNING;ERROR;CRITICAL
	// +kubebuilder:default=INFO
	LogLevel string `json:"logLevel,omitempty"`
	// Disable Authorization for OLS server. Default: "false"
	// +kubebuilder:default=false
	DisableAuth bool `json:"disableAuth,omitempty"`
	// Default model for usage
	DefaultModel string `json:"defaultModel,omitempty"`
	// Default provider for usage
	DefaultProvider string `json:"defaultProvider,omitempty"`
	// Classifier provider name
	ClassifierProvider string `json:"classifierProvider,omitempty"`
	// Classifier model name
	ClassifierModel string `json:"classifierModel,omitempty"`
	// Summarizer provider name
	SummarizerProvider string `json:"summarizerProvider,omitempty"`
	// Summarizer model name
	SummarizerModel string `json:"summarizerModel,omitempty"`
	// Validator provider name
	ValidatorProvider string `json:"validatorProvider,omitempty"`
	// Validator model name
	ValidatorModel string `json:"validatorModel,omitempty"`
	// YAML provider name
	YamlProvider string `json:"yamlProvider,omitempty"`
	// YAML model name
	YamlModel string `json:"yamlModel,omitempty"`
	// Query filters
	QueryFilters []QueryFiltersSpec `json:"queryFilters,omitempty"`
}

// DeploymentConfig defines the schema for overriding deployment of OLS instance.
type DeploymentConfig struct {
	// Defines the number of desired OLS pods. Default: "1"
	// +kubebuilder:default=1
	Replicas  *int32                       `json:"replicas,omitempty"`
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// +kubebuilder:validation:Enum=postgres
type CacheType string

const (
	Postgres CacheType = "postgres"
)

// ConversationCacheSpec defines the desired state of OLS conversation cache.
type ConversationCacheSpec struct {
	// Conversation cache type. Default: "postgres"
	// +kubebuilder:default=postgres
	Type CacheType `json:"type,omitempty"`
	// +optional
	Postgres PostgresSpec `json:"postgres,omitempty"`
}

// PostgresSpec defines the desired state of Postgres.
type PostgresSpec struct {
	// Postgres user name
	// +kubebuilder:default="postgres"
	User string `json:"user,omitempty"`
	// Postgres database name
	// +kubebuilder:default="postgres"
	DbName string `json:"dbName,omitempty"`
	// Secret that holds postgres credentials
	// +kubebuilder:default="lightspeed-postgres-secret"
	CredentialsSecret string `json:"credentialsSecret,omitempty"`
	// Postgres sharedbuffers
	// +kubebuilder:validation:XIntOrString
	// +kubebuilder:default="256MB"
	SharedBuffers *intstr.IntOrString `json:"sharedBuffers,omitempty"`
	// Postgres maxconnections. Default: "2000"
	// +kubebuilder:default=2000
	MaxConnections int `json:"maxConnections,omitempty"`
}

// QueryFiltersSpec defines filters to manipulate questions/queries.
type QueryFiltersSpec struct {
	// Filter name.
	Name string `json:"name,omitempty"`
	// Filter pattern.
	Pattern string `json:"pattern,omitempty"`
	// Replacement for the matched pattern.
	ReplaceWith string `json:"replaceWith,omitempty"`
}

// ModelSpec defines the desired state of cache.
type ModelSpec struct {
	// Model name
	// +kubebuilder:validation:Required
	// +required
	Name string `json:"name"`
	// Model API URL
	URL string `json:"url,omitempty"`
}

// ProviderSpec defines the desired state of LLM provider.
type ProviderSpec struct {
	// Provider name
	// +kubebuilder:validation:Required
	// +required
	Name string `json:"name,omitempty"`
	// Provider API URL
	URL string `json:"url,omitempty"`
	// Name of a Kubernetes Secret resource containing API provider credentials.
	// +kubebuilder:validation:Required
	// +required
	CredentialsSecretRef corev1.LocalObjectReference `json:"credentialsSecretRef,omitempty"`
	// List of models from the provider
	// +kubebuilder:validation:Required
	// +required
	Models []ModelSpec `json:"models,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// OLSConfig is the Schema for the olsconfigs API
type OLSConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +kubebuilder:validation:Required
	// +required
	Spec   OLSConfigSpec   `json:"spec"`
	Status OLSConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// OLSConfigList contains a list of OLSConfig
type OLSConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OLSConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OLSConfig{}, &OLSConfigList{})
}
