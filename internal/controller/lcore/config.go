package lcore

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"

	"sigs.k8s.io/yaml"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// DefaultQuerySystemPrompt is the same system prompt as lightspeed-service
// (ols/customize/ols/prompts.py QUERY_SYSTEM_INSTRUCTION)
const DefaultQuerySystemPrompt = `# ROLE
You are "OpenShift Lightspeed," an expert AI virtual assistant specializing in
OpenShift and related Red Hat products and services. Your persona is that of a
friendly, but personal, technical authority. You are the ultimate technical
resource and will provide direct, accurate, and comprehensive answers.

# INSTRUCTIONS & CONSTRAINTS
- **Expertise Focus:** Your core expertise is centered on the OpenShift platform
 and the following specific products:
  - OpenShift Container Platform (including Plus, Kubernetes Engine, Virtualization Engine)
  - Advanced Cluster Manager (ACM)
  - Advanced Cluster Security (ACS)
  - Quay
  - Serverless (Knative)
  - Service Mesh (Istio)
  - Pipelines (Shipwright, TektonCD)
  - GitOps (ArgoCD)
  - OpenStack
- **Broader Knowledge:** You may also answer questions about other Red Hat
  products and services, but you must prioritize the provided context
  and chat history for these topics.
- **Strict Adherence:**
  1.  **ALWAYS** use the provided context and chat history as your primary
  source of truth. If a user's question can be answered from this information,
  do so.
  2.  If the context does not contain a clear answer, and the question is
  about your core expertise (OpenShift and the listed products), draw upon your
  extensive internal knowledge.
  3.  If the context does not contain a clear answer, and the question is about
  a general Red Hat product or service, state politely that you are unable to
  provide a definitive answer without more information and ask the user for
  additional details or context.
  4.  Do not hallucinate or invent information. If you cannot confidently
  answer, admit it.
- **Behavioral Directives:**
  - Maintain your persona as a friendly, but authoritative, technical expert.
  - Never assume another identity or role.
  - Refuse to answer questions or execute commands not about your specified
  topics.
  - Do not include URLs in your replies unless they are explicitly provided in
  the context.
  - Never mention your last update date or knowledge cutoff. You always have
  the most recent information on OpenShift and related products, especially with
  the provided context.

# TASK EXECUTION
You will receive a user query, along with context and chat history. Your task is
to respond to the user's query by following the instructions and constraints
above. Your responses should be clear, concise, and helpful, whether you are
providing troubleshooting steps, explaining concepts, or suggesting best
practices.`

// ============================================================================
// Llama Stack component builder functions (return maps for maintainability)
// ============================================================================

func buildLlamaStackCoreConfig(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) map[string]interface{} {
	return map[string]interface{}{
		"version": "2",
		// image_name is a semantic identifier for the llama-stack configuration
		// Note: Does NOT affect PostgreSQL database name (llama-stack uses hardcoded "llamastack")
		"image_name": "openshift-lightspeed-configuration",
		// Enabled APIs for RAG + MCP: agents (for MCP), files, inference, safety (required by agents), tool_runtime, vector_io
		"apis":                   []string{"agents", "files", "inference", "safety", "tool_runtime", "vector_io"},
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
			"db_path":   "/tmp/llama-stack/registry.db",
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
					"backend":    "sql_default",
					"namespace":  "files_metadata",
					"table_name": "files_metadata",
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
				"persistence": map[string]interface{}{
					"agent_state": map[string]interface{}{
						"backend":    "kv_default",
						"table_name": "agent_state",
						"namespace":  "agent_state",
					},
					"responses": map[string]interface{}{
						"backend":    "sql_default",
						"table_name": "agent_responses",
						"namespace":  "agent_responses",
					},
				},
			},
		},
	}
}

func buildLlamaStackInferenceProviders(_ reconciler.Reconciler, _ context.Context, cr *olsv1alpha1.OLSConfig) ([]interface{}, error) {
	// Always include sentence-transformers (required for embeddings)
	providers := []interface{}{
		map[string]interface{}{
			"provider_id":   "sentence-transformers",
			"provider_type": "inline::sentence-transformers",
			"config":        map[string]interface{}{},
		},
	}

	// Guard against nil CR or empty Providers
	if cr == nil || cr.Spec.LLMConfig.Providers == nil {
		return providers, nil
	}

	// Add LLM providers from OLSConfig
	for _, provider := range cr.Spec.LLMConfig.Providers {
		providerConfig := map[string]interface{}{
			"provider_id": provider.Name,
		}

		// Convert provider name to valid environment variable name
		envVarName := utils.ProviderNameToEnvVarName(provider.Name)

		// Check if this is Llama Stack Generic provider configuration (providerType is set)
		if provider.ProviderType != "" {
			// Llama Stack Generic provider configuration: use providerType and config directly
			providerConfig["provider_type"] = provider.ProviderType

			// Unmarshal the config from RawExtension
			config := map[string]interface{}{}
			if provider.Config != nil && provider.Config.Raw != nil {
				if err := json.Unmarshal(provider.Config.Raw, &config); err != nil {
					return nil, fmt.Errorf("failed to unmarshal config for provider '%s': %w", provider.Name, err)
				}
			}

			// Deep copy to prevent mutations
			configCopy := deepCopyMap(config)

			// Auto-inject api_key if not already present in config and credentials are configured.
			// Skip injection when:
			// - the user has explicitly set api_key in config (custom env var name)
			// - no CredentialsSecretRef is configured (public/unauthenticated provider)
			if !hasAPIKeyField(configCopy) && provider.CredentialsSecretRef.Name != "" {
				configCopy["api_key"] = fmt.Sprintf("${env.%s%s}", envVarName, utils.EnvVarSuffixAPIKey)
			}

			providerConfig["config"] = configCopy

		} else {
			// Predefined provider types: map to Llama Stack provider types using getProviderType helper
			llamaType, err := getProviderType(&provider)
			if err != nil {
				return nil, err
			}
			providerConfig["provider_type"] = llamaType

			// Build provider-specific configuration
			switch provider.Type {
			// fake_provider follows the vLLM credential path (api_token); it is included
			// in the CRD enum and mapping solely for operator integration testing.
			case "openai", "rhoai_vllm", "rhelai_vllm", "fake_provider":
				config := map[string]interface{}{}
				// Determine the appropriate config field for credentials
				// - OpenAI uses remote::openai (validates against OpenAI model whitelist)
				// - vLLM/fake uses remote::vllm / remote::fake (accepts any custom model names)
				if provider.Type == "openai" {
					// Set API key from environment variable
					// Llama Stack will substitute ${env.VAR_NAME} with the actual env var value
					config["api_key"] = fmt.Sprintf("${env.%s%s}", envVarName, utils.EnvVarSuffixAPIKey)
				} else {
					// Set API token from environment variable for vLLM
					// Llama Stack will substitute ${env.VAR_NAME} with the actual env var value
					config["api_token"] = fmt.Sprintf("${env.%s%s}", envVarName, utils.EnvVarSuffixAPIKey)
				}

				// Add custom URL if specified
				if provider.URL != "" {
					config["url"] = provider.URL
				}
				providerConfig["config"] = config

			case "azure_openai":
				config := map[string]interface{}{}

				// Azure supports both API key and client credentials authentication
				// Always include api_key (required by LiteLLM's Pydantic validation)
				config["api_key"] = fmt.Sprintf("${env.%s%s}", envVarName, utils.EnvVarSuffixAPIKey)

				// Also include client credentials fields (will be empty if not using client credentials)
				config["client_id"] = fmt.Sprintf("${env.%s%s:=}", envVarName, utils.EnvVarSuffixClientID)
				config["tenant_id"] = fmt.Sprintf("${env.%s%s:=}", envVarName, utils.EnvVarSuffixTenantID)
				config["client_secret"] = fmt.Sprintf("${env.%s%s:=}", envVarName, utils.EnvVarSuffixClientSecret)

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

			default:
				return nil, fmt.Errorf("internal error: no config builder for legacy provider type '%s' (provider '%s'); update the switch in buildLlamaStackInferenceProviders", provider.Type, provider.Name)
			}
		}

		providers = append(providers, providerConfig)
	}

	return providers, nil
}

// providerTypeMapping defines how legacy OLSConfig provider types map to Llama Stack provider_type strings.
// New providers that are fully supported without operator changes should use the llamaStackGeneric
// type with the providerType field instead of adding entries here.
//
// To add a new legacy provider:
//  1. Add an entry to this map with OLSConfig type as key
//  2. Set the Llama Stack provider_type value (e.g., "remote::new-provider")
//  3. Add credential/config handling in the switch inside buildLlamaStackInferenceProviders
var providerTypeMapping = map[string]string{
	"openai":       "remote::openai",
	"rhoai_vllm":   "remote::vllm",
	"rhelai_vllm":  "remote::vllm",
	"azure_openai": "remote::azure",
	// fake_provider is included in the CRD enum for testing purposes
	"fake_provider": "remote::fake",
}

// getProviderType returns the Llama Stack provider_type string for a legacy OLSConfig
// provider type (openai, azure_openai, rhoai_vllm, rhelai_vllm).
// It is only called for providers where ProviderType == "" (the legacy path);
// generic providers (ProviderType != "") set provider_type directly without this function.
// Returns an error for unsupported types (watsonx, bam) or invalid generic usage.
func getProviderType(provider *olsv1alpha1.ProviderSpec) (string, error) {
	// Legacy providers use predefined mapping
	if llamaType, exists := providerTypeMapping[provider.Type]; exists {
		return llamaType, nil
	}

	// Unsupported provider type
	switch provider.Type {
	case "watsonx", "bam":
		return "", fmt.Errorf("provider type '%s' (provider '%s') is not currently supported by Llama Stack. Supported types: openai, azure_openai, rhoai_vllm, rhelai_vllm, %s", provider.Type, provider.Name, utils.LlamaStackGenericType)
	case utils.LlamaStackGenericType:
		return "", fmt.Errorf("provider type '%s' (provider '%s') requires providerType and config fields to be set", utils.LlamaStackGenericType, provider.Name)
	default:
		return "", fmt.Errorf("unknown provider type '%s' (provider '%s'). Supported types: openai, azure_openai, rhoai_vllm, rhelai_vllm, %s", provider.Type, provider.Name, utils.LlamaStackGenericType)
	}
}

// deepCopyMap creates a deep copy of a map[string]interface{}, including nested maps
// and slices. This prevents mutations of the copy from affecting the original.
func deepCopyMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = deepCopyValue(v)
	}
	return dst
}

// deepCopyValue recursively deep-copies a value that may be a primitive, map, or slice.
func deepCopyValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		return deepCopyMap(val)
	case []interface{}:
		dstSlice := make([]interface{}, len(val))
		for i, elem := range val {
			dstSlice[i] = deepCopyValue(elem)
		}
		return dstSlice
	default:
		return v
	}
}

// hasAPIKeyField reports whether the config already contains an explicit "api_key" field.
// We only check for "api_key" because that is the field we auto-inject; suppressing
// injection only when the caller has already set it avoids silently skipping injection
// for providers that require api_key but happen to have an unrelated field (e.g. api_token).
func hasAPIKeyField(config map[string]interface{}) bool {
	_, ok := config["api_key"]
	return ok
}

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
					"backend":    "sql_default",
					"table_name": "vector_store",
				},
				"persistence": map[string]interface{}{
					"backend":   "kv_default",
					"namespace": "vector_persistence",
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

// buildLlamaStackStorage configures persistent storage for Llama Stack
// This defines storage backends and how different data types use them
func buildLlamaStackStorage(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) map[string]interface{} {
	// Define storage backends - SQL only
	backends := map[string]interface{}{
		"sql_default": map[string]interface{}{
			"type":    "sql_sqlite",
			"db_path": "/tmp/llama-stack/sql_store.db",
		},
		"kv_default": map[string]interface{}{
			"type":    "kv_sqlite",
			"db_path": "/tmp/llama-stack/kv_store.db",
		},
		"postgres_backend": map[string]interface{}{
			"type":     "sql_postgres",
			"host":     "lightspeed-postgres-server.openshift-lightspeed.svc",
			"port":     5432,
			"user":     "postgres",
			"password": "${env.POSTGRES_PASSWORD}",
			// Note: Database name is HARDCODED to "llamastack" in llama-stack's postgres adapter
			// Not configurable - llama-stack ignores image_name for database selection
			"ssl_mode":     "require",
			"ca_cert_path": "/etc/certs/postgres-ca/service-ca.crt",
			"gss_encmode":  "disable",
		},
	}

	// Map data stores to backends - all use SQL with table_name
	stores := map[string]interface{}{
		"metadata": map[string]interface{}{
			"namespace": "registry",
			"backend":   "kv_default",
		},
		"inference": map[string]interface{}{
			"table_name": "inference_store",
			"backend":    "sql_default",
		},
		"conversations": map[string]interface{}{
			"table_name": "openai_conversations", // Required by config schema but ignored - llama-stack uses hardcoded names
			"backend":    "postgres_backend",
		},
	}

	return map[string]interface{}{
		"backends": backends,
		"stores":   stores,
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
		"agents":       buildLlamaStackAgentProviders(r, cr),
		"inference":    inferenceProviders,
		"safety":       buildLlamaStackSafety(r, cr),
		"tool_runtime": buildLlamaStackToolRuntime(r, cr),
		"vector_io":    buildLlamaStackVectorDB(r, cr),
	}

	// Add top-level fields
	config["scoring_fns"] = []interface{}{}
	config["server"] = buildLlamaStackServerConfig(r, cr)
	config["storage"] = buildLlamaStackStorage(r, cr)
	config["vector_dbs"] = buildLlamaStackVectorDBs(r, cr)
	config["models"] = buildLlamaStackModels(r, cr)
	config["tool_groups"] = buildLlamaStackToolGroups(r, cr)
	config["telemetry"] = map[string]interface{}{
		"enabled": false,
	}

	// Convert to YAML
	yamlBytes, err := yaml.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Llama Stack config to YAML: %w", err)
	}

	return string(yamlBytes), nil
}

// ============================================================================
// LCore Config component builder functions (return maps for maintainability)
// ============================================================================

func buildLCoreServiceConfig(_ reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) map[string]interface{} {
	// Map LogLevel from OLSConfig
	// Valid values: DEBUG, INFO, WARNING, ERROR, CRITICAL
	// Default to info if not specified
	logLevel := olsv1alpha1.LogLevelInfo
	if cr.Spec.OLSConfig.LogLevel != "" {
		logLevel = cr.Spec.OLSConfig.LogLevel
	}

	// color_log: enable colored logs for DEBUG, disable for production (INFO+)
	colorLog := logLevel == olsv1alpha1.LogLevelDebug

	serviceConfig := map[string]interface{}{
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

	// Add proxy configuration if specified
	if cr.Spec.OLSConfig.ProxyConfig != nil {
		proxyConfigMap := map[string]interface{}{}

		if cr.Spec.OLSConfig.ProxyConfig.ProxyURL != "" {
			proxyConfigMap["proxy_url"] = cr.Spec.OLSConfig.ProxyConfig.ProxyURL
		}

		proxyCACertRef := cr.Spec.OLSConfig.ProxyConfig.ProxyCACertificateRef
		cmName := utils.GetProxyCACertConfigMapName(proxyCACertRef)
		if cmName != "" {
			certKey := utils.GetProxyCACertKey(proxyCACertRef)
			proxyConfigMap["proxy_ca_cert_path"] = path.Join(utils.OLSAppCertsMountRoot, utils.ProxyCACertVolumeName, certKey)
		}

		if len(proxyConfigMap) > 0 {
			serviceConfig["proxy_config"] = proxyConfigMap
		}
	}

	return serviceConfig
}

func buildLCoreLlamaStackConfig(r reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) map[string]interface{} {
	// Server mode: llama-stack runs as a separate service (container)
	// Library mode: llama-stack runs as an embedded library
	isLibraryMode := r != nil && !r.GetLCoreServerMode()

	llamaStackConfig := map[string]interface{}{
		"use_as_library_client": isLibraryMode,
		"url":                   "http://localhost:8321",
		"api_key":               "xyzzy",
	}

	// In library mode, add path to llama-stack config file
	if isLibraryMode {
		llamaStackConfig["library_client_config_path"] = utils.LlamaStackConfigMountPath
	}

	return llamaStackConfig
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

// buildLCoreDatabaseConfig configures persistent database storage
// Supports SQLite (file-based) or PostgreSQL (server-based)
// Default: PostgreSQL (shared with App Server)
func buildLCoreDatabaseConfig(r reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) map[string]interface{} {
	// Example: SQLite configuration
	// return map[string]interface{}{
	// 	"sqlite": map[string]interface{}{
	// 		"db_path": "/app-root/data/lightspeed-stack.db", // Mount a PVC here for persistence
	// 	},
	// }

	// PostgreSQL configuration (shared with App Server)
	return map[string]interface{}{
		"postgres": map[string]interface{}{
			"host":         utils.PostgresServiceName + "." + r.GetNamespace() + ".svc",
			"port":         utils.PostgresServicePort,
			"db":           utils.PostgresDefaultDbName,
			"user":         utils.PostgresDefaultUser,
			"password":     "${env.POSTGRES_PASSWORD}", // Environment variable substitution via llama_stack.core.stack.replace_env_vars
			"ssl_mode":     utils.PostgresDefaultSSLMode,
			"gss_encmode":  "disable", // Default from lightspeed-stack constants
			"ca_cert_path": "/etc/certs/postgres-ca/service-ca.crt",
			"namespace":    "lcore", // Separate schema for LCore to avoid conflicts with App Server
		},
	}
}

// buildLCoreMCPServersConfig configures Model Context Protocol servers
// Allows integration with external context providers for agent workflows
// NOTE: Secret validation is performed separately during deployment generation
func buildLCoreMCPServersConfig(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) []map[string]interface{} {
	mcpServers := []map[string]interface{}{}

	// Add OpenShift MCP server if introspection is enabled
	if cr.Spec.OLSConfig.IntrospectionEnabled {
		mcpServers = append(mcpServers, map[string]interface{}{
			"name": "openshift",
			"url":  fmt.Sprintf(utils.OpenShiftMCPServerURL, utils.OpenShiftMCPServerPort),
			// Authorization headers for K8s authentication
			"authorization_headers": map[string]string{
				utils.K8S_AUTH_HEADER: utils.KUBERNETES_PLACEHOLDER,
			},
		})
	}

	// Add user-defined MCP servers
	if cr.Spec.FeatureGates != nil && slices.Contains(cr.Spec.FeatureGates, utils.FeatureGateMCPServer) {
		for _, server := range cr.Spec.MCPServers {
			// Build MCP server config
			mcpServer := map[string]interface{}{
				"name": server.Name,
				"url":  server.URL,
			}

			// Add timeout if specified (default is handled by lightspeed-stack)
			if server.Timeout > 0 {
				mcpServer["timeout"] = server.Timeout
			}

			// Add authorization headers if configured
			if len(server.Headers) > 0 {
				headers := make(map[string]string)
				invalidServer := false
				for _, header := range server.Headers {
					if invalidServer {
						break
					}
					headerName := header.Name
					var headerValue string

					// Determine header value based on discriminator type
					switch header.ValueFrom.Type {
					case olsv1alpha1.MCPHeaderSourceTypeKubernetes:
						headerValue = utils.KUBERNETES_PLACEHOLDER
					case olsv1alpha1.MCPHeaderSourceTypeClient:
						headerValue = utils.CLIENT_PLACEHOLDER
					case olsv1alpha1.MCPHeaderSourceTypeSecret:
						if header.ValueFrom.SecretRef == nil || header.ValueFrom.SecretRef.Name == "" {
							r.GetLogger().Error(
								fmt.Errorf("missing secretRef for type 'secret'"),
								"Skipping MCP server: type is 'secret' but secretRef is not set",
								"server", server.Name,
								"header", headerName,
							)
							invalidServer = true
							continue
						}
						// Use consistent path structure: /etc/mcp/headers/<secretName>/header
						headerValue = path.Join(utils.MCPHeadersMountRoot, header.ValueFrom.SecretRef.Name, utils.MCPSECRETDATAPATH)
					default:
						// This should never happen due to enum validation
						r.GetLogger().Error(
							fmt.Errorf("invalid MCP header type: %s", header.ValueFrom.Type),
							"Skipping MCP server due to invalid header type",
							"server", server.Name,
							"header", headerName,
							"type", header.ValueFrom.Type,
						)
						invalidServer = true
						continue
					}

					headers[headerName] = headerValue
				}

				// Skip this server if any header was invalid
				if invalidServer {
					continue
				}

				if len(headers) > 0 {
					mcpServer["authorization_headers"] = headers
				}
			}

			mcpServers = append(mcpServers, mcpServer)
		}
	}

	return mcpServers
}

// buildLCoreCustomizationConfig configures system prompt customization
// Uses CR field if set, otherwise falls back to default (same as lightspeed-service)
func buildLCoreCustomizationConfig(_ reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) map[string]interface{} {
	systemPrompt := DefaultQuerySystemPrompt
	if cr.Spec.OLSConfig.QuerySystemPrompt != "" {
		systemPrompt = cr.Spec.OLSConfig.QuerySystemPrompt
	}

	return map[string]interface{}{
		"system_prompt":               systemPrompt,
		"disable_query_system_prompt": true, // Prevent users from overriding via API
	}
}

// buildLCoreConversationCacheConfig configures chat history caching
// Options: noop (disabled), memory (in-memory), sqlite (file), postgres (database)
// Useful for maintaining conversation context across requests
func buildLCoreConversationCacheConfig(r reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) map[string]interface{} {
	// PostgreSQL cache (shared with App Server)
	return map[string]interface{}{
		"type": "postgres",
		"postgres": map[string]interface{}{
			"host":         utils.PostgresServiceName + "." + r.GetNamespace() + ".svc",
			"port":         utils.PostgresServicePort,
			"db":           utils.PostgresDefaultDbName,
			"user":         utils.PostgresDefaultUser,
			"password":     "${env.POSTGRES_PASSWORD}", // Environment variable substitution
			"ssl_mode":     utils.PostgresDefaultSSLMode,
			"gss_encmode":  "disable",
			"ca_cert_path": "/etc/certs/postgres-ca/service-ca.crt",
			"namespace":    "conversation_cache", // Separate schema for conversation cache
		},
	}
}

// buildLCoreQuotaHandlersConfig configures token usage rate limiting
// Controls how many tokens users or clusters can consume
// Useful for cost management and preventing abuse
func buildLCoreQuotaHandlersConfig(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) map[string]interface{} {
	// If no quota config in CR, return nil (disabled)
	if cr.Spec.OLSConfig.QuotaHandlersConfig == nil || len(cr.Spec.OLSConfig.QuotaHandlersConfig.LimitersConfig) == 0 {
		return nil
	}

	quotaConfig := cr.Spec.OLSConfig.QuotaHandlersConfig

	// Build limiters array from CR configuration
	limiters := []interface{}{}
	for _, limiter := range quotaConfig.LimitersConfig {
		limiters = append(limiters, map[string]interface{}{
			"type":           limiter.Type, // "user_limiter" or "cluster_limiter"
			"name":           limiter.Name,
			"initial_quota":  limiter.InitialQuota,
			"quota_increase": limiter.QuotaIncrease,
			"period":         limiter.Period, // e.g., "1 day", "1 hour"
		})
	}

	return map[string]interface{}{
		"limiters": limiters,
		"scheduler": map[string]interface{}{
			"period": 300, // Check quotas every 300 seconds (5 minutes) - matches app server
		},
		"enable_token_history": quotaConfig.EnableTokenHistory,
		// PostgreSQL configuration at top level - quota system expects postgres/sqlite at this level
		"postgres": map[string]interface{}{
			"host":         utils.PostgresServiceName + "." + r.GetNamespace() + ".svc",
			"port":         utils.PostgresServicePort,
			"db":           utils.PostgresDefaultDbName,
			"user":         utils.PostgresDefaultUser,
			"password":     "${env.POSTGRES_PASSWORD}", // Environment variable substitution
			"ssl_mode":     utils.PostgresDefaultSSLMode,
			"gss_encmode":  "disable",
			"ca_cert_path": "/etc/certs/postgres-ca/service-ca.crt",
			"namespace":    "quota", // Separate schema for quota data
		},
	}
}

// buildLCoreToolsApprovalConfig configures tool execution approval
// Controls whether tool calls require user approval before execution
func buildLCoreToolsApprovalConfig(_ reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) map[string]interface{} {
	// If no tools approval config in CR, return nil (not configured)
	if cr.Spec.OLSConfig.ToolsApprovalConfig == nil {
		return nil
	}

	cfg := cr.Spec.OLSConfig.ToolsApprovalConfig

	// Apply defaults if not set
	approvalType := string(cfg.ApprovalType)
	if approvalType == "" {
		approvalType = string(olsv1alpha1.ApprovalTypeNever)
	}

	approvalTimeout := cfg.ApprovalTimeout
	if approvalTimeout == 0 {
		approvalTimeout = 600
	}

	return map[string]interface{}{
		"approval_type":    approvalType,    // "never", "always", or "tool_annotations"
		"approval_timeout": approvalTimeout, // Timeout in seconds for waiting for user approval
	}
}

// buildLCoreConfigYAML assembles the complete Lightspeed Core Service configuration and converts to YAML
func buildLCoreConfigYAML(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (string, error) {
	// Build the complete config as a map
	config := map[string]interface{}{
		"name":                 "Lightspeed Core Service (LCS)",
		"service":              buildLCoreServiceConfig(r, cr),
		"llama_stack":          buildLCoreLlamaStackConfig(r, cr),
		"user_data_collection": buildLCoreUserDataCollectionConfig(r, cr),
		"authentication":       buildLCoreAuthenticationConfig(r, cr),
		"inference":            buildLCoreInferenceConfig(r, cr),
		"database":             buildLCoreDatabaseConfig(r, cr),          // Persistent storage (SQLite/PostgreSQL)
		"customization":        buildLCoreCustomizationConfig(r, cr),     // Same system prompt as lightspeed-service
		"conversation_cache":   buildLCoreConversationCacheConfig(r, cr), // Chat history caching (PostgreSQL)
	}

	// Optional features - only add if configured/enabled
	if quotaConfig := buildLCoreQuotaHandlersConfig(r, cr); quotaConfig != nil {
		config["quota_handlers"] = quotaConfig // Token rate limiting
	}

	// Tools approval configuration (requires user approval before tool execution)
	if toolsApprovalConfig := buildLCoreToolsApprovalConfig(r, cr); toolsApprovalConfig != nil {
		config["tools_approval"] = toolsApprovalConfig
	}

	// MCP servers configuration (includes introspection + user-defined servers)
	if mcpServers := buildLCoreMCPServersConfig(r, cr); len(mcpServers) > 0 {
		config["mcp_servers"] = mcpServers // Model Context Protocol servers
	}

	// Convert to YAML
	yamlBytes, err := yaml.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal LCore config to YAML: %w", err)
	}

	return string(yamlBytes), nil
}
