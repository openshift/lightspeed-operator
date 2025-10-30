# LCore OLSConfig Configuration Support

This document provides a comprehensive list of all OLSConfig fields and their implementation status in the `lcore` package (Lightspeed Core with Llama Stack backend).

> **Note**: LCore and AppServer are mutually exclusive backends. Use the `--enable-lcore` flag to enable LCore.

## Legend

- ✅ **Implemented** - Fully supported and tested
- 🔄 **Partially Implemented** - Basic support exists, may need enhancement
- ❌ **Not Implemented** - Not yet supported
- 🚫 **Not Applicable** - Not relevant for LCore architecture

---

## Configuration Fields Status

### Top-Level OLSSpec Fields

| Field | Status | Implementation Details | Location |
|-------|--------|----------------------|----------|
| `logLevel` | ✅ | Maps to `LOG_LEVEL` environment variable in lightspeed-stack container. Supports DEBUG, INFO, WARNING, ERROR, CRITICAL. Also controls `color_log` setting (enabled for DEBUG). | `deployment.go` (env var), `assets.go:buildLCoreServiceConfig()` (color_log) |
| `defaultModel` | ✅ | Passed to LCore inference config as `default_model` | `assets.go:buildLCoreInferenceConfig()` |
| `defaultProvider` | ✅ | Passed to LCore inference config as `default_provider` | `assets.go:buildLCoreInferenceConfig()` |
| `queryFilters` | ❌ | Regex-based query manipulation. Requires LCore service implementation. | Not implemented |
| `userDataCollection` | ✅ | Controls feedback and transcripts collection via `FeedbackDisabled` and `TranscriptsDisabled` flags | `assets.go:buildLCoreUserDataCollectionConfig()` |
| `tlsConfig` | 🔄 | Default TLS cert mounted (`lightspeed-tls`). Custom certs via `KeyCertSecretRef` not yet implemented. | `deployment.go` |
| `additionalCAConfigMapRef` | ✅ | Mounts additional CA certificates for outbound LLM connections | `deployment.go`, `reconciler.go` |
| `tlsSecurityProfile` | ❌ | OpenShift TLS security profile | Not implemented |
| `introspectionEnabled` | ❌ | Debug/introspection features. Unclear mapping to Llama Stack. | Not implemented |
| `proxyConfig` | 🔄 | Proxy CA ConfigMap reconciliation exists, but HTTP_PROXY env vars not set | `reconciler.go:reconcileProxyCAConfigMap()` |
| `rag` | 🔄 | Vector DB config generated in Llama Stack config, but volume mounting and init containers not implemented | `assets.go:buildLlamaStackVectorDBs()` |
| `quotaHandlersConfig` | ❌ | Token usage limits. Requires LCore service implementation. | Not implemented |
| `storage` | ❌ | PVC for persistent storage. Currently using EmptyDir. | Not implemented |
| `byokRAGOnly` | 🔄 | Logic exists in Llama Stack vector DB config but volume mounting not implemented | `assets.go:buildLlamaStackVectorDBs()` |

---

### DeploymentConfig Fields

| Field | Status | Implementation Details | Location |
|-------|--------|----------------------|----------|
| `replicas` | ✅ | Pod replica count. Default: 1 | `deployment.go:getLCoreReplicas()` |
| `api.resources` | ✅ | CPU/memory limits for `lightspeed-stack` container (main API) | `deployment.go:getLightspeedStackResources()` |
| `api.tolerations` | ✅ | Pod toleration of node taints | `deployment.go:GenerateLCoreDeployment()` |
| `api.nodeSelector` | ✅ | Pod scheduling based on node labels | `deployment.go:GenerateLCoreDeployment()` |
| `dataCollector.resources` | 🚫 | Not applicable - LCore doesn't use separate data collector container | N/A |
| `console.resources` | 🚫 | Console UI is independent of backend choice | Managed by `console` package |
| `console.tolerations` | 🚫 | Console UI is independent of backend choice | Managed by `console` package |
| `database.resources` | 🚫 | PostgreSQL is independent of backend choice | Managed by `postgres` package |
| `mcpServer.resources` | ❌ | Could be used for `llama-stack` sidecar, but currently uses fixed resources | `deployment.go:getLlamaStackResources()` |

> **Note on Resources**: 
> - `lightspeed-stack` container (main API): Uses `api.resources` from OLSConfig
> - `llama-stack` sidecar (inference): Uses fixed resources (500m/512Mi requests, 1000m/1Gi limits)

---

### LLMConfig Fields (Spec.LLMConfig)

| Field | Status | Implementation Details | Location |
|-------|--------|----------------------|----------|
| `providers[]` | ✅ | **Fully dynamic configuration** from OLSConfig | `assets.go:buildLlamaStackInferenceProviders()` |
| `providers[].name` | ✅ | Used as `provider_id` in Llama Stack config | - |
| `providers[].type` | ✅ | **Supported**: `openai`, `azure_openai`. **Unsupported**: `watsonx`, `bam`, `rhoai_vllm`, `rhelai_vllm` (returns error) | - |
| `providers[].url` | ✅ | Custom endpoint URL for provider | - |
| `providers[].credentialsSecretRef` | ✅ | API key mounted as env var (`{PROVIDER}_API_KEY`) | `deployment.go:buildLlamaStackEnvVars()` |
| `providers[].models[]` | ✅ | All models registered in Llama Stack config | `assets.go:buildLlamaStackModels()` |
| `providers[].models[].name` | ✅ | Used as `model_id` in Llama Stack | - |
| `providers[].models[].url` | ✅ | Model-specific URL (stored in metadata) | - |
| `providers[].models[].contextWindowSize` | ✅ | Stored in model metadata | - |
| `providers[].models[].parameters.maxTokensForResponse` | ✅ | Stored in model metadata | - |
| `providers[].azureDeploymentName` | ✅ | Azure-specific: `deployment_name` in config | - |
| `providers[].apiVersion` | ✅ | Azure-specific: `api_version` in config | - |
| `providers[].watsonProjectID` | 🚫 | WatsonX not supported by Llama Stack | - |
| `conversationCache` | ❌ | PostgreSQL conversation cache. Llama Stack uses SQLite. | Not implemented |

---

### ProxyConfig Fields

| Field | Status | Implementation Details | Location |
|-------|--------|----------------------|----------|
| `proxyURL` | ❌ | HTTP_PROXY/HTTPS_PROXY env vars not set | Not implemented |
| `proxyCACertificateRef` | 🔄 | ConfigMap reconciliation exists but not mounted | `reconciler.go:reconcileProxyCAConfigMap()` |

---

### TLSConfig Fields

| Field | Status | Implementation Details | Location |
|-------|--------|----------------------|----------|
| `keyCertSecretRef` | 🔄 | Default TLS secret (`lightspeed-tls`) mounted. Custom secret support not implemented. | `deployment.go`, `assets.go` |

---

### UserDataCollectionSpec Fields

| Field | Status | Implementation Details | Location |
|-------|--------|----------------------|----------|
| `feedbackDisabled` | ✅ | Controls feedback collection (inverted: disabled → enabled) | `assets.go:buildLCoreUserDataCollectionConfig()` |
| `transcriptsDisabled` | ✅ | Controls transcripts collection (inverted: disabled → enabled) | `assets.go:buildLCoreUserDataCollectionConfig()` |

---

### RAGSpec Fields

| Field | Status | Implementation Details | Location |
|-------|--------|----------------------|----------|
| `indexPath` | 🔄 | Used in vector DB config but volume mounting not implemented | `assets.go:buildLlamaStackVectorDBs()` |
| `indexID` | 🔄 | Used as `vector_db_id` in Llama Stack config | `assets.go:buildLlamaStackVectorDBs()` |
| `image` | 🔄 | Used in vector DB config but init containers not implemented | `assets.go:buildLlamaStackVectorDBs()` |

---

## Optional LCore Configuration (Commented Out in Code)

The following LCore-specific configurations are **documented as commented-out functions** in `assets.go` for future implementation:

| Configuration | Function | Description | Status |
|--------------|----------|-------------|--------|
| **Database** | `buildLCoreDatabaseConfig()` | Persistent database storage (SQLite file or PostgreSQL) | ❌ Not implemented - currently uses ephemeral SQLite in /tmp |
| **MCP Servers** | `buildLCoreMCPServersConfig()` | Model Context Protocol server integration for agent workflows | ❌ Not implemented |
| **Authorization** | `buildLCoreAuthorizationConfig()` | Role-based access control (admin/user roles, action permissions) | ❌ Not implemented |
| **Customization** | `buildLCoreCustomizationConfig()` | System prompt customization and behavior overrides | ❌ Not implemented |
| **Conversation Cache** | `buildLCoreConversationCacheConfig()` | Chat history caching (memory, SQLite, or PostgreSQL) | ❌ Not implemented |
| **BYOK RAG** | `buildLCoreByokRagConfig()` | Bring Your Own Knowledge custom RAG sources | ❌ Not implemented |
| **Quota Handlers** | `buildLCoreQuotaHandlersConfig()` | Token usage rate limiting (per-user, cluster-wide) | ❌ Not implemented |

> **Note**: These functions are commented out with detailed examples in `internal/controller/lcore/assets.go` (lines 670-838) to serve as documentation and templates for future implementation.

---

## Implementation Priority (Future Work)

### High Priority (Phase 2)
1. ✅ **DefaultModel/DefaultProvider** - ✅ **IMPLEMENTED** - Passed to LCore inference config
2. **ProxyConfig** - Add HTTP_PROXY env vars and mount proxy CA cert (ConfigMap reconciliation exists)
3. **TLSConfig.KeyCertSecretRef** - Support custom TLS certificates
4. **Database (Persistent Storage)** - Replace ephemeral SQLite with persistent volume or PostgreSQL

### Medium Priority (Phase 3)
5. **RAG Volume Mounting** - Add init containers and volume mounts like appserver
6. **Conversation Cache** - Implement chat history caching (memory/SQLite/PostgreSQL options)
7. **QueryFilters** - Requires LCore service implementation
8. **MCP Servers** - Model Context Protocol integration for agent workflows

### Low Priority
9. **Authorization** - Role-based access control
10. **Customization** - System prompt customization
11. **QuotaHandlersConfig** - Token usage rate limiting
12. **IntrospectionEnabled** - Unclear Llama Stack mapping
13. **TLSSecurityProfile** - OpenShift-specific TLS hardening

---

## Testing

All implemented features have corresponding unit tests in:
- `internal/controller/lcore/assets_test.go`
- `internal/controller/lcore/deployment_test.go`
- `internal/controller/lcore/reconciler_test.go`

Current test coverage: **75.3%**

Recent improvements:
- Replaced hardcoded test literals with constants from `utils/constants.go`
- Added 11 new LCore-specific test constants for better maintainability

---

## Architecture Notes

### Resource Allocation Strategy

The LCore deployment uses a **dual-container pod**:

1. **llama-stack** (sidecar):
   - Runs Llama Stack inference service
   - **Fixed resources**: 500m/512Mi requests, 1000m/1Gi limits
   - Not configurable via OLSConfig (ensures stable inference performance)

2. **lightspeed-stack** (main):
   - Runs Lightspeed Core API service
   - **Configurable**: Via `DeploymentConfig.APIContainer.Resources`
   - Default: 500m/512Mi requests, 1000m/1Gi limits

### Provider Support

**Supported LLM Providers** (Llama Stack compatible):
- ✅ OpenAI
- ✅ Azure OpenAI

**Unsupported Providers** (handled by LCore service directly, not Llama Stack):
- ❌ WatsonX
- ❌ BAM
- ❌ RHOAI vLLM
- ❌ RHELAI vLLM

When unsupported providers are used, the operator returns a clear error message.

### Model Warmup

The `llama-stack` container includes a startup script that:
1. Starts Llama Stack in background
2. Waits for health check
3. Pre-loads embedding model (sentence-transformers)
4. Pre-loads LLM model (triggers Llama Guard safety model)

This eliminates first-request latency (~15 seconds for embedding model load).

---

## Related Documentation

- [ARCHITECTURE.md](../ARCHITECTURE.md) - Full architecture overview
- [CLAUDE.md](../CLAUDE.md) - AI assistant guide for development
- [CONTRIBUTING.md](../CONTRIBUTING.md) - Development guidelines

---

**Last Updated**: November 11, 2025
**LCore Package Version**: Coverage 75.3%

