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
5. NetworkPolicy `openshift-mcp-server` — ingress from any pod in the operator namespace on TCP `:8443`.

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
13. On TLS Secret data change, the watcher restarts both `openshift-mcp-server` and `ACTIVE_BACKEND` (app-server). `RestartAppServer` refreshes client CA Secrets from `openshift-service-ca.crt` and touches the handoff ConfigMap (fail-closed if CA refresh fails — see `agentic-sandbox-profile.md`).
14. ConfigMap `openshift-service-ca.crt` changes also restart `ACTIVE_BACKEND`, refreshing both OTEL and MCP client CA Secrets.
15. MCP Deployment also tracks ConfigMap and TLS Secret ResourceVersions and rolls when they change.

### Security
16. TOML denies `core/v1` `Secret` and all `rbac.authorization.k8s.io/v1` resources so Secret/RBAC data cannot reach the LLM via the shipped server.
17. Toolsets are pinned to `core`, `config`, `helm`, `metrics`. Metrics uses in-cluster Thanos Querier and Alertmanager URLs. Metrics `guardrails = "!tsdb"` (PromQL query safety, not RBAC) follows upstream OpenShift guidance when Thanos lacks the TSDB status API; auth remains the caller's bearer token.
18. User-defined MCP servers (`spec.mcpServers`) are out of scope for this operand.

### Finalizer
19. On CR deletion, `ocpmcp.Remove()` deletes Deployment, Service, NetworkPolicy, ConfigMap, ServiceAccount, and TLS Secret (`openshift-mcp-server-tls`) before owned-resource sweep.

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
3. Bundle/CSV/related_images updates for digests are a separate release step from the operator cutover PR.
4. Agentic/sandbox reuse of the MCP Service URL is published in the handoff ConfigMap; the MCP client CA Secret is owned by appserver when introspection is enabled — see `agentic-sandbox-profile.md`. Optional auto-injection into agent runs remains deferred (OLS-3594).

## Planned Changes

None for the standalone HTTPS cutover itself. Optional agentic auto-injection remains planned (OLS-3594).
