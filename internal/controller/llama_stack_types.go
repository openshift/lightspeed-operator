package controller

/*** Llama stack configuration ***/

// LlamaStackConfig is the root structure of the llama stack configuration file
type LlamaStackConfig struct {
	// Version is the version of the llama stack configuration file, currently only version 2 is supported
	Version string `json:"version"`
	// Providers is the list of providers for the llama stack, it contains providers for each of the llama stack components
	// such as inference, datasetio, etc.
	Providers ProviderList `json:"providers"`
}

// ProviderList is the list of providers for the llama stack, it contains providers for each of the llama stack components
// such as inference, datasetio, etc.
// here is an example of the providers section of the llama stack configuration file:
/*
   providers:
     agents:
     - config:
         persistence_store:
           db_path: .llama/distributions/ollama/agents_store.db
           namespace: null
           type: sqlite
         responses_store:
           db_path: .llama/distributions/ollama/responses_store.db
           type: sqlite
       provider_id: meta-reference
       provider_type: inline::meta-reference
     datasetio:
     - config:
         kvstore:
           db_path: .llama/distributions/ollama/huggingface_datasetio.db
           namespace: null
           type: sqlite
       provider_id: huggingface
       provider_type: remote::huggingface
     - config:
         kvstore:
           db_path: .llama/distributions/ollama/localfs_datasetio.db
           namespace: null
           type: sqlite
       provider_id: localfs
       provider_type: inline::localfs
     inference:
       - provider_id: openai
         provider_type: remote::openai
         config:
           api_key: ${env.OPENAI_API_KEY}
*/
type ProviderList struct {
	Inference []InferenceProviderConfig `json:"inference"`

	// todo: add other providers here
}

type InferenceProviderConfig struct {
	// Provider ID is the unique identifier in the llama stack configuration
	ProviderID string `json:"provider_id"`
	// Provider Type is the type of the provider, this determines the underlyingtype of Config field below
	ProviderType string `json:"provider_type"`
	// Config is the provider specific configuration, can be one of the following:
	// - InferenceProviderOpenAI
	// - InferenceProviderAzureOpenAI
	// - InferenceProviderWatsonX
	Config interface{} `json:"config"`
}

/*** Inference provider configuration ***/

// OpenAI inference provider configuration
// https://llamastack.github.io/docs/providers/inference/remote_openai
type InferenceProviderOpenAI struct {
	APIKey  string `json:"api_key,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
}

// Azure OpenAI inference provider configuration
// https://llamastack.github.io/docs/providers/inference/remote_azure_openai
type InferenceProviderAzureOpenAI struct {
	APIKey     string `json:"api_key,omitempty"`
	APIBase    string `json:"api_base,omitempty"`
	APIVersion string `json:"api_version,omitempty"`
	APIType    string `json:"api_type,omitempty"`
}

// WatsonX inference provider configuration
// https://llamastack.github.io/docs/providers/inference/remote_watsonx
type InferenceProviderWatsonX struct {
	APIKey    string `json:"api_key,omitempty"`
	URL       string `json:"url,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Timeout   int    `json:"timeout,omitempty"`
}

// RHELAI inference provider configuration
// https://llamastack.github.io/docs/providers/inference/remote_vllm
type InferenceProviderVLLM struct {
	APIToken      string `json:"api_token,omitempty"`
	URL           string `json:"url,omitempty"`
	MaxTokens     int    `json:"max_tokens,omitempty"`
	TLSVerify     bool   `json:"tls_verify,omitempty"`
	RefreshModels bool   `json:"refresh_models,omitempty"`
}
