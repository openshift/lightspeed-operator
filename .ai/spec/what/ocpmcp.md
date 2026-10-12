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
17. Base toolsets are pinned to `core`, `config`, `helm`, `observability/metrics`, `kubevirt`. [PLANNED: OLS-4391] See rules 21–28 for NetObserv configuration and filtering. Metrics uses in-cluster Thanos Querier and Alertmanager URLs. Metrics `guardrails = "!tsdb"` (PromQL query safety, not RBAC) follows upstream OpenShift guidance when Thanos lacks the TSDB status API; auth remains the caller's bearer token.
18. User-defined MCP servers (`spec.mcpServers`) are out of scope for this operand.

### Monitoring
19. ServiceMonitor `openshift-mcp-server-monitor` (OLS-3728) — scrapes MCP server metrics via HTTPS on port 8443, path `/metrics` (Go promhttp). Server TLS only (service-ca CA bundle + `serverName`; no client certs / Bearer token), 30s interval. Reconciled in Phase 2 via `utils.ReconcileServiceMonitor()`. Skipped if Prometheus Operator CRDs are not installed. The NetworkPolicy ingress in rule 5 admits the cluster Prometheus scrape (OLS-3943); the ServiceMonitor alone does not grant network access.

### Finalizer
20. On CR deletion, `ocpmcp.Remove()` deletes Deployment, Service, NetworkPolicy, ConfigMap, ServiceAccount, TLS Secret (`openshift-mcp-server-tls`), and ServiceMonitor (`openshift-mcp-server-monitor`) before owned-resource sweep.

### NetObserv Compatibility Filtering in the OCP MCP server [PLANNED: OLS-4391]
21. With introspection enabled, default TOML contains the base toolsets from rule 17 plus `netobserv` exactly once from OCP MCP server startup, independent of API presence. Keep `experimental_enable_target_compatibility_tool_filters = true`; base toolset configuration and its existing compatibility behavior remain unchanged.
22. The OCP MCP server owns advertised tool visibility. With compatibility filtering enabled, it advertises `netobserv_list_flows`, `netobserv_get_flow_metrics`, and `netobserv_export_flows` when at least one target exposes kind `FlowCollector` in any served version of `flows.netobserv.io`. Confirmed absence across all targets hides them. The predicate checks kind discovery, not resource-name spelling or FlowCollector instances. An explicit nonblank `[toolset_configs.netobserv].url` bypasses this check; disabling compatibility filtering also leaves tools visible. OLS generates neither override.
23. Discovery follows the OCP MCP server's fail-open policy: permission errors, timeouts, relevant partial-discovery failures, and missing inspector/group-discovery support keep tools available. An unrelated stale API group alone does not enable NetObserv. Discovery errors do not block operator reconciliation or alter OCP MCP server readiness.
24. The operator adds no NetObserv-specific API discovery, CRD watches, retries, or RBAC bindings, and does not inspect NetObserv Deployments/OLM resources or probe backend readiness. It generates no new OLSConfig fields or NetObserv-specific TOML section; endpoint defaults belong to the OCP MCP server. FlowCollector auto-configuration, backend provisioning, network-policy changes, TLS relaxation, and caller-permission grants are outside scope.
25. The OCP MCP server's cluster-state polling invalidates discovery caches and triggers tool re-evaluation when API-group names change. Configuration reloads also re-evaluate tools, but do not themselves guarantee fresh discovery. Resource/served-version-only changes within an unchanged group do not trigger the cluster-state callback; re-evaluation after discovery refresh or an OCP MCP server restart may be needed. Refresh is not immediate and uses no periodic operator reconciliation.
26. Runtime API-presence changes affect only OCP MCP server tool visibility, not the generated TOML; they cause no operator ConfigMap writes or Deployment rollouts. Introspection disablement follows rule 2 and takes precedence over filtering.
27. API presence is an installation proxy, not proof of a healthy or usable NetObserv backend; retained CRDs can yield false positives. Successful calls still require plugin connectivity, service-CA trust, caller authorization, and the appropriate flow/metrics backends.
28. Before enabling this configuration in a release, select an OCP MCP server image containing both the NetObserv toolset and its compatibility filter, and verify the acceptance scenarios below. Image pinning and bundle regeneration follow constraint 3. Administrator-configurable toolsets remain separate work under OLS-2715; agentic auto-injection remains deferred under OLS-3594.

## Acceptance Coverage [PLANNED: OLS-4391]

| Scenario | Required result |
|---|---|
| Initial config, with or without FlowCollector API | Default toolsets include `netobserv` exactly once; compatibility filtering is enabled (rule 21). |
| FlowCollector kind present in any served version on any target | Advertise all three NetObserv tools. |
| Successful discovery confirms FlowCollector absence on all targets | Hide all three tools; base tools remain usable. |
| API present but no FlowCollector instance | Advertise tools without reading instances. |
| API group installed/removed while the OCP MCP server runs | Refresh visibility through the OCP MCP server's cluster-state path (rule 25). |
| Resource/served-version change within an unchanged API group | Re-evaluate after discovery refresh; do not assume an automatic callback. |
| Permission, timeout, or relevant partial-discovery error, including after restart | Keep tools available; operator readiness is unaffected. |
| Unrelated stale group with confirmed FlowCollector absence | Hide tools. |
| Inspector/group-discovery support unavailable | Keep tools available. |
| Repeated reconciliation or runtime API-presence change | No identical-config rewrite or detection-only rollout (rule 26). |
| Introspection disabled | Existing removal/`NotConfigured` behavior applies (rule 2). |
| Backend absent/unreachable despite API presence | The OCP MCP server starts and base tools work; NetObserv calls may return backend errors. |
| Security and scope regression | Preserve Secret/RBAC denials and caller-token authorization; verify rule 24's exclusions. |

Operator tests cover default TOML, security regressions, idempotence, and introspection disablement. OCP MCP server tests cover discovery/filtering outcomes, multi-target aggregation, explicit-URL bypass, and reloads. Validate the selected shipped image on OpenShift with `tools/list`, runtime API-group changes, and base-tool calls when the plugin is absent.

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
| [OLS-4391](https://redhat.atlassian.net/browse/OLS-4391) | NetObserv defaults and OCP MCP server compatibility filtering (rules 21–28); rationale in [decision 0001](../decisions/0001-netobserv-api-presence-gating.md). |
| OLS-3594 | Deferred optional agentic auto-injection. |
