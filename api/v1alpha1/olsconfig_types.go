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
	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="OLS Data Collector Settings"
	OLSDataCollectorConfig OLSDataCollectorSpec `json:"olsDataCollector,omitempty"`
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
	// Default model for usage
	// +kubebuilder:validation:Required
	// +required
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Default Model",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	DefaultModel string `json:"defaultModel"`
	// Default provider for usage
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Default Provider",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	DefaultProvider string `json:"defaultProvider,omitempty"`
	// Query filters
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Query Filters"
	QueryFilters []QueryFiltersSpec `json:"queryFilters,omitempty"`
	// User data collection switches
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="User Data Collection"
	UserDataCollection UserDataCollectionSpec `json:"userDataCollection,omitempty"`
	// TLS configuration of the Lightspeed backend's HTTPS endpoint
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="TLS Configuration"
	TLSConfig *TLSConfig `json:"tlsConfig,omitempty"`
	// Additional CA certificates for TLS communication between OLS service and LLM Provider
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Additional CA Configmap",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:advanced"}
	AdditionalCAConfigMapRef *corev1.LocalObjectReference `json:"additionalCAConfigMapRef,omitempty"`
	// TLS Security Profile used by API endpoints
	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="TLS Security Profile",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:advanced"}
	TLSSecurityProfile *configv1.TLSSecurityProfile `json:"tlsSecurityProfile,omitempty"`
	// Enable introspection features
	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Introspection Enabled",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:booleanSwitch"}
	IntrospectionEnabled bool `json:"introspectionEnabled,omitempty"`
	// RAG databases
	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="RAG Databases",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:advanced"}
	RAG []RAGSpec `json:"rag,omitempty"`
}

// RAGSpec defines how to retrieve a RAG databases.
type RAGSpec struct {
	// +kubebuilder:validation:Required
	// +required
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Index Path in the Image"
	IndexPath string `json:"indexPath,omitempty"`
	// +kubebuilder:validation:Required
	// +required
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Index ID"
	IndexID string `json:"indexID,omitempty"`
	// +kubebuilder:validation:Required
	// +required
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Image"
	Image string `json:"image,omitempty"`
}

// DeploymentConfig defines the schema for overriding deployment of OLS instance.
type DeploymentConfig struct {
	// Defines the number of desired OLS pods. Default: "1"
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Number of replicas",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:podCount"}
	Replicas *int32 `json:"replicas,omitempty"`
	// API container settings.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="API Container"
	APIContainer APIContainerConfig `json:"api,omitempty"`
	// Data Collector container settings.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Data Collector Container"
	DataCollectorContainer DataCollectorContainerConfig `json:"dataCollector,omitempty"`
	// Console container settings.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Console Container"
	ConsoleContainer ConsoleContainerConfig `json:"console,omitempty"`
}

type APIContainerConfig struct {
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Resource Requirements",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:resourceRequirements"}
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Tolerations",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:tolerations"}
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Node Selector",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:nodeSelector"}
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

type DataCollectorContainerConfig struct {
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Resource Requirements",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:resourceRequirements"}
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

type ConsoleContainerConfig struct {
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Resource Requirements",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:resourceRequirements"}
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Tolerations",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:tolerations"}
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Node Selector",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:nodeSelector"}
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Defines the number of desired Console pods. Default: "1"
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Number of replicas",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:podCount"}
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`
	// Certificate Authority (CA) certificate used by the console proxy endpoint.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="CA Certificate",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:caCertificate"}
	// +kubebuilder:validation:Pattern=`^-----BEGIN CERTIFICATE-----([\s\S]*)-----END CERTIFICATE-----\s?$`
	// +optional
	CAcertificate string `json:"caCertificate,omitempty"`
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
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="PostgreSQL Settings"
	Postgres PostgresSpec `json:"postgres,omitempty"`
}

// PostgresSpec defines the desired state of Postgres.
type PostgresSpec struct {
	// Postgres user name
	// +kubebuilder:default="postgres"
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="User Name"
	User string `json:"user,omitempty"`
	// Postgres database name
	// +kubebuilder:default="postgres"
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Database Name"
	DbName string `json:"dbName,omitempty"`
	// Secret that holds postgres credentials
	// +kubebuilder:default="lightspeed-postgres-secret"
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Credentials Secret"
	CredentialsSecret string `json:"credentialsSecret,omitempty"`
	// Postgres sharedbuffers
	// +kubebuilder:validation:XIntOrString
	// +kubebuilder:default="256MB"
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Shared Buffer Size"
	SharedBuffers string `json:"sharedBuffers,omitempty"`
	// Postgres maxconnections. Default: "2000"
	// +kubebuilder:default=2000
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Maximum Connections"
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

// ModelParametersSpec
type ModelParametersSpec struct {
	// Max tokens for response
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Max Tokens For Response"
	MaxTokensForResponse int `json:"maxTokensForResponse,omitempty"`
}

// ModelSpec defines the LLM model to use and its parameters.
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
	// Defines the model's context window size. Default is specific to provider/model.
	// +kubebuilder:validation:Minimum=1024
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Context Window Size"
	ContextWindowSize uint `json:"contextWindowSize,omitempty"`
	// Model API parameters
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Parameters"
	Parameters ModelParametersSpec `json:"parameters,omitempty"`
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
	// +kubebuilder:validation:Enum=azure_openai;bam;openai;watsonx;rhoai_vllm;rhelai_vllm;fake_provider
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Provider Type"
	Type string `json:"type"`
	// Azure OpenAI deployment name
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Azure OpenAI deployment name"
	AzureDeploymentName string `json:"deploymentName,omitempty"`
	// API Version for Azure OpenAI provider
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Azure OpenAI API Version"
	APIVersion string `json:"apiVersion,omitempty"`
	// Watsonx Project ID
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Watsonx Project ID"
	WatsonProjectID string `json:"projectID,omitempty"`
	// TLS Security Profile used by connection to provider
	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="TLS Security Profile",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:advanced"}
	TLSSecurityProfile *configv1.TLSSecurityProfile `json:"tlsSecurityProfile,omitempty"`
}

// UserDataCollectionSpec defines how we collect user data.
type UserDataCollectionSpec struct {
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Do Not Collect User Feedback"
	FeedbackDisabled bool `json:"feedbackDisabled,omitempty"`
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Do Not Collect Transcripts"
	TranscriptsDisabled bool `json:"transcriptsDisabled,omitempty"`
}

// OLSDataCollectorSpec defines allowed OLS data collector configuration.
type OLSDataCollectorSpec struct {
	// Log level. Valid options are DEBUG, INFO, WARNING, ERROR and CRITICAL. Default: "INFO".
	// +kubebuilder:validation:Enum=DEBUG;INFO;WARNING;ERROR;CRITICAL
	// +kubebuilder:default=INFO
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Log level"
	LogLevel string `json:"logLevel,omitempty"`
}

type TLSConfig struct {
	// KeySecretRef is the secret that holds the TLS key.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Key Secret"
	KeyCertSecretRef corev1.LocalObjectReference `json:"keyCertSecretRef,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'cluster'",message=".metadata.name must be 'cluster'"
// Red Hat OpenShift Lightspeed instance. OLSConfig is the Schema for the olsconfigs API
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
