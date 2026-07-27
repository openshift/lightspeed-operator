# System Overview

The OpenShift Lightspeed Operator is a Kubernetes operator that manages the lifecycle of the OpenShift Lightspeed (OLS) AI assistant stack on an OpenShift cluster. It reconciles a single cluster-scoped OLSConfig custom resource into a set of Kubernetes resources that form the complete Lightspeed deployment.

## Behavioral Rules

### Operator Role

1. OLSConfig is treated as a singleton per cluster: the operator only reconciles the cluster-scoped instance named "cluster". Any other OLSConfig objects are ignored. Reconciled workloads are created in the openshift-lightspeed namespace.
2. The operator deploys and manages three components: an application server backend, a PostgreSQL database, and a Console UI plugin (Lightspeed chat), plus operator-level monitoring/networking resources.
3. The operator is fully event-driven. It does not use periodic/timer-based reconciliation. All changes are detected via Kubernetes watches on owned resources and annotated external resources.

### Co-deployed Agentic Controller

4. The lightspeed-operator OLM bundle deploys **two controllers** side by side: the lightspeed-operator controller and the lightspeed-agentic-operator controller. OLM applies all static manifests (deployments, service accounts, roles, role bindings, CRDs) for both controllers at installation time.
5. The lightspeed-operator controller has no runtime interaction with the lightspeed-agentic-operator controller. The two reconcile different API groups (`ols.openshift.io` vs `agentic.openshift.io`) and share no runtime state.
6. Feature gates on `OLSConfig` (`MCPServer`, `ToolFiltering`) do not control the activation of the agentic controller.
7. The agentic controller is inert until its CRs (`AgenticOLSConfig`, `Agent`, `LLMProvider`, `ApprovalPolicy`, etc.) are created.
8. The agentic controller image is specified in the CSV deployment spec, following the same pattern as the lightspeed-operator's own controller image. Operand images for the agentic controller (agentic console plugin, etc.) are configured via startup flags on the agentic controller deployment.
8a. The agentic controller accepts a `--sandbox-mode` flag (`bare-pod` default, `sandbox-claim` optional) that determines whether agent sandboxes run as bare Pods or use the Agent Sandbox API. In `bare-pod` mode, no Agent Sandbox API CRDs need to be installed. Classic operator also publishes `spec.agenticOLS.sandboxMode` and a thin PodSpec via `lightspeed-agentic-configuration` for agentic-operator to consume (see `agentic-sandbox-profile.md`); the sandbox container image in that PodSpec is set from classic `--agentic-sandbox-image`.
9. See `bundle-composition.md` for details on the bundle structure, CRD ownership, and image references.

### Component Inventory

10. Application server backend: Python/FastAPI application (lightspeed-service) that handles LLM queries, RAG retrieval, conversation management, and tool execution. Talks to LLM providers directly.
11. PostgreSQL: single-replica database providing conversation cache and quota state. Always deployed.
12. Console UI Plugin: OpenShift console extension that provides the Lightspeed chat interface. Integrates via ConsolePlugin CR and proxies requests to the backend.
13. Operator-level resources: ServiceMonitor for operator metrics, NetworkPolicy restricting operator pod access.

### Lifecycle

14. On CR creation: the operator adds a finalizer, then reconciles all component resources in two phases.
15. On CR update: the operator re-reconciles, detecting changes via resource version tracking and content hashing.
16. On CR deletion: the operator runs finalizer cleanup -- removes console UI from the Console CR, explicitly deletes all owned resources, waits for deletion to complete, then removes the finalizer.
17. The operator reports status via conditions (ApiReady, CacheReady, ConsolePluginReady, ResourceReconciliation) and an aggregate OverallStatus (Ready/NotReady).
18. When deployments are unhealthy, the operator collects pod-level diagnostics and populates status.diagnosticInfo with container failure details.

### Deployment Model

19. The operator runs as a single-instance deployment in the openshift-lightspeed namespace (configurable).
20. It supports leader election for HA deployments.
21. Images for all operands are configurable via command-line flags, with defaults embedded in the binary.

### Integration Points

22. The operator deploys a single console plugin image configured via `--console-image`. After detecting the OpenShift cluster version (major.minor), it sets `OCP_VERSION` on the console plugin container so the console image entrypoint can select the correct UI library at runtime. The operator no longer chooses among multiple console images by OCP minor version; version detection is still used for this env var and for other component configuration.
23. The operator detects Prometheus Operator availability and conditionally creates ServiceMonitor and PrometheusRule resources (operator, app-server, and OTEL Collector monitors).
24. The operator uses the OpenShift service-ca operator for automatic TLS certificate generation (unless custom certificates are provided).
25. The operator watches the telemetry pull secret in openshift-config namespace to determine whether data collection is enabled.

## Configuration Surface

### Operator Startup Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--namespace` | string | `WATCH_NAMESPACE` env or `openshift-lightspeed` | Operator namespace |
| `--leader-elect` | bool | `false` | Enable leader election |
| `--secure-metrics-server` | bool | `false` | Enable mTLS for operator metrics |
| `--service-image` | string | built-in default | Override lightspeed-service container image |
| `--console-image` | string | built-in default | Override console plugin image |
| `--postgres-image` | string | built-in default | Override PostgreSQL image |
| `--openshift-mcp-server-image` | string | built-in default | Override OpenShift MCP server image |
| `--dataverse-exporter-image` | string | built-in default | Override dataverse exporter image |
| `--rhokp-image` | string | `related_images.json` (`rhokp`) | Override RHOKP (Solr / OKP) sidecar image |
| `--agentic-sandbox-image` | string | `related_images.json` (`lightspeed-agentic-sandbox`) | Override sandbox container image embedded in the handoff PodSpec |

### Environment Variables

| Variable | Description |
|---|---|
| `WATCH_NAMESPACE` | Fallback namespace when `--namespace` is not set |

## Constraints

1. Only one OLSConfig CR named "cluster" is processed. Others are silently ignored.
2. The operator must be able to run in disconnected (air-gapped) environments. All image references must be overridable.
3. The operator must function correctly with or without Prometheus Operator installed.

## Planned Changes

None.
