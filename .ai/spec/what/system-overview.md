# System Overview

The OpenShift Lightspeed Operator is a Kubernetes operator that manages the lifecycle of the OpenShift Lightspeed (OLS) AI assistant stack on an OpenShift cluster. It reconciles a single cluster-scoped OLSConfig custom resource into a set of Kubernetes resources that form the complete Lightspeed deployment.

## Behavioral Rules

### Operator Role

1. OLSConfig is treated as a singleton per cluster: the operator only reconciles the cluster-scoped instance named "cluster". Any other OLSConfig objects are ignored. Reconciled workloads are created in the openshift-lightspeed namespace.
2. The operator deploys and manages three components: an application server backend, a PostgreSQL database, and a Console UI plugin, plus operator-level monitoring/networking resources.
3. The operator is fully event-driven. It does not use periodic/timer-based reconciliation. All changes are detected via Kubernetes watches on owned resources and annotated external resources.

### Component Inventory

4. Application server backend: Python/FastAPI application (lightspeed-service) that handles LLM queries, RAG retrieval, conversation management, and tool execution. Talks to LLM providers directly.
5. PostgreSQL: single-replica database providing conversation cache and quota state. Always deployed.
6. Console UI Plugin: OpenShift console extension that provides the Lightspeed chat interface. Integrates via ConsolePlugin CR and proxies requests to the backend.
7. Operator-level resources: ServiceMonitor for operator metrics, NetworkPolicy restricting operator pod access.

### Lifecycle

8. On CR creation: the operator adds a finalizer, then reconciles all component resources in two phases.
9. On CR update: the operator re-reconciles, detecting changes via resource version tracking and content hashing.
10. On CR deletion: the operator runs finalizer cleanup -- removes console UI from the Console CR, explicitly deletes all owned resources, waits for deletion to complete, then removes the finalizer.
11. The operator reports status via conditions (ApiReady, CacheReady, ConsolePluginReady, ResourceReconciliation) and an aggregate OverallStatus (Ready/NotReady).
12. When deployments are unhealthy, the operator collects pod-level diagnostics and populates status.diagnosticInfo with container failure details.

### Deployment Model

13. The operator runs as a single-instance deployment in the openshift-lightspeed namespace (configurable).
14. It supports leader election for HA deployments.
15. Images for all operands are configurable via command-line flags, with defaults embedded in the binary.

### Integration Points

16. The operator reads OpenShift cluster version to select the correct console plugin image (PatternFly 5 for OCP < 4.19, PatternFly 6 for OCP >= 4.19).
17. The operator detects Prometheus Operator availability and conditionally creates ServiceMonitor and PrometheusRule resources.
18. The operator uses the OpenShift service-ca operator for automatic TLS certificate generation (unless custom certificates are provided).
19. The operator watches the telemetry pull secret in openshift-config namespace to determine whether data collection is enabled.

## Configuration Surface

### Operator Startup Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--namespace` | string | `WATCH_NAMESPACE` env or `openshift-lightspeed` | Operator namespace |
| `--leader-elect` | bool | `false` | Enable leader election |
| `--secure-metrics-server` | bool | `false` | Enable mTLS for operator metrics |
| `--service-image` | string | built-in default | Override lightspeed-service container image |
| `--console-image` | string | built-in default | Override console plugin image (PatternFly 6) |
| `--console-image-pf5` | string | built-in default | Override console plugin image (PatternFly 5) |
| `--postgres-image` | string | built-in default | Override PostgreSQL image |
| `--openshift-mcp-server-image` | string | built-in default | Override OpenShift MCP server image |
| `--dataverse-exporter-image` | string | built-in default | Override dataverse exporter image |
| `--ocp-rag-image` | string | built-in default | Override OCP RAG database image |

### Environment Variables

| Variable | Description |
|---|---|
| `WATCH_NAMESPACE` | Fallback namespace when `--namespace` is not set |

## Constraints

1. Only one OLSConfig CR named "cluster" is processed. Others are silently ignored.
2. The operator must be able to run in disconnected (air-gapped) environments. All image references must be overridable.
3. The operator must function correctly with or without Prometheus Operator installed.

## Planned Changes

| Jira | Summary |
|---|---|
| OLS-2322 | Streamline OLSConfig CR deployment configuration |
| OLS-2323 | Extend OLSConfig CR to report specific deployment errors |
| OLS-2325 | Create type-safe log-level definition in the operator CR |
