#!/usr/bin/env bash
# Rebuilds and redeploys all agentic components on OpenShift.
# A faster alternative to full deploy — no CRD/namespace/secret churn.
#
# Usage:
#   KUBECONFIG=/path/to/kubeconfig bash hack/redeploy-agentic.sh
#   KUBECONFIG=/path/to/kubeconfig bash hack/redeploy-agentic.sh --skip-build
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/agentic-lib.sh"
parse_args "$@"

check_cluster
get_registry

###############################################################################
# Build all
###############################################################################
step "Building all images"
build_image "operator" "${OPERATOR_DIR}" "${IMG_OPERATOR}"

if [[ -d "${AGENT_DIR}" ]]; then
    build_image "lightspeed-agentic-sandbox" "${AGENT_DIR}" "${IMG_AGENT}"
else
    warn "Agent directory not found, skipping: ${AGENT_DIR}"
fi

if [[ -d "${SKILLS_DIR}" ]]; then
    build_image "skills" "${SKILLS_DIR}" "${IMG_SKILLS}" "Containerfile"
else
    warn "Skills directory not found, skipping: ${SKILLS_DIR}"
fi

if [[ -d "${CONSOLE_DIR}" ]]; then
    build_image "console-plugin" "${CONSOLE_DIR}" "${IMG_CONSOLE}"
else
    warn "Console directory not found, skipping: ${CONSOLE_DIR}"
fi

###############################################################################
# Push all
###############################################################################
step "Pushing all images"
push_image "lightspeed-operator" "${NS_OPERATOR}" "${IMG_OPERATOR}"

if [[ -d "${AGENT_DIR}" ]]; then
    push_image "lightspeed-agentic-sandbox" "${NS_OPERATOR}" "${IMG_AGENT}"
fi

if [[ -d "${SKILLS_DIR}" ]]; then
    push_image "lightspeed-skills" "${NS_OPERATOR}" "${IMG_SKILLS}"
fi

if [[ -d "${CONSOLE_DIR}" ]]; then
    push_image "lightspeed-console-plugin" "${NS_CONSOLE}" "${IMG_CONSOLE}"
fi

###############################################################################
# Update CRDs and RBAC
###############################################################################
step "Updating CRDs and RBAC"
cd "${OPERATOR_DIR}"
make manifests kustomize >/dev/null 2>&1
oc apply -f config/crd/bases/ >/dev/null 2>&1
bin/kustomize build config/default \
    | oc apply -f - -l app.kubernetes.io/component=rbac --server-side --force-conflicts >/dev/null 2>&1 \
    || warn "RBAC update via kustomize failed — may need full deploy-agentic.sh"
info "CRDs and RBAC updated"

###############################################################################
# Rollout all (pause operator while patching console image)
###############################################################################
step "Rolling out all deployments"
pause_operator
rollout "${DEPLOY_OPERATOR}" "${NS_OPERATOR}" "Operator"

# Restart agent sandbox pod if it exists
if oc get pod lightspeed-chat -n "${NS_OPERATOR}" >/dev/null 2>&1; then
    oc delete pod lightspeed-chat -n "${NS_OPERATOR}" >/dev/null 2>&1
    info "Agent pod restarted"
fi

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
