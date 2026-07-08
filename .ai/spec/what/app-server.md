# App Server

The App Server is the backend deployment for OpenShift Lightspeed. It runs the lightspeed-service Python/FastAPI application that handles LLM queries, RAG retrieval, conversation management, and tool execution.

## Behavioral Rules

### Deployment Composition
1. The deployment contains a primary API container and up to three sidecar containers.
2. The primary container (lightspeed-service-api) runs the OLS service, listening on HTTPS.
3. The data collector sidecar (lightspeed-to-dataverse-exporter) is added when data collection is enabled AND the telemetry pull secret exists in the openshift-config namespace with a cloud.openshift.com auth entry.
4. The OpenShift MCP server sidecar is added when `spec.ols.introspectionEnabled` is true. It provides Kubernetes resource access via MCP protocol.
5. The RHOKP sidecar is always added to the deployment. It serves OKP (Offline Knowledge Portal) content via Solr HTTP on localhost:8080, providing Red Hat product documentation for tool-based retrieval. It requires ~75 GiB ephemeral storage. The RHOKP sidecar is NOT deployed when `spec.ols.byokRAGOnly` is true.
6. A PostgreSQL wait init container always runs before the main containers to ensure database readiness.
7. When `spec.ols.rag` is configured, additional init containers copy BYOK RAG data from container images into a shared volume.

### Configuration Mapping
8. The operator generates an OLS config file (olsconfig.yaml) from the CR spec. This ConfigMap is the primary interface between the operator and the service.
9. LLM provider credentials are mounted as files from their respective secrets, at a path derived from the secret name.
10. The default credential key read from each provider's secret is "apitoken", overridable by `spec.llm.providers[].credentialKey`.
11. PostgreSQL connection settings are hardcoded to point to the operator-managed PostgreSQL service within the same namespace.
12. If `spec.ols.querySystemPrompt` is set, the custom prompt is written as a second key in the config ConfigMap and referenced by file path in the config.
13. BYOK reference content indexes from `spec.ols.rag` are configured when present. OCP documentation is served by OKP via the RHOKP sidecar, not via FAISS indexes.
14. The operator always generates a `solr_hybrid` config section in `olsconfig.yaml` pointing to `http://localhost:8080` with default hybrid retrieval tuning parameters, unless `byokRAGOnly` is true.
15a. Unless `byokRAGOnly` is true, the app-server container receives `OCP_CLUSTER_VERSION` (`<major>.<minor>` from the operator's cluster-version lookup) for Solr `chunk_filter_query` resolution in lightspeed-service.

### ROSA-Aware OKP Retrieval [PLANNED: OLS-1894]
15b. Unless `byokRAGOnly` is true, the operator detects whether the cluster is ROSA and, if so, which variant (Classic vs HCP). Detection uses two standard OpenShift API resources, following the same pattern as OCP version detection — determined once and passed to the service as an environment variable:
  - **ROSA detection:** Read `console.operator.openshift.io/v1` Console `cluster` resource, field `.spec.customization.brand`. Value `ROSA` indicates a ROSA cluster (reliable on OCP 4.16+).
  - **Variant detection:** Read `infrastructure.config.openshift.io/v1` Infrastructure `cluster` resource, field `.status.controlPlaneTopology`. `External` = HCP, `HighlyAvailable` = Classic.
  - When ROSA is detected, the operator sets `OLS_ROSA_PRODUCT` on the app-server container: `red_hat_openshift_service_on_aws` for HCP, `red_hat_openshift_service_on_aws_classic_architecture` for Classic.
  - On non-ROSA clusters the env var is absent and the service uses OCP-only retrieval.
  - RBAC: requires `get` on `consoles` in the `operator.openshift.io` API group (Infrastructure is already covered by existing cluster-version permissions).

### MCP Server Integration
15. When `spec.ols.introspectionEnabled` is true, an "openshift" MCP server entry is added to the config pointing to localhost on the sidecar port.
16. When the MCPServer feature gate is enabled, user-defined servers from `spec.mcpServers` are added to the config.
17. MCP header values of type "secret" are mounted as files from the referenced secret. Types "kubernetes" and "client" use placeholder strings that the service resolves at runtime.

### Service and Networking
18. The service exposes HTTPS on the configured port.
19. The network policy allows ingress from: Prometheus (openshift-monitoring), OpenShift Console (openshift-console), and ingress controllers.
20. Egress is unrestricted (empty egress rules).

### RBAC
21. The service account is granted SubjectAccessReview and TokenReview permissions for user authorization.
22. The service account can read the cluster version and the telemetry pull secret.

### Change Detection
23. Deployment updates are triggered when: the deployment spec changes, the config ConfigMap resource version changes, the MCP config ConfigMap resource version changes, or the proxy CA certificate hash changes.
24. When any of these change, the operator forces a rolling restart by updating a pod template annotation with the current timestamp.

### Health Probes [CHANGED: OLS-3221]
24. The app server deployment's liveness probe must point to the `/liveness` endpoint with `failureThreshold: 3` and `periodSeconds: 30`, giving the pod 90 seconds to self-heal via the background health-check loop before Kubernetes restarts it. These values are not currently user-configurable.
25. The app server deployment's readiness probe must point to the `/readiness` endpoint. The readiness probe checks RAG index, LLM reachability, and cache health status (read from the background health-check loop). No changes to existing readiness probe configuration.

### Observability
26. The operator creates a ServiceMonitor for Prometheus scraping of the /metrics endpoint.
27. The operator creates a PrometheusRule with recording rules aggregating query call counts by status code class (2xx, 4xx, 5xx) and provider/model configuration.

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
| `spec.ols.byokRAGOnly` | Disable OKP (RHOKP sidecar not deployed, solr_hybrid config not generated). Only BYOK FAISS indexes are used. |
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
28. All operator-managed container defaults follow the [OpenShift resource conventions](https://github.com/openshift/enhancements/blob/master/CONVENTIONS.md#resources-and-limits): defaults declare CPU and memory requests only, and do not set resource limits. This applies to the primary API container and all sidecars (data collector, MCP server, RHOKP).
29. Users may still set limits via the CRD (`spec.ols.deployment.<component>.resources`) if their environment requires it. The CRD uses standard `corev1.ResourceRequirements` which accepts both requests and limits.
30. The RHOKP sidecar's ~75 GiB ephemeral storage requirement is unchanged by this convention — it applies only to CPU and memory.

## Planned Changes

- [PLANNED: OLS-3221] Liveness probe now checks PostgreSQL health via the service's background health-check loop status. Probe configuration (failureThreshold, periodSeconds) added to deployment generation. See Rules 24–25.
