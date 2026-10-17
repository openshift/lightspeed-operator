# Resource Lifecycle

The operator manages two categories of Kubernetes resources: owned resources (created by the operator) and external resources (created by users or other controllers). Each category uses a different mechanism for change detection and reconciliation triggering.

## Behavioral Rules

### Owned Resources

1. Operator-owned resources have an OwnerReference pointing to the OLSConfig CR. Controller-runtime detects changes and triggers reconciliation for resource types registered with `Owns()`.
2. Owned resource types: Deployments, ServiceAccounts, ClusterRoles, ClusterRoleBindings, RoleBindings, Services, ConfigMaps, Secrets, PersistentVolumeClaims, ConsolePlugins, ServiceMonitors, PrometheusRules, ImageStreams.
3. The ConsolePlugin CR is cluster-scoped and cannot use standard namespace-scoped owner references. It is cleaned up explicitly during finalizer processing.
4. On OLSConfig deletion, the finalizer uses `listOwnedResources()` to inventory owned resources in the operator namespace by OwnerReference UID (not labels), explicitly deletes that inventory, and waits for it subject to the finalizer cleanup timeout before removing the finalizer. See `what/reconciliation.md` for finalizer sequencing.
4a. The OTel Dataverse exporter ConfigMap is covered by that namespaced inventory. Its two ClusterRoles, ClusterRoleBinding `lightspeed-otel-dataverse-exporter-binding`, and RoleBinding `lightspeed-otel-dataverse-exporter-pull-secret` in `openshift-config` carry OLSConfig owner references but are outside that inventory; on CR deletion, they rely on Kubernetes garbage collection rather than explicit finalizer deletion or wait. Garbage collection is asynchronous, so the finalizer does not guarantee those RBAC objects are gone before OLSConfig is recreated.
5. Owned resource changes (e.g., someone manually edits a managed ConfigMap) trigger reconciliation, and the operator overwrites them with the desired state.

### External Resources

6. External resources fall into two categories: system resources (fixed, known at compile time) and user-provided resources (derived from the CR spec at runtime).
7. System secrets: the telemetry pull secret (`openshift-config/pull-secret`) gates the existing app-server exporter when its `cloud.openshift.com` auth entry is present; that app-server presence check is unchanged. When the transcript gate enables credential checking, the OTel Dataverse credential gate requires a readable Secret with valid `.dockerconfigjson` JSON and a nonempty (after trimming whitespace) `.dockerconfigjson.auths["cloud.openshift.com"].auth` token; a NotFound pull-secret object or valid JSON with absent/empty telemetry auth disables the OTel sidecar. A missing `.dockerconfigjson` key on an existing Secret, malformed JSON, or API read errors other than NotFound are reconciliation errors, not disabled auth. Other system secrets: console UI service cert (`lightspeed-console-plugin-cert`); PostgreSQL certs (`lightspeed-postgres-certs`).
8. System configmaps: the OpenShift root CA (`kube-root-ca.crt`), the service CA bundle (`openshift-service-ca.crt`).
9. User-provided secrets: LLM provider credential secrets (`spec.llm.providers[].credentialsSecretRef`), custom TLS secret (`spec.ols.tlsConfig.keyCertSecretRef`), MCP server header secrets (`spec.mcpServers[].headers[].valueFrom.secretRef`).
10. User-provided configmaps: additional CA ConfigMap (`spec.ols.additionalCAConfigMapRef`), proxy CA ConfigMap (`spec.ols.proxyConfig.proxyCACertificate`), alerts adapter runtime config (`spec.ols.deployment.alertsAdapter.configMapRef`, when set).

### Annotation-Based Watching

11. The operator annotates each user-provided external resource with `ols.openshift.io/watcher: cluster` to mark it for watching.
11a. **Credential hot-reload exception (OLS-3450):** When `spec.ols.credentialHotReload` is `true`, LLM credential secrets (those with source prefix `llm-provider-*`) are excluded from annotation. Instead, `removeSecretAnnotationIfNeeded()` removes the watcher annotation if it was previously set. This prevents the watcher predicate from matching these secrets, so `SecretUpdateHandler` never fires for them. Non-LLM secrets (TLS, MCP headers) are always annotated regardless of the flag.
12. On each reconciliation, the operator clears the `AnnotatedSecretMapping` and `AnnotatedConfigMapMapping` in `WatcherConfig` and repopulates them from the current CR spec via `ForEachExternalSecret()` and `ForEachExternalConfigMap()`, then annotates any resources that lack the annotation.
13. Update events are accepted for annotated resources and configured system resources. Create events are accepted for resources in the operator namespace and for the configured telemetry pull secret in `openshift-config`; the handler verifies the resource is configured before acting. Creating or recreating the telemetry secret enqueues OLSConfig reconciliation to reevaluate the OTel exporter gate. Delete events are accepted for operator-namespace resources and configured system resources elsewhere; deleting the telemetry secret likewise enqueues reconciliation. Objects owned by OLSConfig are skipped by the external-resource handlers and handled through `Owns()`.

### Change Detection and Restart

14. When a watched secret's `.data` changes (compared via `apiequality.Semantic.DeepEqual`), the `SecretUpdateHandler` directly restarts affected deployments. A telemetry pull-secret data update also enqueues OLSConfig reconciliation so the OTel exporter gate and its sidecar-only ConfigMap/RBAC are reevaluated while enabled exporter processes refresh their credentials.
15. When a watched configmap's `.data` or `.binaryData` changes, the `ConfigMapUpdateHandler` triggers restarts of affected deployments directly.
16. Each external resource has an explicit list of affected deployments in `WatcherConfig`. The telemetry pull secret affects both `lightspeed-app-server` and `lightspeed-otel-collector`.
17. Restarts are triggered by updating the `ols.openshift.io/force-reload` annotation on the deployment's pod template with the current timestamp (RFC3339Nano), causing a rolling update. Alerts adapter runtime ConfigMap changes restart `lightspeed-agentic-alerts-adapter` via `RestartAlertsAdapter()`.
18. TLS secrets are mapped to affect the relevant operand deployment plus the app-server and the agentic configuration ConfigMap. User-provided secrets default to app-server only.

### Validation

19. Before annotating resources, the operator validates LLM provider credential secrets via `ValidateLLMCredentials()` (secret must exist and contain expected key) and custom TLS secrets via `ValidateTLSSecret()` (must contain `tls.crt` and `tls.key`).
20. Missing secrets for user-provided resources during annotation are not treated as errors. If a secret does not exist, `annotateSecretIfNeeded()` returns nil, and the resource will be picked up on the next reconciliation when it appears.
21. If `ValidateLLMCredentials()` or `ValidateTLSSecret()` fails, the operator sets `OverallStatus=NotReady` and a `ResourceReconciliation` Failed condition (preserving existing component conditions), then returns an error so controller-runtime retries with backoff.

## Configuration Surface

Resource lifecycle behavior is not directly user-configurable. External resources are derived from CRD fields:

| CR field | Resulting external resource |
|---|---|
| `spec.llm.providers[].credentialsSecretRef` | Provider credential secret |
| `spec.ols.tlsConfig.keyCertSecretRef` | Custom TLS secret |
| `spec.ols.additionalCAConfigMapRef` | Additional CA ConfigMap |
| `spec.ols.proxyConfig.proxyCACertificate` | Proxy CA ConfigMap |
| `spec.ols.deployment.alertsAdapter.configMapRef` | Alerts adapter runtime ConfigMap (restarts `lightspeed-agentic-alerts-adapter` on data change) |
| `spec.mcpServers[].headers[].valueFrom.secretRef` | MCP header secret |

## Constraints

1. The operator can only watch resources in its own namespace and in fixed external namespaces (`openshift-config` for the pull secret, `openshift-monitoring` for the client CA).
2. Delete events on watched external resources enqueue OLSConfig reconciliation so missing credentials, TLS secrets, or CA/config ConfigMaps are detected without waiting for an unrelated event.
2a. Create and data-update events for `openshift-config/pull-secret` also enqueue OLSConfig reconciliation so the OTel exporter’s nonempty-token gate and sidecar resources are reevaluated.
3. System resources are always watched regardless of CR configuration. They are defined in `WatcherConfig.Secrets.SystemResources` and `WatcherConfig.ConfigMaps.SystemResources`.
4. Owned resources with an OwnerReference are skipped by the external resource Create handler to avoid redundant processing; they are handled via the `Owns()` relationship.
5. Gate-off reconciliation explicitly deletes all five OTel exporter resources—the ConfigMap, both ClusterRoles, ClusterRoleBinding `lightspeed-otel-dataverse-exporter-binding`, and RoleBinding `lightspeed-otel-dataverse-exporter-pull-secret` in `openshift-config`—when transcripts are disabled, the pull-secret object is NotFound, or valid telemetry auth is absent/empty; missing `.dockerconfigjson`, malformed JSON, or API read errors other than NotFound are not treated as a disabled gate.

## Planned Changes

- [OLS-3450] Credential hot-reload: `removeSecretAnnotationIfNeeded()` added to remove watcher annotations from LLM credential secrets when `credentialHotReload` is enabled. See design spec `docs/superpowers/specs/2026-09-01-credential-hot-reload-design.md`.
