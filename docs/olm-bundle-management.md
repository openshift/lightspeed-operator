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
bundle/
├── manifests/
│   ├── lightspeed-operator.clusterserviceversion.yaml  # Main metadata and install strategy
│   ├── ols.openshift.io_olsconfigs.yaml                # CRD definition
│   └── *_rbac.authorization.k8s.io_*.yaml              # RBAC resources
├── metadata/
│   └── annotations.yaml                                # Bundle metadata (channels, OCP versions)
└── tests/scorecard/
    └── config.yaml                                     # Scorecard configuration

bundle.Dockerfile                                        # Bundle image build
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

**Generate bundle:**
```bash
make bundle BUNDLE_TAG=0.1.0

# With custom images
make bundle BUNDLE_TAG=0.1.0 RELATED_IMAGES_FILE=related_images.json
```

**What happens:**
1. Generates manifests via `operator-sdk` and `kustomize`
2. Updates image references from `related_images.json`
3. Adds OpenShift compatibility annotations
4. Generates bundle Dockerfile
5. Validates bundle

**Validate:**
```bash
operator-sdk bundle validate ./bundle

# For OpenShift
operator-sdk bundle validate ./bundle --select-optional name=operatorhub
```

**Build and push:**
```bash
make bundle-build BUNDLE_IMG=quay.io/myorg/lightspeed-operator-bundle:v0.1.0
make bundle-push BUNDLE_IMG=quay.io/myorg/lightspeed-operator-bundle:v0.1.0
```

### Implementation Files

- Makefile: [`Makefile`](../Makefile) (lines 329-346)
- Script: [`hack/update_bundle.sh`](../hack/update_bundle.sh)
- Images: [`related_images.json`](../related_images.json)
- Dockerfile: [`bundle.Dockerfile`](../bundle.Dockerfile)

---

## Related Images Management

**Purpose:** `related_images.json` tracks all container images for CSV updates, CI/CD promotion, and image mirroring.

**Format:**
```json
 {
  "name": "lightspeed-operator",
  "image": "quay.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols/lightspeed-operator@sha256:65b288d147503bd66808eecf69deee8e71ec890752a70f83a9314061fe0d4420",
  "revision": "e5e1454f3fa8b19293200868684abcaf18f38097"
}
```

**Workflow:**
```
related_images.json → hack/update_bundle.sh → CSV relatedImages → CSV deployment args → Controller → Operand deployments
```

**Best practice:**
- Development: Use tags (`:latest`, `:v1.0.0`)
- Production: Use digests (`@sha256:abc123...`) for reproducibility

---

## Version Management

**Bump version:**
```bash
# 1. Update version
vim Makefile  # Update BUNDLE_TAG

# 2. Generate bundle
make bundle BUNDLE_TAG=0.2.0

# 3. Review and commit
git diff bundle/
git add bundle/ bundle.Dockerfile
git commit -m "chore: bump bundle version to v0.2.0"
```

**Semantic Versioning:**
- **Major (x.0.0)**: Breaking changes
- **Minor (0.x.0)**: New features, backward-compatible
- **Patch (0.0.x)**: Bug fixes

**Ensure version consistency across:**
1. `Makefile` (`BUNDLE_TAG`)
2. CSV metadata name (`lightspeed-operator.v0.2.0`)
3. CSV spec `version` field
4. Bundle Dockerfile labels

---

## Common Tasks

### Update Operator Image

```bash
# Get image references from Konflux snapshot
./hack/snapshot_to_image_list.sh -s <snapshot-ref> -o related_images.json

# Update bundle (uses version from related_images.json or current CSV)
make bundle

# Verify operator image was updated
grep "lightspeed-operator" bundle/manifests/*.clusterserviceversion.yaml
```

### Add RBAC Permission

```bash
vim config/rbac/role.yaml  # Update RBAC
make manifests && make bundle BUNDLE_TAG=0.1.0
yq '.spec.install.spec.clusterPermissions[0].rules' \
  bundle/manifests/lightspeed-operator.clusterserviceversion.yaml  # Verify
```

### Change OpenShift Version Support

```bash
vim bundle/metadata/annotations.yaml  # Change: com.redhat.openshift.versions
operator-sdk bundle validate ./bundle
```

---

## Troubleshooting

### Bundle Validation Fails

```bash
operator-sdk bundle validate ./bundle -o text  # Verbose output
```

**Common fixes:**
- Check CSV YAML syntax (indentation)
- Ensure required fields present (`minKubeVersion`, `displayName`, `version`)
- Verify image references are valid
- Check RBAC rules format

### Images Not Updated in CSV

```bash
YQ=$(which yq) JQ=$(which jq) ./hack/update_bundle.sh -v 0.1.0 -i related_images.json
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
