# 0002: One OLM Bundle with Runtime Agentic Gating

- **Status:** Accepted design; cross-version release migration subject to validation.
- **Jira:** [OLS-4007](https://redhat.atlassian.net/browse/OLS-4007), OLS-4348–OLS-4352.
- **Behavioral contract:** [bundle composition](../what/bundle-composition.md), [agentic console](../what/agentic-console-ui.md), [FBC release process](../how/fbc-release.md).

## Context

The earlier OLS-3899 design split Classic-only 1.x and Classic + Agentic 2.x bundles by OCP version. The current bundle generator and CSV on upstream main instead generate one full artifact for the `lightspeed-operator` package with both controllers, an OCP compatibility range starting at 4.16, and a combined image set. The old two-bundle spec no longer describes that source. A single bundle does not provide OCP-version isolation by omitting Agentic CRDs/RBAC on 4.x; safety and opt-in therefore require runtime gates.

## Decision

Build and release one unified bundle and distribute the same released digest through the per-OCP-version FBC catalogs. The Classic-only 1.x bundle remains a historical migration source until explicitly retired; do not maintain two new bundle variants or bundle release Applications. A bundle install provides both controller processes and static Agentic infrastructure on 4.x and 5.x. It does not enable AgenticRun workflow execution. On OCP 4.x, Classic behavior remains available and normal Agentic operand reconciliation is Disabled; on a matching completed OCP 5.0+ version configured Classic-side Agentic console/adapter operands may reconcile without checking for `AgenticOLSConfig`. Workflow execution separately requires explicit creation of an unsuspended `AgenticOLSConfig` and an independently validated agentic-controller gate. Unreadable or incomplete version state is Unknown and must not be treated as Disabled or allow Agentic mutations. Explicit OLSConfig deletion cleanup is not blocked by the version gate.

Before release cutover and v1 retirement, validate cross-version install/upgrade/downgrade, AgenticRun disable/cleanup, catalog compatibility, digest equality, and migration/rollback instructions (OLS-4348–OLS-4352). The Agentic controller's own workflow gating must also be validated; gating Classic operands alone does not protect AgenticRun reconciliation.

## Alternatives

- **Retain version-split v1/v2 bundles and release streams:** rejected for the unified release because separate artifacts duplicate packaging, catalog membership, and migration complexity.
- **Trust catalog selection or OCP desired version to activate Agentic:** rejected because the bundle is shared across OCP versions and desired version can advance before an upgrade completes.
- **Enable AgenticRun execution solely when OCP 5.0 is reached:** rejected because an explicitly created, unsuspended `AgenticOLSConfig` is required.

## Consequences

- Agentic CRDs, static RBAC, webhook infrastructure, and the controller are installed on supported OCP 4.x clusters even though Agentic workloads must remain inactive.
- Runtime version/opt-in checks and cleanup become release-critical. A generated bundle alone is not proof of a safe downgrade or a validated release migration.
- Catalogs remain version-specific, but must consume the same unified bundle digest after cutover.
