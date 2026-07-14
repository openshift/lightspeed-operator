---
name: update-bundle
description: >-
  Sync related_images.json from Quay (oras + konflux_prefix/revision in
  related_images.json) and regenerate the OLM bundle. Pass mode dev (CI quay) or
  release (stable images, version bump — /version-update). Do not use oc.
disable-model-invocation: true
---

# Update Bundle

One entry point, two modes:

```
/update-bundle dev
/update-bundle release X.Y.Z    # same as /version-update X.Y.Z
```

| | **dev** | **release** |
|---|---------|-------------|
| **When** | PR/CI, local e2e | Shipping to `main` |
| **Refresh** | `hack/related_images_from_quay.sh -r ci` | `hack/related_images_from_quay.sh -r stable` |
| **Image hosts** | `quay.io/redhat-user-workloads/...` | `registry.redhat.io/openshift-lightspeed/...` |
| **Bundle version** | Keep current CSV `spec.version` | Bump to `X.Y.Z` |

**Do not use `oc` or Konflux snapshots** for routine bundle updates. `related_images.json` already carries `konflux_prefix`, `stable_prefix`, and `revision` (git tag on Quay). Resolve digests with `oras`.

---

## Development mode (`dev`)

### Step 1: Refresh `related_images.json` from Quay

**Prerequisite:** `oras` and `jq` on PATH.

```bash
./hack/related_images_from_quay.sh -l -r ci -o related_images.json
```

**How it works** (loops `related_images.json`):

| Field | Role |
|-------|------|
| `konflux_prefix` | Quay repo base (e.g. `quay.io/.../ols/lightspeed-operator`) |
| `revision` | Git SHA tag on Quay → `oras resolve <prefix>:<revision>` |
| `stable_prefix` | Product registry path; substituted when `-r stable` |
| (no `konflux_prefix`) | External/manual pin — image left unchanged |

**`-l` (latest):** for each Konflux operand, discover the newest build on Quay (`:main` digest mapped to a git SHA tag, or the most recently pushed SHA tag), update `revision`, then resolve the digest. Use this for dev/PR bundle updates so operands are not stuck on an old snapshot pin.

Without `-l`, only re-resolves digests for `revision` values already in the file (legacy pin mode).

**Entry types:** see `docs/olm-bundle-management.md` (Related Images Management).

**Optional:** `hack/snapshot_to_image_list.sh` (requires `oc` login) only when you need to **discover new revisions** from a Konflux snapshot; then re-run `related_images_from_quay.sh`.

**Bundle entry:** when only operand images change, back up `lightspeed-operator-bundle` per `README.md` before refresh if you do not intend to update the bundle image.

### Step 2: Regenerate bundle (current version)

```bash
make bundle BUNDLE_TAG=<current-version>
```

If CRD/RBAC changed: `make manifests` first.

### Step 3: Verify

```bash
operator-sdk bundle validate ./bundle
git diff related_images.json bundle/
```

Confirm:

- [ ] Dev: images on `quay.io/redhat-user-workloads/...` (or external `registry.redhat.io` pins)
- [ ] `snapshot_component`, `konflux_prefix`, `stable_prefix` preserved; `image` digests updated
- [ ] CSV `spec.relatedImages` and deployment args match `related_images.json`
- [ ] **Konflux operands current:** dev refresh uses `-l` so `revision` tracks the latest Quay build (not only digest re-resolve for stale SHAs)

**Operator / CSV flag check** (when deployment-patch args changed):

```bash
REV=$(jq -r '.[] | select(.name=="lightspeed-operator") | .revision' related_images.json)
git merge-base --is-ancestor <commit-that-added-flag> "$REV" \
  && echo "operator revision includes flag" || echo "STALE: bump operator revision"
```

Example: RHOKP flag landed in merge `912ea22c` (#1653); revision `7bc611cb` (Jul 10 mintmaker) is **before** that and must not be used with `--rhokp-image` in the CSV.

Quick Quay check for latest `main`:

```bash
HEAD_SHA=$(git rev-parse origin/main)
oras resolve quay.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols/lightspeed-operator:"$HEAD_SHA" \
  && echo "latest main is on Quay — consider updating operator revision"
```

### Step 4: Commit (only when user asks)

---

## Release mode (`release X.Y.Z`)

Follow `version-update/SKILL.md` in full: `-r stable`, version bump, `make bundle`.

## Related

- `/version-update` — release workflow
- `docs/olm-bundle-management.md`
- `hack/related_images_from_quay.sh`
