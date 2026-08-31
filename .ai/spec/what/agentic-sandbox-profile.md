# Agentic Integration Handoff

Classic lightspeed-operator publishes cluster objects that lightspeed-agentic-operator consumes for sandbox provisioning and OTEL/MCP connectivity. Sandbox Pod/Claim lifecycle stays in agentic-operator. Stories: [OLS-3683](https://redhat.atlassian.net/browse/OLS-3683) (CRD + image), [OLS-3684](https://redhat.atlassian.net/browse/OLS-3684) (handoff artifacts). Agentic-operator consumption: OLS-3685+.

See also: `templog.md` (collector), `ocpmcp.md` (MCP Service/CA), `rhokp.md` (RHOKP Service/CA), `crd-api.md` (`spec.agenticOLS`), `bundle-composition.md` (dual controller), `app-server.md` (client CA Secrets).

## Behavioral Rules

### Ownership
1. Package `internal/controller/appserver` owns the client-only CA Secrets (`lightspeed-agentic-otel-ca`, `lightspeed-agentic-mcp-ca` when introspection is enabled, and `lightspeed-agentic-rhokp-ca` when OKP is enabled) and mounts them into the app-server Deployment. It copies public PEM from service-ca source ConfigMaps; serving-cert private keys are never published.
2. Package `internal/controller/agenticintegration` owns only the handoff ConfigMap (`lightspeed-agentic-configuration`). It references CA Secret **names** in ConfigMap data; it does not create or refresh those Secrets. It does not manage sandbox Pods, SandboxClaims, or SandboxTemplates.
3. The former OTEL client ConfigMap `lightspeed-otel-collector-client` is no longer created. OTEL endpoints and CA are published only via the handoff ConfigMap + appserver-owned CA Secrets (no dual-write). On upgrade, otelcollector Phase 1 deletes any leftover `lightspeed-otel-collector-client` ConfigMap (`IgnoreNotFound`). Likewise, ocpmcp Phase 1 / `Remove` deletes leftover `openshift-mcp-server-ca`.

### OLSConfig
4. `spec.agenticOLS` is optional. When omitted or `sandboxMode` is empty, sandbox mode is `bare-pod`.
5. `spec.agenticOLS.sandboxMode` is `bare-pod` or `sandbox-claim` (OpenAPI enum).
6. `spec.agenticOLS.agenticSandboxConfig` uses shared `Config` for resources, tolerations, and nodeSelector. Replicas are ignored (sandbox count is managed by agentic-operator).
7. Sandbox container image comes from classic operator `--agentic-sandbox-image` / `related_images.json` entry `lightspeed-agentic-sandbox`, not from the CR.

### Handoff ConfigMap (`lightspeed-agentic-configuration`)
8. Always reconciled **last in Phase 2** (after appserver deployment) so CA Secrets and OTEL/MCP Services exist before the ConfigMap advertises their names/endpoints.
8a. **Create gate (reconcile path only):** first create is skipped (error/`ErrAgenticConfigurationPrerequisitesNotReady`, requeue) until the OTEL Collector Service and OTEL client CA Secret exist, and — when introspection is enabled — the MCP Service and MCP client CA Secret exist. This avoids advertising endpoints/trust refs before infrastructure is present.
8b. **Update path:** if the ConfigMap already exists, reconcile still applies Data updates (`sandbox-mode`, `sandbox-pod-spec`, endpoint/CA keys) even when those prerequisites are temporarily unmet (e.g. OTEL Progressing).
8c. **Watcher path:** `TouchAgenticConfiguration` is ungated (except `RestartAppServer` fail-closed on CA refresh — see Refresh / rotation).
9. Keys always present:
   - `sandbox-mode` — `bare-pod` or `sandbox-claim`
   - `sandbox-pod-spec` — JSON-serialized thin `corev1.PodSpec`
   - `otel-collector-endpoint` — `lightspeed-otel-collector.<ns>.svc:4317`
   - `otel-admin-endpoint` — `https://lightspeed-otel-collector.<ns>.svc:8080`
   - `otel-ca-secret` — name of the OTEL client CA Secret (`lightspeed-agentic-otel-ca`)
10. When `spec.ols.introspectionEnabled` is true (default), also set:
   - `mcp-endpoint` — OpenShift MCP HTTPS Service URL
   - `mcp-ca-secret` — name of the MCP client CA Secret (`lightspeed-agentic-mcp-ca`)
11. When introspection is disabled, MCP keys are omitted. Appserver deletes the MCP client CA Secret when present.
11b. When OKP is enabled (`!spec.ols.byokRAGOnly`, default), also set `rhokp-endpoint` (RHOKP HTTPS Service URL) and `rhokp-ca-secret` (name of the RHOKP client CA Secret, `lightspeed-agentic-rhokp-ca`). When `byokRAGOnly` is true, both keys are omitted and appserver does not publish the RHOKP CA Secret.
11a. [PLANNED: OLS-3491] Optional per-step instruction keys derived from `spec.agenticOLS.instructions`:
   - `instructions-analysis` — cluster default analysis system instructions
   - `instructions-execution` — cluster default execution system instructions
   - `instructions-verification` — cluster default verification system instructions
   - `instructions-escalation` — cluster default escalation system instructions
   On each reconcile, the classic operator MUST replace the complete `instructions-*` key subset: publish a key only when its source string is non-empty; **delete** any existing `instructions-*` key whose source is empty or unset. Stale keys MUST NOT remain after a non-empty→empty/unset transition. Agentic-operator consumes these for create-time materialization (analysis/execution/verification) and for call-time escalation resolution. See agentic-operator `what/sandbox-execution.md` and `what/crd-api.md`.

### Thin sandbox PodSpec
12. PodSpec contains one container (`lightspeed-agentic-sandbox`) with image from `GetAgenticSandboxImage()`, optional resource/toleration/nodeSelector overrides, and writable emptyDirs:
    - `home` → `/home/agent`
    - `skills-workdir` → `/app/skills/.agents`
13. Default resources are requests only: `500m` CPU, `128Mi` memory (no limits; OpenShift / OLS-3397).
14. PodSpec does **not** include OTEL/MCP env vars, CA volume mounts, or TLS Secret mounts. Agentic-operator injects connectivity from the ConfigMap and CA Secrets.

### Client-CA Secrets (appserver)
15. `lightspeed-agentic-otel-ca` — opaque Secret with sole key `otel-ca.crt` (PEM copied from `openshift-service-ca.crt` / `service-ca.crt`). Always reconciled in appserver Phase 2 before Deployment.
16. `lightspeed-agentic-mcp-ca` — opaque Secret with sole key `mcp-ca.crt` (same cluster service-ca PEM as OTEL). Published only when introspection is enabled; deleted when introspection is disabled.
16a. `lightspeed-agentic-rhokp-ca` — opaque Secret with sole key `rhokp-ca.crt` (same cluster service-ca PEM). Published only when OKP is enabled (`!byokRAGOnly`); deleted when `byokRAGOnly` is true.
17. Secrets contain public CA material only — never serving-cert private keys.
18. App-server mounts these Secrets at `/etc/certs/otel-collector-ca/`, `/etc/certs/openshift-mcp-server-ca/`, and `/etc/certs/rhokp-ca/` (projected filename `service-ca.crt` for path compatibility). There is no dedicated MCP or RHOKP inject-cabundle ConfigMap.

### Refresh / rotation
19. Serving-cert watchers restart the server Deployment then the app-server (`lightspeed-app-server`) and touch `lightspeed-agentic-configuration`:
    - OTEL: `lightspeed-otel-collector-cert` → `RestartOtelCollector` + `RestartAppServer` + `TouchAgenticConfiguration`
    - MCP: `openshift-mcp-server-tls` → MCP restart + `RestartAppServer` + `TouchAgenticConfiguration`
    - RHOKP: `lightspeed-rhokp-tls` → RHOKP restart + `RestartAppServer` + `TouchAgenticConfiguration`
    All three targets are declared in `AffectedDeployments` for each secret; the watcher invokes them independently.
20. `RestartAppServer` order: (1) refresh client CA Secrets from `openshift-service-ca.crt` (`RefreshClientCASecrets`), (2) **re-Get** the app-server Deployment (current resourceVersion), apply any caller Spec mutations, bump `force-reload`, Update. **Fail-closed:** if step (1) fails (source CA ConfigMap missing/empty), the app-server roll is skipped so pods are not rolled with stale CA material. Retry happens on a later OLSConfig reconcile or watcher event once the source CA is ready.
21. `TouchAgenticConfiguration` bumps `ols.openshift.io/client-ca-reload` annotation on the handoff ConfigMap so agentic-operator detects the change. It is a separate watcher callback (not part of `RestartAppServer`).
21. `RestartOtelCollector` only rolls the collector; it does **not** refresh agentic artifacts (that work is on the app-server restart path).
22. Agenticintegration ConfigMap reconcile preserves the cert-reload annotation when updating Data/Labels.
23. Content equality skips Secret/ConfigMap updates when Data, Labels, and OwnerReferences are unchanged.

### Agentic-operator contract (classic operator expectations)
24. Agentic-operator reads fixed object names from the cluster (bundle may pass names as flags). Prefer Kubernetes objects over importing `ols.openshift.io` types.
25. Agentic-operator should wait/requeue until the handoff ConfigMap exists and (when needed) collector Service Endpoints / CA Secrets are present. Do not rely on CSV install order between the two controllers.
26. Watch for `ols.openshift.io/client-ca-reload` (or ConfigMap RV) to reload mounted CA PEMs after rotation.
27. Consuming the new ConfigMap/Secrets (and dropping `lightspeed-otel-collector-client`) is agentic-operator work (OLS-3685+).

## Resource Names

| Resource | Name | Owner |
|---|---|---|
| Handoff ConfigMap | `lightspeed-agentic-configuration` | `agenticintegration` |
| OTEL client-CA Secret | `lightspeed-agentic-otel-ca` (`otel-ca.crt`) | `appserver` |
| MCP client-CA Secret | `lightspeed-agentic-mcp-ca` (`mcp-ca.crt`) | `appserver` |
| RHOKP client-CA Secret | `lightspeed-agentic-rhokp-ca` (`rhokp-ca.crt`) | `appserver` |
| Sandbox container (in PodSpec) | `lightspeed-agentic-sandbox` | (embedded in ConfigMap) |

## Configuration Surface

| Field / flag | Description |
|---|---|
| `spec.agenticOLS.sandboxMode` | `bare-pod` (default) or `sandbox-claim` |
| `spec.agenticOLS.agenticSandboxConfig` | Resources / tolerations / nodeSelector for thin PodSpec |
| `spec.agenticOLS.instructions.*` | [PLANNED: OLS-3491] Optional cluster per-step system instructions → ConfigMap `instructions-*` keys |
| `spec.ols.introspectionEnabled` | Gates MCP keys and MCP client CA Secret |
| `--agentic-sandbox-image` | Sandbox container image in thin PodSpec |

## Constraints

1. Classic operator does not create SandboxTemplate or manage sandbox lifecycle.
2. No raw user-editable full PodSpec on OLSConfig.
3. Serving Secrets must not be published for agentic mount (private key risk).
4. OTEL collector remains always deployed; handoff OTEL keys are always present regardless of `spec.audit.logging`.
5. `RestartAppServer` is fail-closed on client CA refresh failure (see Refresh / rotation rule 20).

## Out of Scope

- Agentic-operator wait loop, PodSpec injection, and SandboxTemplate path (OLS-3685+)
- Optional agentic auto-injection of MCP into runs ([OLS-3594](https://redhat.atlassian.net/browse/OLS-3594))
- Defining `agentic.openshift.io` CRD changes

## Cross-References

- `what/templog.md` — collector operand; OTEL connectivity consumed via this handoff
- `what/ocpmcp.md` — MCP Service and CA source for MCP handoff keys
- `what/reconciliation.md` — Phase 2 ordering (appserver then agenticintegration)
- `what/tls.md` — service-ca PEM sources and rotation
- `how/project-structure.md` — `appserver` / `agenticintegration` packages
