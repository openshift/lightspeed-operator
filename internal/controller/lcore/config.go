package lcore

import (
	"context"
	"fmt"
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
		// Minimal APIs for RAG + MCP: agents (for MCP), files, inference, safety (required by agents), telemetry, tool_runtime, vector_io
		// Commented out: datasetio, eval, post_training, scoring (not needed for basic RAG + MCP)
		// Commented out: datasetio, eval, post_training, prompts, scoring, telemetry
		"apis":                   []string{"agents" /* "datasetio", "eval", */, "files", "inference" /* , "post_training", */, "safety" /* , "scoring", "telemetry"*/, "tool_runtime", "vector_io"},
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
						"namespace":  "agent_responces",
					},
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
					"db_path":   "/tmp/llama-stack/huggingface_datasetio.db",
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
					"db_path":   "/tmp/llama-stack/localfs_datasetio.db",
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
					"db_path":   "/tmp/llama-stack/meta_reference_eval.db",
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
		switch provider.Type {
		case "openai", "rhoai_vllm", "rhelai_vllm":
			config := map[string]interface{}{}
			// Determine the appropriate Llama Stack provider type
			// - OpenAI uses remote::openai (validates against OpenAI model whitelist)
			// - vLLM uses remote::vllm (accepts any custom model names)
			if provider.Type == "openai" {
				providerConfig["provider_type"] = "remote::openai"
				// Set API key from environment variable
				// Llama Stack will substitute ${env.VAR_NAME} with the actual env var value
				config["api_key"] = fmt.Sprintf("${env.%s_API_KEY}", envVarName)
			} else {
				providerConfig["provider_type"] = "remote::vllm"
				// Set API key from environment variable
				// Llama Stack will substitute ${env.VAR_NAME} with the actual env var value
				config["api_token"] = fmt.Sprintf("${env.%s_API_KEY}", envVarName)
			}

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

		case "watsonx", "bam":
			// These providers are not supported by Llama Stack
			// They are handled directly by lightspeed-stack (LCS), not Llama Stack
			return nil, fmt.Errorf("provider type '%s' (provider '%s') is not currently supported by Llama Stack. Supported types: openai, azure_openai, rhoai_vllm, rhelai_vllm", provider.Type, provider.Name)

		default:
			// Unknown provider type
			return nil, fmt.Errorf("unknown provider type '%s' (provider '%s'). Supported types: openai, azure_openai, rhoai_vllm, rhelai_vllm", provider.Type, provider.Name)
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

// Telemetry provider - commented out as the API doesn't exist in this Llama Stack version
// Uncomment and enable in apis list when telemetry API becomes available
/*
func buildLlamaStackTelemetry(_ reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) []interface{} {
	// Console telemetry provider - logs to stdout
	return []interface{}{
		map[string]interface{}{
			"provider_id":   "console",
			"provider_type": "inline::console",
			"config": map[string]interface{}{
				"service_name": "lightspeed-stack",
				"sinks":        []string{"console"},
			},
		},
	}

	// Alternative options (change return statement above):
	// SQLite telemetry provider - stores traces in SQLite:
	//   provider_id: "sqlite", provider_type: "inline::meta-reference"
	//   config: {service_name, sinks: "sqlite", sqlite_db_path: "/tmp/llama-stack/trace_store.db"}
	//
	// OTLP telemetry provider - sends traces to Jaeger/OTLP endpoint:
	//   provider_id: "otlp", provider_type: "inline::otlp"
	//   config: {service_name, sinks: "otlp", otlp_endpoint: "http://jaeger:4317"}
}
*/

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
	// Note: telemetry provider commented out as the API doesn't exist in this Llama Stack version
	config["providers"] = map[string]interface{}{
		"files":     buildLlamaStackFileProviders(r, cr),
		"agents":    buildLlamaStackAgentProviders(r, cr), // Required for MCP
		"inference": inferenceProviders,
		"safety":    buildLlamaStackSafety(r, cr), // Required by agents provider
		// "telemetry":    buildLlamaStackTelemetry(r, cr),   // Telemetry and tracing
		"tool_runtime": buildLlamaStackToolRuntime(r, cr), // Required for RAG
		"vector_io":    buildLlamaStackVectorDB(r, cr),    // Required for RAG
	}

	// Add top-level fields
	config["scoring_fns"] = []interface{}{} // Keep empty for now
	config["server"] = buildLlamaStackServerConfig(r, cr)
	config["storage"] = buildLlamaStackStorage(r, cr) // Persistent storage configuration
	// config["shields"] = buildLlamaStackShields(r, cr) // Commented out - not needed without safety API
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

	// Example: SQLite cache (requires persistent volume)
	// return map[string]interface{}{
	//     "type": "sqlite",
	//     "sqlite": map[string]interface{}{
	//         "db_path": "/app-root/data/conversation_cache.db",
	//     },
	// }

}

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
			"period": 1, // Check quotas every 1 second
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
		"database":             buildLCoreDatabaseConfig(r, cr),          // Persistent storage (SQLite/PostgreSQL)
		"customization":        buildLCoreCustomizationConfig(r, cr),     // Same system prompt as lightspeed-service
		"conversation_cache":   buildLCoreConversationCacheConfig(r, cr), // Chat history caching (PostgreSQL)
	}

	// Optional features - only add if configured/enabled
	if quotaConfig := buildLCoreQuotaHandlersConfig(r, cr); quotaConfig != nil {
		config["quota_handlers"] = quotaConfig // Token rate limiting
	}

	// Optional features (uncomment to enable):
	// "mcp_servers":        buildLCoreMCPServersConfig(r, cr),         // Model Context Protocol servers
	// "authorization":      buildLCoreAuthorizationConfig(r, cr),      // Role-based access control
	// "byok_rag":           buildLCoreByokRagConfig(r, cr),            // Custom RAG sources

	// Convert to YAML
	yamlBytes, err := yaml.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal LCore config to YAML: %w", err)
	}

	return string(yamlBytes), nil
}
