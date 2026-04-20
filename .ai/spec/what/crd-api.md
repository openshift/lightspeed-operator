# CRD API

Specification of the OLSConfig Custom Resource Definition. Source of truth: `api/v1alpha1/olsconfig_types.go`.

## Behavioral Rules

### Resource Identity

1. API group: `ols.openshift.io`, version: `v1alpha1`, kind: `OLSConfig`.
2. Cluster-scoped (not namespaced). Marker: `+kubebuilder:resource:scope=Cluster`.
3. `.metadata.name` must be `"cluster"`. Enforced by XValidation rule on the OLSConfig type: `self.metadata.name == 'cluster'`.
4. Has a status subresource (`+kubebuilder:subresource:status`).
5. Finalizer: `ols.openshift.io/finalizer` (constant `OLSConfigFinalizer` in `internal/controller/utils/constants.go`).
6. `spec` is required on the OLSConfig object.

### Top-Level Spec Structure

Field path | JSON key | Go type | Required | Description
---|---|---|---|---
`spec.llm` | `llm` | `LLMSpec` | Yes | LLM provider configuration
`spec.ols` | `ols` | `OLSSpec` | Yes | OLS service settings
`spec.olsDataCollector` | `olsDataCollector` | `OLSDataCollectorSpec` | No | Data collector settings (logLevel only)
`spec.mcpServers` | `mcpServers` | `[]MCPServerConfig` | No | External MCP server configurations. MaxItems=20
`spec.featureGates` | `featureGates` | `[]FeatureGate` | No | Feature gates. Enum values: `MCPServer`, `ToolFiltering`

### LLM Provider Configuration (spec.llm)

7. `spec.llm.providers` is required. Type: `[]ProviderSpec`. MaxItems=10.

#### ProviderSpec Fields

Field path (relative to each provider) | JSON key | Go type | Required | Description
---|---|---|---|---
`name` | `name` | `string` | Yes | Provider name
`url` | `url` | `string` | No | Provider API URL. Pattern: `^https?://.*$`
`credentialsSecretRef` | `credentialsSecretRef` | `corev1.LocalObjectReference` | Yes | Secret containing API credentials
`models` | `models` | `[]ModelSpec` | Yes | Provider models. MaxItems=50
`type` | `type` | `string` | Yes | Provider type enum: `azure_openai`, `bam`, `openai`, `watsonx`, `rhoai_vllm`, `rhelai_vllm`, `fake_provider`, `llamaStackGeneric`
`deploymentName` | `deploymentName` | `string` | No | Azure OpenAI deployment name
`apiVersion` | `apiVersion` | `string` | No | Azure OpenAI API version
`projectID` | `projectID` | `string` | No | Watsonx project ID
`fakeProviderMCPToolCall` | `fakeProviderMCPToolCall` | `bool` | No | Fake provider MCP tool call flag
`tlsSecurityProfile` | `tlsSecurityProfile` | `*configv1.TLSSecurityProfile` | No | TLS profile for provider connection
`providerType` | `providerType` | `string` | No | Llama Stack Generic provider type. Pattern: `^(inline\|remote)::[a-z0-9][a-z0-9_-]*$`
`config` | `config` | `*runtime.RawExtension` | No | Arbitrary provider config (Llama Stack Generic mode). PreserveUnknownFields enabled
`credentialKey` | `credentialKey` | `string` | No | Key name within `credentialsSecretRef` to read credential from. Defaults to `"apitoken"` if unset

#### Provider XValidation Rules

8. Azure OpenAI requires `deploymentName`: when `type == "azure_openai"`, `deploymentName` must not be empty.
9. Watsonx requires `projectID`: when `type == "watsonx"`, `projectID` must not be empty.
10. `providerType` and `config` must be used together: if `providerType` is set, `config` must also be set, and vice versa.
11. Llama Stack Generic requires `type='llamaStackGeneric'`: if `providerType` is set, `type` must be `"llamaStackGeneric"`.
12. Llama Stack Generic cannot use legacy fields: when `type == "llamaStackGeneric"`, none of `deploymentName`, `projectID`, `url`, or `apiVersion` may be set.
13. `credentialKey` must not be empty or whitespace: if set, it must not match `^[ \t\n\r\v\f]*$`.

#### ModelSpec Fields

Field path (relative to each model) | JSON key | Go type | Required | Description
---|---|---|---|---
`name` | `name` | `string` | Yes | Model name
`url` | `url` | `string` | No | Model API URL. Pattern: `^https?://.*$`
`contextWindowSize` | `contextWindowSize` | `uint` | No | Context window in tokens. Minimum=1024
`parameters` | `parameters` | `ModelParametersSpec` | No | Model parameters

#### ModelParametersSpec Fields

Field path (relative to parameters) | JSON key | Go type | Required | Default | Validation
---|---|---|---|---|---
`maxTokensForResponse` | `maxTokensForResponse` | `int` | No | (unset; application default is 2048) | None
`toolBudgetRatio` | `toolBudgetRatio` | `float64` | No | `0.25` | Minimum=0.1, Maximum=0.5

### OLS Configuration (spec.ols)

#### Core Fields

14. `spec.ols.defaultModel` -- `string`, required. The default model name for usage.
15. `spec.ols.defaultProvider` -- `string`, required. The default provider name for usage.
16. `spec.ols.logLevel` -- `LogLevel` enum, optional. Values: `DEBUG`, `INFO`, `WARNING`, `ERROR`, `CRITICAL`. Default: `INFO`.

#### Conversation Cache (spec.ols.conversationCache)

17. `spec.ols.conversationCache.type` -- `CacheType` enum. Only valid value: `postgres`. Default: `postgres`.
18. `spec.ols.conversationCache.postgres.sharedBuffers` -- `string`, XIntOrString. Default: `"256MB"`.
19. `spec.ols.conversationCache.postgres.maxConnections` -- `int`. Default: `2000`. Minimum=1, Maximum=262143.

#### Deployment Configuration (spec.ols.deployment)

The deployment config uses two struct types:

- **`Config`**: has `replicas`, `resources`, `tolerations`, `nodeSelector`, `affinity`, `topologySpreadConstraints`.
- **`ContainerConfig`**: has `resources` only.

Field path (relative to `spec.ols.deployment`) | JSON key | Go type | Notes
---|---|---|---
`api` | `api` | `Config` | API container. Replicas configurable (default 1, min 0)
`dataCollector` | `dataCollector` | `ContainerConfig` | Data collector container. Resources only
`mcpServer` | `mcpServer` | `ContainerConfig` | MCP server container. Resources only
`llamaStack` | `llamaStack` | `ContainerConfig` | Llama Stack container. Resources only
`console` | `console` | `Config` | Console container. Has replicas field but operator forces 1
`database` | `database` | `Config` | Database container. Has replicas field but operator forces 1

20. Replicas are only user-configurable for the API container (`spec.ols.deployment.api.replicas`). For console and database, the operator always overrides replicas to 1 regardless of spec value.

##### Config Fields

Field path (relative to Config) | JSON key | Go type | Default | Validation
---|---|---|---|---
`replicas` | `replicas` | `*int32` | `1` | Minimum=0
`resources` | `resources` | `*corev1.ResourceRequirements` | (none) | Standard k8s resource requirements
`tolerations` | `tolerations` | `[]corev1.Toleration` | (none) | Standard k8s tolerations
`nodeSelector` | `nodeSelector` | `map[string]string` | (none) | Key-value label selector
`affinity` | `affinity` | `*corev1.Affinity` | (none) | Standard k8s affinity rules
`topologySpreadConstraints` | `topologySpreadConstraints` | `[]corev1.TopologySpreadConstraint` | (none) | Standard k8s topology spread

##### ContainerConfig Fields

Field path (relative to ContainerConfig) | JSON key | Go type
---|---|---
`resources` | `resources` | `*corev1.ResourceRequirements`

#### Query Filters (spec.ols.queryFilters)

21. Type: `[]QueryFiltersSpec`. Each entry has:

Field | JSON key | Go type | Required
---|---|---|---
`name` | `name` | `string` | No
`pattern` | `pattern` | `string` | No
`replaceWith` | `replaceWith` | `string` | No

#### User Data Collection (spec.ols.userDataCollection)

22. `spec.ols.userDataCollection.feedbackDisabled` -- `bool`, optional. Disables user feedback collection.
23. `spec.ols.userDataCollection.transcriptsDisabled` -- `bool`, optional. Disables transcript collection.

#### TLS Configuration (spec.ols.tlsConfig)

24. `spec.ols.tlsConfig` -- `*TLSConfig`, optional. Pointer type (nil when absent).
25. `spec.ols.tlsConfig.keyCertSecretRef` -- `corev1.LocalObjectReference`. Secret must contain keys: `tls.crt` (required), `tls.key` (required), `ca.crt` (optional, for console proxy trust).

#### Additional CA (spec.ols.additionalCAConfigMapRef)

26. `spec.ols.additionalCAConfigMapRef` -- `*corev1.LocalObjectReference`, optional. ConfigMap with additional CA certificates for LLM provider TLS.

#### TLS Security Profile (spec.ols.tlsSecurityProfile)

27. `spec.ols.tlsSecurityProfile` -- `*configv1.TLSSecurityProfile`, optional. OpenShift TLS security profile for API endpoints.

#### Introspection (spec.ols.introspectionEnabled)

28. `spec.ols.introspectionEnabled` -- `bool`, optional. Enables introspection features.

#### MCP Kubernetes Server (spec.ols.mcpKubeServerConfig)

29. `spec.ols.mcpKubeServerConfig.timeout` -- `int`. Default: `60`. Minimum=5. Timeout in seconds for the built-in MCP Kubernetes server.

#### Proxy Configuration (spec.ols.proxyConfig)

30. `spec.ols.proxyConfig.proxyURL` -- `string`, optional. Pattern: `^https?://.*$`. If unset, cluster-wide proxy is used via `https_proxy` env var.
31. `spec.ols.proxyConfig.proxyCACertificate` -- `*ProxyCACertConfigMapRef`, optional. Struct type `atomic`.

`ProxyCACertConfigMapRef` fields:
- Inline `corev1.LocalObjectReference` (provides `name` field for the ConfigMap name)
- `key` -- `string`. Default: `"proxy-ca.crt"`. Key within the ConfigMap holding the proxy CA certificate.

#### RAG Configuration (spec.ols.rag)

32. Type: `[]RAGSpec`, optional.

Field | JSON key | Go type | Required | Default
---|---|---|---|---
`image` | `image` | `string` | Yes | (none)
`indexPath` | `indexPath` | `string` | No | `"/rag/vector_db"`
`indexID` | `indexID` | `string` | No | `""`

#### Quota Handlers (spec.ols.quotaHandlersConfig)

33. `spec.ols.quotaHandlersConfig` -- `*QuotaHandlersConfig`, optional.
34. `spec.ols.quotaHandlersConfig.limitersConfig` -- `[]LimiterConfig`.
35. `spec.ols.quotaHandlersConfig.enableTokenHistory` -- `bool`, optional.

`LimiterConfig` fields:

Field | JSON key | Go type | Required | Validation
---|---|---|---|---
`name` | `name` | `string` | Yes (by convention) | None
`type` | `type` | `string` | Yes (by convention) | Enum: `cluster_limiter`, `user_limiter`
`initialQuota` | `initialQuota` | `int` | Yes (by convention) | Minimum=0
`quotaIncrease` | `quotaIncrease` | `int` | Yes (by convention) | Minimum=0
`period` | `period` | `string` | Yes (by convention) | Pattern: `^(1\s+(second\|minute\|hour\|day\|month\|year\|s\|min\|h\|d\|m\|y)\|([2-9][0-9]*\|[1-9][0-9]{2,})\s+(seconds\|minutes\|hours\|days\|months\|years\|s\|min\|h\|d\|m\|y))$`

36. Period pattern explanation: quantity 1 requires singular unit name or abbreviation; quantities >= 2 require plural unit name or abbreviation. Abbreviations (`s`, `min`, `h`, `d`, `m`, `y`) are accepted with any quantity.

#### Storage (spec.ols.storage)

37. `spec.ols.storage.size` -- `resource.Quantity`, optional. Size of the requested persistent volume.
38. `spec.ols.storage.class` -- `string`, optional. Storage class name.

#### Boolean/String Fields

39. `spec.ols.byokRAGOnly` -- `bool`, optional. When true, only BYOK RAG sources are used; built-in OpenShift documentation RAG is ignored.
40. `spec.ols.querySystemPrompt` -- `string`, optional. Custom system prompt for LLM queries. If unset, the default OpenShift Lightspeed prompt is used.
41. `spec.ols.maxIterations` -- `int`. Default: `5`. Minimum=1. Maximum number of iterations for agent execution.
42. `spec.ols.imagePullSecrets` -- `[]corev1.LocalObjectReference`, optional. Pull secrets for BYOK RAG images.

#### Tool Filtering (spec.ols.toolFilteringConfig)

43. `spec.ols.toolFilteringConfig` -- `*ToolFilteringConfig`, optional. Presence enables tool filtering; absence means all tools are used.

Field | JSON key | Go type | Default | Validation
---|---|---|---|---
`alpha` | `alpha` | `float64` | `0.8` | XValidation: must be >= 0.0 and <= 1.0. Weight for dense vs sparse retrieval (1.0 = full dense, 0.0 = full sparse)
`topK` | `topK` | `int` | `10` | Minimum=1, Maximum=50. Number of tools to retrieve
`threshold` | `threshold` | `float64` | `0.01` | XValidation: must be >= 0.0 and <= 1.0. Minimum similarity threshold

44. Tool filtering requires the `ToolFiltering` feature gate to be enabled in `spec.featureGates`.

#### Tools Approval (spec.ols.toolsApprovalConfig)

45. `spec.ols.toolsApprovalConfig` -- `*ToolsApprovalConfig`, optional.

Field | JSON key | Go type | Default | Validation
---|---|---|---|---
`approvalType` | `approvalType` | `ApprovalType` | `tool_annotations` | Enum: `never`, `always`, `tool_annotations`
`approvalTimeout` | `approvalTimeout` | `int` | `600` | Minimum=1. Timeout in seconds for user approval

46. `never`: all tools execute without approval. `always`: all tool calls require approval. `tool_annotations`: approval decision is per-tool based on annotations.

### Data Collector Configuration (spec.olsDataCollector)

47. `spec.olsDataCollector.logLevel` -- `LogLevel` enum. Default: `INFO`. Same enum as `spec.ols.logLevel`.

### MCP Server Configuration (spec.mcpServers)

48. Array of `MCPServerConfig`. MaxItems=20.

Field | JSON key | Go type | Required | Default | Validation
---|---|---|---|---|---
`name` | `name` | `string` | Yes | (none) | None
`url` | `url` | `string` | Yes | (none) | Pattern: `^https?://.*$`
`timeout` | `timeout` | `int` | No | `5` | None (no min/max markers)
`headers` | `headers` | `[]MCPHeader` | No | (none) | MaxItems=20

#### MCPHeader Fields

Field | JSON key | Go type | Required | Validation
---|---|---|---|---
`name` | `name` | `string` | Yes | MinLength=1, Pattern: `^[A-Za-z0-9-]+$`
`valueFrom` | `valueFrom` | `MCPHeaderValueSource` | Yes | Discriminated union (see below)

#### MCPHeaderValueSource Fields (discriminated union)

Field | JSON key | Go type | Required | Validation
---|---|---|---|---
`type` | `type` | `MCPHeaderSourceType` | Yes | Enum: `secret`, `kubernetes`, `client`. Union discriminator
`secretRef` | `secretRef` | `*corev1.LocalObjectReference` | Conditional | Required with non-empty `name` when `type == "secret"`. Must not be set when `type != "secret"`

49. XValidation: when `type == "secret"`, `secretRef` must be present with a non-empty `name`.
50. XValidation: when `type != "secret"` (i.e., `kubernetes` or `client`), `secretRef` must not be set.

### Status (status)

#### Conditions (status.conditions)

51. Type: `[]metav1.Condition`. Populated after first reconciliation.

Condition types used by the operator:
- `ApiReady` -- API server deployment health
- `CacheReady` -- PostgreSQL cache deployment health
- `ConsolePluginReady` -- Console UI plugin deployment health
- `ResourceReconciliation` -- Overall resource reconciliation status (set directly, not deployment-based)

#### Overall Status (status.overallStatus)

52. `status.overallStatus` -- `OverallStatus` enum. Values: `Ready`, `NotReady`. Aggregation of all component conditions. `Ready` only when all components are healthy.

#### Diagnostic Info (status.diagnosticInfo)

53. Type: `[]PodDiagnostic`, optional. Auto-populated during deployment failures, cleared on recovery.

`PodDiagnostic` fields:

Field | JSON key | Go type | Required | Description
---|---|---|---|---
`failedComponent` | `failedComponent` | `string` | Yes | Matches condition type (e.g., `"ApiReady"`, `"CacheReady"`)
`podName` | `podName` | `string` | Yes | Name of the failing pod
`containerName` | `containerName` | `string` | No | Container within the pod (empty for pod-level issues)
`reason` | `reason` | `string` | Yes | Failure reason (e.g., `ImagePullBackOff`, `CrashLoopBackOff`, `Unschedulable`, `OOMKilled`)
`message` | `message` | `string` | Yes | Detailed error from Kubernetes
`exitCode` | `exitCode` | `*int32` | No | Exit code for terminated containers only
`type` | `type` | `DiagnosticType` | Yes | Enum: `ContainerWaiting`, `ContainerTerminated`, `PodScheduling`, `PodCondition`
`lastUpdated` | `lastUpdated` | `metav1.Time` | Yes | Timestamp of diagnostic collection

## Configuration Surface

Complete field reference. All paths are relative to the OLSConfig object.

Path | Type | Default | Required | Validation | Description
---|---|---|---|---|---
`spec` | `OLSConfigSpec` | -- | Yes | -- | Top-level spec
`spec.llm` | `LLMSpec` | -- | Yes | -- | LLM settings
`spec.llm.providers` | `[]ProviderSpec` | -- | Yes | MaxItems=10 | LLM providers
`spec.llm.providers[].name` | `string` | -- | Yes | -- | Provider name
`spec.llm.providers[].url` | `string` | -- | No | Pattern `^https?://.*$` | Provider API URL
`spec.llm.providers[].credentialsSecretRef` | `LocalObjectReference` | -- | Yes | -- | Secret with credentials
`spec.llm.providers[].models` | `[]ModelSpec` | -- | Yes | MaxItems=50 | Models
`spec.llm.providers[].models[].name` | `string` | -- | Yes | -- | Model name
`spec.llm.providers[].models[].url` | `string` | -- | No | Pattern `^https?://.*$` | Model API URL
`spec.llm.providers[].models[].contextWindowSize` | `uint` | -- | No | Min=1024 | Context window (tokens)
`spec.llm.providers[].models[].parameters` | `ModelParametersSpec` | -- | No | -- | Model parameters
`spec.llm.providers[].models[].parameters.maxTokensForResponse` | `int` | -- | No | -- | Max response tokens
`spec.llm.providers[].models[].parameters.toolBudgetRatio` | `float64` | `0.25` | No | Min=0.1, Max=0.5 | Tool token budget ratio
`spec.llm.providers[].type` | `string` | -- | Yes | Enum (see rule 7) | Provider type
`spec.llm.providers[].deploymentName` | `string` | -- | No | XValidation (rule 8) | Azure deployment name
`spec.llm.providers[].apiVersion` | `string` | -- | No | -- | Azure API version
`spec.llm.providers[].projectID` | `string` | -- | No | XValidation (rule 9) | Watsonx project ID
`spec.llm.providers[].fakeProviderMCPToolCall` | `bool` | -- | No | -- | Fake provider MCP flag
`spec.llm.providers[].tlsSecurityProfile` | `*TLSSecurityProfile` | -- | No | -- | Provider TLS profile
`spec.llm.providers[].providerType` | `string` | -- | No | Pattern `^(inline\|remote)::[a-z0-9][a-z0-9_-]*$` | Llama Stack provider type
`spec.llm.providers[].config` | `*RawExtension` | -- | No | PreserveUnknownFields | Llama Stack config blob
`spec.llm.providers[].credentialKey` | `string` | -- | No | XValidation (rule 13) | Secret key name
`spec.ols` | `OLSSpec` | -- | Yes | -- | OLS settings
`spec.ols.defaultModel` | `string` | -- | Yes | -- | Default model name
`spec.ols.defaultProvider` | `string` | -- | Yes | -- | Default provider name
`spec.ols.logLevel` | `LogLevel` | `INFO` | No | Enum: DEBUG/INFO/WARNING/ERROR/CRITICAL | Log level
`spec.ols.conversationCache` | `ConversationCacheSpec` | -- | No | -- | Cache config
`spec.ols.conversationCache.type` | `CacheType` | `postgres` | No | Enum: `postgres` | Cache type
`spec.ols.conversationCache.postgres` | `PostgresSpec` | -- | No | -- | Postgres settings
`spec.ols.conversationCache.postgres.sharedBuffers` | `string` | `"256MB"` | No | XIntOrString | Shared buffers
`spec.ols.conversationCache.postgres.maxConnections` | `int` | `2000` | No | Min=1, Max=262143 | Max connections
`spec.ols.deployment` | `DeploymentConfig` | -- | No | -- | Deployment overrides
`spec.ols.deployment.api` | `Config` | -- | No | -- | API container
`spec.ols.deployment.api.replicas` | `*int32` | `1` | No | Min=0 | API replicas (user-configurable)
`spec.ols.deployment.api.resources` | `*ResourceRequirements` | -- | No | -- | API resources
`spec.ols.deployment.api.tolerations` | `[]Toleration` | -- | No | -- | API tolerations
`spec.ols.deployment.api.nodeSelector` | `map[string]string` | -- | No | -- | API node selector
`spec.ols.deployment.api.affinity` | `*Affinity` | -- | No | -- | API affinity
`spec.ols.deployment.api.topologySpreadConstraints` | `[]TopologySpreadConstraint` | -- | No | -- | API topology spread
`spec.ols.deployment.dataCollector` | `ContainerConfig` | -- | No | -- | Data collector container
`spec.ols.deployment.dataCollector.resources` | `*ResourceRequirements` | -- | No | -- | Data collector resources
`spec.ols.deployment.mcpServer` | `ContainerConfig` | -- | No | -- | MCP server container
`spec.ols.deployment.mcpServer.resources` | `*ResourceRequirements` | -- | No | -- | MCP server resources
`spec.ols.deployment.llamaStack` | `ContainerConfig` | -- | No | -- | Llama Stack container
`spec.ols.deployment.llamaStack.resources` | `*ResourceRequirements` | -- | No | -- | Llama Stack resources
`spec.ols.deployment.console` | `Config` | -- | No | -- | Console container
`spec.ols.deployment.console.replicas` | `*int32` | `1` | No | Min=0 | Console replicas (operator forces 1)
`spec.ols.deployment.console.resources` | `*ResourceRequirements` | -- | No | -- | Console resources
`spec.ols.deployment.console.tolerations` | `[]Toleration` | -- | No | -- | Console tolerations
`spec.ols.deployment.console.nodeSelector` | `map[string]string` | -- | No | -- | Console node selector
`spec.ols.deployment.console.affinity` | `*Affinity` | -- | No | -- | Console affinity
`spec.ols.deployment.console.topologySpreadConstraints` | `[]TopologySpreadConstraint` | -- | No | -- | Console topology spread
`spec.ols.deployment.database` | `Config` | -- | No | -- | Database container
`spec.ols.deployment.database.replicas` | `*int32` | `1` | No | Min=0 | Database replicas (operator forces 1)
`spec.ols.deployment.database.resources` | `*ResourceRequirements` | -- | No | -- | Database resources
`spec.ols.deployment.database.tolerations` | `[]Toleration` | -- | No | -- | Database tolerations
`spec.ols.deployment.database.nodeSelector` | `map[string]string` | -- | No | -- | Database node selector
`spec.ols.deployment.database.affinity` | `*Affinity` | -- | No | -- | Database affinity
`spec.ols.deployment.database.topologySpreadConstraints` | `[]TopologySpreadConstraint` | -- | No | -- | Database topology spread
`spec.ols.queryFilters` | `[]QueryFiltersSpec` | -- | No | -- | Query filters
`spec.ols.queryFilters[].name` | `string` | -- | No | -- | Filter name
`spec.ols.queryFilters[].pattern` | `string` | -- | No | -- | Regex pattern
`spec.ols.queryFilters[].replaceWith` | `string` | -- | No | -- | Replacement text
`spec.ols.userDataCollection` | `UserDataCollectionSpec` | -- | No | -- | Data collection switches
`spec.ols.userDataCollection.feedbackDisabled` | `bool` | -- | No | -- | Disable feedback
`spec.ols.userDataCollection.transcriptsDisabled` | `bool` | -- | No | -- | Disable transcripts
`spec.ols.tlsConfig` | `*TLSConfig` | -- | No | -- | Backend HTTPS TLS config
`spec.ols.tlsConfig.keyCertSecretRef` | `LocalObjectReference` | -- | No | -- | Secret with tls.crt, tls.key, ca.crt
`spec.ols.additionalCAConfigMapRef` | `*LocalObjectReference` | -- | No | -- | Extra CA certs for LLM TLS
`spec.ols.tlsSecurityProfile` | `*TLSSecurityProfile` | -- | No | -- | API endpoint TLS profile
`spec.ols.introspectionEnabled` | `bool` | -- | No | -- | Enable introspection
`spec.ols.mcpKubeServerConfig` | `*MCPKubeServerConfiguration` | -- | No | -- | Built-in MCP kube server config
`spec.ols.mcpKubeServerConfig.timeout` | `int` | `60` | No | Min=5 | Timeout (seconds)
`spec.ols.proxyConfig` | `*ProxyConfig` | -- | No | -- | Proxy settings
`spec.ols.proxyConfig.proxyURL` | `string` | -- | No | Pattern `^https?://.*$` | Proxy URL
`spec.ols.proxyConfig.proxyCACertificate` | `*ProxyCACertConfigMapRef` | -- | No | -- | Proxy CA cert ref
`spec.ols.proxyConfig.proxyCACertificate.name` | `string` | -- | Yes (inline) | -- | ConfigMap name
`spec.ols.proxyConfig.proxyCACertificate.key` | `string` | `"proxy-ca.crt"` | No | -- | Key in ConfigMap
`spec.ols.rag` | `[]RAGSpec` | -- | No | -- | RAG databases
`spec.ols.rag[].image` | `string` | -- | Yes | -- | Container image URL
`spec.ols.rag[].indexPath` | `string` | `"/rag/vector_db"` | No | -- | Path in container
`spec.ols.rag[].indexID` | `string` | `""` | No | -- | Index ID
`spec.ols.quotaHandlersConfig` | `*QuotaHandlersConfig` | -- | No | -- | Token quota config
`spec.ols.quotaHandlersConfig.limitersConfig` | `[]LimiterConfig` | -- | No | -- | Limiter definitions
`spec.ols.quotaHandlersConfig.limitersConfig[].name` | `string` | -- | Yes | -- | Limiter name
`spec.ols.quotaHandlersConfig.limitersConfig[].type` | `string` | -- | Yes | Enum: cluster_limiter, user_limiter | Limiter type
`spec.ols.quotaHandlersConfig.limitersConfig[].initialQuota` | `int` | -- | Yes | Min=0 | Initial token quota
`spec.ols.quotaHandlersConfig.limitersConfig[].quotaIncrease` | `int` | -- | Yes | Min=0 | Quota increase step
`spec.ols.quotaHandlersConfig.limitersConfig[].period` | `string` | -- | Yes | Pattern (rule 36) | Time period
`spec.ols.quotaHandlersConfig.enableTokenHistory` | `bool` | -- | No | -- | Enable token history
`spec.ols.storage` | `*Storage` | -- | No | -- | Persistent storage
`spec.ols.storage.size` | `resource.Quantity` | -- | No | -- | Volume size
`spec.ols.storage.class` | `string` | -- | No | -- | Storage class
`spec.ols.byokRAGOnly` | `bool` | -- | No | -- | Use only BYOK RAG sources
`spec.ols.querySystemPrompt` | `string` | -- | No | -- | Custom system prompt
`spec.ols.maxIterations` | `int` | `5` | No | Min=1 | Max agent iterations
`spec.ols.imagePullSecrets` | `[]LocalObjectReference` | -- | No | -- | Image pull secrets
`spec.ols.toolFilteringConfig` | `*ToolFilteringConfig` | -- | No | -- | Tool filtering config
`spec.ols.toolFilteringConfig.alpha` | `float64` | `0.8` | No | XValidation: 0.0-1.0 | Dense/sparse weight
`spec.ols.toolFilteringConfig.topK` | `int` | `10` | No | Min=1, Max=50 | Tools to retrieve
`spec.ols.toolFilteringConfig.threshold` | `float64` | `0.01` | No | XValidation: 0.0-1.0 | Similarity threshold
`spec.ols.toolsApprovalConfig` | `*ToolsApprovalConfig` | -- | No | -- | Tool approval config
`spec.ols.toolsApprovalConfig.approvalType` | `ApprovalType` | `tool_annotations` | No | Enum: never/always/tool_annotations | Approval strategy
`spec.ols.toolsApprovalConfig.approvalTimeout` | `int` | `600` | No | Min=1 | Approval timeout (seconds)
`spec.olsDataCollector` | `OLSDataCollectorSpec` | -- | No | -- | Data collector settings
`spec.olsDataCollector.logLevel` | `LogLevel` | `INFO` | No | Enum: DEBUG/INFO/WARNING/ERROR/CRITICAL | Data collector log level
`spec.mcpServers` | `[]MCPServerConfig` | -- | No | MaxItems=20 | External MCP servers
`spec.mcpServers[].name` | `string` | -- | Yes | -- | Server name
`spec.mcpServers[].url` | `string` | -- | Yes | Pattern `^https?://.*$` | Server URL
`spec.mcpServers[].timeout` | `int` | `5` | No | -- | Timeout (seconds)
`spec.mcpServers[].headers` | `[]MCPHeader` | -- | No | MaxItems=20 | HTTP headers
`spec.mcpServers[].headers[].name` | `string` | -- | Yes | MinLen=1, Pattern `^[A-Za-z0-9-]+$` | Header name
`spec.mcpServers[].headers[].valueFrom` | `MCPHeaderValueSource` | -- | Yes | -- | Value source
`spec.mcpServers[].headers[].valueFrom.type` | `MCPHeaderSourceType` | -- | Yes | Enum: secret/kubernetes/client | Source type
`spec.mcpServers[].headers[].valueFrom.secretRef` | `*LocalObjectReference` | -- | Conditional | XValidation (rules 49-50) | Secret reference
`spec.featureGates` | `[]FeatureGate` | -- | No | Enum per item: MCPServer/ToolFiltering | Feature gates
`status.conditions` | `[]metav1.Condition` | -- | -- | -- | Component conditions
`status.overallStatus` | `OverallStatus` | -- | -- | Enum: Ready/NotReady | Aggregate health
`status.diagnosticInfo` | `[]PodDiagnostic` | -- | -- | -- | Pod failure diagnostics
`status.diagnosticInfo[].failedComponent` | `string` | -- | -- | -- | Component name
`status.diagnosticInfo[].podName` | `string` | -- | -- | -- | Pod name
`status.diagnosticInfo[].containerName` | `string` | -- | -- | -- | Container name
`status.diagnosticInfo[].reason` | `string` | -- | -- | -- | Failure reason
`status.diagnosticInfo[].message` | `string` | -- | -- | -- | Error message
`status.diagnosticInfo[].exitCode` | `*int32` | -- | -- | -- | Container exit code
`status.diagnosticInfo[].type` | `DiagnosticType` | -- | -- | Enum (see rule 53) | Diagnostic category
`status.diagnosticInfo[].lastUpdated` | `metav1.Time` | -- | -- | -- | Collection timestamp

## Constraints

1. `.metadata.name` must be `"cluster"` (XValidation on OLSConfig type).
2. Only `azure_openai` provider type uses `deploymentName`; it is required for that type and forbidden (by convention) for others.
3. Only `watsonx` provider type uses `projectID`; it is required for that type.
4. Llama Stack Generic mode (`providerType` set) is incompatible with legacy provider-specific fields (`deploymentName`, `projectID`, `url`, `apiVersion`).
5. `providerType` and `config` must both be set or both be absent; `providerType` requires `type == "llamaStackGeneric"`.
6. Replicas are only user-configurable for the API container (`spec.ols.deployment.api`). Console and database always run with 1 replica enforced by the operator.
7. Period format for quota limiters must match the regex pattern in rule 36, enforcing human-readable duration strings with correct singular/plural agreement.
8. `credentialKey` if set must contain at least one non-whitespace character.
9. Tool filtering requires the `ToolFiltering` feature gate in `spec.featureGates`.
10. MCP server functionality requires the `MCPServer` feature gate in `spec.featureGates`.
11. There is exactly one allowed CacheType value: `postgres`.
12. `ToolFilteringConfig.alpha` and `ToolFilteringConfig.threshold` are validated via XValidation (not kubebuilder min/max) to enforce 0.0-1.0 range.
