# System Overview

The OpenShift Lightspeed Operator is a Kubernetes operator that manages the lifecycle of the OpenShift Lightspeed (OLS) AI assistant stack on an OpenShift cluster. It reconciles a single cluster-scoped OLSConfig custom resource into a set of Kubernetes resources that form the complete Lightspeed deployment.

## Behavioral Rules

### Operator Role

1. The operator manages exactly one OLSConfig CR per cluster, named "cluster". CRs with any other name must be ignored.
2. The operator deploys and manages four components: an application backend (AppServer or LCore), a PostgreSQL database, a Console UI plugin, and operator-level monitoring/networking resources.
3. The operator is fully event-driven. It does not use periodic/timer-based reconciliation. All changes are detected via Kubernetes watches on owned resources and annotated external resources.
4. The operator selects between two mutually exclusive backend implementations at startup via the `--use-lcore` flag: AppServer (legacy, direct LLM proxy) or LCore (new, agent-based with Llama Stack). Both implement the same Lightspeed API surface.

### Component Inventory

5. PostgreSQL: single-replica database providing conversation cache, quota state, and (in LCore mode) Llama Stack persistence. Always deployed.
6. Console UI Plugin: OpenShift console extension that provides the Lightspeed chat interface. Integrates via ConsolePlugin CR and proxies requests to the backend.
7. AppServer backend: Python/FastAPI application that handles LLM queries, RAG retrieval, conversation management, and tool execution. Talks to LLM providers directly.
8. LCore backend: Dual-container deployment (Llama Stack + Lightspeed Stack) that provides the same API but routes through Llama Stack for LLM communication, enabling agent-based tool use and provider abstraction.
9. Operator-level resources: ServiceMonitor for operator metrics, NetworkPolicy restricting operator pod access.

### Lifecycle

10. On CR creation: the operator adds a finalizer, then reconciles all component resources in two phases.
11. On CR update: the operator re-reconciles, detecting changes via resource version tracking and content hashing.
12. On CR deletion: the operator runs finalizer cleanup -- removes console UI from the Console CR, explicitly deletes all owned resources, waits for deletion to complete, then removes the finalizer.
13. The operator reports status via conditions (ApiReady, CacheReady, ConsolePluginReady, ResourceReconciliation) and an aggregate OverallStatus (Ready/NotReady).
14. When deployments are unhealthy, the operator collects pod-level diagnostics and populates status.diagnosticInfo with container failure details.

### Deployment Model

15. The operator runs as a single-instance deployment in the openshift-lightspeed namespace (configurable).
16. It supports leader election for HA deployments.
17. Images for all operands are configurable via command-line flags, with defaults embedded in the binary.

### Integration Points

18. The operator reads OpenShift cluster version to select the correct console plugin image (PatternFly 5 for OCP < 4.19, PatternFly 6 for OCP >= 4.19).
19. The operator detects Prometheus Operator availability and conditionally creates ServiceMonitor and PrometheusRule resources.
20. The operator uses the OpenShift service-ca operator for automatic TLS certificate generation (unless custom certificates are provided).
21. The operator watches the telemetry pull secret in openshift-config namespace to determine whether data collection is enabled.

## Configuration Surface

### Operator Startup Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--use-lcore` | bool | `false` | Select LCore backend instead of AppServer |
| `--lcore-server` | bool | `true` | LCore server mode (two containers) vs library mode (one container) |
| `--namespace` | string | `WATCH_NAMESPACE` env or `openshift-lightspeed` | Operator namespace |
| `--leader-elect` | bool | `false` | Enable leader election |
| `--secure-metrics-server` | bool | `false` | Enable mTLS for operator metrics |
| `--service-image` | string | built-in default | Override lightspeed-service container image |
| `--console-image` | string | built-in default | Override console plugin image (PatternFly 6) |
| `--console-image-pf5` | string | built-in default | Override console plugin image (PatternFly 5) |
| `--postgres-image` | string | built-in default | Override PostgreSQL image |
| `--openshift-mcp-server-image` | string | built-in default | Override OpenShift MCP server image |
| `--lcore-image` | string | built-in default | Override Llama Stack / LCore image |
| `--dataverse-exporter-image` | string | built-in default | Override dataverse exporter image |
| `--ocp-rag-image` | string | built-in default | Override OCP RAG database image |

### Environment Variables

| Variable | Description |
|---|---|
| `WATCH_NAMESPACE` | Fallback namespace when `--namespace` is not set |

## Constraints

1. Only one OLSConfig CR named "cluster" is processed. Others are silently ignored.
2. AppServer and LCore are mutually exclusive. The choice is made at operator startup and cannot be changed without restarting the operator.
3. The operator must be able to run in disconnected (air-gapped) environments. All image references must be overridable.
4. The operator must function correctly with or without Prometheus Operator installed.

## Planned Changes

| Jira | Summary |
|---|---|
| OLS-2322 | Streamline OLSConfig CR deployment configuration |
| OLS-2323 | Extend OLSConfig CR to report specific deployment errors |
| OLS-2325 | Create type-safe log-level definition in the operator CR |
| OLS-2140 | Remove time-based operator reconciliation (completed -- now fully event-driven) |
