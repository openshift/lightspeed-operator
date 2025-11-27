# Architecture

This document describes the internal architecture of the OpenShift Lightspeed Operator codebase.

## Overview

The operator follows a modular, component-based architecture where each major component (application server, Lightspeed Core/Llama Stack, PostgreSQL, Console UI) is managed by its own dedicated package with independent reconciliation logic.

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
- Selects backend: calls either `appserver.ReconcileAppServer()` OR `lcore.ReconcileLCore()` based on `--enable-lcore` flag

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

**Key Flags:** `--enable-lcore` (backend selection), `--controller-namespace`, `--reconcile-interval-minutes`. See `cmd/main.go` for complete list.

### Reconciler Interface (`internal/controller/reconciler`)

Provides clean contract between main controller and component packages:
- **Dependency Injection**: Components receive only what they need
- **No Circular Dependencies**: Components don't import main controller
- **Testability**: Easy to mock for unit tests
- Exposes: Kubernetes client, logger, namespace, image getters, configuration

### Application Server Package (`internal/controller/appserver`)

**Purpose:** Manages OpenShift Lightspeed application server (LEGACY backend - LLM API proxy)

**Entry Point:** `ReconcileAppServer(reconciler.Reconciler, context, *OLSConfig)`

### Lightspeed Core Package (`internal/controller/lcore`)

**Purpose:** Manages Lightspeed Core + Llama Stack server (NEW backend - agent-based with MCP support)

**Entry Point:** `ReconcileLCore(reconciler.Reconciler, context, *OLSConfig)`

**Key Features:**
- Dynamic LLM configuration (supports OpenAI, Azure OpenAI, others)
- CA certificate support for custom TLS
- RAG support with vector database
- MCP (Model Context Protocol) integration
- Metrics with K8s authentication

### PostgreSQL Package (`internal/controller/postgres`)

**Purpose:** Manages PostgreSQL database for conversation cache storage

**Entry Point:** `ReconcilePostgres(reconciler.Reconciler, context, *OLSConfig)`

### Console UI Package (`internal/controller/console`)

**Purpose:** Manages OpenShift Console plugin for web UI integration

**Entry Points:** `ReconcileConsoleUI()` (setup), `RemoveConsoleUI()` (cleanup when disabled)

### Utilities Package (`internal/controller/utils`)

**Purpose:** Shared functionality across all components

**Contains:**
- Constants (resource names, labels, annotations, error messages)
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
1. Validate OLSConfig CR exists
2. Reconcile LLM Secrets (validate credentials)
3. Reconcile Components:
   - Console UI (if enabled)
   - PostgreSQL (if conversation cache enabled)
   - Backend (AppServer OR LCore - mutually exclusive, controlled by --enable-lcore flag)
4. Update Status Conditions based on deployment readiness
```

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