# Spec health report

Last evaluated: 2026-07-23
Trigger: OLS-3683/3684 client-CA ownership lock (appserver Secrets + agenticintegration ConfigMap/touch)

## Status: Updated ✓

### Updates

**Agentic handoff specs** ✓
- Rewrote `what/agentic-sandbox-profile.md` for appserver-owned client CA Secrets and ConfigMap-only `agenticintegration`
- Documented `RestartAppServer` refresh+touch (`ols.openshift.io/client-ca-reload`); removed stale `RefreshAgenticIntegration` / Phase 1 handoff claims
- Updated `tls.md`, `templog.md`, `reconciliation.md`, `how/project-structure.md`, `crd-api.md`, `ocpmcp.md`, `app-server.md`, `AGENTS.md` / `CLAUDE.md`
- Removed `lightspeed-otel-collector-client` from current design docs

## Verification

✓ No remaining behavioral references to `RefreshAgenticIntegration` as current design
✓ Client CA Secrets owned by appserver; handoff ConfigMap owned by agenticintegration
✓ OLS-3683/3684 marked done in crd-api; OLS-3594 / OLS-3685+ remain planned
✓ Cross-refs to `agentic-sandbox-profile.md` from templog, tls, ocpmcp, reconciliation, README
