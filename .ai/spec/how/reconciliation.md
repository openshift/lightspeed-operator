# Reconciliation Architecture

## Module Map

| File | Key Symbols | Responsibility |
|---|---|---|
| `internal/controller/olsconfig_controller.go` | `OLSConfigReconciler`, `Reconcile()`, `SetupWithManager()` | Main reconciler, orchestration, watcher setup |
| `internal/controller/olsconfig_helpers.go` | `UpdateStatusCondition()`, `checkDeploymentStatus()`, `annotateExternalResources()` | Status management, diagnostics, resource annotation |
| `internal/controller/operator_assets.go` | `ReconcileServiceMonitorForOperator()`, `ReconcileNetworkPolicyForOperator()` | Operator-level resources |
| `internal/controller/reconciler/interface.go` | `Reconciler` interface | Dependency injection for component packages |

## Data Flow

Main reconciliation loop:
```
Reconcile(ctx, req)
  -> getAndValidateCR()                    # Fetch CR, validate name == "cluster"
  -> handleFinalizer()                      # Add/remove finalizer, run cleanup
  -> reconcileOperatorResources()           # ServiceMonitor, NetworkPolicy (operator-level)
  -> annotateExternalResources()            # Validate secrets, annotate for watching
  -> reconcileIndependentResources()        # Phase 1 (continue-on-error; order below matches code)
  |   |-- console.ReconcileConsoleUIResources()
  |   |-- postgres.ReconcilePostgresResources()
  |   |-- ocpmcp.ReconcileResources()
  |   |   (when introspectionEnabled; else ocpmcp.Remove())
  |   |-- rhokp.ReconcileResources()
  |   |   (when !byokRAGOnly; else rhokp.Remove())
  |   |-- agenticconsole.ReconcileAgenticConsoleUIResources()
  |   |-- alertsadapter.ReconcileAlertsAdapterResources()
  |   |   (opt-in via configMapRef; RemoveAlertsAdapter() when disabled; no ConfigMap validation;
  |   |    mount at /etc/alerts-adapter when CM exists)
  |   |-- otelcollector.ReconcileOtelCollectorResources()     # runtime resources; gate-managed exporter ConfigMap, two ClusterRoles, existing ClusterRoleBinding, and openshift-config RoleBinding; transcript/token gate treats NotFound as off and malformed/missing .dockerconfigjson or other API read errors as Phase 1 failures
  |   +-- appserver.ReconcileAppServerResources()
  -> reconcileDeploymentsAndStatus()        # Phase 2: deployments + status update (order below matches code)
      |-- console.ReconcileConsoleUIDeploymentAndPlugin()   # ConsolePluginReady
      |-- postgres.ReconcilePostgresDeployment()            # CacheReady
      |-- ocpmcp.ReconcileDeployment()                      # MCPServerReady / NotConfigured
      |-- rhokp.ReconcileDeployment()                       # RHOKPReady / NotConfigured
      |-- appserver.ReconcileAppServerDeployment()          # ApiReady (MCP/RHOKP Services already reconciled)
      |-- otelcollector.ReconcileOtelCollectorDeployment()  # gated Dataverse sidecar; tracks its separate ConfigMap
      |-- agenticconsole.ReconcileAgenticConsoleUIDeploymentAndPlugin() # AgenticConsolePluginReady
      |-- alertsadapter.ReconcileAlertsAdapterDeployment()  # when configMapRef set
      |   (each deployment step above: checkDeploymentStatus → conditions)
      |-- agenticintegration.ReconcileAgenticIntegrationResources()  # last (separate call after the loop): ConfigMap only — no deployment health check; failure → OverallStatus NotReady
      +-- UpdateStatusCondition()           # Single status update
```

## Key Abstractions

### Reconciler Interface
The `reconciler.Reconciler` interface breaks the circular dependency between the main controller and component packages. Component packages (appserver, postgres, otelcollector, ocpmcp, rhokp, agenticintegration, console, agenticconsole, alertsadapter) receive this interface instead of importing the controller package directly. It embeds `client.Client` and adds getter methods for images, namespace, and OpenShift version.

### ReconcileSteps Pattern
Both phases use a slice of `ReconcileSteps` structs, each containing a Name, reconcile function, and (for Phase 2) a ConditionType and Deployment name. Phase 1 iterates with continue-on-error; Phase 2 iterates but tracks all conditions and diagnostics.

### Resource Ownership
Two ownership models:
1. **Owned resources**: Controller-runtime Owns() declarations. Owner references set on creation. Changes trigger reconciliation automatically.
2. **External resources**: Watches() with custom predicates. Annotation-based filtering. Secret/ConfigMap handlers compare data and trigger affected deployment restarts on update. Telemetry pull-secret create/update/delete events also enqueue OLSConfig reconciliation so the OTel Dataverse gate and sidecar-only resources are reevaluated. With transcripts enabled, a NotFound pull-secret object is gate-off, as is valid JSON with absent/empty telemetry auth; a missing `.dockerconfigjson` key, malformed JSON, or other API read errors are reconciliation errors, not disabled auth.

### Finalizer Cleanup
`finalizeOLSConfig()` removes Console UI and deletes alerts adapter operand resources via `alertsadapter.RemoveAlertsAdapter()` (deployment, namespaced RBAC, SA, NetworkPolicy, cross-namespace monitoring RoleBinding; AgenticRun ClusterRole/ClusterRoleBinding when permitted—may remain on managed OpenShift if admission webhook blocks delete). It then uses `listOwnedResources()` only to inventory owned objects in the operator namespace by OwnerReference UID (not labels), explicitly deletes that inventory, and waits for the deletions with `wait.PollUntilContextTimeout`. The wait is bounded; if it times out, finalization continues without guaranteeing all listed objects have disappeared before finalizer removal. This inventory includes the OTel Dataverse exporter ConfigMap but not its two ClusterRoles, ClusterRoleBinding, or `openshift-config` RoleBinding. Those RBAC objects carry OLSConfig owner references and rely on asynchronous Kubernetes garbage collection on CR deletion; the finalizer does not explicitly delete or wait for them and does not guarantee they are gone before OLSConfig recreation.

### Status Update Mechanics
`UpdateStatusCondition()` uses `retry.RetryOnConflict` with `client.MergeFrom` patch. It preserves `LastTransitionTime` for conditions whose status hasn't changed. It re-fetches the CR before each update attempt to get the latest ResourceVersion.

### Deployment Health Check
`checkDeploymentStatus()` returns one of three states:
- "Ready": `DeploymentAvailable` condition is True
- "Failed": Terminal pod failures detected (CrashLoopBackOff, ImagePullBackOff, etc.)
- "Progressing": Not ready but no terminal failures

`collectDeploymentDiagnostics()` lists pods matching the deployment's selector and inspects:
- Container statuses (Waiting with reason, Terminated with non-zero exit)
- Last termination state (for CrashLoopBackOff context)
- Init container statuses
- Pod scheduling conditions (Unschedulable)
- Pod readiness conditions
- Pod phase (Failed, Unknown)

## Integration Points

| Consumer | Provider | Mechanism |
|---|---|---|
| Component packages | Main controller | `reconciler.Reconciler` interface |
| Watcher handlers | Component restart functions | `watchers.SecretUpdateHandler`, `watchers.ConfigMapUpdateHandler` |
| Status updates | Kubernetes API | `retry.RetryOnConflict` with `client.MergeFrom` patch |
| Finalizer cleanup | Kubernetes API | Owner reference UID matching + explicit delete |

## Implementation Notes

- `SetupWithManager()` registers `Owns()` watches for 13 resource types, including RoleBinding, and Watches() for Secrets and ConfigMaps with custom predicates.
- Secret watch predicates: operator-namespace create events are allowed; the configured `openshift-config/pull-secret` also accepts create/recreation events. Update events are filtered by watcher annotation or system-resource rules; pull-secret data updates refresh affected exporter Deployments and enqueue OLSConfig reconciliation. Delete events are allowed for operator-namespace secrets and configured system secrets elsewhere, and enqueue reconciliation for the pull secret.
- ConfigMap watch predicates: Same pattern as secrets.
- The `LOCAL_DEV_MODE` environment variable skips operator ServiceMonitor creation and app-server metrics reader secret reconciliation when running locally (`make run`).
- Phase 1 failures update status with `ResourceReconciliation` condition type (not the component-specific types used in Phase 2).
