#!/usr/bin/env bash
# Rebuilds and pushes the skills OCI image on OpenShift.
# Skills are mounted as OCI image volumes — new sandbox pods
# to pick up the new volumes.
#
# Usage:
#   KUBECONFIG=/path/to/kubeconfig bash hack/agentic/redeploy-skills.sh
#   KUBECONFIG=/path/to/kubeconfig bash hack/agentic/redeploy-skills.sh --skip-build
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"
parse_args "$@"

[[ -d "${SKILLS_DIR}" ]] || fail "Skills directory not found: ${SKILLS_DIR}\nSet SKILLS_DIR env var to override."

check_cluster
ensure_buildconfigs

# Build skills image on cluster
build_on_cluster "${BC_SKILLS}" "${SKILLS_DIR}" "skills"

# Sandbox pods use OCI image volumes — new sandboxes will automatically
# pick up the updated skills image. No pod restart needed.

echo -e "\n${GREEN}Skills image redeployed.${NC}"
