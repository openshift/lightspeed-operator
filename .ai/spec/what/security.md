# Security

The operator enforces security boundaries through RBAC, network policies, pod security contexts, and credential management.

## Behavioral Rules

### RBAC
1. The operator creates a ClusterRole (`lightspeed-app-server-sar-role`) and ClusterRoleBinding for the backend service account with permissions for: SubjectAccessReview (create), TokenReview (create), ClusterVersion (get, list), and pull-secret Secret (get by resourceName).
2. These permissions enable the backend service to authenticate users via Kubernetes TokenReview and authorize API access via SubjectAccessReview.
3. The operator controller itself requires RBAC including: managing deployments, services, configmaps, secrets, PVCs, network policies, RBAC resources (clusterroles, clusterrolebindings, roles, rolebindings), console plugins, image streams, and monitoring resources (servicemonitors, prometheusrules). It also has NonResourceURL permissions for `/ls-access` and `/ols-metrics-access`.
4. The backend service account also receives a NonResourceURL permission for `/ls-access` to control Lightspeed API access (declared via kubebuilder RBAC markers on the controller).

### Network Policies
5. Each component has its own NetworkPolicy restricting ingress:
   - Operator (`lightspeed-operator`): allows Prometheus scraping from `openshift-monitoring` namespace on port 8443.
   - Backend/AppServer (`lightspeed-app-server`): allows Prometheus from `openshift-monitoring`, OpenShift Console pods from `openshift-console`, and ingress controllers (namespaces with `network.openshift.io/policy-group: ingress`), all on port 8443.
   - PostgreSQL (`lightspeed-postgres-server`): allows only backend pods (matched by `app.kubernetes.io/name: lightspeed-service-api` label).
   - Console UI (`lightspeed-console-plugin`): allows only OpenShift Console pods from `openshift-console` namespace.
6. Network policies use combined pod label selectors and namespace selectors for source filtering.
7. Egress is unrestricted for all components. PolicyTypes includes only `Ingress`; egress rules are empty (`[]`), meaning no egress restrictions.

### Pod Security
8. All containers (main containers and sidecars) run with restricted security context: `allowPrivilegeEscalation: false`, `readOnlyRootFilesystem: true`, `runAsNonRoot: true`, `seccompProfile: RuntimeDefault`, `capabilities: {drop: [ALL]}`. This is enforced via `utils.RestrictedContainerSecurityContext()`.
9. Writable paths (`/tmp`, llama-cache, user-data) use `emptyDir` volumes to provide write access on an otherwise read-only root filesystem.

### Credential Management
10. LLM provider credentials are validated during the annotation phase via `ValidateLLMCredentials()`. The operator verifies that each referenced secret exists and contains the expected key before proceeding with reconciliation.
11. Standard providers must have a secret with the `apitoken` key (or the key specified by `credentialKey`). Azure OpenAI providers must have either `apitoken` or all three of `client_id`, `tenant_id`, `client_secret`.
12. Custom TLS secrets are validated via `ValidateTLSSecret()` to ensure they contain `tls.crt` and `tls.key`.
13. Provider credentials are mounted as read-only volume files at `/etc/apikeys/<secretName>/`, never exposed as environment variables.
14. PostgreSQL passwords are generated randomly on first creation (via the postgres reconciler) and never updated on subsequent reconciliations.
15. MCP server header secrets must contain a specific key `header` (constant `MCPSECRETDATAPATH`) and are mounted read-only at `/etc/mcp/headers/<secretName>/`.

### OpenShift MCP Server Security
16. The shipped OpenShift MCP server runs with the `--read-only` flag and is configured via a TOML config file that blocks access to Secret and RBAC resources, preventing secret data from reaching the LLM.
17. The denied resources are configured in the `openshift-mcp-server-config` ConfigMap as a TOML config with entries blocking `core/v1/secrets`, `rbac.authorization.k8s.io/v1/roles`, `rbac.authorization.k8s.io/v1/rolebindings`, `rbac.authorization.k8s.io/v1/clusterroles`, and `rbac.authorization.k8s.io/v1/clusterrolebindings`.
18. User-defined MCP servers (via `spec.mcpServers`) are the user's responsibility to secure.

## Configuration Surface

Security behavior is not directly user-configurable beyond the TLS and network-related fields documented in `what/tls.md`. RBAC, network policies, and pod security contexts are fixed by the operator implementation.

## Constraints

1. The operator must not store credentials in ConfigMaps or environment variables directly. Secrets are always file-mounted as read-only volumes.
2. Network policies require a CNI plugin that supports NetworkPolicy enforcement.
3. All containers must run as non-root with read-only root filesystems.
