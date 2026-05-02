#!/usr/bin/env bash
# Rebuilds and redeploys the lightspeed-operator on OpenShift.
#
# Usage:
#   KUBECONFIG=/path/to/kubeconfig bash hack/agentic/redeploy-operator.sh
#   KUBECONFIG=/path/to/kubeconfig bash hack/agentic/redeploy-operator.sh --skip-build
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"
parse_args "$@"

check_cluster
ensure_buildconfigs

build_operator

update_crds_and_rbac

# Rollout
step "Rolling out operator"
rollout "${DEPLOY_OPERATOR}" "${NS_OPERATOR}" "Operator"

# Verify
step "Verifying"
oc logs "deployment/${DEPLOY_OPERATOR}" -n "${NS_OPERATOR}" --tail=5 2>&1 | grep -v "^$"
echo -e "\n${GREEN}Operator redeployed.${NC}"
