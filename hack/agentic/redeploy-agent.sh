#!/usr/bin/env bash
# Rebuilds and redeploys the lightspeed-agentic-sandbox (chat pod) on OpenShift.
# Also builds/pushes the skills image and ensures the SandboxTemplate
# uses an OCI image volume (not emptyDir) for /app/skills.
#
# Usage:
#   KUBECONFIG=/path/to/kubeconfig bash hack/agentic/redeploy-agent.sh
#   KUBECONFIG=/path/to/kubeconfig bash hack/agentic/redeploy-agent.sh --skip-build
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"
parse_args "$@"

SKILLS_IMAGE="${INTERNAL_REG}/${NS_OPERATOR}/lightspeed-skills:${TAG}"

[[ -d "${AGENT_DIR}" ]] || fail "Agent directory not found: ${AGENT_DIR}"

check_cluster
ensure_buildconfigs

# Build agent and skills on cluster (no local container engine needed)
build_on_cluster "${BC_AGENT}" "${AGENT_DIR}" "agent sandbox"

if [[ -d "${SKILLS_DIR}" ]]; then
    build_on_cluster "${BC_SKILLS}" "${SKILLS_DIR}" "skills"
fi

# Ensure SandboxTemplate uses image volume for skills (not emptyDir)
step "Ensuring skills image volume in SandboxTemplate"
CURRENT_SKILLS_VOL=$(oc get sandboxtemplate lightspeed-agent -n "${NS_OPERATOR}" \
    -o jsonpath='{.spec.podTemplate.spec.volumes[?(@.name=="skills")].image.reference}' 2>/dev/null)
if [[ -z "${CURRENT_SKILLS_VOL}" ]]; then
    echo "    Patching SandboxTemplate: emptyDir → image volume..."
    VOL_INDEX=$(oc get sandboxtemplate lightspeed-agent -n "${NS_OPERATOR}" \
        -o json 2>/dev/null | python3 -c "
import json,sys
d=json.load(sys.stdin)
for i,v in enumerate(d['spec']['podTemplate']['spec']['volumes']):
    if v.get('name')=='skills': print(i); break
" 2>/dev/null)
    if [[ -n "${VOL_INDEX}" ]]; then
        oc patch sandboxtemplate lightspeed-agent -n "${NS_OPERATOR}" --type=json -p "[
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

# Sandbox pods are ephemeral — created per-proposal by the operator.
# Existing sandbox templates are garbage-collected when config changes.
info "Agent image updated. New sandbox pods will use the updated image."

echo -e "\n${GREEN}Agent redeployed.${NC}"
