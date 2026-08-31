# RHOKP (Red Hat Offline Knowledge Portal)

Standalone HTTPS RHOKP operand managed by the `rhokp` package ([OLS-3697](https://redhat.atlassian.net/browse/OLS-3697)). Replaces the former app-server sidecar. Follows the ocp-mcp standalone HTTPS pattern ([OLS-3526](https://redhat.atlassian.net/browse/OLS-3526)). Related: [OLS-3572](https://redhat.atlassian.net/browse/OLS-3572) (agentic handoff), [OLS-1894](https://redhat.atlassian.net/browse/OLS-1894) (ROSA-aware OKP).

## Architecture

```
lightspeed-service (app-server)
  └─ HTTPS Solr client
       url: https://lightspeed-rhokp.<ns>.svc:8443/solr/portal-rag/hybrid-search
       trust: /etc/certs/rhokp-ca/service-ca.crt  (extra_ca, from Secret lightspeed-agentic-rhokp-ca)
            │
            ▼
lightspeed-rhokp Deployment + ClusterIP Service (:8443)
  ├─ service-ca serving cert Secret  lightspeed-rhokp-tls
  └─ NetworkPolicy                   lightspeed-rhokp
```

Gated by `!spec.ols.byokRAGOnly` (default: OKP enabled). When `byokRAGOnly` is true, the operator removes managed RHOKP resources and omits `solr_hybrid` config.

## Behavioral Rules

### Activation
1. When `spec.ols.byokRAGOnly` is false (or absent), Phase 1 and Phase 2 reconcile the standalone RHOKP operand.
2. When true, Phase 1 calls `rhokp.Remove()`; Phase 2 skips deployment reconciliation. The status condition `RHOKPReady=False, Reason=Disabled` is emitted to signal that RHOKP is intentionally off.

### Phase 1 Resources
3. NetworkPolicy `lightspeed-rhokp` — ingress from any pod in the operator namespace on TCP `:8443`. (Client trust is provided by the appserver-owned Secret `lightspeed-agentic-rhokp-ca`, not an inject-cabundle ConfigMap — see rule 16 and `agentic-sandbox-profile.md`.)

### Phase 2 Resources
5. Service `lightspeed-rhokp` — ClusterIP, port `https` `:8443`, serving-cert annotation → Secret `lightspeed-rhokp-tls`.
6. Wait for TLS Secret keys `tls.crt` / `tls.key` before creating/updating the Deployment.
7. Deployment `lightspeed-rhokp` — RHOKP image with Apache HTTPS on port 8443 using service-ca cert. Single replica (operator forces 1). Image from `--rhokp-image` / `related_images.json` entry `rhokp`, `PullIfNotPresent`. Replicas/resources/tolerations/nodeSelector from `spec.ols.deployment.rhokp` (`Config`).

### Deployment Spec
8. Container name: `rhokp`.
9. No port remapping — standalone mode uses Apache native port 8443 for HTTPS. The sidecar-era sed-patching of Apache config is removed.
10. Storage: EmptyDir volume with `sizeLimit: 75Gi`. The Solr corpus is baked into the image; EmptyDir provides explicit quota.
11. Environment: optional `ACCESS_KEY` from Secret `rhokp-access-key` (same as sidecar).
12. Security context: restricted PSS except `readOnlyRootFilesystem: false` (Solr/httpd writes at startup).
13. Startup probe: HTTPS GET `/solr/portal-rag/admin/ping` on port 8443. Tolerates ~6 min cold start (large corpus load). Readiness and liveness probes use the same endpoint.
14. Resource defaults: 2 CPU, 2 GiB memory requests (no limits), per OpenShift resource conventions.

### App-server Integration
15. `olsconfig.yaml` `solr_hybrid.solr_http_base` is set to `https://lightspeed-rhokp.<namespace>.svc:8443` (replaces former `http://localhost:9080`).
16. App-server mounts Secret `lightspeed-agentic-rhokp-ca` at `/etc/certs/rhokp-ca/` and adds `service-ca.crt` to `extra_ca`. See `tls.md`.
17. Client CA Secrets for RHOKP are refreshed via the table-driven `RefreshClientCASecrets` in `RestartAppServer`. No hash annotation is stored on the app-server Deployment.

### Monitoring
18. ServiceMonitor `lightspeed-rhokp-monitor` (OLS-3727) — scrapes RHOKP Solr metrics via HTTPS on port 8443, path `/solr/admin/metrics` (Solr built-in Prometheus metrics reporter). Server TLS only (service-ca CA bundle + `serverName`), 30s interval. Reconciled in Phase 2 via `utils.ReconcileServiceMonitor()`. Skipped if Prometheus Operator CRDs are not installed.

### Agentic Handoff
19. When OKP is enabled, the inter-operator handoff ConfigMap (`lightspeed-agentic-configuration`) includes `rhokp-endpoint` and `rhokp-ca-secret` keys. When `byokRAGOnly` is true, both are absent.

### Watching and Restarts
20. Secret `lightspeed-rhokp-tls` is watched via the operator's watcher infrastructure (same pattern as `openshift-mcp-server-tls`).
21. On TLS Secret data change, the watcher restarts `lightspeed-rhokp`, `lightspeed-app-server` (app-server), and touches the `lightspeed-agentic-configuration` ConfigMap.
22. RHOKP Deployment tracks TLS Secret ResourceVersion and rolls when it changes.

### Finalizer
23. On CR deletion, `rhokp.Remove()` deletes Deployment, Service, NetworkPolicy, TLS Secret (`lightspeed-rhokp-tls`), and ServiceMonitor (`lightspeed-rhokp-monitor`) before owned-resource sweep.

## Configuration Surface

| Field path | Description |
|---|---|
| `spec.ols.byokRAGOnly` | When true, disable OKP: no standalone RHOKP, no `solr_hybrid` config |
| `spec.ols.deployment.rhokp` | Standalone RHOKP `Config` (replicas, resources, tolerations, nodeSelector) |
| `--rhokp-image` | RHOKP container image override |

## Constraints

1. Single replica only — the RHOKP corpus is ephemeral per-pod; multi-replica would multiply storage usage with no shared benefit. Operator forces replicas to 1.
2. The ~75 GiB EmptyDir sizeLimit is not user-configurable. It is determined by the RHOKP image corpus size.
3. Apache must be configured to use the service-ca cert for HTTPS on port 8443. Implementation may inject cert paths via environment variables or Apache config overrides, depending on OKP team guidance.
4. Bundle/CSV/related_images updates for digests are a separate release step from the operator cutover PR.

## Planned Changes

None.
