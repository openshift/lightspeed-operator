# OLM Catalog Management Guide

This guide covers catalog management for the OpenShift Lightspeed Operator.

> **📖 For OLM Catalog Fundamentals:** See [OLM File-Based Catalogs](https://olm.operatorframework.io/docs/reference/file-based-catalogs/)  
> **📖 For Bundle Basics:** See [OLM Bundle Management Guide](./olm-bundle-management.md)

---

## Overview

A catalog organizes operator bundles by versions and channels.

**Workflow:**
```
Bundle (single version) → Add to Catalog(s) → OLM discovers operator → Users install
```

**Key Concepts:**
- **Catalog**: Collection of bundles organized by channels
- **Channel**: Update stream (e.g., `alpha`, `stable`)
- **Package**: Operator identity across versions (`lightspeed-operator`)
- **FBC**: File-Based Catalog (declarative YAML format)
- **Skip Range**: Version ranges that can be skipped during upgrades

---

## Our Multi-Catalog Strategy

We maintain **separate catalogs per OpenShift version** because bundles have version-specific configurations.

### Catalog Structure

```
lightspeed-catalog/           # Main catalog (latest OCP)
lightspeed-catalog-4.16/      # OCP 4.16-specific
lightspeed-catalog-4.17/      # OCP 4.17-specific
lightspeed-catalog-4.18/      # OCP 4.18-specific
lightspeed-catalog-4.19/      # OCP 4.19-specific
lightspeed-catalog-4.20/      # OCP 4.20-specific
```

Each contains:
```
lightspeed-catalog-4.18/
├── index.yaml              # FBC file with bundles, channels, upgrade paths
└── lightspeed-catalog-4.18.Dockerfile
```

### OpenShift Version Mapping

| OpenShift Version | Catalog Directory | Kubernetes Version |
|-------------------|-------------------|-------------------|
| 4.16 | `lightspeed-catalog-4.16/` | 1.29 |
| 4.17 | `lightspeed-catalog-4.17/` | 1.30 |
| 4.18 | `lightspeed-catalog-4.18/` | 1.31 |
| 4.19 | `lightspeed-catalog-4.19/` | 1.32 |
| 4.20 | `lightspeed-catalog-4.20/` | 1.33 |

**Why separate catalogs?**
- Bundle metadata differs per OCP version (`com.redhat.openshift.versions` annotation)
- Different minimum Kubernetes versions
- Version-specific features and APIs

---

## Catalog Generation Workflow

**Add bundle to all catalogs:**

```bash
# 1. Build and push bundle first
make bundle BUNDLE_TAG=0.2.0
make bundle-build BUNDLE_IMG=quay.io/org/bundle:v0.2.0
make bundle-push BUNDLE_IMG=quay.io/org/bundle:v0.2.0

# 2. Add to catalogs using our script
./hack/bundle_to_catalog.sh quay.io/org/bundle:v0.2.0

# 3. Build and push catalog images
for v in 4.16 4.17 4.18 4.19 4.20; do
  make catalog-build VERSION=$v
  make catalog-push VERSION=$v
done
```

**Our script** (`hack/bundle_to_catalog.sh`):
- Renders bundle to FBC format
- Adds to all OCP-version catalogs
- Updates channel entries
- Handles skip ranges

**Validate catalogs:**
```bash
for d in lightspeed-catalog*; do opm validate $d; done
```

---

## Channel Management

**Our Channels:**
- **alpha**: Latest development versions, frequent updates

**Channel properties:**
- `replaces`: Defines upgrade path from previous version
- `skipRange`: Versions that can be skipped (optional, e.g., `">=0.1.0 <0.3.0"`)

> **📖 Skip Range & Upgrade Graphs:** See [OLM Upgrade Graph](https://olm.operatorframework.io/docs/concepts/olm-architecture/operator-catalog/creating-an-update-graph/)

**Example FBC structure:**
```yaml
---
schema: olm.package
name: lightspeed-operator
defaultChannel: alpha

---
schema: olm.bundle
name: lightspeed-operator.v0.1.0
package: lightspeed-operator
image: registry.redhat.io/.../bundle@sha256:...

---
schema: olm.channel
name: alpha
package: lightspeed-operator
entries:
  - name: lightspeed-operator.v0.1.0
  - name: lightspeed-operator.v0.2.0
    skipRange: ">=0.1.0 <0.2.0"
    replaces: lightspeed-operator.v0.1.0
```

---

## Common Tasks

### Add New Bundle to All Catalogs

```bash
# 1. Create and push bundle
make bundle BUNDLE_TAG=0.3.0
make bundle-build BUNDLE_IMG=quay.io/org/bundle:v0.3.0
make bundle-push BUNDLE_IMG=quay.io/org/bundle:v0.3.0

# 2. Add to all catalogs
./hack/bundle_to_catalog.sh quay.io/org/bundle:v0.3.0

# 3. Build and push catalogs
for v in 4.16 4.17 4.18 4.19 4.20; do
  make catalog-build VERSION=$v
  make catalog-push VERSION=$v
done
```

### Add Bundle to Specific Catalog

```bash
opm render quay.io/org/bundle:v0.3.0 > /tmp/bundle.yaml
cat /tmp/bundle.yaml >> lightspeed-catalog-4.18/index.yaml
vim lightspeed-catalog-4.18/index.yaml  # Add to channel entries
opm validate lightspeed-catalog-4.18
make catalog-build VERSION=4.18
```

### Update Skip Range

```bash
vim lightspeed-catalog-4.18/index.yaml  # Edit channel entry skipRange
opm validate lightspeed-catalog-4.18
make catalog-build VERSION=4.18
```

### Create Catalog for New OCP Version

```bash
mkdir lightspeed-catalog-4.21
cp lightspeed-catalog-4.20/index.yaml lightspeed-catalog-4.21/
cp lightspeed-catalog-4.20.Dockerfile lightspeed-catalog-4.21.Dockerfile
# Edit Dockerfile to reference 4.21
vim Makefile  # Add 4.21 to catalog targets
```

---

## Troubleshooting

**Validation fails:**
```bash
opm validate lightspeed-catalog-4.18
yq eval lightspeed-catalog-4.18/index.yaml  # Check YAML syntax
```

**Bundle not appearing:**
```bash
grep "lightspeed-operator.v0.3.0" lightspeed-catalog-4.18/index.yaml
yq '.[] | select(.schema == "olm.channel") | .entries' lightspeed-catalog-4.18/index.yaml
```

**Catalog image build fails:**
```bash
opm validate lightspeed-catalog-4.18  # Validate first
podman build -f lightspeed-catalog-4.18.Dockerfile -t test-catalog .
```

**View catalog contents:**
```bash
# List all bundles
yq '.[] | select(.schema == "olm.bundle") | .name' lightspeed-catalog-4.18/index.yaml

# List channel entries
yq '.[] | select(.schema == "olm.channel") | .entries' lightspeed-catalog-4.18/index.yaml
```

---

## Key Commands

```bash
# Add bundle to catalogs
./hack/bundle_to_catalog.sh <bundle-image>

# Build catalog
make catalog-build VERSION=4.18

# Validate catalog
opm validate lightspeed-catalog-4.18

# Render bundle to FBC
opm render <bundle-image>
```

---

## Additional Resources

- [OLM Bundle Management](./olm-bundle-management.md)
- [OLM Integration & Lifecycle](./olm-integration-lifecycle.md)
- [OLM File-Based Catalogs](https://olm.operatorframework.io/docs/reference/file-based-catalogs/)
- [Creating Update Graphs](https://olm.operatorframework.io/docs/concepts/olm-architecture/operator-catalog/creating-an-update-graph/)

**Project Scripts:**
- `hack/bundle_to_catalog.sh` - Add bundle to catalogs
- `hack/snapshot_to_catalog.sh` - Snapshot-based catalog generation
