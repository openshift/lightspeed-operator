# Reconciliation

The operator reconciles the OLSConfig CR into Kubernetes resources through a two-phase process with finalizer-based lifecycle management.

## Behavioral Rules

### Reconciliation Trigger
1. Reconciliation is triggered by changes to the OLSConfig CR, any owned resource, or annotated external resources. No periodic reconciliation.
2. The controller handles error retries via controller-runtime exponential backoff. No custom retry logic.

### Reconciliation Order
3. Step 1: Fetch and validate CR (ignore if name != "cluster", return silently if not found)
4. Step 2: Handle finalizer (add if missing, run cleanup if CR being deleted)
5. Step 3: Reconcile operator-level resources (ServiceMonitor, NetworkPolicy)
6. Step 4: Annotate external resources for watching (validate LLM credentials and TLS secrets first)
7. Step 5 (Phase 1): Reconcile independent resources -- ConfigMaps, Secrets, ServiceAccounts, Roles, NetworkPolicies for all components. Uses continue-on-error: reconcile as many as possible, report all failures.
8. Step 6 (Phase 2): Reconcile deployments and dependent resources -- Deployments, Services, TLS certificates, ServiceMonitors, PrometheusRules. After reconciliation, check deployment health and update CR status.

### Phase 1: Independent Resources
9. Six component groups are reconciled in Phase 1: chat Console UI, agentic console plugin, PostgreSQL, OTEL Collector, the application server, and (when enabled) the agentic alerts adapter.
10. All Phase 1 resource groups are independent and can be reconciled in any order.
11. If any Phase 1 resource fails, the operator continues reconciling the remaining resources, then reports all failures in the CR status with ResourceReconciliation conditions.
11a. Alerts adapter (OLS-3348) is **opt-in** via `spec.ols.deployment.alertsAdapter.configMapRef`. When unset, `ReconcileAlertsAdapterResources()` calls `RemoveAlertsAdapter()` to delete operator-managed operand resources (deployment, SA, namespaced RBAC, NetworkPolicy, monitoring RoleBinding; AgenticRun ClusterRole/ClusterRoleBinding when the platform allows delete) and Phase 2 is skipped with `AlertsAdapterReady=True`, `Reason=NotConfigured`.
11b. When `configMapRef` is set, Phase 1 reconciles: ServiceAccount, ClusterRole (`agentic.openshift.io/agenticruns`: create, list, get), ClusterRoleBinding, legacy config Role/RoleBinding cleanup, RoleBinding in `openshift-monitoring` (binds SA to `monitoring-alertmanager-view`), NetworkPolicy. The operator does not create, update, or validate ConfigMap data. When the referenced ConfigMap exists, Phase 2 mounts it at `/etc/alerts-adapter`; when absent, no config volume is mounted. The adapter reads `config.yaml` and uses built-in defaults when the file is missing or invalid.
11c. Agentic console Phase 1 resources: ServiceAccount, ConfigMap (nginx.conf), NetworkPolicy.
11d. OTEL Collector Phase 1 resources (OLS-3510): ConfigMap (collector runtime YAML `lightspeed-otel-collector-config`), ServiceAccount, Postgres DSN Secret, NetworkPolicy.

### Phase 2: Deployments and Status
12. Deployments reconciled in Phase 2: chat Console UI (condition: `ConsolePluginReady`), agentic console plugin (condition: `AgenticConsolePluginReady`), PostgreSQL (condition: `CacheReady`), OTEL Collector (condition: `OtelCollectorReady`), the active backend (condition: `ApiReady`), and (when `configMapRef` set) the agentic alerts adapter (condition: `AlertsAdapterReady`).
12a. Alerts adapter Phase 2 (OLS-3348): Deployment (1 replica, `ALERTMANAGER_URL` env hardcoded to `https://alertmanager-main.openshift-monitoring.svc:9094`, `POD_NAMESPACE` via downward API).
12b. Agentic console Phase 2: Deployment (1 replica, nginx with TLS via service-ca cert), Service (port 9443, serving-cert annotation), ConsolePlugin CR, Console CR activation.
12c. OTEL Collector Phase 2 (OLS-3510): Service (OTLP gRPC `:4317`, HTTP `:4318`, admin HTTPS `:8080`, serving-cert annotation), TLS secret wait (`lightspeed-otel-collector-cert`), client connectivity ConfigMap (`lightspeed-otel-collector-client` with `collector-endpoint`, `admin-endpoint`, `ca.crt`), Deployment (1 replica). Reconciled after PostgreSQL and before the app-server so the collector Service and client ConfigMap are available when the backend / agentic-operator start.
13. After each deployment reconciliation, the operator checks the deployment's health status.
14. Deployment health has three states: Ready (Available condition true), Progressing (not yet available, no terminal failures), Failed (terminal pod failures detected).
15. Terminal pod failures include: CrashLoopBackOff, ImagePullBackOff, ErrImagePull, OOMKilled, and containers terminated with non-zero exit codes after CrashLoopBackOff.
16. If any deployment has pod failures, the operator returns an error to trigger exponential backoff retry.
17. If deployments are progressing, the operator returns an error to trigger retry, enabling early issue detection rather than relying solely on deployment watch events.
18. Status is updated once per reconciliation cycle, covering all component conditions.

### Finalizer Lifecycle
19. On CR creation: add finalizer, return immediately (controller-runtime auto-requeues).
20. On CR deletion: run finalizer cleanup before removing finalizer.
21. Finalizer cleanup sequence: remove chat Console UI from Console CR, delete chat ConsolePlugin CR, remove agentic console plugin from Console CR, delete agentic ConsolePlugin CR (`agenticconsole.RemoveAgenticConsole()`), delete alerts adapter operand resources via `alertsadapter.RemoveAlertsAdapter()` (deployment, namespaced RBAC, SA, NetworkPolicy, monitoring RoleBinding; AgenticRun ClusterRole/ClusterRoleBinding when permitted), list all owned resources by owner reference, explicitly delete them, wait for deletion (polling with timeout).
22. If cleanup times out, the finalizer is removed anyway to prevent the CR from being stuck in Terminating state.
23. Console UI and agentic component removal errors during finalization are logged but do not block finalization.

### Status Conditions
24. The operator sets these condition types: `ApiReady`, `CacheReady`, `ConsolePluginReady`, `AgenticConsolePluginReady`, `OtelCollectorReady`, `AlertsAdapterReady` (`NotConfigured` when `configMapRef` unset; does not block `OverallStatus=Ready`), `ResourceReconciliation`.
25. OverallStatus is Ready only when all deployment conditions are True.
26. OverallStatus is NotReady if any condition is False.
27. When deployments are not ready, diagnosticInfo is populated with per-pod failure details including container name, reason, message, exit code, and diagnostic type.
28. Status updates preserve LastTransitionTime for unchanged conditions.
29. Status updates use retry-on-conflict to handle concurrent modifications.

### Resource Lifecycle
30. The operator tracks resources through two mechanisms: owned resources (via OwnerReferences and `Owns()`) and external resources (via annotation-based watching). See `what/resource-lifecycle.md` for the full specification of both models, including change detection, restart triggers, and cleanup behavior.

## Configuration Surface

Reconciliation behavior is not directly user-configurable. It is driven by the OLSConfig CR spec (see `what/crd-api.md`) and operator startup flags (see `what/system-overview.md`).

## Constraints

1. Phase 1 must complete before Phase 2 begins (deployments depend on ConfigMaps, Secrets, etc.).
2. Finalizer removal must succeed even if cleanup partially fails, to prevent stuck CRs.
3. The operator must not create ServiceMonitor or PrometheusRule resources if Prometheus Operator CRDs are not installed.
4. Status updates must always set OverallStatus (required field after first reconciliation).

## Planned Changes

| Ticket | Summary |
|---|---|
| OLS-3236 | Remove duplicate agentic console deployment from agentic-operator CSV; productize agentic operand images |
| OLS-3526 | Standalone HTTPS ocp-mcp cluster service (replaces sidecar). Full reconciliation rules land with implementation; still in refinement. Related: OLS-3572 (inter-operator handoff), OLS-3594 (deferred agentic auto-injection) |
