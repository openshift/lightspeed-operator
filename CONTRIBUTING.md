# Contributing to OpenShift Lightspeed Operator

This guide provides instructions for contributing to the OpenShift Lightspeed Operator.

> **For architecture details**, see [ARCHITECTURE.md](ARCHITECTURE.md)  
> **For coding conventions**, see [AGENTS.md](AGENTS.md)

---

## Quick Context

The operator uses a **modular, component-based architecture**:
- Each component (appserver, lcore, postgres, console) is a self-contained package
- Components use the `reconciler.Reconciler` interface (no circular dependencies)
- Task-based reconciliation pattern (list of tasks executed sequentially)
- Two resource approaches: **Owned** (operator-created, ResourceVersion tracking) vs **External** (user-provided, data comparison)

**Best way to learn**: Read existing component code (`appserver/`, `postgres/`, `console/`, `lcore/`)

---

## Adding a New Component

Follow these steps to add a new component. Use existing components as reference implementations.

### Step 1: Create Package Structure

```bash
mkdir -p internal/controller/mycomponent
cd internal/controller/mycomponent
touch reconciler.go assets.go suite_test.go reconciler_test.go assets_test.go
```

### Step 2: Implement Reconciler

**File**: `reconciler.go`

**Structure**:
- Package doc comment explaining what the component does
- Main function: `ReconcileMyComponent(reconciler.Reconciler, context, *OLSConfig) error`
- Task-based pattern: list of `ReconcileTask` structs
- Individual `reconcile<Resource>` functions for each Kubernetes resource

**Pattern**:
```go
func ReconcileMyComponent(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
    tasks := []utils.ReconcileTask{
        {Name: "reconcile ConfigMap", Task: reconcileConfigMap},
        {Name: "reconcile Deployment", Task: reconcileDeployment},
        {Name: "reconcile Service", Task: reconcileService},
    }
    for _, task := range tasks {
        if err := task.Task(r, ctx, cr); err != nil {
            return fmt.Errorf("%s: %w", task.Name, err)
        }
    }
    return nil
}
```

**Reference**: See `appserver/reconciler.go`, `postgres/reconciler.go`, or `lcore/reconciler.go`

### Step 3: Implement Asset Generation

**File**: `assets.go`

**Structure**:
- One `generate<Resource>()` function per Kubernetes resource
- Each function returns the resource and error
- Always set owner references with `controllerutil.SetControllerReference()`
- Use `utils.DefaultLabels()` for consistent labeling

**Reference**: See `appserver/assets.go`, `postgres/assets.go`, or `lcore/assets.go`

### Step 4: Add Constants

**File**: `internal/controller/utils/constants.go`

Add:
- Resource names: `MyComponentDeploymentName`, `MyComponentServiceName`, etc.
- Error constants: `ErrGenerateMyComponent`, `ErrCreateMyComponent`, etc.

**Reference**: See existing constants in `utils/constants.go`

### Step 5: Create Test Suite

**File**: `suite_test.go`

**Setup**:
- Use `envtest` for test environment
- Create test namespace
- Use `utils.NewTestReconciler()` helper
- BeforeSuite/AfterSuite for setup/teardown

**Reference**: Copy structure from `appserver/suite_test.go` or `postgres/suite_test.go`

### Step 6: Write Tests

**Files**: `reconciler_test.go`, `assets_test.go`

**Pattern**:
- Use Ginkgo/Gomega BDD style
- Test resource generation (assets_test.go)
- Test reconciliation (reconciler_test.go)
- Use `utils.GetDefaultOLSConfigCR()` for test fixtures

**Reference**: See `appserver/reconciler_test.go`, `postgres/assets_test.go`

### Step 7: Integrate with Main Controller

**File**: `internal/controller/olsconfig_controller.go`

1. Import your component package
2. Add to reconciliation steps in `Reconcile()` method
3. Add status condition type to `utils/constants.go`

**Reference**: See how `appserver`, `postgres`, `console` are integrated

### Step 8: Update Reconciler Interface (if needed)

**File**: `internal/controller/reconciler/interface.go`

Add getter methods if your component needs specific configuration (e.g., `GetMyComponentImage()`).

Implement in `internal/controller/olsconfig_controller.go`.

### Step 9: Run Tests

```bash
make test  # ALWAYS use make test, NEVER go test directly
```

### Step 10: Update OLM Bundle (if needed)

The OLM bundle needs regeneration when changes affect how the operator is deployed or what permissions it needs.

#### When to Update the Bundle

**Bundle Update Required:**
- **RBAC Changes**: Modified `//+kubebuilder:rbac` markers in Go code OR changed files in `config/rbac/`
- **CRD Changes**: Modified API types in `api/v1alpha1/olsconfig_types.go`
- **Image Changes**: New operator image, new operand images (appserver, lcore, postgres, console), or image version changes
- **CSV Metadata**: Changed operator description, keywords, maintainers, links, or other metadata

**Bundle Update NOT Required:**
- Reconciliation logic changes (internal/controller/)
- Test changes (*_test.go files)
- Documentation updates (*.md files)
- Internal utilities (utils/ package)
- Bug fixes that don't affect RBAC, CRD, or images

#### How to Update the Bundle

**1. For RBAC or CRD Changes:**
```bash
# Regenerate manifests (updates config/rbac/ and config/crd/)
make manifests

# Regenerate bundle (updates bundle/ directory)
make bundle

# Validate bundle
operator-sdk bundle validate ./bundle
```

**2. For Image Changes:**
```bash
# Update related_images.json with new image references
# (This file defines all images used by the operator and operands)

# Regenerate bundle with image updates
make bundle RELATED_IMAGES_FILE=related_images.json

# Validate bundle
operator-sdk bundle validate ./bundle
```

**3. For CSV Metadata Changes:**
```bash
# Edit bundle/manifests/lightspeed-operator.clusterserviceversion.yaml directly
# OR edit the CSV template if one exists

# Regenerate bundle to sync changes
make bundle

# Validate bundle
operator-sdk bundle validate ./bundle
```

#### What Gets Updated

When you run `make bundle`, it updates:
- `bundle/manifests/lightspeed-operator.clusterserviceversion.yaml` - Main operator metadata
- `bundle/manifests/ols.openshift.io_olsconfigs.yaml` - CRD definition
- `bundle/manifests/*_rbac.authorization.k8s.io_*.yaml` - RBAC resources
- `bundle/metadata/annotations.yaml` - Bundle metadata

#### Testing Bundle Changes

```bash
# Validate bundle structure and metadata
operator-sdk bundle validate ./bundle

# Test deployment with updated bundle (optional)
make deploy    # Deploys using manifests
# Test functionality
make undeploy
```

#### Common Issues

- **"invalid bundle" errors**: Run `make manifests` before `make bundle`
- **Missing RBAC permissions**: Ensure `//+kubebuilder:rbac` markers are in reconciler code
- **Image references not updated**: Check `related_images.json` has correct image URLs
- **CSV validation fails**: Check CSV syntax with `operator-sdk bundle validate`

> 📖 For detailed bundle workflows, version management, and catalog publishing, see [docs/olm-bundle-management.md](./docs/olm-bundle-management.md)

### Step 11: Update Documentation

Update [ARCHITECTURE.md](ARCHITECTURE.md) Component Overview section with brief description of your component.

---

## Modifying an Existing Component

### Adding a Resource to a Component

1. **Add generation function** in `assets.go`: `generate<Resource>()`
2. **Add reconciliation function** in `reconciler.go`: `reconcile<Resource>()`
3. **Add to task list** in main `Reconcile<Component>()` function
4. **Add constants** in `utils/constants.go`
5. **Add tests** in `assets_test.go` and `reconciler_test.go`

**Reference**: Search existing code for similar resources (e.g., `generateConfigMap` or `reconcileService`)

### Working with External Resources

**User-provided secrets/configmaps** (not owned by operator):

Use iterator helpers:
```go
err := utils.ForEachExternalSecret(cr, func(name string, source string) error {
    // Process secret
    return nil
})

err = utils.ForEachExternalConfigMap(cr, func(name string, source string) error {
    // Process configmap
    return nil
})
```

**Benefits**: Centralizes traversal, handles annotation, prevents duplication

### Updating Watchers

If your component needs to watch system resources (not from CR), update `cmd/main.go`:

```go
watcherConfig := &utils.WatcherConfig{
    Secrets: utils.SecretWatcherConfig{
        SystemResources: []utils.SystemSecret{
            {
                Name:                "my-system-secret",
                Namespace:           namespace,
                Description:         "Description for debugging",
                AffectedDeployments: []string{"ACTIVE_BACKEND"},
            },
        },
    },
}
```

**Reference**: See existing `WatcherConfig` in `cmd/main.go`

> 📖 See [ARCHITECTURE.md](ARCHITECTURE.md#resource-management) for owned vs external resources

---

## Testing Your Changes

### Unit Tests

CRITICAL: ALWAYS use `make test` - NEVER use `go test` directly!

The Makefile handles essential setup (envtest, CRDs, build flags, coverage) that `go test` doesn't.

```bash
make test                    # Run all unit tests
go tool cover -html=cover.out  # View coverage report
```

### Linting

```bash
make lint       # Check linting
make lint-fix   # Fix lint issues
```

### E2E Tests

```bash
export KUBECONFIG=/path/to/kubeconfig
export LLM_TOKEN=your-token
make test-e2e
```

### Local Development

```bash
make run  # Run controller locally (auto-setup RBAC, CRDs, namespace)
```

### Cluster Deployment

```bash
make docker-build
make deploy
oc apply -f config/samples/ols_v1alpha1_olsconfig.yaml
oc logs -n openshift-lightspeed deployment/lightspeed-operator-controller-manager -f
```

### Bundle Testing

If you modified RBAC, CRD, images, or CSV:

```bash
operator-sdk bundle validate ./bundle
```

> 📖 For comprehensive OLM testing, see [docs/olm-testing-validation.md](./docs/olm-testing-validation.md)

---

## Code Style and Conventions

### Naming
- **Functions**: `reconcile<Resource>`, `generate<Resource>`
- **Constants**: `<Component><Resource>Name`, `Err<Action><Resource>`
- **Files**: `reconciler.go`, `assets.go`, `suite_test.go`
- **Tests**: `reconciler_test.go`, `assets_test.go`

### Error Handling
```go
return fmt.Errorf("%s: %w", utils.ErrConstant, err)
```

### Logging
```go
r.GetLogger().Info("action", "key", value)
r.GetLogger().Error(err, "description", "key", value)
```

### Owner References
```go
controllerutil.SetControllerReference(cr, resource, r.GetScheme())
```

### Testing Pattern
```go
It("should do something", func() {
    // Arrange
    cr := utils.GetDefaultOLSConfigCR()
    // Act
    err := ReconcileMyComponent(testReconcilerInstance, ctx, cr)
    // Assert
    Expect(err).NotTo(HaveOccurred())
})
```

### Code Reuse

Before implementing new functionality:
- Check `utils/` for existing helpers
- Use iterator patterns: `ForEachExternalSecret()`, `ForEachExternalConfigMap()`
- Use constants: `VolumeDefaultMode`, `VolumeRestrictedMode`
- Use helpers: `GetResourcesOrDefault()`

### Documentation
- Package docs explaining purpose
- Function docs for public functions
- Inline comments for complex logic

**Reference**: Read existing component code for patterns

---

## OLM Documentation

For operators deployed via OLM, see comprehensive guides in `docs/`:
- [OLM Bundle Management](./docs/olm-bundle-management.md)
- [OLM Catalog Management](./docs/olm-catalog-management.md)
- [OLM Integration & Lifecycle](./docs/olm-integration-lifecycle.md)
- [OLM Testing & Validation](./docs/olm-testing-validation.md)
- [OLM RBAC & Security](./docs/olm-rbac-security.md)

---

## Additional Resources

- [ARCHITECTURE.md](./ARCHITECTURE.md) - Architecture and design decisions
- [AGENTS.md](./AGENTS.md) - Coding conventions and patterns
- [Operator SDK Documentation](https://sdk.operatorframework.io/)
- [Kubebuilder Book](https://book.kubebuilder.io/)
- [Ginkgo Testing Framework](https://onsi.github.io/ginkgo/)

---

## Getting Help

- Check existing components as reference implementations
- Review test files for testing patterns
- Ask questions in pull requests or issues

---

## Submitting Your Changes

1. Run tests: `make test`
2. Check linting: `make lint`
3. Update documentation if needed:
   - **ARCHITECTURE.md**: If you changed component structure or design decisions
   - **AGENTS.md**: If you changed architectural patterns, conventions, or critical rules (e.g., new naming patterns, error handling approaches, testing requirements)
   - **CONTRIBUTING.md**: If you changed development workflows or added new processes
4. Create pull request with clear description
5. Ensure CI passes

---

**Thank you for contributing to OpenShift Lightspeed Operator!** 🚀
