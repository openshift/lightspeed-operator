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
  -> reconcileIndependentResources()        # Phase 1: console, postgres, backend resources
  |   |-- console.ReconcileConsoleUIResources()
  |   |-- postgres.ReconcilePostgresResources()
  |   +-- appserver.ReconcileAppServerResources() OR lcore.ReconcileLCoreResources()
  -> reconcileDeploymentsAndStatus()        # Phase 2: deployments + status update
      |-- console.ReconcileConsoleUIDeploymentAndPlugin()
      |-- postgres.ReconcilePostgresDeployment()
      |-- appserver.ReconcileAppServerDeployment() OR lcore.ReconcileLCoreDeployment()
      |-- checkDeploymentStatus() for each  # Collect diagnostics
      +-- UpdateStatusCondition()           # Single status update
```

## Key Abstractions

### Reconciler Interface
The `reconciler.Reconciler` interface breaks the circular dependency between the main controller and component packages. Component packages (appserver, lcore, postgres, console) receive this interface instead of importing the controller package directly. It embeds `client.Client` and adds getter methods for images, namespace, feature flags, and OpenShift version.

### ReconcileSteps Pattern
Both phases use a slice of `ReconcileSteps` structs, each containing a Name, reconcile function, and (for Phase 2) a ConditionType and Deployment name. Phase 1 iterates with continue-on-error; Phase 2 iterates but tracks all conditions and diagnostics.

### Resource Ownership
Two ownership models:
1. **Owned resources**: Controller-runtime Owns() declarations. Owner references set on creation. Changes trigger reconciliation automatically.
2. **External resources**: Watches() with custom predicates. Annotation-based filtering. Secret/ConfigMap handlers compare data and trigger deployment restarts.

### Finalizer Cleanup
The `finalizeOLSConfig()` method uses `listOwnedResources()` which queries every resource type by owner reference UID (not labels). This is more reliable than label-based cleanup. The wait loop polls with a fixed interval and timeout, using `wait.PollUntilContextTimeout`.

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

- `SetupWithManager()` registers Owns() for 12 resource types and Watches() for Secrets and ConfigMaps with custom predicates.
- Secret watch predicates: Create events allowed for all secrets in operator namespace (handles recreated secrets); Update events filtered by watcher annotation; Delete events ignored.
- ConfigMap watch predicates: Same pattern as secrets.
- The `LOCAL_DEV_MODE` environment variable skips ServiceMonitor creation when running locally.
- Phase 1 failures update status with `ResourceReconciliation` condition type (not the component-specific types used in Phase 2).
