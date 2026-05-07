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
9. Three component groups are reconciled in Phase 1: Console UI, PostgreSQL, and the application server.
10. All Phase 1 resource groups are independent and can be reconciled in any order.
11. If any Phase 1 resource fails, the operator continues reconciling the remaining resources, then reports all failures in the CR status with ResourceReconciliation conditions.

### Phase 2: Deployments and Status
12. Three deployments are reconciled in Phase 2: Console UI (condition: ConsolePluginReady), PostgreSQL (condition: CacheReady), and the active backend (condition: ApiReady).
13. After each deployment reconciliation, the operator checks the deployment's health status.
14. Deployment health has three states: Ready (Available condition true), Progressing (not yet available, no terminal failures), Failed (terminal pod failures detected).
15. Terminal pod failures include: CrashLoopBackOff, ImagePullBackOff, ErrImagePull, OOMKilled, and containers terminated with non-zero exit codes after CrashLoopBackOff.
16. If any deployment has pod failures, the operator returns an error to trigger exponential backoff retry.
17. If deployments are progressing, the operator returns an error to trigger retry, enabling early issue detection rather than relying solely on deployment watch events.
18. Status is updated once per reconciliation cycle, covering all component conditions.

### Finalizer Lifecycle
19. On CR creation: add finalizer, return immediately (controller-runtime auto-requeues).
20. On CR deletion: run finalizer cleanup before removing finalizer.
21. Finalizer cleanup sequence: remove Console UI from Console CR, delete ConsolePlugin CR, list all owned resources by owner reference, explicitly delete them, wait for deletion (polling with timeout).
22. If cleanup times out, the finalizer is removed anyway to prevent the CR from being stuck in Terminating state.
23. Console UI removal errors during finalization are logged but do not block finalization.

### Status Conditions
24. The operator sets these condition types: ApiReady, CacheReady, ConsolePluginReady, ResourceReconciliation.
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
