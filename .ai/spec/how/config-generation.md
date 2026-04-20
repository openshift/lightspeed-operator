# Config Generation

## Module Map

| File | Key Functions | Responsibility |
|---|---|---|
| `internal/controller/appserver/assets.go` | `GenerateOLSConfigMap()`, `buildProviderConfigs()`, `buildOLSConfig()`, `generateMCPServerConfigs()`, `buildToolFilteringConfig()` | AppServer OLS config YAML (olsconfig.yaml) |
| `internal/controller/lcore/config.go` | `buildLlamaStackYAML()`, `buildLCoreConfigYAML()`, `buildLlamaStackInferenceProviders()`, `getProviderType()` | Llama Stack config (run.yaml) + LCore config (lightspeed-stack.yaml) |
| `internal/controller/postgres/assets.go` | `GeneratePostgresConfigMap()`, `GeneratePostgresBootstrapSecret()`, `GeneratePostgresSecret()` | PostgreSQL config + bootstrap script + credentials |
| `internal/controller/console/assets.go` | `GenerateConsoleUIConfigMap()` | Nginx config for console plugin |
| `internal/controller/utils/mcp_server_config.go` | `GenerateOpenShiftMCPServerConfigMap()` | MCP server denied-resources config (TOML) |

## Data Flow

### AppServer OLS Config (olsconfig.yaml)
```
CR spec -> GenerateOLSConfigMap() -> ConfigMap "olsconfig"
```

Generated YAML structure (marshaled from `utils.AppSrvConfigFile`):
```yaml
llm_providers:
  - name: <provider.Name>
    type: <provider.Type>  # direct from CRD enum: openai, azure_openai, etc.
    url: <provider.URL>                     # non-Azure providers
    credentials_path: /etc/apikeys/<secretName>  # mount path to secret dir
    models:
      - name: <model.Name>
        url: <model.URL>
        context_window_size: <model.ContextWindowSize>
        parameters:
          max_tokens_for_response: <model.Parameters.MaxTokensForResponse>
          tool_budget_ratio: <default 0.25 if zero>
    # Azure-specific:
    azure_openai_config:
      url: <provider.URL>
      credentials_path: /etc/apikeys/<secretName>
      azure_deployment_name: <deploymentName>
    api_version: <apiVersion>
    # Watsonx-specific:
    project_id: <projectID>
    # Fake provider:
    fake_provider_config:
      url: "http://example.com"
      response: "This is a preconfigured fake response."
      chunks: 30
      sleep: 0.1
      stream: false
      mcp_tool_call: <fakeProviderMCPToolCall>

ols_config:
  default_model: <spec.ols.defaultModel>
  default_provider: <spec.ols.defaultProvider>
  max_iterations: <spec.ols.maxIterations>
  logging:
    app_log_level: <spec.ols.logLevel>
    lib_log_level: <spec.ols.logLevel>
    uvicorn_log_level: <spec.ols.logLevel>
  conversation_cache:
    type: postgres
    postgres:
      host: lightspeed-postgres-server.<namespace>.svc
      port: 5432
      user: postgres
      db: postgres
      password_path: /etc/credentials/lightspeed-postgres-secret/password
      ssl_mode: require
      ca_cert_path: /etc/certs/postgres-ca/service-ca.crt
  tls_config:
    tls_certificate_path: /etc/certs/lightspeed-tls/tls.crt
    tls_key_path: /etc/certs/lightspeed-tls/tls.key
  reference_content:
    indexes:
      - path: /app-root/rag/rag-0          # BYOK first (one per spec.ols.rag entry)
        index_id: <rag.IndexID>
        origin: <rag.Image>
      - path: /app-root/vector_db/ocp_product_docs/<major>.<minor>  # OCP docs (unless byokRAGOnly)
        index_id: ocp-product-docs-<major>_<minor>
        origin: "Red Hat OpenShift <major>.<minor> documentation"
    embeddings_model_path: /app-root/embeddings_model
  user_data_collection:
    feedback_disabled: <computed: CRvalue || !dataCollectorEnabled>
    feedback_storage: /app-root/ols-user-data/feedback
    transcripts_disabled: <computed: CRvalue || !dataCollectorEnabled>
    transcripts_storage: /app-root/ols-user-data/transcripts
  extra_cas: [<list of cert file paths from kube-root-ca.crt + additional CA CM>]
  certificate_directory: /etc/certs/cert-bundle
  proxy_config:
    proxy_url: <proxyConfig.proxyURL>
    proxy_ca_cert_path: /etc/certs/cm-proxycacert/<certKey>
  query_filters: [{name, pattern, replace_with}]   # if spec.ols.queryFilters set
  system_prompt_path: /etc/ols/system_prompt        # if spec.ols.querySystemPrompt set
  quota_handlers_config:                             # if spec.ols.quotaHandlersConfig set
    storage: <postgres cache config>
    scheduler: {period: 300}
    limiters_config: [{name, type, initial_quota, quota_increase, period}]
    enable_token_history: <bool>
  tool_filtering:                                    # if ToolFiltering gate + MCP servers exist
    alpha: <default 0.8>
    top_k: <default 10>
    threshold: <default 0.01>
  tools_approval:                                    # always present
    approval_type: <default "tool_annotations">
    approval_timeout: <default 600>

mcp_servers:                                         # if any MCP servers configured
  - name: openshift                                  # if introspectionEnabled
    url: http://localhost:<OpenShiftMCPServerPort>
    timeout: <mcpKubeServerConfig.timeout or default 60>
    headers:
      x-kube-auth: "{{KUBERNETES_TOKEN}}"
  - name: <user server>                              # if MCPServer feature gate
    url: <url>
    timeout: <timeout>
    headers:
      <name>: <resolved value>                       # kubernetes -> "{{KUBERNETES_TOKEN}}"
                                                     # client -> "{{CLIENT_TOKEN}}"
                                                     # secret -> /etc/mcp/headers/<secretName>/header

user_data_collector_config:                          # if dataCollectorEnabled
  data_storage: /app-root/ols-user-data
  log_level: <spec.olsDataCollector.logLevel>
```

### Llama Stack Config (run.yaml)
```
CR spec -> buildLlamaStackYAML() -> ConfigMap "llama-stack-config"
```

Structure (built from Go maps, marshaled to YAML):
```yaml
version: "2"
image_name: openshift-lightspeed-configuration
apis: [agents, files, inference, safety, tool_runtime, vector_io]

providers:
  inference:
    - provider_id: sentence-transformers         # always present
      provider_type: inline::sentence-transformers
      config: {}
    - provider_id: <provider.Name>
      provider_type: <mapped type>               # see Provider Type Mapping
      config:
        api_key: "${env.<PROVIDER>_API_KEY}"     # openai, azure
        api_token: "${env.<PROVIDER>_API_KEY}"   # vllm, fake
        url: <provider.URL>                       # if set
        # Azure additional:
        client_id: "${env.<PROVIDER>_CLIENT_ID:=}"
        tenant_id: "${env.<PROVIDER>_TENANT_ID:=}"
        client_secret: "${env.<PROVIDER>_CLIENT_SECRET:=}"
        deployment_name: <azureDeploymentName>
        api_version: <apiVersion>
        api_base: <provider.URL>
  files: [{provider_id: localfs, provider_type: inline::localfs, ...}]
  agents: [{provider_id: meta-reference, provider_type: inline::meta-reference, ...}]
  safety: [{provider_id: llama-guard, provider_type: inline::llama-guard, ...}]
  tool_runtime:
    - {provider_id: model-context-protocol, provider_type: remote::model-context-protocol}
    - {provider_id: rag-runtime, provider_type: inline::rag-runtime}
  vector_io: [{provider_id: faiss, provider_type: inline::faiss, ...}]

models:
  - model_id: sentence-transformers/all-mpnet-base-v2    # always
    model_type: embedding
    provider_id: sentence-transformers
    metadata: {embedding_dimension: 768}
  - model_id: <model.Name>
    model_type: llm
    provider_id: <provider.Name>
    provider_model_id: <model.Name>

vector_dbs:
  - vector_db_id: <rag.IndexID or sanitized image name>
    embedding_model: sentence-transformers/all-mpnet-base-v2
    embedding_dimension: 768
    provider_id: faiss

server:
  host: 0.0.0.0
  port: 8321

storage:
  backends:
    sql_default: {type: sql_sqlite, db_path: /tmp/llama-stack/sql_store.db}
    kv_default: {type: kv_sqlite, db_path: /tmp/llama-stack/kv_store.db}
    postgres_backend:
      type: sql_postgres
      host: lightspeed-postgres-server.openshift-lightspeed.svc
      port: 5432
      user: postgres
      password: "${env.POSTGRES_PASSWORD}"
      ssl_mode: require
      ca_cert_path: /etc/certs/postgres-ca/service-ca.crt
      gss_encmode: disable
  stores:
    metadata: {namespace: registry, backend: kv_default}
    inference: {table_name: inference_store, backend: sql_default}
    conversations: {table_name: openai_conversations, backend: postgres_backend}

tool_groups: [{toolgroup_id: builtin::rag, provider_id: rag-runtime}]
telemetry: {enabled: false}
```

### LCore Config (lightspeed-stack.yaml)
```
CR spec -> buildLCoreConfigYAML() -> ConfigMap "lightspeed-stack-config"
```

Key structural differences from AppServer config:
```yaml
name: "Lightspeed Core Service (LCS)"

service:
  host: 0.0.0.0
  port: 8443
  auth_enabled: false
  workers: 1
  color_log: <true if DEBUG, false otherwise>
  access_log: true
  tls_config:
    tls_certificate_path: /etc/certs/lightspeed-tls/tls.crt
    tls_key_path: /etc/certs/lightspeed-tls/tls.key
  proxy_config:                                     # if configured
    proxy_url: <proxyURL>
    proxy_ca_cert_path: /etc/certs/cm-proxycacert/<certKey>

llama_stack:
  use_as_library_client: <false for server, true for library>
  url: "http://localhost:8321"
  api_key: "xyzzy"
  library_client_config_path: <path>                # only in library mode

authentication:
  module: k8s

inference:
  default_provider: <spec.ols.defaultProvider>
  default_model: <spec.ols.defaultModel>

database:
  postgres:
    host: lightspeed-postgres-server.<namespace>.svc
    port: 5432
    db: postgres                                     # same default db
    user: postgres
    password: "${env.POSTGRES_PASSWORD}"
    ssl_mode: require
    gss_encmode: disable
    ca_cert_path: /etc/certs/postgres-ca/service-ca.crt
    namespace: lcore                                  # separate schema

customization:
  system_prompt: <embedded prompt text, not file path>
  disable_query_system_prompt: true

conversation_cache:
  type: postgres
  postgres:
    host: lightspeed-postgres-server.<namespace>.svc
    # ... same postgres config ...
    namespace: conversation_cache                     # separate schema

user_data_collection:
  feedback_enabled: <bool>
  feedback_storage: /tmp/data/feedback
  transcripts_enabled: <bool>
  transcripts_storage: /tmp/data/transcripts

quota_handlers:                                       # if configured
  limiters: [{type, name, initial_quota, quota_increase, period}]
  scheduler: {period: 300}
  enable_token_history: <bool>
  postgres:
    # ... same postgres config ...
    namespace: quota                                  # separate schema

tools_approval:
  approval_type: <default "tool_annotations">
  approval_timeout: <default 600>

mcp_servers:                                          # if any MCP servers configured
  - name: openshift
    url: http://localhost:<port>
    authorization_headers:                            # note: different field name than appserver
      x-kube-auth: "{{KUBERNETES_TOKEN}}"
  - name: <user server>
    url: <url>
    timeout: <timeout>
    authorization_headers:
      <name>: <value>                                 # same placeholder format as appserver
```

### PostgreSQL Bootstrap Script
Content is in `utils.PostgresBootStrapScriptContent` constant. Deployed as a Secret (not ConfigMap) named `lightspeed-postgres-bootstrap`.

```bash
#!/bin/bash
cat /var/lib/pgsql/data/userdata/postgresql.conf

echo "attempting to create llama-stack database and pg_trgm extension if they do not exist"
_psql () { psql --set ON_ERROR_STOP=1 "$@" ; }

# Create database "llamastack" for Llama Stack conversation storage (hardcoded name)
DB_NAME="llamastack"
echo "SELECT 'CREATE DATABASE $DB_NAME' WHERE NOT EXISTS (...)\gexec" | _psql -d $POSTGRESQL_DATABASE

# Create pg_trgm extension in default database (for OLS conversation cache)
echo "CREATE EXTENSION IF NOT EXISTS pg_trgm;" | _psql -d $POSTGRESQL_DATABASE

# Create pg_trgm extension in llamastack database
echo "CREATE EXTENSION IF NOT EXISTS pg_trgm;" | _psql -d $DB_NAME

# Create schemas for isolating different components' data
echo "CREATE SCHEMA IF NOT EXISTS lcore;" | _psql -d $POSTGRESQL_DATABASE
echo "CREATE SCHEMA IF NOT EXISTS quota;" | _psql -d $POSTGRESQL_DATABASE
echo "CREATE SCHEMA IF NOT EXISTS conversation_cache;" | _psql -d $POSTGRESQL_DATABASE
```

### PostgreSQL Config (postgresql.conf.sample)
Content is in `utils.PostgresConfigMapContent` constant. Deployed as ConfigMap.
```
huge_pages = off
ssl = on
ssl_cert_file = '/etc/certs/tls.crt'
ssl_key_file = '/etc/certs/tls.key'
ssl_ca_file = '/etc/certs/cm-olspostgresca/service-ca.crt'
```

### PostgreSQL Password Secret
Generated via `GeneratePostgresSecret()`: 12 random bytes, base64 encoded, stored in secret key `password` (`utils.PostgresSecretKeyName`).

### Nginx Config (Console UI)
Inline in `GenerateConsoleUIConfigMap()`:
- PID file: `/tmp/nginx/nginx.pid`
- Temp paths: `/tmp/nginx/{client_body,proxy,fastcgi,uwsgi,scgi}` (for read-only root filesystem)
- Serves static files from `/usr/share/nginx/html` on port 9443 with SSL
- TLS cert/key from `/var/cert/tls.crt` and `/var/cert/tls.key`

### MCP Server Config (TOML)
Inline in `utils.OpenShiftMCPServerConfigTOML` constant:
```toml
[[denied_resources]]
group = ""
version = "v1"
kind = "Secret"

[[denied_resources]]
group = "rbac.authorization.k8s.io"
version = "v1"
```

## Key Abstractions

### Provider Type Mapping (LCore)
Maps CRD provider types to Llama Stack provider types (in `providerTypeMapping` var):
| CRD Type | Llama Stack Type |
|---|---|
| `openai` | `remote::openai` |
| `azure_openai` | `remote::azure` |
| `rhoai_vllm` | `remote::vllm` |
| `rhelai_vllm` | `remote::vllm` |
| `fake_provider` | `remote::fake` |
| `llamaStackGeneric` | uses `providerType` field directly from CR |
| `watsonx`, `bam` | **not supported** by LCore (returns error) |

For `llamaStackGeneric` providers, the `config` field from the CR (`runtime.RawExtension`) is unmarshaled and passed through. `api_key` is auto-injected as `${env.<NAME>_API_KEY}` unless the config already contains an explicit `api_key` field.

### Credential Injection Patterns
- **AppServer:** Mounts secrets as files at `/etc/apikeys/<secretName>/`. OLS config references the directory path as `credentials_path`. The secret key name is not configurable for AppServer.
- **LCore:** Sets env vars `<PROVIDER_NAME>_API_KEY` from secret refs. Llama Stack config references `${env.<PROVIDER_NAME>_API_KEY}`. Provider name is converted to env var format via `utils.ProviderNameToEnvVarName()`.
- **Azure in LCore:** Additionally reads secret data keys to determine auth method. Sets `_CLIENT_ID`, `_TENANT_ID`, `_CLIENT_SECRET` env vars with `${env.NAME_SUFFIX:=}` default syntax (empty string if env var not set). If no `apitoken` key in secret, adds placeholder `_API_KEY` env var to satisfy LiteLLM validation.
- **Generic providers (LCore):** Uses `credentialKey` field from CR to determine which secret key to read. Defaults to `"apitoken"` (`utils.DefaultCredentialKey`).

### External Resource Iteration
`utils.ForEachExternalSecret(cr, callback)` and `utils.ForEachExternalConfigMap(cr, callback)` provide consistent iteration over CR-referenced external resources. Each callback receives `(name, source)` where `source` identifies the reference origin:
- `"llm-provider-<providerName>"` for LLM credential secrets
- `"mcp-<serverName>"` for MCP header secrets
- `"additional-ca"` for additional CA configmaps
- `"proxy-ca"` for proxy CA configmaps

### Config Building Pattern
Both AppServer and LCore build configs programmatically using Go maps/structs and marshal with `yaml.Marshal()`. No templates are used. AppServer uses typed structs from `utils/` package (e.g., `utils.AppSrvConfigFile`). LCore uses `map[string]interface{}` for flexibility with Llama Stack's more dynamic schema.

### LCore System Prompt
Unlike AppServer which stores the system prompt as a separate file and references it via `system_prompt_path`, LCore embeds the system prompt directly in the config YAML under `customization.system_prompt`. If `spec.ols.querySystemPrompt` is empty, a hardcoded default prompt (`DefaultQuerySystemPrompt` in `config.go`) is used.

### PostgreSQL Schema Isolation
LCore uses PostgreSQL schema namespaces to isolate data from different components within the same `postgres` database:
- `lcore` schema: main lightspeed-stack data
- `conversation_cache` schema: conversation history
- `quota` schema: token quota tracking
These schemas are created by the bootstrap script and referenced via the `namespace` field in database configs.

## Integration Points

| Config Section | Source | Notes |
|---|---|---|
| Provider credentials | CR `spec.llm.providers[].credentialsSecretRef` | AppServer: file mount. LCore: env var. |
| Default model/provider | CR `spec.ols.defaultModel`, `spec.ols.defaultProvider` | Required fields |
| Log level | CR `spec.ols.logLevel` | Enum: DEBUG, INFO, WARNING, ERROR, CRITICAL. Default: INFO |
| PostgreSQL connection | `utils/constants.go` | Host built from service name + namespace + ".svc" |
| TLS certs | Service-ca operator or user-provided secret | Path: `/etc/certs/lightspeed-tls/` |
| RAG indexes | CR `spec.ols.rag[]` | AppServer: file paths. LCore: vector_dbs config. |
| OpenShift version | Reconciler options | Used for OCP docs RAG index path |
| MCP servers | CR `spec.mcpServers[]` + `spec.ols.introspectionEnabled` | Feature gated by `MCPServer` gate |
| Tool filtering | CR `spec.ols.toolFilteringConfig` | Feature gated by `ToolFiltering` gate; requires MCP servers |
| Proxy config | CR `spec.ols.proxyConfig` | Proxy URL + optional CA cert configmap |
| Query filters | CR `spec.ols.queryFilters[]` | Regex patterns for content filtering |
| Quota config | CR `spec.ols.quotaHandlersConfig` | Rate limiting with scheduler period fixed at 300s |

## Implementation Notes

- Config YAML is built programmatically using Go maps/structs and marshaled with `yaml.Marshal()`, not templates.
- The AppServer fake provider config is hardcoded with test response values (`"This is a preconfigured fake response."`).
- Llama Stack config includes infrastructure components (safety/llama-guard, file storage/localfs, agent persistence/meta-reference) even if the CR doesn't explicitly reference them.
- The embedding model warmup in LCore server mode sends a test request to `/v1/inference/embeddings` to prevent cold-start latency. The safety/LLM warmup sends a test chat completion.
- PostgreSQL uses `POSTGRESQL_ADMIN_PASSWORD` env var for the admin password (mapped from the generated secret in the deployment spec, not shown in config files).
- The `llamastack` database name is hardcoded in both the bootstrap script and the Llama Stack storage config because llama-stack internally hardcodes this database name.
- LCore's `authorization_headers` field name differs from AppServer's `headers` field name for MCP server configs.
- Exporter config for data collector uses a separate ConfigMap (`utils.ExporterConfigCmName`) with collection interval of 300 seconds, cleanup after send, and ingress URL to `console.redhat.com`.
- The `OLSSystemPromptFileName` is stored as a separate key in the OLS config ConfigMap when `querySystemPrompt` is set (AppServer path).
