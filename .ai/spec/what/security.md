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
   - PostgreSQL (`lightspeed-postgres-server`): allows only backend pods (matched by `app.kubernetes.io/name: lightspeed-service-api` label) and OTel Collector pods (matched by the OTel Collector labels).
   - Console UI (`lightspeed-console-plugin`): allows only OpenShift Console pods from `openshift-console` namespace.
   - Agentic console plugin (`lightspeed-agentic-console-plugin`): allows OpenShift Console pods from `openshift-console` namespace.
   - Local alerts adapter (`lightspeed-agentic-alerts-adapter`): denies ingress.
   - OTEL Collector (`lightspeed-otel-collector`): allows all pods in the operator namespace (empty `PodSelector`) on OTLP gRPC `:4317` and `postgres_admin` HTTPS `:8080`; allows Prometheus from `openshift-monitoring` on HTTPS metrics `:8888` only.
   - Standalone OpenShift MCP (`openshift-mcp-server`) and RHOKP (`lightspeed-rhokp`): each allows any pod in the operator namespace and cluster Prometheus pods in `openshift-monitoring` on TCP `:8443` (OLS-3943). Both policies are removed with their feature-gated operands.
6. For MCP and RHOKP monitoring ingress, a single NetworkPolicy peer combines the `openshift-monitoring` namespace selector (`kubernetes.io/metadata.name`) with Prometheus pod selectors (`app.kubernetes.io/name: prometheus` and `prometheus: k8s`); neither policy grants access to every pod in the monitoring namespace.
6a. For cross-namespace ingress that must target specific source pods, combine namespace and pod selectors in the same NetworkPolicy peer.
7. PostgreSQL, the classic console plugin, the agentic console plugin (where deployed), and RHOKP each have an additional pod-specific NetworkPolicy with `policyTypes: [Egress]` and no egress allow rules. Their existing ingress-only policies retain their original source selectors and ports. The RHOKP egress policy is removed with RHOKP when `byokRAGOnly` is enabled. Standard NetworkPolicies are additive: another matching policy may allow egress, and these rules do not impose namespace-wide isolation.
7a. The opt-in local alerts adapter retains its ingress-deny NetworkPolicy and has a separate egress-only NetworkPolicy, reconciled only when enabled and removed when disabled. The egress policy allows DNS to `openshift-dns` pods over UDP/TCP 5353 (the `dns-default` Service exposes port 53 and forwards to pod port 5353), platform `alertmanager-main` pods in `openshift-monitoring` over TCP 9095 (the Service port is HTTPS TCP 9094), and the Kubernetes API over TCP 6443 with no destination peer. The API rule therefore permits the selected adapter pods to reach any destination on TCP 6443; this is an intentional tradeoff to avoid maintaining dynamic API endpoint IPs. NetworkPolicy connectivity rules do not grant API verbs or resources. On disable, retain the egress policy until the adapter Deployment and selector-matching pods are gone. During finalization, retain it while cleanup retries, bounded by the three-minute finalizer timeout; finalization then proceeds even if a pod remains. Validate the Alertmanager selector/port and API service translation on a target cluster before release.
7b. The app-server (including its Dataverse exporter sidecar), standalone MCP server, and OTel Collector retain no OLS-managed egress restriction because their destinations vary. These are deliberate non-coverage, not a guarantee of unrestricted connectivity or HPSTRAT-104 compliance. Agentic sandbox and hub-managed multicluster adapter exceptions are owned by their respective operators; see the parent spec.
7c. These are namespace-scoped standard `NetworkPolicy` rules; pending confirmation that they satisfy the operand portion of HPSTRAT-104, do not claim full compliance or add an `AdminNetworkPolicy` by assumption. Egress isolation applies to the selected pod, including sidecars and init containers. Other additive namespace policies may expand allowed egress; administrator policies may impose stricter limits. No new CRD field is required. See the [parent contract](https://github.com/openshift/ols/pull/104).

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
16. The shipped OpenShift MCP server is configured via a TOML config file (`read_only = false`, denied Secret/RBAC resources) so the LLM can use core write tools (e.g. `resources_create_or_update`) while secret data stays blocked at the server level. The standalone MCP Deployment (`ocpmcp`) does not pass `--read-only` on the command line; `read_only = false` in TOML overrides the RHEL image build default of `ReadOnly: true`.
17. The denied resources are configured in the `openshift-mcp-server-config` ConfigMap as a TOML config with entries blocking `core/v1/secrets`, `rbac.authorization.k8s.io/v1/roles`, `rbac.authorization.k8s.io/v1/rolebindings`, `rbac.authorization.k8s.io/v1/clusterroles`, and `rbac.authorization.k8s.io/v1/clusterrolebindings`.
18. User-defined MCP servers (via `spec.mcpServers`) are the user's responsibility to secure.

## Configuration Surface

Security behavior is not directly user-configurable beyond the TLS and network-related fields documented in `what/tls.md`. RBAC, network policies, and pod security contexts are fixed by the operator implementation.

## Constraints

1. The operator must not store credentials in ConfigMaps or environment variables directly. Secrets are always file-mounted as read-only volumes.
2. Network policies require a CNI plugin that supports NetworkPolicy enforcement.
3. All containers must run as non-root with read-only root filesystems.

## Known Limitations

1. **Agentic v2 dynamic cluster RBAC is not yet ownership-safe.** The agentic
   controller currently creates and deletes per-run `ClusterRole` and
   `ClusterRoleBinding` resources named `ls-exec-cluster-<AgenticRun UID>` and
   updates reader bindings discovered by ServiceAccount subject. Its controller
   identity therefore requires unrestricted mutation of those cluster-scoped
   RBAC resource types. Kubernetes RBAC cannot restrict this access to a
   dynamic name prefix, and a static `resourceNames` list would break creation
   and cleanup for new runs.

## Planned Changes

| Ticket | Summary |
|---|---|
| OLS-4324 | Validate the implemented opt-in local alerts-adapter egress policy's destinations and service translation on a target cluster before release. |
