# Architecture

This document describes the internal architecture of the OpenShift Lightspeed Operator codebase.

## Overview

The operator follows a modular, component-based architecture where each major component (application server, PostgreSQL, chat console plugin, agentic console plugin, alerts adapter) is managed by its own dedicated package with independent reconciliation logic.

## Key Design Decisions

### Why Component Packages?
- **Modularity**: Each component is self-contained
- **Maintainability**: Changes to one component don't affect others
- **Testability**: Independent test suites per component
- **Code Organization**: Clear boundaries and responsibilities

### Why Reconciler Interface?
- **Avoid Circular Dependencies**: Components don't import main controller
- **Clean Testing**: Easy to create test implementations
- **Flexibility**: Main controller can evolve without breaking components

### Why Two Resource Watching Approaches?
- **Owned Resources (ResourceVersion)**: Auto-cleanup on deletion, Kubernetes-native lifecycle
- **External Resources (Data Comparison)**: Respects user ownership, supports cross-namespace sharing
- **Right Tool for Job**: Operator resources need lifecycle management, user resources need change tracking without interference

### Why ResourceVersion-Based Detection?
- **Efficiency**: Only update when resources actually change
- **Reliability**: Leverages Kubernetes' built-in change tracking
- **Simplicity**: No custom hash computation or state management
- **Correctness**: Kubernetes guarantees ResourceVersion changes on modification

## Component Overview

### Main Controller (`olsconfig_controller.go`, `olsconfig_helpers.go`, `operator_assets.go`)

**Core Orchestration:**
- Main `Reconcile()` method coordinates all reconciliation phases
- `SetupWithManager()` configures controller watches and event handlers
- Reconciles the application server via `appserver` (see `ReconcileAppServerResources` / `ReconcileAppServerDeployment` in `olsconfig_controller.go`)

**Support Functions:**
- Implements `reconciler.Reconciler` interface (provides config/images to components)
- Watcher predicate helpers for filtering events
- Status management and deployment health checks
- External resource annotation for change tracking

**Operator Infrastructure:**
- ServiceMonitor for operator metrics
- NetworkPolicy for operator security

### Entry Point (`cmd/main.go`)

**Responsibilities:**
- Parse command-line flags (images, namespace, reconcile interval, backend selection)
- Initialize controller manager with TLS, metrics, health probes
- **Configure WatcherConfig** - declarative setup defining all watched external resources (secrets, configmaps)
- Detect OpenShift version and select appropriate images
- Start controller and handle graceful shutdown

**Key Flags:** Image URLs (`--service-image`, `--console-image`, `--agentic-console-image`, `--alerts-adapter-image`, etc.), `--namespace`, and related runtime options. See `cmd/main.go` for the complete list. `make run` sets `LOCAL_DEV_MODE=true` to skip operator metrics resources during local development.

### Reconciler Interface (`internal/controller/reconciler`)

Provides clean contract between main controller and component packages:
- **Dependency Injection**: Components receive only what they need
- **No Circular Dependencies**: Components don't import main controller
- **Testability**: Easy to mock for unit tests
- Exposes: Kubernetes client, logger, namespace, image getters, configuration

### Application Server Package (`internal/controller/appserver`)

**Purpose:** Manages the OpenShift Lightspeed application server (LLM API, RAG, optional MCP, metrics).

**Entry Points:** `ReconcileAppServerResources` and `ReconcileAppServerDeployment` (invoked from `olsconfig_controller.go`).

### PostgreSQL Package (`internal/controller/postgres`)

**Purpose:** Manages PostgreSQL database for conversation cache storage

**Entry Point:** `ReconcilePostgres(reconciler.Reconciler, context, *OLSConfig)`

### Console UI Package (`internal/controller/console`)

**Purpose:** Manages the chat OpenShift Console plugin (Lightspeed assistant UI).

**Entry Points:** `ReconcileConsoleUIResources`, `ReconcileConsoleUIDeploymentAndPlugin`, `RemoveConsoleUI()`.

**Notes:** Includes a `ConsolePlugin` proxy to the app-server for API calls. Shared reconcile logic lives in `utils/console_plugin_reconciler.go`.

### Agentic Console UI Package (`internal/controller/agenticconsole`)

**Purpose:** Manages the agentic OpenShift Console plugin (AI Hub: proposals and configuration UI).

**Entry Points:** `ReconcileAgenticConsoleUIResources`, `ReconcileAgenticConsoleUIDeploymentAndPlugin`, `RemoveAgenticConsole()`.

**Notes:** No app-server proxy on the `ConsolePlugin` CR. Reuses `utils/console_plugin_reconciler.go`. Status condition: `AgenticConsolePluginReady`. CR tuning: `spec.ols.deployment.agenticConsole`.

### Alerts Adapter Package (`internal/controller/alertsadapter`)

**Purpose:** Manages the agentic alerts adapter that polls Alertmanager and creates `AgenticRun` CRs for firing alerts.

**Opt-in:** Enabled only when `spec.ols.deployment.alertsAdapter.configMapRef` is set (non-empty name). When unset, Phase 1 calls `RemoveAlertsAdapter()` to tear down operand resources and Phase 2 sets `AlertsAdapterReady=True` with `Reason=NotConfigured`.

**Entry Points:** `ReconcileAlertsAdapterResources()` (Phase 1), `ReconcileAlertsAdapterDeployment()` (Phase 2), `RemoveAlertsAdapter()` (operand teardown on disable and during finalization), `RestartAlertsAdapter()` (rolling restart on deployment spec or runtime ConfigMap changes).

**Phase 1 resources (when enabled):** ServiceAccount, ClusterRole/ClusterRoleBinding for `agentic.openshift.io/agenticruns`, legacy config Role/RoleBinding cleanup (removed from reconcile; deleted if still present), RoleBinding in `openshift-monitoring` to `monitoring-alertmanager-view`, NetworkPolicy. The operator does not create, update, or validate user ConfigMap data.

**Runtime config:** User creates the ConfigMap (see [adapter manifests](https://github.com/openshift/lightspeed-agentic-alerts-adapter/tree/main/manifests)). When the referenced ConfigMap exists, it is mounted read-only at `/etc/alerts-adapter`; when absent, no config volume is mounted. The adapter reads `config.yaml` from that path and uses built-in defaults when the file is missing or invalid. ConfigMap data changes trigger a deployment restart via the external ConfigMap watcher (`RestartAlertsAdapter`).

**Phase 2 resources:** Deployment (`lightspeed-agentic-alerts-adapter`, 1 replica) with `ALERTMANAGER_URL` and `POD_NAMESPACE` env vars, and conditional ConfigMap volume mount as above. Image from `--alerts-adapter-image` / `GetAlertsAdapterImage()`.

### Utilities Package (`internal/controller/utils`)

**Purpose:** Shared functionality across all components

**Contains:**
- Constants (resource names, labels, annotations, error messages)
- Console plugin shared reconcilers (`console_plugin_reconciler.go`)
- Helper functions (hash computation, resource comparison, equality checks)
- Status utilities (condition management)
- Validation (certificates, version detection)
- Test helpers (shared fixtures, test reconciler, CR generators)

### Watchers Package (`internal/controller/watchers`)

**Purpose:** External resource watching with multi-level filtering

**Architecture:**
1. **Predicate Filtering** - Fast O(1) event filtering at watch level
2. **Data Comparison** - Deep equality checks using `apiequality.Semantic.DeepEqual()`
3. **Restart Logic** - Maps changed resources to affected deployments via WatcherConfig

**Configuration:** All watcher behavior defined in `cmd/main.go` via `WatcherConfig` (data-driven, no hardcoded resource names)

**Watches:** OpenShift system resources and user-provided resources referenced in OLSConfig.

See `internal/controller/watchers/` and `cmd/main.go` for implementation details.

## Reconciliation Flow

High-level reconciliation sequence:

```
1. Reconcile operator-level resources (ServiceMonitor, NetworkPolicy)
2. Check if CR is being deleted → run finalizer cleanup if needed
3. Add finalizer if not present
4. Validate OLSConfig CR exists
5. Annotate external resources; validate LLM credentials and TLS secrets
6. Phase 1 — independent resources (continue on error):
   - Chat console UI (`console/`)
   - Agentic console UI (`agenticconsole/`)
   - PostgreSQL (`postgres/`)
   - Application server (`appserver/`)
   - Alerts adapter (`alertsadapter/`, when `configMapRef` set; else `RemoveAlertsAdapter()`)
7. Phase 2 — deployments and status (fail-fast on pod failures):
   - Chat console UI → ConsolePluginReady
   - Agentic console UI → AgenticConsolePluginReady
   - PostgreSQL → CacheReady
   - Application server → ApiReady
   - Alerts adapter → AlertsAdapterReady (or `NotConfigured` when `configMapRef` unset)
```

### Finalizer Pattern

The operator uses a finalizer (`ols.openshift.io/finalizer`) to ensure proper cleanup when `OLSConfig` CR is deleted.

**Why Needed:**
- **Console plugin cleanup**: `ConsolePlugin` is cluster-scoped and not cascade-deleted by owner references; chat and agentic plugins must be deactivated in the Console CR
- **Alerts adapter cleanup**: RoleBinding in `openshift-monitoring` is outside the operator namespace; `RemoveAlertsAdapter()` also deletes deployment, namespaced RBAC, SA, and NetworkPolicy when the operand is disabled or during finalization. AgenticRun ClusterRole/ClusterRoleBinding deletion may be blocked on managed OpenShift clusters (admission webhook); the operator logs and continues.
- **PVC cleanup**: PersistentVolumeClaims can block deletion if not properly released
- **Race condition prevention**: Ensures complete cleanup before CR can be recreated (important for tests and sequential deployments)

**Implementation** (`internal/controller/olsconfig_controller.go`):

```go
// Finalizer is added on first reconciliation
if !controllerutil.ContainsFinalizer(olsconfig, utils.OLSConfigFinalizer) {
    controllerutil.AddFinalizer(olsconfig, utils.OLSConfigFinalizer)
    r.Update(ctx, olsconfig)
    return
}

// On deletion, run cleanup before removing finalizer
if !olsconfig.DeletionTimestamp.IsZero() {
    if controllerutil.ContainsFinalizer(olsconfig, utils.OLSConfigFinalizer) {
        r.finalizeOLSConfig(ctx, olsconfig)  // Cleanup logic
        controllerutil.RemoveFinalizer(olsconfig, utils.OLSConfigFinalizer)
        r.Update(ctx, olsconfig)
    }
    return
}
```

**Cleanup Sequence** (`finalizeOLSConfig`):
1. **Remove chat console UI**: Deactivate plugin from Console CR, delete ConsolePlugin CR
2. **Remove agentic console UI**: Deactivate plugin from Console CR, delete ConsolePlugin CR
3. **Remove alerts adapter operand**: Delete deployment, namespaced RBAC, SA, NetworkPolicy, monitoring RoleBinding, and attempt AgenticRun ClusterRoleBinding/ClusterRole deletion (`alertsadapter.RemoveAlertsAdapter()`). ClusterRoleBinding deletion may be blocked on managed OpenShift; remaining cluster RBAC is harmless when the operand is disabled.
4. **Wait for owned resources**: Poll for up to 3 minutes until deployments, services, PVCs are deleted (cascade deletion)
5. **Remove finalizer**: Allows Kubernetes to remove CR from etcd

**Error Handling:**
- Cleanup errors are logged but don't block finalizer removal
- Prevents CRs from being stuck in `Terminating` state
- Console UI removal handles missing Console CR gracefully (test environments, non-OpenShift clusters)

**Testing:**
- Unit tests: `internal/controller/olsconfig_finalizer_test.go`
- Test helper: `cleanupOLSConfig()` in `suite_test.go` (removes finalizers, waits for deletion)
- E2E test timeout: 3 minutes for `DeleteAndWait()` to account for finalizer cleanup

## Resource Management

**Implementation:** See `internal/controller/watchers/` for watcher logic, `cmd/main.go` for WatcherConfig, `olsconfig_helpers.go` for annotation logic.

The operator uses two distinct approaches for different resource ownership models:

### Owned Resources (Operator-Managed)

Resources created and fully managed by the operator (Deployments, Services, operator-generated ConfigMaps/Secrets).

**Change Detection:**
- Uses Kubernetes owner references (`controllerutil.SetControllerReference`)
- Monitored via `Owns()` in `SetupWithManager()`
- Changes detected through ResourceVersion tracking in deployment annotations
- Automatic reconciliation on modification/deletion

**Benefits:** Auto-cleanup on CR deletion, Kubernetes-native lifecycle, efficient change detection

### External Resources (User-Provided)

Resources created by users, referenced but not owned by the operator (LLM credentials, user TLS certs, CA ConfigMaps, OpenShift system resources).

**Change Detection:**
- Uses watcher annotations (`ols.openshift.io/watch-olsconfig`) and name-based filtering
- Monitored via `Watches()` with custom event handlers
- Changes detected through data comparison (`apiequality.Semantic.DeepEqual`)
- Targeted deployment restarts configured via `WatcherConfig` in `cmd/main.go`

**Benefits:** Respects user ownership, supports cross-namespace sharing, fine-grained restart control

---

## Testing & Documentation

**Testing:** See [CONTRIBUTING.md](CONTRIBUTING.md) for testing strategy, test helpers, and running tests. Unit tests use Ginkgo/Gomega, E2E tests in `test/e2e/`. Always use `make test` (never `go test` directly).

**OLM Documentation:** For operators deployed via OLM, see comprehensive guides in `docs/`:
- [OLM Bundle Management](./docs/olm-bundle-management.md)
- [OLM Catalog Management](./docs/olm-catalog-management.md)
- [OLM Integration & Lifecycle](./docs/olm-integration-lifecycle.md)
- [OLM Testing & Validation](./docs/olm-testing-validation.md)
- [OLM RBAC & Security](./docs/olm-rbac-security.md)

**Contributing:** For adding components or modifying existing ones, see [CONTRIBUTING.md](CONTRIBUTING.md) for detailed step-by-step instructions.

**Coding Conventions:** See [AGENTS.md](AGENTS.md) for coding conventions and patterns used in this codebase.

---

For user-facing documentation, see [README.md](README.md).