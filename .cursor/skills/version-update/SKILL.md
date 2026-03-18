---
name: version-update
description: Updates the operator version number across all required files and regenerates the bundle. Use when bumping the version for a release or when the user asks to update, bump, or change the version number.
disable-model-invocation: true
---

# Version Update

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

### Step 1: Update bundle.Dockerfile

```bash
# Update both LABEL release and LABEL version
sed -i '' 's/^LABEL release=.*/LABEL release=X.Y.Z/' bundle.Dockerfile
sed -i '' 's/^LABEL version=.*/LABEL version=X.Y.Z/' bundle.Dockerfile
```

Or manually edit lines 63 and 66.

### Step 2: Update CSV metadata

**Line ~58** (name field with `v` prefix):
```yaml
name: lightspeed-operator.vX.Y.Z
```

**Line ~715** (version field WITHOUT prefix):
```yaml
version: X.Y.Z
```

### Step 3: Regenerate Bundle

After updating both files, regenerate the bundle:

```bash
make bundle
```

Or use the script:

```bash
hack/update_bundle.sh -v X.Y.Z
```

This ensures all generated files are consistent with the new version.

### Step 4: Verify Changes

```bash
git diff bundle.Dockerfile bundle/manifests/
```

Confirm:
- [ ] `bundle.Dockerfile` has `release=X.Y.Z` and `version=X.Y.Z`
- [ ] CSV `name` field: `lightspeed-operator.vX.Y.Z` (with `v` prefix)
- [ ] CSV `version` field: `X.Y.Z` (without prefix)
- [ ] All generated bundle files are updated

### Step 5: Commit

```bash
git add bundle.Dockerfile bundle/
git commit -m "Bump version to X.Y.Z"
```

## Checklist

- [ ] `bundle.Dockerfile` updated (lines 63, 66)
- [ ] `bundle/manifests/lightspeed-operator.clusterserviceversion.yaml` name updated (line ~58, with `v` prefix)
- [ ] `bundle/manifests/lightspeed-operator.clusterserviceversion.yaml` version updated (line ~715, without prefix)
- [ ] Bundle regenerated with `make bundle` or `hack/update_bundle.sh`
- [ ] Both files have matching versions
- [ ] Changes committed

## Common Mistakes to Avoid

❌ Forgetting the `v` prefix in CSV name field
❌ Adding a `v` prefix to CSV version field
❌ Updating only one file
❌ Not regenerating the bundle after changes
❌ Mismatched version numbers between files
