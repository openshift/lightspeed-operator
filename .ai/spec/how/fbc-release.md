# FBC Release Process

How the **single unified** OLS bundle is published to OCP File-Based Catalog (FBC) indexes through Konflux. See [bundle composition](../what/bundle-composition.md) and [decision 0002](../decisions/0002-unified-bundle-runtime-gating.md) for the runtime contract.

## Architecture

Each cataloged OCP version has its own FBC Konflux Application (`ols-fbc-v4-XX` or `ols-fbc-v5-0`) and staging/production ReleasePlans in `konflux-release-data`; onboarding a new supported version also requires its catalog fragment and release-path wiring. These are **catalog targets**, not separate Classic and Agentic bundle variants. Existing catalog history may still contain Classic-only entries until migration and cutover are validated. `ols-bundle` is the sole bundle Konflux Application; the bundle is built once with the complete Classic + Agentic CSV, CRDs, RBAC, and related images. The release catalog update path (`.tekton/release/catalog-update/pipeline.yaml`) updates each supported catalog from the same released bundle snapshot. [PLANNED: OLS-4352] Validate that all published catalog entries resolve to the identical bundle digest before cutover.

The historical Classic-only 1.x bundle may remain in older catalog history for upgrade/migration. Do not generate or publish a new v1 bundle in parallel with the unified release, or create an `ols-bundle-v2` Application. The Classic-side runtime gate currently blocks its Agentic operand reconciliation on OCP 4.x; [PLANNED: OLS-4350] an independent agentic-controller workflow gate must be validated before claiming AgenticRun execution is blocked there. The FBC Application and bundle annotation alone do not enforce either runtime boundary.

## Naming Conventions

| Resource | Pattern | Example |
| --- | --- | --- |
| FBC Konflux Application | `ols-fbc-v<major>-<minor>` | `ols-fbc-v4-23`, `ols-fbc-v5-0` |
| Prod ReleasePlan | `ols-fbc-releaseplan-prod-v<major>-<minor>` | `ols-fbc-releaseplan-prod-v4-23` |
| Staging ReleasePlan | `ols-fbc-releaseplan-staging-v<major>-<minor>` | `ols-fbc-releaseplan-staging-v4-23` |
| Bundle Konflux Application | `ols-bundle` | One Application across supported versions |

## Adding a Supported OCP Version

1. Verify the unified `bundle/metadata/annotations.yaml` `com.redhat.openshift.versions` range includes the new version. The current generator in `hack/update_bundle.sh` uses `>=v4.16`; do not introduce a 4.x/5.x bundle selector or 5.x-only range. Validate compatibility on the new target version, not just the annotation.
2. Create or configure the corresponding per-version FBC Konflux Application and catalog fragment (`lightspeed-catalog-<version>/index.yaml`). Add that catalog to the release catalog-update path if not already listed.
3. Add staging (`release.appstudio.openshift.io/auto-release: "true"`) and production (`release.appstudio.openshift.io/auto-release: "false"`) FBC ReleasePlans to `konflux-release-data` with the correct catalog admissions, `release.appstudio.openshift.io/standing-attribution: "true"` label, and `spec.target: rhtap-releng-tenant`. Use the existing bundle release plan for `ols-bundle`; do not add a second bundle-level release stream.
4. [PLANNED: OLS-4351, OLS-4352] Before publishing, validate the catalog on the target OCP release, install/upgrade from supported predecessors, explicit Agentic enablement and cleanup, and the shared released bundle digest across catalogs. A generated CSV or local validation alone does not establish safe rollback/downgrade or permission to retire Classic v1.

## Constraints

- All supported per-version catalogs for a unified release must consume the same bundle digest after release cutover; different FBC Applications do not imply different bundle artifacts.
- Keep the Classic-only v1 migration/retirement decision separate from the new unified bundle publication. Approval criteria are tracked by OLS-4348; do not claim v1 retirement solely on FBC generation.
- ReleasePlan file naming and admissions must be confirmed against the live `konflux-release-data` configuration when adding a version; this spec defines the invariant, not a new set of v2-only files.
