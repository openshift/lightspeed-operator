---
name: version-update
description: >-
  Release workflow: refresh stable related images and generate a selected v1
  classic or v2 agentic OLM bundle. Equivalent to
  /update-bundle release <v1|v2> <X.Y.Z>.
disable-model-invocation: true
---

# Version Update (Release)

Invoke with:

```text
/version-update <v1|v2> <X.Y.Z>
/update-bundle release <v1|v2> <X.Y.Z>
```

Examples:

```text
/version-update v1 1.2.1
/version-update v2 2.0.0
```

Not for PR/CI — use `/update-bundle dev`.

## Variant contract

| Variant | Release version | Bundle content | OCP compatibility |
|---|---|---|---|
| `v1` | `1.x.y` | Classic OLS only | `v4.16-v4.22` |
| `v2` | `2.x.y` | Classic OLS plus agentic components | `>=v5.0` |

Do not mix a selector and major version. The generator rejects invalid pairs:

```text
v1 + 2.x.y → invalid
v2 + 1.x.y → invalid
```

## Bundle sources and generated output

The variant CSV templates are the authoritative bundle inputs:

```text
config/manifests/bases/lightspeed-operator-v1.clusterserviceversion.yaml
config/manifests/bases/lightspeed-operator-v2.clusterserviceversion.yaml
```

`hack/update_bundle.sh` selects the requested template and generates:

```text
v1: bundle-v1/manifests/lightspeed-operator.clusterserviceversion.yaml
    bundle-v1/metadata/annotations.yaml
    bundle-v1.Dockerfile
v2: bundle-v2/manifests/lightspeed-operator.clusterserviceversion.yaml
    bundle-v2/metadata/annotations.yaml
    bundle-v2.Dockerfile
```

Do not edit those generated files as the source of a release version. The
operator-sdk generation step writes the selected `BUNDLE_TAG` into the output
CSV.

`hack/bundle.Dockerfile` is the Dockerfile template. Generation writes the
selected version, CPE, bundle identity, and OCP compatibility labels to the
variant Dockerfile; do not edit generated Dockerfile labels by hand.

## Step 1: Refresh `related_images.json` from stable images

Prerequisite: `oras` and `jq`. Do not rely on `oc`.

```bash
./hack/related_images_from_quay.sh -r stable -o related_images.json
```

Use `-r preview` for tech-preview paths. The tool resolves each Konflux-managed
entry and replaces its `konflux_prefix` with its `stable_prefix`; entries with
no Konflux prefix are unchanged.

Keep the `bundles` selectors intact:

```text
shared entries       → ["v1", "v2"]
agentic-only entries → ["v2"]
no bundles field     → selected for both variants
```

Optional: use `hack/snapshot_to_image_list.sh` only to discover new Konflux
revisions when `oc` is available, then rerun the stable refresh above.

If CRDs or RBAC changed since the last release, generate them before the bundle:

```bash
make manifests
```

For v2, first synchronize the pinned agentic contract when that target is
available:

```bash
make sync-agentic-crds
```

## Step 2: Generate the selected bundle

Use the normal Make target. It supplies the required `related_images.json`
input and passes the selector to `hack/update_bundle.sh`.

```bash
# Classic OCP 4.x release
make bundle BUNDLE_VARIANT=v1 BUNDLE_TAG=1.2.1

# Agentic OCP 5.0+ release
make bundle BUNDLE_VARIANT=v2 BUNDLE_TAG=2.0.0
```

Direct script use requires the selector and image list explicitly:

```bash
./hack/update_bundle.sh v1 -v 1.2.1 -i related_images.json
./hack/update_bundle.sh v2 -v 2.0.0 -i related_images.json
```

## Step 3: Validate the generated bundle

```bash
operator-sdk bundle validate ./bundle-v1 # use bundle-v2 for v2
```

Verify the selected output:

```bash
BUNDLE_DIR=bundle-v1 # use bundle-v2 for a v2 release
CSV="${BUNDLE_DIR}/manifests/lightspeed-operator.clusterserviceversion.yaml"

yq '.metadata.name' "$CSV"
yq '.metadata.annotations."com.redhat.openshift.versions"' \
  "${BUNDLE_DIR}/metadata/annotations.yaml"
yq -r '.spec.relatedImages[].name' "$CSV" | sort
```

Expected results:

```text
v1:
- CSV version/name is 1.x.y
- annotation is v4.16-v4.22
- no lightspeed-agentic-* related images or deployment arguments

v2:
- CSV version/name is 2.x.y
- annotation is >=v5.0
- includes lightspeed-agentic-operator, agentic console, alerts adapter,
  and sandbox related images
```

The generated CSV must not contain source-only image metadata:

```text
bundles
revision
snapshot_component
snapshot_source
konflux_prefix
stable_prefix
operator_arg
operator_target
```

## Step 4: Review and commit

```bash
git diff -- related_images.json hack/bundle.Dockerfile bundle-v1/ bundle-v2/ \
  bundle-v1.Dockerfile bundle-v2.Dockerfile
```

Confirm:

- [ ] Stable image digests are used for Konflux-managed entries.
- [ ] `bundles` selectors are present only in `related_images.json`, not in the CSV.
- [ ] The generated CSV version has the required major version.
- [ ] The generated OCP compatibility annotation matches the selected variant.
- [ ] Related images and deployment arguments match the selected variant.
- [ ] `operator-sdk bundle validate ./bundle-v1` or `./bundle-v2` passes for the selected variant.

Commit only when requested:

```bash
git add related_images.json hack/bundle.Dockerfile bundle-v1/ bundle-v2/ \
  bundle-v1.Dockerfile bundle-v2.Dockerfile
git commit -m "OLS-XXXX Release vX.Y.Z"
```

## Common mistakes

- Omitting `v1` or `v2` when calling `hack/update_bundle.sh`.
- Using a 1.x version with `v2`, or a 2.x version with `v1`.
- Editing the generated CSV instead of regenerating it with `BUNDLE_TAG`.
- Removing image `bundles` selectors during a stable-image refresh.
- Publishing a v1 bundle with agentic images or arguments.
- Publishing a v2 bundle without the synchronized agentic CRD/RBAC contract.
- Merging CI/Quay development image references without refreshing stable images.

## Related

- `/update-bundle dev` — PR/CI workflow.
- `docs/olm-bundle-management.md` — bundle structure and image metadata.
- `hack/related_images_from_quay.sh` — stable digest refresh.
- `hack/release_tools.md` — bundle release tooling.
