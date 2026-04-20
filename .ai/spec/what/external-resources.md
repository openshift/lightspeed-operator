# External Resources

The operator watches external resources (secrets, configmaps not created by the operator) to detect changes that require deployment restarts.

## Behavioral Rules

### Resource Categories
1. External resources fall into two categories: system resources (fixed, known at compile time) and user-provided resources (derived from the CR spec at runtime).
2. System secrets: the telemetry pull secret (`openshift-config/pull-secret`), console UI service cert (`lightspeed-console-plugin-cert`), PostgreSQL certs (`lightspeed-postgres-certs`).
3. System configmaps: the OpenShift root CA (`kube-root-ca.crt`), the service CA bundle (`openshift-service-ca.crt`).
4. User-provided secrets: LLM provider credential secrets (`spec.llm.providers[].credentialsSecretRef`), custom TLS secret (`spec.ols.tlsConfig.keyCertSecretRef`), MCP server header secrets (`spec.mcpServers[].headers[].valueFrom.secretRef`).
5. User-provided configmaps: additional CA ConfigMap (`spec.ols.additionalCAConfigMapRef`), proxy CA ConfigMap (`spec.ols.proxyConfig.proxyCACertificate`).

### Annotation-Based Watching
6. The operator annotates each user-provided external resource with `ols.openshift.io/watcher: cluster` to mark it for watching.
7. On each reconciliation, the operator clears the `AnnotatedSecretMapping` and `AnnotatedConfigMapMapping` in `WatcherConfig` and repopulates them from the current CR spec via `ForEachExternalSecret()` and `ForEachExternalConfigMap()`, then annotates any resources that lack the annotation.
8. The watcher predicate on Update events checks for two conditions: (a) the resource has the `ols.openshift.io/watcher` annotation, or (b) the resource is a configured system resource. Create events are allowed for all resources in the operator namespace (to handle recreated resources that have not been annotated yet). Create events also verify the resource is referenced in the CR before acting. Delete events are always ignored.

### Change Detection and Restart
9. When a watched secret's `.data` changes (compared via `apiequality.Semantic.DeepEqual`), the `SecretUpdateHandler` triggers restarts of affected deployments directly, without triggering a full reconciliation.
10. When a watched configmap's `.data` or `.binaryData` changes, the `ConfigMapUpdateHandler` triggers restarts of affected deployments directly.
11. Each external resource has a list of affected deployments configured in `WatcherConfig`. The special value `ACTIVE_BACKEND` resolves to the currently active backend deployment name: `lightspeed-app-server` (AppServer) or `lightspeed-stack-deployment` (LCore), based on the `--enable-lcore` flag.
12. Restarts are triggered by updating the `ols.openshift.io/force-reload` annotation on the deployment's pod template with the current timestamp (RFC3339Nano), causing a rolling update.
13. TLS secrets are mapped to affect both `lightspeed-console-plugin` and `ACTIVE_BACKEND` deployments. All other user-provided secrets default to `ACTIVE_BACKEND` only.

### Validation
14. Before annotating resources, the operator validates LLM provider credential secrets via `ValidateLLMCredentials()` (secret must exist and contain expected key) and custom TLS secrets via `ValidateTLSSecret()` (must contain `tls.crt` and `tls.key`).
15. Missing secrets for user-provided resources during annotation are not treated as errors. If a secret does not exist, `annotateSecretIfNeeded()` returns nil, and the resource will be picked up on the next reconciliation when it appears.

## Configuration Surface

External resources are not directly configured. They are derived from:

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
4. Operator-owned resources (those with an OwnerReference pointing to the OLSConfig CR) are skipped by the Create handler to avoid redundant processing; they are handled via the Owns() relationship in the controller setup.
