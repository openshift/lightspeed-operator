# App Server

The App Server is the backend deployment for OpenShift Lightspeed. It runs the lightspeed-service Python/FastAPI application that handles LLM queries, RAG retrieval, conversation management, and tool execution.

## Behavioral Rules

### Deployment Composition
1. The deployment contains a primary API container and up to three sidecar containers.
2. The primary container (lightspeed-service-api) runs the OLS service, listening on HTTPS.
3. The data collector sidecar (lightspeed-to-dataverse-exporter) is added when data collection is enabled AND the telemetry pull secret exists in the openshift-config namespace with a cloud.openshift.com auth entry.
4. The OpenShift MCP server sidecar is added when `spec.ols.introspectionEnabled` is true. It provides Kubernetes resource access via MCP protocol.
5. OKP (Offline Knowledge Portal) / Solr hybrid RAG is operator-managed (no CR toggle besides `byokRAGOnly`). When OKP is enabled, the RHOKP sidecar serves Solr HTTP on localhost:8080 for the `search_openshift_documentation` tool path. It requires ~75 GiB ephemeral storage. OKP is on by default; set `spec.ols.byokRAGOnly` to true to skip the RHOKP sidecar, `solr_hybrid` config, and built-in OCP documentation retrieval.
6. A PostgreSQL wait init container always runs before the main containers to ensure database readiness.
7. When `spec.ols.rag` is configured, additional init containers copy BYOK RAG data from container images into a shared volume.

### Configuration Mapping
8. The operator generates an OLS config file (olsconfig.yaml) from the CR spec. This ConfigMap is the primary interface between the operator and the service.
9. LLM provider credentials are mounted as files from their respective secrets, at a path derived from the secret name.
10. The default credential key read from each provider's secret is "apitoken", overridable by `spec.llm.providers[].credentialKey`.
11. PostgreSQL connection settings are hardcoded to point to the operator-managed PostgreSQL service within the same namespace.
12. If `spec.ols.querySystemPrompt` is set, the custom prompt is written as a second key in the config ConfigMap and referenced by file path in the config.
13. BYOK reference content indexes from `spec.ols.rag` are configured when present. Unless `byokRAGOnly` is true, the operator also emits a versioned OCP FAISS index entry under `reference_content.indexes` (kept for service readiness until FAISS is removed in a follow-up). When OKP is enabled, all reference indexes get `byok_index: true`.
14. Unless `byokRAGOnly` is true, the operator generates a `solr_hybrid` config section in `olsconfig.yaml` pointing to `http://localhost:8080` with default hybrid retrieval tuning parameters. OCP documentation queries use Solr hybrid via the search tool; FAISS remains a transitional fallback in config.
15. Unless `byokRAGOnly` is true, the app-server container receives `OCP_CLUSTER_VERSION` (`<major>.<minor>` from the operator's cluster-version lookup) for Solr `chunk_filter_query` resolution in lightspeed-service.

### MCP Server Integration
16. When `spec.ols.introspectionEnabled` is true, an "openshift" MCP server entry is added to the config pointing to localhost on the sidecar port.
17. When the MCPServer feature gate is enabled, user-defined servers from `spec.mcpServers` are added to the config.
18. MCP header values of type "secret" are mounted as files from the referenced secret. Types "kubernetes" and "client" use placeholder strings that the service resolves at runtime.

### Service and Networking
19. The service exposes HTTPS on the configured port.
20. The network policy allows ingress from: Prometheus (openshift-monitoring), OpenShift Console (openshift-console), and ingress controllers.
21. Egress is unrestricted (empty egress rules).

### RBAC
22. The service account is granted SubjectAccessReview and TokenReview permissions for user authorization.
23. The service account can read the cluster version and the telemetry pull secret.

### Change Detection
24. Deployment updates are triggered when: the deployment spec changes, the config ConfigMap resource version changes, the MCP config ConfigMap resource version changes, or the proxy CA certificate hash changes.
25. When any of these change, the operator forces a rolling restart by updating a pod template annotation with the current timestamp.

### Health Probes [CHANGED: OLS-3221]
26. The app server deployment's liveness probe must point to the `/liveness` endpoint with `failureThreshold: 3` and `periodSeconds: 30`, giving the pod 90 seconds to self-heal via the background health-check loop before Kubernetes restarts it. These values are not currently user-configurable.
27. The app server deployment's readiness probe must point to the `/readiness` endpoint. The readiness probe checks RAG index, LLM reachability, and cache health status (read from the background health-check loop). No changes to existing readiness probe configuration.

### Observability
28. The operator creates a ServiceMonitor for Prometheus scraping of the /metrics endpoint.
29. The operator creates a PrometheusRule with recording rules aggregating query call counts by status code class (2xx, 4xx, 5xx) and provider/model configuration.

## Configuration Surface

| Field path | Description |
|---|---|
| `spec.ols.deployment.api.replicas` | Number of API server replicas |
| `spec.ols.deployment.api.resources` | API container resource requirements |
| `spec.ols.deployment.api.tolerations` | Pod tolerations |
| `spec.ols.deployment.api.nodeSelector` | Node selector constraints |
| `spec.ols.deployment.dataCollector.resources` | Data collector container resources |
| `spec.ols.deployment.mcpServer.resources` | MCP server container resources |
| `spec.ols.defaultModel` | Default LLM model name |
| `spec.ols.defaultProvider` | Default LLM provider name |
| `spec.ols.logLevel` | Logging level for all service components |
| `spec.ols.maxIterations` | Maximum agent execution iterations |
| `spec.ols.querySystemPrompt` | Custom system prompt for LLM queries |
| `spec.ols.byokRAGOnly` | Disable OKP: no RHOKP sidecar, no `solr_hybrid` section, no built-in OCP FAISS index, no `OCP_CLUSTER_VERSION` env. Only BYOK FAISS indexes from `spec.ols.rag` are used. |
| `spec.ols.introspectionEnabled` | Enable OpenShift MCP server sidecar |
| `spec.ols.userDataCollection.feedbackDisabled` | Disable feedback collection |
| `spec.ols.userDataCollection.transcriptsDisabled` | Disable transcript collection |
| `spec.ols.queryFilters` | Query text pattern replacements |
| `spec.ols.rag` | BYOK RAG database image references |
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
5. The RHOKP sidecar requires approximately 75 GiB of ephemeral storage for Solr data. This must be documented in product infrastructure requirements.

### Resource Conventions [OLS-3397]
30. All operator-managed container defaults follow the [OpenShift resource conventions](https://github.com/openshift/enhancements/blob/master/CONVENTIONS.md#resources-and-limits): defaults declare CPU and memory requests only, and do not set resource limits. This applies to the primary API container and all sidecars (data collector, MCP server, RHOKP).
31. Users may still set limits via the CRD (`spec.ols.deployment.<component>.resources`) if their environment requires it. The CRD uses standard `corev1.ResourceRequirements` which accepts both requests and limits.
32. The RHOKP sidecar's ~75 GiB ephemeral storage requirement is unchanged by this convention — it applies only to CPU and memory.

### RHOKP Image
33. The RHOKP sidecar image is set via the operator `--rhokp-image` startup flag (default in `utils.RHOOKPImageDefault`). It is not listed in `related_images.json` until productized in the OLM bundle.

## Planned Changes

- [PLANNED: OLS-3221] Liveness probe now checks PostgreSQL health via the service's background health-check loop status. Probe configuration (failureThreshold, periodSeconds) added to deployment generation. See Rules 24–25.
