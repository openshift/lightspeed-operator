# Config Generation

## Module Map

| File | Key Functions | Responsibility |
|---|---|---|
| `internal/controller/appserver/assets.go` | `GenerateOLSConfigMap()`, `buildProviderConfigs()`, `buildOLSConfig()`, `generateMCPServerConfigs()`, `buildToolFilteringConfig()` | OLS config YAML (olsconfig.yaml) |
| `internal/controller/postgres/assets.go` | `GeneratePostgresConfigMap()`, `GeneratePostgresBootstrapSecret()`, `GeneratePostgresSecret()` | PostgreSQL config + bootstrap script + credentials |
| `internal/controller/console/assets.go` | `GenerateConsoleUIConfigMap()` | Nginx config for console plugin |
| `internal/controller/utils/mcp_server_config.go` | `GenerateOpenShiftMCPServerConfigMap()` | MCP server denied-resources config (TOML) |

## Data Flow

### OLS Config (olsconfig.yaml)
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

### PostgreSQL Bootstrap Script
Content is in `utils.PostgresBootStrapScriptContent` constant. Deployed as a Secret (not ConfigMap) named `lightspeed-postgres-bootstrap`.

```bash
#!/bin/bash
cat /var/lib/pgsql/data/userdata/postgresql.conf

_psql () { psql --set ON_ERROR_STOP=1 "$@" ; }

# Create pg_trgm extension in default database (for OLS conversation cache)
echo "CREATE EXTENSION IF NOT EXISTS pg_trgm;" | _psql -d $POSTGRESQL_DATABASE

# Create schemas for isolating different components' data
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

### Credential Injection Pattern
Provider credentials are mounted as files at `/etc/apikeys/<secretName>/`. The OLS config references the directory path as `credentials_path`. The secret key used is `apitoken` by default, overridable by `credentialKey` in the CR.

### External Resource Iteration
`utils.ForEachExternalSecret(cr, callback)` and `utils.ForEachExternalConfigMap(cr, callback)` provide consistent iteration over CR-referenced external resources. Each callback receives `(name, source)` where `source` identifies the reference origin:
- `"llm-provider-<providerName>"` for LLM credential secrets
- `"mcp-<serverName>"` for MCP header secrets
- `"additional-ca"` for additional CA configmaps
- `"proxy-ca"` for proxy CA configmaps

### Config Building Pattern
Config is built programmatically using typed Go structs from the `utils/` package (e.g., `utils.AppSrvConfigFile`) and marshaled with `yaml.Marshal()`. No templates are used.

### PostgreSQL Schema Isolation
PostgreSQL schemas isolate data from different components within the same database:
- `conversation_cache` schema: conversation history
- `quota` schema: token quota tracking
These schemas are created by the bootstrap script.

## Integration Points

| Config Section | Source | Notes |
|---|---|---|
| Provider credentials | CR `spec.llm.providers[].credentialsSecretRef` | File mount at `/etc/apikeys/<secretName>/` |
| Default model/provider | CR `spec.ols.defaultModel`, `spec.ols.defaultProvider` | Required fields |
| Log level | CR `spec.ols.logLevel` | Enum: DEBUG, INFO, WARNING, ERROR, CRITICAL. Default: INFO |
| PostgreSQL connection | `utils/constants.go` | Host built from service name + namespace + ".svc" |
| TLS certs | Service-ca operator or user-provided secret | Path: `/etc/certs/lightspeed-tls/` |
| RAG indexes | CR `spec.ols.rag[]` | File paths in config YAML |
| OpenShift version | Reconciler options | Used for OCP docs RAG index path |
| MCP servers | CR `spec.mcpServers[]` + `spec.ols.introspectionEnabled` | Feature gated by `MCPServer` gate |
| Tool filtering | CR `spec.ols.toolFilteringConfig` | Feature gated by `ToolFiltering` gate; requires MCP servers |
| Proxy config | CR `spec.ols.proxyConfig` | Proxy URL + optional CA cert configmap |
| Query filters | CR `spec.ols.queryFilters[]` | Regex patterns for content filtering |
| Quota config | CR `spec.ols.quotaHandlersConfig` | Rate limiting with scheduler period fixed at 300s |

## Implementation Notes

- Config YAML is built programmatically using Go structs and marshaled with `yaml.Marshal()`, not templates.
- The fake provider config is hardcoded with test response values (`"This is a preconfigured fake response."`).
- PostgreSQL uses `POSTGRESQL_ADMIN_PASSWORD` env var for the admin password (mapped from the generated secret in the deployment spec, not shown in config files).
- Exporter config for data collector uses a separate ConfigMap (`utils.ExporterConfigCmName`) with collection interval of 300 seconds, cleanup after send, and ingress URL to `console.redhat.com`.
- The `OLSSystemPromptFileName` is stored as a separate key in the OLS config ConfigMap when `querySystemPrompt` is set.
