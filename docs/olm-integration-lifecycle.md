# OLM Integration & Operator Lifecycle Guide

> **Part of the OLM Documentation Series:**
> 1. [Bundle Management](./olm-bundle-management.md) - Creating and managing operator bundles
> 2. [Catalog Management](./olm-catalog-management.md) - Organizing bundles into catalogs
> 3. **Integration & Lifecycle** ← You are here
> 4. Testing & Validation (coming soon)

This guide explains how Operator Lifecycle Manager (OLM) integrates with your operator and manages its complete lifecycle from installation through upgrades and eventual removal.

---

## Table of Contents

- [Overview](#overview)
- [OLM Architecture](#olm-architecture)
- [Installation Workflow](#installation-workflow)
- [CatalogSource Integration](#catalogsource-integration)
- [Subscription & InstallPlan](#subscription--installplan)
- [Operator Lifecycle States](#operator-lifecycle-states)
- [Upgrade Mechanisms](#upgrade-mechanisms)
- [Watch & Reconciliation](#watch--reconciliation)
- [Dependency Resolution](#dependency-resolution)
- [RBAC & Permissions](#rbac--permissions)
- [Monitoring Integration](#monitoring-integration)
- [Uninstallation](#uninstallation)
- [Common Patterns](#common-patterns)
- [Troubleshooting](#troubleshooting)

---

## Overview

### What is OLM?

Operator Lifecycle Manager (OLM) is a component of Kubernetes/OpenShift that manages the lifecycle of operators in a cluster. It handles:

- **Discovery**: Making operators available through OperatorHub
- **Installation**: Deploying operators and their dependencies
- **Upgrades**: Automatically updating operators to new versions
- **RBAC**: Managing permissions required by operators
- **Dependency Resolution**: Ensuring required dependencies are present
- **Health Monitoring**: Tracking operator status and health

### Relationship to Previous Guides

```
┌─────────────────────────────────────────────────────────────────┐
│              Complete Operator Distribution Flow                 │
└─────────────────────────────────────────────────────────────────┘

1. Bundle Creation [Guide #1]
   └── Package operator version with CSV, CRDs, RBAC
          ↓
2. Catalog Creation [Guide #2]
   └── Organize bundles into channels with upgrade paths
          ↓
3. OLM Integration [This Guide]
   ├── CatalogSource: Make catalog available to cluster
   ├── Subscription: Request operator installation
   ├── InstallPlan: Execute installation/upgrade
   ├── CSV: Operator running and managed
   └── Operator: Reconcile custom resources
          ↓
4. User Interaction
   └── Create/Update custom resources (e.g., OLSConfig)
```

### Prerequisites

Before using this guide:
- ✅ Understand [Bundle Management](./olm-bundle-management.md) - CSV structure, annotations
- ✅ Understand [Catalog Management](./olm-catalog-management.md) - Channels, FBC format
- ✅ Have a catalog deployed or access to OpenShift OperatorHub
- ✅ Cluster admin access (for installation)

---

## OLM Architecture

### Core Components

```
┌───────────────────────────────────────────────────────────────┐
│                      OLM Architecture                          │
└───────────────────────────────────────────────────────────────┘

┌─────────────────┐    ┌──────────────────┐    ┌──────────────┐
│  CatalogSource  │───▶│  PackageServer   │───▶│ OperatorHub  │
│  (Catalog img)  │    │  (REST API)      │    │     UI       │
└─────────────────┘    └──────────────────┘    └──────────────┘
        │
        │ provides bundles
        ▼
┌─────────────────┐    ┌──────────────────┐    ┌──────────────┐
│  Subscription   │───▶│   InstallPlan    │───▶│     CSV      │
│  (user intent)  │    │  (install steps) │    │  (operator)  │
└─────────────────┘    └──────────────────┘    └──────────────┘
        │                       │                       │
        │                       │                       │
        ▼                       ▼                       ▼
┌─────────────────────────────────────────────────────────────┐
│              OLM Operator & Catalog Operator                 │
│  - Watches Subscriptions, InstallPlans, CSVs                │
│  - Resolves dependencies                                     │
│  - Creates/Updates resources                                 │
│  - Manages upgrades                                          │
└─────────────────────────────────────────────────────────────┘
```

### Key CRDs

| CRD | Purpose | Scope | Created By |
|-----|---------|-------|------------|
| **CatalogSource** | Points to catalog image | Namespace | Admin |
| **Subscription** | Declares intent to install operator | Namespace | User/Admin |
| **InstallPlan** | Execution plan for install/upgrade | Namespace | OLM |
| **ClusterServiceVersion** | Running operator instance | Namespace | OLM |
| **OperatorGroup** | Defines operator watch scope | Namespace | Admin |

### OLM Operators

**OLM Operator (`olm-operator`)**:
- Watches: `ClusterServiceVersion`, `InstallPlan`, `Subscription`
- Responsibilities:
  - Deploys operators from CSVs
  - Manages operator lifecycle
  - Handles dependency resolution
  - Creates/manages RBAC resources

**Catalog Operator (`catalog-operator`)**:
- Watches: `CatalogSource`, `Subscription`
- Responsibilities:
  - Syncs catalog contents
  - Resolves upgrade paths
  - Creates InstallPlans from Subscriptions
  - Watches for new bundle versions

---

## Installation Workflow

### Complete Installation Flow

```
User Action: Create Subscription
         ↓
┌─────────────────────────────────────────────────────────────┐
│ 1. Catalog Operator detects new Subscription                │
│    - Reads channel, package name, install approval          │
│    - Queries CatalogSource for available bundles            │
└─────────────────────────────────────────────────────────────┘
         ↓
┌─────────────────────────────────────────────────────────────┐
│ 2. Dependency Resolution                                     │
│    - Analyzes CSV dependencies                               │
│    - Checks if required operators are installed              │
│    - Validates install modes & RBAC requirements             │
└─────────────────────────────────────────────────────────────┘
         ↓
┌─────────────────────────────────────────────────────────────┐
│ 3. Create InstallPlan                                        │
│    - Lists all resources to create (CSV, CRDs, RBAC)        │
│    - Sets approval status (Automatic/Manual)                 │
│    - Resolves related images                                 │
└─────────────────────────────────────────────────────────────┘
         ↓
┌─────────────────────────────────────────────────────────────┐
│ 4. InstallPlan Approval (if manual)                          │
│    - Admin reviews plan                                      │
│    - Approves or rejects                                     │
└─────────────────────────────────────────────────────────────┘
         ↓
┌─────────────────────────────────────────────────────────────┐
│ 5. OLM Operator Executes InstallPlan                         │
│    a. Create CRDs (if not exists)                            │
│    b. Create RBAC (ServiceAccount, Roles, Bindings)          │
│    c. Create CSV                                             │
└─────────────────────────────────────────────────────────────┘
         ↓
┌─────────────────────────────────────────────────────────────┐
│ 6. CSV Lifecycle                                             │
│    Phase: Pending → InstallReady → Installing → Succeeded   │
│    - OLM creates Deployment from CSV spec                    │
│    - Waits for Deployment to be ready                        │
│    - CSV enters "Succeeded" phase                            │
└─────────────────────────────────────────────────────────────┘
         ↓
┌─────────────────────────────────────────────────────────────┐
│ 7. Operator Running                                          │
│    - Operator pod starts                                     │
│    - Watches for custom resources (e.g., OLSConfig)          │
│    - Ready to reconcile user workloads                       │
└─────────────────────────────────────────────────────────────┘
```

### Example: Lightspeed Operator Installation

**Real-world reference:**
- E2E Suite Setup: [`test/e2e/suite_test.go`](../test/e2e/suite_test.go) (lines 49-61) - Operator readiness check
- Installation Tests: [`test/e2e/reconciliation_test.go`](../test/e2e/reconciliation_test.go) - Post-installation verification

**Step 1: User creates Subscription**

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: lightspeed-operator
  namespace: openshift-lightspeed
spec:
  channel: alpha
  name: lightspeed-operator
  source: lightspeed-catalog
  sourceNamespace: openshift-marketplace
  installPlanApproval: Automatic
```

**Step 2: OLM creates InstallPlan**

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: InstallPlan
metadata:
  name: install-abcde
  namespace: openshift-lightspeed
spec:
  approval: Automatic
  approved: true
  clusterServiceVersionNames:
    - lightspeed-operator.v1.0.6
  generation: 1
status:
  phase: Complete
  catalogSources:
    - lightspeed-catalog
  plan:
    - resolving: lightspeed-operator.v1.0.6
      resource:
        kind: ClusterServiceVersion
        name: lightspeed-operator.v1.0.6
        manifest: |
          # Full CSV content
    - resolving: lightspeed-operator.v1.0.6
      resource:
        kind: CustomResourceDefinition
        name: olsconfigs.ols.openshift.io
        manifest: |
          # Full CRD content
```

**Step 3: CSV Created and Operator Deployed**

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: lightspeed-operator.v1.0.6
  namespace: openshift-lightspeed
spec:
  install:
    strategy: deployment
    spec:
      deployments:
        - name: lightspeed-operator-controller-manager
          spec:
            replicas: 1
            selector:
              matchLabels:
                control-plane: controller-manager
            template:
              # Pod template
status:
  phase: Succeeded
  reason: InstallSucceeded
  conditions:
    - type: Ready
      status: "True"
```

---

## CatalogSource Integration

### Creating a CatalogSource

A `CatalogSource` makes your catalog available to the cluster.

**For Custom Catalogs:**

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: lightspeed-catalog
  namespace: openshift-marketplace
spec:
  sourceType: grpc
  image: quay.io/openshift-lightspeed/lightspeed-catalog:v4.18-latest
  displayName: OpenShift Lightspeed Operators
  publisher: Red Hat
  updateStrategy:
    registryPoll:
      interval: 30m
```

**Key Fields:**

| Field | Description | Values |
|-------|-------------|--------|
| `sourceType` | How catalog is served | `grpc` (FBC), `internal` (built-in) |
| `image` | Catalog container image | Registry path |
| `updateStrategy.registryPoll.interval` | How often to check for updates | Duration (e.g., `30m`, `1h`) |
| `priority` | Preference when multiple catalogs have same package | Integer (-100 to 100) |

### Built-in vs Custom Catalogs

**Built-in CatalogSources (OpenShift):**
```bash
oc get catalogsources -n openshift-marketplace
```
```
NAME                  DISPLAY               TYPE   PUBLISHER   AGE
redhat-operators      Red Hat Operators     grpc   Red Hat     30d
certified-operators   Certified Operators   grpc   Red Hat     30d
community-operators   Community Operators   grpc   Red Hat     30d
```

**Custom CatalogSource (Lightspeed):**
```bash
oc apply -f - <<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: lightspeed-catalog-4-18
  namespace: openshift-marketplace
spec:
  sourceType: grpc
  image: quay.io/openshift-lightspeed/lightspeed-catalog:v4.18-latest
  displayName: OpenShift Lightspeed (4.18)
  publisher: Red Hat OpenShift
  updateStrategy:
    registryPoll:
      interval: 45m
EOF
```

### CatalogSource Lifecycle

```
CatalogSource Created
         ↓
Pod Created (catalog-operator)
         ↓
gRPC Server Starts
         ↓
PackageManifest Created
         ↓
Available in OperatorHub UI
```

**Checking CatalogSource Status:**

```bash
# View CatalogSource
oc get catalogsource lightspeed-catalog -n openshift-marketplace -o yaml

# Check catalog pod
oc get pods -n openshift-marketplace | grep lightspeed

# View available packages from catalog
oc get packagemanifests | grep lightspeed
```

**CatalogSource Status Conditions:**

```yaml
status:
  connectionState:
    address: lightspeed-catalog.openshift-marketplace.svc:50051
    lastObservedState: READY
  registryService:
    serviceName: lightspeed-catalog
    serviceNamespace: openshift-marketplace
    port: "50051"
    protocol: grpc
```

---

## Subscription & InstallPlan

### Subscription Anatomy

A `Subscription` declares your intent to install and keep an operator updated.

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: lightspeed-operator
  namespace: openshift-lightspeed
  labels:
    operators.coreos.com/lightspeed-operator.openshift-lightspeed: ""
spec:
  # Required: Which package to install
  name: lightspeed-operator
  
  # Required: Which channel to track
  channel: alpha
  
  # Required: Which catalog to use
  source: lightspeed-catalog
  sourceNamespace: openshift-marketplace
  
  # Installation approval
  installPlanApproval: Automatic  # or Manual
  
  # Optional: Pin to specific version
  startingCSV: lightspeed-operator.v1.0.6
  
  # Optional: Configuration for the operator
  config:
    env:
      - name: CUSTOM_ENV_VAR
        value: "custom-value"
    resources:
      requests:
        memory: "256Mi"
        cpu: "100m"
      limits:
        memory: "512Mi"
        cpu: "500m"
    
    # Node selector for operator pod
    nodeSelector:
      node-role.kubernetes.io/worker: ""
    
    # Tolerations
    tolerations:
      - key: "custom-taint"
        operator: "Exists"
        effect: "NoSchedule"

status:
  state: AtLatestKnown
  installedCSV: lightspeed-operator.v1.0.6
  currentCSV: lightspeed-operator.v1.0.6
  installPlanRef:
    name: install-abcde
    namespace: openshift-lightspeed
  conditions:
    - type: CatalogSourcesUnhealthy
      status: "False"
```

### Subscription Fields Explained

| Field | Description | When to Use |
|-------|-------------|-------------|
| `installPlanApproval` | `Automatic` or `Manual` | Manual for prod to review changes |
| `startingCSV` | Pin to specific version | When you need exact version control |
| `config.env` | Environment variables | Pass config to operator |
| `config.resources` | Resource requests/limits | Constrain operator resource usage |
| `config.nodeSelector` | Where to run operator | Dedicated operator nodes |

### InstallPlan Anatomy

An `InstallPlan` is created by OLM and defines the exact steps to install or upgrade an operator.

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: InstallPlan
metadata:
  name: install-abcde
  namespace: openshift-lightspeed
  ownerReferences:
    - apiVersion: operators.coreos.com/v1alpha1
      kind: Subscription
      name: lightspeed-operator
spec:
  approval: Automatic  # or Manual
  approved: true
  clusterServiceVersionNames:
    - lightspeed-operator.v1.0.6
  generation: 1
  
status:
  phase: Complete  # Pending, InstallReady, Installing, Complete, Failed
  catalogSources:
    - lightspeed-catalog
  conditions:
    - type: Installed
      status: "True"
      reason: InstallSucceeded
  plan:
    # Step 1: Install CRD
    - resolving: lightspeed-operator.v1.0.6
      status: Created
      resource:
        group: apiextensions.k8s.io
        version: v1
        kind: CustomResourceDefinition
        name: olsconfigs.ols.openshift.io
        manifest: |
          # Full CRD YAML
    
    # Step 2: Create ServiceAccount
    - resolving: lightspeed-operator.v1.0.6
      status: Created
      resource:
        group: ""
        version: v1
        kind: ServiceAccount
        name: lightspeed-operator-controller-manager
        manifest: |
          # Full ServiceAccount YAML
    
    # Step 3: Create ClusterRole
    - resolving: lightspeed-operator.v1.0.6
      status: Created
      resource:
        group: rbac.authorization.k8s.io
        version: v1
        kind: ClusterRole
        name: lightspeed-operator-manager-role
        manifest: |
          # Full ClusterRole YAML
    
    # ... more RBAC resources ...
    
    # Final Step: Create CSV
    - resolving: lightspeed-operator.v1.0.6
      status: Created
      resource:
        group: operators.coreos.com
        version: v1alpha1
        kind: ClusterServiceVersion
        name: lightspeed-operator.v1.0.6
        manifest: |
          # Full CSV YAML
```

### Approval Workflows

**Automatic Approval (Recommended for Dev/Test):**

```yaml
spec:
  installPlanApproval: Automatic
```

- OLM automatically approves and executes InstallPlans
- Upgrades happen without human intervention
- Good for: development, staging, CI/CD pipelines

**Manual Approval (Recommended for Production):**

```yaml
spec:
  installPlanApproval: Manual
```

- Admin must review and approve each InstallPlan
- Provides control over when upgrades occur

```bash
# List pending InstallPlans
oc get installplans -n openshift-lightspeed

# Review InstallPlan
oc get installplan install-abcde -o yaml

# Approve manually
oc patch installplan install-abcde -n openshift-lightspeed \
  --type merge \
  --patch '{"spec":{"approved":true}}'
```

---

## Operator Lifecycle States

### CSV Phase Transitions

```
┌─────────────────────────────────────────────────────────────┐
│               ClusterServiceVersion Phases                   │
└─────────────────────────────────────────────────────────────┘

     Pending
        │
        │ (InstallPlan approved)
        ▼
   InstallReady
        │
        │ (OLM creates resources)
        ▼
   Installing
        │
        │ (Deployment ready)
        ▼
   Succeeded ◄──┐
        │       │
        │       │ (Upgrade available)
        ▼       │
   Replacing ──┘
        
   Failed / Unknown
        (Error states)
```

### Phase Descriptions

| Phase | Description | Actions |
|-------|-------------|---------|
| **Pending** | CSV created, waiting for prerequisites | Check dependencies |
| **InstallReady** | Ready to install, waiting for approval | Approve InstallPlan |
| **Installing** | Resources being created | Wait for deployment |
| **Succeeded** | Operator running successfully | Normal operation |
| **Replacing** | Being replaced by newer version | Upgrade in progress |
| **Deleting** | Operator being removed | Cleanup in progress |
| **Failed** | Installation/upgrade failed | Check logs, events |

### Checking CSV Status

```bash
# View all CSVs in namespace
oc get csv -n openshift-lightspeed

# Detailed status
oc get csv lightspeed-operator.v1.0.6 -n openshift-lightspeed -o yaml

# Quick status check
oc get csv lightspeed-operator.v1.0.6 -n openshift-lightspeed \
  -o jsonpath='{.status.phase}'
```

**Example Succeeded CSV:**

```yaml
status:
  phase: Succeeded
  reason: InstallSucceeded
  message: install strategy completed with no errors
  lastUpdateTime: "2025-11-02T10:30:00Z"
  lastTransitionTime: "2025-11-02T10:28:00Z"
  requirementStatus:
    - group: apiextensions.k8s.io
      version: v1
      kind: CustomResourceDefinition
      name: olsconfigs.ols.openshift.io
      status: Present
  conditions:
    - type: Ready
      status: "True"
      lastTransitionTime: "2025-11-02T10:30:00Z"
```

---

## Upgrade Mechanisms

### How OLM Handles Upgrades

```
Catalog Operator polls CatalogSource (every 30m by default)
         ↓
New bundle version detected in channel
         ↓
Checks upgrade graph (replaces, skips, skipRange)
         ↓
Creates new InstallPlan with upgrade steps
         ↓
InstallPlan approved (automatic or manual)
         ↓
OLM Operator executes upgrade:
  1. Creates new CSV (phase: Pending)
  2. Old CSV transitions to Replacing
  3. New CSV transitions to Installing → Succeeded
  4. Old CSV deleted
         ↓
Operator pods recreated with new version
```

### Upgrade Strategies

**1. Sequential Upgrades (Default)**

Defined in CSV:

```yaml
spec:
  replaces: lightspeed-operator.v1.0.5
```

Upgrade path: `v1.0.5 → v1.0.6 → v1.0.7`

**2. Skip Ranges (Recommended)**

Defined in bundle's FBC entry:

```yaml
schema: olm.bundle
name: lightspeed-operator.v1.0.6
properties:
  - type: olm.bundle.object
    value:
      data: # CSV with skipRange
entries:
  - name: lightspeed-operator.v1.0.6
    skipRange: ">=1.0.0 <1.0.6"
```

Or in CSV:

```yaml
metadata:
  annotations:
    olm.skipRange: ">=1.0.0 <1.0.6"
```

Upgrade path: `v1.0.0-v1.0.5 → v1.0.6` (skip intermediate versions)

**3. Skips (Advanced)**

Defined in CSV:

```yaml
spec:
  skips:
    - lightspeed-operator.v1.0.5
    - lightspeed-operator.v1.0.4
```

### Upgrade Decision Matrix

| Current Version | New Version in Channel | Has skipRange | Has replaces | Result |
|----------------|------------------------|---------------|--------------|---------|
| v1.0.0 | v1.0.6 | `>=1.0.0 <1.0.6` | v1.0.5 | Direct upgrade to v1.0.6 |
| v1.0.5 | v1.0.6 | - | v1.0.5 | Sequential upgrade |
| v1.0.3 | v1.0.6 | - | v1.0.5 | Must go v1.0.3→v1.0.4→v1.0.5→v1.0.6 |
| v1.0.5 | v1.0.6 | - | - | No upgrade path (error) |

### Z-Stream Updates

For patch releases within the same minor version:

```yaml
# In catalog channel
entries:
  - name: lightspeed-operator.v1.0.6-1  # Patch 1
    replaces: lightspeed-operator.v1.0.6
  - name: lightspeed-operator.v1.0.6    # Original
    replaces: lightspeed-operator.v1.0.5
```

### Monitoring Upgrades

```bash
# Watch for new InstallPlans
oc get installplans -n openshift-lightspeed -w

# Check Subscription status for upgrade availability
oc get subscription lightspeed-operator -n openshift-lightspeed \
  -o jsonpath='{.status.currentCSV} → {.status.installedCSV}'

# View upgrade conditions
oc get subscription lightspeed-operator -n openshift-lightspeed \
  -o jsonpath='{.status.conditions[?(@.type=="ResolutionFailed")]}'
```

### Upgrade Rollback

OLM doesn't automatically rollback failed upgrades. Manual rollback:

```bash
# Delete failed CSV
oc delete csv lightspeed-operator.v1.0.7 -n openshift-lightspeed

# Pin Subscription to previous version
oc patch subscription lightspeed-operator -n openshift-lightspeed \
  --type merge \
  --patch '{"spec":{"startingCSV":"lightspeed-operator.v1.0.6"}}'

# Delete any pending InstallPlans
oc delete installplan <failed-plan> -n openshift-lightspeed
```

---

## Watch & Reconciliation

### How OLM Watches Your Operator

Once installed, your operator watches for custom resources it owns (e.g., `OLSConfig`).

```
User creates OLSConfig CR
         ↓
Kubernetes API Server
         ↓
Operator Controller (via controller-runtime)
         ↓
Reconcile() function
         ↓
Create/Update managed resources
         ↓
Update CR status
```

### OperatorGroup & Watch Scope

An `OperatorGroup` defines which namespaces an operator can watch.

**OwnNamespace (Lightspeed pattern):**

```yaml
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: openshift-lightspeed
  namespace: openshift-lightspeed
spec:
  targetNamespaces:
    - openshift-lightspeed
```

Operator can only watch resources in `openshift-lightspeed` namespace.

**AllNamespaces:**

```yaml
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: global-operators
  namespace: openshift-operators
spec: {}  # Empty spec = all namespaces
```

**MultiNamespace:**

```yaml
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: multi-namespace-group
  namespace: operator-namespace
spec:
  targetNamespaces:
    - namespace-a
    - namespace-b
    - namespace-c
```

### Install Modes (from CSV)

These must match your `OperatorGroup` configuration:

```yaml
spec:
  installModes:
    - type: OwnNamespace
      supported: true       # Lightspeed supports this
    - type: SingleNamespace
      supported: true       # Also supported
    - type: MultiNamespace
      supported: false      # Not supported
    - type: AllNamespaces
      supported: false      # Not supported
```

**Mismatch Example (causes failure):**

- CSV declares `OwnNamespace: true`, `AllNamespaces: false`
- OperatorGroup has empty `spec` (AllNamespaces mode)
- Result: CSV fails with `UnsupportedOperatorGroup` condition

---

## Dependency Resolution

### Declaring Dependencies

Dependencies are declared in the CSV using `olm.properties` in bundle metadata.

**Example: Prometheus Operator Dependency**

```yaml
# In bundle metadata or CSV annotations
dependencies:
  - type: olm.package
    value:
      packageName: prometheus-operator
      version: ">=0.47.0"
  
  - type: olm.gvk
    value:
      group: monitoring.coreos.com
      kind: ServiceMonitor
      version: v1
```

### Resolution Process

```
Subscription created
         ↓
Catalog Operator reads dependencies
         ↓
┌─────────────────────────────────────────┐
│ For each dependency:                     │
│ 1. Check if satisfied in cluster        │
│ 2. If not, find in available catalogs   │
│ 3. Add to InstallPlan                   │
└─────────────────────────────────────────┘
         ↓
All dependencies satisfied?
  ├── Yes → Create InstallPlan
  └── No  → ConstraintsNotSatisfiable condition
```

### Dependency Types

**1. Package Dependency:**

```yaml
- type: olm.package
  value:
    packageName: cert-manager
    version: ">=1.0.0 <2.0.0"
```

Requires another operator package.

**2. GVK Dependency:**

```yaml
- type: olm.gvk
  value:
    group: route.openshift.io
    kind: Route
    version: v1
```

Requires a specific API resource (checks if CRD/API exists).

**3. Label Dependency:**

```yaml
- type: olm.label
  value:
    label: "environment=production"
```

Requires cluster with specific label.

**4. Constraint Dependency (CEL):**

```yaml
- type: olm.constraint
  value:
    failureMessage: "OpenShift 4.16+ required"
    cel:
      rule: 'properties.exists(p, p.type == "olm.package" && p.value.packageName == "openshift" && semver(p.value.version) >= semver("4.16.0"))'
```

### Handling Unresolved Dependencies

```bash
# Check Subscription status
oc get subscription lightspeed-operator -n openshift-lightspeed -o yaml

# Look for ConstraintsNotSatisfiable condition
status:
  conditions:
    - type: ResolutionFailed
      status: "True"
      reason: ConstraintsNotSatisfiable
      message: "no operators found matching GVK monitoring.coreos.com/v1/ServiceMonitor"
```

**Resolution:**
1. Install missing dependency manually
2. Add required CatalogSource
3. Remove unsatisfiable dependency from CSV

---

## RBAC & Permissions

### How OLM Manages RBAC

OLM automatically creates RBAC resources defined in the CSV:

```yaml
spec:
  install:
    spec:
      clusterPermissions:
        - serviceAccountName: lightspeed-operator-controller-manager
          rules:
            - apiGroups: ["ols.openshift.io"]
              resources: ["olsconfigs"]
              verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
            - apiGroups: [""]
              resources: ["secrets", "configmaps"]
              verbs: ["get", "list", "watch", "create", "update"]
      
      permissions:
        - serviceAccountName: lightspeed-operator-controller-manager
          rules:
            - apiGroups: [""]
              resources: ["pods"]
              verbs: ["get", "list"]
```

### RBAC Resources Created by OLM

1. **ServiceAccount** (in operator namespace):
   ```yaml
   apiVersion: v1
   kind: ServiceAccount
   metadata:
     name: lightspeed-operator-controller-manager
     namespace: openshift-lightspeed
   ```

2. **ClusterRole** (for `clusterPermissions`):
   ```yaml
   apiVersion: rbac.authorization.k8s.io/v1
   kind: ClusterRole
   metadata:
     name: lightspeed-operator.v1.0.6-xxxx
   rules:
     - apiGroups: ["ols.openshift.io"]
       resources: ["olsconfigs"]
       verbs: ["*"]
   ```

3. **ClusterRoleBinding**:
   ```yaml
   apiVersion: rbac.authorization.k8s.io/v1
   kind: ClusterRoleBinding
   metadata:
     name: lightspeed-operator.v1.0.6-xxxx
   roleRef:
     apiGroup: rbac.authorization.k8s.io
     kind: ClusterRole
     name: lightspeed-operator.v1.0.6-xxxx
   subjects:
     - kind: ServiceAccount
       name: lightspeed-operator-controller-manager
       namespace: openshift-lightspeed
   ```

4. **Role** (for `permissions`, namespace-scoped):
   ```yaml
   apiVersion: rbac.authorization.k8s.io/v1
   kind: Role
   metadata:
     name: lightspeed-operator.v1.0.6-xxxx
     namespace: openshift-lightspeed
   rules:
     - apiGroups: [""]
       resources: ["pods"]
       verbs: ["get", "list"]
   ```

5. **RoleBinding**:
   ```yaml
   apiVersion: rbac.authorization.k8s.io/v1
   kind: RoleBinding
   metadata:
     name: lightspeed-operator.v1.0.6-xxxx
     namespace: openshift-lightspeed
   roleRef:
     apiGroup: rbac.authorization.k8s.io
     kind: Role
     name: lightspeed-operator.v1.0.6-xxxx
   subjects:
     - kind: ServiceAccount
       name: lightspeed-operator-controller-manager
       namespace: openshift-lightspeed
   ```

### User RBAC for Custom Resources

Users need permissions to create custom resources:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: olsconfig-creator
rules:
  - apiGroups: ["ols.openshift.io"]
    resources: ["olsconfigs"]
    verbs: ["create", "get", "list", "watch", "update", "patch", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: olsconfig-creators
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: olsconfig-creator
subjects:
  - kind: Group
    name: lightspeed-users
    apiGroup: rbac.authorization.k8s.io
```

Apply with:

```bash
oc apply -f user-rbac.yaml
```

### Verifying RBAC

```bash
# Check ServiceAccount
oc get sa -n openshift-lightspeed | grep lightspeed

# Check ClusterRoles created by operator
oc get clusterroles | grep lightspeed

# Check RoleBindings
oc get rolebindings -n openshift-lightspeed

# Test user permissions
oc auth can-i create olsconfig --as=system:serviceaccount:openshift-lightspeed:default
```

---

## Monitoring Integration

### Prometheus Operator Integration

Lightspeed operator integrates with Prometheus via ServiceMonitor:

**1. CSV Declares Monitoring Annotations:**

```yaml
metadata:
  annotations:
    operatorframework.io/cluster-monitoring: "true"
    console.openshift.io/operator-monitoring-default: "true"
```

**2. ServiceMonitor Created:**

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: lightspeed-operator-controller-manager-metrics-monitor
  namespace: openshift-lightspeed
spec:
  endpoints:
    - path: /metrics
      port: https
      scheme: https
      bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
      tlsConfig:
        insecureSkipVerify: false
        ca:
          secret:
            name: metrics-server-cert
            key: ca.crt
  selector:
    matchLabels:
      control-plane: controller-manager
```

**3. Service for Metrics:**

```yaml
apiVersion: v1
kind: Service
metadata:
  name: lightspeed-operator-controller-manager-metrics-service
  namespace: openshift-lightspeed
spec:
  ports:
    - name: https
      port: 8443
      targetPort: https
  selector:
    control-plane: controller-manager
```

### Available Metrics

Common controller-runtime metrics:

- `controller_runtime_reconcile_total` - Total reconciliations
- `controller_runtime_reconcile_errors_total` - Failed reconciliations
- `controller_runtime_reconcile_time_seconds` - Reconciliation duration
- `workqueue_depth` - Work queue depth
- `workqueue_adds_total` - Items added to queue

**Querying Metrics:**

```bash
# Port-forward to metrics service
oc port-forward -n openshift-lightspeed \
  svc/lightspeed-operator-controller-manager-metrics-service 8443:8443

# Query (in another terminal)
curl -k https://localhost:8443/metrics
```

### RBAC for Monitoring

OLM creates monitoring RBAC:

```yaml
# ClusterRole for Prometheus
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: lightspeed-operator-ols-metrics-reader
rules:
  - nonResourceURLs:
      - /metrics
    verbs:
      - get
---
# RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: lightspeed-operator-ols-metrics-reader
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: lightspeed-operator-ols-metrics-reader
subjects:
  - kind: ServiceAccount
    name: prometheus-k8s
    namespace: openshift-monitoring
```

---

## Uninstallation

### Complete Uninstallation Process

```
1. Delete Custom Resources (User Data)
         ↓
2. Delete Subscription
         ↓
3. Delete CSV
         ↓
4. Delete Operator Namespace (optional)
         ↓
5. Clean up CRDs (optional, manual)
```

### Step-by-Step Uninstallation

**1. Delete Custom Resources First (Important!)**

```bash
# List all OLSConfig instances
oc get olsconfigs --all-namespaces

# Delete them
oc delete olsconfig cluster -n openshift-lightspeed

# Wait for cleanup to complete
oc wait --for=delete olsconfig/cluster -n openshift-lightspeed --timeout=300s
```

**Why?** Operator finalizers ensure clean resource cleanup. If you delete the operator first, finalizers may prevent cleanup.

**2. Delete Subscription**

```bash
oc delete subscription lightspeed-operator -n openshift-lightspeed
```

This stops OLM from managing the operator, but doesn't remove it immediately.

**3. Delete ClusterServiceVersion**

```bash
# Find CSV
oc get csv -n openshift-lightspeed

# Delete it
oc delete csv lightspeed-operator.v1.0.6 -n openshift-lightspeed
```

OLM will:
- Delete operator Deployment
- Remove RBAC resources (ServiceAccount, Roles, Bindings)
- Clean up operator pods

**4. (Optional) Delete Operator Namespace**

```bash
oc delete namespace openshift-lightspeed
```

**5. (Optional) Delete CRDs**

⚠️ **Warning:** Deleting CRDs will delete ALL custom resources of that type cluster-wide.

```bash
# Check if any instances remain
oc get olsconfigs --all-namespaces

# If none, safe to delete CRD
oc delete crd olsconfigs.ols.openshift.io
```

**6. (Optional) Delete CatalogSource**

```bash
oc delete catalogsource lightspeed-catalog -n openshift-marketplace
```

### Cleanup Verification

```bash
# Verify operator pods gone
oc get pods -n openshift-lightspeed

# Verify CSV deleted
oc get csv -n openshift-lightspeed

# Verify Subscription deleted
oc get subscription -n openshift-lightspeed

# Verify RBAC cleaned up
oc get clusterroles | grep lightspeed
oc get clusterrolebindings | grep lightspeed
```

### Stuck Deletion / Finalizers

If resources won't delete, check for finalizers:

```bash
# Check finalizers on CSV
oc get csv lightspeed-operator.v1.0.6 -n openshift-lightspeed -o yaml | grep finalizers -A 5

# Remove finalizer (emergency only)
oc patch csv lightspeed-operator.v1.0.6 -n openshift-lightspeed \
  --type json \
  --patch='[{"op": "remove", "path": "/metadata/finalizers"}]'
```

---

## Common Patterns

### Pattern 1: Dev-to-Prod Deployment

**Development:**
- Use `installPlanApproval: Automatic`
- Track `alpha` channel
- Auto-upgrade to latest

**Staging:**
- Use `installPlanApproval: Manual`
- Track `alpha` channel
- Test before approving

**Production:**
- Use `installPlanApproval: Manual`
- Track `stable` channel (once available)
- Require change approval process

### Pattern 2: Multi-Cluster Deployment

**Hub Cluster (RHACM):**
```yaml
apiVersion: apps.open-cluster-management.io/v1
kind: Subscription
metadata:
  name: lightspeed-operator-sub
  namespace: lightspeed-ops
spec:
  channel: lightspeed-channel/alpha
  placement:
    placementRef:
      kind: PlacementRule
      name: all-openshift-clusters
```

**Spoke Clusters:**
- Operator deployed via RHACM
- Configurations managed centrally
- Policies enforce compliance

### Pattern 3: Airgapped/Disconnected Installation

**1. Mirror Catalog Image:**

```bash
# Mirror catalog
oc image mirror \
  quay.io/openshift-lightspeed/lightspeed-catalog:v4.18-latest \
  registry.internal.company.com/lightspeed/catalog:v4.18-latest

# Mirror bundle image
oc image mirror \
  quay.io/openshift-lightspeed/lightspeed-operator-bundle:v1.0.6 \
  registry.internal.company.com/lightspeed/bundle:v1.0.6
```

**2. Update CatalogSource:**

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: lightspeed-catalog-mirrored
  namespace: openshift-marketplace
spec:
  sourceType: grpc
  image: registry.internal.company.com/lightspeed/catalog:v4.18-latest
  displayName: Lightspeed (Mirrored)
```

**3. Configure ImageContentSourcePolicy:**

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: ImageContentSourcePolicy
metadata:
  name: lightspeed-mirror
spec:
  repositoryDigestMirrors:
    - mirrors:
        - registry.internal.company.com/lightspeed
      source: quay.io/openshift-lightspeed
```

### Pattern 4: Operator Configuration Override

**Via Subscription Config:**

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: lightspeed-operator
  namespace: openshift-lightspeed
spec:
  name: lightspeed-operator
  channel: alpha
  source: lightspeed-catalog
  sourceNamespace: openshift-marketplace
  config:
    env:
      - name: WATCH_NAMESPACE
        value: "custom-namespace"
      - name: LOG_LEVEL
        value: "debug"
    resources:
      limits:
        memory: "1Gi"
        cpu: "1000m"
```

### Pattern 5: Blue-Green Operator Upgrade

For critical operators, test new version in parallel:

**1. Create separate namespace for new version:**

```bash
oc create namespace openshift-lightspeed-v2
```

**2. Install new version:**

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: lightspeed-operator-v2
  namespace: openshift-lightspeed-v2
spec:
  name: lightspeed-operator
  channel: alpha
  startingCSV: lightspeed-operator.v1.1.0  # New version
  source: lightspeed-catalog
  sourceNamespace: openshift-marketplace
```

**3. Test with duplicate CR in new namespace**

**4. Switch traffic / Update references**

**5. Delete old namespace**

---

## Troubleshooting

### Common Issues & Solutions

#### Issue 1: Operator Not Appearing in OperatorHub

**Symptoms:**
- CatalogSource is ready
- But operator not visible in UI

**Diagnosis:**

```bash
# Check CatalogSource status
oc get catalogsource lightspeed-catalog -n openshift-marketplace -o yaml

# Check catalog pod logs
oc logs -n openshift-marketplace $(oc get pods -n openshift-marketplace -l olm.catalogSource=lightspeed-catalog -o name)

# Check PackageManifest
oc get packagemanifests | grep lightspeed
```

**Solutions:**
- Verify catalog image is accessible (pull secrets)
- Check catalog pod is running
- Verify FBC format is correct
- Force catalog refresh: delete catalog pod

#### Issue 2: InstallPlan Stuck in Pending

**Symptoms:**
- Subscription created
- No InstallPlan generated or InstallPlan stuck

**Diagnosis:**

```bash
# Check Subscription status
oc get subscription lightspeed-operator -n openshift-lightspeed -o yaml

# Look for conditions
oc get subscription lightspeed-operator -n openshift-lightspeed \
  -o jsonpath='{.status.conditions}'

# Check catalog-operator logs
oc logs -n openshift-operator-lifecycle-manager \
  $(oc get pods -n openshift-operator-lifecycle-manager -l app=catalog-operator -o name)
```

**Common Causes:**
- **Dependency resolution failure**: Missing required operator
  - Solution: Install dependencies or remove from CSV
- **Invalid version constraint**: No version satisfies requirements
  - Solution: Fix version ranges in dependencies
- **CatalogSource unhealthy**: Catalog not available
  - Solution: Fix CatalogSource, check image accessibility

#### Issue 3: CSV Phase Stuck in Installing

**Symptoms:**
- InstallPlan completed
- CSV created but stuck in "Installing" phase

**Diagnosis:**

```bash
# Check CSV status
oc get csv lightspeed-operator.v1.0.6 -n openshift-lightspeed -o yaml

# Check conditions
oc get csv lightspeed-operator.v1.0.6 -n openshift-lightspeed \
  -o jsonpath='{.status.conditions}'

# Check operator deployment
oc get deployment -n openshift-lightspeed

# Check operator pod status
oc get pods -n openshift-lightspeed
oc describe pod <operator-pod> -n openshift-lightspeed
```

**Common Causes:**
- **ImagePullBackOff**: Can't pull operator image
  - Solution: Check image path, add pull secrets
- **CrashLoopBackOff**: Operator pod crashing
  - Solution: Check pod logs, fix startup issues
- **Insufficient resources**: Node doesn't have capacity
  - Solution: Scale cluster or reduce resource requests
- **Invalid RBAC**: Missing permissions
  - Solution: Review CSV permissions section

#### Issue 4: Upgrade Not Happening

**Symptoms:**
- New bundle in catalog
- Subscription not upgrading

**Diagnosis:**

```bash
# Check current vs available version
oc get subscription lightspeed-operator -n openshift-lightspeed \
  -o jsonpath='Current: {.status.currentCSV}, Installed: {.status.installedCSV}'

# Check catalog for new bundles
oc get packagemanifest lightspeed-operator -o yaml

# Check for upgrade constraints
oc get subscription lightspeed-operator -n openshift-lightspeed -o yaml
```

**Common Causes:**
- **Manual approval required**: Check `installPlanApproval: Manual`
  - Solution: Approve pending InstallPlan
- **No upgrade path**: Missing `replaces` or `skipRange`
  - Solution: Fix upgrade graph in bundle
- **Subscription pinned**: `startingCSV` set to specific version
  - Solution: Remove `startingCSV` or update it
- **Catalog not refreshed**: OLM hasn't polled yet
  - Solution: Wait for polling interval or restart catalog pod

#### Issue 5: Operator Can't Create Resources

**Symptoms:**
- CSV in Succeeded phase
- Operator running
- But can't create resources when user creates CR

**Diagnosis:**

```bash
# Check operator logs
oc logs -n openshift-lightspeed deployment/lightspeed-operator-controller-manager

# Check RBAC
oc auth can-i create deployments --as=system:serviceaccount:openshift-lightspeed:lightspeed-operator-controller-manager -n openshift-lightspeed

# Check CSV permissions
oc get csv lightspeed-operator.v1.0.6 -n openshift-lightspeed \
  -o jsonpath='{.spec.install.spec.clusterPermissions}'
```

**Solutions:**
- Add missing permissions to CSV `clusterPermissions` or `permissions`
- Regenerate bundle with `make bundle`
- Upgrade operator to new CSV version

#### Issue 6: OLM Consuming Too Many Resources

**Symptoms:**
- `catalog-operator` or `olm-operator` using high CPU/memory
- Cluster performance degraded

**Diagnosis:**

```bash
# Check OLM operator resource usage
oc adm top pods -n openshift-operator-lifecycle-manager

# Check number of CatalogSources
oc get catalogsources --all-namespaces

# Check catalog operator logs for errors
oc logs -n openshift-operator-lifecycle-manager deployment/catalog-operator
```

**Solutions:**
- Reduce catalog polling frequency:
  ```yaml
  spec:
    updateStrategy:
      registryPoll:
        interval: 1h  # Increase from default 30m
  ```
- Remove unused CatalogSources
- Consolidate multiple catalogs into one
- Set resource limits on catalog pods

### Debugging Commands Reference

```bash
# OLM Operator Status
oc get pods -n openshift-operator-lifecycle-manager
oc logs -n openshift-operator-lifecycle-manager deployment/olm-operator
oc logs -n openshift-operator-lifecycle-manager deployment/catalog-operator

# Catalog Health
oc get catalogsources --all-namespaces
oc get packagemanifests

# Operator Lifecycle
oc get subscription -A
oc get installplans -A
oc get csv -A

# Operator Resources
oc get all -n openshift-lightspeed
oc get events -n openshift-lightspeed --sort-by='.lastTimestamp'

# RBAC Verification
oc auth can-i --list --as=system:serviceaccount:openshift-lightspeed:lightspeed-operator-controller-manager

# Force Refresh
oc delete pod -n openshift-marketplace -l olm.catalogSource=lightspeed-catalog
```

### Getting Help

**Check Operator Status Dashboard (OpenShift Console):**
1. Navigate to Operators → Installed Operators
2. Select your operator
3. View "Status" tab for conditions and events

**Collect Must-Gather Data:**

```bash
oc adm must-gather \
  --image=quay.io/openshift/origin-must-gather \
  --image=quay.io/operator-framework/olm:latest
```

**Relevant Logs:**
- OLM Operator: `openshift-operator-lifecycle-manager/olm-operator`
- Catalog Operator: `openshift-operator-lifecycle-manager/catalog-operator`
- Your Operator: `openshift-lightspeed/lightspeed-operator-controller-manager`
- Catalog Pod: `openshift-marketplace/lightspeed-catalog-xxxxx`

---

## Additional Resources

### Related Guides

- **[OLM Bundle Management Guide](./olm-bundle-management.md)** - Creating and packaging bundles (prerequisite)
- **[OLM Catalog Management Guide](./olm-catalog-management.md)** - Organizing bundles into catalogs (prerequisite)
- **[Contributing Guide](../CONTRIBUTING.md)** - General contribution guidelines
- **[Architecture Documentation](../ARCHITECTURE.md)** - Operator architecture overview

### External Resources

- [OLM Concepts](https://olm.operatorframework.io/docs/concepts/)
- [OLM Architecture](https://olm.operatorframework.io/docs/concepts/olm-architecture/)
- [Subscription API](https://olm.operatorframework.io/docs/concepts/crds/subscription/)
- [InstallPlan API](https://olm.operatorframework.io/docs/concepts/crds/installplan/)
- [OpenShift Operators Documentation](https://docs.openshift.com/container-platform/latest/operators/understanding/olm/olm-understanding-olm.html)

### OpenShift Console

The OpenShift web console provides visual tools for:
- **OperatorHub**: Browse and install operators
- **Installed Operators**: View operator status, create custom resources
- **Operator Details**: View CSV details, events, metrics
- **OperatorConditions**: Monitor operator health

Access: OpenShift Console → Operators section

---

**Next Steps:**
- After installing an operator, create custom resources (e.g., `OLSConfig`)
- Monitor operator metrics and logs
- Plan upgrade strategy and test in non-production first
- Review security and RBAC configurations

For questions or issues with the Lightspeed Operator specifically, see the main [README](../README.md) or [CONTRIBUTING](../CONTRIBUTING.md) guide.

