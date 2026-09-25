# OLM Bundle Management Guide

This guide covers bundle management for the OpenShift Lightspeed Operator.

> **📖 For OLM Fundamentals:** See [Operator SDK Bundle Documentation](https://sdk.operatorframework.io/docs/olm-integration/tutorial-bundle/)  
> **📖 For CSV Field Reference:** See [ClusterServiceVersion Spec](https://olm.operatorframework.io/docs/concepts/crds/clusterserviceversion/)

---

## Overview

An OLM bundle packages an operator for distribution and installation. It contains:
- **Manifests**: ClusterServiceVersion (CSV), CRD, RBAC
- **Metadata**: OLM annotations (channels, versions, compatibility)
- **Dockerfile**: Bundle image build instructions

---

## Bundle Structure

```
bundle/                                                   # Legacy operator-sdk workspace; not a release build input
bundle.Dockerfile                                         # Legacy operator-sdk workspace; preserved during generation

bundle-v1/                                                # Generated classic OCP 4.x release bundle
bundle-v1.Dockerfile                                      # Builds bundle-v1/

bundle-v2/                                                # Generated agentic OCP 5.0+ bundle
├── manifests/
│   ├── lightspeed-operator.clusterserviceversion.yaml
│   ├── ols.openshift.io_olsconfigs.yaml
│   ├── agentic.openshift.io_*.yaml                       # v2 only
│   └── *_rbac.authorization.k8s.io_*.yaml
├── metadata/annotations.yaml
└── tests/scorecard/
    └── config.yaml
bundle-v2.Dockerfile                                      # Builds bundle-v2/
```

### Important: Install Mode vs CRD Scope

**Our Configuration:**
- **CSV install mode**: `OwnNamespace` (operator deployed in `openshift-lightspeed`)
- **CRD scope**: `Cluster` (OLSConfig is cluster-scoped, no namespace required)

**Why this matters:**

The `OLSConfig` CRD is intentionally **cluster-scoped** despite `OwnNamespace` install mode:

1. **Singleton Pattern**: One OLSConfig instance per cluster (name must be `cluster`)
2. **Semantic Correctness**: Cluster-wide service = cluster-scoped resource
3. **Cross-Namespace Watching**: Can watch Secrets/ConfigMaps in any namespace
4. **User Convenience**: `oc get olsconfig cluster` (no namespace flag needed)

**Key Distinction:**
- **CSV install mode**: Where operator deployment lives (`openshift-lightspeed`)
- **CRD scope**: How users access the custom resource (cluster-wide)

All operand resources (deployments, services) are still created in `openshift-lightspeed` namespace.

---

## Bundle Generation Workflow

### When to Regenerate Bundle

**Required:**
- RBAC changes (`//+kubebuilder:rbac` markers or `config/rbac/`)
- CRD changes (`api/v1alpha1/olsconfig_types.go`)
- Image changes (operator or operand images)
- CSV metadata changes (description, keywords, maintainers)
- Any other change in .config directory

**Not Required:**
- Reconciliation logic changes
- Tests, docs, internal utilities

### Commands

**Generate and validate a selected bundle:**
```bash
# Classic OCP 4.x bundle
make bundle BUNDLE_VARIANT=v1 BUNDLE_TAG=1.1.4
operator-sdk bundle validate ./bundle-v1

# Agentic OCP 5.0+ bundle; uses committed agentic CRD/RBAC inputs
make bundle BUNDLE_VARIANT=v2 BUNDLE_TAG=2.0.0
operator-sdk bundle validate ./bundle-v2
```

**What happens:**
1. Selects the v1 or v2 CSV template and matching related images
2. Generates manifests from the committed CRD/RBAC inputs via `operator-sdk` and `kustomize`
3. Adds the selected OpenShift compatibility annotation
4. Writes `bundle-v1/` or `bundle-v2/` and its matching Dockerfile
5. Validates the generated bundle

Run `make sync-agentic-crds` separately before v2 generation only when
refreshing the pinned agentic CRD/RBAC contract.

**Build and push:**
```bash
make bundle-build BUNDLE_VARIANT=v1 BUNDLE_IMG=quay.io/myorg/lightspeed-operator-bundle:v1.1.4
make bundle-push BUNDLE_VARIANT=v1 BUNDLE_IMG=quay.io/myorg/lightspeed-operator-bundle:v1.1.4

make bundle-build BUNDLE_VARIANT=v2 BUNDLE_IMG=quay.io/myorg/lightspeed-agentic-operator-bundle:v2.0.0
make bundle-push BUNDLE_VARIANT=v2 BUNDLE_IMG=quay.io/myorg/lightspeed-agentic-operator-bundle:v2.0.0
```

### Implementation Files

- Makefile: [`Makefile`](../Makefile) (lines 329-346)
- Script: [`hack/update_bundle.sh`](../hack/update_bundle.sh)
- Images: [`related_images.json`](../related_images.json)
- Generated Dockerfiles: `bundle-v1.Dockerfile` and `bundle-v2.Dockerfile`

---

## Related Images Management

**Purpose:** `related_images.json` is the **single source of truth** for operand images and operator deployment wiring. Each operand entry may include `operator_arg` (passed as `--<operator_arg>=<image>` to the operator) or `operator_target: image` for the operator container itself. `hack/generate_deployment_patch.sh` (via `make manifests`) generates `config/default/deployment-patch.yaml`; `hack/update_bundle.sh` and `make deploy` substitute image digests from the same file.

**Format:** Each entry has at least `name` and `image`. Optional fields depend on how the image is sourced (see **Entry types** below).

**Entry types:**

Entries fall into two categories. Do not add inline comments to `related_images.json` (JSON does not support them); use this section as the reference.

| Type | When to use | Required fields | Optional Konflux fields |
|------|-------------|-----------------|-------------------------|
| **Konflux-managed** | Image built in the OLS Konflux tenant | `name`, `image`, `revision` | `snapshot_component`, `konflux_prefix`, `stable_prefix`; add `snapshot_source: "bundle"` only for `lightspeed-operator-bundle` |
| **External / manual** | Third-party or Red Hat product images not in the OLS snapshot | `name`, `image`, `revision: ""` | None — pin `image` yourself; snapshot refresh leaves these unchanged |

**Konflux-managed example** (refreshed by `hack/snapshot_to_image_list.sh`; metadata stripped from CSV by `hack/update_bundle.sh`):

```json
{
  "name": "lightspeed-service-api",
  "image": "quay.io/.../lightspeed-service@sha256:...",
  "revision": "e5e1454f3fa8b19293200868684abcaf18f38097",
  "operator_arg": "service-image",
  "snapshot_component": "lightspeed-service",
  "konflux_prefix": "quay.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols/lightspeed-service",
  "stable_prefix": "registry.redhat.io/openshift-lightspeed/lightspeed-service-api-rhel9"
}
```

**External / manual example** (e.g. PostgreSQL, dataverse exporter, RHOKP):

```json
{
  "name": "rhokp",
  "image": "registry.redhat.io/offline-knowledge-portal/rhokp-rhel9@sha256:f46082f2dc2972582f3b85ed2a563b554d0aba3255ba2f00835e65f4929ae9a9",
  "revision": "",
  "operator_arg": "rhokp-image"
}
```

Field reference for Konflux-managed entries:

- `snapshot_component` — component name in the Konflux snapshot (`spec.components[].name`)
- `konflux_prefix` — Quay image prefix in CI/Konflux workloads
- `stable_prefix` — product registry prefix when refreshing with `-r stable`
- `snapshot_source` — omit (defaults to OLS snapshot); set to `"bundle"` only for `lightspeed-operator-bundle`

**Workflow:**
```
related_images.json → make manifests (deployment-patch.yaml) → hack/update_bundle.sh v1|v2 → variant CSV relatedImages + deployment args → Controller → Operand deployments
```

**Best practice:**
- Development: Use tags (`:latest`, `:v1.0.0`)
- Production: Use digests (`@sha256:abc123...`) for reproducibility

---

## Version Management

**Bump a selected bundle line:**
```bash
# Classic v1 release
make bundle BUNDLE_VARIANT=v1 BUNDLE_TAG=1.2.0

# Agentic v2 release
make bundle BUNDLE_VARIANT=v2 BUNDLE_TAG=2.0.0

# Review and commit the selected generated output
git diff -- bundle-v2/ bundle-v2.Dockerfile
git add bundle-v2/ bundle-v2.Dockerfile
git commit -m "OLS-XXXX Release v2.0.0"
```

**Semantic Versioning:**
- **Major (x.0.0)**: Breaking changes
- **Minor (0.x.0)**: New features, backward-compatible
- **Patch (0.0.x)**: Bug fixes

**Ensure version consistency across:**
1. selected `BUNDLE_VARIANT` and `BUNDLE_TAG`
2. generated CSV metadata name (`lightspeed-operator.vX.Y.Z`)
3. generated CSV spec `version` field (`X.Y.Z`)
4. selected variant Dockerfile `release`, `version`, and CPE labels

---

## Common Tasks

### Update Operator Image

```bash
# Get image references from Konflux snapshot (pass -b for bundle snapshot when updating ols-bundle)
./hack/snapshot_to_image_list.sh -s <ols-snapshot-ref> -b <ols-bundle-snapshot-ref> -o related_images.json

# Update the selected bundle
make bundle BUNDLE_VARIANT=v1 BUNDLE_TAG=1.1.4

# Verify operator image was updated
grep "lightspeed-operator" bundle-v1/manifests/*.clusterserviceversion.yaml
```

### Add RBAC Permission

```bash
vim config/rbac/role.yaml  # Update RBAC
make manifests && make bundle BUNDLE_VARIANT=v1 BUNDLE_TAG=1.0.0
yq '.spec.install.spec.clusterPermissions[0].rules' \
  bundle-v1/manifests/lightspeed-operator.clusterserviceversion.yaml  # Verify
```

### Change OpenShift Version Support

```bash
# Compatibility is selected by variant; do not edit generated annotations.
make bundle BUNDLE_VARIANT=v1 BUNDLE_TAG=1.1.4
operator-sdk bundle validate ./bundle-v1

make bundle BUNDLE_VARIANT=v2 BUNDLE_TAG=2.0.0
operator-sdk bundle validate ./bundle-v2
```

---

## Troubleshooting

### Bundle Validation Fails

```bash
operator-sdk bundle validate ./bundle-v1 -o text  # Use bundle-v2 for v2
```

**Common fixes:**
- Check CSV YAML syntax (indentation)
- Ensure required fields present (`minKubeVersion`, `displayName`, `version`)
- Verify image references are valid
- Check RBAC rules format

### Images Not Updated in CSV

```bash
YQ=$(which yq) JQ=$(which jq) ./hack/update_bundle.sh v1 -v 1.0.0 -i related_images.json
```

**Common fixes:**
- Verify `related_images.json` format
- Ensure `yq` and `jq` are installed
- Check image names match expected patterns

### OLM Can't Install Bundle

```bash
# Check subscription and install plan
oc get subscription lightspeed-operator -n openshift-lightspeed -o yaml
oc get installplan -n openshift-lightspeed
```

**Common fixes:**
- Verify RBAC permissions complete
- Ensure CRD is valid
- Review deployment spec in CSV

---

## Additional Resources

- [OLM Catalog Management](./olm-catalog-management.md) - Next: organize bundles into catalogs
- [OLM Integration & Lifecycle](./olm-integration-lifecycle.md) - Deploy bundles via OLM
- [Operator SDK Bundle Docs](https://sdk.operatorframework.io/docs/olm-integration/tutorial-bundle/)
- [CSV Field Reference](https://olm.operatorframework.io/docs/concepts/crds/clusterserviceversion/)
