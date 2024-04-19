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
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="LLM Settings"
	LLMConfig LLMSpec `json:"llm"`
	// +kubebuilder:validation:Required
	// +required
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="OLS Settings"
	OLSConfig OLSSpec `json:"ols"`
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
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Providers"
	Providers []ProviderSpec `json:"providers"`
}

// OLSSpec defines the desired state of OLS deployment.
type OLSSpec struct {
	// Conversation cache settings
	// +operator-sdk:csv:customresourcedefinitions:type=spec,order=2,displayName="Conversation Cache"
	ConversationCache ConversationCacheSpec `json:"conversationCache,omitempty"`
	// OLS deployment settings
	// +operator-sdk:csv:customresourcedefinitions:type=spec,order=1,displayName="Deployment"
	DeploymentConfig DeploymentConfig `json:"deployment,omitempty"`
	// Log level. Valid options are DEBUG, INFO, WARNING, ERROR and CRITICAL. Default: "INFO".
	// +kubebuilder:validation:Enum=DEBUG;INFO;WARNING;ERROR;CRITICAL
	// +kubebuilder:default=INFO
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Log level"
	LogLevel string `json:"logLevel,omitempty"`
	// Disable Authorization for OLS server. Default: "false"
	// +kubebuilder:default=false
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Disable Authorization",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:booleanSwitch"}
	DisableAuth bool `json:"disableAuth,omitempty"`
	// Default model for usage
	// +kubebuilder:validation:Required
	// +required
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Default Model",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:advanced"}
	DefaultModel string `json:"defaultModel"`
	// Default provider for usage
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Default Provider",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:advanced"}
	DefaultProvider string `json:"defaultProvider,omitempty"`
	// Classifier provider name
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Classifier Provider",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:advanced"}
	ClassifierProvider string `json:"classifierProvider,omitempty"`
	// Classifier model name
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Classifier Model",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:advanced"}
	ClassifierModel string `json:"classifierModel,omitempty"`
	// Summarizer provider name
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Summarizer Provider",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:advanced"}
	SummarizerProvider string `json:"summarizerProvider,omitempty"`
	// Summarizer model name
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Summarizer Model",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:advanced"}
	SummarizerModel string `json:"summarizerModel,omitempty"`
	// Validator provider name
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Validator Provider",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:advanced"}
	ValidatorProvider string `json:"validatorProvider,omitempty"`
	// Validator model name
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Validator Model",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:advanced"}
	ValidatorModel string `json:"validatorModel,omitempty"`
	// YAML provider name
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="YAML Provider",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:advanced"}
	YamlProvider string `json:"yamlProvider,omitempty"`
	// YAML model name
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="YAML Model",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:advanced"}
	YamlModel string `json:"yamlModel,omitempty"`
	// Query filters
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Query Filters"
	QueryFilters []QueryFiltersSpec `json:"queryFilters,omitempty"`
}

// DeploymentConfig defines the schema for overriding deployment of OLS instance.
type DeploymentConfig struct {
	// Defines the number of desired OLS pods. Default: "1"
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Number of replicas",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:podCount"}
	Replicas *int32 `json:"replicas,omitempty"`
	// The resource requirements for OLS instance.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Resource Requirements",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:resourceRequirements"}
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
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Cache Type"
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
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Filter Name"
	Name string `json:"name,omitempty"`
	// Filter pattern.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="The pattern to replace"
	Pattern string `json:"pattern,omitempty"`
	// Replacement for the matched pattern.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Replace With"
	ReplaceWith string `json:"replaceWith,omitempty"`
}

// ModelSpec defines the desired state of cache.
type ModelSpec struct {
	// Model name
	// +kubebuilder:validation:Required
	// +required
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Name"
	Name string `json:"name"`
	// Model API URL
	// +kubebuilder:validation:Pattern=`^https?://.*$`
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="URL"
	URL string `json:"url,omitempty"`
}

// ProviderSpec defines the desired state of LLM provider.
// +kubebuilder:validation:XValidation:message="'deploymentName' must be specified for 'azure_openai' provider",rule="self.type != \"azure_openai\" || self.deploymentName != \"\""
// +kubebuilder:validation:XValidation:message="'projectID' must be specified for 'watsonx' provider",rule="self.type != \"watsonx\" || self.projectID != \"\""
type ProviderSpec struct {
	// Provider name
	// +kubebuilder:validation:Required
	// +required
	// +operator-sdk:csv:customresourcedefinitions:type=spec,order=1,displayName="Name"
	Name string `json:"name,omitempty"`
	// Provider API URL
	// +kubebuilder:validation:Pattern=`^https?://.*$`
	// +operator-sdk:csv:customresourcedefinitions:type=spec,order=2,displayName="URL"
	URL string `json:"url,omitempty"`
	// The name of the secret object that stores API provider credentials
	// +kubebuilder:validation:Required
	// +required
	// +operator-sdk:csv:customresourcedefinitions:type=spec,order=3,displayName="Credential Secret"
	CredentialsSecretRef corev1.LocalObjectReference `json:"credentialsSecretRef,omitempty"`
	// List of models from the provider
	// +kubebuilder:validation:Required
	// +required
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Models"
	Models []ModelSpec `json:"models,omitempty"`
	// Provider type
	// +kubebuilder:validation:Required
	// +required
	// +kubebuilder:validation:Enum=azure_openai;bam;openai;watsonx
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Provider Type"
	Type string `json:"type"`
	// Azure OpenAI deployment name
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Azure OpenAI deployment name"
	AzureDeploymentName string `json:"deploymentName,omitempty"`
	// Watsonx Project ID
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Watsonx Project ID"
	WatsonProjectID string `json:"projectID,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'cluster'",message=".metadata.name must be 'cluster'"
// Red Hat OpenShift LightSpeed instance. OLSConfig is the Schema for the olsconfigs API
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
