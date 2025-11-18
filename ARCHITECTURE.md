# Architecture

This document describes the internal architecture of the OpenShift Lightspeed Operator codebase.

> **💡 Want to add or modify a component?** See the [Contributing Guide](CONTRIBUTING.md) for step-by-step instructions.

## Overview

The operator follows a modular, component-based architecture where each major component (application server, Lightspeed Core/Llama Stack, PostgreSQL, Console UI) is managed by its own dedicated package with independent reconciliation logic.

## Directory Structure

```
internal/controller/
├── reconciler/              # Interface definitions
│   └── interface.go         # Reconciler interface contract
├── appserver/              # Application server component
│   ├── reconciler.go       # Main reconciliation logic
│   ├── assets.go          # Resource generation (ConfigMaps, Services, etc.)
│   ├── deployment.go      # Deployment-specific logic
│   └── rag.go            # RAG (Retrieval-Augmented Generation) support
├── lcore/                 # Lightspeed Core (Llama Stack) component
│   ├── reconciler.go      # Main reconciliation logic
│   ├── assets.go         # Resource generation (Llama Stack config, LCore config)
│   └── deployment.go     # Deployment-specific logic
├── postgres/              # PostgreSQL database component
│   ├── reconciler.go     # Main reconciliation logic
│   └── assets.go        # Resource generation
├── console/              # Console UI plugin component
│   ├── reconciler.go    # Main reconciliation logic
│   └── assets.go       # Resource generation
├── utils/               # Shared utilities and constants
│   ├── utils.go        # Core utilities
│   ├── types.go        # Shared type definitions
│   └── test_helpers.go # Test helper functions
└── olsconfig_controller.go  # Main operator controller

cmd/
└── main.go              # Operator entry point and initialization
```

## Component Architecture

### Entry Point (`cmd/main.go`)

The main package is the operator's entry point that initializes and starts the controller manager.

**Key Responsibilities:**
- Parse command-line flags for operator configuration
- Set up Kubernetes schemes and API types
- Configure controller manager (metrics, health probes, leader election)
- Detect OpenShift version and select appropriate images
- Configure TLS security for metrics server
- Initialize and start the OLSConfigReconciler
- Handle graceful shutdown

**Configuration Options:**
- Image overrides for all components (service, console, postgres, MCP server, lcore)
- Namespace configuration
- Reconciliation interval
- Metrics and health probe addresses
- TLS security settings
- Leader election for HA deployments
- **Backend selection**: `--enable-lcore` flag (default: false, uses appserver)

### Main Controller (`olsconfig_controller.go`)

The main `OLSConfigReconciler` orchestrates the reconciliation of all components. It:
- Implements the `reconciler.Reconciler` interface
- Manages the OLSConfig custom resource lifecycle
- Coordinates reconciliation steps across components
- Updates status conditions
- Delegates LLM provider secret reconciliation to appserver or lcore package
- Sets up resource watchers for automatic updates
- **Selects backend**: Calls either `appserver.ReconcileAppServer()` OR `lcore.ReconcileLCore()` based on `--enable-lcore` flag
- Delegates LLM provider secret reconciliation to appserver or lcore package
- Registers resource watchers (via `watchers` package) for automatic updates

**Key Responsibilities:**
- Overall reconciliation coordination
- Status management
- Error handling and retries
- Component orchestration (calls console, postgres, watchers, and either appserver OR lcore reconcilers)

### Reconciler Interface (`internal/controller/reconciler`)

The `Reconciler` interface provides a clean contract between the main controller and component packages, enabling:
- **Dependency Injection**: Components receive only what they need
- **Testability**: Easy to mock for unit testing
- **No Circular Dependencies**: Components don't import the main controller
- **Consistent Access**: Uniform way to access Kubernetes client and configuration

```go
type Reconciler interface {
    client.Client  // Embedded Kubernetes client
    GetScheme() *runtime.Scheme
    GetLogger() logr.Logger
    GetNamespace() string
    GetPostgresImage() string
    GetConsoleUIImage() string
    GetAppServerImage() string
    GetLCoreImage() string
    // ... other configuration getters
}
```

### Watchers Package (`internal/controller/watchers`)

Manages Kubernetes resource watching and automatic reconciliation triggers.

**Main Components:**
- `SecretWatcherFilter()` - Watches Secrets with watcher annotations
- `TelemetryPullSecretWatcherFilter()` - Watches telemetry pull secret
- `ConfigMapWatcherFilter()` - Watches ConfigMaps and triggers backend restarts

**Note**: Annotation helpers (`AnnotateSecretWatcher`, `AnnotateConfigMapWatcher`) are provided by the `utils` package and used directly by component packages.

**Key Features:**
- **Backend-aware**: Caller passes `useLCore` flag to determine which backend to restart
- **Rolling restarts**: Triggers deployment restarts when watched ConfigMaps change (e.g., CA certificates)
- **Annotation-based**: Uses `ols.openshift.io/watch-olsconfig` annotation to identify watched resources
- **Special cases**: Watches OpenShift default certs ConfigMap (`kube-root-ca.crt`) automatically

**Watched Resources:**
- Secrets with watcher annotations (LLM credentials, TLS certificates)
- Telemetry pull secret (`openshift-config/pull-secret`)
- ConfigMaps with watcher annotations (CA certificates, configuration)
- OpenShift default certificates ConfigMap

**Usage in Main Controller:**
```go
Watches(&corev1.ConfigMap{}, handler.EnqueueRequestsFromMapFunc(
    func(ctx context.Context, obj client.Object) []reconcile.Request {
        return watchers.ConfigMapWatcherFilter(r, ctx, obj, r.Options.UseLCore)
    }
))
```

**Restart Logic:**
```go
// Inside ConfigMapWatcherFilter
if useLCore {
    lcore.RestartLCore(r, ctx)  // Restart LCore deployment
} else {
    appserver.RestartAppServer(r, ctx)  // Restart AppServer deployment
}
```

### Application Server Package (`internal/controller/appserver`)

Manages the OpenShift Lightspeed application server lifecycle.

**Main Components:**
- `ReconcileAppServer()` - Main entry point for reconciliation
- `GenerateOLSConfigMap()` - Creates OLS configuration
- `GenerateOLSDeployment()` - Creates application deployment
- `ReconcileLLMSecrets()` - Handles LLM provider credentials
- `ReconcileTLSSecret()` - Manages TLS certificates

**Managed Resources:**
- Deployment (app server pods)
- Service (ClusterIP for internal access)
- ServiceAccount & RBAC (cluster roles and bindings)
- ConfigMap (application configuration)
- Service Monitor (Prometheus monitoring - conditionally created if Prometheus Operator is available)
- Prometheus Rules (alerting - conditionally created if Prometheus Operator is available)
- Network Policy (security)
- Secrets (TLS certificates, metrics tokens)

### Lightspeed Core Package (`internal/controller/lcore`)

Manages the Lightspeed Core (LCS) and Llama Stack server lifecycle. This component provides AI agent capabilities and MCP (Model Context Protocol) support.

> **⚠️ Backend Selection**: LCore and AppServer are **mutually exclusive** backend implementations. Only one can be active at a time.
> - The operator uses a feature flag (`--enable-lcore`) to determine which backend to reconcile
> - **LCore (NEW)**: Agent-based architecture with Llama Stack integration, MCP support, and RAG capabilities
> - **AppServer (LEGACY)**: Traditional LLM API proxy

**Main Components:**
- `ReconcileLCore()` - Main entry point for reconciliation
- `GenerateLlamaStackConfigMap()` - Creates Llama Stack run configuration
- `GenerateLcoreConfigMap()` - Creates Lightspeed Stack application configuration
- `GenerateLCoreDeployment()` - Creates dual-container deployment (llama-stack + lightspeed-stack)

**Key Features:**
- **Dynamic LLM Configuration**: Automatically generates Llama Stack provider config from OLSConfig
- **Supported Providers**: OpenAI, Azure OpenAI (with provider-specific fields like deployment name, API version)
- **Unsupported Providers**: WatsonX, BAM, RHOAI vLLM, RHELAI vLLM (returns error with clear message)
- **CA Certificate Support**: Mounts `kube-root-ca.crt` and optional `additionalCAConfigMapRef` for custom TLS
- **RAG Support**: Vector database configuration for Retrieval-Augmented Generation
- **MCP Integration**: Model Context Protocol for agent workflows
- **Metrics & Monitoring**: Prometheus metrics with K8s authentication
- **Configuration from OLSConfig**:
  - `LogLevel`: Controls logging verbosity via `LOG_LEVEL` env var (DEBUG, INFO, WARNING, ERROR, CRITICAL)
  - `DefaultModel` & `DefaultProvider`: Default inference model and provider selection
  - `UserDataCollection.{FeedbackDisabled, TranscriptsDisabled}`: Enable/disable data collection
  - `DeploymentConfig.Replicas`: Pod replica count (default: 1)
  - `DeploymentConfig.APIContainer.Resources`: CPU/memory limits for lightspeed-stack container
  - `DeploymentConfig.APIContainer.NodeSelector`: Pod scheduling based on node labels
  - `DeploymentConfig.APIContainer.Tolerations`: Pod toleration of node taints
  - Note: llama-stack sidecar uses fixed resources (500m/512Mi requests, 1000m/1Gi limits)

**Managed Resources:**
- Deployment (dual-container: llama-stack + lightspeed-stack)
- Service (ClusterIP for LCS access on port 8443)
- ServiceAccount & RBAC (cluster roles and bindings for metrics)
- ConfigMaps (Llama Stack config, LCore config, Additional CA, Proxy CA)
- Service Monitor (Prometheus monitoring with user-workload support - conditionally created if Prometheus Operator is available)
- Network Policy (security)
- Secrets (TLS certificates, metrics tokens, LLM provider API keys)

**Architecture:**
The lcore package is **completely independent** from appserver, following the same patterns:
- Task-based reconciliation
- Interface-based dependency injection via `reconciler.Reconciler`
- ResourceVersion-based change detection
- Separate test suite with comprehensive coverage (75.7%)
- No shared code or imports between lcore and appserver (ensures clean separation)

### PostgreSQL Package (`internal/controller/postgres`)

Manages the PostgreSQL database used for conversation cache storage.

**Main Components:**
- `ReconcilePostgres()` - Main entry point
- Resource generation functions for all PostgreSQL components

**Managed Resources:**
- Deployment (PostgreSQL pods)
- Service (database access)
- PersistentVolumeClaim (data persistence)
- ConfigMap (PostgreSQL configuration)
- Secrets (database credentials, bootstrap)
- Network Policy (database security)
- CA Certificates (secure connections)

### Console UI Package (`internal/controller/console`)

Manages the OpenShift Console plugin for web UI integration.

**Main Components:**
- `ReconcileConsoleUI()` - Main entry point for setup
- `RemoveConsoleUI()` - Cleanup when disabled
- Console plugin integration logic

**Managed Resources:**
- ConsolePlugin CR (OpenShift console integration)
- Deployment (UI plugin pods)
- Service (plugin serving)
- ConfigMap (Nginx configuration)
- Network Policy (security)
- TLS Certificates (secure connections)

### Utilities Package (`internal/controller/utils`)

Provides shared functionality across all components.

**Contains:**
- **Constants**: Resource names, labels, annotations, error messages
- **Helper Functions**: Hash computation, resource comparison, equality checks
- **Status Utilities**: Condition management functions
- **Validation**: Certificate validation, version detection
- **Test Helpers**: Shared test fixtures and utilities
- **Types**: Configuration structures for OLS components

## Reconciliation Flow

```
1. Main Controller receives reconciliation request
   └── Validates OLSConfig CR exists
   
2. Reconcile LLM Secrets
   └── appserver.ReconcileLLMSecrets() OR lcore.checkLLMCredentials()
       ├── Validate provider credentials
       ├── Hash provider credentials
       └── Store hash in state cache
   
3. Reconcile Console UI (if enabled)
   └── console.ReconcileConsoleUI()
       ├── ConsolePlugin CR
       ├── ConfigMap
       ├── Service
       ├── Deployment
       └── Network Policy
   
4. Reconcile PostgreSQL (if conversation cache enabled)
   └── postgres.ReconcilePostgres()
       ├── ConfigMap
       ├── Secrets (bootstrap, credentials)
       ├── PVC
       ├── Service
       ├── Deployment
       └── Network Policy
   
5. Reconcile Backend (MUTUALLY EXCLUSIVE - controlled by --enable-lcore flag)
   
   OPTION A: Reconcile Application Server (LEGACY)
   └── appserver.ReconcileAppServer()
       ├── ServiceAccount & RBAC
       ├── ConfigMap (OLS config)
       ├── Service
       ├── TLS Secret
       ├── Deployment
       ├── Service Monitor (conditional - only if Prometheus Operator available)
       ├── Prometheus Rules (conditional - only if Prometheus Operator available)
       └── Network Policy
   
   OPTION B: Reconcile Lightspeed Core (NEW)
   └── lcore.ReconcileLCore()
       ├── ServiceAccount & RBAC
       ├── ConfigMap (Llama Stack config)
       ├── ConfigMap (LCore config)
       ├── Additional CA ConfigMap (if specified)
       ├── Proxy CA ConfigMap (if specified)
       ├── Service
       ├── TLS Secret
       ├── Deployment (dual-container: llama-stack + lightspeed-stack)
       ├── Service Monitor
       ├── Metrics Reader Secret
       └── Network Policy
   
6. Update Status Conditions
   └── Set condition based on deployment readiness
```

## Change Detection & Updates

The operator uses **ResourceVersion-based change detection** to trigger updates:

1. **Direct Spec Comparison**: Deployments are compared using `DeploymentSpecEqual()` utility
2. **ResourceVersion Tracking**: ConfigMaps and Secrets track their ResourceVersion in deployment annotations
3. **Annotation-based Triggers**: ResourceVersions are stored in deployment annotations
4. **Automatic Updates**: When ResourceVersions change, deployments are updated, triggering pod restarts

Example:
```go
// Get current ResourceVersion
currentVersion, _ := utils.GetConfigMapResourceVersion(r, ctx, configMapName)

// Store in deployment annotations
deployment.Annotations[OLSConfigMapResourceVersionAnnotation] = currentVersion

// Change detected: ResourceVersion differs -> update deployment -> pod restart
if storedVersion != currentVersion {
    // Update deployment
}
```

## Resource Watching

The operator uses the `watchers` package to watch for changes in:
- **OLSConfig CR**: Main configuration resource (owned resource)
- **Secrets**: LLM provider credentials, TLS certificates (watched via `SecretWatcherFilter`)
- **ConfigMaps**: Additional CA certificates, configuration overrides (watched via `ConfigMapWatcherFilter`)
- **Deployments**: Status monitoring for readiness (owned resource)
- **Telemetry Pull Secret**: Special watcher for `openshift-config/pull-secret` (via `TelemetryPullSecretWatcherFilter`)

**Watcher Annotations:**

Resources are annotated to identify which ones should trigger reconciliation:
```go
annotations[utils.WatcherAnnotationKey] = "cluster"  // OLSConfig name
// Key: "ols.openshift.io/watch-olsconfig"
```

**Automatic Restart Behavior:**

When a watched ConfigMap changes (e.g., CA certificates), the watcher:
1. Detects the change via annotation or special name matching
2. Determines which backend is active (AppServer or LCore)
3. Triggers a rolling restart of the appropriate deployment
4. Returns a reconciliation request to update the OLSConfig

This ensures that configuration changes are picked up without manual intervention.

## Testing Strategy

The codebase employs a comprehensive testing strategy with strong coverage:

### Test Coverage Summary
- **Main Controller**: 57.6% coverage
- **Appserver**: 82.2% coverage  
- **Console**: 70.5% coverage
- **Postgres**: 58.8% coverage
- **Utils**: 26.4% coverage

### Unit Tests
- **Location**: Co-located with source code in each package
  - `internal/controller/*_test.go` - Main controller tests (Reconcile loop, status updates, deployment checks)
  - `internal/controller/appserver/*_test.go` - App server component tests
  - `internal/controller/lcore/*_test.go` - LCore component tests
  - `internal/controller/postgres/*_test.go` - PostgreSQL component tests
  - `internal/controller/console/*_test.go` - Console UI component tests
  - `internal/controller/watchers/*_test.go` - Resource watcher tests (Secret/ConfigMap filters, annotations)
  - `internal/controller/utils/*_test.go` - Utility function tests (hashing, secrets, volume comparison)

- **Framework**: Ginkgo (BDD) + Gomega (assertions)
- **Environment**: envtest (local Kubernetes API server with CRDs)
- **Pattern**: Each package has its own test suite (`suite_test.go`) with mock reconciler implementing the `reconciler.Reconciler` interface

**Key Test Areas:**
- Main reconciliation loop (OLSConfig handling, error cases)
- Component-specific reconcilers (appserver, postgres, console)
- Resource generation and validation
- ResourceVersion-based change detection
- Status condition updates
- Deployment status checking
- Secret and ConfigMap operations
- Volume and container comparison

**Running Unit Tests:**
```bash
make test  # Runs all unit tests with coverage report
```

### Test Helpers
- **Location**: `internal/controller/utils/test_helpers.go`
- **Purpose**: Shared fixtures, CR generators, secret generators
- **Benefits**: Consistency across test suites, reduced duplication
- **Examples**: `GetDefaultOLSConfigCR()`, `GenerateRandomSecret()`, `GenerateRandomTLSSecret()`

### E2E Tests
- **Location**: `test/e2e/`
- **Scope**: Full operator behavior on real OpenShift clusters
- **Coverage**: Reconciliation, upgrades, database operations, TLS, metrics, BYOK, proxy support
- **Requirements**: Running cluster, KUBECONFIG, LLM_TOKEN

**Running E2E Tests:**
```bash
make test-e2e         # Full E2E test suite
make test-e2e-local   # E2E tests without storage requirements
make test-upgrade     # Upgrade scenario tests
```

## Design Patterns

### 1. Interface-Based Dependency Injection
Components receive a `reconciler.Reconciler` interface, not concrete types.

**Benefits:**
- Loose coupling
- Easy mocking for tests
- Clear contracts

### 2. Task-Based Reconciliation
Each component reconciles through a list of tasks:

```go
tasks := []ReconcileTask{
    {Name: "reconcile ConfigMap", Task: reconcileConfigMap},
    {Name: "reconcile Service", Task: reconcileService},
    // ...
}
for _, task := range tasks {
    if err := task.Task(ctx, cr); err != nil {
        return err
    }
}
```

### 3. Generate-Then-Apply Pattern
Resources are generated first, then applied:

```go
// Generate the desired resource
deployment := GenerateDeployment(r, cr)

// Apply to cluster
if err := r.Create(ctx, deployment); err != nil {
    if errors.IsAlreadyExists(err) {
        // Update existing
    }
}
```

### 4. Change Detection

The operator uses multiple strategies for detecting changes:

**For Deployments:**
- Direct spec comparison using `DeploymentSpecEqual()` utility
- ResourceVersion tracking for mounted ConfigMaps and Secrets
- Annotations store ResourceVersions of dependent resources

**For External Resources (Secrets/ConfigMaps not owned by operator):**
- Watcher annotations (`watchers.openshift.io/watch`) mark resources for monitoring
- Watchers detect changes and trigger deployment restarts via `Restart*()` functions

**For External Resources Owned by Kubernetes/Other Applications:**
- Watched by name (e.g., telemetry pull secret `pull-secret` in `openshift-config` namespace)
- Special watchers monitor specific named resources
- Changes trigger reconciliation without ownership or annotations

**For Owned Resources:**
- Direct equality comparison (e.g., `ConfigMapEqual()`)
- Kubernetes ownership triggers automatic reconciliation

## Key Design Decisions

### ✅ Why Component Packages?
- **Modularity**: Each component is self-contained
- **Maintainability**: Changes to one component don't affect others
- **Testability**: Independent test suites per component
- **Code Organization**: Clear boundaries and responsibilities

### ✅ Why Reconciler Interface?
- **Avoid Circular Dependencies**: Components don't import main controller
- **Clean Testing**: Easy to create test implementations
- **Flexibility**: Main controller can evolve without breaking components

### ✅ Why ResourceVersion-Based Detection?
- **Efficiency**: Only update when resources actually change (tracked via Kubernetes ResourceVersion)
- **Reliability**: Leverages Kubernetes' built-in change tracking mechanism
- **Simplicity**: No custom hash computation or state management needed
- **Correctness**: Kubernetes guarantees ResourceVersion changes on every modification

## Contributing Guidelines for Developers

### Adding a New Resource to a Component

1. **Create a generation function** in `assets.go`:
   ```go
   func GenerateMyResource(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*MyResourceType, error) {
       // Generate resource
   }
   ```

2. **Add reconciliation logic** in `reconciler.go`:
   ```go
   func reconcileMyResource(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
       resource, err := GenerateMyResource(r, cr)
       // Apply resource
   }
   ```

3. **Add to task list**:
   ```go
   tasks = append(tasks, ReconcileTask{
       Name: "reconcile my resource",
       Task: reconcileMyResource,
   })
   ```

4. **Write tests** in `*_test.go`:
   ```go
   It("should create my resource", func() {
       resource, err := GenerateMyResource(testReconcilerInstance, cr)
       Expect(err).NotTo(HaveOccurred())
       Expect(resource.Name).To(Equal("expected-name"))
   })
   ```

### Adding a New Component Package

1. Create directory: `internal/controller/newcomponent/`
2. Implement reconciliation: `reconciler.go` with `ReconcileNewComponent()` function
3. Implement resource generation: `assets.go`
4. Create test suite: `suite_test.go` with test reconciler
5. Add tests: `*_test.go` files
6. Update main controller: Call `newcomponent.ReconcileNewComponent(r, ctx, cr)`

### Code Style Guidelines

- **Error Messages**: Use constants from `utils` package
- **Logging**: Use structured logging via `r.GetLogger()`
- **Resource Names**: Define constants in `utils/constants.go`
- **Labels**: Use generator functions like `GenerateAppServerSelectorLabels()`
- **Testing**: Co-locate tests with source code, use shared test helpers

## Future Improvements

Potential areas for enhancement:
- Consolidate `ReconcileTask` and `DeleteTask` types into utils
- Consider builder pattern for test reconcilers
- Add integration test framework for cross-component testing
- Enhance observability with more detailed metrics
- Implement graceful degradation for optional components

---

## OLM Documentation

For operators deployed via Operator Lifecycle Manager (OLM), see our comprehensive OLM guide series:

1. **[OLM Bundle Management](./docs/olm-bundle-management.md)** - Creating and managing operator bundles
   - CSV (ClusterServiceVersion) structure and anatomy
   - Bundle annotations and metadata
   - Bundle generation workflow (`make bundle`)
   - Related images management
   - Version management and semantic versioning

2. **[OLM Catalog Management](./docs/olm-catalog-management.md)** - Organizing bundles into catalogs
   - File-Based Catalogs (FBC) structure
   - Multi-version catalog strategy (see `lightspeed-catalog-*` directories)
   - Channel management (alpha, beta, stable)
   - Skip ranges and upgrade paths
   - Catalog building and validation

3. **[OLM Integration & Lifecycle](./docs/olm-integration-lifecycle.md)** - OLM integration and operator lifecycle
   - OLM architecture and components
   - Installation workflow (Subscription, InstallPlan, CSV)
   - Upgrade mechanisms and strategies
   - Dependency resolution
   - RBAC and permissions management

4. **[OLM Testing & Validation](./docs/olm-testing-validation.md)** - Testing strategies and validation
   - Bundle and catalog validation
   - Installation and upgrade testing
   - E2E testing patterns (maps to `test/e2e/` implementation)
   - Scorecard and Preflight testing
   - CI/CD integration

5. **[OLM RBAC & Security](./docs/olm-rbac-security.md)** - Security and RBAC best practices
   - Operator RBAC permissions (see `config/rbac/` implementation)
   - User roles and API access (viewer, editor, query-access)
   - Security context configuration (see `config/manager/manager.yaml`)
   - Secrets management patterns
   - Network security and Pod Security Standards

**Quick Reference for OLM Tasks:**
- Generate bundle: `make bundle BUNDLE_TAG=x.y.z`
- Build catalog: `make catalog-build VERSION=4.18`
- Validate bundle: `operator-sdk bundle validate ./bundle`
- Check implementation: See `bundle/`, `config/rbac/`, and `hack/` directories

---

## Contributing

Want to add a new component or modify an existing one? The modular architecture makes this straightforward:

- **Adding Components**: See [CONTRIBUTING.md](CONTRIBUTING.md) for detailed step-by-step instructions
- **Modifying Components**: Follow the patterns established in existing components (appserver, postgres, console)
- **Testing**: Use the test helpers in `utils/test_helpers.go` for consistency

Key benefits of the modular architecture:
- **Isolated development**: Work on components independently
- **Clear boundaries**: Interface-based contracts prevent tight coupling
- **Easy testing**: Mock the reconciler interface for unit tests
- **Consistent patterns**: Follow established conventions across all components

---

For more information about the operator's functionality from a user perspective, see [README.md](README.md).

For AI assistant guidelines when working with this codebase, see [CLAUDE.md](CLAUDE.md).

