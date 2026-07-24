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
`spec.audit` | `audit` | `AuditConfig` | No | OTEL Collector audit log storage and trace forwarding. Does not configure lightspeed-service. Value type (not pointer).
`spec.agenticOLS` | `agenticOLS` | `*AgenticOLSSpec` | No | Classic→agentic sandbox handoff settings. When omitted, sandbox mode is treated as `bare-pod`.

### Audit Configuration (spec.audit)

Collector-only settings for the in-cluster OTEL Collector ([OLS-3505](https://redhat.atlassian.net/browse/OLS-3505)). See `templog.md` for operand behavior. **Does not** propagate into `olsconfig.yaml`.

#### AuditConfig Fields

Field path (relative to `spec.audit`) | JSON key | Go type | Required | Default | Validation | Description
---|---|---|---|---|---|---
`logging` | `logging` | `*bool` | No | `true` when absent | Optional | Enable Collector logs → Postgres pipeline
`tracingEndpoint` | `tracingEndpoint` | `string` | No | (empty) | MaxLength=253 | OTLP trace export backend (e.g. `"jaeger:4317"`). TLS always used by collector.

#### Audit Behavioral Rules

54. `spec.audit` is a value type (`Audit AuditConfig`). Go's `encoding/json` always serializes it as at least `{}` when other spec fields are present; the tag has no `omitempty`. No helper methods on `AuditConfig`.
55. `spec.audit.logging` defaults to **enabled** (`true`) when absent. When `false`, the collector omits the Postgres logs pipeline ([OLS-3510](https://redhat.atlassian.net/browse/OLS-3510)).
56. `spec.audit.tracingEndpoint` is optional. When set, the collector forwards traces to that backend with TLS ([OLS-3510](https://redhat.atlassian.net/browse/OLS-3510)).
57. Service stdout audit and in-cluster trace export are configured separately — see `spec.ols.auditEventsEnabled` and `audit-logging.md`.

#### Removed (breaking change)

- `AuditLoggingMode`, `AuditOTELConfig`, `AuditOTELTLSMode` types
- `spec.audit.otel.endpoint`, `spec.audit.otel.tlsMode`
- `AuditConfig.LoggingEnabled()`, `OTELEndpoint()`, `OTELInsecure()` helpers
- Previous semantics: `spec.audit.logging` as `Enabled`/`Disabled` stdout enum — replaced by `spec.ols.auditEventsEnabled`

### Agentic OLS Configuration (spec.agenticOLS)

Settings consumed by the classic operator when publishing the agentic handoff ConfigMap (`lightspeed-agentic-configuration`). See OLS-3683 / OLS-3684.

#### AgenticOLSSpec Fields

Field path (relative to `spec.agenticOLS`) | JSON key | Go type | Required | Default | Validation | Description
---|---|---|---|---|---|---
`sandboxMode` | `sandboxMode` | `SandboxMode` | No | `bare-pod` | Enum: `bare-pod`, `sandbox-claim` | How the agentic operator provisions sandbox pods
`agenticSandboxConfig` | `agenticSandboxConfig` | `Config` | No | — | — | Resources, tolerations, nodeSelector for the thin sandbox PodSpec. Replicas ignored.

#### AgenticOLS Behavioral Rules

58. `spec.agenticOLS` is optional (pointer). When omitted or `sandboxMode` is empty, the operator treats sandbox mode as `bare-pod`.
59. `sandboxMode=bare-pod` runs agent sandboxes as bare Pods (no Agent Sandbox API CRDs required). `sandbox-claim` uses the Agent Sandbox API.
60. OpenAPI enum validation rejects values other than `bare-pod` and `sandbox-claim`.
61. `agenticSandboxConfig` overrides default sandbox PodSpec scheduling/resources (requests-only defaults: 500m CPU / 128Mi memory). Replicas are ignored.
62. Classic operator publishes handoff via appserver-owned client CA Secrets plus `agenticintegration` ConfigMap (`lightspeed-agentic-configuration`). See `agentic-sandbox-profile.md`.

### LLM Provider Configuration (spec.llm)

7. `spec.llm.providers` is required. Type: `[]ProviderSpec`. MaxItems=10.

#### ProviderSpec Fields

Field path (relative to each provider) | JSON key | Go type | Required | Description
---|---|---|---|---
`name` | `name` | `string` | Yes | Provider name
`url` | `url` | `string` | No | Provider API URL. Pattern: `^https?://.*$`
`credentialsSecretRef` | `credentialsSecretRef` | `corev1.LocalObjectReference` | Yes | Secret containing API credentials
`models` | `models` | `[]ModelSpec` | Yes | Provider models. MaxItems=50
`type` | `type` | `string` | Yes | Provider type enum: `azure_openai`, `bam`, `openai`, `watsonx`, `rhoai_vllm`, `rhelai_vllm`, `fake_provider`, `google_vertex`, `google_vertex_anthropic`, `bedrock`
`deploymentName` | `deploymentName` | `string` | No | Azure OpenAI deployment name
`apiVersion` | `apiVersion` | `string` | No | Azure OpenAI API version
`projectID` | `projectID` | `string` | No | Watsonx project ID
`googleVertexConfig` | `googleVertexConfig` | `*VertexConfig` | No | Google Vertex provider configuration. Required when `type == "google_vertex"`, forbidden otherwise
`googleVertexAnthropicConfig` | `googleVertexAnthropicConfig` | `*VertexConfig` | No | Google Vertex Anthropic provider configuration. Required when `type == "google_vertex_anthropic"`, forbidden otherwise
`fakeProviderMCPToolCall` | `fakeProviderMCPToolCall` | `bool` | No | Fake provider MCP tool call flag
`tlsSecurityProfile` | `tlsSecurityProfile` | `*configv1.TLSSecurityProfile` | No | TLS profile for provider connection
`credentialKey` | `credentialKey` | `string` | No | Key name within `credentialsSecretRef` to read credential from. Defaults to `"apitoken"` if unset

#### VertexConfig Fields

Field path (relative to VertexConfig) | JSON key | Go type | Required | Description
---|---|---|---|---
`projectID` | `projectID` | `string` | No | Google Cloud project ID
`location` | `location` | `string` | No | Server region location

For `type == "bedrock"`, use provider `url` for the Mantle gateway endpoint and `credentialsSecretRef` for authentication (Bearer `apitoken` or IAM keys `aws_access_key_id` / `aws_secret_access_key`, with optional `role_arn`). The operator validates credentials at reconcile time and maps them to `credentials_path` for the service.

#### Provider XValidation Rules

8. Azure OpenAI requires `deploymentName`: when `type == "azure_openai"`, `deploymentName` must not be empty.
9. Watsonx requires `projectID`: when `type == "watsonx"`, `projectID` must not be empty.
10. `credentialKey` must not be empty or whitespace: if set, it must not match `^[ \t\n\r\v\f]*$`.
11. Google Vertex requires `googleVertexConfig`: when `type == "google_vertex"`, `googleVertexConfig` must be present.
12. Google Vertex Anthropic requires `googleVertexAnthropicConfig`: when `type == "google_vertex_anthropic"`, `googleVertexAnthropicConfig` must be present.
13. `googleVertexConfig` may only be set when `type == "google_vertex"`.
14. `googleVertexAnthropicConfig` may only be set when `type == "google_vertex_anthropic"`.

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
`toolBudgetRatio` | `toolBudgetRatio` | `float64` | No | `0.5` | Minimum=0.1, Maximum=0.5
`reasoningConfig` | `reasoningConfig` | `map[string]interface{}` | No | (unset) | None. [PLANNED: OLS-3442] Freeform map of provider-specific reasoning/thinking parameters. Passed through to the service as `reasoning_config`. Valid keys vary by provider and model generation — see lightspeed-service `what/llm-providers.md` rule 13. When absent, no reasoning params are sent. When present with invalid keys, the provider API returns a clear 400 error.

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

- **`Config`**: has `replicas`, `resources`, `tolerations`, `nodeSelector`. Affinity and topology spread constraints are intentionally omitted to keep the CRD OpenAPI schema under the Kubernetes annotation size limit (controller-gen inlines `Config` per operand).
- **`ContainerConfig`**: has `resources` only.

Field path (relative to `spec.ols.deployment`) | JSON key | Go type | Notes
---|---|---|---
`api` | `api` | `Config` | API container. Replicas configurable (default 1, min 0)
`dataCollector` | `dataCollector` | `ContainerConfig` | Data collector container. Resources only
`mcpServer` | `mcpServer` | `Config` | Standalone OpenShift MCP server Deployment (replicas, resources, tolerations, nodeSelector)
`rhokp` | `rhokp` | `ContainerConfig` | RHOKP sidecar container (Solr / OKP). Resources only
`console` | `console` | `Config` | Console container. Has replicas field but operator forces 1
`database` | `database` | `Config` | Database container. Has replicas field but operator forces 1
`alertsAdapter` | `alertsAdapter` | `AlertsAdapterSpec` | Agentic alerts adapter deployment and user-managed runtime config reference. Replicas forced to 1

`AlertsAdapterSpec` embeds `Config` (deployment scheduling/resources) and optional `configMapRef` (`LocalObjectReference`). Setting `configMapRef` **enables** the alerts adapter operand. The referenced ConfigMap name is `configMapRef.name` (commonly `alerts-adapter-config`; see [adapter manifests](https://github.com/openshift/lightspeed-agentic-alerts-adapter/tree/main/manifests)). The operator does not create or validate ConfigMap content. When the ConfigMap exists, it is mounted at `/etc/alerts-adapter`; when absent, no config volume is mounted. The adapter reads `config.yaml` from that path and uses built-in defaults when the file is missing or invalid.
`agenticConsole` | `agenticConsole` | `Config` | Agentic console plugin container. Replicas forced to 1
`otelCollector` | `otelCollector` | `Config` | OTEL Collector container ([OLS-3510](https://redhat.atlassian.net/browse/OLS-3510)). Replicas forced to 1

20. Replicas are user-configurable for the API container (`spec.ols.deployment.api.replicas`). For console, database, alerts adapter, agentic console, and otel collector, the operator always overrides replicas to 1 regardless of spec value.

##### Config Fields

Field path (relative to Config) | JSON key | Go type | Default | Validation
---|---|---|---|---
`replicas` | `replicas` | `*int32` | `1` | Minimum=0
`resources` | `resources` | `*corev1.ResourceRequirements` | (none) | Standard k8s resource requirements
`tolerations` | `tolerations` | `[]corev1.Toleration` | (none) | Standard k8s tolerations
`nodeSelector` | `nodeSelector` | `map[string]string` | (none) | Key-value label selector

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

28. `spec.ols.introspectionEnabled` -- `*bool`, optional. Default: `true` when absent. Enables introspection features (built-in OpenShift MCP server).

#### Service audit events (spec.ols.auditEventsEnabled)

29. `spec.ols.auditEventsEnabled` -- `*bool`, optional. Default: **`true`** when absent. Controls structured compliance audit JSON on stdout by lightspeed-service.
30. Maps to `audit.logging: Enabled|Disabled` in generated `olsconfig.yaml`. Independent of `spec.audit.logging` (collector Postgres pipeline).

#### MCP Kubernetes Server (spec.ols.mcpKubeServerConfig)

31. `spec.ols.mcpKubeServerConfig.timeout` -- `int`. Default: `60`. Minimum=5. Timeout in seconds for the built-in MCP Kubernetes server.

#### Proxy Configuration (spec.ols.proxyConfig)

32. `spec.ols.proxyConfig.proxyURL` -- `string`, optional. Pattern: `^https?://.*$`. If unset, cluster-wide proxy is used via `https_proxy` env var.
33. `spec.ols.proxyConfig.proxyCACertificate` -- `*ProxyCACertConfigMapRef`, optional. Struct type `atomic`.

`ProxyCACertConfigMapRef` fields:
- Inline `corev1.LocalObjectReference` (provides `name` field for the ConfigMap name)
- `key` -- `string`. Default: `"proxy-ca.crt"`. Key within the ConfigMap holding the proxy CA certificate.

#### RAG Configuration (spec.ols.rag)

34. Type: `[]RAGSpec`, optional.

Field | JSON key | Go type | Required | Default
---|---|---|---|---
`image` | `image` | `string` | Yes | (none)
`indexPath` | `indexPath` | `string` | No | `"/rag/vector_db"`
`indexID` | `indexID` | `string` | No | `""`

#### Quota Handlers (spec.ols.quotaHandlersConfig)

35. `spec.ols.quotaHandlersConfig` -- `*QuotaHandlersConfig`, optional.
36. `spec.ols.quotaHandlersConfig.limitersConfig` -- `[]LimiterConfig`.
37. `spec.ols.quotaHandlersConfig.enableTokenHistory` -- `bool`, optional.

`LimiterConfig` fields:

Field | JSON key | Go type | Required | Validation
---|---|---|---|---
`name` | `name` | `string` | Yes (by convention) | None
`type` | `type` | `string` | Yes (by convention) | Enum: `cluster_limiter`, `user_limiter`
`initialQuota` | `initialQuota` | `int` | Yes (by convention) | Minimum=0
`quotaIncrease` | `quotaIncrease` | `int` | Yes (by convention) | Minimum=0
`period` | `period` | `string` | Yes (by convention) | Pattern: `^(1\s+(second\|minute\|hour\|day\|month\|year\|s\|min\|h\|d\|m\|y)\|([2-9][0-9]*\|[1-9][0-9]{2,})\s+(seconds\|minutes\|hours\|days\|months\|years\|s\|min\|h\|d\|m\|y))$`

38. Period pattern explanation: quantity 1 requires singular unit name or abbreviation; quantities >= 2 require plural unit name or abbreviation. Abbreviations (`s`, `min`, `h`, `d`, `m`, `y`) are accepted with any quantity.

#### Storage (spec.ols.storage)

39. `spec.ols.storage.size` -- `resource.Quantity`, optional. Size of the requested persistent volume.
40. `spec.ols.storage.class` -- `string`, optional. Storage class name.

#### Boolean/String Fields

41. `spec.ols.byokRAGOnly` -- `bool`, optional. When true, only BYOK RAG sources are used: the operator does not deploy the RHOKP sidecar, does not write `solr_hybrid` into `olsconfig.yaml`, and does not set `OCP_CLUSTER_VERSION` on the app-server pod.

#### Operator-managed OKP (not on CR)

OKP / Solr hybrid RAG has no `spec.ols.solrHybrid` (or similar) field. It is enabled by default and turned off only via `byokRAGOnly`. When active, the operator:
- deploys the RHOKP sidecar and writes `ols_config.solr_hybrid` with operator defaults (`http://localhost:9080`, hybrid tuning);
- sets `OCP_CLUSTER_VERSION` on the app-server container for Solr version filtering;
- serves OCP product documentation via Solr hybrid only; `reference_content.indexes` lists BYOK FAISS indexes from `spec.ols.rag` only.

RHOKP sidecar CPU, memory, and ephemeral storage are overridable via `spec.ols.deployment.rhokp.resources` (defaults: 2 CPU, 2 GiB memory, and 75 GiB ephemeral storage requests).
42. `spec.ols.querySystemPrompt` -- `string`, optional. Custom system prompt for LLM queries. If unset, the default OpenShift Lightspeed prompt is used.
43. `spec.ols.maxIterations` -- `int`. Default: `5`. Minimum=1. Maximum number of iterations for agent execution.
44. `spec.ols.imagePullSecrets` -- `[]corev1.LocalObjectReference`, optional. Pull secrets for BYOK RAG images.

#### Tool Filtering (spec.ols.toolFilteringConfig)

45. `spec.ols.toolFilteringConfig` -- `*ToolFilteringConfig`, optional. Presence enables tool filtering; absence means all tools are used.

Field | JSON key | Go type | Default | Validation
---|---|---|---|---
`alpha` | `alpha` | `float64` | `0.8` | XValidation: must be >= 0.0 and <= 1.0. Weight for dense vs sparse retrieval (1.0 = full dense, 0.0 = full sparse)
`topK` | `topK` | `int` | `10` | Minimum=1, Maximum=50. Number of tools to retrieve
`threshold` | `threshold` | `float64` | `0.01` | XValidation: must be >= 0.0 and <= 1.0. Minimum similarity threshold

46. Tool filtering requires the `ToolFiltering` feature gate to be enabled in `spec.featureGates`.

#### Tools Approval (spec.ols.toolsApprovalConfig)

47. `spec.ols.toolsApprovalConfig` -- `*ToolsApprovalConfig`, optional.

Field | JSON key | Go type | Default | Validation
---|---|---|---|---
`approvalType` | `approvalType` | `ApprovalType` | `tool_annotations` | Enum: `never`, `always`, `tool_annotations`
`approvalTimeout` | `approvalTimeout` | `int` | `600` | Minimum=1. Timeout in seconds for user approval

48. `never`: all tools execute without approval. `always`: all tool calls require approval. `tool_annotations`: approval decision is per-tool based on annotations.

### Data Collector Configuration (spec.olsDataCollector)

49. `spec.olsDataCollector.logLevel` -- `LogLevel` enum. Default: `INFO`. Same enum as `spec.ols.logLevel`.

### MCP Server Configuration (spec.mcpServers)

50. Array of `MCPServerConfig`. MaxItems=20.

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

51. XValidation: when `type == "secret"`, `secretRef` must be present with a non-empty `name`.
52. XValidation: when `type != "secret"` (i.e., `kubernetes` or `client`), `secretRef` must not be set.

### Status (status)

#### Conditions (status.conditions)

53. Type: `[]metav1.Condition`. Populated after first reconciliation.

Condition types used by the operator:
- `ApiReady` -- API server deployment health
- `CacheReady` -- PostgreSQL cache deployment health
- `ConsolePluginReady` -- Console UI plugin deployment health
- `AgenticConsolePluginReady` -- Agentic console plugin deployment health
- `OtelCollectorReady` -- OTEL Collector deployment health
- `AlertsAdapterReady` -- Agentic alerts adapter deployment health
- `ResourceReconciliation` -- Overall resource reconciliation status (set directly, not deployment-based)

#### Overall Status (status.overallStatus)

54. `status.overallStatus` -- `OverallStatus` enum. Values: `Ready`, `NotReady`. Aggregation of all component conditions. `Ready` only when all components are healthy.

#### Diagnostic Info (status.diagnosticInfo)

55. Type: `[]PodDiagnostic`, optional. Auto-populated during deployment failures, cleared on recovery.

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
`spec.llm.providers[].models[].parameters.reasoningConfig` | `map[string]interface{}` | -- | No | -- | [PLANNED: OLS-3442] Provider-specific reasoning/thinking params
`spec.llm.providers[].type` | `string` | -- | Yes | Enum (see rule 7; includes `bedrock`) | Provider type
`spec.llm.providers[].deploymentName` | `string` | -- | No | XValidation (rule 8) | Azure deployment name
`spec.llm.providers[].apiVersion` | `string` | -- | No | -- | Azure API version
`spec.llm.providers[].projectID` | `string` | -- | No | XValidation (rule 9) | Watsonx project ID
`spec.llm.providers[].googleVertexConfig` | `*VertexConfig` | -- | No | XValidation (rules 11, 13) | Google Vertex config
`spec.llm.providers[].googleVertexConfig.projectID` | `string` | -- | No | -- | Google Cloud project ID
`spec.llm.providers[].googleVertexConfig.location` | `string` | -- | No | -- | Server region location
`spec.llm.providers[].googleVertexAnthropicConfig` | `*VertexConfig` | -- | No | XValidation (rules 12, 14) | Google Vertex Anthropic config
`spec.llm.providers[].googleVertexAnthropicConfig.projectID` | `string` | -- | No | -- | Google Cloud project ID
`spec.llm.providers[].googleVertexAnthropicConfig.location` | `string` | -- | No | -- | Server region location
`spec.llm.providers[].fakeProviderMCPToolCall` | `bool` | -- | No | -- | Fake provider MCP flag
`spec.llm.providers[].tlsSecurityProfile` | `*TLSSecurityProfile` | -- | No | -- | Provider TLS profile
`spec.llm.providers[].credentialKey` | `string` | -- | No | XValidation (rule 10) | Secret key name
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
`spec.ols.deployment.dataCollector` | `ContainerConfig` | -- | No | -- | Data collector container
`spec.ols.deployment.dataCollector.resources` | `*ResourceRequirements` | -- | No | -- | Data collector resources
`spec.ols.deployment.mcpServer` | `Config` | -- | No | -- | Standalone OpenShift MCP server Deployment
`spec.ols.deployment.mcpServer.resources` | `*ResourceRequirements` | -- | No | -- | MCP server resources
`spec.ols.deployment.rhokp` | `ContainerConfig` | -- | No | -- | RHOKP sidecar container
`spec.ols.deployment.rhokp.resources` | `*ResourceRequirements` | -- | No | -- | RHOKP sidecar resources (default requests: 2 CPU, 2 GiB memory, 75 GiB ephemeral storage)
`spec.ols.deployment.console` | `Config` | -- | No | -- | Console container
`spec.ols.deployment.console.replicas` | `*int32` | `1` | No | Min=0 | Console replicas (operator forces 1)
`spec.ols.deployment.console.resources` | `*ResourceRequirements` | -- | No | -- | Console resources
`spec.ols.deployment.console.tolerations` | `[]Toleration` | -- | No | -- | Console tolerations
`spec.ols.deployment.console.nodeSelector` | `map[string]string` | -- | No | -- | Console node selector
`spec.ols.deployment.database` | `Config` | -- | No | -- | Database container
`spec.ols.deployment.database.replicas` | `*int32` | `1` | No | Min=0 | Database replicas (operator forces 1)
`spec.ols.deployment.database.resources` | `*ResourceRequirements` | -- | No | -- | Database resources
`spec.ols.deployment.database.tolerations` | `[]Toleration` | -- | No | -- | Database tolerations
`spec.ols.deployment.database.nodeSelector` | `map[string]string` | -- | No | -- | Database node selector
`spec.ols.deployment.alertsAdapter` | `AlertsAdapterSpec` | -- | No | -- | Alerts adapter deployment and config reference
`spec.ols.deployment.alertsAdapter.configMapRef` | `LocalObjectReference` | (none) | No | -- | Opt-in switch and runtime config reference: ConfigMap name in operator namespace; mounted at `/etc/alerts-adapter` when present (adapter reads `config.yaml`)
`spec.ols.deployment.alertsAdapter.replicas` | `*int32` | `1` | No | Min=0 | Alerts adapter replicas (operator forces 1)
`spec.ols.deployment.alertsAdapter.resources` | `*ResourceRequirements` | -- | No | -- | Alerts adapter resources
`spec.ols.deployment.alertsAdapter.tolerations` | `[]Toleration` | -- | No | -- | Alerts adapter tolerations
`spec.ols.deployment.alertsAdapter.nodeSelector` | `map[string]string` | -- | No | -- | Alerts adapter node selector
`spec.ols.deployment.agenticConsole` | `Config` | -- | No | -- | Agentic console deployment
`spec.ols.deployment.agenticConsole.replicas` | `*int32` | `1` | No | Min=0 | Agentic console replicas (operator forces 1)
`spec.ols.deployment.agenticConsole.resources` | `*ResourceRequirements` | -- | No | -- | Agentic console resources
`spec.ols.deployment.agenticConsole.tolerations` | `[]Toleration` | -- | No | -- | Agentic console tolerations
`spec.ols.deployment.agenticConsole.nodeSelector` | `map[string]string` | -- | No | -- | Agentic console node selector
`spec.ols.deployment.otelCollector` | `Config` | -- | No | -- | OTEL Collector deployment ([OLS-3510](https://redhat.atlassian.net/browse/OLS-3510))
`spec.ols.deployment.otelCollector.replicas` | `*int32` | `1` | No | Min=0 | Collector replicas (operator forces 1)
`spec.ols.deployment.otelCollector.resources` | `*ResourceRequirements` | -- | No | -- | Collector resources
`spec.ols.deployment.otelCollector.tolerations` | `[]Toleration` | -- | No | -- | Collector tolerations
`spec.ols.deployment.otelCollector.nodeSelector` | `map[string]string` | -- | No | -- | Collector node selector
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
`spec.ols.introspectionEnabled` | `*bool` | `true` | No | -- | Enable introspection
`spec.ols.auditEventsEnabled` | `*bool` | `true` | No | -- | Stdout compliance audit JSON events
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
`spec.ols.quotaHandlersConfig.limitersConfig[].period` | `string` | -- | Yes | Pattern (rule 38) | Time period
`spec.ols.quotaHandlersConfig.enableTokenHistory` | `bool` | -- | No | -- | Enable token history
`spec.ols.storage` | `*Storage` | -- | No | -- | Persistent storage
`spec.ols.storage.size` | `resource.Quantity` | -- | No | -- | Volume size
`spec.ols.storage.class` | `string` | -- | No | -- | Storage class
`spec.ols.byokRAGOnly` | `bool` | -- | No | -- | Disable operator-managed OKP; BYOK FAISS only
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
`spec.audit` | `AuditConfig` | -- | No | -- | Collector audit log storage and trace forwarding
`spec.audit.logging` | `*bool` | `true` | No | Optional | Collector Postgres logs pipeline
`spec.audit.tracingEndpoint` | `string` | -- | No | MaxLen=253 | Collector external OTLP trace export (TLS)
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
4. Replicas are user-configurable for the API container (`spec.ols.deployment.api`). Console, database, alerts adapter, agentic console, and otel collector always run with 1 replica enforced by the operator.
5. Period format for quota limiters must match the regex pattern in rule 38, enforcing human-readable duration strings with correct singular/plural agreement.
6. `credentialKey` if set must contain at least one non-whitespace character.
7. Tool filtering requires the `ToolFiltering` feature gate in `spec.featureGates`.
8. User-defined MCP servers (`spec.mcpServers`) require the `MCPServer` feature gate in `spec.featureGates`. The built-in openshift MCP server is the standalone `ocpmcp` operand controlled exclusively by `spec.ols.introspectionEnabled` and does not require this gate.
9. There is exactly one allowed CacheType value: `postgres`.
10. `ToolFilteringConfig.alpha` and `ToolFilteringConfig.threshold` are validated via XValidation (not kubebuilder min/max) to enforce 0.0-1.0 range.
11. Bedrock credentials: `credentialsSecretRef` must contain either `apitoken` (Bearer) or both `aws_access_key_id` and `aws_secret_access_key` (IAM). Optional `role_arn` is passed through to the service when present.

## Planned Changes

- [PLANNED: OLS-3442] Add `reasoningConfig` field (`map[string]interface{}`) to `ModelParametersSpec`. Freeform map passed through to the service as `reasoning_config` for provider-specific reasoning/thinking parameters. Includes release notes and user-facing documentation for valid keys per provider.
- [DONE: OLS-3683 / OLS-3684] `spec.agenticOLS` (`sandboxMode`, `agenticSandboxConfig`), appserver-owned client CA Secrets, and handoff ConfigMap (`lightspeed-agentic-configuration`). See `agentic-sandbox-profile.md`.
- [PLANNED: OLS-3594] Optional agentic auto-injection of MCP into agent runs (deferred).
- [PLANNED: OLS-3685+] Agentic-operator consumption of the handoff ConfigMap/Secrets.
