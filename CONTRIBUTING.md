# Contributing to OpenShift Lightspeed Operator

This guide provides detailed instructions for contributing to the OpenShift Lightspeed Operator, with a focus on adding or modifying components.

## Table of Contents

- [Architecture Overview](#architecture-overview)
- [Adding a New Component](#adding-a-new-component)
- [Modifying an Existing Component](#modifying-an-existing-component)
- [Testing Your Changes](#testing-your-changes)
- [Code Style and Conventions](#code-style-and-conventions)

---

## Architecture Overview

The operator is designed with a **modular, component-based architecture** to simplify development and maintenance. Each major component is isolated in its own package under `internal/controller/`:

```
internal/controller/
├── reconciler/              # Interface contract
│   └── interface.go
├── appserver/              # Application server component
│   ├── reconciler.go       # Main reconciliation logic
│   ├── assets.go          # Resource generation
│   ├── deployment.go      # Deployment-specific logic
│   ├── rag.go            # RAG support
│   └── *_test.go         # Component tests
├── postgres/              # PostgreSQL component
│   ├── reconciler.go
│   ├── assets.go
│   └── *_test.go
├── console/               # Console UI component
│   ├── reconciler.go
│   ├── assets.go
│   └── *_test.go
├── utils/                 # Shared utilities
│   ├── utils.go
│   └── test_helpers.go
└── olsconfig_controller.go  # Main orchestrator
```

### Why This Structure?

1. **Isolation**: Each component can be developed and tested independently
2. **Clarity**: Component boundaries are explicit and well-defined
3. **Maintainability**: Changes to one component don't affect others
4. **Testability**: Mock the `reconciler.Reconciler` interface for unit tests
5. **Scalability**: Adding new components follows a consistent pattern

---

## Adding a New Component

Follow this step-by-step guide to add a new top-level component (e.g., a new service, database, or plugin).

### Step 1: Create the Package Structure

```bash
mkdir -p internal/controller/mycomponent
```

Create these files:
- `reconciler.go` - Main reconciliation logic
- `assets.go` - Resource generation (ConfigMaps, Secrets, Services, etc.)
- `suite_test.go` - Test suite setup
- `reconciler_test.go` - Reconciliation tests
- `assets_test.go` - Asset generation tests

### Step 2: Define the Reconciler Interface Usage

**File**: `internal/controller/mycomponent/reconciler.go`

```go
// Package mycomponent provides reconciliation logic for [describe your component].
//
// This package manages:
//   - [Resource 1] - description
//   - [Resource 2] - description
//   - [Resource 3] - description
//
// [Add more context about what this component does and why it exists]
package mycomponent

import (
    "context"
    "fmt"

    olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
    "github.com/openshift/lightspeed-operator/internal/controller/reconciler"
    "github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// ReconcileMyComponent is the main entry point for reconciling the MyComponent component.
// It orchestrates all sub-tasks required to deploy and configure the component.
func ReconcileMyComponent(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
    tasks := []utils.ReconcileTask{
        {Name: "reconcile MyComponent ConfigMap", Task: reconcileMyComponentConfigMap},
        {Name: "reconcile MyComponent Deployment", Task: reconcileMyComponentDeployment},
        {Name: "reconcile MyComponent Service", Task: reconcileMyComponentService},
        // Add more tasks as needed
    }

    for _, task := range tasks {
        r.GetLogger().Info("Running task", "task", task.Name)
        if err := task.Task(r, ctx, cr); err != nil {
            return fmt.Errorf("%s: %w", task.Name, err)
        }
    }

    return nil
}

// reconcileMyComponentConfigMap creates or updates the ConfigMap for MyComponent.
func reconcileMyComponentConfigMap(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
    cm, err := generateMyComponentConfigMap(r, cr)
    if err != nil {
        return fmt.Errorf("%s: %w", utils.ErrGenerateConfigMap, err)
    }

    found := &corev1.ConfigMap{}
    err = r.Get(ctx, client.ObjectKey{Name: cm.Name, Namespace: r.GetNamespace()}, found)
    if err != nil && errors.IsNotFound(err) {
        r.GetLogger().Info("creating ConfigMap", "name", cm.Name)
        return r.Create(ctx, cm)
    } else if err != nil {
        return fmt.Errorf("%s: %w", utils.ErrGetConfigMap, err)
    }

    // Update logic if needed
    if !reflect.DeepEqual(found.Data, cm.Data) {
        r.GetLogger().Info("updating ConfigMap", "name", cm.Name)
        found.Data = cm.Data
        return r.Update(ctx, found)
    }

    return nil
}

// Add more reconcile functions for other resources...
```

### Step 3: Implement Asset Generation

**File**: `internal/controller/mycomponent/assets.go`

```go
package mycomponent

import (
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

    olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
    "github.com/openshift/lightspeed-operator/internal/controller/reconciler"
    "github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// generateMyComponentConfigMap generates the ConfigMap for MyComponent.
func generateMyComponentConfigMap(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) {
    cm := &corev1.ConfigMap{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "mycomponent-config",
            Namespace: r.GetNamespace(),
            Labels:    utils.DefaultLabels(),
        },
        Data: map[string]string{
            "config.yaml": "# Your configuration here",
        },
    }

    // Set owner reference
    if err := controllerutil.SetControllerReference(cr, cm, r.GetScheme()); err != nil {
        return nil, err
    }

    return cm, nil
}

// Add more generate functions for Deployment, Service, etc...
```

### Step 4: Add Constants (if needed)

**File**: `internal/controller/utils/utils.go`

```go
// MyComponent constants
const (
    MyComponentDeploymentName = "mycomponent"
    MyComponentServiceName    = "mycomponent-service"
    MyComponentConfigMapName  = "mycomponent-config"
)

// MyComponent error constants
const (
    ErrGenerateMyComponentConfig = "failed to generate MyComponent config"
    ErrCreateMyComponentDeployment = "failed to create MyComponent deployment"
)
```

### Step 5: Create Test Suite

**File**: `internal/controller/mycomponent/suite_test.go`

```go
package mycomponent

import (
    "context"
    "path/filepath"
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/rest"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/envtest"

    olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
    "github.com/openshift/lightspeed-operator/internal/controller/reconciler"
    "github.com/openshift/lightspeed-operator/internal/controller/utils"
)

var (
    cfg                     *rest.Config
    k8sClient               client.Client
    testEnv                 *envtest.Environment
    testReconcilerInstance  reconciler.Reconciler
)

// testReconciler implements the reconciler.Reconciler interface for testing
type testReconciler struct {
    client.Client
    scheme *runtime.Scheme
    logger logr.Logger
    cache  map[string]string
}

func (r *testReconciler) GetScheme() *runtime.Scheme          { return r.scheme }
func (r *testReconciler) GetLogger() logr.Logger              { return r.logger }
func (r *testReconciler) GetStateCache() map[string]string    { return r.cache }
func (r *testReconciler) GetNamespace() string                { return utils.OLSNamespaceDefault }
func (r *testReconciler) GetPostgresImage() string            { return "postgres:latest" }
func (r *testReconciler) GetConsoleUIImage() string           { return "console:latest" }
func (r *testReconciler) GetOpenshiftMinor() string           { return "4.18" }
func (r *testReconciler) GetAppServerImage() string           { return "app:latest" }
func (r *testReconciler) GetOpenShiftMCPServerImage() string  { return "mcp:latest" }

func TestMyComponent(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "MyComponent Suite")
}

var _ = BeforeSuite(func() {
    By("bootstrapping test environment")
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{
            filepath.Join("..", "..", "..", "config", "crd", "bases"),
            filepath.Join("..", "..", "..", ".testcrds"),
        },
        ErrorIfCRDPathMissing: true,
    }

    var err error
    cfg, err = testEnv.Start()
    Expect(err).NotTo(HaveOccurred())
    Expect(cfg).NotTo(BeNil())

    scheme := runtime.NewScheme()
    err = corev1.AddToScheme(scheme)
    Expect(err).NotTo(HaveOccurred())
    err = olsv1alpha1.AddToScheme(scheme)
    Expect(err).NotTo(HaveOccurred())

    k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
    Expect(err).NotTo(HaveOccurred())
    Expect(k8sClient).NotTo(BeNil())

    // Create test namespace
    ns := &corev1.Namespace{
        ObjectMeta: metav1.ObjectMeta{Name: utils.OLSNamespaceDefault},
    }
    err = k8sClient.Create(context.Background(), ns)
    Expect(err).NotTo(HaveOccurred())

    testReconcilerInstance = &testReconciler{
        Client: k8sClient,
        scheme: scheme,
        logger: logf.Log.WithName("test.mycomponent"),
        cache:  make(map[string]string),
    }
})

var _ = AfterSuite(func() {
    By("tearing down the test environment")
    err := testEnv.Stop()
    Expect(err).NotTo(HaveOccurred())
})
```

### Step 6: Add Tests

**File**: `internal/controller/mycomponent/reconciler_test.go`

```go
package mycomponent

import (
    "context"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    corev1 "k8s.io/api/core/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"

    "github.com/openshift/lightspeed-operator/internal/controller/utils"
)

var _ = Describe("MyComponent Reconciler", func() {
    var ctx context.Context

    BeforeEach(func() {
        ctx = context.Background()
    })

    Context("ReconcileMyComponent", func() {
        It("should successfully reconcile MyComponent resources", func() {
            cr := utils.GetDefaultOLSConfigCR()

            err := ReconcileMyComponent(testReconcilerInstance, ctx, cr)
            Expect(err).NotTo(HaveOccurred())

            // Verify ConfigMap was created
            cm := &corev1.ConfigMap{}
            err = testReconcilerInstance.Get(ctx, client.ObjectKey{
                Name:      "mycomponent-config",
                Namespace: utils.OLSNamespaceDefault,
            }, cm)
            Expect(err).NotTo(HaveOccurred())
            Expect(cm.Data).NotTo(BeEmpty())
        })
    })
})
```

### Step 7: Integrate with Main Controller

**File**: `internal/controller/olsconfig_controller.go`

Add your component to the reconciliation steps:

```go
import (
    // ... existing imports ...
    "github.com/openshift/lightspeed-operator/internal/controller/mycomponent"
)

func (r *OLSConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // ... existing code ...

    reconcileSteps := []utils.ReconcileSteps{
        // ... existing steps ...
        {
            Name:         "mycomponent",
            Fn: func(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
                return mycomponent.ReconcileMyComponent(r, ctx, cr)
            },
            ConditionType: utils.TypeMyComponentReady,  // Add this constant to utils
            Deployment:    utils.MyComponentDeploymentName,
        },
    }

    // ... rest of reconciliation logic ...
}
```

### Step 8: Update Interface (if needed)

If your component needs specific configuration from the main controller:

**File**: `internal/controller/reconciler/interface.go`

```go
type Reconciler interface {
    // ... existing methods ...
    
    // GetMyComponentImage returns the MyComponent image to use
    GetMyComponentImage() string
}
```

**File**: `internal/controller/olsconfig_controller.go`

```go
func (r *OLSConfigReconciler) GetMyComponentImage() string {
    return r.Options.MyComponentImage
}
```

### Step 9: Run Tests

```bash
# Run unit tests for your component
go test ./internal/controller/mycomponent/... -v

# Run all tests
make test

# Check coverage
go test ./internal/controller/mycomponent/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Step 10: Update Documentation

1. Update `ARCHITECTURE.md` with your component's description
2. Update `AGENTS.md` with file locations and patterns
3. Add package documentation to your `reconciler.go` file

---

## Modifying an Existing Component

When modifying an existing component, follow these guidelines:

### Adding a New Resource to a Component

**Example**: Adding a ServiceMonitor to the appserver component

1. **Add resource generation function** in `assets.go`:

```go
func generateServiceMonitor(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*monv1.ServiceMonitor, error) {
    sm := &monv1.ServiceMonitor{
        ObjectMeta: metav1.ObjectMeta{
            Name:      utils.AppServerServiceMonitorName,
            Namespace: r.GetNamespace(),
            Labels:    utils.DefaultLabels(),
        },
        Spec: monv1.ServiceMonitorSpec{
            // ... spec details ...
        },
    }
    
    if err := controllerutil.SetControllerReference(cr, sm, r.GetScheme()); err != nil {
        return nil, err
    }
    
    return sm, nil
}
```

2. **Add reconciliation function** in `reconciler.go`:

```go
func reconcileServiceMonitor(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
    sm, err := generateServiceMonitor(r, cr)
    if err != nil {
        return fmt.Errorf("%s: %w", utils.ErrGenerateServiceMonitor, err)
    }
    
    found := &monv1.ServiceMonitor{}
    err = r.Get(ctx, client.ObjectKey{Name: sm.Name, Namespace: r.GetNamespace()}, found)
    if err != nil && errors.IsNotFound(err) {
        r.GetLogger().Info("creating ServiceMonitor", "name", sm.Name)
        return r.Create(ctx, sm)
    } else if err != nil {
        return fmt.Errorf("%s: %w", utils.ErrGetServiceMonitor, err)
    }
    
    // Update if needed
    return nil
}
```

3. **Add to task list** in `ReconcileAppServer()`:

```go
tasks := []utils.ReconcileTask{
    // ... existing tasks ...
    {Name: "reconcile ServiceMonitor", Task: reconcileServiceMonitor},
}
```

4. **Add constants** in `utils/utils.go`:

```go
const (
    AppServerServiceMonitorName = "appserver-metrics"
    ErrGenerateServiceMonitor   = "failed to generate ServiceMonitor"
    ErrGetServiceMonitor        = "failed to get ServiceMonitor"
)
```

5. **Add tests** in `assets_test.go` and `reconciler_test.go`

### Modifying Resource Generation

When changing how a resource is generated:

1. **Update the generate function** in `assets.go`
2. **Add/update tests** to verify the new behavior
3. **Consider hash-based updates** if the change should trigger pod restarts
4. **Document the change** in comments and commit messages

### Changing Reconciliation Logic

When modifying reconciliation flow:

1. **Update the reconcile function** in `reconciler.go`
2. **Ensure error handling is consistent** with existing patterns
3. **Update or add tests** for new code paths
4. **Verify idempotency** - reconciliation should be safe to run multiple times

---

## Testing Your Changes

### Unit Tests

```bash
# Test specific component
go test ./internal/controller/mycomponent/... -v

# Test with coverage
go test ./internal/controller/mycomponent/... -coverprofile=coverage.out

# View coverage report
go tool cover -html=coverage.out
```

### Integration Tests

```bash
# Run all unit tests
make test

# Check linting
make lint

# Fix lint issues
make lint-fix
```

### E2E Tests

```bash
# Requires running OpenShift cluster
export KUBECONFIG=/path/to/kubeconfig
export LLM_TOKEN=your-token

make test-e2e
```

### Manual Testing

1. Build and deploy your changes:
```bash
make docker-build
make deploy
```

2. Create or update an OLSConfig CR:
```bash
oc apply -f config/samples/ols_v1alpha1_olsconfig.yaml
```

3. Check operator logs:
```bash
oc logs -n openshift-lightspeed deployment/lightspeed-operator-controller-manager -f
```

4. Verify resources:
```bash
oc get all -n openshift-lightspeed
oc get olsconfig cluster -o yaml
```

---

## Code Style and Conventions

### Naming Conventions

- **Functions**: `reconcile<Resource>`, `generate<Resource>`
- **Constants**: `<Component><Resource>Name`, `Err<Action><Resource>`
- **Files**: `reconciler.go`, `assets.go`, `<feature>.go`
- **Tests**: `reconciler_test.go`, `assets_test.go`, `<feature>_test.go`

### Error Handling

Always wrap errors with context:

```go
if err != nil {
    return fmt.Errorf("%s: %w", utils.ErrConstant, err)
}
```

### Logging

Use structured logging:

```go
r.GetLogger().Info("action description", "key", value, "key2", value2)
r.GetLogger().Error(err, "error description", "key", value)
```

### Owner References

Always set controller references for resources:

```go
if err := controllerutil.SetControllerReference(cr, resource, r.GetScheme()); err != nil {
    return nil, err
}
```

### Testing Patterns

Follow the Arrange-Act-Assert pattern:

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

### Documentation

- **Package docs**: Every package should have a doc comment explaining its purpose
- **Function docs**: Public functions should have doc comments
- **Complex logic**: Add inline comments explaining non-obvious behavior

---

## Additional Resources

- [Operator SDK Documentation](https://sdk.operatorframework.io/)
- [Kubebuilder Book](https://book.kubebuilder.io/)
- [Ginkgo Testing Framework](https://onsi.github.io/ginkgo/)
- [Architecture Documentation](./ARCHITECTURE.md)
- [Development Guidelines](./AGENTS.md)

---

## Getting Help

- Check existing components (appserver, postgres, console) as reference implementations
- Review test files for examples of testing patterns
- Ask questions in pull requests or issues

## Submitting Your Changes

1. Run all tests: `make test`
2. Check linting: `make lint`
3. Update documentation if needed
4. Create a pull request with clear description
5. Ensure CI passes

---

**Thank you for contributing to OpenShift Lightspeed Operator!** 🚀

