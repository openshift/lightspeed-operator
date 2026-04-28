#!/usr/bin/env bash
# Remove the full agentic stack from an OpenShift cluster.
#
# Usage:
#   KUBECONFIG=/path/to/kubeconfig bash hack/undeploy-agentic.sh
#   KUBECONFIG=/path/to/kubeconfig VERTEX_PROJECT=my-project bash hack/undeploy-agentic.sh  # also cleans GCP SA

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/agentic-lib.sh"

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
oc delete crd \
    olsconfigs.ols.openshift.io \
    agents.agentic.openshift.io \
    componenttools.agentic.openshift.io \
    llmproviders.agentic.openshift.io \
    proposals.agentic.openshift.io \
    workflows.agentic.openshift.io \
    --ignore-not-found >/dev/null 2>&1
info "CRDs removed"

step "Removing ImageDigestMirrorSet"
oc delete imagedigestmirrorset lightspeed-operator-openshift-lightspeed-prod-to-ci --ignore-not-found >/dev/null 2>&1
info "ImageDigestMirrorSet removed"

step "Removing namespace"
oc delete ns "${NS_OPERATOR}" --ignore-not-found >/dev/null 2>&1
info "Namespace ${NS_OPERATOR} removed"

if [[ -n "${VERTEX_PROJECT:-}" ]]; then
    step "Cleaning up Vertex AI credentials"
    cleanup_vertex_credentials
fi

step "Cleaning temp files"
rm -f /tmp/lightspeed-vertex-key-*
info "Temp files cleaned"

echo -e "\n${GREEN}Agentic stack fully removed.${NC}"
