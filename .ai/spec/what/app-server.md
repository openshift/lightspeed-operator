# App Server

The App Server is the backend deployment for OpenShift Lightspeed. It runs the lightspeed-service Python/FastAPI application that handles LLM queries, RAG retrieval, conversation management, and tool execution.

## Behavioral Rules

### Deployment Composition
1. The deployment contains a primary API container and an optional sidecar container (data collector).
2. The primary container (lightspeed-service-api) runs the OLS service, listening on HTTPS.
3. The data collector sidecar (lightspeed-to-dataverse-exporter) is added when data collection is enabled AND the telemetry pull secret exists in the openshift-config namespace with a cloud.openshift.com auth entry.
4. The OpenShift MCP server runs as a standalone HTTPS Deployment/Service (`ocpmcp` package) when `spec.ols.introspectionEnabled` is true. The app-server connects via `https://openshift-mcp-server.<ns>.svc:8443/mcp` and trusts client CA Secret `lightspeed-agentic-mcp-ca` (cluster service-ca PEM). See `ocpmcp.md`.
5. OKP (Offline Knowledge Portal) / Solr hybrid RAG is operator-managed (no CR toggle besides `byokRAGOnly`). When OKP is enabled, the RHOKP standalone Deployment serves Solr via HTTPS at `https://lightspeed-rhokp.<ns>.svc:8443`. The app-server connects as a client, trusting client CA Secret `lightspeed-agentic-rhokp-ca` (cluster service-ca PEM) via `extra_ca`. OKP is on by default; set `spec.ols.byokRAGOnly` to true to skip the RHOKP standalone operand, `solr_hybrid` config, and OCP documentation retrieval via Solr. See `rhokp.md`.
6. A PostgreSQL wait init container always runs before the main containers to ensure database readiness.
6a. [PLANNED: OLS-3799] When `byokRAGOnly` is false, a RHOKP wait init container runs after the PostgreSQL wait init container and before the main containers. It polls the RHOKP Solr ping endpoint until it responds, with a timeout matching RHOKP's startup probe budget (~360s). This follows the existing PostgreSQL wait pattern and ensures the app-server main process does not start until RHOKP is reachable. (Not yet implemented; app-server init containers are currently PostgreSQL wait + RAG only.)
7. When `spec.ols.rag` is configured, additional init containers copy BYOK RAG data from container images into a shared volume.

### Configuration Mapping
8. The operator generates an OLS config file (olsconfig.yaml) from the CR spec. This ConfigMap is the primary interface between the operator and the service.
9. LLM provider credentials are mounted as files from their respective secrets, at a path derived from the secret name.
10. The default credential key read from each provider's secret is "apitoken", overridable by `spec.llm.providers[].credentialKey`.
11. PostgreSQL connection settings are hardcoded to point to the operator-managed PostgreSQL service within the same namespace.
12. If `spec.ols.querySystemPrompt` is set, the custom prompt is written as a second key in the config ConfigMap and referenced by file path in the config.
13. BYOK reference content indexes from `spec.ols.rag` are written to `reference_content.indexes` when present. OCP product documentation is served exclusively via `solr_hybrid` (OKP); the operator does not emit a built-in OCP FAISS index.
14. Unless `byokRAGOnly` is true, the operator generates a `solr_hybrid` config section in `olsconfig.yaml` pointing to `https://lightspeed-rhokp.<ns>.svc:8443` with default hybrid retrieval tuning parameters.
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
16. When `spec.ols.introspectionEnabled` is true, an "openshift" MCP server entry is added to the config pointing at the standalone Service URL (`https://openshift-mcp-server.<ns>.svc:8443/mcp`).
17. When the MCPServer feature gate is enabled, user-defined servers from `spec.mcpServers` are added to the config.
18. MCP header values of type "secret" are mounted as files from the referenced secret. Types "kubernetes" and "client" use placeholder strings that the service resolves at runtime.

### Service and Networking
19. The service exposes HTTPS on the configured port.
20. The network policy allows ingress from: Prometheus (openshift-monitoring), OpenShift Console (openshift-console), and ingress controllers.
21. Egress is unrestricted (empty egress rules).

### RBAC
22. The app-server service account (`lightspeed-app-server`) is granted SubjectAccessReview and TokenReview permissions for user authorization.
23. The app-server service account can read the cluster version and the telemetry pull secret.

### Change Detection
24. Deployment updates are triggered when: the deployment spec changes, the config ConfigMap resource version changes, or the proxy CA certificate hash changes.
25. Client CA Secrets (OTEL, MCP, RHOKP) are refreshed via the table-driven `RefreshClientCASecrets` in `RestartAppServer`. The watcher detects TLS secret rotation and invokes `RestartAppServer`, which re-reads the service-ca ConfigMap and updates each enabled client CA Secret. No hash annotation is stored on the Deployment.
26. When any change is detected, the operator forces a rolling restart by updating a pod template annotation with the current timestamp.

### Health Probes
26. The app server deployment's liveness probe points to the `/liveness` endpoint with `failureThreshold: 3` and `periodSeconds: 30`, giving the pod 90 seconds to self-heal via the background health-check loop before Kubernetes restarts it. These values are not user-configurable.
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
| `spec.ols.deployment.mcpServer` | Standalone MCP Deployment settings (`Config`: replicas, resources, tolerations, nodeSelector) |
| `spec.ols.deployment.rhokp` | Standalone RHOKP Deployment settings (`Config`: replicas, resources, tolerations, nodeSelector) |
| `spec.ols.defaultModel` | Default LLM model name |
| `spec.ols.defaultProvider` | Default LLM provider name |
| `spec.ols.logLevel` | Logging level for all service components |
| `spec.ols.maxIterations` | Maximum agent execution iterations |
| `spec.ols.querySystemPrompt` | Custom system prompt for LLM queries |
| `spec.ols.byokRAGOnly` | Disable OKP: no RHOKP standalone operand, no `solr_hybrid` section, no `OCP_CLUSTER_VERSION` env. Only BYOK FAISS indexes from `spec.ols.rag` are used. |
| `spec.ols.introspectionEnabled` | Enable standalone OpenShift MCP server operand |
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
5. RHOKP runs as a standalone Deployment (`lightspeed-rhokp`) with its own 75 GiB EmptyDir. The app-server pod no longer requires ephemeral storage for OKP. A wait-for-rhokp init container ([PLANNED: OLS-3799], Rule 6a) will ensure the app-server does not start until RHOKP is reachable. See `rhokp.md`.

### Resource Conventions [OLS-3397]
30. All operator-managed container defaults follow the [OpenShift resource conventions](https://github.com/openshift/enhancements/blob/master/CONVENTIONS.md#resources-and-limits): defaults declare CPU and memory requests only, and do not set resource limits. This applies to the primary API container, sidecars (data collector), the standalone MCP Deployment, and the standalone RHOKP Deployment.
31. Users may still set limits via the CRD (`spec.ols.deployment.<component>.resources`, including `spec.ols.deployment.rhokp.resources`) if their environment requires it. The CRD uses standard `corev1.ResourceRequirements` which accepts both requests and limits.
32. The RHOKP standalone Deployment's ~75 GiB EmptyDir sizeLimit is unchanged by this convention — it applies only to CPU and memory.

### RHOKP Image
33. The RHOKP standalone Deployment image is set via the operator `--rhokp-image` startup flag. Default comes from `related_images.json` entry `rhokp` (`utils.RHOOKPImageDefault` / `imageDefaultOr`). The OLM bundle lists it in CSV `spec.relatedImages` and passes the image via `--rhokp-image` on the manager deployment. See `rhokp.md`.

### Agentic Sandbox Configuration Handoff

34. Classic→agentic sandbox connectivity is published via the handoff ConfigMap `lightspeed-agentic-configuration` (owned by the `agenticintegration` package, reconciled last in Phase 2) plus appserver-owned client CA Secrets (`lightspeed-agentic-otel-ca`, `lightspeed-agentic-mcp-ca`, `lightspeed-agentic-rhokp-ca`). The app-server does **not** create a `lightspeed-sandbox-config` ConfigMap — the earlier OLS-3572 design under that name was superseded by OLS-3683 / OLS-3684. The handoff ConfigMap carries `sandbox-mode`, a thin `sandbox-pod-spec`, and OTEL/MCP/RHOKP endpoint + CA-Secret-name keys. See `agentic-sandbox-profile.md` for the authoritative contract.

## Planned Changes

- [PLANNED: OLS-3799] Wait-for-rhokp init container added when `!byokRAGOnly` to block app-server startup until RHOKP Solr is reachable. See Rule 6a.
- Classic→agentic sandbox handoff: appserver owns client CA Secrets (`lightspeed-agentic-otel-ca` / `lightspeed-agentic-mcp-ca` / `lightspeed-agentic-rhokp-ca`) and mounts them; `agenticintegration` owns the handoff ConfigMap — see `agentic-sandbox-profile.md` (OLS-3683 / OLS-3684). Optional agentic auto-injection remains deferred ([OLS-3594](https://redhat.atlassian.net/browse/OLS-3594)).
