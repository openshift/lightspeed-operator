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
- `internal/controller/olsconfig_controller.go` - Main reconciler
- `internal/controller/ols_app_server_reconciliator.go` - App server components (12 tasks)
- `internal/controller/constants.go` - All constant definitions
- `internal/controller/utils.go` - Utility functions

### API & Types
- `api/v1alpha1/olsconfig_types.go` - Main CRD struct definitions
- Includes: `LLMSpec`, `OLSSpec`, `DeploymentConfig`, etc.

### Tests to Check
- Unit: `internal/controller/*_test.go`
- E2E: `test/e2e/reconciliation_test.go`, `test/e2e/upgrade_test.go`

## Common Tasks & Patterns

### Adding New Reconciliation Step
1. Add to `ReconcileTask` slice in `reconcileAppServer()`
2. Implement `reconcile<NewComponent>()` method
3. Add constants to `constants.go`
4. Add error constants with `Err<Action><Component>` pattern
5. Write unit tests in `*_test.go`

### Resource Generation Pattern
```go
func (r *OLSConfigReconciler) reconcile<Resource>(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
    resource, err := r.generate<Resource>(cr)
    if err != nil {
        return fmt.Errorf("%s: %w", Err<Action><Resource>, err)
    }

    found := &<ResourceType>{}
    err = r.Get(ctx, client.ObjectKey{Name: <name>, Namespace: r.Options.Namespace}, found)
    if err != nil && errors.IsNotFound(err) {
        r.logger.Info("creating new <resource>", "<resource>", resource.Name)
        return r.Create(ctx, resource)
    } else if err != nil {
        return fmt.Errorf("%s: %w", ErrGet<Resource>, err)
    }

    // Update logic if needed
    return nil
}
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