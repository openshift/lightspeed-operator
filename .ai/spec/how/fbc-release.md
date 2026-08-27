# FBC Release Process

How OLS operator bundles are published to OCP FBC (File-Based Catalog) indexes via Konflux for each supported OCP version.

## Architecture

Each supported OCP version has its own **FBC Konflux Application** (`ols-fbc-v4-XX` or `ols-fbc-v5-0`). The application holds the FBC fragment that declares the OLS operator entry point in that version's catalog index. ReleasePlan resources in `konflux-release-data` wire each application to the staging and production release pipelines.

The v1 bundle (`1.x` line) is published to OCP 4.x FBC Applications. The v2 bundle (`2.x` line) is published to OCP 5.x FBC Applications via a separate Konflux Application representing the v2 bundle itself.

## Naming Conventions

| Resource | Pattern | Example |
|---|---|---|
| FBC Konflux Application | `ols-fbc-v<major>-<minor>` | `ols-fbc-v4-23`, `ols-fbc-v5-0` |
| Prod ReleasePlan | `ols-fbc-releaseplan-prod-v<major>-<minor>` | `ols-fbc-releaseplan-prod-v4-23` |
| Staging ReleasePlan | `ols-fbc-releaseplan-staging-v<major>-<minor>` | `ols-fbc-releaseplan-staging-v4-23` |
| Bundle Konflux Application (v1) | `ols-bundle` | — |
| Bundle Konflux Application (v2) | `ols-bundle-v2` | — |

## Files in `konflux-release-data`

All ReleasePlan resources for the `crt-nshift-lightspeed-tenant` namespace live under:

```
tenants-config/cluster/stone-prd-rh01/tenants/crt-nshift-lightspeed-tenant/
  release-plan-fbc-prod.yaml     # prod ReleasePlan per OCP 4.x version (v1 bundle)
  release-plan-fbc-staging.yaml  # staging ReleasePlan per OCP 4.x version (v1 bundle)
  release-plan-bundle.yaml       # bundle-level release (v1)
```

For OCP 5.x (v2 bundle), analogous files are created in the same directory:

```
  release-plan-fbc-v2-prod.yaml     # prod ReleasePlan per OCP 5.x version (v2 bundle)
  release-plan-fbc-v2-staging.yaml  # staging ReleasePlan per OCP 5.x version (v2 bundle)
  release-plan-bundle-v2.yaml       # bundle-level release (v2)
```

## Adding a New OCP 4.x Version (e.g., 4.23)

1. **Extend the v1 bundle annotation** in `bundle/metadata/annotations.yaml`:
   ```
   com.redhat.openshift.versions: v4.16-v4.23
   ```

2. **Create the FBC Konflux Application** `ols-fbc-v4-23` in the Konflux UI/API under the `crt-nshift-lightspeed-tenant` namespace.

3. **Add ReleasePlan entries** to `konflux-release-data`:
   - In `release-plan-fbc-prod.yaml` — add a new `---`-delimited document with:
     - `name: ols-fbc-releaseplan-prod-v4-23`
     - `spec.application: ols-fbc-v4-23`
     - `auto-release: "false"`, admission `ols-fbc-prod-index`
   - In `release-plan-fbc-staging.yaml` — add the matching document with:
     - `name: ols-fbc-releaseplan-staging-v4-23`
     - `auto-release: "true"`, admission `ols-fbc-staging-index`

4. **Open a PR** against `konflux-release-data` with the ReleasePlan additions. The FBC Application creation is a separate Konflux-side step (not a PR to this repo).

## Adding a New OCP 5.x Version (inaugural: 5.0) — [PLANNED: OLS-2992]

OCP 5.0 is the first release using the v2 bundle and requires additional steps:

1. **Create the v2 bundle Konflux Application** (`ols-bundle-v2`) and configure it to build the v2 bundle variant via `hack/update_bundle.sh v2`.

2. **Create the FBC Konflux Application** `ols-fbc-v5-0` in the `crt-nshift-lightspeed-tenant` namespace.

3. **Add ReleasePlan entries** in new files `release-plan-fbc-v2-prod.yaml` and `release-plan-fbc-v2-staging.yaml` following the same per-version document pattern, referencing the v2 bundle admissions.

4. **Add bundle-level ReleasePlan** `release-plan-bundle-v2.yaml` for the v2 bundle image.

5. **Verify the `olm.skipRange`** on the v2 channel head covers `">=1.0.0 <2.0.0"` so a cluster upgrading from OCP 4.x to 5.0 transitions from the v1 to v2 bundle automatically.

## Constraints

- The v1 and v2 bundle Konflux Applications are independent — shared component images are cross-referenced by digest.
- Staging ReleasePlans set `auto-release: "true"` (triggered on every build); prod ReleasePlans set `auto-release: "false"` (triggered manually per advisory).
- The `standing-attribution: "true"` label and `target: rhtap-releng-tenant` are required on all FBC ReleasePlans.
