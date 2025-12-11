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
	resource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
	// MCP Server settings
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="MCP Server Settings"
	MCPServers []MCPServer `json:"mcpServers,omitempty"`
	// Feature Gates holds list of features to be enabled explicitly, otherwise they are disabled by default.
	// possible values: MCPServer
	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Feature Gates"
	FeatureGates []FeatureGate `json:"featureGates,omitempty"`
}

// +kubebuilder:validation:Enum=MCPServer
type FeatureGate string

// OLSConfigStatus defines the observed state of OLS deployment.
type OLSConfigStatus struct {
	// Conditions represent the state of individual components
	// Always populated after first reconciliation
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Conditions []metav1.Condition `json:"conditions"`

	// OverallStatus provides a high-level summary of the entire system's health.
	// Aggregates all component conditions into a single status value.
	// - Ready: All components are healthy
	// - NotReady: At least one component is not ready (check conditions for details)
	// Always set after first reconciliation
	// +kubebuilder:validation:Enum=Ready;NotReady
	// +operator-sdk:csv:customresourcedefinitions:type=status
	OverallStatus OverallStatus `json:"overallStatus"`

	// DiagnosticInfo provides detailed troubleshooting information when deployments fail.
	// Each entry contains pod-level error details for a specific component.
	// This array is automatically populated when deployments fail and cleared when they recover.
	// Only present during deployment failures.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=status
	DiagnosticInfo []PodDiagnostic `json:"diagnosticInfo,omitempty"`
}

// PodDiagnostic describes a pod-level issue
type PodDiagnostic struct {
	// FailedComponent identifies which component this diagnostic relates to,
	// using the same type as the Conditions field (e.g., "ApiReady", "CacheReady")
	// This allows easy correlation between condition status and diagnostic details.
	FailedComponent string `json:"failedComponent"`

	// PodName is the name of the pod with issues
	PodName string `json:"podName"`

	// ContainerName is the container within the pod that failed
	// Empty if the issue is at the pod level (e.g., scheduling)
	// +optional
	ContainerName string `json:"containerName,omitempty"`

	// Reason is the failure reason
	// Examples: ImagePullBackOff, CrashLoopBackOff, Unschedulable, OOMKilled
	Reason string `json:"reason"`

	// Message provides detailed error information from Kubernetes
	Message string `json:"message"`

	// ExitCode for terminated containers (only set for container failures)
	// +optional
	ExitCode *int32 `json:"exitCode,omitempty"`

	// Type indicates the diagnostic type
	// +kubebuilder:validation:Enum=ContainerWaiting;ContainerTerminated;PodScheduling;PodCondition
	Type DiagnosticType `json:"type"`

	// LastUpdated is the timestamp when this diagnostic was collected
	LastUpdated metav1.Time `json:"lastUpdated"`
}

// DiagnosticType categorizes the type of diagnostic
// +kubebuilder:validation:Enum=ContainerWaiting;ContainerTerminated;PodScheduling;PodCondition
type DiagnosticType string

const (
	DiagnosticTypeContainerWaiting    DiagnosticType = "ContainerWaiting"
	DiagnosticTypeContainerTerminated DiagnosticType = "ContainerTerminated"
	DiagnosticTypePodScheduling       DiagnosticType = "PodScheduling"
	DiagnosticTypePodCondition        DiagnosticType = "PodCondition"
)

// DeploymentStatus represents the status of a deployment check
type DeploymentStatus string

const (
	DeploymentStatusReady       DeploymentStatus = "Ready"
	DeploymentStatusProgressing DeploymentStatus = "Progressing"
	DeploymentStatusFailed      DeploymentStatus = "Failed"
)

// OverallStatus represents the aggregate status of the entire system
type OverallStatus string

const (
	OverallStatusReady    OverallStatus = "Ready"
	OverallStatusNotReady OverallStatus = "NotReady"
)

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
	// +kubebuilder:validation:Required
	// +required
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Default Provider",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	DefaultProvider string `json:"defaultProvider"`
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
	// Proxy settings for connecting to external servers, such as LLM providers.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Proxy Settings",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:advanced"}
	// +kubebuilder:validation:Optional
	ProxyConfig *ProxyConfig `json:"proxyConfig,omitempty"`
	// RAG databases
	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="RAG Databases",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:advanced"}
	RAG []RAGSpec `json:"rag,omitempty"`
	// LLM Token Quota Configuration
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="LLM Token Quota Configuration"
	QuotaHandlersConfig *QuotaHandlersConfig `json:"quotaHandlersConfig,omitempty"`
	// Persistent Storage Configuration
	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Persistent Storage Configuration",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:advanced"}
	Storage *Storage `json:"storage,omitempty"`
	// Only use BYOK RAG sources, ignore the OpenShift documentation RAG
	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Only use BYOK RAG sources",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:booleanSwitch"}
	ByokRAGOnly bool `json:"byokRAGOnly,omitempty"`
	// Custom system prompt for LLM queries. If not specified, uses the default OpenShift Lightspeed prompt.
	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Query System Prompt",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:advanced"}
	QuerySystemPrompt string `json:"querySystemPrompt,omitempty"`
}

// Persistent Storage Configuration
type Storage struct {
	// Size of the requested volume
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Size of the Requested Volume"
	// +kubebuilder:validation:Optional
	Size resource.Quantity `json:"size,omitempty"`
	// Storage class of the requested volume
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Storage Class of the Requested Volume"
	Class string `json:"class,omitempty"`
}

// RAGSpec defines how to retrieve a RAG databases.
type RAGSpec struct {
	// The path to the RAG database inside of the container image
	// +kubebuilder:default:="/rag/vector_db"
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Index Path in the Image"
	IndexPath string `json:"indexPath,omitempty"`
	// The Index ID of the RAG database. Only needed if there are multiple indices in the database.
	// +kubebuilder:default:=""
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Index ID"
	IndexID string `json:"indexID,omitempty"`
	// The URL of the container image to use as a RAG source
	// +kubebuilder:validation:Required
	// +required
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Image"
	Image string `json:"image"`
}

// QuotaHandlersConfig defines the token quota configuration
type QuotaHandlersConfig struct {
	// Token quota limiters
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Token Quota Limiters",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:advanced"}
	LimitersConfig []LimiterConfig `json:"limitersConfig,omitempty"`
	// Enable token history
	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Enable Token History",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:booleanSwitch"}
	EnableTokenHistory bool `json:"enableTokenHistory,omitempty"`
}

// LimiterConfig defines settings for a token quota limiter
type LimiterConfig struct {
	// Name of the limiter
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Limiter Name"
	Name string `json:"name"`
	// Type of the limiter
	// +kubebuilder:validation:Enum=cluster_limiter;user_limiter
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Limiter Type. Accepted Values: cluster_limiter, user_limiter."
	Type string `json:"type"`
	// Initial value of the token quota
	// +kubebuilder:validation:Minimum=0
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Initial Token Quota"
	InitialQuota int `json:"initialQuota"`
	// Token quota increase step
	// +kubebuilder:validation:Minimum=0
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Token Quota Increase Step"
	QuotaIncrease int `json:"quotaIncrease"`
	// Period of time the token quota is for
	// +kubebuilder:validation:Pattern=`^(1\s+(day|month|year|d|m|y)|([2-9][0-9]*|[1-9][0-9]{2,})\s+(days|months|years|d|m|y))$`
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Period of Time the Token Quota Is For"
	Period string `json:"period"`
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
	// Database container settings.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Database Container"
	DatabaseContainer DatabaseContainerConfig `json:"database,omitempty"`
	// MCP server container settings.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="MCP Server Container"
	MCPServerContainer MCPServerContainerConfig `json:"mcpServer,omitempty"`
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

type MCPServerContainerConfig struct {
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

type DatabaseContainerConfig struct {
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Resource Requirements",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:resourceRequirements"}
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Tolerations",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:tolerations"}
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Node Selector",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:nodeSelector"}
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
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
	// Postgres sharedbuffers
	// +kubebuilder:validation:XIntOrString
	// +kubebuilder:default="256MB"
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Shared Buffer Size"
	SharedBuffers string `json:"sharedBuffers,omitempty"`
	// Postgres maxconnections. Default: "2000"
	// +kubebuilder:default=2000
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=262143
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
	// Max tokens for response. The default is 2048 tokens.
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
	// Defines the model's context window size, in tokens. The default is 128k tokens.
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
	Name string `json:"name"`
	// Provider API URL
	// +kubebuilder:validation:Pattern=`^https?://.*$`
	// +operator-sdk:csv:customresourcedefinitions:type=spec,order=2,displayName="URL"
	URL string `json:"url,omitempty"`
	// The name of the secret object that stores API provider credentials
	// +kubebuilder:validation:Required
	// +required
	// +operator-sdk:csv:customresourcedefinitions:type=spec,order=3,displayName="Credential Secret"
	CredentialsSecretRef corev1.LocalObjectReference `json:"credentialsSecretRef"`
	// List of models from the provider
	// +kubebuilder:validation:Required
	// +required
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Models"
	Models []ModelSpec `json:"models"`
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

// ProxyConfig defines the proxy settings for connecting to external servers, such as LLM providers.
type ProxyConfig struct {
	// Proxy URL, e.g. https://proxy.example.com:8080
	// If not specified, the cluster wide proxy will be used, though env var "https_proxy".
	// +kubebuilder:validation:Pattern=`^https?://.*$`
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Proxy URL"
	ProxyURL string `json:"proxyURL,omitempty"`
	// The configmap holding proxy CA certificate
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Proxy CA Certificate"
	ProxyCACertificateRef *corev1.LocalObjectReference `json:"proxyCACertificate,omitempty"`
}

// MCPServer defines the settings for a single MCP server.
type MCPServer struct {
	// Name of the MCP server
	// +kubebuilder:validation:Required
	// +required
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Name"
	Name string `json:"name"`
	// Streamable HTTP Transport settings
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Streamable HTTP Transport"
	StreamableHTTP *MCPServerStreamableHTTPTransport `json:"streamableHTTP,omitempty"`
}

// MCPServerStreamableHTTPTransport configures the MCP server to use streamable HTTP transport.
type MCPServerStreamableHTTPTransport struct {
	// URL of the MCP server
	// +kubebuilder:validation:Required
	// +required
	// +kubebuilder:validation:Pattern=`^https?://.*$`
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="URL"
	URL string `json:"url"`
	// Timeout for the MCP server, default is 5 seconds
	// +kubebuilder:default=5
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Timeout in seconds"
	Timeout int `json:"timeout,omitempty"`
	// SSE Read Timeout, default is 10 seconds
	// +kubebuilder:default=10
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="SSE Read Timeout in seconds"
	SSEReadTimeout int `json:"sseReadTimeout,omitempty"`
	// Headers to send to the MCP server
	// the map contains the header name and the name of the secret with the content of the header. This secret
	// should contain a header path in the data containing a header value.
	// A special case is usage of the kubernetes token in the header. to specify this use
	// a string "kubernetes" instead of the secret name
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Headers"
	Headers map[string]string `json:"headers,omitempty"`
	// Enable Server Sent Events
	// +kubebuilder:default=false
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Enable Server Sent Events"
	EnableSSE bool `json:"enableSSE,omitempty"`
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
