#!/usr/bin/env bash
# Rebuilds and redeploys all agentic components on OpenShift.
# A faster alternative to full deploy — no CRD/namespace/secret churn.
#
# Usage:
#   KUBECONFIG=/path/to/kubeconfig bash hack/agentic/redeploy-all.sh
#   KUBECONFIG=/path/to/kubeconfig bash hack/agentic/redeploy-all.sh --skip-build
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"
parse_args "$@"

check_cluster
ensure_buildconfigs

###############################################################################
# Build all on cluster (parallel)
###############################################################################
step "Building all images in parallel"
start_operator_build_async

[[ -d "${AGENT_DIR}" ]] \
    && start_build_async "${BC_AGENT}" "${AGENT_DIR}" "agent sandbox" \
    || warn "Agent directory not found, skipping: ${AGENT_DIR}"

[[ -d "${SKILLS_DIR}" ]] \
    && start_build_async "${BC_SKILLS}" "${SKILLS_DIR}" "skills" \
    || warn "Skills directory not found, skipping: ${SKILLS_DIR}"

[[ -d "${CONSOLE_DIR}" ]] \
    && start_build_async "${BC_CONSOLE}" "${CONSOLE_DIR}" "console plugin" \
    || warn "Console directory not found, skipping: ${CONSOLE_DIR}"

wait_all_builds

update_crds_and_rbac

###############################################################################
# Rollout all (pause operator while patching console image)
###############################################################################
step "Rolling out all deployments"
pause_operator
rollout "${DEPLOY_OPERATOR}" "${NS_OPERATOR}" "Operator"

# Sandbox pods are ephemeral — created per-proposal by the operator.
info "Agent image updated. New sandbox pods will use the updated image."

if oc get "deployment/${DEPLOY_CONSOLE}" -n "${NS_CONSOLE}" >/dev/null 2>&1; then
    patch_console_image
    rollout "${DEPLOY_CONSOLE}" "${NS_CONSOLE}" "Console plugin"
fi
resume_operator

###############################################################################
# Summary
###############################################################################
step "Deployed digests"
show_digest "${DEPLOY_OPERATOR}" "${NS_OPERATOR}" "Operator"
if oc get "deployment/${DEPLOY_CONSOLE}" -n "${NS_CONSOLE}" >/dev/null 2>&1; then
    show_digest "${DEPLOY_CONSOLE}" "${NS_CONSOLE}" "Console"
fi

echo -e "\n${GREEN}All agentic components redeployed.${NC}"
