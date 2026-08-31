# Spec health report

Last evaluated: 2026-08-31
Trigger: post-milestone: standalone MCP/RHOKP + ServiceMonitors + agentic handoff drift sweep
Layout: software (.ai/spec/)

## Stale
Fixed in this pass (drift confirmed against current code, then corrected under spec-first:init alignment):

- **Shipped work still marked `[PLANNED]`.** ServiceMonitors for MCP (OLS-3728, `openshift-mcp-server-monitor`, path `/metrics`) and RHOKP (OLS-3727, `lightspeed-rhokp-monitor`, path `/solr/admin/metrics`) are implemented (Phase 2 via `utils.ReconcileServiceMonitor`); markers removed from `what/ocpmcp.md`, `what/rhokp.md`, `what/reconciliation.md`, `what/observability.md`. App-server liveness probe (OLS-3221, `/liveness`, failureThreshold 3, periodSeconds 30) is shipped; `[PLANNED]` removed from `what/app-server.md` and `how/deployment-generation.md`.
- **Dead sidecar architecture.** MCP and RHOKP are standalone HTTPS Deployments, not app-server sidecars. Removed `--read-only` "sidecar" language (`what/security.md`), the non-existent `lightspeed-rhokp-ca` inject-cabundle ConfigMap from the RHOKP diagram (`what/rhokp.md` — rhokp owns only Service/NetworkPolicy/ServiceMonitor), RHOKP-sidecar wait/mount steps (`how/deployment-generation.md`), and `solr_http_base: http://localhost:9080` → `https://lightspeed-rhokp.<ns>.svc:8443` (`how/config-generation.md`).
- **Superseded handoff design.** OLS-3572 `lightspeed-sandbox-config` replaced by `lightspeed-agentic-configuration` owned by the `agenticintegration` package; `what/app-server.md` rules 34-36 collapsed to point at `agentic-sandbox-profile.md`. Removed a dangling ADR path reference in `agentic-sandbox-profile.md`.
- **Condition-count drift.** "eight condition types" → nine (added `RHOKPReady`); condition lists in `what/observability.md`, `what/crd-api.md` updated.
- **Reconcile ordering wrong vs code.** Phase 1 and Phase 2 execution order in `what/reconciliation.md` (rule 9), `how/reconciliation.md`, and `how/project-structure.md` did not match `internal/controller/olsconfig_controller.go`. Corrected to actual order — Phase 1: console → postgres → mcp → rhokp → agenticconsole → alertsadapter → otel → appserver; Phase 2: console → postgres → mcp → rhokp → appserver → otel → agenticconsole → alertsadapter → agenticintegration (last, separate call). Rule 12c wrongly claimed OTEL is reconciled "before the app-server"; corrected to after (app-server reaches the collector by Service DNS, so ordering is not a dependency).
- **Duplicate/dangling lines.** Removed a duplicated `alertsadapter.ReconcileAlertsAdapterResources()` line (`how/reconciliation.md`) and a duplicated `TouchAgenticConfiguration()` line (`how/project-structure.md`). Fixed a duplicate rule number in `what/ocpmcp.md` (Finalizer 19→20).
- **Wrong repo reference.** `what/templog.md` pointed at a `lightspeed-service` file that does not exist → corrected to ols repo.

## Missing
- **Module maps thin.** `how/deployment-generation.md` Module Map lacked the standalone operand generators; added verified `deployment.go` entries for agenticconsole/otelcollector/ocpmcp/rhokp/alertsadapter. `what/agentic-sandbox-profile.md` lacked RHOKP handoff keys / CA secret / mount; added.
- **Quick-start / cross-ref gaps.** `README.md` Quick Start and Cross-Reference tables did not list `what/ocpmcp.md` / `what/rhokp.md`; added.

## Structural concerns
None. what/ vs how/ separation is intact; no behavioral rules leaked into how/ files. No file needs splitting or merging.

## Findability issues
None material. The handoff design was split across `app-server.md` and `agentic-sandbox-profile.md`; cross-links were tightened so the app-server spec now delegates to the sandbox-profile spec rather than duplicating rules.

## No issues (verified current, deliberately left unchanged)
- **Genuinely planned, absent from code:** `reasoningConfig` (OLS-3442), instructions (OLS-3491), wait-for-rhokp init container / `GenerateRHOKPWaitInitContainer` (OLS-3799) — markers kept; each confirmed not implemented.
- **Cross-repo / deferred, not this repo's drift:** OLS-3236 (agentic-operator CSV dedupe), OLS-3594 (deferred MCP auto-injection), OLS-3685+ (agentic-operator consumption of the handoff ConfigMap/CA Secrets), OLS-2991/2992 (recent genuine spec work).
- **Repo `CLAUDE.md`** still shows the old Phase 1/2 order (postgres-first, mcp/rhokp after otel). It is now stale vs the corrected specs, but is out of scope for this pass (agent-instruction files require human review) and was not modified.
