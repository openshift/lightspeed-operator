# OpenShift Lightspeed Operator - Claude AI Assistant Guide

## Project Overview
**lightspeed-operator** is a Kubernetes operator managing OpenShift Lightspeed (AI-powered OpenShift Virtual Assistant). Written in Go using controller-runtime/kubebuilder patterns.

## Architecture Quick Reference

### Core Components
- **API**: `api/v1alpha1/` - CRD definitions (primarily `OLSConfig`)
- **Controllers**: `internal/controller/` - Reconciliation logic
- **Entry Point**: Main reconciler in `olsconfig_controller.go`
- **Key CRD**: `OLSConfig` - cluster-scoped, single instance per cluster

### Reconciliation Pattern
```
OLSConfigReconciler.Reconcile() →
├── reconcileLLMSecrets()
├── reconcileConsoleUI()
├── reconcilePostgresServer()
└── reconcileAppServer() → [12 sub-tasks via ReconcileTask pattern]
```

## Code Conventions

### Go Style
- **Package**: `controller` (main logic), `v1alpha1` (API types)
- **Receivers**: `(r *OLSConfigReconciler)` pattern
- **Error Handling**: Wrap with `fmt.Errorf("%s: %w", ErrConstant, err)`
- **Logging**: `r.logger.Info/Error()` with structured fields
- **Imports**: Grouped (stdlib, k8s, third-party, internal)

### Naming Patterns
- **Constants**: `const OLSConfigCmName = "olsconfig"`
- **Error Constants**: `const ErrCreateAPIConfigmap = "failed to create API configmap"`
- **Functions**: `reconcile<ComponentName>()`, `generate<ResourceType>()`
- **File Names**: `ols_<component>_<type>.go` (e.g., `ols_app_server_reconciliator.go`)

### Resource Management
- **Pattern**: Get → Check if exists → Create/Update with error wrapping
- **Caching**: Uses `r.stateCache` for hash-based change detection
- **Annotations**: Extensive use for watching/change detection

## Testing Conventions

### Framework
- **Unit Tests**: Ginkgo v2 + Gomega (BDD-style)
- **E2E Tests**: `test/e2e/` - Real cluster testing
- **Suite Pattern**: `suite_test.go` for test setup

### Test Structure
```go
var _ = Describe("Component Name", func() {
    It("should describe expected behavior", func() {
        // Arrange
        // Act
        // Assert with Gomega matchers
        Expect(result).To(BeTrue())
    })
})
```

### Test Categories
- **Unit**: `internal/controller/*_test.go`
- **E2E**: `test/e2e/*_test.go` (requires cluster)
- **Asset Tests**: `*_assets_test.go` (resource generation)

## Key Files & Their Purpose

### Critical Controller Files
- `internal/controller/olsconfig_controller.go` - Main reconciler orchestrator
- `internal/controller/appserver/reconciler.go` - App server components
- `internal/controller/postgres/reconciler.go` - PostgreSQL database components
- `internal/controller/console/reconciler.go` - Console UI plugin components
- `internal/controller/utils/utils.go` - Shared utilities and constants

### API & Types
- `api/v1alpha1/olsconfig_types.go` - Main CRD struct definitions
- Includes: `LLMSpec`, `OLSSpec`, `DeploymentConfig`, etc.

### Tests to Check
- **Unit Tests** (co-located with source):
  - `internal/controller/*_test.go` - Main controller tests
  - `internal/controller/appserver/*_test.go` - App server component tests
  - `internal/controller/postgres/*_test.go` - PostgreSQL component tests
  - `internal/controller/console/*_test.go` - Console UI component tests
  - `internal/controller/utils/*_test.go` - Utility function tests
- **E2E Tests**: `test/e2e/reconciliation_test.go`, `test/e2e/upgrade_test.go`
- **Test Helpers**: `internal/controller/utils/test_helpers.go`

## Common Tasks & Patterns

### Adding New Reconciliation Step

**For App Server Components:**
1. Add to `ReconcileTask` slice in `internal/controller/appserver/reconciler.go`
2. Implement `reconcile<NewComponent>()` function in appropriate file
3. Add constants to `internal/controller/utils/utils.go`
4. Add error constants with `Err<Action><Component>` pattern
5. Write unit tests in `internal/controller/appserver/*_test.go`

**For New Top-Level Components:**
1. Create new package under `internal/controller/<component>/`
2. Implement `Reconcile<Component>()` function accepting `reconciler.Reconciler`
3. Add reconciliation step to `olsconfig_controller.go`
4. Create test suite with `suite_test.go` and component tests
5. Use shared test helpers from `utils/test_helpers.go`

### Resource Generation Pattern
```go
func reconcile<Resource>(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
    resource, err := generate<Resource>(r, cr)
    if err != nil {
        return fmt.Errorf("%s: %w", utils.Err<Action><Resource>, err)
    }

    found := &<ResourceType>{}
    err = r.Get(ctx, client.ObjectKey{Name: <name>, Namespace: r.GetNamespace()}, found)
    if err != nil && errors.IsNotFound(err) {
        r.GetLogger().Info("creating new <resource>", "<resource>", resource.Name)
        return r.Create(ctx, resource)
    } else if err != nil {
        return fmt.Errorf("%s: %w", utils.ErrGet<Resource>, err)
    }

    // Update logic if needed
    return nil
}
```

### Testing Pattern
```go
// In suite_test.go
var _ = Describe("<Component> Name", func() {
    It("should describe expected behavior", func() {
        // Arrange - Create test resources
        cr := utils.GetDefaultOLSConfigCR()
        
        // Act - Call reconciliation function
        err := Reconcile<Component>(testReconcilerInstance, ctx, cr)
        
        // Assert with Gomega matchers
        Expect(err).NotTo(HaveOccurred())
        
        // Verify resource was created
        resource := &<ResourceType>{}
        err = testReconcilerInstance.Get(ctx, client.ObjectKey{...}, resource)
        Expect(err).NotTo(HaveOccurred())
        Expect(resource.Spec.Field).To(Equal(expectedValue))
    })
})
```

## Dependencies & Tools

### Core Dependencies
- `controller-runtime` - Operator framework
- `k8s.io/api` - Kubernetes API types
- `github.com/openshift/client-go` - OpenShift API extensions
- `github.com/prometheus-operator/prometheus-operator` - Monitoring

### Testing
- `github.com/onsi/ginkgo/v2` - BDD test framework
- `github.com/onsi/gomega` - Matcher library

### Build Tools
- **Makefile**: Standard operator-sdk generated targets
- **Go Version**: 1.24.0
- **Base Image**: `registry.redhat.io/ubi9/ubi-minimal`

## Configuration Management

### Key Environment Variables
- `CONTROLLER_NAMESPACE` - Operator namespace
- Various timeout configurations for E2E tests

### Secret Management
- LLM provider credentials in secrets (key: `apitoken`)
- TLS certificates for secure communications
- Supports multiple providers: OpenAI, Azure OpenAI, WatsonX, RHELAI, RHOAI

## State Management

### Hash-Based Change Detection
```go
r.stateCache[OLSConfigHashStateCacheKey] = configHash
r.stateCache[LLMProviderHashStateCacheKey] = providerHash
```

### Reconciliation Triggers
- Config changes (detected via hash comparison)
- Secret updates (via annotations + watchers)
- Resource deletions/modifications

## Token-Efficient Debugging Tips

### Quick Status Check
```bash
oc get olsconfig cluster -o yaml  # Check main CR status
oc get pods -n openshift-lightspeed  # Check running components
```

### Common Issues
- **Secret not found**: Check provider credentials in secrets
- **Deployment not ready**: Check resource limits/requests
- **TLS issues**: Verify certificate secrets and CA configs

### Log Analysis
- Controller logs: Look for reconciliation errors
- App server logs: Check LLM provider connectivity
- E2E test patterns: Check `test/e2e/` for real-world scenarios

## Development Workflow

### Local Development
```bash
make test              # Run unit tests
make e2e-test         # Run E2E tests (requires cluster)
make docker-build     # Build operator image
make deploy           # Deploy to cluster
```

### Code Changes
1. Modify controller logic in `internal/controller/`
2. Update tests (`*_test.go` files)
3. Run `make test` for validation
4. For API changes: Update `api/v1alpha1/` and regenerate manifests

This guide provides Claude assistants with essential context for efficient analysis and modification of the lightspeed-operator codebase.