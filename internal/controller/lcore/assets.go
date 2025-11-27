package lcore

import (
	"context"
	"fmt"
	"strings"

	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"

	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// Service account for running LCore server
func GenerateServiceAccount(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.ServiceAccount, error) {
	sa := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.OLSAppServerServiceAccountName,
			Namespace: r.GetNamespace(),
		},
	}

	if err := controllerutil.SetControllerReference(cr, &sa, r.GetScheme()); err != nil {
		return nil, err
	}

	return &sa, nil
}

// SAR = SubjectAccessReview
// SARClusterRole provides permissions for the OLS Application Server to perform
// authentication and authorization checks for users accessing the OLS API.
func GenerateSARClusterRole(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*rbacv1.ClusterRole, error) {
	role := rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: utils.OLSAppServerSARRoleName,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"authorization.k8s.io"},
				Resources: []string{"subjectaccessreviews"},
				Verbs:     []string{"create"},
			},
			{
				APIGroups: []string{"authentication.k8s.io"},
				Resources: []string{"tokenreviews"},
				Verbs:     []string{"create"},
			},
			{
				APIGroups: []string{"config.openshift.io"},
				Resources: []string{"clusterversions"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups:     []string{""},
				Resources:     []string{"secrets"},
				ResourceNames: []string{"pull-secret"},
				Verbs:         []string{"get"},
			},
			{
				NonResourceURLs: []string{"/ls-access"},
				Verbs:           []string{"get"},
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, &role, r.GetScheme()); err != nil {
		return nil, err
	}

	return &role, nil
}

// Binding SARClusterRole to server account
func generateSARClusterRoleBinding(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*rbacv1.ClusterRoleBinding, error) {
	rb := rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: utils.OLSAppServerSARRoleBindingName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      utils.OLSAppServerServiceAccountName,
				Namespace: r.GetNamespace(),
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     utils.OLSAppServerSARRoleName,
		},
	}

	if err := controllerutil.SetControllerReference(cr, &rb, r.GetScheme()); err != nil {
		return nil, err
	}

	return &rb, nil
}

// Llama Stack component builder functions (return maps for maintainability)

func buildLlamaStackCoreConfig(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) map[string]interface{} {
	return map[string]interface{}{
		"version":    "2",
		"image_name": "minimal-viable-llama-stack-configuration",
		// Minimal APIs for RAG + MCP: agents (for MCP), files, inference, safety (required by agents), telemetry, tool_runtime, vector_io
		// Commented out: datasetio, eval, post_training, scoring (not needed for basic RAG + MCP)
		"apis":                   []string{"agents" /* "datasetio", "eval", */, "files", "inference" /* , "post_training", */, "safety" /* , "scoring" */, "telemetry", "tool_runtime", "vector_io"},
		"benchmarks":             []interface{}{},
		"container_image":        nil,
		"datasets":               []interface{}{},
		"external_providers_dir": nil,
		"inference_store": map[string]interface{}{
			"db_path": ".llama/distributions/ollama/inference_store.db",
			"type":    "sqlite",
		},
		"logging": nil,
		"metadata_store": map[string]interface{}{
			"db_path":   ".llama/distributions/ollama/registry.db",
			"namespace": nil,
			"type":      "sqlite",
		},
	}
}

func buildLlamaStackFileProviders(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) []interface{} {
	return []interface{}{
		map[string]interface{}{
			"provider_id":   "localfs",
			"provider_type": "inline::localfs",
			"config": map[string]interface{}{
				"storage_dir": "/tmp/llama-stack-files",
				"metadata_store": map[string]interface{}{
					"type":    "sqlite",
					"db_path": ".llama/distributions/ollama/files_metadata.db",
				},
			},
		},
	}
}

func buildLlamaStackAgentProviders(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) []interface{} {
	return []interface{}{
		map[string]interface{}{
			"provider_id":   "meta-reference",
			"provider_type": "inline::meta-reference",
			"config": map[string]interface{}{
				"persistence_store": map[string]interface{}{
					"db_path":   ".llama/distributions/ollama/agents_store.db",
					"namespace": nil,
					"type":      "sqlite",
				},
				"responses_store": map[string]interface{}{
					"db_path": ".llama/distributions/ollama/responses_store.db",
					"type":    "sqlite",
				},
			},
		},
	}
}

// Commented out - datasetio API not needed for basic RAG + MCP
// Uncomment if you need dataset operations
/*
func buildLlamaStackDatasetIOProviders(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) []interface{} {
	return []interface{}{
		map[string]interface{}{
			"provider_id":   "huggingface",
			"provider_type": "remote::huggingface",
			"config": map[string]interface{}{
				"kvstore": map[string]interface{}{
					"db_path":   ".llama/distributions/ollama/huggingface_datasetio.db",
					"namespace": nil,
					"type":      "sqlite",
				},
			},
		},
		map[string]interface{}{
			"provider_id":   "localfs",
			"provider_type": "inline::localfs",
			"config": map[string]interface{}{
				"kvstore": map[string]interface{}{
					"db_path":   ".llama/distributions/ollama/localfs_datasetio.db",
					"namespace": nil,
					"type":      "sqlite",
				},
			},
		},
	}
}
*/

// Commented out - eval API not needed for basic RAG + MCP
// Uncomment if you need to run evaluations
/*
func buildLlamaStackEvalProviders(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) []interface{} {
	return []interface{}{
		map[string]interface{}{
			"provider_id":   "meta-reference",
			"provider_type": "inline::meta-reference",
			"config": map[string]interface{}{
				"kvstore": map[string]interface{}{
					"db_path":   ".llama/distributions/ollama/meta_reference_eval.db",
					"namespace": nil,
					"type":      "sqlite",
				},
			},
		},
	}
}
*/

func buildLlamaStackInferenceProviders(_ reconciler.Reconciler, _ context.Context, cr *olsv1alpha1.OLSConfig) ([]interface{}, error) {
	providers := []interface{}{
		// Always include sentence-transformers for embeddings
		map[string]interface{}{
			"provider_id":   "sentence-transformers",
			"provider_type": "inline::sentence-transformers",
			"config":        map[string]interface{}{},
		},
	}

	// Add LLM providers from OLSConfig
	for _, provider := range cr.Spec.LLMConfig.Providers {
		providerConfig := map[string]interface{}{
			"provider_id": provider.Name,
		}

		// Convert provider name to valid environment variable name
		envVarName := utils.ProviderNameToEnvVarName(provider.Name)

		// Map OLSConfig provider types to Llama Stack provider types
		// Note: Only providers supported by Llama Stack are included
		switch provider.Type {
		case "openai":
			providerConfig["provider_type"] = "remote::openai"
			config := map[string]interface{}{}

			// Set environment variable name for API key
			// Llama Stack will substitute ${env.VAR_NAME} with the actual env var value
			config["api_key"] = fmt.Sprintf("${env.%s_API_KEY}", envVarName)

			// Add custom URL if specified
			if provider.URL != "" {
				config["url"] = provider.URL
			}
			providerConfig["config"] = config

		case "azure_openai":
			providerConfig["provider_type"] = "remote::azure"
			config := map[string]interface{}{}

			// Azure supports both API key and client credentials authentication
			// Always include api_key (required by LiteLLM's Pydantic validation)
			config["api_key"] = fmt.Sprintf("${env.%s_API_KEY}", envVarName)

			// Also include client credentials fields (will be empty if not using client credentials)
			config["client_id"] = fmt.Sprintf("${env.%s_CLIENT_ID:=}", envVarName)
			config["tenant_id"] = fmt.Sprintf("${env.%s_TENANT_ID:=}", envVarName)
			config["client_secret"] = fmt.Sprintf("${env.%s_CLIENT_SECRET:=}", envVarName)

			// Azure-specific fields
			if provider.AzureDeploymentName != "" {
				config["deployment_name"] = provider.AzureDeploymentName
			}
			if provider.APIVersion != "" {
				config["api_version"] = provider.APIVersion
			}
			if provider.URL != "" {
				config["api_base"] = provider.URL
			}
			providerConfig["config"] = config

		case "watsonx", "rhoai_vllm", "rhelai_vllm", "bam":
			// These providers are not supported by Llama Stack
			// They are handled directly by lightspeed-stack (LCS), not Llama Stack
			return nil, fmt.Errorf("provider type '%s' (provider '%s') is not currently supported by Llama Stack. Supported types: openai, azure_openai", provider.Type, provider.Name)

		default:
			// Unknown provider type
			return nil, fmt.Errorf("unknown provider type '%s' (provider '%s'). Supported types: openai, azure_openai", provider.Type, provider.Name)
		}

		providers = append(providers, providerConfig)
	}

	return providers, nil
}

// Commented out - post_training API not needed for basic RAG + MCP
// Uncomment if you need fine-tuning capabilities
/*
func buildLlamaStackPostTraining(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) []interface{} {
	return []interface{}{
		map[string]interface{}{
			"provider_id":   "huggingface",
			"provider_type": "inline::huggingface-gpu",
			"config": map[string]interface{}{
				"checkpoint_format":   "huggingface",
				"device":              "cpu",
				"distributed_backend": nil,
				"dpo_output_dir":      ".",
			},
		},
	}
}
*/

// Safety API - Required by agents provider (for MCP)
// Note: You can configure excluded_categories if needed
func buildLlamaStackSafety(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) []interface{} {
	return []interface{}{
		map[string]interface{}{
			"provider_id":   "llama-guard",
			"provider_type": "inline::llama-guard",
			"config": map[string]interface{}{
				"excluded_categories": []interface{}{},
			},
		},
	}
}

// Commented out - scoring API not needed for basic RAG + MCP
// Uncomment if you need response scoring capabilities
/*
func buildLlamaStackScoring(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) []interface{} {
	return []interface{}{
		map[string]interface{}{
			"provider_id":   "basic",
			"provider_type": "inline::basic",
			"config":        map[string]interface{}{},
		},
		map[string]interface{}{
			"provider_id":   "llm-as-judge",
			"provider_type": "inline::llm-as-judge",
			"config":        map[string]interface{}{},
		},
		map[string]interface{}{
			"provider_id":   "braintrust",
			"provider_type": "inline::braintrust",
			"config": map[string]interface{}{
				"openai_api_key": "********",
			},
		},
	}
}
*/

func buildLlamaStackTelemetry(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) []interface{} {
	return []interface{}{
		map[string]interface{}{
			"provider_id":   "meta-reference",
			"provider_type": "inline::meta-reference",
			"config": map[string]interface{}{
				"service_name":   "lightspeed-stack-telemetry",
				"sinks":          "sqlite",
				"sqlite_db_path": ".llama/distributions/ollama/trace_store.db",
			},
		},
	}
}

func buildLlamaStackToolRuntime(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) []interface{} {
	return []interface{}{
		map[string]interface{}{
			"provider_id":   "model-context-protocol",
			"provider_type": "remote::model-context-protocol",
			"config":        map[string]interface{}{},
		},
		map[string]interface{}{
			"provider_id":   "rag-runtime",
			"provider_type": "inline::rag-runtime",
			"config":        map[string]interface{}{},
		},
	}
}

func buildLlamaStackVectorDB(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) []interface{} {
	return []interface{}{
		map[string]interface{}{
			"provider_id":   "faiss",
			"provider_type": "inline::faiss",
			"config": map[string]interface{}{
				"kvstore": map[string]interface{}{
					"db_path":   ".llama/distributions/ollama/faiss_store.db",
					"namespace": nil,
					"type":      "sqlite",
				},
			},
		},
	}
}

func buildLlamaStackServerConfig(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) map[string]interface{} {
	return map[string]interface{}{
		"auth":         nil,
		"host":         "0.0.0.0", // Listen on all interfaces so lightspeed-stack container can connect
		"port":         8321,
		"quota":        nil,
		"tls_cafile":   nil,
		"tls_certfile": nil,
		"tls_keyfile":  nil,
	}
}

// Commented out - shields not needed without safety API
// Uncomment if you enable safety API and need shields
/*
func buildLlamaStackShields(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) []interface{} {
	return []interface{}{
		map[string]interface{}{
			"shield_id":          "llama-guard-shield",
			"provider_id":        "llama-guard",
			"provider_shield_id": "gpt-3.5-turbo",
		},
	}
}
*/

func buildLlamaStackVectorDBs(_ reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) []interface{} {
	vectorDBs := []interface{}{}

	// Use RAG configuration from OLSConfig if available
	if len(cr.Spec.OLSConfig.RAG) > 0 {
		for _, rag := range cr.Spec.OLSConfig.RAG {
			vectorDB := map[string]interface{}{
				"embedding_model":     "sentence-transformers/all-mpnet-base-v2",
				"embedding_dimension": 768,
				"provider_id":         "faiss",
			}

			// Use IndexID if specified, otherwise generate a default
			if rag.IndexID != "" {
				vectorDB["vector_db_id"] = rag.IndexID
			} else {
				// Generate a simple ID from the image name
				vectorDB["vector_db_id"] = "rag_" + sanitizeID(rag.Image)
			}

			vectorDBs = append(vectorDBs, vectorDB)
		}
	} else {
		// Default fallback if no RAG configured
		vectorDBs = append(vectorDBs, map[string]interface{}{
			"vector_db_id":        "my_knowledge_base",
			"embedding_model":     "sentence-transformers/all-mpnet-base-v2",
			"embedding_dimension": 768,
			"provider_id":         "faiss",
		})
	}

	return vectorDBs
}

// sanitizeID creates a valid ID from an image name
func sanitizeID(image string) string {
	// Extract just the image name without registry/tag
	// e.g., "quay.io/my-org/my-rag:latest" -> "my-rag"
	parts := strings.Split(image, "/")
	name := parts[len(parts)-1]
	name = strings.Split(name, ":")[0]
	// Replace invalid characters with underscores
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, name)
	return name
}

func buildLlamaStackModels(_ reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) []interface{} {
	models := []interface{}{
		// Always include sentence-transformers embedding model for RAG
		map[string]interface{}{
			"model_id":          "sentence-transformers/all-mpnet-base-v2",
			"model_type":        "embedding",
			"provider_id":       "sentence-transformers",
			"provider_model_id": "sentence-transformers/all-mpnet-base-v2",
			"metadata": map[string]interface{}{
				"embedding_dimension": 768,
			},
		},
	}

	// Add LLM models from OLSConfig providers
	for _, provider := range cr.Spec.LLMConfig.Providers {
		for _, model := range provider.Models {
			modelConfig := map[string]interface{}{
				"model_id":          model.Name,
				"model_type":        "llm",
				"provider_id":       provider.Name,
				"provider_model_id": model.Name,
			}

			// Add model-specific metadata if available
			metadata := map[string]interface{}{}
			if model.ContextWindowSize > 0 {
				metadata["context_window_size"] = model.ContextWindowSize
			}
			if model.Parameters.MaxTokensForResponse > 0 {
				metadata["max_tokens"] = model.Parameters.MaxTokensForResponse
			}
			if model.URL != "" {
				metadata["url"] = model.URL
			}
			if len(metadata) > 0 {
				modelConfig["metadata"] = metadata
			}

			models = append(models, modelConfig)
		}
	}

	return models
}

func buildLlamaStackToolGroups(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) []interface{} {
	return []interface{}{
		map[string]interface{}{
			"toolgroup_id": "builtin::rag",
			"provider_id":  "rag-runtime",
		},
	}
}

// buildLlamaStackYAML assembles the complete Llama Stack configuration and converts to YAML
func buildLlamaStackYAML(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) (string, error) {
	// Build the complete config as a map
	config := buildLlamaStackCoreConfig(r, cr)

	// Build inference providers with error handling
	inferenceProviders, err := buildLlamaStackInferenceProviders(r, ctx, cr)
	if err != nil {
		return "", fmt.Errorf("failed to build inference providers: %w", err)
	}

	// Build providers map - only include providers for enabled APIs
	config["providers"] = map[string]interface{}{
		"files":        buildLlamaStackFileProviders(r, cr),
		"agents":       buildLlamaStackAgentProviders(r, cr), // Required for MCP
		"inference":    inferenceProviders,
		"telemetry":    buildLlamaStackTelemetry(r, cr),
		"safety":       buildLlamaStackSafety(r, cr),      // Required by agents provider
		"tool_runtime": buildLlamaStackToolRuntime(r, cr), // Required for RAG
		"vector_io":    buildLlamaStackVectorDB(r, cr),    // Required for RAG
	}

	// Add top-level fields
	config["scoring_fns"] = []interface{}{} // Keep empty for now
	config["server"] = buildLlamaStackServerConfig(r, cr)
	// config["shields"] = buildLlamaStackShields(r, cr) // Commented out - not needed without safety API
	config["vector_dbs"] = buildLlamaStackVectorDBs(r, cr)
	config["models"] = buildLlamaStackModels(r, cr)
	config["tool_groups"] = buildLlamaStackToolGroups(r, cr)

	// Convert to YAML
	yamlBytes, err := yaml.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Llama Stack config to YAML: %w", err)
	}

	return string(yamlBytes), nil
}

// Generates Llama stack config map
func GenerateLlamaStackConfigMap(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) {
	llamaStackYAML, err := buildLlamaStackYAML(r, ctx, cr)
	if err != nil {
		return nil, fmt.Errorf("failed to build Llama Stack YAML: %w", err)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.LlamaStackConfigCmName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GenerateAppServerSelectorLabels(),
		},
		Data: map[string]string{
			"run.yaml": llamaStackYAML,
		},
	}

	if err := controllerutil.SetControllerReference(cr, cm, r.GetScheme()); err != nil {
		return nil, err
	}

	return cm, nil
}

// LCore Config component builder functions (return maps for maintainability)

func buildLCoreServiceConfig(_ reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) map[string]interface{} {
	// Map LogLevel from OLSConfig
	// Valid values: DEBUG, INFO, WARNING, ERROR, CRITICAL
	// Default to INFO if not specified
	logLevel := "INFO"
	if cr.Spec.OLSConfig.LogLevel != "" {
		logLevel = cr.Spec.OLSConfig.LogLevel
	}

	// color_log: enable colored logs for DEBUG, disable for production (INFO+)
	colorLog := logLevel == "DEBUG"

	return map[string]interface{}{
		"host":         "0.0.0.0",
		"port":         utils.OLSAppServerContainerPort,
		"auth_enabled": false,
		"workers":      1,
		"color_log":    colorLog,
		"access_log":   true,
		// Note: log_level is not a valid field in lightspeed-stack service config
		// The service uses standard Python logging which respects the LOG_LEVEL env var
		"tls_config": map[string]interface{}{
			"tls_certificate_path": "/etc/certs/lightspeed-tls/tls.crt",
			"tls_key_path":         "/etc/certs/lightspeed-tls/tls.key",
		},
	}
}

func buildLCoreLlamaStackConfig(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) map[string]interface{} {
	return map[string]interface{}{
		"use_as_library_client": false,
		"url":                   "http://localhost:8321",
		"api_key":               "xyzzy",
	}
}

func buildLCoreUserDataCollectionConfig(_ reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) map[string]interface{} {
	// Map UserDataCollection from OLSConfig
	// Feedback and transcripts are enabled by default, disabled if specified in CR
	feedbackEnabled := !cr.Spec.OLSConfig.UserDataCollection.FeedbackDisabled
	transcriptsEnabled := !cr.Spec.OLSConfig.UserDataCollection.TranscriptsDisabled

	return map[string]interface{}{
		"feedback_enabled":    feedbackEnabled,
		"feedback_storage":    "/tmp/data/feedback",
		"transcripts_enabled": transcriptsEnabled,
		"transcripts_storage": "/tmp/data/transcripts",
	}
}

func buildLCoreAuthenticationConfig(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) map[string]interface{} {
	return map[string]interface{}{
		"module": "k8s",
	}
}

func buildLCoreInferenceConfig(_ reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) map[string]interface{} {
	return map[string]interface{}{
		"default_provider": cr.Spec.OLSConfig.DefaultProvider,
		"default_model":    cr.Spec.OLSConfig.DefaultModel,
	}
}

// ============================================================================
// Optional Configuration Builders (commented out - uncomment to implement)
// ============================================================================

// buildLCoreDatabaseConfig configures persistent database storage
// Supports SQLite (file-based) or PostgreSQL (server-based)
// Default: SQLite in /tmp (ephemeral)
//
//func buildLCoreDatabaseConfig(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) map[string]interface{} {
//	// Example: SQLite configuration
//	return map[string]interface{}{
//		"sqlite": map[string]interface{}{
//			"db_path": "/app-root/data/lightspeed-stack.db", // Mount a PVC here for persistence
//		},
//	}
//
//	// Example: PostgreSQL configuration (requires PostgreSQL deployment)
//	// return map[string]interface{}{
//	//     "postgres": map[string]interface{}{
//	//         "host":     "postgres-service",
//	//         "port":     5432,
//	//         "db":       "lightspeed",
//	//         "user":     "lightspeed",
//	//         "password": "${POSTGRES_PASSWORD}", // From secret
//	//         "ssl_mode": "require",
//	//     },
//	// }
//}

// buildLCoreMCPServersConfig configures Model Context Protocol servers
// Allows integration with external context providers for agent workflows
//
//func buildLCoreMCPServersConfig(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) []map[string]interface{} {
//	return []map[string]interface{}{
//		{
//			"name":        "server1",
//			"provider_id": "provider1",
//			"url":         "http://url.com:1",
//		},
//		{
//			"name":        "server2",
//			"provider_id": "provider2",
//			"url":         "http://url.com:2",
//		},
//		{
//			"name":        "server3",
//			"provider_id": "provider3",
//			"url":         "http://url.com:3",
//		},
//	}
//}

// buildLCoreAuthorizationConfig configures role-based access control
// Defines which actions different roles can perform
// Actions: query, list_models, list_providers, get_provider, get_metrics, get_config, info, model_override
//
//func buildLCoreAuthorizationConfig(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) map[string]interface{} {
//	return map[string]interface{}{
//		"access_rules": []interface{}{
//			map[string]interface{}{
//				"role": "admin",
//				"actions": []string{
//					"query", "list_models", "list_providers", "get_provider",
//					"get_metrics", "get_config", "info", "model_override",
//				},
//			},
//			map[string]interface{}{
//				"role":    "user",
//				"actions": []string{"query", "info"},
//			},
//		},
//	}
//}

// buildLCoreCustomizationConfig configures system prompt customization
// Allows overriding default system prompts and behavior
//
//func buildLCoreCustomizationConfig(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) map[string]interface{} {
//	return map[string]interface{}{
//		"disable_query_system_prompt": false,
//		"system_prompt":                "You are a helpful OpenShift AI assistant.",
//		// Alternative: Load from file
//		// "system_prompt_path": "/app-root/config/system_prompt.txt",
//		// Or use a custom profile
//		// "profile_path": "/app-root/config/custom_profile.py",
//	}
//}

// buildLCoreConversationCacheConfig configures chat history caching
// Options: noop (disabled), memory (in-memory), sqlite (file), postgres (database)
// Useful for maintaining conversation context across requests
//
//func buildLCoreConversationCacheConfig(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) map[string]interface{} {
//	// Example: In-memory cache
//	return map[string]interface{}{
//		"type": "memory",
//		"memory": map[string]interface{}{
//			"max_entries": 1000,
//		},
//	}
//
//	// Example: SQLite cache (requires persistent volume)
//	// return map[string]interface{}{
//	//     "type": "sqlite",
//	//     "sqlite": map[string]interface{}{
//	//         "db_path": "/app-root/data/conversation_cache.db",
//	//     },
//	// }
//
//	// Example: PostgreSQL cache (requires PostgreSQL deployment)
//	// return map[string]interface{}{
//	//     "type": "postgres",
//	//     "postgres": map[string]interface{}{
//	//         "host": "postgres-service",
//	//         "port": 5432,
//	//         "db":   "conversation_cache",
//	//         "user": "lightspeed",
//	//         "password": "${POSTGRES_PASSWORD}",
//	//     },
//	// }
//}

// buildLCoreByokRagConfig configures Bring Your Own Knowledge RAG sources
// Allows adding custom document collections beyond default RAG databases
// Requires vector database setup and embedding model configuration
//
//func buildLCoreByokRagConfig(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) []interface{} {
//	return []interface{}{
//		map[string]interface{}{
//			"rag_id":              "custom-docs",
//			"rag_type":            "chromadb", // or "pgvector"
//			"embedding_model":     "sentence-transformers/all-mpnet-base-v2",
//			"embedding_dimension": 768,
//			"vector_db_id":        "custom-vectordb",
//			"db_path":             "/app-root/data/custom_rag.db",
//		},
//	}
//}

// buildLCoreQuotaHandlersConfig configures token usage rate limiting
// Controls how many tokens users or clusters can consume
// Useful for cost management and preventing abuse
//
//func buildLCoreQuotaHandlersConfig(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) map[string]interface{} {
//	return map[string]interface{}{
//		"limiters": []interface{}{
//			// Per-user token limit
//			map[string]interface{}{
//				"type":           "user_limiter",
//				"name":           "user_daily_tokens",
//				"initial_quota":  10000,  // 10k tokens to start
//				"quota_increase": 10000,  // Refill 10k tokens
//				"period":         "1 day", // Every day
//			},
//			// Cluster-wide token limit
//			map[string]interface{}{
//				"type":           "cluster_limiter",
//				"name":           "cluster_hourly_tokens",
//				"initial_quota":  100000,   // 100k tokens total
//				"quota_increase": 100000,   // Refill 100k tokens
//				"period":         "1 hour", // Every hour
//			},
//		},
//		"scheduler": map[string]interface{}{
//			"period": 1, // Check quotas every 1 second
//		},
//		"enable_token_history": false, // Set to true to track token usage history
//		// Database configuration for quota storage (optional, uses main database if not specified)
//		// "sqlite": map[string]interface{}{
//		//     "db_path": "/app-root/data/quota.db",
//		// },
//	}
//}

// buildLCoreConfigYAML assembles the complete Lightspeed Core Service configuration and converts to YAML
func buildLCoreConfigYAML(r reconciler.Reconciler, _ context.Context, cr *olsv1alpha1.OLSConfig) (string, error) {
	// Build the complete config as a map
	config := map[string]interface{}{
		"name":                 "Lightspeed Core Service (LCS)",
		"service":              buildLCoreServiceConfig(r, cr),
		"llama_stack":          buildLCoreLlamaStackConfig(r, cr),
		"user_data_collection": buildLCoreUserDataCollectionConfig(r, cr),
		"authentication":       buildLCoreAuthenticationConfig(r, cr),
		"inference":            buildLCoreInferenceConfig(r, cr),

		// Optional features (uncomment to enable):
		// "database":           buildLCoreDatabaseConfig(r, cr),           // Persistent storage (SQLite/PostgreSQL)
		// "mcp_servers":        buildLCoreMCPServersConfig(r, cr),         // Model Context Protocol servers
		// "authorization":      buildLCoreAuthorizationConfig(r, cr),      // Role-based access control
		// "customization":      buildLCoreCustomizationConfig(r, cr),      // System prompt customization
		// "conversation_cache": buildLCoreConversationCacheConfig(r, cr),  // Chat history caching
		// "byok_rag":           buildLCoreByokRagConfig(r, cr),            // Custom RAG sources
		// "quota_handlers":     buildLCoreQuotaHandlersConfig(r, cr),      // Token rate limiting
	}

	// Convert to YAML
	yamlBytes, err := yaml.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal LCore config to YAML: %w", err)
	}

	return string(yamlBytes), nil
}

// Generates LCoreConfigMap using YAML string building (like Llama Stack)
func GenerateLcoreConfigMap(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) {
	// Build OLS config YAML from components
	lcoreConfigYAML, err := buildLCoreConfigYAML(r, ctx, cr)
	if err != nil {
		return nil, fmt.Errorf("failed to build OLS config YAML: %w", err)
	}

	// Create ConfigMap
	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.LCoreConfigCmName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GenerateAppServerSelectorLabels(),
		},
		Data: map[string]string{
			"lightspeed-stack.yaml": lcoreConfigYAML,
		},
	}

	if err := controllerutil.SetControllerReference(cr, &cm, r.GetScheme()); err != nil {
		return nil, err
	}

	return &cm, nil
}

func GenerateService(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.Service, error) {
	annotations := map[string]string{}

	// Let service-ca operator generate a TLS certificate if the user does not provide one
	if cr.Spec.OLSConfig.DeploymentConfig.ConsoleContainer.CAcertificate == "" {
		annotations[utils.ServingCertSecretAnnotationKey] = utils.OLSCertsSecretName
	} else {
		delete(annotations, utils.ServingCertSecretAnnotationKey)
	}

	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        utils.OLSAppServerServiceName,
			Namespace:   r.GetNamespace(),
			Labels:      utils.GenerateAppServerSelectorLabels(),
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "https",
					Protocol:   corev1.ProtocolTCP,
					Port:       utils.OLSAppServerServicePort,
					TargetPort: intstr.Parse("https"),
				},
			},
			Selector: utils.GenerateAppServerSelectorLabels(),
		},
	}
	if err := controllerutil.SetControllerReference(cr, &service, r.GetScheme()); err != nil {
		return nil, err
	}

	return &service, nil
}

func GenerateServiceMonitor(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*monv1.ServiceMonitor, error) {
	metaLabels := utils.GenerateAppServerSelectorLabels()
	metaLabels["monitoring.openshift.io/collection-profile"] = "full"
	metaLabels["app.kubernetes.io/component"] = "metrics"
	metaLabels["openshift.io/user-monitoring"] = "true"

	valFalse := false
	serverName := strings.Join([]string{utils.OLSAppServerServiceName, r.GetNamespace(), "svc"}, ".")

	serviceMonitor := monv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.AppServerServiceMonitorName,
			Namespace: r.GetNamespace(),
			Labels:    metaLabels,
		},
		Spec: monv1.ServiceMonitorSpec{
			Endpoints: []monv1.Endpoint{
				{
					Port:     "https",
					Path:     utils.AppServerMetricsPath,
					Interval: "30s",
					Scheme:   "https",
					TLSConfig: &monv1.TLSConfig{
						CAFile:   "/etc/prometheus/configmaps/serving-certs-ca-bundle/service-ca.crt",
						CertFile: "/etc/prometheus/secrets/metrics-client-certs/tls.crt",
						KeyFile:  "/etc/prometheus/secrets/metrics-client-certs/tls.key",
						SafeTLSConfig: monv1.SafeTLSConfig{
							InsecureSkipVerify: &valFalse,
							ServerName:         &serverName,
						},
					},
					Authorization: &monv1.SafeAuthorization{
						Type: "Bearer",
						Credentials: &corev1.SecretKeySelector{
							Key: "token",
							LocalObjectReference: corev1.LocalObjectReference{
								Name: utils.MetricsReaderServiceAccountTokenSecretName,
							},
						},
					},
				},
			},
			JobLabel: "app.kubernetes.io/name",
			Selector: metav1.LabelSelector{
				MatchLabels: utils.GenerateAppServerSelectorLabels(),
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, &serviceMonitor, r.GetScheme()); err != nil {
		return nil, err
	}

	return &serviceMonitor, nil
}

func GeneratePrometheusRule(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*monv1.PrometheusRule, error) {
	metaLabels := utils.GenerateAppServerSelectorLabels()
	metaLabels["app.kubernetes.io/component"] = "metrics"

	rule := monv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.AppServerPrometheusRuleName,
			Namespace: r.GetNamespace(),
			Labels:    metaLabels,
		},
		Spec: monv1.PrometheusRuleSpec{
			Groups: []monv1.RuleGroup{
				{
					Name: "ols.operations.rules",
					Rules: []monv1.Rule{
						{
							Record: "ols:rest_api_query_calls_total:2xx",
							Expr:   intstr.FromString("sum by(status_code) (ols_rest_api_calls_total{path=\"/v1/streaming_query\",status_code=~\"2..\"})"),
							Labels: map[string]string{"status_code": "2xx"},
						},
						{
							Record: "ols:rest_api_query_calls_total:4xx",
							Expr:   intstr.FromString("sum by(status_code) (ols_rest_api_calls_total{path=\"/v1/streaming_query\",status_code=~\"4..\"})"),
							Labels: map[string]string{"status_code": "4xx"},
						},
						{
							Record: "ols:rest_api_query_calls_total:5xx",
							Expr:   intstr.FromString("sum by(status_code) (ols_rest_api_calls_total{path=\"/v1/streaming_query\",status_code=~\"5..\"})"),
							Labels: map[string]string{"status_code": "5xx"},
						},
						{
							Record: "ols:provider_model_configuration",
							Expr:   intstr.FromString("max by (provider,model) (ols_provider_model_configuration)"),
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, &rule, r.GetScheme()); err != nil {
		return nil, err
	}

	return &rule, nil
}

func GenerateAppServerNetworkPolicy(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*networkingv1.NetworkPolicy, error) {
	np := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.OLSAppServerNetworkPolicyName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GenerateAppServerSelectorLabels(),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: utils.GenerateAppServerSelectorLabels(),
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					// allow prometheus to scrape metrics (both cluster and user-workload monitoring)
					From: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "app.kubernetes.io/name",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{"prometheus"},
									},
									{
										Key:      "prometheus",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{"k8s", "user-workload"},
									},
								},
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "kubernetes.io/metadata.name",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{"openshift-monitoring", "openshift-user-workload-monitoring"},
									},
								},
							},
						},
					},

					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
							Port:     &[]intstr.IntOrString{intstr.FromInt(utils.OLSAppServerContainerPort)}[0],
						},
					},
				},
				{
					// allow the console to access the API
					From: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "console",
								},
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": "openshift-console",
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
							Port:     &[]intstr.IntOrString{intstr.FromInt(utils.OLSAppServerContainerPort)}[0],
						},
					},
				},
				{
					// allow ingress controller to access the API
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"network.openshift.io/policy-group": "ingress",
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
							Port:     &[]intstr.IntOrString{intstr.FromInt(utils.OLSAppServerContainerPort)}[0],
						},
					},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, &np, r.GetScheme()); err != nil {
		return nil, err
	}

	return &np, nil
}

func GenerateMetricsReaderSecret(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.Secret, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.MetricsReaderServiceAccountTokenSecretName,
			Namespace: r.GetNamespace(),
			Annotations: map[string]string{
				"kubernetes.io/service-account.name": utils.MetricsReaderServiceAccountName,
			},
			Labels: map[string]string{
				"app.kubernetes.io/name":      "service-account-token",
				"app.kubernetes.io/component": "metrics",
				"app.kubernetes.io/part-of":   "lightspeed-operator",
			},
		},
		Type: corev1.SecretTypeServiceAccountToken,
	}

	if err := controllerutil.SetControllerReference(cr, secret, r.GetScheme()); err != nil {
		return nil, err
	}

	return secret, nil
}

// checkLLMCredentials validates that all LLM provider credentials are present and valid
func checkLLMCredentials(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	for _, provider := range cr.Spec.LLMConfig.Providers {
		if provider.CredentialsSecretRef.Name == "" {
			return fmt.Errorf("provider %s missing credentials secret", provider.Name)
		}
		secret := &corev1.Secret{}
		err := r.Get(ctx, client.ObjectKey{Name: provider.CredentialsSecretRef.Name, Namespace: r.GetNamespace()}, secret)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return fmt.Errorf("LLM provider %s credential secret %s not found", provider.Name, provider.CredentialsSecretRef.Name)
			}
			return fmt.Errorf("failed to get LLM provider %s credential secret %s: %w", provider.Name, provider.CredentialsSecretRef.Name, err)
		}
		if provider.Type == utils.AzureOpenAIType {
			// Azure OpenAI secret must contain "apitoken" or 3 keys named "client_id", "tenant_id", "client_secret"
			if _, ok := secret.Data["apitoken"]; ok {
				continue
			}
			for _, key := range []string{"client_id", "tenant_id", "client_secret"} {
				if _, ok := secret.Data[key]; !ok {
					return fmt.Errorf("LLM provider %s credential secret %s missing key '%s'", provider.Name, provider.CredentialsSecretRef.Name, key)
				}
			}
		} else {
			// Other providers (e.g. WatsonX, OpenAI) must contain a key named "apitoken"
			if _, ok := secret.Data["apitoken"]; !ok {
				return fmt.Errorf("LLM provider %s credential secret %s missing key 'apitoken'", provider.Name, provider.CredentialsSecretRef.Name)
			}
		}
	}
	return nil
}
