#!/usr/bin/env bash
# Shared helpers for agentic deploy/redeploy scripts.
# Source this file; do not execute directly.

set -euo pipefail

RED='\033[0;31m' GREEN='\033[0;32m' CYAN='\033[0;36m' YELLOW='\033[0;33m' NC='\033[0m'
step()  { echo -e "\n${CYAN}==> $1${NC}"; }
info()  { echo -e "    ${GREEN}✓${NC} $1"; }
warn()  { echo -e "    ${YELLOW}!${NC} $1"; }
fail()  { echo -e "    ${RED}✗${NC} $1"; exit 1; }

# Paths — this file lives in lightspeed-operator/hack/
# Sibling repos are next to the operator repo in the workspace.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[1]}")" && pwd)"
OPERATOR_DIR="$(dirname "${SCRIPT_DIR}")"
WORKSPACE_ROOT="$(dirname "${OPERATOR_DIR}")"

CONSOLE_DIR="${CONSOLE_DIR:-${WORKSPACE_ROOT}/lightspeed-agentic-console}"
SKILLS_DIR="${SKILLS_DIR:-${WORKSPACE_ROOT}/lightspeed-skills}"
AGENT_DIR="${AGENT_DIR:-${WORKSPACE_ROOT}/lightspeed-agentic-sandbox}"

# Namespaces
NS_OPERATOR="openshift-lightspeed"
NS_CONSOLE="openshift-lightspeed"

# Deployment names (match operator constants.go)
DEPLOY_OPERATOR="lightspeed-operator-controller-manager"
DEPLOY_CONSOLE="lightspeed-console-plugin"

# Image tag — unique per worktree so concurrent deploys don't clobber each other.
# .worktrees/<name>/ → "wt-<name>", main repo → "latest".
if [[ "${WORKSPACE_ROOT}" == */.worktrees/* ]]; then
    TAG="wt-$(basename "${WORKSPACE_ROOT}")"
else
    TAG="latest"
fi

# Local image tags (used for docker build, then pushed via skopeo)
IMG_OPERATOR="lightspeed-operator:${TAG}"
IMG_CONSOLE="lightspeed-console-plugin:${TAG}"
IMG_AGENT="lightspeed-agentic-sandbox:${TAG}"
IMG_SKILLS="lightspeed-skills:${TAG}"
IMG_SKILLS_DESIGN="lightspeed-skills-design:${TAG}"
IMG_SKILLS_REMEDIATE="lightspeed-skills-remediate:${TAG}"
IMG_SKILLS_ESCALATE="lightspeed-skills-escalate:${TAG}"
IMG_SKILLS_MONITOR="lightspeed-skills-monitor:${TAG}"

# Skills profiles — order matters for iteration
SKILLS_PROFILES=(design remediate escalate monitor)

# Internal registry endpoint (for image references inside the cluster)
INTERNAL_REG="image-registry.openshift-image-registry.svc:5000"

# Parse flags: --skip-build, --provider=<vertex|bedrock>, --with-demo
SKIP_BUILD=false
LLM_PROVIDER=""
WITH_DEMO=false
parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --skip-build) SKIP_BUILD=true; shift ;;
            --provider=*) LLM_PROVIDER="${1#*=}"; shift ;;
            --provider) LLM_PROVIDER="${2:-}"; shift 2 ;;
            --with-demo) WITH_DEMO=true; shift ;;
            --help|-h) show_usage; exit 0 ;;
            *) echo "Unknown flag: $1"; exit 1 ;;
        esac
    done
}

# Verify cluster access
check_cluster() {
    [[ -z "${KUBECONFIG:-}" ]] && fail "KUBECONFIG not set"
    oc whoami >/dev/null 2>&1 || fail "Cannot reach cluster (check KUBECONFIG)"
    info "Cluster: $(oc whoami --show-server 2>/dev/null | sed 's|https://||')"
}

# Get the external registry route
get_registry() {
    REGISTRY=$(oc get route default-route -n openshift-image-registry \
        -o jsonpath='{.spec.host}' 2>/dev/null) \
        || fail "Registry route not found. Enable with:\n    oc patch configs.imageregistry.operator.openshift.io/cluster --type merge -p '{\"spec\":{\"defaultRoute\":true}}'"
}

# Build an image for linux/amd64 (cross-compile from arm64 Mac)
build_image() {
    local name="$1" dir="$2" tag="$3" containerfile="${4:-Dockerfile}"
    if [[ "${SKIP_BUILD}" == "true" ]]; then
        warn "Skipping build of ${name}"
        return
    fi
    echo "    Building ${name} for linux/amd64..."
    docker build --platform linux/amd64 --provenance=false --sbom=false \
        -f "${dir}/${containerfile}" -t "${tag}" "${dir}" >/dev/null 2>&1
    info "${name} built ($(docker inspect "${tag}" --format '{{.Id}}' | cut -c8-19))"
}

# Push a locally built image to the OpenShift internal registry
push_image() {
    local name="$1" ns="$2" tag="$3"
    local token
    token=$(oc create token builder -n "${ns}" --duration=10m 2>/dev/null) \
        || fail "Cannot create builder token in ${ns}"
    echo "    Pushing ${name}..."
    skopeo copy --dest-tls-verify=false --dest-creds="unused:${token}" \
        "docker-daemon:${tag}" "docker://${REGISTRY}/${ns}/${name}:${TAG}" >/dev/null 2>&1
    info "${name} pushed"
}

# Delete and wait for a standalone pod (managed by SandboxTemplate, not a Deployment)
restart_pod() {
    local pod="$1" ns="$2" label="${3:-${pod}}"
    echo "    Deleting pod ${pod}..."
    oc delete pod -n "${ns}" "${pod}" --ignore-not-found >/dev/null 2>&1
    echo "    Waiting for pod to restart..."
    oc wait --for=condition=Ready pod/"${pod}" -n "${ns}" --timeout=120s >/dev/null 2>&1
    info "${label} ready"
}

# Rollout restart a deployment and wait for it
rollout() {
    local deploy="$1" ns="$2" label="$3"
    echo "    Restarting ${label}..."
    oc rollout restart "deployment/${deploy}" -n "${ns}" >/dev/null 2>&1
    oc rollout status "deployment/${deploy}" -n "${ns}" --timeout=180s >/dev/null 2>&1
    info "${label} ready"
}

# Pause operator reconciler so it doesn't revert console plugin image
pause_operator() {
    echo "    Pausing operator reconciler..."
    oc scale deployment/"${DEPLOY_OPERATOR}" -n "${NS_OPERATOR}" --replicas=0 >/dev/null 2>&1
}

# Resume operator reconciler
resume_operator() {
    echo "    Resuming operator reconciler..."
    oc scale deployment/"${DEPLOY_OPERATOR}" -n "${NS_OPERATOR}" --replicas=1 >/dev/null 2>&1
}

# Patch console plugin deployment to use internal registry image
patch_console_image() {
    oc set image "deployment/${DEPLOY_CONSOLE}" -n "${NS_CONSOLE}" \
        "*=${INTERNAL_REG}/${NS_CONSOLE}/lightspeed-console-plugin:${TAG}" >/dev/null 2>&1
}

# Print the running image digest for a deployment
show_digest() {
    local deploy="$1" ns="$2" label="$3"
    local digest
    digest=$(oc get pods -n "${ns}" -l "app.kubernetes.io/name=${deploy}" \
        -o jsonpath='{.items[0].status.containerStatuses[0].imageID}' 2>/dev/null \
        || oc get deployment/"${deploy}" -n "${ns}" \
        -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null)
    info "${label}: $(echo "${digest}" | awk -F@ '{print $2}' | cut -c8-19)"
}

###############################################################################
# Full deploy helpers — shared by deploy-agentic.sh
###############################################################################

ensure_registry_route() {
    step "Ensuring internal registry route"
    if oc get route default-route -n openshift-image-registry >/dev/null 2>&1; then
        info "Registry route already exposed"
    else
        oc patch configs.imageregistry.operator.openshift.io/cluster \
            --type merge -p '{"spec":{"defaultRoute":true}}' >/dev/null 2>&1
        for i in {1..30}; do
            oc get route default-route -n openshift-image-registry >/dev/null 2>&1 && break
            sleep 2
        done
        oc get route default-route -n openshift-image-registry >/dev/null 2>&1 \
            || fail "Registry route not available after 60s"
        info "Registry route exposed"
    fi
    get_registry
}

install_crds() {
    step "Installing CRDs"
    cd "${OPERATOR_DIR}"
    make manifests kustomize >/dev/null 2>&1
    bin/kustomize build config/crd | oc apply -f - >/dev/null 2>&1
    if [[ -f config/crd/bases/agentic.openshift.io_proposals.yaml ]]; then
        oc apply -f config/crd/bases/agentic.openshift.io_proposals.yaml >/dev/null 2>&1
    fi
    info "CRDs installed"
    oc get crd olsconfigs.agentic.openshift.io --no-headers 2>/dev/null | awk '{print "    " $1}'
    oc get crd proposals.agentic.openshift.io --no-headers 2>/dev/null | awk '{print "    " $1}'
}

ensure_namespace() {
    step "Ensuring namespace"
    oc create namespace "${NS_OPERATOR}" --dry-run=client -o yaml | oc apply -f - >/dev/null 2>&1
    info "Namespace ${NS_OPERATOR} ready"
}

deploy_operator_manifests() {
    step "Deploying operator manifests"
    local PATCH_FILE="config/default/deployment-patch.yaml"
    cp "${PATCH_FILE}" "${PATCH_FILE}.bak"

    local OPERATOR_IMG="${INTERNAL_REG}/${NS_OPERATOR}/lightspeed-operator:${TAG}"
    local CONSOLE_IMG="${INTERNAL_REG}/${NS_OPERATOR}/lightspeed-console-plugin:${TAG}"
    sed -i '' "s|__REPLACE_LIGHTSPEED_OPERATOR__|${OPERATOR_IMG}|g" "${PATCH_FILE}" 2>/dev/null \
        || sed -i "s|__REPLACE_LIGHTSPEED_OPERATOR__|${OPERATOR_IMG}|g" "${PATCH_FILE}"
    sed -i '' "s|__REPLACE_LIGHTSPEED_CONSOLE_PLUGIN__|${CONSOLE_IMG}|g" "${PATCH_FILE}" 2>/dev/null \
        || sed -i "s|__REPLACE_LIGHTSPEED_CONSOLE_PLUGIN__|${CONSOLE_IMG}|g" "${PATCH_FILE}"

    if [[ -f related_images.json ]] && [[ -f hack/image_placeholders.json ]]; then
        jq -r '.[] | "\(.name)|\(.placeholder)"' hack/image_placeholders.json | while IFS='|' read -r name placeholder; do
            if [[ "${name}" != "lightspeed-operator" ]]; then
                local img
                img=$(jq -r --arg n "${name}" '.[] | select(.name==$n) | .image' related_images.json)
                if [[ -n "${img}" ]] && [[ "${img}" != "null" ]]; then
                    sed -i '' "s|${placeholder}|${img}|g" "${PATCH_FILE}" 2>/dev/null \
                        || sed -i "s|${placeholder}|${img}|g" "${PATCH_FILE}"
                fi
            fi
        done
    fi

    bin/kustomize build config/default | oc apply -f - >/dev/null 2>&1
    cp "${PATCH_FILE}.bak" "${PATCH_FILE}"
    rm -f "${PATCH_FILE}.bak"
    info "Operator manifests applied"

    step "Ensuring operator cluster-admin"
    oc adm policy add-cluster-role-to-user cluster-admin \
        -z lightspeed-operator-controller-manager -n "${NS_OPERATOR}" >/dev/null 2>&1
    info "Operator cluster-admin bound"
}

build_push_operator() {
    build_image "operator" "${OPERATOR_DIR}" "${IMG_OPERATOR}"
    step "Pushing operator image"
    push_image "lightspeed-operator" "${NS_OPERATOR}" "${IMG_OPERATOR}"

    step "Waiting for operator rollout"
    oc rollout restart "deployment/${DEPLOY_OPERATOR}" -n "${NS_OPERATOR}" >/dev/null 2>&1
    oc rollout status "deployment/${DEPLOY_OPERATOR}" -n "${NS_OPERATOR}" --timeout=180s >/dev/null 2>&1
    info "Operator ready"
}

install_agent_sandbox_controller() {
    step "Installing agent-sandbox controller"
    if oc get crd sandboxes.agents.x-k8s.io >/dev/null 2>&1; then
        info "Agent-sandbox CRDs already installed"
    else
        kubectl apply -f https://github.com/kubernetes-sigs/agent-sandbox/releases/download/v0.2.1/manifest.yaml >/dev/null 2>&1
        kubectl apply -f https://github.com/kubernetes-sigs/agent-sandbox/releases/download/v0.2.1/extensions.yaml >/dev/null 2>&1
        oc rollout status deployment/agent-sandbox-controller -n agent-sandbox-system --timeout=120s >/dev/null 2>&1
        info "Agent-sandbox controller ready"
    fi
}

ensure_agent_rbac() {
    step "Ensuring agent RBAC"
    cat <<'RBACEOF' | oc apply -f - >/dev/null 2>&1
apiVersion: v1
kind: ServiceAccount
metadata:
  name: lightspeed-agent
  namespace: openshift-lightspeed
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: lightspeed-agent-reader
rules:
  - apiGroups: [""]
    resources: ["pods", "services", "configmaps", "namespaces", "events", "nodes", "pods/log"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["apps"]
    resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["batch"]
    resources: ["jobs", "cronjobs"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["networking.k8s.io"]
    resources: ["networkpolicies", "ingresses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["route.openshift.io"]
    resources: ["routes"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["monitoring.coreos.com"]
    resources: ["prometheuses", "prometheusrules", "servicemonitors", "alertmanagers"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: lightspeed-agent-reader
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: lightspeed-agent-reader
subjects:
  - kind: ServiceAccount
    name: lightspeed-agent
    namespace: openshift-lightspeed
RBACEOF
    info "Agent RBAC ready"
}

ensure_agent_service() {
    step "Ensuring agent service"
    if ! oc get svc lightspeed-agent -n "${NS_OPERATOR}" >/dev/null 2>&1; then
        cat <<'SVCEOF' | oc apply -f - >/dev/null 2>&1
apiVersion: v1
kind: Service
metadata:
  name: lightspeed-agent
  namespace: openshift-lightspeed
  annotations:
    service.beta.openshift.io/serving-cert-secret-name: lightspeed-agent-tls
spec:
  selector:
    agents.x-k8s.io/sandbox-name-hash: placeholder
  ports:
    - name: https
      port: 8080
      targetPort: 8080
      protocol: TCP
SVCEOF
        for i in {1..30}; do
            oc get secret lightspeed-agent-tls -n "${NS_OPERATOR}" >/dev/null 2>&1 && break
            sleep 2
        done
        info "Agent service created (TLS secret ready)"
    else
        info "Agent service already exists"
    fi
}

# Ensure a scoped GCP service account exists for Vertex AI and create a
# short-lived key. The SA is created automatically if it doesn't exist yet,
# with only roles/aiplatform.user — so even if the key leaks from a test
# cluster, the blast radius is limited to Vertex AI on one project.
#
# SA name is per-cluster (derived from the API server hostname) so multiple
# clusters sharing the same GCP project get separate SAs.
#
# The key is scheduled for automatic revocation after VERTEX_KEY_TTL seconds
# (default: 18 hours / 64800s).
VERTEX_KEY_TTL="${VERTEX_KEY_TTL:-64800}"

# Derive a cluster-scoped SA name: ls-<cluster-id> (max 30 chars for GCP).
_vertex_sa_name() {
    local cluster_id
    cluster_id=$(oc whoami --show-server 2>/dev/null \
        | sed 's|https://api\.||; s|\..*||') || true
    if [[ -z "${cluster_id}" ]]; then
        cluster_id="default"
    fi
    local name="ls-${cluster_id}"
    echo "${name:0:30}"
}

ensure_vertex_credentials() {
    local output_file="$1"
    local project="${VERTEX_PROJECT:?Set VERTEX_PROJECT to your GCP project ID}"
    local sa_name
    sa_name=$(_vertex_sa_name)
    local sa_email="${sa_name}@${project}.iam.gserviceaccount.com"
    local ttl="${VERTEX_KEY_TTL}"

    if ! command -v gcloud >/dev/null 2>&1; then
        fail "gcloud CLI not found — install it from https://cloud.google.com/sdk/docs/install"
    fi

    if ! gcloud iam service-accounts describe "${sa_email}" --project="${project}" >/dev/null 2>&1; then
        info "Creating service account ${sa_name} in project ${project}..."
        gcloud iam service-accounts create "${sa_name}" \
            --display-name="Lightspeed Vertex AI — ${sa_name}" \
            --project="${project}" --quiet 2>/dev/null \
            || fail "Failed to create service account ${sa_email}"

        gcloud projects add-iam-policy-binding "${project}" \
            --member="serviceAccount:${sa_email}" \
            --role="roles/aiplatform.user" \
            --condition=None --quiet >/dev/null 2>&1 \
            || fail "Failed to grant roles/aiplatform.user to ${sa_email}"

        info "Service account created with roles/aiplatform.user only"
    fi

    gcloud iam service-accounts keys create "${output_file}" \
        --iam-account="${sa_email}" --quiet 2>/dev/null \
        || fail "Failed to create SA key for ${sa_email}"

    local key_id
    key_id=$(python3 -c "import json,sys; print(json.load(open('${output_file}'))['private_key_id'])" 2>/dev/null)
    if [[ -z "${key_id}" ]]; then
        fail "Could not extract key ID from ${output_file}"
    fi

    local ttl_hours=$(( ttl / 3600 ))
    info "SA key created (id: ${key_id:0:12}…, SA: ${sa_email})"
    info "Key auto-revokes in ${ttl_hours}h (${ttl}s)"

    # Schedule key revocation in the background
    (
        sleep "${ttl}"
        gcloud iam service-accounts keys delete "${key_id}" \
            --iam-account="${sa_email}" --quiet 2>/dev/null \
            && echo "[lightspeed] Revoked SA key ${key_id:0:12}… after ${ttl_hours}h" \
            || echo "[lightspeed] Failed to revoke SA key ${key_id:0:12}… — delete manually: gcloud iam service-accounts keys delete ${key_id} --iam-account=${sa_email}"
    ) &
    disown

    return 0
}

# Clean up the cluster-scoped Vertex AI SA and all its keys.
cleanup_vertex_credentials() {
    local project="${VERTEX_PROJECT:?Set VERTEX_PROJECT to your GCP project ID}"
    local sa_name
    sa_name=$(_vertex_sa_name)
    local sa_email="${sa_name}@${project}.iam.gserviceaccount.com"

    if ! command -v gcloud >/dev/null 2>&1; then
        warn "gcloud CLI not found — cannot clean up SA"
        return 1
    fi

    if ! gcloud iam service-accounts describe "${sa_email}" --project="${project}" >/dev/null 2>&1; then
        info "No Vertex SA found for this cluster (${sa_name}) — nothing to clean up"
        return 0
    fi

    gcloud projects remove-iam-policy-binding "${project}" \
        --member="serviceAccount:${sa_email}" \
        --role="roles/aiplatform.user" \
        --quiet >/dev/null 2>&1 || true

    gcloud iam service-accounts delete "${sa_email}" \
        --project="${project}" --quiet 2>/dev/null \
        || { warn "Failed to delete SA ${sa_email}"; return 1; }

    info "Vertex SA deleted: ${sa_email}"
}

ensure_tool_secrets() {
    step "Ensuring GitHub token"
    local GH_TOKEN_VAL="${GH_TOKEN:-$(security find-generic-password -a "$USER" -s "GITHUB_TOKEN" -w 2>/dev/null || true)}"
    if ! oc get secret github-token -n "${NS_OPERATOR}" >/dev/null 2>&1; then
        if [[ -n "${GH_TOKEN_VAL}" ]]; then
            oc create secret generic github-token -n "${NS_OPERATOR}" \
                --from-literal=token="${GH_TOKEN_VAL}" >/dev/null 2>&1
            info "GitHub token created"
        else
            warn "No GH_TOKEN env var or macOS Keychain entry — create github-token secret manually"
        fi
    else
        info "GitHub token already exists"
    fi

    step "Ensuring Red Hat API token"
    local RH_TOKEN_VAL="${RH_API_OFFLINE_TOKEN:-$(security find-generic-password -a "$USER" -s "RH_API_OFFLINE_TOKEN" -w 2>/dev/null || true)}"
    if ! oc get secret redhat-api-token -n "${NS_OPERATOR}" >/dev/null 2>&1; then
        if [[ -n "${RH_TOKEN_VAL}" ]]; then
            oc create secret generic redhat-api-token -n "${NS_OPERATOR}" \
                --from-literal=token="${RH_TOKEN_VAL}" >/dev/null 2>&1
            info "Red Hat API token created"
        else
            warn "No RH_API_OFFLINE_TOKEN env var or macOS Keychain entry — create redhat-api-token secret manually"
        fi
    else
        info "Red Hat API token already exists"
    fi

    step "Ensuring ACS API token"
    if ! oc get secret acs-api-token -n "${NS_OPERATOR}" >/dev/null 2>&1; then
        local ACS_ROUTE
        ACS_ROUTE=$(oc get route central -n stackrox -o jsonpath='{.spec.host}' 2>/dev/null || true)
        if [[ -n "${ACS_ROUTE}" ]]; then
            local ACS_PASS ACS_TOKEN
            ACS_PASS=$(oc get secret central-htpasswd -n stackrox -o jsonpath='{.data.password}' | base64 -d)
            ACS_TOKEN=$(curl -sk -u "admin:${ACS_PASS}" "https://${ACS_ROUTE}/v1/apitokens/generate" \
                -X POST -H "Content-Type: application/json" \
                -d '{"name": "lightspeed-agent", "roles": ["Admin"]}' | python3 -c "import json,sys; print(json.load(sys.stdin).get('token',''))" 2>/dev/null)
            if [[ -n "${ACS_TOKEN}" ]]; then
                oc create secret generic acs-api-token -n "${NS_OPERATOR}" \
                    --from-literal=token="${ACS_TOKEN}" \
                    --from-literal=url="https://${ACS_ROUTE}" >/dev/null 2>&1
                info "ACS API token created (from Central at ${ACS_ROUTE})"
            else
                warn "ACS Central found but token generation failed"
            fi
        else
            info "No ACS Central found on cluster — skipping ACS token"
        fi
    else
        info "ACS API token already exists"
    fi
}

build_push_agent_and_skills() {
    local AGENT_DIR="${AGENT_DIR:-${WORKSPACE_ROOT}/lightspeed-agentic-sandbox}"
    if [[ -d "${AGENT_DIR}" ]]; then
        build_image "agent" "${AGENT_DIR}" "${IMG_AGENT}"
        step "Pushing agent image"
        push_image "lightspeed-agentic-sandbox" "${NS_OPERATOR}" "${IMG_AGENT}"
    else
        warn "Agent directory not found: ${AGENT_DIR}"
    fi

    if [[ -d "${SKILLS_DIR}" ]]; then
        build_image "skills" "${SKILLS_DIR}" "${IMG_SKILLS}" "Containerfile"
        step "Pushing skills image"
        push_image "lightspeed-skills" "${NS_OPERATOR}" "${IMG_SKILLS}"

        for profile in "${SKILLS_PROFILES[@]}"; do
            local img_var="IMG_SKILLS_${profile^^}"
            local img_tag="${!img_var}"
            build_image "skills-${profile}" "${SKILLS_DIR}" "${img_tag}" "Containerfile.${profile}"
            step "Pushing skills-${profile} image"
            push_image "lightspeed-skills-${profile}" "${NS_OPERATOR}" "${img_tag}"
        done
    else
        warn "Skills directory not found: ${SKILLS_DIR}"
    fi
}

# Apply a SandboxTemplate from stdin, create claim, wait for pod, update service selector
deploy_sandbox() {
    local LLM_SECRET="$1"
    step "Deploying agent sandbox"
    cat | oc apply -f - >/dev/null 2>&1
    info "SandboxTemplate created"

    if ! oc get sandboxclaim lightspeed-chat -n "${NS_OPERATOR}" >/dev/null 2>&1; then
        cat <<'CLAIMEOF' | oc apply -f - >/dev/null 2>&1
apiVersion: extensions.agents.x-k8s.io/v1alpha1
kind: SandboxClaim
metadata:
  name: lightspeed-chat
  namespace: openshift-lightspeed
spec:
  sandboxTemplateRef:
    name: lightspeed-chat
CLAIMEOF
        info "SandboxClaim created"
    else
        info "SandboxClaim already exists"
    fi

    step "Waiting for agent pod"
    for i in {1..60}; do
        local READY
        READY=$(oc get pod lightspeed-chat -n "${NS_OPERATOR}" \
            -o jsonpath='{.status.containerStatuses[0].ready}' 2>/dev/null)
        [[ "${READY}" == "true" ]] && break
        sleep 3
    done
    local POD_HASH
    POD_HASH=$(oc get pod lightspeed-chat -n "${NS_OPERATOR}" \
        -o jsonpath='{.metadata.labels.agents\.x-k8s\.io/sandbox-name-hash}' 2>/dev/null)
    if [[ -n "${POD_HASH:-}" ]]; then
        oc patch svc lightspeed-agent -n "${NS_OPERATOR}" --type merge \
            -p "{\"spec\":{\"selector\":{\"agents.x-k8s.io/sandbox-name-hash\":\"${POD_HASH}\"}}}" >/dev/null 2>&1
        info "Agent pod running, service selector updated (hash: ${POD_HASH})"
    else
        warn "Agent pod not ready yet — update service selector manually"
    fi
}

ensure_olsconfig() {
    step "Ensuring OLSConfig"
    if ! oc get olsconfig cluster >/dev/null 2>&1; then
        cat <<'OLSEOF' | oc apply -f - >/dev/null 2>&1
apiVersion: agentic.openshift.io/v1alpha1
kind: OLSConfig
metadata:
  name: cluster
spec:
  sandbox:
    baseTemplate: lightspeed-chat
OLSEOF
        info "OLSConfig created"
        echo "    Waiting for console plugin deployment..."
        for i in {1..30}; do
            oc get "deployment/${DEPLOY_CONSOLE}" -n "${NS_CONSOLE}" >/dev/null 2>&1 && break
            sleep 3
        done
    else
        info "OLSConfig already exists"
    fi
}

build_push_console() {
    if [[ -d "${CONSOLE_DIR}" ]]; then
        build_image "console-plugin" "${CONSOLE_DIR}" "${IMG_CONSOLE}"
        step "Pushing console plugin image"
        push_image "lightspeed-console-plugin" "${NS_CONSOLE}" "${IMG_CONSOLE}"

        if oc get "deployment/${DEPLOY_CONSOLE}" -n "${NS_CONSOLE}" >/dev/null 2>&1; then
            step "Deploying console plugin"
            pause_operator
            patch_console_image
            rollout "${DEPLOY_CONSOLE}" "${NS_CONSOLE}" "Console plugin"
            resume_operator
        else
            warn "Console plugin deployment not found — operator may still be reconciling"
        fi
    else
        warn "Console directory not found: ${CONSOLE_DIR}"
    fi
}

# Setup proposal API chain: agents, component tools, workflows, monitoring RBAC.
# Caller must create LLMProvider CRs before calling this.
setup_proposal_agents_and_workflows() {
    local SKILLS_IMG="${INTERNAL_REG}/${NS_OPERATOR}/lightspeed-skills:${TAG}"
    local SKILLS_REMEDIATE_IMG="${INTERNAL_REG}/${NS_OPERATOR}/lightspeed-skills-remediate:${TAG}"

    local AGENTIC_OPERATOR_DIR="${WORKSPACE_ROOT}/lightspeed-agentic-operator"

    # Apply example manifests from lightspeed-agentic-operator if available
    for f in "${AGENTIC_OPERATOR_DIR}/examples/setup/01-agents.yaml" \
             "${AGENTIC_OPERATOR_DIR}/examples/setup/02-component-tools.yaml" \
             "${AGENTIC_OPERATOR_DIR}/examples/setup/03-workflows.yaml"; do
        if [[ -f "${f}" ]]; then
            oc apply -f "${f}" >/dev/null 2>&1
            info "Applied $(basename "${f}")"
        fi
    done

    # Create Agent tiers (cluster-scoped)
    cat <<'AGENTEOF' | oc apply -f - >/dev/null 2>&1
apiVersion: agentic.openshift.io/v1alpha1
kind: Agent
metadata:
  name: default
spec:
  llmProvider:
    name: smart
  maxTurns: 200
---
apiVersion: agentic.openshift.io/v1alpha1
kind: Agent
metadata:
  name: smart
spec:
  llmProvider:
    name: smart
  maxTurns: 200
  providerSettings:
    reasoningEffort: "high"
---
apiVersion: agentic.openshift.io/v1alpha1
kind: Agent
metadata:
  name: fast
spec:
  llmProvider:
    name: fast
  maxTurns: 100
  providerSettings:
    reasoningEffort: "low"
AGENTEOF
    info "Agent CRs created (default, smart, fast)"

    # Create ComponentTools (namespace-scoped)
    cat <<CTEOF | oc apply -f - >/dev/null 2>&1
apiVersion: agentic.openshift.io/v1alpha1
kind: ComponentTools
metadata:
  name: cluster-ops-analysis
  namespace: ${NS_OPERATOR}
spec:
  skills:
    - image: ${SKILLS_IMG}
  systemPrompt: |
    You are an expert SRE agent for OpenShift clusters. Investigate the issue,
    gather evidence from metrics/logs/events, identify the root cause, and
    propose remediation options.
---
apiVersion: agentic.openshift.io/v1alpha1
kind: ComponentTools
metadata:
  name: cluster-ops-execution
  namespace: ${NS_OPERATOR}
spec:
  skills:
    - image: ${SKILLS_REMEDIATE_IMG}
  systemPrompt: |
    You are an expert SRE agent. Execute the approved remediation plan.
    Verify each action took effect. Do NOT perform changes beyond what was approved.
---
apiVersion: agentic.openshift.io/v1alpha1
kind: ComponentTools
metadata:
  name: cluster-ops-verification
  namespace: ${NS_OPERATOR}
spec:
  skills:
    - image: ${SKILLS_IMG}
  systemPrompt: |
    You are an independent verification agent. Verify the remediation was
    successful by checking actual cluster state. Do NOT trust execution results.
CTEOF
    info "ComponentTools CRs created (cluster-ops-analysis, execution, verification)"

    # Create Workflows (namespace-scoped)
    cat <<WFEOF | oc apply -f - >/dev/null 2>&1
apiVersion: agentic.openshift.io/v1alpha1
kind: Workflow
metadata:
  name: remediation
  namespace: ${NS_OPERATOR}
spec:
  analysis:
    agent: smart
    componentTools:
      name: cluster-ops-analysis
  execution:
    agent: fast
    componentTools:
      name: cluster-ops-execution
  verification:
    agent: smart
    componentTools:
      name: cluster-ops-verification
---
apiVersion: agentic.openshift.io/v1alpha1
kind: Workflow
metadata:
  name: gitops-remediation
  namespace: ${NS_OPERATOR}
spec:
  analysis:
    agent: smart
    componentTools:
      name: cluster-ops-analysis
  verification:
    agent: smart
    componentTools:
      name: cluster-ops-verification
---
apiVersion: agentic.openshift.io/v1alpha1
kind: Workflow
metadata:
  name: advisory-only
  namespace: ${NS_OPERATOR}
spec:
  analysis:
    agent: smart
    componentTools:
      name: cluster-ops-analysis
WFEOF
    info "Workflow CRs created (remediation, gitops-remediation, advisory-only)"

    if ! oc get clusterrolebinding lightspeed-agent-monitoring >/dev/null 2>&1; then
        oc adm policy add-cluster-role-to-user cluster-monitoring-view \
            -z lightspeed-agent -n "${NS_OPERATOR}" >/dev/null 2>&1
        info "Agent monitoring RBAC granted (cluster-monitoring-view)"
    else
        info "Agent monitoring RBAC already exists"
    fi
}

deploy_test_fixtures() {
    step "Deploying test fixtures"
    oc create namespace demo --dry-run=client -o yaml | oc apply -f - >/dev/null 2>&1
    cat <<'DEMOEOF' | oc apply -f - >/dev/null 2>&1
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo-app
  namespace: demo
  labels:
    app: demo-app
spec:
  replicas: 1
  selector:
    matchLabels:
      app: demo-app
  template:
    metadata:
      labels:
        app: demo-app
    spec:
      containers:
      - name: app
        image: registry.access.redhat.com/ubi9-minimal:latest
        command: ["sh", "-c", "echo Starting demo-app... && sleep 2 && exit 1"]
DEMOEOF
    info "Demo app deployed (crash-looping)"

    cat <<'RULEEOF' | oc apply -f - >/dev/null 2>&1
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: lightspeed-test-crashloop
  namespace: openshift-monitoring
spec:
  groups:
  - name: lightspeed-test
    rules:
    - alert: KubePodCrashLooping
      annotations:
        description: "Pod {{ $labels.namespace }}/{{ $labels.pod }} is crash looping"
        summary: "Pod is crash looping"
      expr: |
        max_over_time(kube_pod_container_status_waiting_reason{reason="CrashLoopBackOff"}[5m]) >= 1
      for: 1m
      labels:
        severity: warning
RULEEOF
    info "Test PrometheusRule created (KubePodCrashLooping, 1m 'for' duration)"
}

verify_deploy() {
    step "Verifying"
    oc get pods -n "${NS_OPERATOR}" --no-headers 2>/dev/null
}
