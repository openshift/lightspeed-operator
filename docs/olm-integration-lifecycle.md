# OLM Integration & Operator Lifecycle Guide

This guide covers deploying and managing the OpenShift Lightspeed Operator through OLM.

> **📖 For OLM Fundamentals:** See [OLM Architecture](https://olm.operatorframework.io/docs/concepts/crds/)  
> **📖 Prerequisites:** [Bundle Management](./olm-bundle-management.md) and [Catalog Management](./olm-catalog-management.md)

---

## Overview

OLM manages operator installation, upgrades, and removal through:
- **CatalogSource**: Makes your catalog available to the cluster
- **Subscription**: Declares intent to install an operator
- **InstallPlan**: Executes the installation/upgrade
- **CSV**: Running operator managed by OLM

**Workflow:**
```
Deploy CatalogSource → Create Subscription → OLM creates InstallPlan → CSV installed → Operator runs
```

---

## Installing the Operator

### Step 1: Deploy CatalogSource

**Example:** See `hack/example_catalogsource.yaml`

```bash
oc apply -f hack/example_catalogsource.yaml

# Verify
oc get catalogsource -n openshift-marketplace
oc get pods -n openshift-marketplace | grep lightspeed-catalog
```

**Key fields:**
- `sourceType: grpc`
- `image: quay.io/openshift-lightspeed/lightspeed-catalog:v4.18-latest`
- `updateStrategy.registryPoll.interval: 30m` (polls for catalog updates)

### Step 2: Create Subscription

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
  installPlanApproval: Automatic  # or Manual for controlled upgrades
```

**Deploy:**
```bash
oc create namespace openshift-lightspeed
oc apply -f subscription.yaml

# Watch installation
oc get csv -n openshift-lightspeed -w
```

### Step 3: Verify Installation

```bash
# Check CSV status (should show "Succeeded")
oc get csv -n openshift-lightspeed

# Check operator pod
oc get pods -n openshift-lightspeed

# Check logs
oc logs -n openshift-lightspeed deployment/lightspeed-operator-controller-manager -f
```

**CSV phases:**
- `Succeeded` - Operator running normally
- `Installing` - Installation in progress
- `Failed` - Installation failed (check logs)
- `Replacing` - Upgrade in progress

---

## Upgrade Management

### Automatic Upgrades

With `installPlanApproval: Automatic`, OLM upgrades automatically when new versions appear in the catalog.

```bash
# Watch for new CSV
oc get csv -n openshift-lightspeed -w

# Check events
oc get events -n openshift-lightspeed --sort-by='.lastTimestamp'
```

### Manual Upgrades

With `installPlanApproval: Manual`, approve upgrades explicitly:

```bash
# List pending install plans
oc get installplan -n openshift-lightspeed

# Approve upgrade
oc patch installplan <install-plan-name> -n openshift-lightspeed \
  --type merge -p '{"spec":{"approved":true}}'

# Watch upgrade
oc get csv -n openshift-lightspeed -w
```

**Recommendation:** Use `Automatic` for dev/staging, `Manual` for production.

---

## Uninstalling the Operator

```bash
# 1. Delete custom resources (triggers operand cleanup)
oc delete olsconfig cluster

# 2. Delete subscription
oc delete subscription lightspeed-operator -n openshift-lightspeed

# 3. Delete CSV
oc delete csv -n openshift-lightspeed \
  -l operators.coreos.com/lightspeed-operator.openshift-lightspeed=

# 4. (Optional) Remove namespace
oc delete namespace openshift-lightspeed

# 5. (Optional) Remove catalog
oc delete catalogsource lightspeed-catalog -n openshift-marketplace
```

---

## Troubleshooting

### Installation Issues

**Symptoms:** CSV not appearing, InstallPlan stuck

**Diagnosis:**
```bash
# Check subscription
oc get subscription lightspeed-operator -n openshift-lightspeed -o yaml

# Check install plan
oc get installplan -n openshift-lightspeed -o yaml

# Check events
oc get events -n openshift-lightspeed | grep -E 'CSV|InstallPlan'

# Check catalog pod
oc get pods -n openshift-marketplace
oc logs -n openshift-marketplace <catalog-pod>
```

**Common causes:**
- CatalogSource pod not running or image pull failed
- Invalid CSV (missing RBAC, malformed YAML)
- Network issues pulling images
- Insufficient cluster permissions

### Runtime Issues

**Symptoms:** Operator crashlooping, OLSConfig not reconciling

**Diagnosis:**
```bash
# Operator logs
oc logs -n openshift-lightspeed \
  deployment/lightspeed-operator-controller-manager | grep -i error

# CSV status and conditions
oc get csv -n openshift-lightspeed -o yaml

# OLSConfig status
oc get olsconfig cluster -o yaml
```

**Common causes:**
- Missing required secrets (LLM credentials)
- RBAC permission issues
- Invalid OLSConfig specification
- Resource limits too low

**Quick fixes:**
```bash
# Restart operator
oc rollout restart deployment/lightspeed-operator-controller-manager \
  -n openshift-lightspeed

# Delete failed InstallPlan to retry
oc delete installplan <failed-plan> -n openshift-lightspeed

# Check RBAC
oc auth can-i create deployments \
  --as=system:serviceaccount:openshift-lightspeed:lightspeed-operator-controller-manager
```

### Check Logs

```bash
# Operator logs
oc logs -n openshift-lightspeed deployment/lightspeed-operator-controller-manager -f

# Previous logs (after restart)
oc logs -n openshift-lightspeed deployment/lightspeed-operator-controller-manager --previous

# OLM catalog operator
oc logs -n openshift-operator-lifecycle-manager deployment/catalog-operator -f
```

---

## Additional Resources

- [OLM Bundle Management](./olm-bundle-management.md)
- [OLM Catalog Management](./olm-catalog-management.md)
- [OLM Testing & Validation](./olm-testing-validation.md)
- [OLM Architecture](https://olm.operatorframework.io/docs/concepts/crds/)
- [Subscription API](https://olm.operatorframework.io/docs/concepts/crds/subscription/)
