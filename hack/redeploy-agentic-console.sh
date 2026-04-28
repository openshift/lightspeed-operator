#!/usr/bin/env bash
# Rebuilds and redeploys the lightspeed console plugin on OpenShift.
# Pauses the operator reconciler during deploy to prevent image revert.
#
# Usage:
#   KUBECONFIG=/path/to/kubeconfig bash hack/redeploy-console.sh
#   KUBECONFIG=/path/to/kubeconfig bash hack/redeploy-console.sh --skip-build
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/agentic-lib.sh"
parse_args "$@"

[[ -d "${CONSOLE_DIR}" ]] || fail "Console directory not found: ${CONSOLE_DIR}"

check_cluster
get_registry

build_image "console-plugin" "${CONSOLE_DIR}" "${IMG_CONSOLE}"

step "Pushing console plugin image"
push_image "lightspeed-console-plugin" "${NS_CONSOLE}" "${IMG_CONSOLE}"

step "Deploying console plugin"
pause_operator
patch_console_image
rollout "${DEPLOY_CONSOLE}" "${NS_CONSOLE}" "Console plugin"
resume_operator

echo -e "\n${GREEN}Console plugin redeployed.${NC}"
