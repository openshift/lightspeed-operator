# OpenShift MCP Server (ocp-mcp)

Standalone HTTPS OpenShift MCP server operand managed by the `ocpmcp` package ([OLS-3526](https://redhat.atlassian.net/browse/OLS-3526)). Replaces the former app-server sidecar. Related: [OLS-3684](https://redhat.atlassian.net/browse/OLS-3684) (agentic handoff MCP keys/CA), [OLS-3594](https://redhat.atlassian.net/browse/OLS-3594) (deferred agentic auto-injection).

## Architecture

```text
lightspeed-service (app-server)
  └─ HTTPS MCP client
       url: https://openshift-mcp-server.<ns>.svc:8443/mcp
       trust: Secret lightspeed-agentic-mcp-ca → /etc/certs/openshift-mcp-server-ca/service-ca.crt
            │  (PEM from openshift-service-ca.crt; same cluster CA as OTEL)
            ▼
openshift-mcp-server Deployment + ClusterIP Service (:8443)
  ├─ service-ca serving cert Secret  openshift-mcp-server-tls
  └─ TOML ConfigMap                   openshift-mcp-server-config
```

Gated by `spec.ols.introspectionEnabled` (default `true` when absent). When false, the operator removes managed MCP resources and sets `MCPServerReady=True` with `Reason=NotConfigured`.

## Behavioral Rules

### Activation
1. When `spec.ols.introspectionEnabled` is true (or absent), Phase 1 and Phase 2 reconcile the standalone MCP operand.
2. When false, Phase 1 calls `ocpmcp.Remove()`; Phase 2 skips deployment reconciliation and records `MCPServerReady` as `NotConfigured`.

### Phase 1 Resources
3. ConfigMap `openshift-mcp-server-config` — TOML runtime config (pinned toolsets, denied Secret/RBAC resources, metrics endpoints).
4. ServiceAccount `openshift-mcp-server` — no RBAC bindings; callers pass their own token (app-server uses `Authorization: ols`).
5. NetworkPolicy `openshift-mcp-server` — allows TCP `:8443` ingress from any pod in the operator namespace and from cluster Prometheus pods in `openshift-monitoring`. The Prometheus peer requires both the namespace label `kubernetes.io/metadata.name: openshift-monitoring` and pod labels `app.kubernetes.io/name: prometheus` and `prometheus: k8s` (OLS-3943); it does not allow every pod in the monitoring namespace. The policy remains ingress-only and is removed when introspection is disabled.

### Phase 2 Resources
6. Service `openshift-mcp-server` — ClusterIP, port `https` `:8443`, serving-cert annotation → Secret `openshift-mcp-server-tls`.
7. Wait for TLS Secret keys `tls.crt` / `tls.key` before creating/updating the Deployment.
8. Deployment `openshift-mcp-server` — HTTPS (`--tls-cert` / `--tls-key`), probes on `/healthz` (HTTPS), image from `--openshift-mcp-server-image`, `PullIfNotPresent`. Replicas/resources/tolerations/nodeSelector from `spec.ols.deployment.mcpServer` (`Config`).

### App-server Integration
9. olsconfig `mcp_servers` includes an `openshift` entry pointing at `https://openshift-mcp-server.<namespace>.svc:8443/mcp` with `Authorization: ols` when introspection is enabled. See `app-server.md` and `config-generation.md`.
10. App-server mounts client CA Secret `lightspeed-agentic-mcp-ca` (sourced from `openshift-service-ca.crt`) at `/etc/certs/openshift-mcp-server-ca/` and adds `service-ca.crt` to `extra_ca`. See `tls.md` / `agentic-sandbox-profile.md`. There is no dedicated MCP inject-cabundle ConfigMap; Phase 1 / `Remove` deletes leftover `openshift-mcp-server-ca` on upgrade.
11. App-server Deployment tracks MCP client CA content hash (`ols.openshift.io/mcp-server-ca-configmap-hash`) only while introspection is enabled.

### Watching and Restarts
12. Secret `openshift-mcp-server-tls` is listed statically in `WatcherConfig.Secrets.SystemResources`. Watching is gated by `OpenShiftMCPServerTLSWatchEnabled` (`syncOpenShiftMCPServerTLSWatcher`), set from `introspectionEnabled`, so enable/disable does not rewrite the SystemResources slice under the informer.
13. On TLS Secret data change, the watcher restarts `openshift-mcp-server`, `lightspeed-app-server`, and touches `lightspeed-agentic-configuration`. `RestartAppServer` refreshes client CA Secrets from `openshift-service-ca.crt` and touches the handoff ConfigMap (fail-closed if CA refresh fails — see `agentic-sandbox-profile.md`).
14. ConfigMap `openshift-service-ca.crt` changes also restart `lightspeed-app-server`, refreshing all client CA Secrets (OTEL, MCP, RHOKP).
15. MCP Deployment also tracks ConfigMap and TLS Secret ResourceVersions and rolls when they change.

### Security
16. TOML denies `core/v1` `Secret` and all `rbac.authorization.k8s.io/v1` resources so Secret/RBAC data cannot reach the LLM via the shipped server.
17. Base toolsets are pinned to `core`, `config`, `helm`, `observability/metrics`, `kubevirt`. [PLANNED: OLS-4391] The operator additionally includes `netobserv` when FlowCollector API discovery confirms presence, subject to the discovery-error retention rules below. Metrics uses in-cluster Thanos Querier and Alertmanager URLs. Metrics `guardrails = "!tsdb"` (PromQL query safety, not RBAC) follows upstream OpenShift guidance when Thanos lacks the TSDB status API; auth remains the caller's bearer token.
18. User-defined MCP servers (`spec.mcpServers`) are out of scope for this operand.

### Monitoring
19. ServiceMonitor `openshift-mcp-server-monitor` (OLS-3728) — scrapes MCP server metrics via HTTPS on port 8443, path `/metrics` (Go promhttp). Server TLS only (service-ca CA bundle + `serverName`; no client certs / Bearer token), 30s interval. Reconciled in Phase 2 via `utils.ReconcileServiceMonitor()`. Skipped if Prometheus Operator CRDs are not installed. The NetworkPolicy ingress in rule 5 admits the cluster Prometheus scrape (OLS-3943); the ServiceMonitor alone does not grant network access.

### Finalizer
20. On CR deletion, `ocpmcp.Remove()` deletes Deployment, Service, NetworkPolicy, ConfigMap, ServiceAccount, TLS Secret (`openshift-mcp-server-tls`), and ServiceMonitor (`openshift-mcp-server-monitor`) before owned-resource sweep.

### NetObserv API-presence Gating [PLANNED: OLS-4391]
21. With introspection enabled, the operator uses its own Kubernetes API discovery client to determine whether any served API version in group `flows.netobserv.io` advertises the resource `flowcollectors` with kind `FlowCollector`. Presence in any successfully discovered served version is sufficient. Confirmed absence requires successful discovery of the relevant API surface; permission errors, timeouts, and partial discovery failures affecting this group are unknown, not absence.
22. On confirmed presence, generated TOML contains the base toolsets from rule 17 plus `netobserv` exactly once. On confirmed absence, it contains only the base toolsets. This decision gates only NetObserv; it must not disable the MCP operand or change the base toolsets.
23. Detection does not read FlowCollector instances, inspect operator Deployments or OLM resources, probe plugin readiness, or derive backend settings. The operator does not generate a NetObserv-specific configuration section or add OLSConfig fields. The MCP server uses its existing NetObserv defaults.
24. A discovery error retains the last successfully determined NetObserv inclusion state. On restart, retain that state from the existing operator-generated MCP configuration when available. If no prior decision is available, omit `netobserv` until discovery succeeds. Log the discovery failure and retry through the existing controller-runtime error backoff; reconcile the remaining MCP resources and independent components before reporting the error. Discovery uncertainty must not prevent the base MCP configuration from being deployed.
25. Detect API installation and removal without an OLSConfig edit or operator restart. A targeted watch on the FlowCollector CRD is the preferred event-driven mechanism; relevant creation, deletion, establishment, or served-version changes must enqueue reconciliation and refresh cached discovery. If the CRD event precedes API availability, retry until discovery reflects the change rather than relying on another user action. Verify and provide the operator watch RBAC needed by the selected implementation; do not add FlowCollector-read privileges or MCP operand ServiceAccount bindings. No periodic steady-state reconciliation is introduced.
26. When the inclusion decision changes, update `openshift-mcp-server-config` and use its existing Deployment change tracking to roll the MCP server. An unchanged decision must not rewrite identical configuration or cause a rollout solely because of detection. With introspection disabled, the existing removal/`NotConfigured` behavior takes precedence; presence detection must not recreate the operand.
27. API presence is a minimum installation proxy, not proof that Network Observability is installed, healthy, or usable. Retained CRDs can yield a false positive, and a FlowCollector instance is not required for enablement. Plugin connectivity, service-CA trust, caller permissions, and the appropriate flow/metrics backends remain prerequisites for successful calls. The operator does not modify NetObserv network policies, provision backends, weaken TLS verification, or grant caller permissions.
28. No OCP MCP server code change, FlowCollector auto-configuration feature, or upstream PR #1448 is required for this gate. The shipped image must contain the existing `netobserv` toolset; verification evidence for the image examined during design is recorded in decision 0001. Generic user-configurable toolsets remain separate work under OLS-2715. Agentic auto-injection remains deferred under OLS-3594.

## Acceptance Coverage [PLANNED: OLS-4391]

| Scenario | Required result |
|---|---|
| FlowCollector API present in a served version | Include `netobserv` exactly once; preserve all base toolsets. |
| Relevant API discovery succeeds and the FlowCollector API is absent | Omit `netobserv`; deploy the remaining MCP operand normally. |
| CRD/API present but no FlowCollector instance exists | Include `netobserv`; no instance read is performed. |
| API installed after OLS starts | Reconcile and enable NetObserv without editing OLSConfig or restarting the operator; refresh stale negative discovery results and retry establishment races. |
| API removed, or no FlowCollector version remains served | Reconcile and omit NetObserv; refresh stale positive discovery results. |
| Permission, timeout, or relevant partial-discovery error | Preserve the prior inclusion state, log, and retry; do not interpret the error as absence. |
| Operator restart followed by discovery failure | Retain the existing generated configuration's inclusion state; when no prior state exists, deploy base toolsets without NetObserv and retry. |
| Repeated discovery with unchanged inclusion | No detection-only ConfigMap rewrite or MCP Deployment rollout. |
| Introspection disabled, including during a CRD event | Keep MCP resources removed and `MCPServerReady=True`, `Reason=NotConfigured`. |
| NetObserv backend absent/unreachable despite API presence | MCP server remains startable and base tools remain usable; a NetObserv invocation may return a backend error. |
| Security and scope regression | Secret/RBAC denials, caller-token authorization, and unprivileged MCP ServiceAccount remain unchanged; no NetObserv endpoints, new OLSConfig fields, or FlowCollector-read permissions are introduced. |

Use unit tests for discovery outcomes, generated TOML, error retention, and event selection. Use controller/integration tests for CRD events, discovery-cache refresh, startup/restart behavior, retry paths, and existing ConfigMap-driven rollouts. Validate on OpenShift that the selected shipped MCP image starts with NetObserv enabled and that base tools remain available when the plugin is absent. Successful NetObserv calls additionally require the prerequisites in rule 27; they are not guaranteed by this gate.

## Configuration Surface

| Field path | Description |
|---|---|
| `spec.ols.introspectionEnabled` | Enable/disable standalone MCP (`*bool`, default true) |
| `spec.ols.mcpKubeServerConfig.timeout` | Timeout seconds for the built-in openshift MCP entry in olsconfig |
| `spec.ols.deployment.mcpServer` | Standalone MCP `Config` (replicas, resources, tolerations, nodeSelector) |
| `--openshift-mcp-server-image` | MCP container image override |

## Constraints

1. Multi-replica is allowed; Streamable HTTP is configured for stateless operation upstream.
2. The MCP ServiceAccount has no cluster RBAC; authorization uses the calling user's token.
3. The `openshift-mcp-server` image is shipped by the OCP MCP team from `registry.redhat.io/openshift-mcp/openshift-mcp-server-rhel9`. OLS does not build or release this image. Digest/tag updates track the OCP MCP team's releases; bump `related_images.json` and regenerate the bundle when a new release is available.
4. Agentic/sandbox reuse of the MCP Service URL is published in the handoff ConfigMap; the MCP client CA Secret is owned by appserver when introspection is enabled — see `agentic-sandbox-profile.md`. Optional auto-injection into agent runs remains deferred (OLS-3594).

## Planned Changes

| Ticket | Summary |
|---|---|
| [OLS-4391](https://redhat.atlassian.net/browse/OLS-4391) | Gate default NetObserv enablement on FlowCollector API presence in the OLS operator; retain decisions on discovery errors and react to API installation/removal. See [decision 0001](../decisions/0001-netobserv-api-presence-gating.md). |
| OLS-3594 | Deferred optional agentic auto-injection. |
