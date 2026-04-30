# App Server

The App Server is the legacy backend deployment for OpenShift Lightspeed. It runs the lightspeed-service Python/FastAPI application that handles LLM queries, RAG retrieval, conversation management, and tool execution.

## Behavioral Rules

### Deployment Composition
1. The deployment contains a primary API container and up to two optional sidecar containers.
2. The primary container (lightspeed-service-api) runs the OLS service, listening on HTTPS.
3. The data collector sidecar (lightspeed-to-dataverse-exporter) is added when data collection is enabled AND the telemetry pull secret exists in the openshift-config namespace with a cloud.openshift.com auth entry.
4. The OpenShift MCP server sidecar is added when `spec.ols.introspectionEnabled` is true. It provides Kubernetes resource access via MCP protocol.
5. A PostgreSQL wait init container always runs before the main containers to ensure database readiness.
6. When `spec.ols.rag` is configured, additional init containers copy RAG data from container images into a shared volume.

### Configuration Mapping
7. The operator generates an OLS config file (olsconfig.yaml) from the CR spec. This ConfigMap is the primary interface between the operator and the service.
8. LLM provider credentials are mounted as files from their respective secrets, at a path derived from the secret name.
9. The default credential key read from each provider's secret is "apitoken", overridable by `spec.llm.providers[].credentialKey`.
10. PostgreSQL connection settings are hardcoded to point to the operator-managed PostgreSQL service within the same namespace.
11. If `spec.ols.querySystemPrompt` is set, the custom prompt is written as a second key in the config ConfigMap and referenced by file path in the config.
12. RAG reference content indexes are ordered: user-provided (BYOK) indexes first, then the OCP documentation index (unless `spec.ols.byokRAGOnly` is true).
13. The OCP documentation RAG index path is derived from the detected OpenShift cluster version.

### MCP Server Integration
14. When `spec.ols.introspectionEnabled` is true, an "openshift" MCP server entry is added to the config pointing to localhost on the sidecar port.
15. When the MCPServer feature gate is enabled, user-defined servers from `spec.mcpServers` are added to the config.
16. MCP header values of type "secret" are mounted as files from the referenced secret. Types "kubernetes" and "client" use placeholder strings that the service resolves at runtime.

### Service and Networking
17. The service exposes HTTPS on the configured port.
18. The network policy allows ingress from: Prometheus (openshift-monitoring), OpenShift Console (openshift-console), and ingress controllers.
19. Egress is unrestricted (empty egress rules).

### RBAC
20. The service account is granted SubjectAccessReview and TokenReview permissions for user authorization.
21. The service account can read the cluster version and the telemetry pull secret.

### Change Detection
22. Deployment updates are triggered when: the deployment spec changes, the config ConfigMap resource version changes, the MCP config ConfigMap resource version changes, or the proxy CA certificate hash changes.
23. When any of these change, the operator forces a rolling restart by updating a pod template annotation with the current timestamp.

### Observability
24. The operator creates a ServiceMonitor for Prometheus scraping of the /metrics endpoint.
25. The operator creates a PrometheusRule with recording rules aggregating query call counts by status code class (2xx, 4xx, 5xx) and provider/model configuration.

## Configuration Surface

| Field path | Description |
|---|---|
| `spec.ols.deployment.api.replicas` | Number of API server replicas |
| `spec.ols.deployment.api.resources` | API container resource requirements |
| `spec.ols.deployment.api.tolerations` | Pod tolerations |
| `spec.ols.deployment.api.nodeSelector` | Node selector constraints |
| `spec.ols.deployment.api.affinity` | Pod affinity rules |
| `spec.ols.deployment.api.topologySpreadConstraints` | Topology spread constraints |
| `spec.ols.deployment.dataCollector.resources` | Data collector container resources |
| `spec.ols.deployment.mcpServer.resources` | MCP server container resources |
| `spec.ols.defaultModel` | Default LLM model name |
| `spec.ols.defaultProvider` | Default LLM provider name |
| `spec.ols.logLevel` | Logging level for all service components |
| `spec.ols.maxIterations` | Maximum agent execution iterations |
| `spec.ols.querySystemPrompt` | Custom system prompt for LLM queries |
| `spec.ols.byokRAGOnly` | Skip OCP documentation RAG index |
| `spec.ols.introspectionEnabled` | Enable OpenShift MCP server sidecar |
| `spec.ols.userDataCollection.feedbackDisabled` | Disable feedback collection |
| `spec.ols.userDataCollection.transcriptsDisabled` | Disable transcript collection |
| `spec.ols.queryFilters` | Query text pattern replacements |
| `spec.ols.rag` | RAG database image references |
| `spec.ols.imagePullSecrets` | Pull secrets for RAG images |
| `spec.ols.quotaHandlersConfig` | Token quota limiter configuration |
| `spec.ols.toolFilteringConfig` | Tool filtering parameters (requires ToolFiltering feature gate) |
| `spec.ols.toolsApprovalConfig` | Tool execution approval settings |
| `spec.mcpServers` | External MCP server definitions (requires MCPServer feature gate) |

## Constraints

1. Data collection requires both: at least one of feedback/transcripts enabled, AND the telemetry pull secret present with cloud.openshift.com credentials.
2. Tool filtering requires MCP servers to be configured (either introspection or user-defined).
3. The service always connects to PostgreSQL via the internal cluster service DNS.
4. RAG init containers run in index order, copying data to subdirectories of the shared RAG volume.
