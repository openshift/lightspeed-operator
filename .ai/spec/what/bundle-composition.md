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

14. Each controller deploys its own, separate console plugin. The lightspeed-operator deploys the Lightspeed chat console plugin. The lightspeed-agentic-operator deploys the agentic console plugin.
15. The lightspeed-operator never deploys the agentic console plugin, and vice versa.

## Configuration Surface

| Item | Location | Description |
|---|---|---|
| Agentic CRD pinned ref | lightspeed-operator repo (Makefile or config) | Git ref/tag for fetching agentic CRD YAML |
| Agentic controller image | CSV deployment spec | Container image for the agentic controller |
| Agentic controller startup flags | CSV deployment spec args | Operand image overrides for the agentic controller |

## Constraints

1. The lightspeed-operator controller code does not import, reference, or reconcile any `agentic.openshift.io` types.
2. The agentic CRD YAML in `bundle/manifests/` must not be hand-edited — it is synced from the agentic-operator repo via the make target.
3. Both controllers must be able to run in disconnected (air-gapped) environments. All image references must be overridable.
