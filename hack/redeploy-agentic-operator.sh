#!/usr/bin/env bash
# Rebuilds and redeploys the lightspeed-operator on OpenShift.
#
# Usage:
#   KUBECONFIG=/path/to/kubeconfig bash hack/redeploy-operator.sh
#   KUBECONFIG=/path/to/kubeconfig bash hack/redeploy-operator.sh --skip-build
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/agentic-lib.sh"
parse_args "$@"

check_cluster
get_registry

# Build — use Dockerfile.dev with workspace root as context so local
# lightspeed-agentic-operator changes are picked up without pushing.
build_image "operator" "${OPERATOR_DIR}" "${IMG_OPERATOR}" "Dockerfile.dev" "${WORKSPACE_ROOT}"

# Push
step "Pushing operator image"
push_image "lightspeed-operator" "${NS_OPERATOR}" "${IMG_OPERATOR}"

# Re-apply CRDs and RBAC (may have changed since initial deploy)
step "Updating CRDs and RBAC"
cd "${OPERATOR_DIR}"
make manifests kustomize >/dev/null 2>&1
oc apply -f config/crd/bases/ >/dev/null 2>&1
bin/kustomize build config/default \
    | oc apply -f - -l app.kubernetes.io/component=rbac --server-side --force-conflicts >/dev/null 2>&1 \
    || warn "RBAC update via kustomize failed — may need full deploy-agentic.sh"
info "CRDs and RBAC updated"

# Rollout
step "Rolling out operator"
rollout "${DEPLOY_OPERATOR}" "${NS_OPERATOR}" "Operator"

# Verify
step "Verifying"
oc logs "deployment/${DEPLOY_OPERATOR}" -n "${NS_OPERATOR}" --tail=5 2>&1 | grep -v "^$"
echo -e "\n${GREEN}Operator redeployed.${NC}"
