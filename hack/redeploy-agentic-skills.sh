#!/usr/bin/env bash
# Rebuilds and pushes all skills OCI images (full + per-profile) on OpenShift.
# Skills are mounted as OCI image volumes — the agent pod needs a restart
# to pick up the new volumes.
#
# Usage:
#   KUBECONFIG=/path/to/kubeconfig bash hack/redeploy-skills.sh
#   KUBECONFIG=/path/to/kubeconfig bash hack/redeploy-skills.sh --skip-build
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/agentic-lib.sh"
parse_args "$@"

[[ -d "${SKILLS_DIR}" ]] || fail "Skills directory not found: ${SKILLS_DIR}\nSet SKILLS_DIR env var to override."

check_cluster
get_registry

# Build and push full image (all skills)
build_image "skills" "${SKILLS_DIR}" "${IMG_SKILLS}" "Containerfile"

step "Pushing full skills image"
push_image "lightspeed-skills" "${NS_OPERATOR}" "${IMG_SKILLS}"

# Build and push per-profile images
for profile in "${SKILLS_PROFILES[@]}"; do
    img_var="IMG_SKILLS_${profile^^}"
    img_tag="${!img_var}"

    build_image "skills-${profile}" "${SKILLS_DIR}" "${img_tag}" "Containerfile.${profile}"

    step "Pushing skills-${profile} image"
    push_image "lightspeed-skills-${profile}" "${NS_OPERATOR}" "${img_tag}"
done

# Sandbox pods use OCI image volumes — new sandboxes will automatically
# pick up the updated skills images. No pod restart needed.

echo -e "\n${GREEN}All skills images redeployed.${NC}"
