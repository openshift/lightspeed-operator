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

// olsConfigOptions holds configuration options for generating OLSConfig
type olsConfigOptions struct {
	replicas          int32
	sharedBuffers     string
	maxConnections    int
	logLevel          olsv1alpha1.LogLevel
	multiProvider     bool
	apiResources      *corev1.ResourceRequirements
	sidecarResources  *corev1.ResourceRequirements
	databaseResources *corev1.ResourceRequirements
}

// generateBaseOLSConfig creates a base OLSConfig with common settings and applies custom options
func generateBaseOLSConfig(opts olsConfigOptions, customizer func(*olsv1alpha1.OLSConfig)) (*olsv1alpha1.OLSConfig, error) { // nolint:unused
	llmProvider := os.Getenv(LLMProviderEnvVar)
	if llmProvider == "" {
		llmProvider = LLMDefaultProvider
	}
	llmModel := os.Getenv(LLMModelEnvVar)
	if llmModel == "" {
		llmModel = OpenAIDefaultModel
	}

	// Create base provider configuration
	baseProvider := olsv1alpha1.ProviderSpec{
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
	}

	// Add Azure-specific fields if needed
	if llmProvider == "azure_openai" {
		baseProvider.AzureDeploymentName = llmModel
		baseProvider.URL = AzureURL
	}

	// Build providers list
	providers := []olsv1alpha1.ProviderSpec{baseProvider}

	// Add second provider if multi-provider is enabled
	if opts.multiProvider {
		// Add second model to first provider
		providers[0].Models = append(providers[0].Models, olsv1alpha1.ModelSpec{
			Name: OpenAIAlternativeModel,
		})

		secondProvider := olsv1alpha1.ProviderSpec{
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
		}
		providers = append(providers, secondProvider)
	}

	// Create base deployment config
	deploymentConfig := olsv1alpha1.DeploymentConfig{
		APIContainer: olsv1alpha1.Config{
			Replicas:  &opts.replicas,
			Resources: opts.apiResources,
		},
	}

	// Add sidecar and database resources if specified
	if opts.sidecarResources != nil {
		deploymentConfig.DataCollectorContainer = olsv1alpha1.ContainerConfig{
			Resources: opts.sidecarResources,
		}
		deploymentConfig.MCPServerContainer = olsv1alpha1.ContainerConfig{
			Resources: opts.sidecarResources,
		}
		deploymentConfig.ConsoleContainer = olsv1alpha1.Config{
			Resources: opts.sidecarResources,
		}
	}
	if opts.databaseResources != nil {
		deploymentConfig.DatabaseContainer = olsv1alpha1.Config{
			Resources: opts.databaseResources,
		}
	}

	config := &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: OLSCRName,
		},
		Spec: olsv1alpha1.OLSConfigSpec{
			LLMConfig: olsv1alpha1.LLMSpec{
				Providers: providers,
			},
			OLSConfig: olsv1alpha1.OLSSpec{
				ConversationCache: olsv1alpha1.ConversationCacheSpec{
					Type: olsv1alpha1.Postgres,
					Postgres: olsv1alpha1.PostgresSpec{
						SharedBuffers:  opts.sharedBuffers,
						MaxConnections: opts.maxConnections,
					},
				},
				DefaultModel:     llmModel,
				DefaultProvider:  llmProvider,
				LogLevel:         opts.logLevel,
				DeploymentConfig: deploymentConfig,
			},
		},
	}

	// Apply customizations
	if customizer != nil {
		customizer(config)
	}

	return config, nil
}

func generateOLSConfig() (*olsv1alpha1.OLSConfig, error) { // nolint:unused
	opts := olsConfigOptions{
		replicas:       1,
		sharedBuffers:  "256MB",
		maxConnections: 2000,
		logLevel:       olsv1alpha1.LogLevelInfo,
		multiProvider:  false,
	}
	return generateBaseOLSConfig(opts, nil)
}

// generateAllFeaturesOLSConfig generates an OLSConfig with ALL features enabled for comprehensive testing
func generateAllFeaturesOLSConfig() (*olsv1alpha1.OLSConfig, error) { // nolint:unused
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

	opts := olsConfigOptions{
		replicas:          2, // Test with 2 replicas
		sharedBuffers:     "512MB",
		maxConnections:    3000,
		logLevel:          olsv1alpha1.LogLevelDebug,
		multiProvider:     true,
		apiResources:      apiResources,
		sidecarResources:  sidecarResources,
		databaseResources: databaseResources,
	}

	return generateBaseOLSConfig(opts, func(config *olsv1alpha1.OLSConfig) {
		// Add all additional features not in base config
		config.Spec.OLSConfig.IntrospectionEnabled = true
		config.Spec.OLSConfig.ByokRAGOnly = true
		config.Spec.OLSConfig.QuerySystemPrompt = "You are a comprehensive test assistant for OpenShift."
		config.Spec.OLSConfig.MaxIterations = 10

		// User data collection settings
		config.Spec.OLSConfig.UserDataCollection = olsv1alpha1.UserDataCollectionSpec{
			FeedbackDisabled:    true,
			TranscriptsDisabled: false,
		}

		// Query filters
		config.Spec.OLSConfig.QueryFilters = []olsv1alpha1.QueryFiltersSpec{
			{
				Name:        "test-filter",
				Pattern:     "oldterm",
				ReplaceWith: "newterm",
			},
		}

		// Quota handlers configuration
		config.Spec.OLSConfig.QuotaHandlersConfig = &olsv1alpha1.QuotaHandlersConfig{
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
		}

		// Tool filtering configuration
		config.Spec.OLSConfig.ToolFilteringConfig = &olsv1alpha1.ToolFilteringConfig{
			Alpha:     0.75,
			TopK:      15,
			Threshold: 0.05,
		}

		// Tools approval configuration
		config.Spec.OLSConfig.ToolsApprovalConfig = &olsv1alpha1.ToolsApprovalConfig{
			ApprovalType:    olsv1alpha1.ApprovalTypeNever,
			ApprovalTimeout: 300,
		}

		// Storage configuration
		config.Spec.OLSConfig.Storage = &olsv1alpha1.Storage{
			Size: resource.MustParse("1Gi"),
		}

		// Proxy configuration
		config.Spec.OLSConfig.ProxyConfig = &olsv1alpha1.ProxyConfig{
			ProxyURL: "https://squid-service.openshift-lightspeed.svc.cluster.local:3349",
			ProxyCACertificateRef: &olsv1alpha1.ProxyCACertConfigMapRef{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "proxy-ca",
				},
			},
		}

		// Additional CA certificates
		config.Spec.OLSConfig.AdditionalCAConfigMapRef = &corev1.LocalObjectReference{
			Name: "additional-ca-certs",
		}

		// RAG configuration (will be set to BYOK image in test)
		config.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{
			{
				Image:     "image-registry.openshift-image-registry.svc:5000/openshift-lightspeed/assisted-installer-guide:latest",
				IndexPath: "/rag/vector_db",
				IndexID:   "",
			},
		}

		// Image pull secrets for BYOK
		config.Spec.OLSConfig.ImagePullSecrets = []corev1.LocalObjectReference{
			{
				Name: "byok-pull-secret",
			},
		}

		// OLS Data Collector configuration
		config.Spec.OLSDataCollectorConfig = olsv1alpha1.OLSDataCollectorSpec{
			LogLevel: olsv1alpha1.LogLevelDebug,
		}

		// MCP Servers configuration
		config.Spec.MCPServers = []olsv1alpha1.MCPServerConfig{
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
		}

		// Feature gates
		config.Spec.FeatureGates = []olsv1alpha1.FeatureGate{
			"MCPServer",
			"ToolFiltering",
		}
	})
}
