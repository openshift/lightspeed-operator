# OpenShift Lightspeed Operator - AI Assistant Guide

## Project Overview
Kubernetes operator managing OpenShift Lightspeed (AI-powered Virtual Assistant). Go + controller-runtime/kubebuilder + Ginkgo v2/Gomega testing.

## Architecture Quick Reference

### Core Components
- **API**: `api/v1alpha1/` - CRD definitions (`OLSConfig`)
- **Controllers**: `internal/controller/` - Reconciliation logic
- **Entry Point**: `cmd/main.go` - Operator entry with watcher configuration
- **Key CRD**: `OLSConfig` - cluster-scoped, single instance per cluster

### Reconciliation Flow
```
OLSConfigReconciler.Reconcile() →
├── reconcileLLMSecrets()
├── reconcileConsoleUI()
├── reconcilePostgresServer()
└── reconcileAppServer() OR reconcileLCore()  [MUTUALLY EXCLUSIVE via --enable-lcore flag]
    └── [12+ sub-tasks via ReconcileTask pattern]
```

## Code Conventions

### Naming Patterns
- **Constants**: `const OLSConfigCmName = "olsconfig"`
- **Error Constants**: `const ErrCreateAPIConfigmap = "failed to create API configmap"`
- **Functions**: `reconcile<ComponentName>()`, `generate<ResourceType>()`
- **File Names**: `reconciler.go`, `assets.go`, `suite_test.go`

### Error Handling
Wrap with `fmt.Errorf("%s: %w", ErrConstant, err)`

## Testing - CRITICAL

NEVER use `go test` directly - ALWAYS use `make test`

The Makefile handles essential setup (envtest, CRDs, build flags) that `go test` doesn't.

```bash
make test       # Unit tests
make test-e2e   # E2E tests (requires cluster)
```

## Key File Locations

### Controllers
- `internal/controller/olsconfig_controller.go` - Main reconciler
- `internal/controller/appserver/` - App server (LEGACY)
- `internal/controller/lcore/` - Lightspeed Core (NEW)
- `internal/controller/postgres/` - PostgreSQL
- `internal/controller/console/` - Console UI
- `internal/controller/watchers/` - External resource watching
- `internal/controller/utils/` - Shared utilities, constants

### Tests
- `*_test.go` - Unit tests (co-located)
- `test/e2e/` - E2E tests
- `internal/controller/utils/testing.go` - Test utilities

## State Management

**Owned Resources**: Tracked automatically by controller-runtime via ResourceVersion

**External Resources**: Three-layer watcher system (predicate filtering, data comparison, restart logic). See `internal/controller/watchers/` and `cmd/main.go`.

## Common Development Tasks

### Adding New Reconciliation Step
- **App Server**: Add to `ReconcileTask` slice in `internal/controller/appserver/reconciler.go`
- **Top-Level**: Create package under `internal/controller/<component>/`, add to `olsconfig_controller.go`
- Add error constants to `internal/controller/utils/utils.go`
- Write unit tests in co-located `*_test.go` files
