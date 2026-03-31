package e2e

import (
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

func generateLLMTokenSecret(name string) (*corev1.Secret, error) { // nolint:unused
	token := os.Getenv(LLMTokenEnvVar)
	var tenantID = os.Getenv(AzureTenantID)
	var clientID = os.Getenv(AzureClientID)
	var clientSecret = os.Getenv(AzureClientSecret)
	if token == "" {
		return nil, fmt.Errorf("LLM token not found in $%s", LLMTokenEnvVar)
	}
	if tenantID == "" {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: OLSNameSpace,
			},
			StringData: map[string]string{
				LLMApiTokenFileName: token,
			},
		}, nil
	} else {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: OLSNameSpace,
			},
			StringData: map[string]string{
				AzureOpenaiClientID:     clientID,
				AzureOpenaiTenantID:     tenantID,
				AzureOpenaiClientSecret: clientSecret,
			},
		}, nil
	}
}

func generateOLSConfig() (*olsv1alpha1.OLSConfig, error) { // nolint:unused
	llmProvider := os.Getenv(LLMProviderEnvVar)
	if llmProvider == "" {
		llmProvider = LLMDefaultProvider
	}
	llmModel := os.Getenv(LLMModelEnvVar)
	if llmModel == "" {
		llmModel = OpenAIDefaultModel
	}
	replicas := int32(1)
	if llmProvider == "azure_openai" {
		return &olsv1alpha1.OLSConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: OLSCRName,
			},
			Spec: olsv1alpha1.OLSConfigSpec{
				LLMConfig: olsv1alpha1.LLMSpec{
					Providers: []olsv1alpha1.ProviderSpec{
						{
							Name: llmProvider,
							Models: []olsv1alpha1.ModelSpec{
								{
									Name: llmModel,
								},
							},
							CredentialsSecretRef: corev1.LocalObjectReference{
								Name: LLMTokenFirstSecretName,
							},
							Type:                llmProvider,
							AzureDeploymentName: llmModel,
							URL:                 AzureURL,
						},
					},
				},
				OLSConfig: olsv1alpha1.OLSSpec{
					ConversationCache: olsv1alpha1.ConversationCacheSpec{
						Type: olsv1alpha1.Postgres,
						Postgres: olsv1alpha1.PostgresSpec{
							SharedBuffers:  "256MB",
							MaxConnections: 2000,
						},
					},
					DefaultModel:    llmModel,
					DefaultProvider: llmProvider,
					LogLevel:        olsv1alpha1.LogLevelInfo,
					DeploymentConfig: olsv1alpha1.DeploymentConfig{
						APIContainer: olsv1alpha1.Config{
							Replicas: &replicas,
						},
					},
				},
			},
		}, nil
	}
	return &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: OLSCRName,
		},
		Spec: olsv1alpha1.OLSConfigSpec{
			LLMConfig: olsv1alpha1.LLMSpec{
				Providers: []olsv1alpha1.ProviderSpec{
					{
						Name: llmProvider,
						Models: []olsv1alpha1.ModelSpec{
							{
								Name: llmModel,
							},
						},
						CredentialsSecretRef: corev1.LocalObjectReference{
							Name: LLMTokenFirstSecretName,
						},
						Type: llmProvider,
					},
				},
			},
			OLSConfig: olsv1alpha1.OLSSpec{
				ConversationCache: olsv1alpha1.ConversationCacheSpec{
					Type: olsv1alpha1.Postgres,
					Postgres: olsv1alpha1.PostgresSpec{
						SharedBuffers:  "256MB",
						MaxConnections: 2000,
					},
				},
				DefaultModel:    llmModel,
				DefaultProvider: llmProvider,
				LogLevel:        olsv1alpha1.LogLevelInfo,
				DeploymentConfig: olsv1alpha1.DeploymentConfig{
					APIContainer: olsv1alpha1.Config{
						Replicas: &replicas,
					},
				},
			},
		},
	}, nil

}

// generateAllFeaturesOLSConfig generates an OLSConfig with ALL features enabled for comprehensive testing
func generateAllFeaturesOLSConfig() (*olsv1alpha1.OLSConfig, error) { // nolint:unused
	llmProvider := os.Getenv(LLMProviderEnvVar)
	if llmProvider == "" {
		llmProvider = LLMDefaultProvider
	}
	llmModel := os.Getenv(LLMModelEnvVar)
	if llmModel == "" {
		llmModel = OpenAIDefaultModel
	}

	replicas := int32(2) // Test with 2 replicas

	// Resource limits for containers
	apiResources := &corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1000m"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("200m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
	}

	sidecarResources := &corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
	}

	databaseResources := &corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1000m"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("200m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
	}

	return &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: OLSCRName,
		},
		Spec: olsv1alpha1.OLSConfigSpec{
			// LLM Configuration - two providers with two models
			LLMConfig: olsv1alpha1.LLMSpec{
				Providers: []olsv1alpha1.ProviderSpec{
					{
						Name: llmProvider,
						Models: []olsv1alpha1.ModelSpec{
							{
								Name: llmModel,
							},
							{
								Name: OpenAIAlternativeModel,
							},
						},
						CredentialsSecretRef: corev1.LocalObjectReference{
							Name: LLMTokenFirstSecretName,
						},
						Type: llmProvider,
					},
					{
						Name: llmProvider + "-alt",
						Models: []olsv1alpha1.ModelSpec{
							{
								Name: llmModel,
							},
						},
						CredentialsSecretRef: corev1.LocalObjectReference{
							Name: LLMTokenSecondSecretName,
						},
						Type: llmProvider,
					},
				},
			},
			// OLS Configuration with ALL features
			OLSConfig: olsv1alpha1.OLSSpec{
				// Conversation cache with custom postgres settings
				ConversationCache: olsv1alpha1.ConversationCacheSpec{
					Type: olsv1alpha1.Postgres,
					Postgres: olsv1alpha1.PostgresSpec{
						SharedBuffers:  "512MB",
						MaxConnections: 3000,
					},
				},
				DefaultModel:    llmModel,
				DefaultProvider: llmProvider,
				LogLevel:        olsv1alpha1.LogLevelDebug,

				// NEW: Introspection for tool calling
				IntrospectionEnabled: true,

				// NEW: BYOK RAG only mode
				ByokRAGOnly: true,

				// NEW: Custom system prompt
				QuerySystemPrompt: "You are a comprehensive test assistant for OpenShift.",

				// NEW: Custom max iterations
				MaxIterations: 10,

				// NEW: User data collection settings
				UserDataCollection: olsv1alpha1.UserDataCollectionSpec{
					FeedbackDisabled:    true,
					TranscriptsDisabled: false,
				},

				// NEW: Query filters
				QueryFilters: []olsv1alpha1.QueryFiltersSpec{
					{
						Name:        "test-filter",
						Pattern:     "oldterm",
						ReplaceWith: "newterm",
					},
				},

				// NEW: Quota handlers configuration
				QuotaHandlersConfig: &olsv1alpha1.QuotaHandlersConfig{
					EnableTokenHistory: true,
					LimitersConfig: []olsv1alpha1.LimiterConfig{
						{
							Name:          "user-quota",
							Type:          "user_limiter",
							InitialQuota:  50000, // Increased to accommodate multiple test queries
							QuotaIncrease: 10000,
							Period:        "1 hour",
						},
						{
							Name:          "cluster-quota",
							Type:          "cluster_limiter",
							InitialQuota:  500000, // Increased to accommodate multiple test queries
							QuotaIncrease: 50000,
							Period:        "1 day",
						},
					},
				},

				// NEW: Tool filtering configuration
				ToolFilteringConfig: &olsv1alpha1.ToolFilteringConfig{
					Alpha:     0.75,
					TopK:      15,
					Threshold: 0.05,
				},

				// NEW: Tools approval configuration
				ToolsApprovalConfig: &olsv1alpha1.ToolsApprovalConfig{
					ApprovalType:    olsv1alpha1.ApprovalTypeNever,
					ApprovalTimeout: 300,
				},

				// Storage configuration
				Storage: &olsv1alpha1.Storage{
					Size: resource.MustParse("1Gi"),
				},

				// Proxy configuration
				ProxyConfig: &olsv1alpha1.ProxyConfig{
					ProxyURL: "https://squid-service.openshift-lightspeed.svc.cluster.local:3349",
					ProxyCACertificateRef: &corev1.LocalObjectReference{
						Name: "proxy-ca",
					},
				},

				// Additional CA certificates
				AdditionalCAConfigMapRef: &corev1.LocalObjectReference{
					Name: "additional-ca-certs",
				},

				// RAG configuration (will be set to BYOK image in test)
				RAG: []olsv1alpha1.RAGSpec{
					{
						Image:     "image-registry.openshift-image-registry.svc:5000/openshift-lightspeed/assisted-installer-guide:latest",
						IndexPath: "/rag/vector_db",
						IndexID:   "",
					},
				},

				// Image pull secrets for BYOK
				ImagePullSecrets: []corev1.LocalObjectReference{
					{
						Name: "byok-pull-secret",
					},
				},

				// Deployment configuration with resource limits
				DeploymentConfig: olsv1alpha1.DeploymentConfig{
					APIContainer: olsv1alpha1.Config{
						Replicas:  &replicas,
						Resources: apiResources,
					},
					DataCollectorContainer: olsv1alpha1.ContainerConfig{
						Resources: sidecarResources,
					},
					MCPServerContainer: olsv1alpha1.ContainerConfig{
						Resources: sidecarResources,
					},
					ConsoleContainer: olsv1alpha1.Config{
						Resources: sidecarResources,
					},
					DatabaseContainer: olsv1alpha1.Config{
						Resources: databaseResources,
					},
				},
			},

			// NEW: OLS Data Collector configuration
			OLSDataCollectorConfig: olsv1alpha1.OLSDataCollectorSpec{
				LogLevel: olsv1alpha1.LogLevelDebug,
			},

			// NEW: MCP Servers configuration
			MCPServers: []olsv1alpha1.MCPServerConfig{
				{
					Name:    "test-mcp-server",
					URL:     "https://mcp-test.example.com",
					Timeout: 10,
					Headers: []olsv1alpha1.MCPHeader{
						{
							Name: "Authorization",
							ValueFrom: olsv1alpha1.MCPHeaderValueSource{
								Type: olsv1alpha1.MCPHeaderSourceTypeSecret,
								SecretRef: &corev1.LocalObjectReference{
									Name: "mcp-auth-secret",
								},
							},
						},
					},
				},
			},

			// NEW: Feature gates
			FeatureGates: []olsv1alpha1.FeatureGate{
				"MCPServer",
				"ToolFiltering",
			},
		},
	}, nil
}
