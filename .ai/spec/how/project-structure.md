# Project Structure

## Module Map

| Path | Key Symbols | Responsibility |
|---|---|---|
| `api/v1alpha1/olsconfig_types.go` | `OLSConfig`, `OLSConfigSpec`, `OLSConfigStatus`, `ProviderSpec`, `ModelSpec` | CRD type definitions, validation markers, defaults |
| `api/v1alpha1/groupversion_info.go` | `SchemeBuilder`, `GroupVersion` | API group/version registration |
| `api/v1alpha1/zz_generated.deepcopy.go` | Generated `DeepCopyObject()` methods | Auto-generated deep copy |
| `cmd/main.go` | `main()`, `overrideImages()` | Operator entry point, flag parsing, manager setup |
| `internal/controller/olsconfig_controller.go` | `OLSConfigReconciler`, `Reconcile()`, `SetupWithManager()` | Main reconciler, orchestration, watcher registration |
| `internal/controller/olsconfig_helpers.go` | `UpdateStatusCondition()`, `checkDeploymentStatus()`, `annotateExternalResources()`, `shouldWatchSecret()` | Status management, diagnostics, annotation, watcher predicates |
| `internal/controller/operator_assets.go` | `ReconcileServiceMonitorForOperator()`, `ReconcileNetworkPolicyForOperator()` | Operator-level resources |
| `internal/controller/appserver/reconciler.go` | `ReconcileAppServerResources()`, `ReconcileAppServerDeployment()` | AppServer Phase 1 + Phase 2 orchestration |
| `internal/controller/appserver/deployment.go` | `GenerateOLSDeployment()`, `updateOLSDeployment()` | AppServer deployment generation, update detection |
| `internal/controller/appserver/assets.go` | `GenerateOLSConfigMap()`, service/RBAC/ServiceMonitor/PrometheusRule generators | AppServer resource generation, OLS config YAML |
| `internal/controller/appserver/rag.go` | `GenerateRAGInitContainers()`, `reconcileImageStreams()` | RAG init container and ImageStream management |
| `internal/controller/lcore/reconciler.go` | `ReconcileLCoreResources()`, `ReconcileLCoreDeployment()` | LCore Phase 1 + Phase 2 orchestration |
| `internal/controller/lcore/deployment.go` | `GenerateLCoreDeployment()`, `generateLCoreServerDeployment()`, `generateLCoreLibraryDeployment()` | LCore deployment generation (server/library modes) |
| `internal/controller/lcore/config.go` | `buildLlamaStackYAML()`, `buildLCoreConfigYAML()` | Llama Stack and LCore config generation |
| `internal/controller/lcore/assets.go` | Service, RBAC, ServiceMonitor, PrometheusRule, ConfigMap generators | LCore resource generation |
| `internal/controller/postgres/reconciler.go` | `ReconcilePostgresResources()`, `ReconcilePostgresDeployment()` | PostgreSQL Phase 1 + Phase 2 |
| `internal/controller/postgres/deployment.go` | `GeneratePostgresDeployment()` | PostgreSQL deployment generation |
| `internal/controller/postgres/assets.go` | `GeneratePostgresConfigMap()`, `GeneratePostgresBootstrapSecret()`, `GeneratePostgresSecret()` | PostgreSQL config, bootstrap script, credentials |
| `internal/controller/console/reconciler.go` | `ReconcileConsoleUIResources()`, `ReconcileConsoleUIDeploymentAndPlugin()`, `RemoveConsoleUI()` | Console UI Phase 1 + Phase 2 + cleanup |
| `internal/controller/console/deployment.go` | `GenerateConsoleUIDeployment()` | Console UI deployment generation |
| `internal/controller/console/assets.go` | ConsolePlugin CR generator, nginx config, service, network policy | Console UI resource generation |
| `internal/controller/reconciler/interface.go` | `Reconciler` interface | Dependency injection interface for component packages |
| `internal/controller/utils/constants.go` | ~200 constants | Resource names, ports, paths, annotation keys, defaults |
| `internal/controller/utils/errors.go` | ~80 error message constants | Structured error messages for all operations |
| `internal/controller/utils/mcp_server_config.go` | `GenerateOpenShiftMCPServerConfigMap()`, TOML config | MCP server configuration with denied resources |
| `internal/controller/utils/postgres_wait.go` | `GeneratePostgresWaitInitContainer()` | PostgreSQL readiness init container |
| `internal/controller/watchers/watchers.go` | `SecretUpdateHandler`, `ConfigMapUpdateHandler`, `SecretWatcherFilter()`, `ConfigMapWatcherFilter()` | External resource change handlers, deployment restart logic |
| `internal/tls/` | `GetTLSProfileSpec()`, `FetchAPIServerTlsProfile()` | TLS profile resolution |
| `config/crd/` | CRD YAML manifests | Generated CRD definitions |
| `config/rbac/` | RBAC YAML manifests | Generated RBAC rules |
| `config/manager/` | Deployment manifest | Operator deployment |
| `test/e2e/` | E2E test suites | End-to-end integration tests |

## Startup Sequence

```
main()
  1. Parse flags (images, namespace, feature flags: --use-lcore, --lcore-server)
  2. Get Kubernetes config and client
  3. Detect OpenShift version (major, minor)
  4. Select console image: if minor < 19 -> PF5, else -> PF6
  5. Check Prometheus Operator availability (probe CRD existence)
  6. Configure metrics TLS (if --secure-metrics-server):
     a. Read client CA from openshift-monitoring/metrics-client-ca
     b. Read TLS profile from OLSConfig CR or API server
  7. Create controller manager with:
     - Multi-namespace cache (operator ns + openshift-config for secrets)
     - TLS metrics server
     - Health/readiness probes (ping)
     - Leader election (if enabled)
  8. Build WatcherConfig (system secrets + configmaps)
  9. Create OLSConfigReconciler with all options
  10. Register with manager via SetupWithManager()
  11. Start manager (blocking)
```

## Data Flow

### Reconciliation Flow
```
OLSConfigReconciler.Reconcile()
  1. getAndValidateCR()             -- Only processes CR named "cluster"
  2. handleFinalizer()              -- Add finalizer or run deletion cleanup
  3. reconcileOperatorResources()   -- ServiceMonitor, NetworkPolicy (operator-level)
  4. annotateExternalResources()    -- Mark external secrets/configmaps for watching
  5. reconcileIndependentResources()  -- Phase 1: ConfigMaps, Secrets, ServiceAccounts, RBAC, NetworkPolicies
     +-- console.ReconcileConsoleUIResources()
     +-- postgres.ReconcilePostgresResources()
     +-- appserver.ReconcileAppServerResources() OR lcore.ReconcileLCoreResources()  [mutually exclusive]
  6. reconcileDeploymentsAndStatus()  -- Phase 2: Deployments, Services, TLS certs, status
     +-- console.ReconcileConsoleUIDeploymentAndPlugin()
     +-- postgres.ReconcilePostgresDeployment()
     +-- appserver.ReconcileAppServerDeployment() OR lcore.ReconcileLCoreDeployment()
     +-- checkDeploymentStatus() per deployment -> build newStatus
     +-- UpdateStatusCondition()
```

Phase 1 uses continue-on-error (reconciles all resources even if some fail).
Phase 2 uses fail-fast per step but collects status for all steps.

### Watcher-Triggered Restart Flow
```
External secret/configmap changes
  -> Watches() with custom predicate (shouldWatchSecret/shouldWatchConfigMap)
  -> SecretUpdateHandler.Update() / ConfigMapUpdateHandler.Update()
     -> Compare old vs new Data (DeepEqual)
     -> If changed: SecretWatcherFilter() / ConfigMapWatcherFilter()
        -> Match against SystemResources list (by name+namespace)
        -> OR match against WatcherAnnotationKey annotation
        -> Resolve "ACTIVE_BACKEND" to appserver or lcore deployment name
        -> Call RestartAppServer() / RestartLCore() / RestartPostgres() / RestartConsoleUI()
           -> Set force-reload annotation with current timestamp
```

## Key Abstractions

### Image Management
Default images are stored in a `defaultImages` map in `cmd/main.go` keyed by logical name (e.g., `"lightspeed-service"`, `"postgres-image"`, `"console-plugin"`). Default values come from `internal/relatedimages/` which reads `related_images.json` at build time. Command-line flags override individual images. The map is passed to the reconciler via `OLSConfigReconcilerOptions` as individual named fields (e.g., `LightspeedServiceImage`, `ConsoleUIImage`).

### WatcherConfig
Declarative configuration for external resource watching. Contains:
- `Secrets.SystemResources`: Fixed list of system secrets with affected deployment names (telemetry pull secret, console TLS cert, postgres TLS cert)
- `ConfigMaps.SystemResources`: Fixed list of system configmaps (kube-root-ca.crt, service-ca bundle)
- `AnnotatedSecretMapping`: Dynamic map populated from CR spec at runtime (maps secret name to deployment names)
- `AnnotatedConfigMapMapping`: Dynamic map populated from CR spec at runtime (maps configmap name to deployment names)
The special deployment name `"ACTIVE_BACKEND"` resolves to either the AppServer or LCore deployment name based on the `--use-lcore` flag.

### Component Package Pattern
Each component (appserver, lcore, postgres, console) follows the same package structure:
- `reconciler.go`: Phase 1 (resources) and Phase 2 (deployment) entry points
- `deployment.go`: Deployment spec generation and update detection
- `assets.go` and/or `config.go`: Resource and config generation
The packages receive `reconciler.Reconciler` interface, never import the controller package.

### Reconciler Interface (`internal/controller/reconciler/interface.go`)
Embeds `client.Client` and adds getter methods for:
- `GetScheme()`, `GetLogger()`, `GetNamespace()`
- Image getters: `GetAppServerImage()`, `GetPostgresImage()`, `GetConsoleUIImage()`, `GetOpenShiftMCPServerImage()`, `GetDataverseExporterImage()`, `GetLCoreImage()`
- Version getters: `GetOpenShiftMajor()`, `GetOpenshiftMinor()`
- Config getters: `IsPrometheusAvailable()`, `GetWatcherConfig()`, `UseLCore()`, `GetLCoreServerMode()`

### Finalizer Pattern
The OLSConfig CR uses finalizer `ols.openshift.io/finalizer` (defined in `utils.OLSConfigFinalizer`). On deletion:
1. Remove Console UI (deactivate plugin, delete ConsolePlugin CR)
2. List all owned resources via owner references
3. Explicitly delete owned resources
4. Wait up to 3 minutes for deletion (poll every 5 seconds)
5. Remove finalizer (proceeds even if cleanup times out)

## Integration Points

| Component | External Dependency | Mechanism |
|---|---|---|
| Manager cache | `openshift-config` namespace | Multi-namespace cache config for telemetry pull secret |
| Console image selection | OpenShift version | API call to `clusterversions.config.openshift.io` |
| Metrics TLS | `openshift-monitoring/metrics-client-ca` | ConfigMap read at startup |
| TLS profile | OLSConfig CR or API server | CR field or `apiservers.config.openshift.io` |
| Prometheus resources | Prometheus Operator CRDs | CRD existence check at startup; skips if unavailable |
| External secret watching | User-provided LLM secrets, MCP header secrets | Annotation-based (`watchers.openshift.io/watch`) |
| External configmap watching | Additional CA, proxy CA configmaps | Annotation-based (`watchers.openshift.io/watch`) |

## Implementation Notes

- The operator uses kubebuilder v3 markers for CRD generation and RBAC.
- The test suite uses envtest (local API server) with Ginkgo v2/Gomega.
- `make test` is required instead of `go test` because the Makefile handles envtest setup.
- The `cmd/check-isa-level/` package is a build-time utility for AMD64 ISA level checking.
- All generated files (deepcopy, CRD YAML) should be regenerated after API type changes using `make generate manifests`.
- The OLSConfig CRD is cluster-scoped and validated to require `.metadata.name == "cluster"`.
- `SetupWithManager()` registers `Owns()` watches for: Deployment, ServiceAccount, ClusterRole, ClusterRoleBinding, Service, ConfigMap, Secret, PersistentVolumeClaim, ConsolePlugin, ServiceMonitor, PrometheusRule, ImageStream.
- Controller-runtime handles retry with exponential backoff; the operator does not use periodic reconciliation.
- `LOCAL_DEV_MODE=true` env var skips ServiceMonitor creation for local development with `make run-local`.
