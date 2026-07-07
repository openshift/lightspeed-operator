---
name: update-bundle
description: >-
  Sync related_images.json from Konflux and regenerate the OLM bundle. Pass mode
  dev (CI quay images, current version) or release (stable images, version bump
  — full steps in /version-update). Use for PR bundle/CI fixes or shipping to
  main.
disable-model-invocation: true
---

# Update Bundle

One entry point, two modes:

```
/update-bundle dev
/update-bundle release X.Y.Z    # same as /version-update X.Y.Z
```

Releases ship from `main` (no separate release branch).

| | **dev** | **release** |
|---|---------|-------------|
| **When** | PR/CI, local e2e, stale Konflux digests | Shipping to `main` |
| **Snapshot** | `-r ci` | `-r stable` |
| **Image hosts** | `quay.io/redhat-user-workloads/...` | `registry.redhat.io/openshift-lightspeed/...` |
| **Bundle version** | Keep current | Bump to `X.Y.Z` |
| **Full steps** | Below | [version-update/SKILL.md](../version-update/SKILL.md) |

**Not required** for reconciliation-only changes under `internal/controller/`, tests, or docs.

---

## Development mode (`dev`)

### Step 1: Refresh `related_images.json`

Requires Konflux `oc login` in namespace `crt-nshift-lightspeed-tenant`. See `README.md` (Update Bundle from Snapshot).

```bash
./hack/snapshot_to_image_list.sh -s <ols-snapshot> -b <ols-bundle-snapshot> -r ci -o related_images.json
```

`-r ci` is the default; images stay on `quay.io/redhat-user-workloads/...`.

**Preserve entries the snapshot does not provide** (script keeps some from the existing file):

- `lightspeed-to-dataverse-exporter`
- Operands not yet in Konflux snapshots — merge manually after the script runs

**Bundle image:** README documents backing up `lightspeed-operator-bundle` before refresh and restoring it when only operand images change.

If `oc` is unavailable, resolve digests manually (e.g. `oras resolve` against Quay revision tags).

### Step 2: Regenerate bundle (current version)

```bash
# From bundle/manifests/lightspeed-operator.clusterserviceversion.yaml spec.version
make bundle BUNDLE_TAG=<current-version>
```

If CRD/RBAC changed:

```bash
make manifests
make bundle BUNDLE_TAG=<current-version>
```

Or:

```bash
hack/update_bundle.sh -v <current-version> -i related_images.json
```

### Step 3: Verify

```bash
operator-sdk bundle validate ./bundle
git diff related_images.json bundle/
```

Confirm:

- [ ] Images use Konflux quay hosts (not `registry.redhat.io`)
- [ ] CSV `spec.relatedImages` and deployment args match `related_images.json`
- [ ] Bundle CRD matches source when CRD changed

### Step 4: Commit (only when user asks)

Do not commit unless requested. Dev quay images are for PR validation; **`main` gets stable images via `/version-update`**.

---

## Release mode (`release X.Y.Z`)

Use **`/version-update X.Y.Z`** or follow [version-update/SKILL.md](../version-update/SKILL.md) in full. That skill includes:

1. Refresh `related_images.json` with `-r stable`
2. Bump `bundle.Dockerfile` and CSV version (with `sed` examples)
3. `make bundle BUNDLE_TAG=X.Y.Z` or `hack/update_bundle.sh`
4. Verify checklist and common mistakes

Do not skip the version-update steps when releasing.

## Related

- `/version-update` — full release workflow
- `docs/olm-bundle-management.md` — bundle structure
