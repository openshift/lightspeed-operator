# Bundle Composition

Each release produces **two version-split OLM bundles under one package**
(`lightspeed-operator`, same channels), partitioned by the
`com.redhat.openshift.versions` annotation and gating the agentic layer to OCP ≥ 5.0
(OLS-3899, cross-repo decision `0037-agentic-version-gating.md`):

- **v1 bundle** (`1.x` line, OCP 4.x): classic only. Installs the lightspeed-operator
  controller alone.
- **v2 bundle** (`2.x` line, OCP ≥ 5.0): classic + agentic. Installs both the
  lightspeed-operator controller and the lightspeed-agentic-operator controller.

This spec defines the bundle structure, CRD ownership, image references, and the
boundaries between the two controllers in the v2 bundle. The v1 bundle contains none of
the agentic resources described here.

## Behavioral Rules

### Bundle Structure

1. Both bundles are built from the same source under `bundle/` via selector-driven
   tooling (`hack/update_bundle.sh v1|v2`). The selector chooses the CSV template, the
   image set from `related_images.json`, and the `com.redhat.openshift.versions`
   annotation. The v1 annotation covers `v4.16-v4.22` (current); it must be extended to
   include each new OCP 4.x minor as it ships — [PLANNED: OLS-2991] to `v4.16-v4.23`.
   The v2 annotation is `>=v5.0` — [PLANNED: OLS-2992] first release for OCP 5.0.
   Each new OCP version also requires a matching FBC Konflux Application
   (`ols-fbc-v4-XX` or `ols-fbc-v5-0`) and ReleasePlan entries in
   `konflux-release-data`. See `how/fbc-release.md` for the full process.
2. The **v1 CSV** defines one deployment (the lightspeed-operator controller). The **v2
   CSV** defines two deployments: the lightspeed-operator controller and the
   lightspeed-agentic-operator controller.
3. In the v2 bundle both controllers start when the operator is installed — no feature
   gate or manual step is required to start either controller process. The v1 bundle
   never contains the agentic controller, its CRDs, or its RBAC.
3a. A cluster only ever installs the bundle whose `com.redhat.openshift.versions`
   matches its OCP version, because each per-OCP-version FBC catalog includes only the
   matching bundle. The 4.x → 5.0 upgrade uses `olm.skipRange: ">=1.0.0 <2.0.0"` on the
   v2 channel head, so the 5.x catalog needs only the v2 bundle.

### CRD Ownership

4. CRD YAML for `agentic.openshift.io` types is generated in the `lightspeed-agentic-operator` repo (via `make manifests`).
5. The agentic-operator repo remains the single source of truth for `agentic.openshift.io` API types. The lightspeed-operator repo does not define or modify these types.
6. The lightspeed-operator repo has a `make` target that fetches CRD YAML from the `lightspeed-agentic-operator` repo via a git-based fetch at a pinned ref/tag, and copies the CRD files into `bundle/manifests/`.
7. When the agentic CRDs change, the pinned ref is updated in the lightspeed-operator repo and the make target is re-run to sync.

### Image References

8. The lightspeed-operator controller image is specified in its CSV deployment spec (as today).
9. The lightspeed-agentic-operator controller image is specified in its own CSV deployment spec, following the same pattern.
10. Operand images for each controller (console plugins, service images, etc.) are passed via startup flags or environment variables on their respective deployments.
10a. Each `related_images.json` entry is tagged with a `bundles` field naming the variants it belongs to: shared entries are `["v1","v2"]`; agentic-only entries (`lightspeed-agentic-operator`, `lightspeed-agentic-console-plugin`, `lightspeed-agentic-alerts-adapter`, `lightspeed-agentic-sandbox`) are `["v2"]`. `hack/update_bundle.sh v1|v2` filters entries by this field so only the v2 bundle carries agentic images in its `spec.relatedImages` and deployment args. Entries without the field default to both bundles.

### Controller Independence

11. The two controllers share no runtime state. They reconcile different CRDs (`ols.openshift.io` vs `agentic.openshift.io`).
12. Feature gates on `OLSConfig` (`MCPServer`, `ToolFiltering`) have no effect on the agentic controller.
13. The agentic controller is effectively inert until its CRs are created — it watches for `AgenticOLSConfig`, `AgenticRun`, and related CRs, but takes no action without them.

### Console Plugins

14. The lightspeed-operator deploys both console plugins: the Lightspeed chat console plugin and the agentic console plugin (`internal/controller/agenticconsole/`). The agentic-operator CSV must stop deploying the agentic console plugin ([PLANNED: OLS-3236]) so only the lightspeed-operator owns that operand.
15. Before this migration, the agentic-operator deployed the agentic console plugin via a fire-and-forget `RunnableFunc`. That path is superseded by lightspeed-operator reconciliation (Phase 1/2, `AgenticConsolePluginReady`, finalizer cleanup via `agenticconsole.RemoveAgenticConsole()`).

### Agentic Operand Deployment

16. The lightspeed-operator reconciles the agentic console plugin as a fully managed operand: Phase 1/2 reconciliation, `AgenticConsolePluginReady` status condition, health monitoring, and finalizer cleanup via `RemoveAgenticConsole()`. The lightspeed-operator reconciles the agentic alerts adapter as a fully managed operand (OLS-3348, opt-in via `spec.ols.deployment.alertsAdapter.configMapRef`): Phase 1/2 reconciliation when enabled, `AlertsAdapterReady` status condition (`NotConfigured` when disabled), health monitoring, operand teardown on disable, ConfigMap watcher restarts, and finalizer cleanup via `RemoveAlertsAdapter()`.
17. Operand images default from `related_images.json` (via `GetDefaultImage` in `constants.go`) and are passed to the operator through CSV deployment args defined by `operator_arg` on each `related_images.json` entry. `config/default/deployment-patch.yaml` is generated from that file (`make manifests`); `hack/update_bundle.sh` substitutes digests at bundle time.

## Configuration Surface

| Item | Location | Description |
|---|---|---|
| Agentic CRD pinned ref | lightspeed-operator repo (Makefile or config) | Git ref/tag for fetching agentic CRD YAML |
| Agentic controller image | CSV deployment spec | Container image for the agentic controller |
| Agentic controller startup flags | CSV deployment spec args | Operand image overrides for the agentic controller |
| Agentic controller `--sandbox-mode` | CSV deployment spec args | `bare-pod` (default) or `sandbox-claim` — selects sandbox provisioning strategy (may later follow `spec.agenticOLS.sandboxMode` via handoff ConfigMap) |
| Lightspeed controller `--agentic-sandbox-image` | `cmd/main.go` flag; CSV deployment spec args; `related_images.json` (`lightspeed-agentic-sandbox`) | Sandbox container image embedded in thin handoff PodSpec (OLS-3683) |
| Lightspeed controller `--rhokp-image` | `cmd/main.go` flag; CSV deployment spec args; `related_images.json` (`rhokp`) | RHOKP sidecar image (external product image; digest-pinned) |
| Lightspeed controller `--alerts-adapter-image` | `cmd/main.go` flag; CSV deployment spec args; `related_images.json` (`lightspeed-agentic-alerts-adapter`) | Alerts adapter container image (interim tags until productized) |
| Lightspeed controller `--agentic-console-image` | CSV deployment spec args; `related_images.json` (`lightspeed-agentic-console-plugin`) | Agentic console plugin container image (interim `:main` until productized) |

## Constraints

1. The lightspeed-operator controller code does not import, reference, or reconcile any `agentic.openshift.io` types.
2. The agentic CRD YAML and agentic RBAC (the `agentic-operator-manager-role` rules that become the v2 CSV `clusterPermissions`, and the standalone `agentic-run-approver` ClusterRole/Binding shipped as v2 bundle manifests) in the v2 bundle must not be hand-edited — they are synced from the agentic-operator repo via the make target at a pinned ref. Per-run sandbox RBAC is created at runtime by the agentic controller, bounded to a subset of its own permissions (no `escalate`/`bind`), and is not part of the bundle.
3. Both controllers must be able to run in disconnected (air-gapped) environments. All image references must be overridable.

## Planned Changes

| Ticket | Summary |
|---|---|
| OLS-3236 | Remove agentic console deployment from agentic-operator CSV (lightspeed-operator now reconciles the plugin and wires `--agentic-console-image` / `--alerts-adapter-image` in its CSV). Productize agentic operand images to SHA-pinned `registry.redhat.io` entries. |
| OLS-3899 | Split into two version-gated bundles (v1 classic / v2 full). Add `bundles` tags to `related_images.json`, selector-driven `update_bundle.sh v1\|v2`, two CSV templates, v2-only agentic CRDs/RBAC/operands, and 5.x FBC catalogs. Sync agentic RBAC (`agentic-operator-manager-role`, `agentic-run-approver`) from the agentic-operator repo into the v2 bundle. See decision 0037. |
| OLS-2991 | OCP 4.23 release artifacts — extend v1 bundle annotation to `v4.16-v4.23`; create `ols-fbc-v4-23` Konflux Application; add staging and prod ReleasePlan entries to `konflux-release-data`. |
| OLS-2992 | OCP 5.0 release artifacts — create v2 bundle Konflux Application; create `ols-fbc-v5-0` FBC Konflux Application; add staging and prod ReleasePlan entries to `konflux-release-data`; inaugural release of the full agentic stack. |
