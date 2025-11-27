# OLM Testing & Validation Guide

This guide covers testing and validation for the OpenShift Lightspeed Operator.

> **📖 For Testing Fundamentals:** See [Operator Testing Guide](https://sdk.operatorframework.io/docs/testing-operators/)  
> **📖 For Test Implementation:** See `test/e2e/` directory for actual test code

---

## Overview

Testing layers:
1. **Unit Tests**: `make test` (NEVER use `go test` directly)
2. **Bundle/Catalog Validation**: `operator-sdk bundle validate` / `opm validate`
3. **E2E Tests**: `make test-e2e`
4. **Scorecard**: `operator-sdk scorecard` (OLM best practices)
5. **Preflight**: Red Hat certification (optional)

---

## Unit Tests

**Location:** Co-located `*_test.go` files in each package

**Framework:** Ginkgo v2 + Gomega

**Run:**
```bash
make test  # CRITICAL: Always use make test, sets up envtest + CRDs
```

**View coverage:**
```bash
go tool cover -html=cover.out
```

---

## E2E Tests

**Location:** `test/e2e/`

**Key files:**
- `suite_test.go` - Test setup
- `reconciliation_test.go` - OLSConfig reconciliation
- `upgrade_test.go` - Upgrade scenarios
- `client.go` - Custom client with wait helpers
- `utils.go` - Test utilities (`SetupOLSTestEnvironment()`, `mustGather()`)

**Run:**
```bash
export KUBECONFIG=/path/to/kubeconfig
export LLM_TOKEN=your-token
make test-e2e

# Run specific test
make test-e2e GINKGO_FOCUS="PostgreSQL"

# Verbose output
make test-e2e GINKGO_V=true

# Increase timeout for slow environments
export CONDITION_TIMEOUT=10m
make test-e2e
```

---

## Bundle & Catalog Validation

### Bundle

```bash
# Basic validation (automatic with make bundle)
operator-sdk bundle validate ./bundle

# OpenShift-specific
operator-sdk bundle validate ./bundle --select-optional name=operatorhub

# Verbose
operator-sdk bundle validate ./bundle -o text
```

### Catalog

```bash
# Validate single catalog
opm validate lightspeed-catalog-4.18

# Validate all catalogs
for d in lightspeed-catalog*; do opm validate $d; done
```

---

## Installation Testing

**Quick test:**
```bash
# 1. Deploy catalog
oc apply -f hack/example_catalogsource.yaml

# 2. Create subscription (see olm-integration-lifecycle.md for full YAML)
oc apply -f subscription.yaml

# 3. Wait and verify
oc get csv -n openshift-lightspeed -w
oc apply -f config/samples/ols_v1alpha1_olsconfig.yaml
oc get olsconfig cluster -o yaml
```

> **📖 Full Installation Steps:** See [OLM Integration & Lifecycle](./olm-integration-lifecycle.md)

---

## Upgrade Testing

**Automated:** `test/e2e/upgrade_test.go`

**Manual test:**
```bash
# 1. Install old version
oc apply -f old-subscription.yaml

# 2. Wait for installation
oc get csv -n openshift-lightspeed -w

# 3. Upgrade (update catalog or subscription)
oc apply -f new-subscription.yaml

# 4. Verify
oc get csv -n openshift-lightspeed -w
oc get olsconfig cluster -o yaml
oc get deployments -n openshift-lightspeed -o yaml | grep image
```

---

## Scorecard Testing

```bash
# Run all OLM tests
operator-sdk scorecard bundle/ --selector=suite=olm --wait-time=5m

# Run specific test
operator-sdk scorecard bundle/ --selector=test=basic-check-spec
```

**Common tests:**
- `olm-bundle-validation` - Bundle structure
- `olm-crds-have-validation` - CRD validation rules
- `olm-spec-descriptors` / `olm-status-descriptors` - UI descriptors

> **📖 Scorecard Details:** See [Scorecard Documentation](https://sdk.operatorframework.io/docs/testing-operators/scorecard/)

---

## Preflight Testing

Red Hat certification (optional):

```bash
# Check bundle
preflight check operator bundle/ --docker-config=$HOME/.docker/config.json

# Check image
preflight check container quay.io/org/lightspeed-operator:v0.1.0
```

> **📖 Preflight Details:** See [Preflight Documentation](https://github.com/redhat-openshift-ecosystem/openshift-preflight)

---

## Quick Reference

### Run Full Test Suite

```bash
make test                                              # Unit tests
make test-e2e                                          # E2E tests (set KUBECONFIG, LLM_TOKEN)
operator-sdk bundle validate ./bundle                  # Bundle validation
for d in lightspeed-catalog*; do opm validate $d; done # Catalog validation
operator-sdk scorecard bundle/ --selector=suite=olm   # Scorecard
```

### Troubleshooting

**E2E tests failing:**
```bash
make test-e2e 2>&1 | tee test.log
oc logs -n openshift-lightspeed deployment/lightspeed-operator-controller-manager
oc get all -n openshift-lightspeed
```

Common: Missing `LLM_TOKEN`, insufficient permissions, timeouts (increase `CONDITION_TIMEOUT`)

**Bundle validation fails:**
```bash
operator-sdk bundle validate ./bundle -o text  # Verbose output
```

Common: Invalid CSV syntax, missing required fields, malformed RBAC

**Scorecard fails:**
```bash
operator-sdk scorecard bundle/ --selector=test=<failing-test> -o text
```

Common: Missing descriptors, CRD validation not defined

---

## Additional Resources

- [OLM Bundle Management](./olm-bundle-management.md)
- [OLM Catalog Management](./olm-catalog-management.md)
- [OLM Integration & Lifecycle](./olm-integration-lifecycle.md)
- [Operator Testing Guide](https://sdk.operatorframework.io/docs/testing-operators/)
- [Ginkgo Framework](https://onsi.github.io/ginkgo/)
