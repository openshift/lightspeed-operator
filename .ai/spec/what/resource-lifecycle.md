# Resource Lifecycle

The operator manages two categories of Kubernetes resources: owned resources (created by the operator) and external resources (created by users or other controllers). Each category uses a different mechanism for change detection and reconciliation triggering.

## Behavioral Rules

### Owned Resources

1. The operator creates resources with an OwnerReference pointing to the OLSConfig CR. Controller-runtime detects changes to these resources automatically via `Owns()` registrations and triggers reconciliation.
2. Owned resource types: Deployments, ServiceAccounts, ClusterRoles, ClusterRoleBindings, Services, ConfigMaps, Secrets, PersistentVolumeClaims, ConsolePlugins, ServiceMonitors, PrometheusRules, ImageStreams.
3. The ConsolePlugin CR is cluster-scoped and cannot use standard namespace-scoped owner references. It is cleaned up explicitly during finalizer processing.
4. On CR deletion, the finalizer lists all owned resources by matching OwnerReference UID (not labels), explicitly deletes them, and waits for deletion to complete before removing the finalizer. See `what/reconciliation.md` for finalizer sequencing.
5. Owned resource changes (e.g., someone manually edits a managed ConfigMap) trigger reconciliation, and the operator overwrites them with the desired state.

### External Resources

6. External resources fall into two categories: system resources (fixed, known at compile time) and user-provided resources (derived from the CR spec at runtime).
7. System secrets: the telemetry pull secret (`openshift-config/pull-secret`), console UI service cert (`lightspeed-console-plugin-cert`), PostgreSQL certs (`lightspeed-postgres-certs`).
8. System configmaps: the OpenShift root CA (`kube-root-ca.crt`), the service CA bundle (`openshift-service-ca.crt`).
9. User-provided secrets: LLM provider credential secrets (`spec.llm.providers[].credentialsSecretRef`), custom TLS secret (`spec.ols.tlsConfig.keyCertSecretRef`), MCP server header secrets (`spec.mcpServers[].headers[].valueFrom.secretRef`).
10. User-provided configmaps: additional CA ConfigMap (`spec.ols.additionalCAConfigMapRef`), proxy CA ConfigMap (`spec.ols.proxyConfig.proxyCACertificate`).

### Annotation-Based Watching

11. The operator annotates each user-provided external resource with `ols.openshift.io/watcher: cluster` to mark it for watching.
12. On each reconciliation, the operator clears the `AnnotatedSecretMapping` and `AnnotatedConfigMapMapping` in `WatcherConfig` and repopulates them from the current CR spec via `ForEachExternalSecret()` and `ForEachExternalConfigMap()`, then annotates any resources that lack the annotation.
13. The watcher predicate on Update events checks for two conditions: (a) the resource has the `ols.openshift.io/watcher` annotation, or (b) the resource is a configured system resource. Create events are allowed for all resources in the operator namespace (to handle recreated resources that have not been annotated yet). Create events also verify the resource is referenced in the CR before acting. Delete events are always ignored.

### Change Detection and Restart

14. When a watched secret's `.data` changes (compared via `apiequality.Semantic.DeepEqual`), the `SecretUpdateHandler` triggers restarts of affected deployments directly, without triggering a full reconciliation.
15. When a watched configmap's `.data` or `.binaryData` changes, the `ConfigMapUpdateHandler` triggers restarts of affected deployments directly.
16. Each external resource has a list of affected deployments configured in `WatcherConfig`. The special value `ACTIVE_BACKEND` resolves to the application server deployment name (`lightspeed-app-server`).
17. Restarts are triggered by updating the `ols.openshift.io/force-reload` annotation on the deployment's pod template with the current timestamp (RFC3339Nano), causing a rolling update.
18. TLS secrets are mapped to affect both `lightspeed-console-plugin` and `ACTIVE_BACKEND` deployments. All other user-provided secrets default to `ACTIVE_BACKEND` only.

### Validation

19. Before annotating resources, the operator validates LLM provider credential secrets via `ValidateLLMCredentials()` (secret must exist and contain expected key) and custom TLS secrets via `ValidateTLSSecret()` (must contain `tls.crt` and `tls.key`).
20. Missing secrets for user-provided resources during annotation are not treated as errors. If a secret does not exist, `annotateSecretIfNeeded()` returns nil, and the resource will be picked up on the next reconciliation when it appears.

## Configuration Surface

Resource lifecycle behavior is not directly user-configurable. External resources are derived from CRD fields:

| CR field | Resulting external resource |
|---|---|
| `spec.llm.providers[].credentialsSecretRef` | Provider credential secret |
| `spec.ols.tlsConfig.keyCertSecretRef` | Custom TLS secret |
| `spec.ols.additionalCAConfigMapRef` | Additional CA ConfigMap |
| `spec.ols.proxyConfig.proxyCACertificate` | Proxy CA ConfigMap |
| `spec.mcpServers[].headers[].valueFrom.secretRef` | MCP header secret |

## Constraints

1. The operator can only watch resources in its own namespace and in fixed external namespaces (`openshift-config` for the pull secret, `openshift-monitoring` for the client CA).
2. Delete events on external resources do not trigger restarts or reconciliation. The operator detects the absence during the next reconciliation triggered by other events.
3. System resources are always watched regardless of CR configuration. They are defined in `WatcherConfig.Secrets.SystemResources` and `WatcherConfig.ConfigMaps.SystemResources`.
4. Owned resources with an OwnerReference are skipped by the external resource Create handler to avoid redundant processing; they are handled via the `Owns()` relationship.
5. Owned resources are not deleted individually during normal operation. They are only explicitly deleted during finalizer cleanup on CR deletion.
