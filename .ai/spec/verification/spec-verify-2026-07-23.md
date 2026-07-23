# Verification Report: lightspeed-operator Spec
Verified: 2026-07-23
Spec root: /Users/xavi/street/github.com/AI/ols/lightspeed-operator/.ai/spec/

## Summary
- 1 broken or inaccurate internal reference
- 4 internal inconsistencies
- 1 completeness gap
- 3 cross-repo alignment issues

## Reference Issues

1. **`how/reconciliation.md` Phase 1 diagram omits `ocpmcp.ReconcileResources()`.**
The data flow diagram (lines 21-28) lists Phase 1 steps: console, agentic console, postgres, otel collector, appserver, alerts adapter — missing `ocpmcp.ReconcileResources()` (the standalone MCP server Phase 1). `how/project-structure.md:87` and `what/reconciliation.md:9` (rule 9) both include it. The Phase 2 diagram (lines 96-97) correctly includes `ocpmcp.ReconcileDeployment()`.

## Internal Inconsistencies

1. **Liveness probe values contradict between `observability.md` and `app-server.md`.**
`observability.md:16` (rule 6) states `failureThreshold: 15`. `app-server.md:54` (rule 26) states `failureThreshold: 3, periodSeconds: 30`. `how/deployment-generation.md:37` agrees with `observability.md` (15). Rule 26 in `app-server.md` is tagged `[CHANGED: OLS-3221]` but OLS-3221 also appears in Planned Changes — likely a planned change written as if already implemented.

2. **`toolBudgetRatio` default value inconsistency.**
`crd-api.md:107` states default `0.5`. `crd-api.md:370` states default `0.25`. `how/config-generation.md:32` states "default 0.25 if zero". The values 0.5 and 0.25 cannot both be the default; 0.5 is also the Maximum validation value, making it a suspicious default.

3. **`observability.md` condition types list is stale.**
Rule 11 says "four condition types: `ApiReady`, `CacheReady`, `ConsolePluginReady`, `ResourceReconciliation`." Missing: `AgenticConsolePluginReady`, `OtelCollectorReady`, `MCPServerReady`, `AlertsAdapterReady` — all documented in `reconciliation.md:49` and the crd-api.md status section.

4. **Postgres NetworkPolicy description inconsistency between `postgres.md` and `security.md`.**
`postgres.md:36` (rule 19) says the NP "allows ingress only from pods matching the application server labels **and the OTel Collector labels**". `security.md:17` says "allows only backend pods (matched by `app.kubernetes.io/name: lightspeed-service-api`)" — omitting the OTel Collector. `postgres.md` is more current (OTel Collector needs Postgres access for the templogs pipeline).

## Completeness Gaps

1. **No `constraints.md` or `glossary.md`.**
Given the complexity (15 what/ specs, 4 how/ specs), a glossary defining shared terms (e.g., "operand", "Phase 1 vs Phase 2", "system resource vs external resource", `ACTIVE_BACKEND`) would reduce ambiguity.

## Cross-Repo Alignment Issues

1. **Parent spec `deployment-lifecycle.md` still describes MCP as a sidecar.**
Line 33 says "OpenShift MCP server sidecar (if introspection enabled)". The operator spec (`ocpmcp.md`, `app-server.md:4`, `system-overview.md`) is clear the MCP server is now a **standalone Deployment/Service**, not a sidecar. The parent spec has not been updated after the sidecar-to-standalone cutover (OLS-3526).

2. **Parent spec `deployment-lifecycle.md` lists stale console image flags.**
Line 85 lists `--console-image-pf5` and `--console-image-4-19` as operator image overrides. The operator spec (`system-overview.md:61`) only lists `--console-image` — old PF5/4-19 flags were removed. `how/project-structure.md:57` still says "Select console image: if minor < 19 → PF5, else → PF6", contradicting `system-overview.md:46` ("The operator no longer chooses among multiple console images by OCP minor version").

3. **Parent spec `deployment-lifecycle.md` missing `OtelCollectorReady` and `MCPServerReady` conditions.**
Rule 15 lists conditions: `ApiReady`, `CacheReady`, `ConsolePluginReady`, `AlertsAdapterReady`, `AgenticConsolePluginReady`, `ResourceReconciliation` — missing `OtelCollectorReady` and `MCPServerReady`, both actively used by the operator (`reconciliation.md:49`).

## Files Checked

### what/ (15 files)
- system-overview.md, reconciliation.md, app-server.md, console-ui.md, agentic-console-ui.md, crd-api.md, postgres.md, audit-logging.md, tls.md, security.md, observability.md, resource-lifecycle.md, bundle-composition.md, ocpmcp.md, templog.md

### how/ (4 files)
- reconciliation.md, project-structure.md, config-generation.md, deployment-generation.md

### Other
- health-report.md, decisions/README.md (no ADRs), README.md
- No constraints.md or glossary.md

### Cross-repo
- /Users/xavi/street/github.com/AI/ols/.ai/spec/what/deployment-lifecycle.md (3 alignment issues)
