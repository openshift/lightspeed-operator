#!/usr/bin/env bash
# Remove the full agentic stack from an OpenShift cluster.
#
# Usage:
#   KUBECONFIG=/path/to/kubeconfig bash hack/agentic/undeploy.sh
#   KUBECONFIG=/path/to/kubeconfig VERTEX_PROJECT=my-project bash hack/agentic/undeploy.sh  # also cleans GCP SA

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

check_cluster

step "Removing operator deployment"
oc delete deployment "${DEPLOY_OPERATOR}" -n "${NS_OPERATOR}" --ignore-not-found >/dev/null 2>&1
info "Operator deployment deleted"

step "Removing cluster-scoped RBAC"
oc delete clusterrolebinding \
    lightspeed-operator-manager-rolebinding \
    lightspeed-operator-agentic-manager-rolebinding \
    lightspeed-operator-ols-metrics-reader \
    --ignore-not-found >/dev/null 2>&1
oc delete clusterrole \
    lightspeed-operator-manager-role \
    lightspeed-operator-agentic-manager-role \
    lightspeed-operator-ols-metrics-reader \
    lightspeed-operator-query-access \
    --ignore-not-found >/dev/null 2>&1
info "Cluster RBAC removed"

step "Removing CRDs"
AGENTIC_CRDS=$(oc get crd -o name 2>/dev/null | grep 'agentic\.openshift\.io' || true)
OLS_CRDS="customresourcedefinition.apiextensions.k8s.io/olsconfigs.ols.openshift.io"
ALL_CRDS="${AGENTIC_CRDS} ${OLS_CRDS}"
if [[ -n "${ALL_CRDS// /}" ]]; then
    # Delete all instances first to prevent finalizer hangs
    for crd in ${AGENTIC_CRDS}; do
        crd_name="${crd#*/}"
        resource="${crd_name%%.*}"
        oc delete "${resource}.agentic.openshift.io" --all -A --timeout=10s 2>/dev/null || true
    done
    # Delete CRDs with timeout, then force-clear finalizers on any that hang
    for crd in ${ALL_CRDS}; do
        crd_name="${crd#*/}"
        if oc get crd "${crd_name}" >/dev/null 2>&1; then
            if ! timeout 15 oc delete crd "${crd_name}" --ignore-not-found 2>/dev/null; then
                warn "${crd_name} stuck — clearing finalizers"
                oc patch crd "${crd_name}" -p '{"metadata":{"finalizers":[]}}' --type=merge 2>/dev/null || true
                timeout 10 oc delete crd "${crd_name}" --ignore-not-found 2>/dev/null || true
            fi
        fi
    done
fi
info "CRDs removed"

step "Removing ImageDigestMirrorSet"
oc delete imagedigestmirrorset lightspeed-operator-openshift-lightspeed-prod-to-ci --ignore-not-found >/dev/null 2>&1
info "ImageDigestMirrorSet removed"

step "Removing namespace"
oc delete ns "${NS_OPERATOR}" --ignore-not-found >/dev/null 2>&1
info "Namespace ${NS_OPERATOR} removed"


echo -e "\n${GREEN}Agentic stack fully removed.${NC}"
