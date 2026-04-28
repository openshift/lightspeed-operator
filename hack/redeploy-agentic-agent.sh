#!/usr/bin/env bash
# Rebuilds and redeploys the lightspeed-agentic-sandbox (chat pod) on OpenShift.
# Also builds/pushes the skills image and ensures the SandboxTemplate
# uses an OCI image volume (not emptyDir) for /app/skills.
#
# Usage:
#   KUBECONFIG=/path/to/kubeconfig bash hack/redeploy-agent.sh
#   KUBECONFIG=/path/to/kubeconfig bash hack/redeploy-agent.sh --skip-build
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/agentic-lib.sh"
parse_args "$@"

AGENT_POD="lightspeed-chat"
SKILLS_IMAGE="${INTERNAL_REG}/${NS_OPERATOR}/lightspeed-skills:${TAG}"

[[ -d "${AGENT_DIR}" ]] || fail "Agent directory not found: ${AGENT_DIR}"

check_cluster
get_registry

# Build and push agent image
build_image "lightspeed-agentic-sandbox" "${AGENT_DIR}" "${IMG_AGENT}"

step "Pushing agent sandbox image"
push_image "lightspeed-agentic-sandbox" "${NS_OPERATOR}" "${IMG_AGENT}"

# Build and push skills image (if skills dir exists)
if [[ -d "${SKILLS_DIR}" ]]; then
    build_image "lightspeed-skills" "${SKILLS_DIR}" "${IMG_SKILLS}" "Containerfile"

    step "Pushing skills image"
    push_image "lightspeed-skills" "${NS_OPERATOR}" "${IMG_SKILLS}"
fi

# Ensure SandboxTemplate uses image volume for skills (not emptyDir)
step "Ensuring skills image volume in SandboxTemplate"
CURRENT_SKILLS_VOL=$(oc get sandboxtemplate lightspeed-chat -n "${NS_OPERATOR}" \
    -o jsonpath='{.spec.podTemplate.spec.volumes[?(@.name=="skills")].image.reference}' 2>/dev/null)
if [[ -z "${CURRENT_SKILLS_VOL}" ]]; then
    echo "    Patching SandboxTemplate: emptyDir → image volume..."
    VOL_INDEX=$(oc get sandboxtemplate lightspeed-chat -n "${NS_OPERATOR}" \
        -o json 2>/dev/null | python3 -c "
import json,sys
d=json.load(sys.stdin)
for i,v in enumerate(d['spec']['podTemplate']['spec']['volumes']):
    if v.get('name')=='skills': print(i); break
" 2>/dev/null)
    if [[ -n "${VOL_INDEX}" ]]; then
        oc patch sandboxtemplate lightspeed-chat -n "${NS_OPERATOR}" --type=json -p "[
          {\"op\": \"replace\", \"path\": \"/spec/podTemplate/spec/volumes/${VOL_INDEX}\", \"value\": {
            \"name\": \"skills\",
            \"image\": {
              \"reference\": \"${SKILLS_IMAGE}\",
              \"pullPolicy\": \"Always\"
            }
          }}
        ]" >/dev/null 2>&1
        info "SandboxTemplate patched"
    else
        warn "Could not find skills volume index — patch manually"
    fi
else
    info "SandboxTemplate already uses image volume: ${CURRENT_SKILLS_VOL}"
fi

step "Restarting agent pod"
restart_pod "${AGENT_POD}" "${NS_OPERATOR}" "Agent pod"

# Verify skills are mounted
SKILLS_COUNT=$(oc exec -n "${NS_OPERATOR}" "${AGENT_POD}" -c agent -- ls /app/skills/.claude/skills/ 2>/dev/null | wc -l)
if [[ "${SKILLS_COUNT}" -gt 0 ]]; then
    info "Skills mounted: ${SKILLS_COUNT} skills in /app/skills/.claude/skills/"
else
    warn "Skills directory is empty — check image volume configuration"
fi

echo -e "\n${GREEN}Agent redeployed.${NC}"
