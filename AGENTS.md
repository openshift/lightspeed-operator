# OpenShift Lightspeed Operator - AI Assistant Guide

## Project Overview
Kubernetes operator managing OpenShift Lightspeed (AI-powered Virtual Assistant). Go + controller-runtime/kubebuilder + Ginkgo v2/Gomega testing.

## Version Management

### Version Update Process
When updating the operator version for a release, you **MUST** update version numbers in **TWO** files:

1. **`bundle.Dockerfile`** - Bundle container labels (lines 63 and 66)
   ```dockerfile
   LABEL release=X.Y.Z
   LABEL version=X.Y.Z
   ```

2. **`bundle/manifests/lightspeed-operator.clusterserviceversion.yaml`** - CSV metadata (lines 58 and 715)
   ```yaml
   name: lightspeed-operator.vX.Y.Z
   # ... line 715:
   version: X.Y.Z
   ```

**Important Notes:**
- Both files MUST have matching versions
- The CSV `name` field includes a `v` prefix (e.g., `lightspeed-operator.v1.0.8`)
- The CSV `version` field does NOT have a prefix (e.g., `1.0.8`)
- After version changes, regenerate bundle using `make bundle` or `hack/update_bundle.sh -v X.Y.Z`

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

## Maintaining This Document

Always suggest AGENTS.md edits when architectural, structural, or conventional changes are made to the codebase.
