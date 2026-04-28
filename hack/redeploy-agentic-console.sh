#!/usr/bin/env bash
# Rebuilds and redeploys the lightspeed agentic console plugin.
# The operator manages the console deployment via --agentic-console-image.
# This script builds, pushes the new image, and restarts the deployment.
#
# Usage:
#   KUBECONFIG=/path/to/kubeconfig bash hack/redeploy-agentic-console.sh
#   KUBECONFIG=/path/to/kubeconfig bash hack/redeploy-agentic-console.sh --skip-build
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/agentic-lib.sh"
parse_args "$@"

[[ -d "${CONSOLE_DIR}" ]] || fail "Console directory not found: ${CONSOLE_DIR}"

check_cluster
get_registry

build_image "console-plugin" "${CONSOLE_DIR}" "${IMG_CONSOLE}"

step "Pushing console plugin image"
push_image "lightspeed-console-plugin" "${NS_CONSOLE}" "${IMG_CONSOLE}"

step "Restarting console deployment"
rollout "${DEPLOY_CONSOLE}" "${NS_CONSOLE}" "Console plugin"

echo -e "\n${GREEN}Console plugin redeployed.${NC}"
