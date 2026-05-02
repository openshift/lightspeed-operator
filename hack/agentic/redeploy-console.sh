#!/usr/bin/env bash
# Rebuilds and redeploys the lightspeed agentic console plugin.
# The operator manages the console deployment via --agentic-console-image.
# This script builds, pushes the new image, and restarts the deployment.
#
# Usage:
#   KUBECONFIG=/path/to/kubeconfig bash hack/agentic/redeploy-console.sh
#   KUBECONFIG=/path/to/kubeconfig bash hack/agentic/redeploy-console.sh --skip-build
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"
parse_args "$@"

[[ -d "${CONSOLE_DIR}" ]] || fail "Console directory not found: ${CONSOLE_DIR}"

check_cluster
ensure_buildconfigs

build_on_cluster "${BC_CONSOLE}" "${CONSOLE_DIR}" "console plugin"

step "Restarting console deployment"
rollout "${DEPLOY_CONSOLE}" "${NS_CONSOLE}" "Console plugin"

echo -e "\n${GREEN}Console plugin redeployed.${NC}"
