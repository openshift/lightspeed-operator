#!/usr/bin/env bash
# Shared helpers for agentic deploy/redeploy scripts.
# Source this file; do not execute directly.

set -euo pipefail

RED='\033[0;31m' GREEN='\033[0;32m' CYAN='\033[0;36m' YELLOW='\033[0;33m' NC='\033[0m'
step()  { echo -e "\n${CYAN}==> $1${NC}"; }
info()  { echo -e "    ${GREEN}✓${NC} $1"; }
warn()  { echo -e "    ${YELLOW}!${NC} $1"; }
fail()  { echo -e "    ${RED}✗${NC} $1"; exit 1; }

# Paths — this file lives in lightspeed-operator/hack/agentic/
# Sibling repos are next to the operator repo in the workspace.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[1]}")" && pwd)"
OPERATOR_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
WORKSPACE_ROOT="$(dirname "${OPERATOR_DIR}")"

CONSOLE_DIR="${CONSOLE_DIR:-${WORKSPACE_ROOT}/lightspeed-agentic-console}"
SKILLS_DIR="${SKILLS_DIR:-${WORKSPACE_ROOT}/lightspeed-skills}"
AGENT_DIR="${AGENT_DIR:-${WORKSPACE_ROOT}/lightspeed-agentic-sandbox}"
AGENTIC_OPERATOR_DIR="${AGENTIC_OPERATOR_DIR:-${WORKSPACE_ROOT}/lightspeed-agentic-operator}"

# Namespaces
NS_OPERATOR="openshift-lightspeed"
NS_CONSOLE="openshift-lightspeed"

# Deployment names (match operator constants.go)
DEPLOY_OPERATOR="lightspeed-operator-controller-manager"
DEPLOY_CONSOLE="lightspeed-agentic-console-plugin"

# Image tag — unique per worktree so concurrent deploys don't clobber each other.
# .worktrees/<name>/ → "wt-<name>", main repo → "latest".
if [[ "${WORKSPACE_ROOT}" == */.worktrees/* ]]; then
    TAG="wt-$(basename "${WORKSPACE_ROOT}")"
else
    TAG="latest"
fi

# BuildConfig names — match ImageStream names in ensure_buildconfigs()
BC_OPERATOR="lightspeed-operator"
BC_CONSOLE="lightspeed-console-plugin"
BC_AGENT="lightspeed-agentic-sandbox"
BC_SKILLS="lightspeed-skills"

# Internal registry endpoint (for image references inside the cluster)
INTERNAL_REG="image-registry.openshift-image-registry.svc:5000"

show_usage() { echo "Usage: KUBECONFIG=<path> bash $0 [--skip-build]"; }

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

# Ensure ImageStreams and BuildConfigs exist for all components.
# BuildConfigs use Binary source + Docker strategy — builds run on the
# cluster natively (no local container engine or cross-compilation needed).
# Idempotent (oc apply), safe to call from every script.
ensure_buildconfigs() {
    step "Ensuring BuildConfigs and ImageStreams"
    cat <<EOF | oc apply -f - >/dev/null 2>&1
apiVersion: image.openshift.io/v1
kind: ImageStream
metadata:
  name: lightspeed-operator
  namespace: ${NS_OPERATOR}
---
apiVersion: image.openshift.io/v1
kind: ImageStream
metadata:
  name: lightspeed-console-plugin
  namespace: ${NS_OPERATOR}
---
apiVersion: image.openshift.io/v1
kind: ImageStream
metadata:
  name: lightspeed-agentic-sandbox
  namespace: ${NS_OPERATOR}
---
apiVersion: image.openshift.io/v1
kind: ImageStream
metadata:
  name: lightspeed-skills
  namespace: ${NS_OPERATOR}
---
apiVersion: build.openshift.io/v1
kind: BuildConfig
metadata:
  name: lightspeed-operator
  namespace: ${NS_OPERATOR}
spec:
  output:
    to:
      kind: ImageStreamTag
      name: "lightspeed-operator:${TAG}"
  source:
    type: Binary
  strategy:
    type: Docker
    dockerStrategy:
      dockerfilePath: lightspeed-operator/Dockerfile.dev
  resources:
    requests:
      cpu: "1"
      memory: 4Gi
    limits:
      memory: 8Gi
---
apiVersion: build.openshift.io/v1
kind: BuildConfig
metadata:
  name: lightspeed-console-plugin
  namespace: ${NS_OPERATOR}
spec:
  output:
    to:
      kind: ImageStreamTag
      name: "lightspeed-console-plugin:${TAG}"
  source:
    type: Binary
  strategy:
    type: Docker
    dockerStrategy:
      dockerfilePath: Dockerfile
  resources:
    requests:
      cpu: "1"
      memory: 2Gi
    limits:
      memory: 4Gi
---
apiVersion: build.openshift.io/v1
kind: BuildConfig
metadata:
  name: lightspeed-agentic-sandbox
  namespace: ${NS_OPERATOR}
spec:
  output:
    to:
      kind: ImageStreamTag
      name: "lightspeed-agentic-sandbox:${TAG}"
  source:
    type: Binary
  strategy:
    type: Docker
    dockerStrategy:
      dockerfilePath: Containerfile.dev
  resources:
    requests:
      cpu: "1"
      memory: 4Gi
    limits:
      memory: 8Gi
---
apiVersion: build.openshift.io/v1
kind: BuildConfig
metadata:
  name: lightspeed-skills
  namespace: ${NS_OPERATOR}
spec:
  output:
    to:
      kind: ImageStreamTag
      name: "lightspeed-skills:${TAG}"
  source:
    type: Binary
  strategy:
    type: Docker
    dockerStrategy:
      dockerfilePath: Containerfile
EOF
    info "BuildConfigs and ImageStreams ready"
}

# Build an image on the cluster via binary build.
# Uploads the source directory to a builder pod, which runs the Dockerfile
# natively on amd64 and pushes to the internal registry — no local container
# engine, cross-compilation, registry route, or auth tokens needed.
build_on_cluster() {
    local bc_name="$1" from_dir="$2" label="$3"
    if [[ "${SKIP_BUILD}" == "true" ]]; then
        warn "Skipping build of ${label}"
        return 0
    fi
    oc patch "bc/${bc_name}" -n "${NS_OPERATOR}" --type=merge \
        -p "{\"spec\":{\"output\":{\"to\":{\"name\":\"${bc_name}:${TAG}\"}}}}" >/dev/null 2>&1
    echo "    Building ${label} on cluster (uploading source)..."
    oc start-build "${bc_name}" -n "${NS_OPERATOR}" \
        --from-dir="${from_dir}" --follow \
        || fail "Failed to build ${label}"
    if [[ "${TAG}" != "latest" ]]; then
        oc tag "${NS_OPERATOR}/${bc_name}:${TAG}" "${NS_OPERATOR}/${bc_name}:latest" >/dev/null 2>&1
    fi
    info "${label} built"
}

# Parallel build support — start builds without blocking, then wait for all.
PENDING_BUILDS=()
PENDING_LABELS=()
PENDING_BCS=()

start_build_async() {
    local bc_name="$1" from_dir="$2" label="$3"
    if [[ "${SKIP_BUILD}" == "true" ]]; then
        warn "Skipping build of ${label}"
        return 0
    fi
    oc patch "bc/${bc_name}" -n "${NS_OPERATOR}" --type=merge \
        -p "{\"spec\":{\"output\":{\"to\":{\"name\":\"${bc_name}:${TAG}\"}}}}" >/dev/null 2>&1
    echo "    Starting ${label} build (uploading source)..."
    local build_output build_name
    build_output=$(oc start-build "${bc_name}" -n "${NS_OPERATOR}" \
        --from-dir="${from_dir}" -o name 2>/dev/null) \
        || fail "Failed to start build for ${label}"
    build_name=$(echo "${build_output}" | tail -1)
    build_name="${build_name#build.build.openshift.io/}"
    PENDING_BUILDS+=("${build_name}")
    PENDING_LABELS+=("${label}")
    PENDING_BCS+=("${bc_name}")
    info "${label} build started (${build_name})"
}

wait_all_builds() {
    if [[ ${#PENDING_BUILDS[@]} -eq 0 ]]; then return; fi
    step "Waiting for ${#PENDING_BUILDS[@]} parallel builds"
    local build_args=()
    for b in "${PENDING_BUILDS[@]}"; do build_args+=("build/${b}"); done

    local all_done=false
    while [[ "${all_done}" != "true" ]]; do
        all_done=true
        local status_line=""
        local phases
        phases=$(oc get "${build_args[@]}" -n "${NS_OPERATOR}" \
            -o custom-columns='NAME:.metadata.name,PHASE:.status.phase' --no-headers 2>/dev/null)
        for i in "${!PENDING_BUILDS[@]}"; do
            local build="${PENDING_BUILDS[$i]}"
            local label="${PENDING_LABELS[$i]}"
            local phase
            phase=$(echo "${phases}" | awk -v b="${build}" '$1==b {print $2}')
            phase="${phase:-Unknown}"
            status_line+="  ${label}=${phase}"
            if [[ "${phase}" == "Failed" ]] || [[ "${phase}" == "Error" ]] || [[ "${phase}" == "Cancelled" ]]; then
                echo ""
                echo "    Build ${build} failed (${phase}):"
                oc logs "build/${build}" -n "${NS_OPERATOR}" --tail=30 2>/dev/null
                fail "Build failed: ${label}"
            fi
            if [[ "${phase}" != "Complete" ]]; then
                all_done=false
            fi
        done
        if [[ "${all_done}" != "true" ]]; then
            printf "\r    %s" "${status_line}"
            sleep 15
        fi
    done
    echo ""
    for i in "${!PENDING_BUILDS[@]}"; do
        local bc_name="${PENDING_BCS[$i]}"
        local label="${PENDING_LABELS[$i]}"
        if [[ "${TAG}" != "latest" ]]; then
            oc tag "${NS_OPERATOR}/${bc_name}:${TAG}" "${NS_OPERATOR}/${bc_name}:latest" >/dev/null 2>&1
        fi
        info "${label} built"
    done
    PENDING_BUILDS=()
    PENDING_LABELS=()
    PENDING_BCS=()
}

# Rollout restart a deployment and wait for it
rollout() {
    local deploy="$1" ns="$2" label="$3"
    echo "    Restarting ${label}..."
    if ! oc rollout restart "deployment/${deploy}" -n "${ns}" 2>&1; then
        warn "rollout restart failed for ${deploy} — may already be in progress"
    fi
    echo "    Waiting for ${label} rollout..."
    if ! oc rollout status "deployment/${deploy}" -n "${ns}" --timeout=180s 2>&1; then
        warn "${label} rollout did not complete within 180s — checking pod status"
        oc get pods -n "${ns}" -l "app=${deploy}" --no-headers 2>/dev/null
        fail "${label} rollout failed"
    fi
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
# Full deploy helpers — shared by deploy.sh
###############################################################################

update_crds_and_rbac() {
    step "Updating CRDs and RBAC"
    cd "${OPERATOR_DIR}"
    make manifests kustomize >/dev/null 2>&1
    oc apply -f config/crd/bases/ >/dev/null 2>&1
    bin/kustomize build config/default \
        | oc apply -f - -l app.kubernetes.io/component=rbac --server-side --force-conflicts >/dev/null 2>&1 \
        || warn "RBAC update via kustomize failed — may need full deploy"
    info "CRDs and RBAC updated"
}

install_crds() {
    step "Installing CRDs"
    cd "${OPERATOR_DIR}"
    make manifests kustomize >/dev/null 2>&1
    bin/kustomize build config/crd | oc apply -f - >/dev/null 2>&1
    info "CRDs installed"
    oc get crd olsconfigs.ols.openshift.io --no-headers 2>/dev/null | awk '{print "    " $1}' || true
    oc get crd proposals.agentic.openshift.io --no-headers 2>/dev/null | awk '{print "    " $1}' || true
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

    bin/kustomize build config/default \
        | awk -v console_img="${CONSOLE_IMG}" '/- --leader-elect/{print; print "        - --enable-agentic"; print "        - --agentic-console-image=" console_img; next}1' \
        | oc apply -f - >/dev/null 2>&1
    cp "${PATCH_FILE}.bak" "${PATCH_FILE}"
    rm -f "${PATCH_FILE}.bak"
    info "Operator manifests applied (agentic enabled)"

    step "Ensuring operator cluster-admin"
    oc adm policy add-cluster-role-to-user cluster-admin \
        -z lightspeed-operator-controller-manager -n "${NS_OPERATOR}" >/dev/null 2>&1
    info "Operator cluster-admin bound"
}

_make_operator_context() {
    local ctx
    ctx=$(mktemp -d /tmp/lightspeed-operator-ctx-XXXXXX)
    cp -rL "${OPERATOR_DIR}" "${ctx}/lightspeed-operator"
    cp -rL "${AGENTIC_OPERATOR_DIR}" "${ctx}/lightspeed-agentic-operator"
    echo "${ctx}"
}

build_operator() {
    local ctx
    ctx=$(_make_operator_context)
    build_on_cluster "${BC_OPERATOR}" "${ctx}" "operator"
    rm -rf "${ctx}"
}

start_operator_build_async() {
    local ctx
    ctx=$(_make_operator_context)
    start_build_async "${BC_OPERATOR}" "${ctx}" "operator"
    rm -rf "${ctx}"
}

build_push_operator() {
    build_operator
    rollout "${DEPLOY_OPERATOR}" "${NS_OPERATOR}" "Operator"
}

install_agent_sandbox_controller() {
    step "Installing agent-sandbox controller"
    if oc get crd sandboxes.agents.x-k8s.io >/dev/null 2>&1; then
        info "Agent-sandbox CRDs already installed"
    else
        kubectl apply -f https://github.com/kubernetes-sigs/agent-sandbox/releases/download/v0.4.2/manifest.yaml >/dev/null 2>&1
        kubectl apply -f https://github.com/kubernetes-sigs/agent-sandbox/releases/download/v0.4.2/extensions.yaml >/dev/null 2>&1
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
  - apiGroups: ["agentic.openshift.io"]
    resources: ["proposals", "analysisresults", "executionresults", "verificationresults", "escalationresults"]
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
        oc wait --for=create secret/lightspeed-agent-tls -n "${NS_OPERATOR}" --timeout=60s >/dev/null 2>&1
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

    # gcloud may exit non-zero due to org policy warnings (e.g. key expiry
    # constraints) even when the key is created successfully. Check the output
    # file instead of trusting the exit code.
    rm -f "${output_file}"
    gcloud iam service-accounts keys create "${output_file}" \
        --iam-account="${sa_email}" --quiet 2>/dev/null || true

    local key_id
    key_id=$(python3 -c "import json,sys; print(json.load(open('${output_file}'))['private_key_id'])" 2>/dev/null)
    if [[ -z "${key_id}" ]]; then
        fail "Failed to create SA key for ${sa_email} (key file empty or missing)"
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

# Setup the Day 0 proposal API chain, following the timeline from the CRD
# design doc (see gist: harche/ac8e8399a9bf69091a38a5cf6e3bc56b).
#
# Timeline — Who creates what, when:
#   Day 0 (Cluster Admin):
#     1. LLMProvider CRs        — LLM backend config (created by caller, provider-specific)
#     2. Agent CRs              — Agent tiers: default, smart, fast (cluster-scoped, model on Agent)
#     3. ApprovalPolicy CR      — Per-stage approval defaults (cluster-scoped singleton)
#     4. Runtime secrets        — Tool credentials in component namespaces
#   Day 1 (Component Owner):
#     5. Proposal CRs           — Created by adapters at runtime (namespaced)
#
# This function handles steps 2-4. Step 1 (LLMProvider) is done by the caller.
# Step 5 (Proposals) happens at runtime via adapters or manual creation.
setup_proposal_agents_and_workflows() {
    local AGENTIC_OPERATOR_DIR="${WORKSPACE_ROOT}/lightspeed-agentic-operator"

    ###########################################################################
    # Step 2: Agent tiers (cluster-scoped) — Cluster Admin
    # Each agent references an LLMProvider and specifies a model.
    # Different agents = different cost/reasoning profiles. The "default" agent is required.
    ###########################################################################
    if [[ -f "${AGENTIC_OPERATOR_DIR}/examples/setup/01-agents.yaml" ]]; then
        oc apply -f "${AGENTIC_OPERATOR_DIR}/examples/setup/01-agents.yaml" >/dev/null 2>&1
        info "Agent CRs applied from examples/setup/01-agents.yaml"
    else
        cat <<'AGENTEOF' | oc apply -f - >/dev/null 2>&1
apiVersion: agentic.openshift.io/v1alpha1
kind: Agent
metadata:
  name: default
spec:
  llmProvider:
    name: vertex-ai
  model: claude-opus-4-6
  maxTurns: 200
---
apiVersion: agentic.openshift.io/v1alpha1
kind: Agent
metadata:
  name: smart
spec:
  llmProvider:
    name: vertex-ai
  model: claude-opus-4-6
  maxTurns: 200
---
apiVersion: agentic.openshift.io/v1alpha1
kind: Agent
metadata:
  name: fast
spec:
  llmProvider:
    name: vertex-ai
  model: claude-haiku-4-5
  maxTurns: 100
AGENTEOF
        info "Agent CRs created (default, smart, fast)"
    fi

    ###########################################################################
    # Step 3: ApprovalPolicy — Cluster Admin
    # Auto-approve analysis and verification; require manual execution approval.
    ###########################################################################
    if [[ -f "${AGENTIC_OPERATOR_DIR}/examples/setup/02-approval-policy.yaml" ]]; then
        oc apply -f "${AGENTIC_OPERATOR_DIR}/examples/setup/02-approval-policy.yaml" >/dev/null 2>&1
        info "ApprovalPolicy applied from examples/setup/02-approval-policy.yaml"
    else
        cat <<'POLICYEOF' | oc apply -f - >/dev/null 2>&1
apiVersion: agentic.openshift.io/v1alpha1
kind: ApprovalPolicy
metadata:
  name: cluster
spec:
  stages:
    - name: Analysis
      approval: Manual
    - name: Execution
      approval: Manual
    - name: Verification
      approval: Manual
POLICYEOF
        info "ApprovalPolicy created (all stages manual)"
    fi

    ###########################################################################
    # Step 4: Monitoring RBAC — Cluster Admin
    # Grant the agent SA read access to cluster monitoring (Thanos/Prometheus)
    # so analysis agents can query metrics.
    ###########################################################################
    if ! oc get clusterrolebinding lightspeed-agent-monitoring >/dev/null 2>&1; then
        oc adm policy add-cluster-role-to-user cluster-monitoring-view \
            -z lightspeed-agent -n "${NS_OPERATOR}" >/dev/null 2>&1
        info "Agent monitoring RBAC granted (cluster-monitoring-view)"
    else
        info "Agent monitoring RBAC already exists"
    fi
}

deploy_test_fixtures() {
    step "Deploying JVM OOMKill demo"
    oc create namespace lightspeed-demo --dry-run=client -o yaml | oc apply -f - >/dev/null 2>&1

    # JVM OOMKill demo — container memory limit (256Mi) is too low for the JVM
    # footprint (256MB Xms + metaspace + threads ≈ 450MB). The fix is to increase
    # the limit to 768Mi.
    cat <<'DEMOEOF' | oc apply -f - >/dev/null 2>&1
apiVersion: apps/v1
kind: Deployment
metadata:
  name: jvm-oomkill-demo
  namespace: lightspeed-demo
  labels:
    app: jvm-oomkill-demo
  annotations:
    description: "Demo app: JVM OOMKill due to container memory limit < JVM heap size"
spec:
  replicas: 1
  selector:
    matchLabels:
      app: jvm-oomkill-demo
  template:
    metadata:
      labels:
        app: jvm-oomkill-demo
    spec:
      containers:
      - name: jvm
        image: registry.access.redhat.com/ubi9/openjdk-21:latest
        command: ["/bin/sh", "-c"]
        args:
        - |
          echo 'public class App { public static void main(String[] a) throws Exception { System.out.println("Starting JVM service..."); byte[][] cache = new byte[200][]; for (int i = 0; i < 200; i++) { cache[i] = new byte[1024*1024]; } System.out.println("Cache loaded: 200MB. Service ready."); Thread.sleep(Long.MAX_VALUE); }}' > /tmp/App.java &&
          java -Xms256m -Xmx512m /tmp/App.java
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            memory: 256Mi
DEMOEOF
    info "JVM OOMKill demo deployed (crash-looping in lightspeed-demo)"

    cat <<'RULEEOF' | oc apply -f - >/dev/null 2>&1
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: lightspeed-demo-alerts
  namespace: openshift-lightspeed
spec:
  groups:
  - name: lightspeed-demo
    rules:
    - alert: KubePodCrashLooping
      annotations:
        description: >-
          Pod {{ $labels.namespace }}/{{ $labels.pod }} ({{ $labels.container }})
          is in a CrashLoopBackOff state and has restarted {{ $value }} times
          in the last 5 minutes.
        summary: Pod is crash-looping.
        runbook_url: https://runbooks.prometheus-operator.dev/runbooks/kubernetes/kubepodcrashlooping
      expr: |
        rate(kube_pod_container_status_restarts_total{namespace="lightspeed-demo"}[5m]) * 60 * 5 > 0
      for: 1m
      labels:
        severity: warning
RULEEOF
    info "PrometheusRule created (KubePodCrashLooping for lightspeed-demo, 1m)"

    cat <<PROPOSALEOF | oc apply -f - >/dev/null 2>&1
apiVersion: agentic.openshift.io/v1alpha1
kind: Proposal
metadata:
  name: alertmanager-jvm-oomkill
  namespace: ${NS_OPERATOR}
  labels:
    agentic.openshift.io/source: alertmanager
spec:
  request: |
    AlertManager alert fired: KubePodCrashLooping (warning)
    Pod jvm-oomkill-demo in namespace lightspeed-demo is in a CrashLoopBackOff
    state. The container is being OOMKilled — the JVM heap configuration exceeds
    the container memory limit.
    Labels: severity=warning, namespace=lightspeed-demo, pod=jvm-oomkill-demo, container=jvm
  targetNamespaces:
    - lightspeed-demo
  maxAttempts: 3
  tools:
    skills:
      - image: ${INTERNAL_REG}/${NS_OPERATOR}/lightspeed-skills:${TAG}
  analysis:
    agent: smart
  execution: {}
  verification:
    agent: fast
PROPOSALEOF
    info "Proposal created (alertmanager-jvm-oomkill)"
}

verify_deploy() {
    step "Verifying"
    oc get pods -n "${NS_OPERATOR}" --no-headers 2>/dev/null
}
