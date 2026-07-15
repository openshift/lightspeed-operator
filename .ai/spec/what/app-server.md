# App Server

The App Server is the backend deployment for OpenShift Lightspeed. It runs the lightspeed-service Python/FastAPI application that handles LLM queries, RAG retrieval, conversation management, and tool execution.

## Behavioral Rules

### Deployment Composition
1. The deployment contains a primary API container and up to two sidecar containers.
2. The primary container (lightspeed-service-api) runs the OLS service, listening on HTTPS.
3. The data collector sidecar (lightspeed-to-dataverse-exporter) is added when data collection is enabled AND the telemetry pull secret exists in the openshift-config namespace with a cloud.openshift.com auth entry.
4. When `spec.ols.introspectionEnabled` is true, the operator deploys the OpenShift MCP server as a **standalone Deployment** (not a sidecar). See "MCP Server Standalone Deployment" section below.
5. OKP (Offline Knowledge Portal) / Solr hybrid RAG is operator-managed (no CR toggle besides `byokRAGOnly`). When OKP is enabled, the RHOKP sidecar serves Solr HTTP on localhost:9080 for the `search_openshift_documentation` tool path. It requires ~75 GiB ephemeral storage. OKP is on by default; set `spec.ols.byokRAGOnly` to true to skip the RHOKP sidecar, `solr_hybrid` config, and OCP documentation retrieval via Solr.
6. A PostgreSQL wait init container always runs before the main containers to ensure database readiness.
7. When `spec.ols.rag` is configured, additional init containers copy BYOK RAG data from container images into a shared volume.

### Configuration Mapping
8. The operator generates an OLS config file (olsconfig.yaml) from the CR spec. This ConfigMap is the primary interface between the operator and the service.
9. LLM provider credentials are mounted as files from their respective secrets, at a path derived from the secret name.
10. The default credential key read from each provider's secret is "apitoken", overridable by `spec.llm.providers[].credentialKey`.
11. PostgreSQL connection settings are hardcoded to point to the operator-managed PostgreSQL service within the same namespace.
12. If `spec.ols.querySystemPrompt` is set, the custom prompt is written as a second key in the config ConfigMap and referenced by file path in the config.
13. BYOK reference content indexes from `spec.ols.rag` are written to `reference_content.indexes` when present. OCP product documentation is served exclusively via `solr_hybrid` (OKP); the operator does not emit a built-in OCP FAISS index.
14. Unless `byokRAGOnly` is true, the operator generates a `solr_hybrid` config section in `olsconfig.yaml` pointing to `http://localhost:9080` with default hybrid retrieval tuning parameters.
15a. Unless `byokRAGOnly` is true, the app-server container receives `OCP_CLUSTER_VERSION` (`<major>.<minor>` from the operator's cluster-version lookup) for Solr `chunk_filter_query` resolution in lightspeed-service.

### ROSA-Aware OKP Retrieval
15b. Unless `byokRAGOnly` is true, the operator detects whether the cluster is ROSA and, if so, which OKP product to scope. Detection uses two standard OpenShift API resources — determined once at operator startup and passed to the app-server as an environment variable:
  - **ROSA detection:** Read `console.operator.openshift.io/v1` Console `cluster` resource, field `.spec.customization.brand`. Value `ROSA` indicates a ROSA cluster (reliable on OCP 4.16+).
  - **Variant detection:** Read `infrastructure.config.openshift.io/v1` Infrastructure `cluster` resource, field `.status.controlPlaneTopology`. `External` = HCP; any other topology on ROSA = Classic.
  - When ROSA is detected, the operator sets `OLS_ROSA_PRODUCT` on the app-server container: `red_hat_openshift_service_on_aws` for HCP, `red_hat_openshift_service_on_aws_classic_architecture` for Classic.
  - On non-ROSA clusters the env var is absent and the service uses OCP-only retrieval.
  - If detection fails at startup (API/RBAC error), the operator logs a warning and omits the env var; reconciliation continues.
  - RBAC: operator requires `get` on `consoles` (`operator.openshift.io`) and `infrastructures` (`config.openshift.io`).

### MCP Server Integration
16. When `spec.ols.introspectionEnabled` is true, an "openshift" MCP server entry is added to the config pointing to the standalone MCP server Service URL: `https://openshift-mcp-server.<namespace>.svc:8443/mcp`.
17. When the MCPServer feature gate is enabled, user-defined servers from `spec.mcpServers` are added to the config.
18. MCP header values of type "secret" are mounted as files from the referenced secret. Types "kubernetes" and "client" use placeholder strings that the service resolves at runtime.
18a. When `spec.ols.introspectionEnabled` is true, the app-server container MUST mount the `openshift-mcp-server` service-ca CA bundle (from the `openshift-mcp-server-ca` ConfigMap, key `service-ca.crt`) so lightspeed-service can verify the standalone MCP server's TLS certificate.

### MCP Server Standalone Deployment [CHANGED: OLS-3526]

When `spec.ols.introspectionEnabled` is true, the operator creates a standalone Deployment and supporting resources for the OpenShift MCP server. When `introspectionEnabled` is false, all MCP server resources MUST be deleted if they exist.

#### Deployment
34. The Deployment is named `openshift-mcp-server` in the operator namespace with replicas, tolerations, and nodeSelector from `spec.ols.deployment.mcpServer`.
35. The container runs the `openshift-mcp-server` image (set via the operator `--mcp-server-image` startup flag, analogous to `--rhokp-image`).
36. The container listens on port 8443 with TLS enabled via `--tls-cert` and `--tls-key` flags pointing to the service-ca-generated certificate files.
37. The container mounts a TOML configuration file from a ConfigMap (`openshift-mcp-server-config`). The TOML includes TLS cert/key paths, denied-resources list, and enabled toolsets.
38. Security context: `runAsNonRoot: true`, `allowPrivilegeEscalation: false`, `seccompProfile: RuntimeDefault`, `capabilities: drop: [ALL]`.
39. Liveness probe: HTTP GET `/healthz` on port 8443 (scheme HTTPS), `periodSeconds: 30`, `failureThreshold: 3`.
40. Readiness probe: HTTP GET `/healthz` on port 8443 (scheme HTTPS), `periodSeconds: 10`, `failureThreshold: 3`.

#### Service
41. A `Service` named `openshift-mcp-server` with annotation `service.beta.openshift.io/serving-cert-secret-name: openshift-mcp-server-tls` is created. This triggers the OpenShift service-ca operator to generate a TLS certificate Secret (`openshift-mcp-server-tls`) trusted by the cluster.
42. The Service targets port 8443 on the MCP server pods.

#### CA Bundle
43. A `ConfigMap` named `openshift-mcp-server-ca` with annotation `service.beta.openshift.io/inject-cabundle: "true"` is created. The service-ca operator injects the CA certificate (key `service-ca.crt`) used to verify the MCP server's TLS certificate.

#### ServiceAccount
44. A dedicated `ServiceAccount` named `openshift-mcp-server` is created. It has no RBAC bindings — the MCP server uses caller token passthrough for all Kubernetes API and metrics calls.

#### NetworkPolicy
45. A `NetworkPolicy` for the MCP server allows ingress on port 8443 from: pods in the operator namespace (app-server, agentic sandbox).

#### Configuration (TOML)
46. The operator generates a TOML ConfigMap (`openshift-mcp-server-config`) with:
  - `tls_cert` and `tls_key` pointing to the mounted service-ca Secret paths.
  - `denied_resources` from the existing sidecar configuration.
  - `toolsets` from the existing sidecar configuration.

#### Change Detection
47. The app-server deployment annotations MUST include the MCP server CA ConfigMap resource version so that changes to the CA trigger a rolling restart of the app-server.
48. The MCP server deployment annotations MUST include the MCP config ConfigMap resource version and the TLS Secret resource version so that changes trigger a rolling restart.

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
| `spec.ols.deployment.mcpServer.replicas` | MCP server deployment replicas |
| `spec.ols.deployment.mcpServer.resources` | MCP server container resources |
| `spec.ols.deployment.mcpServer.tolerations` | MCP server pod tolerations |
| `spec.ols.deployment.mcpServer.nodeSelector` | MCP server node selector constraints |
| `spec.ols.deployment.rhokp.resources` | RHOKP sidecar container resources (CPU, memory, ephemeral storage) |
| `spec.ols.defaultModel` | Default LLM model name |
| `spec.ols.defaultProvider` | Default LLM provider name |
| `spec.ols.logLevel` | Logging level for all service components |
| `spec.ols.maxIterations` | Maximum agent execution iterations |
| `spec.ols.querySystemPrompt` | Custom system prompt for LLM queries |
| `spec.ols.byokRAGOnly` | Disable OKP: no RHOKP sidecar, no `solr_hybrid` section, no `OCP_CLUSTER_VERSION` env. Only BYOK FAISS indexes from `spec.ols.rag` are used. |
| `spec.ols.introspectionEnabled` | Enable standalone OpenShift MCP server deployment |
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
30. All operator-managed container defaults follow the [OpenShift resource conventions](https://github.com/openshift/enhancements/blob/master/CONVENTIONS.md#resources-and-limits): defaults declare CPU and memory requests only, and do not set resource limits. This applies to the primary API container, sidecars (data collector, RHOKP), and the standalone MCP server deployment.
31. Users may still set limits via the CRD (`spec.ols.deployment.<component>.resources`, including `spec.ols.deployment.rhokp.resources`) if their environment requires it. The CRD uses standard `corev1.ResourceRequirements` which accepts both requests and limits.
32. The RHOKP sidecar's ~75 GiB ephemeral storage requirement is unchanged by this convention — it applies only to CPU and memory.

### RHOKP Image
33. The RHOKP sidecar image is set via the operator `--rhokp-image` startup flag. Default comes from `related_images.json` entry `rhokp` (`utils.RHOOKPImageDefault` / `imageDefaultOr`). The OLM bundle lists it in CSV `spec.relatedImages` and passes the image via `--rhokp-image` on the manager deployment.

## Planned Changes

- [PLANNED: OLS-3221] Liveness probe now checks PostgreSQL health via the service's background health-check loop status. Probe configuration (failureThreshold, periodSeconds) added to deployment generation. See Rules 24–25.
- [PLANNED: OLS-3526] MCP server moved from app-server sidecar to standalone Deployment with TLS via service-ca. CRD `MCPServerContainer` type changes from `ContainerConfig` to `Config` to support replicas, tolerations, and nodeSelector. See "MCP Server Standalone Deployment" section.
