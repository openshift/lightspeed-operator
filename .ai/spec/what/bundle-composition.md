# Bundle Composition

The lightspeed-operator OLM bundle installs both the lightspeed-operator controller and the lightspeed-agentic-operator controller. This spec defines the bundle structure, CRD ownership, image references, and the boundaries between the two controllers.

## Behavioral Rules

### Bundle Structure

1. The lightspeed-operator OLM bundle (CSV + manifests in `bundle/`) contains the static resources for both the lightspeed-operator controller and the lightspeed-agentic-operator controller.
2. The CSV defines two deployments: one for the lightspeed-operator controller and one for the lightspeed-agentic-operator controller.
3. Both controllers start when the operator is installed. No feature gate or manual step is required to start either controller process.

### CRD Ownership

4. CRD YAML for `agentic.openshift.io` types is generated in the `lightspeed-agentic-operator` repo (via `make manifests`).
5. The agentic-operator repo remains the single source of truth for `agentic.openshift.io` API types. The lightspeed-operator repo does not define or modify these types.
6. The lightspeed-operator repo has a `make` target that fetches CRD YAML from the `lightspeed-agentic-operator` repo via a git-based fetch at a pinned ref/tag, and copies the CRD files into `bundle/manifests/`.
7. When the agentic CRDs change, the pinned ref is updated in the lightspeed-operator repo and the make target is re-run to sync.

### Image References

8. The lightspeed-operator controller image is specified in its CSV deployment spec (as today).
9. The lightspeed-agentic-operator controller image is specified in its own CSV deployment spec, following the same pattern.
10. Operand images for each controller (console plugins, service images, etc.) are passed via startup flags or environment variables on their respective deployments.

### Controller Independence

11. The two controllers share no runtime state. They reconcile different CRDs (`ols.openshift.io` vs `agentic.openshift.io`).
12. Feature gates on `OLSConfig` (`MCPServer`, `ToolFiltering`) have no effect on the agentic controller.
13. The agentic controller is effectively inert until its CRs are created — it watches for `AgenticOLSConfig`, `Proposal`, and related CRs, but takes no action without them.

### Console Plugins

14. [PLANNED: OLS-3236] The lightspeed-operator deploys both console plugins: the Lightspeed chat console plugin and the agentic console plugin. The agentic-operator does not deploy any console plugins.
15. Prior to OLS-3236, the agentic-operator deployed the agentic console plugin via a fire-and-forget `RunnableFunc`. This is being migrated to the lightspeed-operator for full reconciliation lifecycle management.

### Agentic Operand Deployment

16. The lightspeed-operator reconciles the agentic alerts adapter as a fully managed operand (OLS-3348, opt-in via `spec.ols.deployment.alertsAdapter.configMapRef`): Phase 1/2 reconciliation when enabled, `AlertsAdapterReady` status condition (`NotConfigured` when disabled), health monitoring, operand teardown on disable, ConfigMap watcher restarts, and finalizer cleanup via `RemoveAlertsAdapter()`. The agentic console plugin remains [PLANNED: OLS-3236].
17. Agentic operand images default to `:main` tags until Konflux onboarding provides SHA-pinned productized images. The `--alerts-adapter-image` flag is implemented on the lightspeed-operator binary; wiring it into the CSV deployment spec is [PLANNED: OLS-3236]. The `--agentic-console-image` flag is [PLANNED: OLS-3236].

## Configuration Surface

| Item | Location | Description |
|---|---|---|
| Agentic CRD pinned ref | lightspeed-operator repo (Makefile or config) | Git ref/tag for fetching agentic CRD YAML |
| Agentic controller image | CSV deployment spec | Container image for the agentic controller |
| Agentic controller startup flags | CSV deployment spec args | Operand image overrides for the agentic controller |
| Agentic controller `--sandbox-mode` | CSV deployment spec args | `bare-pod` (default) or `sandbox-claim` — selects sandbox provisioning strategy |
| Agentic controller `--agentic-sandbox-image` | CSV deployment spec args | [PLANNED: OLS-3236] Sandbox container image (default: `:main` tag, overridable) |
| Lightspeed controller `--alerts-adapter-image` | `cmd/main.go` flag (implemented); CSV deployment spec args [PLANNED: OLS-3236] | Alerts adapter container image (default: Konflux `:main` tag) |
| Lightspeed controller `--agentic-console-image` | CSV deployment spec args | [PLANNED: OLS-3236] Agentic console plugin container image (default: `:main` tag) |

## Constraints

1. The lightspeed-operator controller code does not import, reference, or reconcile any `agentic.openshift.io` types.
2. The agentic CRD YAML in `bundle/manifests/` must not be hand-edited — it is synced from the agentic-operator repo via the make target.
3. Both controllers must be able to run in disconnected (air-gapped) environments. All image references must be overridable.

## Planned Changes

| Ticket | Summary |
|---|---|
| OLS-3236 | Migrate agentic console deployment from agentic-operator to lightspeed-operator. Wire `--alerts-adapter-image` and `--agentic-console-image` into lightspeed-operator CSV deployment. Remove `--agentic-console-image` from agentic-operator CSV deployment. |
