# OLM Catalog Management Guide

This guide covers the management of Operator Lifecycle Manager (OLM) catalogs for the OpenShift Lightspeed Operator, including File-Based Catalogs (FBC), multi-version support, and catalog building workflows.

## Table of Contents

- [Overview](#overview)
- [File-Based Catalogs (FBC)](#file-based-catalogs-fbc)
- [Catalog Structure](#catalog-structure)
- [Multi-Version Catalog Strategy](#multi-version-catalog-strategy)
- [Channel Management](#channel-management)
- [Skip Ranges and Upgrade Paths](#skip-ranges-and-upgrade-paths)
- [Catalog Building Workflow](#catalog-building-workflow)
- [Bundle to Catalog Migration](#bundle-to-catalog-migration)
- [Catalog Validation](#catalog-validation)
- [Common Tasks](#common-tasks)
- [Troubleshooting](#troubleshooting)

---

## Overview

OLM catalogs are collections of operator bundles organized by channels and versions. Starting with OLM v1, the **File-Based Catalog (FBC)** format uses declarative YAML to define catalog contents, replacing the older SQLite database format.

### Relationship to Bundle Management

This guide builds on the [OLM Bundle Management Guide](./olm-bundle-management.md) and covers the next layer of the operator distribution workflow:

```
┌─────────────────────────────────────────────────────────────────┐
│                    Operator Distribution Flow                    │
└─────────────────────────────────────────────────────────────────┘

1. Development
   ├── Write operator code
   ├── Define CRDs (api/v1alpha1/)
   └── Configure RBAC (config/rbac/)
          ↓
2. Bundle Creation ← [Bundle Management Guide]
   ├── Generate manifests (make manifests)
   ├── Create CSV with metadata
   ├── Package as bundle (make bundle)
   ├── Build bundle image
   └── Push bundle image to registry
          ↓
3. Catalog Creation ← [This Guide: Catalog Management]
   ├── Render bundle to FBC format
   ├── Add bundle to catalog
   ├── Define channels and upgrade paths
   ├── Build catalog image
   └── Push catalog image to registry
          ↓
4. Distribution
   ├── Deploy CatalogSource to cluster
   ├── Users discover operator in OperatorHub
   ├── Users install via Subscription
   └── OLM manages operator lifecycle
```

### Bundle vs Catalog: Key Differences

| Aspect | Bundle | Catalog |
|--------|--------|---------|
| **Purpose** | Package a single operator version | Organize multiple bundle versions |
| **Content** | CSV, CRDs, RBAC for one version | References to multiple bundles |
| **Format** | Kubernetes manifests + metadata | FBC (File-Based Catalog) YAML |
| **Scope** | One operator version (e.g., v1.0.6) | All versions + channels |
| **Distribution** | Bundle image (`bundle:v1.0.6`) | Catalog image (`catalog:latest`) |
| **Used By** | Catalog building tools | OLM to install operators |
| **Lifecycle** | Created per release | Updated with each release |

**Example:**
- **Bundle**: `quay.io/openshift-lightspeed/lightspeed-operator-bundle:v1.0.6`
  - Contains: CSV, OLSConfig CRD, RBAC rules for v1.0.6
- **Catalog**: `quay.io/openshift-lightspeed/lightspeed-catalog:v4.18-latest`
  - Contains: References to bundles v1.0.0 through v1.0.6, channels, upgrade paths

### When to Use Each Guide

**Use the Bundle Management Guide when:**
- Creating or updating a bundle for a new operator release
- Modifying the CSV (adding permissions, descriptors, images)
- Changing bundle annotations
- Understanding bundle structure and validation
- Working with `related_images.json`

**Use this Catalog Management Guide when:**
- Adding a bundle to a catalog
- Managing multiple operator versions
- Configuring channels (alpha, stable, etc.)
- Defining upgrade paths and skip ranges
- Creating version-specific catalogs for different OpenShift releases
- Understanding how OLM discovers and serves operators

**Typical Workflow:**
1. **Development**: Make code changes
2. **Bundle** (use Bundle Management Guide): `make bundle BUNDLE_TAG=1.0.7`
3. **Catalog** (use this guide): Add bundle to catalog(s)
4. **Deploy**: Push catalog image and create CatalogSource

### Key Concepts

- **Catalog**: A collection of operator bundles organized by channels
- **Package**: An operator's identity across versions (e.g., `lightspeed-operator`)
- **Channel**: A stream of operator updates (e.g., `alpha`, `stable`)
- **Bundle**: A specific operator version (covered in Bundle Management Guide)
- **Skip Range**: Version ranges that can be skipped during upgrades
- **FBC**: File-Based Catalog format (declarative YAML)

### Prerequisites

Before using this guide, you should:
- ✅ Have a built and pushed bundle image (see [Bundle Management Guide](./olm-bundle-management.md))
- ✅ Understand bundle structure and CSV anatomy
- ✅ Know which OpenShift versions you're targeting
- ✅ Have `opm` CLI installed
- ✅ Have access to a container registry

---

## File-Based Catalogs (FBC)

### What is FBC?

File-Based Catalogs use a declarative YAML format to describe operator bundles and their relationships. This format is:
- **Human-readable**: Easy to review and edit
- **Git-friendly**: Can be version-controlled and diffed
- **Composable**: Can be split across multiple files
- **Efficient**: Faster than SQLite-based catalogs

### FBC Schema Types

FBC files contain different schema types identified by the `schema` field:

| Schema | Purpose | Example |
|--------|---------|---------|
| `olm.package` | Package metadata | Package name, icon, default channel |
| `olm.bundle` | Bundle definition | Bundle image, properties, dependencies |
| `olm.channel` | Channel definition | Channel name, entries (bundles) |

### Basic FBC Structure

```yaml
---
# Package definition (one per catalog)
schema: olm.package
name: lightspeed-operator
defaultChannel: alpha
icon:
  base64data: iVBORw0KG...
  mediatype: image/svg+xml

---
# Bundle definition (one per operator version)
schema: olm.bundle
name: lightspeed-operator.v1.0.6
package: lightspeed-operator
image: registry.redhat.io/.../lightspeed-operator-bundle@sha256:...
properties:
  - type: olm.gvk
    value:
      group: ols.openshift.io
      kind: OLSConfig
      version: v1alpha1
  - type: olm.package
    value:
      packageName: lightspeed-operator
      version: 1.0.6
relatedImages:
  - name: lightspeed-service-api
    image: registry.redhat.io/.../lightspeed-service-api@sha256:...

---
# Channel definition (one per channel)
schema: olm.channel
package: lightspeed-operator
name: alpha
entries:
  - name: lightspeed-operator.v1.0.6
    skipRange: ">=0.1.0 <1.0.6"
```

### FBC vs SQLite Comparison

| Aspect | File-Based Catalog | SQLite Catalog (Legacy) |
|--------|-------------------|------------------------|
| **Format** | YAML files | SQLite database |
| **Readability** | Human-readable | Binary format |
| **Version Control** | Git-friendly | Not git-friendly |
| **Editing** | Text editor | Special tools needed |
| **Performance** | Fast | Slower |
| **OLM Support** | OLM v0 & v1 | OLM v0 only |
| **Recommended** | ✅ Yes | ❌ Deprecated |

---

## Catalog Structure

The Lightspeed Operator project uses multiple catalog directories for different OpenShift versions:

```
lightspeed-operator/
├── lightspeed-catalog/              # Latest/development catalog
│   └── index.yaml
├── lightspeed-catalog-4.16/         # OpenShift 4.16 catalog
│   └── index.yaml
├── lightspeed-catalog-4.17/         # OpenShift 4.17 catalog
│   └── index.yaml
├── lightspeed-catalog-4.18/         # OpenShift 4.18 catalog
│   └── index.yaml
├── lightspeed-catalog-4.19/         # OpenShift 4.19 catalog
│   └── index.yaml
├── lightspeed-catalog-4.20/         # OpenShift 4.20 catalog
│   └── index.yaml
├── lightspeed-catalog.Dockerfile
├── lightspeed-catalog-4.16.Dockerfile
├── lightspeed-catalog-4.17.Dockerfile
├── lightspeed-catalog-4.18.Dockerfile
├── lightspeed-catalog-4.19.Dockerfile
└── lightspeed-catalog-4.20.Dockerfile
```

### Catalog Directory Contents

Each catalog directory contains an `index.yaml` file with the complete FBC definition:

```yaml
# lightspeed-catalog-4.18/index.yaml
---
defaultChannel: alpha
icon:
  base64data: <base64-encoded-icon>
  mediatype: image/svg+xml
name: lightspeed-operator
schema: olm.package

---
# Bundle 1
schema: olm.bundle
name: lightspeed-operator.v1.0.5
package: lightspeed-operator
image: registry.redhat.io/.../bundle@sha256:...
properties: [...]
relatedImages: [...]

---
# Bundle 2
schema: olm.bundle
name: lightspeed-operator.v1.0.6
package: lightspeed-operator
image: registry.redhat.io/.../bundle@sha256:...
properties: [...]
relatedImages: [...]

---
# Channel
schema: olm.channel
package: lightspeed-operator
name: alpha
entries:
  - name: lightspeed-operator.v1.0.6
    skipRange: ">=0.1.0 <1.0.6"
```

### Catalog Dockerfiles

Each catalog has a corresponding Dockerfile:

```dockerfile
# lightspeed-catalog-4.18.Dockerfile
FROM registry.redhat.io/openshift4/ose-operator-registry-rhel9:v4.18

# Configure the entrypoint and command
ENTRYPOINT ["/bin/opm"]
CMD ["serve", "/configs", "--cache-dir=/tmp/cache"]

# Copy declarative config root into image at /configs and pre-populate serve cache
ADD lightspeed-catalog-4.18 /configs
RUN ["/bin/opm", "serve", "/configs", "--cache-dir=/tmp/cache", "--cache-only"]

# Set DC-specific label for the location of the DC root directory
LABEL operators.operatorframework.io.index.configs.v1=/configs
```

**Key Components:**
- **Base Image**: Version-specific OCP operator-registry image
- **Configs**: Catalog YAML copied to `/configs`
- **Cache**: Pre-populated for faster startup
- **Label**: Tells OLM where to find the catalog

---

## Multi-Version Catalog Strategy

### Why Multiple Catalogs?

The Lightspeed Operator maintains separate catalogs for each supported OpenShift version:

**Benefits:**
1. **Version-specific content**: Different bundles/features per OCP version
2. **Independent upgrades**: Update one OCP version without affecting others
3. **Compatibility testing**: Test against specific OCP versions
4. **Rollback capability**: Revert specific OCP version catalogs
5. **Bundle migration**: Handle breaking changes between OCP versions

### OpenShift Version to Catalog Mapping

| OpenShift Version | Catalog Directory | Base Image | Bundle Migration |
|-------------------|-------------------|------------|------------------|
| 4.16 | `lightspeed-catalog-4.16/` | `ose-operator-registry-rhel9:v4.16` | No |
| 4.17+ | `lightspeed-catalog-4.17/` | `ose-operator-registry-rhel9:v4.17` | Yes (recommended) |
| 4.18 | `lightspeed-catalog-4.18/` | `ose-operator-registry-rhel9:v4.18` | Yes |
| 4.19 | `lightspeed-catalog-4.19/` | `ose-operator-registry-rhel9:v4.19` | Yes |
| 4.20 | `lightspeed-catalog-4.20/` | `ose-operator-registry-rhel9:v4.20` | Yes |

### Bundle Object Migration

Starting with OpenShift 4.17, OLM changed how bundle metadata is stored:

**Before 4.17** (Bundle Object):
- Bundle metadata stored as separate Kubernetes objects
- `olm.bundle.object` properties in FBC

**4.17+** (CSV Metadata):
- Bundle metadata embedded in ClusterServiceVersion
- Migrated using `--migrate-level=bundle-object-to-csv-metadata`
- More efficient, reduces object count

**Example Migration:**

```yaml
# Pre-4.17: Bundle object property
properties:
  - type: olm.bundle.object
    value:
      data: <base64-encoded-servicemonitor>

# Post-4.17: Metadata embedded in CSV
# (handled automatically by opm with migration flag)
```

### Catalog Lifecycle

```
Development
    ↓
Bundle Build
    ↓
Add to Dev Catalog (lightspeed-catalog/)
    ↓
Testing
    ↓
Add to Version-Specific Catalogs
    ├── 4.16 (no migration)
    ├── 4.17 (with migration)
    ├── 4.18 (with migration)
    ├── 4.19 (with migration)
    └── 4.20 (with migration)
    ↓
Build Catalog Images
    ↓
Push to Registry
    ↓
Deploy to Clusters
```

---

## Channel Management

### What are Channels?

Channels represent different stability/support levels for operator updates:

| Channel | Purpose | Typical Use |
|---------|---------|-------------|
| `alpha` | Early access, frequent updates | Testing, development |
| `beta` | Pre-release, stable features | QA, staging |
| `stable` | Production-ready, LTS | Production |
| `fast` | Quick updates, latest features | Early adopters |
| `candidate` | Release candidate testing | Pre-production validation |

### Current Lightspeed Channels

Lightspeed Operator currently uses:
- **`alpha`**: Primary channel for all releases

**Future Channels** (planned):
- **`stable`**: Production releases
- **`fast`**: Latest stable features

### Channel Definition

```yaml
schema: olm.channel
package: lightspeed-operator
name: alpha
entries:
  - name: lightspeed-operator.v1.0.6
    skipRange: ">=0.1.0 <1.0.6"
  - name: lightspeed-operator.v1.0.5
    skipRange: ">=0.1.0 <1.0.5"
```

**Channel Properties:**

| Field | Description | Example |
|-------|-------------|---------|
| `package` | Package name | `lightspeed-operator` |
| `name` | Channel name | `alpha` |
| `entries` | List of bundle versions | See below |

**Entry Properties:**

| Field | Description | Required |
|-------|-------------|----------|
| `name` | Bundle name | Yes |
| `replaces` | Previous version this replaces | No |
| `skips` | Versions to skip | No |
| `skipRange` | Version range to skip | No |

### Adding a New Channel

**Step 1: Update bundle annotations**

```yaml
# bundle/metadata/annotations.yaml
annotations:
  operators.operatorframework.io.bundle.channels.v1: alpha,stable
  operators.operatorframework.io.bundle.channel.default.v1: stable
```

**Step 2: Add channel to catalog**

```yaml
# In index.yaml
---
schema: olm.channel
package: lightspeed-operator
name: stable
entries:
  - name: lightspeed-operator.v1.0.6
    skipRange: ">=1.0.0 <1.0.6"
```

**Step 3: Update package default channel (optional)**

```yaml
schema: olm.package
name: lightspeed-operator
defaultChannel: stable  # Changed from alpha
```

### Channel Promotion Workflow

```
Development → alpha channel
    ↓
Testing passes
    ↓
Promote to beta channel
    ↓
QA validation
    ↓
Promote to stable channel
    ↓
Production deployment
```

**Promotion Script Example:**

```bash
# Add bundle to new channel
cat >> lightspeed-catalog/index.yaml <<EOF
---
schema: olm.channel
package: lightspeed-operator
name: stable
entries:
  - name: lightspeed-operator.v1.0.6
    skipRange: ">=1.0.0 <1.0.6"
EOF

# Validate
opm validate lightspeed-catalog/
```

---

## Skip Ranges and Upgrade Paths

### Understanding Skip Ranges

Skip ranges allow upgrades to skip intermediate versions, enabling:
- **Direct upgrades**: 1.0.0 → 1.0.6 without installing 1.0.1-1.0.5
- **Faster upgrades**: Fewer intermediate steps
- **Reduced testing**: Less upgrade paths to test

### Skip Range Syntax

```yaml
skipRange: ">=0.1.0 <1.0.6"
```

**Format:** `[operator] [version] [operator] [version]`

**Operators:**
- `=` : Exact version
- `>` : Greater than
- `>=` : Greater than or equal
- `<` : Less than
- `<=` : Less than or equal
- `!=` : Not equal

**Examples:**

| Skip Range | Meaning | Versions Skipped |
|------------|---------|-----------------|
| `>=0.1.0 <1.0.6` | All versions from 0.1.0 up to (but not including) 1.0.6 | 0.1.0 → 1.0.5 |
| `>=1.0.0 <1.1.0` | All 1.0.x versions | 1.0.0 → 1.0.999 |
| `>1.0.0 <=1.5.0` | From 1.0.1 through 1.5.0 | 1.0.1 → 1.5.0 |
| `>=1.0.0 !=1.3.0 <2.0.0` | All 1.x except 1.3.0 | 1.0.0 → 1.9.9 (skip 1.3.0) |

### Skip Range Best Practices

**1. Cover all previous versions:**
```yaml
# Good: Covers everything before this version
skipRange: ">=0.1.0 <1.0.6"

# Bad: Leaves gaps
skipRange: ">=1.0.0 <1.0.6"  # Missing 0.x versions
```

**2. Use consistent patterns:**
```yaml
# v1.0.5
skipRange: ">=0.1.0 <1.0.5"

# v1.0.6
skipRange: ">=0.1.0 <1.0.6"  # Same pattern, new upper bound

# v1.0.7
skipRange: ">=0.1.0 <1.0.7"  # Consistent
```

**3. Don't skip breaking changes:**
```yaml
# v2.0.0 with breaking changes
# Don't use skipRange that skips across major versions
# Force users through v1.x.x first

entries:
  - name: operator.v2.0.0
    replaces: operator.v1.9.9  # Explicit upgrade path, no skip range
```

**4. Test skip range upgrades:**
```bash
# Test direct upgrade from oldest to newest
oc create -f catalogsource.yaml
oc create -f subscription.yaml  # Install oldest version
# Verify installation
oc patch subscription ... --type=merge -p '{"spec":{"startingCSV":"operator.v1.0.6"}}'
# Verify upgrade succeeds
```

### Upgrade Path Examples

**Linear Path (without skip range):**
```yaml
# v1.0.1
entries:
  - name: operator.v1.0.1

# v1.0.2
entries:
  - name: operator.v1.0.2
    replaces: operator.v1.0.1

# v1.0.3
entries:
  - name: operator.v1.0.3
    replaces: operator.v1.0.2

# Required path: 1.0.1 → 1.0.2 → 1.0.3
```

**Skip Range Path:**
```yaml
# v1.0.3
entries:
  - name: operator.v1.0.3
    skipRange: ">=1.0.0 <1.0.3"

# Allowed paths:
# - 1.0.1 → 1.0.3 (direct)
# - 1.0.2 → 1.0.3 (direct)
```

**Complex Path with Multiple Versions:**
```yaml
schema: olm.channel
name: alpha
entries:
  # Latest version
  - name: operator.v1.0.6
    skipRange: ">=0.1.0 <1.0.6"
  
  # Still available for rollback/testing
  - name: operator.v1.0.5
    skipRange: ">=0.1.0 <1.0.5"
  
  - name: operator.v1.0.4
    skipRange: ">=0.1.0 <1.0.4"

# Upgrade paths:
# 0.x.x → 1.0.6 (direct)
# 1.0.4 → 1.0.6 (direct)
# 1.0.5 → 1.0.6 (direct)
```

---

## Catalog Building Workflow

### Build Scripts

The project provides scripts for catalog management:

| Script | Purpose | Usage |
|--------|---------|-------|
| `hack/bundle_to_catalog.sh` | Add bundle to catalog | CI/CD, releases |
| `hack/snapshot_to_catalog.sh` | Create catalog from Konflux snapshot | Konflux integration |
| `hack/snapshot_to_image_list.sh` | Extract images from snapshot | Image management |

### Manual Catalog Building

**Step 1: Prepare bundle**

```bash
# Build and push bundle
make bundle BUNDLE_TAG=1.0.7
make bundle-build BUNDLE_IMG=quay.io/org/bundle:v1.0.7
make bundle-push BUNDLE_IMG=quay.io/org/bundle:v1.0.7
```

**Step 2: Initialize catalog**

```yaml
# Create lightspeed-catalog-4.18/index.yaml
---
defaultChannel: alpha
icon:
  base64data: <icon-data>
  mediatype: image/svg+xml
name: lightspeed-operator
schema: olm.package
```

**Step 3: Render bundle**

```bash
# Render bundle to FBC format
opm render quay.io/org/bundle:v1.0.7 --output=yaml > bundle.yaml

# For OCP 4.17+, use migration
opm render quay.io/org/bundle:v1.0.7 \
  --migrate-level=bundle-object-to-csv-metadata \
  --output=yaml > bundle.yaml
```

**Step 4: Add bundle to catalog**

```bash
# Append bundle to catalog
cat bundle.yaml >> lightspeed-catalog-4.18/index.yaml
```

**Step 5: Add channel entry**

```yaml
# Append to index.yaml
---
schema: olm.channel
package: lightspeed-operator
name: alpha
entries:
  - name: lightspeed-operator.v1.0.7
    skipRange: ">=0.1.0 <1.0.7"
```

**Step 6: Validate catalog**

```bash
opm validate lightspeed-catalog-4.18/
```

**Step 7: Build catalog image**

```bash
podman build \
  -f lightspeed-catalog-4.18.Dockerfile \
  -t quay.io/org/lightspeed-catalog:v4.18-1.0.7 \
  .
```

**Step 8: Push catalog image**

```bash
podman push quay.io/org/lightspeed-catalog:v4.18-1.0.7
```

### Automated Catalog Building

Using `hack/bundle_to_catalog.sh`:

```bash
#!/bin/bash

# Add bundle from Konflux snapshot to catalog
./hack/bundle_to_catalog.sh \
  -b ols-bundle-abc123 \              # Bundle snapshot reference
  -i related_images.json \            # Related images file
  -c lightspeed-catalog-4.18/index.yaml \  # Target catalog
  -n alpha \                          # Channel name
  -m                                  # Enable migration for 4.17+

# Script does:
# 1. Fetches bundle image from Konflux
# 2. Renders bundle with optional migration
# 3. Adds to specified catalog
# 4. Creates/updates channel entry
# 5. Validates result
```

**Script Parameters:**

| Parameter | Description | Required | Example |
|-----------|-------------|----------|---------|
| `-b` | Bundle snapshot reference | Yes | `ols-bundle-2dhtr` |
| `-i` | Related images JSON file | Yes | `related_images.json` |
| `-c` | Catalog file to update | Yes | `lightspeed-catalog-4.18/index.yaml` |
| `-n` | Channel names (comma-separated) | No (default: `alpha`) | `alpha,stable` |
| `-m` | Enable bundle migration | No | Use for OCP 4.17+ |

### Multi-Catalog Build Workflow

For releases, build all catalog versions:

```bash
#!/bin/bash

BUNDLE_IMAGE="quay.io/openshift-lightspeed/bundle:v1.0.7"
VERSION="1.0.7"

# Build for each OpenShift version
for ocp_version in 4.16 4.17 4.18 4.19 4.20; do
  echo "Building catalog for OpenShift ${ocp_version}"
  
  # Determine if migration is needed
  MIGRATE_FLAG=""
  if [[ $(echo "${ocp_version} >= 4.17" | bc -l) -eq 1 ]]; then
    MIGRATE_FLAG="--migrate-level=bundle-object-to-csv-metadata"
  fi
  
  # Render bundle
  opm render ${BUNDLE_IMAGE} ${MIGRATE_FLAG} --output=yaml \
    > bundle-${ocp_version}.yaml
  
  # Add to catalog
  cat bundle-${ocp_version}.yaml >> lightspeed-catalog-${ocp_version}/index.yaml
  
  # Add channel entry
  cat >> lightspeed-catalog-${ocp_version}/index.yaml <<EOF
---
schema: olm.channel
package: lightspeed-operator
name: alpha
entries:
  - name: lightspeed-operator.v${VERSION}
    skipRange: ">=0.1.0 <${VERSION}"
EOF
  
  # Validate
  opm validate lightspeed-catalog-${ocp_version}/
  
  # Build image
  podman build \
    -f lightspeed-catalog-${ocp_version}.Dockerfile \
    -t quay.io/openshift-lightspeed/lightspeed-catalog:v${ocp_version}-${VERSION} \
    .
  
  # Push image
  podman push quay.io/openshift-lightspeed/lightspeed-catalog:v${ocp_version}-${VERSION}
done
```

---

## Bundle to Catalog Migration

### Bundle Object to CSV Metadata

OpenShift 4.17 introduced a new way to store bundle metadata, migrating from separate bundle objects to CSV-embedded metadata.

### Why Migrate?

**Before 4.17 (Bundle Object):**
- Each bundle resource (ServiceMonitor, Service, Role, etc.) stored as property
- Large number of Kubernetes objects
- Higher memory usage
- Slower catalog processing

**After 4.17 (CSV Metadata):**
- Metadata embedded in ClusterServiceVersion
- Fewer Kubernetes objects
- Lower memory usage
- Faster catalog processing
- Required for OCP 4.17+ compatibility

### Migration Process

**Using `opm render` with migration flag:**

```bash
# Without migration (OCP 4.16)
opm render quay.io/org/bundle:v1.0.7 --output=yaml > bundle.yaml

# With migration (OCP 4.17+)
opm render quay.io/org/bundle:v1.0.7 \
  --migrate-level=bundle-object-to-csv-metadata \
  --output=yaml > bundle.yaml
```

**What Changes:**

**Before Migration:**
```yaml
schema: olm.bundle
properties:
  - type: olm.bundle.object
    value:
      data: eyJhcGlWZXJzaW9uIjoi...  # Base64-encoded Service
  - type: olm.bundle.object
    value:
      data: eyJhcGlWZXJzaW9uIjoi...  # Base64-encoded ServiceMonitor
  # ... many more objects
```

**After Migration:**
```yaml
schema: olm.bundle
properties:
  - type: olm.package
    value:
      packageName: lightspeed-operator
      version: 1.0.7
  - type: olm.gvk
    value:
      group: ols.openshift.io
      kind: OLSConfig
      version: v1alpha1
  # Objects are now embedded in the CSV itself
```

### Backward Compatibility

**Catalogs with migration work on:**
- ✅ OpenShift 4.17+
- ✅ OpenShift 4.18+
- ✅ OpenShift 4.19+
- ✅ OpenShift 4.20+

**Catalogs without migration work on:**
- ✅ OpenShift 4.16
- ⚠️ OpenShift 4.17+ (deprecated, may be removed)

**Recommendation:** Use separate catalogs per OCP version for maximum compatibility.

### Migration in CI/CD

```yaml
# .github/workflows/build-catalog.yml
- name: Build catalog for OCP 4.18+
  run: |
    opm render ${BUNDLE_IMAGE} \
      --migrate-level=bundle-object-to-csv-metadata \
      --output=yaml >> lightspeed-catalog-4.18/index.yaml

- name: Build catalog for OCP 4.16
  run: |
    opm render ${BUNDLE_IMAGE} \
      --output=yaml >> lightspeed-catalog-4.16/index.yaml
```

---

## Catalog Validation

### Validation Tools

**1. OPM Validate**

```bash
# Validate entire catalog directory
opm validate lightspeed-catalog-4.18/

# Validate specific file
opm validate lightspeed-catalog-4.18/index.yaml
```

**Common Validation Errors:**

```
Error: invalid bundle "lightspeed-operator.v1.0.7": 
  missing required property "olm.package"
```

**Fix:** Ensure bundle has package property:
```yaml
properties:
  - type: olm.package
    value:
      packageName: lightspeed-operator
      version: 1.0.7
```

**2. YAML Syntax Check**

```bash
# Check YAML syntax
yamllint lightspeed-catalog-4.18/index.yaml

# Or use yq
yq eval '.' lightspeed-catalog-4.18/index.yaml > /dev/null
```

**3. Schema Validation**

Ensure all entries have required schema types:

```bash
# Check for required schemas
yq eval '[.[] | select(.schema == "olm.package")] | length' index.yaml  # Should be 1
yq eval '[.[] | select(.schema == "olm.bundle")] | length' index.yaml   # Should be >= 1
yq eval '[.[] | select(.schema == "olm.channel")] | length' index.yaml  # Should be >= 1
```

### Pre-Build Validation Checklist

- [ ] All bundles have valid `image` references
- [ ] All bundle names match `<package>.v<version>` format
- [ ] Package definition exists with `defaultChannel`
- [ ] All channels reference existing bundles
- [ ] Skip ranges cover appropriate version ranges
- [ ] Related images use digests (SHA256)
- [ ] No duplicate bundle names
- [ ] YAML syntax is valid
- [ ] `opm validate` passes

### Post-Build Validation

After building catalog image:

```bash
# Pull and inspect catalog image
podman pull quay.io/org/lightspeed-catalog:v4.18-1.0.7

# Extract catalog
podman run --rm \
  -v $(pwd):/output:z \
  quay.io/org/lightspeed-catalog:v4.18-1.0.7 \
  cp -r /configs /output/

# Validate extracted catalog
opm validate ./configs/

# Test catalog serves correctly
podman run -p 50051:50051 \
  quay.io/org/lightspeed-catalog:v4.18-1.0.7

# Query catalog (in another terminal)
grpcurl -plaintext localhost:50051 api.Registry/ListPackages
```

---

## Common Tasks

### Task 1: Add New Bundle to Existing Catalog

```bash
# 1. Render bundle
opm render quay.io/org/bundle:v1.0.8 \
  --migrate-level=bundle-object-to-csv-metadata \
  --output=yaml > new-bundle.yaml

# 2. Add to catalog
cat new-bundle.yaml >> lightspeed-catalog-4.18/index.yaml

# 3. Update channel (add to top of entries list)
yq eval '.[] | select(.schema == "olm.channel" and .name == "alpha") | .entries' \
  lightspeed-catalog-4.18/index.yaml

# Manually edit to add:
#   - name: lightspeed-operator.v1.0.8
#     skipRange: ">=0.1.0 <1.0.8"

# 4. Validate
opm validate lightspeed-catalog-4.18/

# 5. Rebuild catalog image
podman build -f lightspeed-catalog-4.18.Dockerfile \
  -t quay.io/org/lightspeed-catalog:v4.18-latest .
```

### Task 2: Create Catalog for New OpenShift Version

```bash
# 1. Copy existing catalog as template
cp -r lightspeed-catalog-4.19 lightspeed-catalog-4.20

# 2. Copy and update Dockerfile
cp lightspeed-catalog-4.19.Dockerfile lightspeed-catalog-4.20.Dockerfile

# 3. Update Dockerfile base image
sed -i 's/v4.19/v4.20/g' lightspeed-catalog-4.20.Dockerfile
sed -i 's/4.19/4.20/g' lightspeed-catalog-4.20.Dockerfile

# 4. Validate
opm validate lightspeed-catalog-4.20/

# 5. Build
podman build -f lightspeed-catalog-4.20.Dockerfile \
  -t quay.io/org/lightspeed-catalog:v4.20-1.0.7 .
```

### Task 3: Remove Bundle from Catalog

```bash
# 1. Back up catalog
cp lightspeed-catalog-4.18/index.yaml lightspeed-catalog-4.18/index.yaml.backup

# 2. Remove bundle entry
yq eval 'del(.[] | select(.schema == "olm.bundle" and .name == "lightspeed-operator.v1.0.5"))' \
  lightspeed-catalog-4.18/index.yaml > temp.yaml
mv temp.yaml lightspeed-catalog-4.18/index.yaml

# 3. Remove from channel entries
yq eval '(.[] | select(.schema == "olm.channel" and .name == "alpha") | .entries) |= 
  map(select(.name != "lightspeed-operator.v1.0.5"))' \
  lightspeed-catalog-4.18/index.yaml > temp.yaml
mv temp.yaml lightspeed-catalog-4.18/index.yaml

# 4. Validate
opm validate lightspeed-catalog-4.18/

# 5. Rebuild
podman build -f lightspeed-catalog-4.18.Dockerfile \
  -t quay.io/org/lightspeed-catalog:v4.18-latest .
```

### Task 4: Test Catalog Locally

```bash
# 1. Build catalog image
podman build -f lightspeed-catalog-4.18.Dockerfile \
  -t localhost/lightspeed-catalog:test .

# 2. Run catalog server
podman run -d --name catalog-server \
  -p 50051:50051 \
  localhost/lightspeed-catalog:test

# 3. Create CatalogSource
cat <<EOF | oc apply -f -
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: lightspeed-test-catalog
  namespace: openshift-marketplace
spec:
  sourceType: grpc
  address: host.containers.internal:50051
  displayName: Lightspeed Test Catalog
  publisher: Test
EOF

# 4. Wait for catalog to be ready
oc get catalogsource lightspeed-test-catalog -n openshift-marketplace

# 5. Check available packages
oc get packagemanifests | grep lightspeed

# 6. Clean up
oc delete catalogsource lightspeed-test-catalog -n openshift-marketplace
podman stop catalog-server
podman rm catalog-server
```

### Task 5: Update Related Images in Catalog

```bash
# 1. Extract current catalog
cp lightspeed-catalog-4.18/index.yaml index-backup.yaml

# 2. Update images in related_images.json
vim related_images.json

# 3. Regenerate bundle with new images
./hack/update_bundle.sh -v 1.0.7 -i related_images.json

# 4. Re-render bundle to catalog
opm render $(yq '.[] | select(.name == "lightspeed-operator-bundle") | .image' related_images.json) \
  --migrate-level=bundle-object-to-csv-metadata \
  --output=yaml > new-bundle.yaml

# 5. Replace old bundle in catalog
# (Remove old, add new as shown in Task 3 and Task 1)

# 6. Validate and rebuild
opm validate lightspeed-catalog-4.18/
podman build -f lightspeed-catalog-4.18.Dockerfile \
  -t quay.io/org/lightspeed-catalog:v4.18-1.0.7 .
```

---

## Troubleshooting

### Issue: Catalog Validation Fails

**Symptom:**
```
Error: invalid catalog: package "lightspeed-operator" has no channels
```

**Diagnosis:**
```bash
# Check if channel exists
yq eval '.[] | select(.schema == "olm.channel")' lightspeed-catalog-4.18/index.yaml
```

**Fix:**
```bash
# Add missing channel
cat >> lightspeed-catalog-4.18/index.yaml <<EOF
---
schema: olm.channel
package: lightspeed-operator
name: alpha
entries:
  - name: lightspeed-operator.v1.0.7
    skipRange: ">=0.1.0 <1.0.7"
EOF
```

### Issue: Bundle Not Appearing in Catalog

**Symptom:** Bundle image built and pushed, but not showing in catalog

**Diagnosis:**
```bash
# Check if bundle is in catalog
yq eval '.[] | select(.schema == "olm.bundle" and .name == "lightspeed-operator.v1.0.7")' \
  lightspeed-catalog-4.18/index.yaml

# Check if bundle is in channel
yq eval '.[] | select(.schema == "olm.channel" and .name == "alpha") | .entries' \
  lightspeed-catalog-4.18/index.yaml
```

**Fix:**
Ensure both bundle entry AND channel entry exist (see Task 1)

### Issue: Skip Range Not Working

**Symptom:** OLM won't upgrade from old version to new despite skip range

**Diagnosis:**
```bash
# Check skip range syntax
yq eval '.[] | select(.schema == "olm.channel") | .entries[] | select(.name == "lightspeed-operator.v1.0.7") | .skipRange' \
  lightspeed-catalog-4.18/index.yaml

# Verify version is actually in range
# Example: skipRange ">=0.1.0 <1.0.7" should match 1.0.6 but not 1.0.7
```

**Common Issues:**
- Skip range doesn't include installed version
- Skip range syntax error
- Multiple versions in channel with conflicting ranges

**Fix:**
```yaml
# Ensure skip range covers installed versions
entries:
  - name: lightspeed-operator.v1.0.7
    skipRange: ">=0.1.0 <1.0.7"  # Includes 0.1.0 through 1.0.6
```

### Issue: Catalog Image Won't Build

**Symptom:**
```
Error: stat lightspeed-catalog-4.18: no such file or directory
```

**Diagnosis:**
```bash
# Check if catalog directory exists
ls -la lightspeed-catalog-4.18/

# Check Dockerfile references
cat lightspeed-catalog-4.18.Dockerfile | grep ADD
```

**Fix:**
```bash
# Ensure catalog directory exists and has content
mkdir -p lightspeed-catalog-4.18
# Add index.yaml as shown earlier

# Verify Dockerfile references correct directory
```

### Issue: Migration Flag Not Applied

**Symptom:** Catalog for OCP 4.17+ still has `olm.bundle.object` properties

**Diagnosis:**
```bash
# Check if bundle objects exist
yq eval '.[] | select(.schema == "olm.bundle") | .properties[] | select(.type == "olm.bundle.object")' \
  lightspeed-catalog-4.18/index.yaml | head
```

**Fix:**
```bash
# Re-render with migration flag
opm render ${BUNDLE_IMAGE} \
  --migrate-level=bundle-object-to-csv-metadata \
  --output=yaml > bundle.yaml

# Replace in catalog
# (Remove old bundle, add new one)
```

### Issue: Conflicting Versions in Channel

**Symptom:**
```
Error: multiple bundles provide the same APIs
```

**Diagnosis:**
```bash
# Check for duplicate GVKs
yq eval '.[] | select(.schema == "olm.bundle") | {name, gvks: (.properties[] | select(.type == "olm.gvk") | .value)}' \
  lightspeed-catalog-4.18/index.yaml
```

**Fix:**
- Ensure each bundle in a channel has unique version
- Don't include multiple versions that manage the same CRs simultaneously
- Use `replaces` or `skipRange` to define clear upgrade paths

---

## Best Practices

### 1. Catalog Organization

✅ **Do:**
- Separate catalogs per OpenShift version
- Use consistent directory naming
- Keep catalog files in version control
- Document catalog structure

❌ **Don't:**
- Mix multiple OpenShift versions in one catalog
- Manually edit complex YAML without backup
- Skip validation steps
- Forget to update Dockerfiles

### 2. Version Management

✅ **Do:**
- Use semantic versioning
- Define clear skip ranges
- Test upgrade paths
- Document breaking changes

❌ **Don't:**
- Skip versions arbitrarily
- Create gaps in version coverage
- Use skip ranges across major versions
- Forget to update channel entries

### 3. Image References

✅ **Do:**
- Use image digests (SHA256) in production
- Maintain `related_images.json`
- Update all catalog versions
- Verify images are accessible

❌ **Don't:**
- Use `:latest` tag in production
- Mix tags and digests
- Forget to update related images
- Reference images from untrusted registries

### 4. Testing

✅ **Do:**
- Validate catalogs before pushing
- Test in staging environment
- Verify upgrade paths
- Check catalog serves correctly

❌ **Don't:**
- Push untested catalogs to production
- Skip validation
- Assume upgrades work without testing
- Deploy without rollback plan

### 5. Documentation

✅ **Do:**
- Document catalog structure
- Explain channel strategy
- Note breaking changes
- Keep upgrade matrix updated

❌ **Don't:**
- Leave catalogs undocumented
- Skip release notes
- Forget to update version mappings
- Ignore feedback from users

---

## Additional Resources

### Related Guides

- **[OLM Bundle Management Guide](./olm-bundle-management.md)** - Learn about creating and managing bundles (prerequisite for this guide)
- **[Contributing Guide](../CONTRIBUTING.md)** - General contribution guidelines
- **[Architecture Documentation](../ARCHITECTURE.md)** - Operator architecture overview

### External Resources

- [OLM File-Based Catalogs Documentation](https://olm.operatorframework.io/docs/reference/file-based-catalogs/)
- [OPM CLI Reference](https://docs.openshift.com/container-platform/latest/cli_reference/opm/cli-opm-ref.html)
- [Operator SDK Catalog Integration](https://sdk.operatorframework.io/docs/olm-integration/generation/)
- [OpenShift Operator Certification](https://redhat-connect.gitbook.io/certified-operator-guide/)

### Project Scripts

**Lightspeed Implementation:**
- [`hack/bundle_to_catalog.sh`](../hack/bundle_to_catalog.sh) - Bundle to catalog automation
- [`hack/snapshot_to_catalog.sh`](../hack/snapshot_to_catalog.sh) - Konflux snapshot integration
- [`hack/snapshot_to_image_list.sh`](../hack/snapshot_to_image_list.sh) - Image extraction utility
- [`hack/update_bundle.sh`](../hack/update_bundle.sh) - Bundle generation and updates

---

## Quick Reference

### Catalog Validation

```bash
# Validate catalog
opm validate lightspeed-catalog-4.18/

# Check YAML syntax
yamllint lightspeed-catalog-4.18/index.yaml

# Test catalog server
podman run -p 50051:50051 quay.io/org/catalog:v4.18-1.0.7
```

### Bundle Rendering

```bash
# OCP 4.16 (no migration)
opm render ${BUNDLE_IMAGE} --output=yaml > bundle.yaml

# OCP 4.17+ (with migration)
opm render ${BUNDLE_IMAGE} \
  --migrate-level=bundle-object-to-csv-metadata \
  --output=yaml > bundle.yaml
```

### Catalog Building

```bash
# Build catalog image
podman build -f lightspeed-catalog-4.18.Dockerfile \
  -t quay.io/org/catalog:v4.18-1.0.7 .

# Push catalog image
podman push quay.io/org/catalog:v4.18-1.0.7
```

### Query Catalog

```bash
# List packages
yq eval '.[] | select(.schema == "olm.package") | .name' index.yaml

# List bundles
yq eval '.[] | select(.schema == "olm.bundle") | .name' index.yaml

# List channels
yq eval '.[] | select(.schema == "olm.channel") | .name' index.yaml

# Get bundle version
yq eval '.[] | select(.schema == "olm.bundle" and .name == "lightspeed-operator.v1.0.7") | .properties[] | select(.type == "olm.package") | .value.version' index.yaml
```

