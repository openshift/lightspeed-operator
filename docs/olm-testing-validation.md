# OLM Testing & Validation Guide

> **Part of the OLM Documentation Series:**
> 1. [Bundle Management](./olm-bundle-management.md) - Creating and managing operator bundles
> 2. [Catalog Management](./olm-catalog-management.md) - Organizing bundles into catalogs
> 3. [Integration & Lifecycle](./olm-integration-lifecycle.md) - OLM integration and operator lifecycle
> 4. **Testing & Validation** ← You are here

This guide covers testing and validation strategies for OLM bundles, catalogs, and operator deployments throughout the development lifecycle.

---

## Table of Contents

- [Overview](#overview)
- [Testing Pyramid](#testing-pyramid)
- [Bundle Validation](#bundle-validation)
- [Catalog Validation](#catalog-validation)
- [Pre-Installation Testing](#pre-installation-testing)
- [Installation Testing](#installation-testing)
- [Upgrade Testing](#upgrade-testing)
- [E2E Testing](#e2e-testing)
- [Scorecard Testing](#scorecard-testing)
- [Preflight Testing](#preflight-testing)
- [CI/CD Integration](#cicd-integration)
- [Manual Testing Checklist](#manual-testing-checklist)
- [Troubleshooting Test Failures](#troubleshooting-test-failures)

---

## Overview

### Why Testing Matters for OLM

OLM operators have unique testing requirements beyond standard Kubernetes applications:

- **Bundle Correctness**: CSV must be valid and complete
- **Catalog Integrity**: Upgrade paths must be correct
- **Installation Reliability**: Operator must deploy successfully
- **Upgrade Safety**: Version transitions must work without data loss
- **RBAC Validation**: Permissions must be sufficient but minimal
- **Multi-Version Support**: Must work across OpenShift versions

### Relationship to Lightspeed E2E Tests

**This guide explains the "why" and "how" of OLM testing, while the `test/e2e/` directory contains the actual implementation.**

```
┌──────────────────────────────────────────────────────────────────┐
│                 Testing Documentation vs Implementation           │
└──────────────────────────────────────────────────────────────────┘

THIS GUIDE (docs/olm-testing-validation.md)
├── Theory: What to test and why
├── Patterns: How to structure tests
├── Tools: operator-sdk, opm, scorecard, preflight
└── Best Practices: From bundle validation to certification

                            ↕ Applied in ↕

LIGHTSPEED E2E TESTS (test/e2e/)
├── suite_test.go              ← Implements test setup patterns from this guide
├── reconciliation_test.go     ← Tests operator CR reconciliation
├── upgrade_test.go            ← Implements upgrade testing patterns
├── tls_test.go                ← Feature-specific tests
├── database_test.go           ← PostgreSQL integration tests
├── metrics_test.go            ← Monitoring validation
├── byok_test.go               ← BYOK feature tests
├── client.go                  ← Custom client with wait helpers
├── utils.go                   ← Test utilities (must-gather, etc.)
└── constants.go               ← Test configuration
```

**Key Relationships:**

| This Guide Section | Implemented In | Purpose |
|-------------------|----------------|---------|
| **Bundle Validation** | `Makefile` (`make bundle`) | Validates CSV before tests run |
| **E2E Test Patterns** | `test/e2e/suite_test.go` | BeforeSuite setup, operator readiness checks |
| **Custom Client Pattern** | `test/e2e/client.go` | `WaitForDeploymentRollout()`, condition helpers |
| **Test Environment Setup** | `test/e2e/utils.go` | `SetupOLSTestEnvironment()`, cleanup functions |
| **Upgrade Testing** | `test/e2e/upgrade_test.go` | Validates operator upgrades work |
| **Must-Gather on Failure** | `test/e2e/utils.go` | `mustGather()` collects diagnostics |
| **Configurable Timeouts** | `test/e2e/suite_test.go` | `CONDITION_TIMEOUT` environment variable |

**Example: How They Work Together**

1. **Guide Section**: "E2E Test Patterns - Pattern 2: Custom Client with Wait Functions"
   ```go
   // Guide shows the pattern
   func (c *Client) WaitForDeploymentRollout(deployment *appsv1.Deployment) error {
       // Wait for deployment ready...
   }
   ```

2. **Implementation**: `test/e2e/client.go` (lines 732+)
   ```go
   // Actual implementation in codebase
   func (c *Client) WaitForDeploymentRollout(deployment *appsv1.Deployment) error {
       return wait.PollImmediate(1*time.Second, c.conditionCheckTimeout, func() (bool, error) {
           // Real wait logic for Lightspeed deployments
       })
   }
   ```

3. **Usage**: `test/e2e/reconciliation_test.go` (line 55)
   ```go
   // Tests use the pattern
   err = client.WaitForDeploymentRollout(deployment)
   Expect(err).NotTo(HaveOccurred())
   ```

**What This Guide Provides Beyond the E2E Tests:**

- ✅ **OLM-Specific Testing**: Bundle validation, catalog validation, Scorecard, Preflight
- ✅ **Installation Testing**: How to test operator installation via OLM (Subscription, InstallPlan)
- ✅ **Upgrade Testing Theory**: Why and how to test upgrades (the E2E tests implement one approach)
- ✅ **Tool Usage**: operator-sdk, opm, preflight commands and flags
- ✅ **CI/CD Integration**: GitHub Actions, Konflux, Jenkins examples
- ✅ **Troubleshooting**: Common test failures and fixes
- ✅ **Certification**: Preparing for Red Hat certification

**What the E2E Tests Provide:**

- ✅ **Working Implementation**: Real tests that run against a deployed operator
- ✅ **Feature Coverage**: Tests for specific Lightspeed features (BYOK, TLS, database, etc.)
- ✅ **Custom Helpers**: Lightspeed-specific utilities (port forwarding, token handling, etc.)
- ✅ **Ginkgo Patterns**: BDD-style test organization with labels and ordering

**Using This Guide with E2E Tests:**

```bash
# 1. Read this guide to understand OLM testing principles
# 2. Validate bundle before running tests
make bundle BUNDLE_TAG=1.0.7
operator-sdk bundle validate ./bundle

# 3. Deploy operator (tested by suite_test.go BeforeSuite)
make deploy

# 4. Run E2E tests (implements patterns from this guide)
make e2e-test

# 5. Run specific test suites
go test ./test/e2e -v -ginkgo.focus="Reconciliation"

# 6. Use must-gather on failure (implements troubleshooting from this guide)
oc adm must-gather --dest-dir=./must-gather
```

### Testing Stages

```
Development → Bundle Validation → Catalog Validation → Installation Testing
     ↓              ↓                    ↓                      ↓
  Unit Tests   operator-sdk         opm validate         Local Cluster
                validate                                  (CRC/Kind)
                                                               ↓
                                                        Upgrade Testing
                                                               ↓
                                                        E2E Testing
                                                               ↓
                                                        Certification
                                                        (Preflight)
```

### Prerequisites

Tools needed:
```bash
# Core tools
operator-sdk    # Bundle validation
opm             # Catalog validation
kubectl/oc      # Cluster interaction

# Testing tools
ginkgo          # E2E test framework
scorecard       # Operator testing framework
preflight       # Red Hat certification

# Optional
kind            # Local Kubernetes cluster
crc             # OpenShift local
kubebuilder     # Controller testing
```

---

## Testing Pyramid

### Operator Testing Layers

```
                    ┌─────────────────┐
                    │  Certification  │  Preflight, Scorecard
                    │    Testing      │  (Manual/CI)
                    └─────────────────┘
                          /\
                         /  \
                ┌──────────────────┐
                │   E2E Testing    │  Full cluster tests
                │  (Reconciliation)│  CR lifecycle
                └──────────────────┘
                        /\
                       /  \
              ┌────────────────────┐
              │  Upgrade Testing   │  Version transitions
              │  OLM Integration   │  Subscription, InstallPlan
              └────────────────────┘
                      /\
                     /  \
            ┌──────────────────────┐
            │ Installation Testing │  Bundle → Running operator
            │   Catalog Testing    │  Channel validation
            └──────────────────────┘
                    /\
                   /  \
          ┌────────────────────────┐
          │  Bundle Validation     │  CSV structure
          │  Catalog Validation    │  FBC format
          └────────────────────────┘
                  /\
                 /  \
        ┌──────────────────────────┐
        │    Unit Testing          │  Controller logic
        │    Manifest Generation   │  RBAC, CRDs
        └──────────────────────────┘
```

### Testing Focus by Stage

| Stage | What to Test | Tools | Frequency |
|-------|--------------|-------|-----------|
| **Unit** | Controller logic, utilities | Go test, Ginkgo | Every commit |
| **Manifest** | Generated RBAC, CRDs correct | make manifests | Every commit |
| **Bundle Validation** | CSV valid, annotations correct | operator-sdk validate | Every bundle change |
| **Catalog Validation** | FBC format, upgrade paths | opm validate | Every catalog update |
| **Installation** | Operator deploys successfully | Manual/CI cluster | Every release |
| **Upgrade** | Version transitions work | Manual/CI cluster | Every release |
| **E2E** | Custom resources reconcile | Ginkgo tests | Every release |
| **Certification** | Red Hat requirements met | Preflight, Scorecard | Before release |

---

## Bundle Validation

### Automatic Bundle Validation

Bundle validation runs automatically during `make bundle`:

```bash
make bundle BUNDLE_TAG=1.0.7
# Includes: operator-sdk bundle validate ./bundle
```

**What it checks:**
- CSV structure and required fields
- CRD references
- RBAC permissions format
- Annotation syntax
- Image reference validity
- Bundle file structure

### Manual Bundle Validation

```bash
# Basic validation
operator-sdk bundle validate ./bundle

# Verbose output
operator-sdk bundle validate ./bundle -o text

# Validation for specific suites
operator-sdk bundle validate ./bundle \
  --select-optional suite=operatorframework

# OpenShift-specific validation
operator-sdk bundle validate ./bundle \
  --select-optional name=operatorhub \
  --optional-values=k8s-version=1.28
```

### Validation Suites

| Suite | Focus | When to Use |
|-------|-------|-------------|
| `operatorframework` | Core OLM requirements | Always |
| `operatorhub` | OperatorHub UI requirements | Publishing to OperatorHub |
| `community` | Community operator standards | Community catalog |

### Common Validation Errors

#### Error 1: Missing Required Fields

```
Error: Value : (lightspeed-operator.v1.0.7) csv.Spec.minKubeVersion not specified
```

**Fix:**
```yaml
spec:
  minKubeVersion: 1.28.0
```

#### Error 2: Invalid InstallMode

```
Error: csv.Spec.installModes at least one InstallMode must be supported
```

**Fix:**
```yaml
spec:
  installModes:
    - type: OwnNamespace
      supported: true
    - type: SingleNamespace
      supported: true
    - type: MultiNamespace
      supported: false
    - type: AllNamespaces
      supported: false
```

#### Error 3: Missing Icon

```
Warning: csv.Spec.icon not specified
```

**Fix:**
```yaml
spec:
  icon:
    - base64data: PHN2ZyB4bWxucz0i...  # Base64 encoded SVG
      mediatype: image/svg+xml
```

#### Error 4: Invalid Related Images

```
Error: csv.Spec.relatedImages[0] image must be a valid container image reference
```

**Fix:**
```yaml
spec:
  relatedImages:
    - name: lightspeed-service
      image: quay.io/openshift-lightspeed/lightspeed-service-api:v1.0.0  # Must be valid
```

### Validation Best Practices

**1. Validate Early and Often**

```bash
# Add to pre-commit hook
#!/bin/bash
if [ -d "./bundle" ]; then
  operator-sdk bundle validate ./bundle || exit 1
fi
```

**2. Use Validation in CI**

```yaml
# GitHub Actions example
- name: Validate Bundle
  run: |
    make operator-sdk
    make bundle BUNDLE_TAG=${{ github.ref_name }}
    operator-sdk bundle validate ./bundle
```

**3. Check Multiple Suites**

```bash
# Comprehensive validation
operator-sdk bundle validate ./bundle \
  --select-optional suite=operatorframework \
  --select-optional name=operatorhub \
  --select-optional name=good-practices
```

**4. Validate with Different OpenShift Versions**

```bash
# For 4.16
operator-sdk bundle validate ./bundle \
  --optional-values=k8s-version=1.29

# For 4.18
operator-sdk bundle validate ./bundle \
  --optional-values=k8s-version=1.31
```

---

## Catalog Validation

### OPM Catalog Validation

Validate File-Based Catalogs (FBC):

```bash
# Validate catalog directory
opm validate ./lightspeed-catalog

# Validate specific catalog
opm validate ./lightspeed-catalog-4.18

# Render and validate
opm render quay.io/openshift-lightspeed/lightspeed-operator-bundle:v1.0.6 \
  | opm validate -
```

**What it checks:**
- FBC YAML syntax
- Schema compliance (olm.package, olm.channel, olm.bundle)
- Channel consistency
- Bundle references valid
- Upgrade graph correctness

### Channel Validation

Validate upgrade paths:

```bash
# Check channel for upgrade issues
opm alpha list channels ./lightspeed-catalog-4.18

# Validate specific channel
opm alpha channel validate alpha \
  --package lightspeed-operator \
  --catalog ./lightspeed-catalog-4.18/index.yaml
```

### Upgrade Graph Validation

```bash
# Visualize upgrade graph (if graphviz installed)
opm alpha render-graph alpha \
  --package lightspeed-operator \
  --catalog ./lightspeed-catalog-4.18/index.yaml \
  --output graph.dot

dot -Tpng graph.dot -o upgrade-graph.png
```

**Example graph validation checks:**
- No orphaned bundles (bundles with no path from previous version)
- No cycles in upgrade paths
- skipRange covers appropriate versions
- replaces references valid bundle versions

### Common Catalog Errors

#### Error 1: Invalid Schema

```
Error: invalid schema: unexpected field 'schemaa'
```

**Fix:** Use correct schema types:
- `olm.package`
- `olm.channel`
- `olm.bundle`

#### Error 2: Missing Package Definition

```
Error: no package definition found
```

**Fix:** Add package definition:
```yaml
---
schema: olm.package
name: lightspeed-operator
defaultChannel: alpha
```

#### Error 3: Channel References Non-Existent Bundle

```
Error: channel 'alpha' references bundle 'lightspeed-operator.v1.0.9' which does not exist
```

**Fix:** Ensure bundle is defined before channel references it.

#### Error 4: Duplicate Bundle

```
Error: duplicate bundle name 'lightspeed-operator.v1.0.6'
```

**Fix:** Each bundle name must appear only once in the catalog.

### Catalog Validation Best Practices

**1. Validate After Every Addition**

```bash
# After adding bundle to catalog
./hack/bundle_to_catalog.sh \
  -b quay.io/openshift-lightspeed/bundle:v1.0.7 \
  -c ./lightspeed-catalog-4.18/index.yaml

opm validate ./lightspeed-catalog-4.18
```

**2. Test Catalog Serving Locally**

```bash
# Serve catalog locally
opm serve ./lightspeed-catalog-4.18 --port 50051

# In another terminal, test connection
grpcurl -plaintext localhost:50051 api.Registry/ListPackages
```

**3. Validate Catalog Image**

```bash
# Build catalog image
docker build -f lightspeed-catalog-4.18.Dockerfile \
  -t localhost:5000/lightspeed-catalog:test .

# Push to local registry
docker push localhost:5000/lightspeed-catalog:test

# Validate by creating CatalogSource
oc apply -f - <<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: test-catalog
  namespace: openshift-marketplace
spec:
  sourceType: grpc
  image: localhost:5000/lightspeed-catalog:test
EOF

# Check catalog pod
oc get pods -n openshift-marketplace | grep test-catalog
```

---

## Pre-Installation Testing

### Bundle Image Inspection

Before deploying, inspect bundle images:

```bash
# Pull and inspect bundle
podman pull quay.io/openshift-lightspeed/lightspeed-operator-bundle:v1.0.6
podman run --rm -it quay.io/openshift-lightspeed/lightspeed-operator-bundle:v1.0.6 ls -la /manifests

# Extract CSV
podman run --rm quay.io/openshift-lightspeed/lightspeed-operator-bundle:v1.0.6 \
  cat /manifests/lightspeed-operator.clusterserviceversion.yaml

# Check annotations
podman run --rm quay.io/openshift-lightspeed/lightspeed-operator-bundle:v1.0.6 \
  cat /metadata/annotations.yaml
```

### Dry-Run Installation

Test InstallPlan generation without installing:

```bash
# Create Subscription without auto-approval
oc apply -f - <<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: lightspeed-operator-test
  namespace: openshift-lightspeed
spec:
  channel: alpha
  name: lightspeed-operator
  source: lightspeed-catalog
  sourceNamespace: openshift-marketplace
  installPlanApproval: Manual  # Won't install automatically
EOF

# Wait for InstallPlan to be created
oc get installplan -n openshift-lightspeed

# Inspect InstallPlan (without approving)
oc get installplan <install-plan-name> -n openshift-lightspeed -o yaml

# Check what resources would be created
oc get installplan <install-plan-name> -n openshift-lightspeed \
  -o jsonpath='{.status.plan[*].resource.kind}'

# Clean up without installing
oc delete subscription lightspeed-operator-test -n openshift-lightspeed
oc delete installplan <install-plan-name> -n openshift-lightspeed
```

### RBAC Pre-Check

Validate RBAC permissions before installation:

```bash
# Extract ClusterRole from CSV
oc get csv lightspeed-operator.v1.0.6 -n openshift-lightspeed \
  -o jsonpath='{.spec.install.spec.clusterPermissions[0].rules}' | jq .

# Test if current user can grant those permissions
oc auth can-i create clusterroles
oc auth can-i create clusterrolebindings

# Check if specific permissions are allowed
oc auth can-i create olsconfigs.ols.openshift.io
```

---

## Installation Testing

### Test Installation Flow

**Step 1: Prepare Test Namespace**

```bash
# Create namespace
oc create namespace openshift-lightspeed-test

# Create OperatorGroup
oc apply -f - <<EOF
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: lightspeed-test-og
  namespace: openshift-lightspeed-test
spec:
  targetNamespaces:
    - openshift-lightspeed-test
EOF
```

**Step 2: Create Test Subscription**

```bash
oc apply -f - <<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: lightspeed-operator
  namespace: openshift-lightspeed-test
spec:
  channel: alpha
  name: lightspeed-operator
  source: lightspeed-catalog
  sourceNamespace: openshift-marketplace
  installPlanApproval: Automatic
  startingCSV: lightspeed-operator.v1.0.6
EOF
```

**Step 3: Monitor Installation**

```bash
# Watch Subscription status
oc get subscription lightspeed-operator -n openshift-lightspeed-test -w

# Watch InstallPlan
oc get installplan -n openshift-lightspeed-test -w

# Watch CSV
oc get csv -n openshift-lightspeed-test -w

# Check events
oc get events -n openshift-lightspeed-test --sort-by='.lastTimestamp'
```

**Step 4: Verify Installation**

```bash
# CSV should be in Succeeded phase
oc get csv -n openshift-lightspeed-test

# Operator pod running
oc get pods -n openshift-lightspeed-test

# CRD installed
oc get crd olsconfigs.ols.openshift.io

# RBAC created
oc get clusterroles | grep lightspeed
oc get clusterrolebindings | grep lightspeed
```

### Installation Test Matrix

Test across different configurations:

| Scenario | Namespace | OperatorGroup | Install Mode | Expected Result |
|----------|-----------|---------------|--------------|-----------------|
| Standard | openshift-lightspeed | OwnNamespace | OwnNamespace | Success |
| Custom NS | custom-ols | OwnNamespace | OwnNamespace | Success |
| Wrong OG | test-ns | AllNamespaces | OwnNamespace | Fail (UnsupportedOperatorGroup) |
| No OG | test-ns | None | Any | Fail (no OperatorGroup) |
| Multi OG | test-ns | Multiple | Any | Fail (conflicting OperatorGroups) |

### Automated Installation Tests

**Using Bash Script:**

```bash
#!/bin/bash
# test-installation.sh

set -e

NAMESPACE=${1:-openshift-lightspeed-test}
TIMEOUT=300

echo "Installing operator in namespace ${NAMESPACE}"

# Create namespace
oc create namespace ${NAMESPACE} || true

# Create OperatorGroup
oc apply -f - <<EOF
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: test-og
  namespace: ${NAMESPACE}
spec:
  targetNamespaces:
    - ${NAMESPACE}
EOF

# Create Subscription
oc apply -f - <<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: lightspeed-operator
  namespace: ${NAMESPACE}
spec:
  channel: alpha
  name: lightspeed-operator
  source: lightspeed-catalog
  sourceNamespace: openshift-marketplace
  installPlanApproval: Automatic
EOF

# Wait for CSV to reach Succeeded
echo "Waiting for CSV to be Succeeded (timeout: ${TIMEOUT}s)"
SECONDS=0
while [ $SECONDS -lt $TIMEOUT ]; do
  PHASE=$(oc get csv -n ${NAMESPACE} -o jsonpath='{.items[0].status.phase}' 2>/dev/null || echo "NotFound")
  echo "CSV phase: ${PHASE}"
  
  if [ "${PHASE}" == "Succeeded" ]; then
    echo "✅ Operator installed successfully"
    exit 0
  elif [ "${PHASE}" == "Failed" ]; then
    echo "❌ Operator installation failed"
    oc get csv -n ${NAMESPACE} -o yaml
    exit 1
  fi
  
  sleep 5
done

echo "❌ Timeout waiting for operator installation"
exit 1
```

**Using Go/Ginkgo (similar to Lightspeed E2E):**

```go
var _ = Describe("Installation", func() {
    It("should install operator successfully", func() {
        By("Creating Subscription")
        subscription := &v1alpha1.Subscription{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "lightspeed-operator",
                Namespace: testNamespace,
            },
            Spec: &v1alpha1.SubscriptionSpec{
                Channel:             "alpha",
                Package:             "lightspeed-operator",
                CatalogSource:       "lightspeed-catalog",
                CatalogSourceNS:     "openshift-marketplace",
                InstallPlanApproval: v1alpha1.ApprovalAutomatic,
            },
        }
        Expect(k8sClient.Create(ctx, subscription)).To(Succeed())

        By("Waiting for CSV to be Succeeded")
        Eventually(func() string {
            csv := &v1alpha1.ClusterServiceVersion{}
            err := k8sClient.Get(ctx, types.NamespacedName{
                Name:      "lightspeed-operator.v1.0.6",
                Namespace: testNamespace,
            }, csv)
            if err != nil {
                return ""
            }
            return string(csv.Status.Phase)
        }, timeout, interval).Should(Equal("Succeeded"))

        By("Verifying operator pod is running")
        deployment := &appsv1.Deployment{}
        Expect(k8sClient.Get(ctx, types.NamespacedName{
            Name:      "lightspeed-operator-controller-manager",
            Namespace: testNamespace,
        }, deployment)).To(Succeed())
        
        Expect(deployment.Status.ReadyReplicas).To(Equal(int32(1)))
    })
})
```

---

## Upgrade Testing

### Upgrade Test Scenarios

| From Version | To Version | Method | Test Focus |
|--------------|------------|--------|------------|
| v1.0.5 | v1.0.6 | Sequential | Normal upgrade path |
| v1.0.0 | v1.0.6 | skipRange | Skip intermediate versions |
| v1.0.6 | v1.0.6-1 | Z-stream | Patch release |
| v1.0.6 | v2.0.0 | Major | Breaking changes, migrations |

### Manual Upgrade Test

**Step 1: Install Base Version**

```bash
# Install v1.0.5
oc apply -f - <<EOF
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
  installPlanApproval: Manual
  startingCSV: lightspeed-operator.v1.0.5
EOF

# Wait for InstallPlan
IP_NAME=$(oc get installplan -n openshift-lightspeed -o jsonpath='{.items[0].metadata.name}')

# Approve
oc patch installplan ${IP_NAME} -n openshift-lightspeed \
  --type merge \
  --patch '{"spec":{"approved":true}}'

# Wait for v1.0.5 to be running
oc wait --for=condition=Ready csv/lightspeed-operator.v1.0.5 \
  -n openshift-lightspeed \
  --timeout=300s
```

**Step 2: Create Custom Resource**

```bash
# Create OLSConfig with v1.0.5
oc apply -f - <<EOF
apiVersion: ols.openshift.io/v1alpha1
kind: OLSConfig
metadata:
  name: cluster
spec:
  llm:
    providers:
      - name: openai
        url: https://api.openai.com/v1
        credentialsSecretRef:
          name: openai-secret
        models:
          - name: gpt-3.5-turbo
  ols:
    deployment:
      replicas: 1
EOF

# Wait for resources to be created
sleep 10

# Verify app server running
oc get deployment -n openshift-lightspeed
```

**Step 3: Trigger Upgrade**

```bash
# Remove startingCSV to allow upgrade
oc patch subscription lightspeed-operator -n openshift-lightspeed \
  --type json \
  --patch '[{"op": "remove", "path": "/spec/startingCSV"}]'

# Or explicitly set to newer version
oc patch subscription lightspeed-operator -n openshift-lightspeed \
  --type merge \
  --patch '{"spec":{"startingCSV":"lightspeed-operator.v1.0.6"}}'

# Wait for new InstallPlan
sleep 10
IP_NAME=$(oc get installplan -n openshift-lightspeed --sort-by='.metadata.creationTimestamp' -o jsonpath='{.items[-1].metadata.name}')

# Approve upgrade
oc patch installplan ${IP_NAME} -n openshift-lightspeed \
  --type merge \
  --patch '{"spec":{"approved":true}}'
```

**Step 4: Monitor Upgrade**

```bash
# Watch CSV transition
oc get csv -n openshift-lightspeed -w

# v1.0.5 should go to "Replacing"
# v1.0.6 should go: Pending → InstallReady → Installing → Succeeded

# Check operator pod recreated
oc get pods -n openshift-lightspeed -w
```

**Step 5: Verify After Upgrade**

```bash
# CSV v1.0.6 is Succeeded
oc get csv lightspeed-operator.v1.0.6 -n openshift-lightspeed

# Old CSV v1.0.5 deleted
oc get csv lightspeed-operator.v1.0.5 -n openshift-lightspeed
# Should return NotFound

# Custom resource still exists
oc get olsconfig cluster -n openshift-lightspeed

# App server still running
oc get deployment lightspeed-app-server -n openshift-lightspeed

# Test functionality (create/update CR)
oc patch olsconfig cluster -n openshift-lightspeed \
  --type merge \
  --patch '{"spec":{"ols":{"deployment":{"replicas":2}}}}'

# Verify change applied
oc get deployment lightspeed-app-server -n openshift-lightspeed \
  -o jsonpath='{.spec.replicas}'
```

### Automated Upgrade Tests

**Lightspeed E2E Upgrade Test Pattern:**

```go
// test/e2e/upgrade_test.go
var _ = Describe("Upgrade operator tests", Ordered, Label("Upgrade"), func() {
    var cr *olsv1alpha1.OLSConfig
    var client *Client

    BeforeAll(func() {
        By("Installing base version operator")
        // Operator assumed to be already installed via Subscription
        
        By("Creating OLSConfig CR")
        cr, err = generateOLSConfig()
        Expect(err).NotTo(HaveOccurred())
        err = client.Create(cr)
        Expect(err).NotTo(HaveOccurred())

        By("Waiting for application server deployment")
        deployment := &appsv1.Deployment{
            ObjectMeta: metav1.ObjectMeta{
                Name:      AppServerDeploymentName,
                Namespace: OLSNameSpace,
            },
        }
        err = client.WaitForDeploymentRollout(deployment)
        Expect(err).NotTo(HaveOccurred())
    })

    It("should upgrade operator without disruption", func() {
        By("Recording current deployment generation")
        deployment := &appsv1.Deployment{}
        err := client.Get(ctx, types.NamespacedName{
            Name:      AppServerDeploymentName,
            Namespace: OLSNameSpace,
        }, deployment)
        Expect(err).NotTo(HaveOccurred())
        oldGeneration := deployment.Generation

        By("Triggering operator upgrade")
        // Upgrade mechanism (update Subscription, approve InstallPlan, etc.)
        
        By("Waiting for new operator CSV")
        Eventually(func() bool {
            csv := &v1alpha1.ClusterServiceVersion{}
            err := client.Get(ctx, types.NamespacedName{
                Name:      "lightspeed-operator.v1.0.6",
                Namespace: OLSNameSpace,
            }, csv)
            return err == nil && csv.Status.Phase == "Succeeded"
        }, timeout, interval).Should(BeTrue())

        By("Verifying OLSConfig CR still exists")
        err = client.Get(ctx, types.NamespacedName{
            Name: "cluster",
        }, cr)
        Expect(err).NotTo(HaveOccurred())

        By("Verifying deployment still running")
        err = client.WaitForDeploymentRollout(deployment)
        Expect(err).NotTo(HaveOccurred())

        By("Testing CR update after upgrade")
        cr.Spec.OLS.Deployment.Replicas = 2
        err = client.Update(cr)
        Expect(err).NotTo(HaveOccurred())

        Eventually(func() int32 {
            err := client.Get(ctx, types.NamespacedName{
                Name:      AppServerDeploymentName,
                Namespace: OLSNameSpace,
            }, deployment)
            if err != nil {
                return 0
            }
            return deployment.Spec.Replicas
        }, timeout, interval).Should(Equal(int32(2)))
    })

    AfterAll(func() {
        By("Collecting must-gather for upgrade test")
        err := mustGather("upgrade_test")
        Expect(err).NotTo(HaveOccurred())
    })
})
```

### Upgrade Checklist

Test these aspects during upgrades:

- [ ] **Subscription** detects new version
- [ ] **InstallPlan** created with correct resources
- [ ] **Old CSV** transitions to "Replacing" phase
- [ ] **New CSV** reaches "Succeeded" phase
- [ ] **Operator pod** recreated with new version
- [ ] **Custom resources** preserved (no data loss)
- [ ] **Managed resources** not disrupted
- [ ] **New features** work correctly
- [ ] **Backward compatibility** maintained
- [ ] **Status conditions** updated correctly
- [ ] **Metrics** still exposed
- [ ] **Webhooks** still functional (if any)

---

## E2E Testing

### Lightspeed E2E Test Structure

**Implementation:** [`test/e2e/`](../test/e2e/) directory

The Lightspeed operator uses Ginkgo v2 for E2E tests:

```
test/e2e/
├── suite_test.go           # Test suite setup
├── client.go               # Kubernetes client wrapper
├── utils.go                # Test utilities
├── reconciliation_test.go  # CR reconciliation tests
├── upgrade_test.go         # Upgrade tests
├── tls_test.go             # TLS configuration tests
├── database_test.go        # PostgreSQL tests
├── metrics_test.go         # Metrics/monitoring tests
├── byok_test.go            # BYOK (Bring Your Own Knowledge) tests
└── ...                     # Other feature tests
```

### Running E2E Tests

**Prerequisites:**

```bash
# Operator must be deployed
oc get csv -n openshift-lightspeed

# Secrets must exist
oc get secret -n openshift-lightspeed | grep llm-token
```

**Run All E2E Tests:**

```bash
make e2e-test
```

**Run Specific Test Suites:**

```bash
# Run only reconciliation tests
go test ./test/e2e -v -ginkgo.focus="Reconciliation"

# Run upgrade tests
go test ./test/e2e -v -ginkgo.label-filter="Upgrade"

# Run with custom timeout
CONDITION_TIMEOUT_SECONDS=600 go test ./test/e2e -v

# Skip certain tests
go test ./test/e2e -v -ginkgo.skip="BYOK"
```

### E2E Test Patterns from Lightspeed

**Pattern 1: Test Environment Setup**

**Implementation:** [`test/e2e/utils.go`](../test/e2e/utils.go) (lines 19-88)

```go
// test/e2e/utils.go
type OLSTestEnvironment struct {
    Client       *Client
    CR           *olsv1alpha1.OLSConfig
    SAToken      string
    ForwardHost  string
    CleanUpFuncs []func()
}

func SetupOLSTestEnvironment(crModifier func(*olsv1alpha1.OLSConfig)) (*OLSTestEnvironment, error) {
    env := &OLSTestEnvironment{
        CleanUpFuncs: make([]func(), 0),
    }

    // Create client
    env.Client, err = GetClient(nil)
    if err != nil {
        return nil, err
    }

    // Create OLSConfig CR
    env.CR, err = generateOLSConfig()
    if err != nil {
        return nil, err
    }

    // Apply modifications
    if crModifier != nil {
        crModifier(env.CR)
    }

    // Create CR
    err = env.Client.Create(env.CR)
    if err != nil {
        return nil, err
    }

    // Wait for deployment
    deployment := &appsv1.Deployment{
        ObjectMeta: metav1.ObjectMeta{
            Name:      AppServerDeploymentName,
            Namespace: OLSNameSpace,
        },
    }
    err = env.Client.WaitForDeploymentRollout(deployment)
    if err != nil {
        return nil, err
    }

    return env, nil
}
```

**Pattern 2: Custom Client with Wait Functions**

**Implementation:** [`test/e2e/client.go`](../test/e2e/client.go) (lines 732+)

```go
// test/e2e/client.go
func (c *Client) WaitForDeploymentRollout(deployment *appsv1.Deployment) error {
    return wait.PollImmediate(1*time.Second, c.conditionCheckTimeout, func() (bool, error) {
        err := c.Get(context.TODO(), types.NamespacedName{
            Name:      deployment.Name,
            Namespace: deployment.Namespace,
        }, deployment)
        if err != nil {
            return false, err
        }

        if deployment.Status.UpdatedReplicas == *deployment.Spec.Replicas &&
            deployment.Status.Replicas == *deployment.Spec.Replicas &&
            deployment.Status.AvailableReplicas == *deployment.Spec.Replicas &&
            deployment.Status.ObservedGeneration >= deployment.Generation {
            return true, nil
        }

        return false, nil
    })
}
```

**Pattern 3: Test with Setup and Cleanup**

**Implementation:** [`test/e2e/byok_test.go`](../test/e2e/byok_test.go)

```go
// test/e2e/byok_test.go
var _ = Describe("BYOK", Ordered, Label("BYOK"), func() {
    var env *OLSTestEnvironment
    var err error

    BeforeAll(func() {
        By("Setting up OLS test environment with RAG configuration")
        env, err = SetupOLSTestEnvironment(func(cr *olsv1alpha1.OLSConfig) {
            cr.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{
                {
                    Image: "quay.io/openshift-lightspeed-test/assisted-installer-guide:2025-1",
                },
            }
            cr.Spec.OLSConfig.ByokRAGOnly = true
        })
        Expect(err).NotTo(HaveOccurred())
    })

    AfterAll(func() {
        By("Cleaning up OLS test environment with CR deletion")
        err = CleanupOLSTestEnvironmentWithCRDeletion(env, "byok_test")
        Expect(err).NotTo(HaveOccurred())
    })

    It("should query the BYOK database", FlakeAttempts(5), func() {
        By("Testing HTTPS POST on /v1/query endpoint")
        reqBody := []byte(`{"query": "what CPU architectures does the assisted installer support?"}`)
        resp, body, err := TestHTTPSQueryEndpoint(env, secret, reqBody)
        Expect(err).NotTo(HaveOccurred())
        defer resp.Body.Close()
        
        Expect(resp.StatusCode).To(Equal(http.StatusOK))
        Expect(string(body)).To(ContainSubstring("x86_64"))
    })
})
```

**Pattern 4: Must-Gather on Failure**

**Implementation:** [`test/e2e/utils.go`](../test/e2e/utils.go) (mustGather function)

```go
// test/e2e/utils.go
func mustGather(testName string) error {
    outputDir := fmt.Sprintf("./must-gather-%s", testName)
    cmd := exec.Command("oc", "adm", "must-gather",
        "--dest-dir", outputDir,
        "--", "/usr/bin/gather")
    
    output, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("must-gather failed: %v, output: %s", err, string(output))
    }
    
    fmt.Printf("Must-gather data saved to %s\n", outputDir)
    return nil
}
```

### E2E Test Best Practices

**1. Use Ordered and Labels**

```go
var _ = Describe("Feature X", Ordered, Label("FeatureX", "Slow"), func() {
    // Ordered: BeforeAll runs once, tests run in order
    // Labels: Filter tests with -ginkgo.label-filter="FeatureX"
})
```

**2. Implement Proper Cleanup**

```go
AfterAll(func() {
    // Always collect diagnostics
    err := mustGather("feature_x_test")
    Expect(err).NotTo(HaveOccurred())
    
    // Delete custom resources
    if cr != nil {
        client.Delete(cr)
    }
    
    // Call cleanup functions
    for _, cleanup := range cleanupFuncs {
        cleanup()
    }
})
```

**3. Use FlakeAttempts for Flaky Tests**

```go
It("should handle network issues", FlakeAttempts(3), func() {
    // Test will retry up to 3 times if it fails
})
```

**4. Configurable Timeouts**

```go
// Allow timeout override via environment variable
conditionTimeout := DefaultPollTimeout
if timeoutStr := os.Getenv("CONDITION_TIMEOUT_SECONDS"); timeoutStr != "" {
    seconds, err := strconv.Atoi(timeoutStr)
    Expect(err).NotTo(HaveOccurred())
    conditionTimeout = time.Duration(seconds) * time.Second
}
```

---

## Scorecard Testing

### What is Scorecard?

Operator SDK Scorecard is a testing framework that validates operator best practices.

**Install:**

```bash
# Included with operator-sdk
operator-sdk version
```

### Running Scorecard Tests

**Basic Scorecard:**

```bash
# Run scorecard tests
operator-sdk scorecard ./bundle \
  --namespace openshift-lightspeed \
  --wait-time 300s

# Run specific test
operator-sdk scorecard ./bundle \
  --selector=test=basic-check-spec \
  --namespace openshift-lightspeed

# Output as JSON
operator-sdk scorecard ./bundle \
  --output json \
  --namespace openshift-lightspeed
```

### Scorecard Test Suites

**1. Basic Tests:**
- `basic-check-spec`: Validates CRs can be created
- `olm-bundle-validation`: Checks bundle structure
- `olm-crds-have-validation`: Validates CRD schemas
- `olm-crds-have-resources`: Checks CRD resource info
- `olm-spec-descriptors`: Validates spec descriptors
- `olm-status-descriptors`: Validates status descriptors

**2. OLM Tests:**
- Bundle annotations correct
- CSV valid
- CRDs properly defined

**3. Custom Tests:**

You can define custom scorecard tests in `bundle/tests/scorecard/config.yaml`:

```yaml
apiVersion: scorecard.operatorframework.io/v1alpha3
kind: Configuration
metadata:
  name: config
stages:
  - parallel: true
    tests:
      - entrypoint:
          - custom-scorecard-tests
        image: quay.io/operator-framework/scorecard-test:latest
        labels:
          suite: custom
          test: custom-test-1
```

### Scorecard in CI

```yaml
# GitHub Actions
- name: Run Scorecard Tests
  run: |
    operator-sdk scorecard ./bundle \
      --namespace ${{ env.TEST_NAMESPACE }} \
      --output json > scorecard-results.json
    
    # Fail if any test failed
    jq -e '.items[] | select(.status.results[].state == "fail") | .status.results[]' scorecard-results.json && exit 1 || exit 0

- name: Upload Scorecard Results
  uses: actions/upload-artifact@v3
  with:
    name: scorecard-results
    path: scorecard-results.json
```

---

## Preflight Testing

### What is Preflight?

Preflight is Red Hat's certification testing tool for operators and container images.

**Install:**

```bash
# Download preflight
wget https://github.com/redhat-openshift-ecosystem/openshift-preflight/releases/download/v1.8.0/preflight-linux-amd64
chmod +x preflight-linux-amd64
sudo mv preflight-linux-amd64 /usr/local/bin/preflight
```

### Running Preflight Tests

**Test Operator Bundle:**

```bash
# Basic check
preflight check operator \
  quay.io/openshift-lightspeed/lightspeed-operator-bundle:v1.0.6

# With certification project ID (for Red Hat partners)
preflight check operator \
  quay.io/openshift-lightspeed/lightspeed-operator-bundle:v1.0.6 \
  --certification-project-id=<project-id> \
  --pyxis-api-token=<token>

# Output as JSON
preflight check operator \
  quay.io/openshift-lightspeed/lightspeed-operator-bundle:v1.0.6 \
  --artifacts ./preflight-results \
  --output json
```

**Test Container Image:**

```bash
# Check operator image
preflight check container \
  quay.io/openshift-lightspeed/lightspeed-operator:v1.0.6

# Check operand images
preflight check container \
  quay.io/openshift-lightspeed/lightspeed-service-api:v1.0.0
```

### Preflight Checks

**Operator Bundle Checks:**
- ✅ Bundle format valid
- ✅ CSV annotations present
- ✅ Images use SHA256 digests (relatedImages)
- ✅ Operator images in container catalog
- ✅ Security best practices
- ✅ Compatible with OpenShift versions

**Container Image Checks:**
- ✅ Base image is Red Hat UBI (Universal Base Image)
- ✅ No root user
- ✅ Proper labels
- ✅ License scannable
- ✅ No vulnerabilities (critical/high)

### Fixing Common Preflight Issues

**Issue: Images don't use digests**

```yaml
# Before (tag-based)
image: quay.io/openshift-lightspeed/lightspeed-operator:v1.0.6

# After (digest-based)
image: quay.io/openshift-lightspeed/lightspeed-operator@sha256:abcdef123456...
```

**Generate digests:**

```bash
# Get image digest
podman inspect quay.io/openshift-lightspeed/lightspeed-operator:v1.0.6 \
  --format '{{.Digest}}'

# Or use skopeo
skopeo inspect docker://quay.io/openshift-lightspeed/lightspeed-operator:v1.0.6 \
  | jq -r '.Digest'
```

**Issue: Running as root**

```dockerfile
# In Dockerfile
USER 65532:65532
```

**Issue: Missing required labels**

```dockerfile
LABEL name="lightspeed-operator" \
      vendor="Red Hat" \
      version="1.0.6" \
      release="1" \
      summary="OpenShift Lightspeed Operator" \
      description="Manages OpenShift Lightspeed deployments"
```

---

## CI/CD Integration

### GitHub Actions Example

```yaml
# .github/workflows/olm-tests.yaml
name: OLM Tests

on:
  pull_request:
    branches: [ main ]
  push:
    branches: [ main ]

jobs:
  bundle-validation:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Setup Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      
      - name: Install tools
        run: |
          make operator-sdk
          make yq
          make jq
      
      - name: Generate bundle
        run: make bundle BUNDLE_TAG=${{ github.sha }}
      
      - name: Validate bundle
        run: operator-sdk bundle validate ./bundle
      
      - name: Upload bundle artifacts
        uses: actions/upload-artifact@v3
        with:
          name: bundle
          path: bundle/

  catalog-validation:
    needs: bundle-validation
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Download bundle
        uses: actions/download-artifact@v3
        with:
          name: bundle
          path: bundle/
      
      - name: Install opm
        run: |
          wget https://github.com/operator-framework/operator-registry/releases/download/v1.28.0/linux-amd64-opm
          chmod +x linux-amd64-opm
          sudo mv linux-amd64-opm /usr/local/bin/opm
      
      - name: Validate catalog
        run: opm validate ./lightspeed-catalog-4.18

  e2e-tests:
    needs: catalog-validation
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Setup OpenShift cluster
        # Use kind/crc/existing cluster
        run: |
          # Setup cluster
          
      - name: Deploy operator
        run: |
          make deploy IMG=quay.io/openshift-lightspeed/lightspeed-operator:${{ github.sha }}
      
      - name: Run E2E tests
        run: make e2e-test
      
      - name: Collect must-gather
        if: failure()
        run: oc adm must-gather --dest-dir=./must-gather
      
      - name: Upload must-gather
        if: failure()
        uses: actions/upload-artifact@v3
        with:
          name: must-gather
          path: must-gather/
```

### Konflux Integration (Lightspeed Pattern)

The Lightspeed operator uses Konflux for CI/CD:

```yaml
# konflux-integration/pipeline.yaml
apiVersion: tekton.dev/v1beta1
kind: Pipeline
metadata:
  name: lightspeed-operator-pipeline
spec:
  tasks:
    - name: build-operator
      taskRef:
        name: buildah
      params:
        - name: IMAGE
          value: $(params.output-image)
    
    - name: validate-bundle
      runAfter: [build-operator]
      taskRef:
        name: operator-bundle-validate
      params:
        - name: BUNDLE_DIR
          value: ./bundle
    
    - name: run-e2e-tests
      runAfter: [validate-bundle]
      taskRef:
        name: ginkgo-test
      params:
        - name: TEST_DIR
          value: ./test/e2e
```

### Jenkins Pipeline

```groovy
// Jenkinsfile
pipeline {
    agent any
    
    environment {
        BUNDLE_TAG = "${env.GIT_COMMIT.take(7)}"
        BUNDLE_IMG = "quay.io/myorg/lightspeed-operator-bundle:${BUNDLE_TAG}"
    }
    
    stages {
        stage('Build & Validate Bundle') {
            steps {
                sh 'make bundle BUNDLE_TAG=${BUNDLE_TAG}'
                sh 'operator-sdk bundle validate ./bundle'
            }
        }
        
        stage('Build Bundle Image') {
            steps {
                sh 'make bundle-build BUNDLE_IMG=${BUNDLE_IMG}'
                sh 'make bundle-push BUNDLE_IMG=${BUNDLE_IMG}'
            }
        }
        
        stage('Install Operator') {
            steps {
                sh '''
                    oc apply -f - <<EOF
                    apiVersion: operators.coreos.com/v1alpha1
                    kind: CatalogSource
                    metadata:
                      name: test-catalog
                      namespace: openshift-marketplace
                    spec:
                      sourceType: grpc
                      image: ${CATALOG_IMG}
                    EOF
                '''
                
                // Create subscription and wait for operator
            }
        }
        
        stage('Run E2E Tests') {
            steps {
                sh 'make e2e-test'
            }
        }
    }
    
    post {
        failure {
            sh 'oc adm must-gather --dest-dir=./must-gather || true'
            archiveArtifacts artifacts: 'must-gather/**/*', allowEmptyArchive: true
        }
    }
}
```

---

## Manual Testing Checklist

### Pre-Release Testing Checklist

#### Bundle Validation
- [ ] `operator-sdk bundle validate ./bundle` passes
- [ ] CSV has all required fields
- [ ] relatedImages section complete
- [ ] OpenShift version annotations correct
- [ ] RBAC permissions reviewed and minimal
- [ ] Icon present and displays correctly

#### Catalog Validation
- [ ] `opm validate` passes for all catalogs
- [ ] Upgrade paths tested (sequential and skipRange)
- [ ] Channel entries correct
- [ ] Bundle references valid

#### Installation Testing
- [ ] Fresh install succeeds in test cluster
- [ ] Operator pod starts and stays running
- [ ] CRDs installed correctly
- [ ] RBAC created successfully
- [ ] Custom resource can be created
- [ ] Managed resources created as expected

#### Upgrade Testing
- [ ] Upgrade from previous version succeeds
- [ ] Skip range upgrade works
- [ ] Z-stream upgrade works
- [ ] Custom resources preserved during upgrade
- [ ] No downtime during upgrade
- [ ] New features work after upgrade

#### Functional Testing
- [ ] Create custom resource
- [ ] Update custom resource
- [ ] Delete custom resource (cleanup works)
- [ ] Status conditions update correctly
- [ ] Metrics exposed
- [ ] Logs are readable and useful
- [ ] Webhooks work (if any)

#### Multi-Version Testing
- [ ] Tested on oldest supported OpenShift version
- [ ] Tested on newest supported OpenShift version
- [ ] Tested on current production OpenShift version

#### Documentation
- [ ] CSV description accurate
- [ ] alm-examples valid and useful
- [ ] specDescriptors comprehensive
- [ ] statusDescriptors comprehensive
- [ ] External documentation linked

#### Security
- [ ] Operator doesn't run as root
- [ ] Secrets handled securely
- [ ] RBAC follows principle of least privilege
- [ ] Images use SHA256 digests
- [ ] No high/critical vulnerabilities

---

## Troubleshooting Test Failures

### Bundle Validation Failures

**Symptom:** `operator-sdk bundle validate` fails

**Diagnosis:**
```bash
operator-sdk bundle validate ./bundle -o text
```

**Common Fixes:**
- Fix YAML syntax errors
- Add missing required fields
- Correct image references
- Review RBAC format

### Catalog Validation Failures

**Symptom:** `opm validate` fails

**Diagnosis:**
```bash
opm validate ./lightspeed-catalog-4.18 --verbose
```

**Common Fixes:**
- Ensure schema types correct (olm.package, olm.channel, olm.bundle)
- Fix channel references
- Remove duplicate bundles
- Correct upgrade path

### Installation Failures

**Symptom:** CSV stuck in Pending or Installing

**Diagnosis:**
```bash
# Check CSV status
oc get csv -n openshift-lightspeed -o yaml

# Check InstallPlan
oc get installplan -n openshift-lightspeed -o yaml

# Check events
oc get events -n openshift-lightspeed --sort-by='.lastTimestamp'

# Check operator logs (if pod exists)
oc logs -n openshift-lightspeed deployment/lightspeed-operator-controller-manager
```

**Common Fixes:**
- Verify image pull secrets
- Check resource quotas
- Verify RBAC permissions
- Review OperatorGroup configuration

### Upgrade Failures

**Symptom:** Upgrade doesn't happen or fails

**Diagnosis:**
```bash
# Check Subscription
oc get subscription -n openshift-lightspeed -o yaml

# Check for new InstallPlan
oc get installplans -n openshift-lightspeed

# Check catalog
oc logs -n openshift-marketplace $(oc get pods -n openshift-marketplace -l olm.catalogSource=lightspeed-catalog -o name)
```

**Common Fixes:**
- Verify upgrade path (replaces/skipRange)
- Approve manual InstallPlan
- Remove startingCSV pin
- Refresh catalog

### E2E Test Failures

**Symptom:** Ginkgo tests fail

**Diagnosis:**
```bash
# Run with verbose output
go test ./test/e2e -v -ginkgo.v

# Check operator logs
oc logs -n openshift-lightspeed deployment/lightspeed-operator-controller-manager

# Collect must-gather
oc adm must-gather --dest-dir=./must-gather
```

**Common Fixes:**
- Increase timeouts (CONDITION_TIMEOUT_SECONDS)
- Check test prerequisites (secrets, CRDs)
- Review test setup (BeforeAll)
- Check for resource conflicts

### Scorecard Failures

**Symptom:** Scorecard tests fail

**Diagnosis:**
```bash
operator-sdk scorecard ./bundle \
  --namespace openshift-lightspeed \
  --output json | jq '.items[] | select(.status.results[].state == "fail")'
```

**Common Fixes:**
- Add missing spec/status descriptors
- Fix CRD validation schemas
- Update CSV annotations
- Ensure CRs can be created

---

## Additional Resources

### Related Guides

- **[OLM Bundle Management Guide](./olm-bundle-management.md)** - Creating and managing bundles
- **[OLM Catalog Management Guide](./olm-catalog-management.md)** - Organizing bundles into catalogs
- **[OLM Integration & Lifecycle Guide](./olm-integration-lifecycle.md)** - OLM integration and operator lifecycle
- **[Contributing Guide](../CONTRIBUTING.md)** - General contribution guidelines
- **[Architecture Documentation](../ARCHITECTURE.md)** - Operator architecture overview

### External Resources

- [Operator SDK Testing Guide](https://sdk.operatorframework.io/docs/testing-operators/)
- [Scorecard Documentation](https://sdk.operatorframework.io/docs/testing-operators/scorecard/)
- [Preflight Documentation](https://github.com/redhat-openshift-ecosystem/openshift-preflight)
- [OpenShift Operator Certification](https://redhat-connect.gitbook.io/certified-operator-guide/ocp-deployment/operator-metadata/testing-your-operator)
- [Ginkgo Testing Framework](https://onsi.github.io/ginkgo/)

### Project Testing Resources

**Lightspeed Operator Tests:**
- [`test/e2e/`](../test/e2e/) - End-to-end test suites
- `internal/controller/*_test.go` - Unit tests (co-located with controllers)
  - [`internal/controller/olsconfig_controller_test.go`](../internal/controller/olsconfig_controller_test.go)
  - [`internal/controller/appserver/reconciler_test.go`](../internal/controller/appserver/reconciler_test.go)
  - [`internal/controller/postgres/reconciler_test.go`](../internal/controller/postgres/reconciler_test.go)
  - [`internal/controller/console/reconciler_test.go`](../internal/controller/console/reconciler_test.go)
- [`Makefile`](../Makefile) - Test targets (test, e2e-test)
- `bundle/tests/scorecard/` - Scorecard configuration

**Key Make Targets:**
```bash
make test           # Run unit tests
make e2e-test       # Run E2E tests
make bundle         # Generate and validate bundle
make bundle-build   # Build bundle image
make deploy         # Deploy operator to cluster
```

---

**Testing is critical for operator reliability.** Follow this guide to ensure your operator bundles, catalogs, and deployments are production-ready.

For questions or issues with testing the Lightspeed Operator, see the main [README](../README.md) or [CONTRIBUTING](../CONTRIBUTING.md) guide.

