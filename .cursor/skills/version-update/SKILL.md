---
name: version-update
description: >-
  Release workflow for main: refresh related_images.json with stable Konflux
  images (-r stable), bump bundle/CSV version, regenerate and validate the
  bundle. Equivalent to /update-bundle release X.Y.Z. For PR/CI at the current
  version, use /update-bundle dev instead.
disable-model-invocation: true
---

# Version Update (Release)

Invoke with `/version-update X.Y.Z` or `/update-bundle release X.Y.Z`.

Not for PR/CI — use `/update-bundle dev`.

## Step 1: Refresh `related_images.json` (stable)

Requires Konflux `oc login` in namespace `crt-nshift-lightspeed-tenant`. See `README.md` (Update Bundle from Snapshot).

```bash
./hack/snapshot_to_image_list.sh -s <ols-snapshot> -b <ols-bundle-snapshot> -r stable -o related_images.json
```

Use `-r preview` only for tech-preview paths (`registry.redhat.io/openshift-lightspeed-tech-preview/...`).

Images must land on **`registry.redhat.io/openshift-lightspeed/...`** (not Konflux quay).

Preserve snapshot-less entries per `hack/snapshot_to_image_list.sh` (e.g. dataverse-exporter, interim operands not in Konflux). README documents backing up `lightspeed-operator-bundle` when only operand images change.

If CRD/RBAC changed since last bundle, run `make manifests` before regenerating the bundle (Step 4).

---

Update the version in **two** files. They must all match.

## Critical Rules

- The CSV `name` field includes a `v` prefix (e.g., `lightspeed-operator.v1.0.8`)
- The CSV `version` field does NOT have a prefix (e.g., `1.0.8`)
- Both files MUST have matching version numbers
- Always regenerate the bundle after version changes

## Files to Update

**1. `bundle.Dockerfile`** — Bundle container labels (lines 63 and 66)

```dockerfile
LABEL release=X.Y.Z
# ...
LABEL version=X.Y.Z
```

**2. `bundle/manifests/lightspeed-operator.clusterserviceversion.yaml`** — CSV metadata

Line 58:
```yaml
name: lightspeed-operator.vX.Y.Z
```

Line 715:
```yaml
version: X.Y.Z
```

**Note**: Line numbers are approximate and may shift. Use search to locate:
- Search for `name: lightspeed-operator.v` (around line 58)
- Search for `version:` near the end of the file (around line 715)

## Step-by-Step Process

### Step 2: Update bundle.Dockerfile

```bash
# Update both LABEL release and LABEL version
sed -i '' 's/^LABEL release=.*/LABEL release=X.Y.Z/' bundle.Dockerfile
sed -i '' 's/^LABEL version=.*/LABEL version=X.Y.Z/' bundle.Dockerfile
```

Or manually edit lines 63 and 66.

### Step 3: Update CSV metadata

**Line ~58** (name field with `v` prefix):
```yaml
name: lightspeed-operator.vX.Y.Z
```

**Line ~715** (version field WITHOUT prefix):
```yaml
version: X.Y.Z
```

### Step 4: Regenerate Bundle

After updating both files, regenerate the bundle:

```bash
make bundle BUNDLE_TAG=X.Y.Z
```

Or use the script:

```bash
hack/update_bundle.sh -v X.Y.Z -i related_images.json
```

This ensures all generated files are consistent with the new version and stable images.

```bash
operator-sdk bundle validate ./bundle
```

### Step 5: Verify Changes

```bash
git diff bundle.Dockerfile related_images.json bundle/
```

Confirm:
- [ ] `related_images.json` uses stable `registry.redhat.io` images
- [ ] `bundle.Dockerfile` has `release=X.Y.Z` and `version=X.Y.Z`
- [ ] CSV `name` field: `lightspeed-operator.vX.Y.Z` (with `v` prefix)
- [ ] CSV `version` field: `X.Y.Z` (without prefix)
- [ ] CSV `relatedImages` and deployment args match `related_images.json`
- [ ] All generated bundle files are updated

### Step 6: Commit (only when user asks)

```bash
git add bundle.Dockerfile related_images.json bundle/
git commit -m "OLS-XXXX: Release vX.Y.Z"
```

## Checklist

- [ ] `related_images.json` refreshed with `-r stable`
- [ ] `bundle.Dockerfile` updated (lines 63, 66)
- [ ] `bundle/manifests/lightspeed-operator.clusterserviceversion.yaml` name updated (line ~58, with `v` prefix)
- [ ] `bundle/manifests/lightspeed-operator.clusterserviceversion.yaml` version updated (line ~715, without prefix)
- [ ] Bundle regenerated with `make bundle BUNDLE_TAG=X.Y.Z` or `hack/update_bundle.sh -v X.Y.Z -i related_images.json`
- [ ] `operator-sdk bundle validate ./bundle` passes
- [ ] Both version files have matching `X.Y.Z`
- [ ] Changes committed (when requested)

## Common Mistakes to Avoid

❌ Forgetting the `v` prefix in CSV name field
❌ Adding a `v` prefix to CSV version field
❌ Updating only one file
❌ Not regenerating the bundle after changes
❌ Mismatched version numbers between files
❌ Bumping version without refreshing stable images (`-r stable`)
❌ Merging dev quay images from `/update-bundle dev` to `main` without re-running this skill

## Related

- `/update-bundle dev` — PR/CI bundle sync (Konflux `-r ci`, current version)
- `docs/olm-bundle-management.md` — bundle structure
- `hack/release_tools.md` — release tooling
