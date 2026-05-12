#!/usr/bin/env bash
# Shared helpers for agentic deploy/redeploy scripts.
# Source this file; do not execute directly.

set -euo pipefail

RED='\033[0;31m' GREEN='\033[0;32m' CYAN='\033[0;36m' YELLOW='\033[0;33m' NC='\033[0m'
step()  { echo -e "\n${CYAN}==> $1${NC}"; }
info()  { echo -e "    ${GREEN}✓${NC} $1"; }
warn()  { echo -e "    ${YELLOW}!${NC} $1"; }
fail()  { echo -e "    ${RED}✗${NC} $1"; exit 1; }

_run() {
    local _out
    _out=$(mktemp)
    if "$@" >"${_out}" 2>&1; then
        rm -f "${_out}"
    else
        local _rc=$?
        echo -e "    ${RED}✗${NC} Command failed: $*" >&2
        cat "${_out}" >&2
        rm -f "${_out}"
        return ${_rc}
    fi
}

# Paths — this file lives in lightspeed-operator/hack/agentic/
# Sibling repos are next to the operator repo in the workspace.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[1]}")" && pwd)"
OPERATOR_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
WORKSPACE_ROOT="$(dirname "${OPERATOR_DIR}")"

CONSOLE_DIR="${CONSOLE_DIR:-${WORKSPACE_ROOT}/lightspeed-agentic-console}"
SKILLS_DIR="${SKILLS_DIR:-${WORKSPACE_ROOT}/agentic-skills}"
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
BC_SKILLS="agentic-skills"

# Internal registry endpoint (for image references inside the cluster)
INTERNAL_REG="image-registry.openshift-image-registry.svc:5000"

# Centralized image references — used by deploy_operator_manifests and redeploy scripts.
# Override via env vars to use external images (e.g. Konflux) instead of on-cluster builds.
# When overridden, the corresponding build is automatically skipped.
#   AGENT_IMG=$(jq -r '.[] | select(.name=="lightspeed-agentic-sandbox") | .image' related_images.json) \
#   CONSOLE_IMG=$(jq -r '.[] | select(.name=="lightspeed-agentic-console") | .image' related_images.json) \
#   bash hack/agentic/deploy.sh --provider=vertex
_IMG_OVERRIDES=()
[[ -n "${OPERATOR_IMG:-}" ]] && _IMG_OVERRIDES+=("${BC_OPERATOR}")
[[ -n "${CONSOLE_IMG:-}" ]] && _IMG_OVERRIDES+=("${BC_CONSOLE}")
[[ -n "${AGENT_IMG:-}" ]] && _IMG_OVERRIDES+=("${BC_AGENT}")
[[ -n "${SKILLS_IMG:-}" ]] && _IMG_OVERRIDES+=("${BC_SKILLS}")
OPERATOR_IMG="${OPERATOR_IMG:-${INTERNAL_REG}/${NS_OPERATOR}/${BC_OPERATOR}:${TAG}}"
CONSOLE_IMG="${CONSOLE_IMG:-${INTERNAL_REG}/${NS_OPERATOR}/${BC_CONSOLE}:${TAG}}"
AGENT_IMG="${AGENT_IMG:-${INTERNAL_REG}/${NS_OPERATOR}/${BC_AGENT}:${TAG}}"
SKILLS_IMG="${SKILLS_IMG:-${INTERNAL_REG}/${NS_OPERATOR}/${BC_SKILLS}:${TAG}}"

_is_overridden() {
    local bc="$1"
    for o in "${_IMG_OVERRIDES[@]+"${_IMG_OVERRIDES[@]}"}"; do
        [[ "${o}" == "${bc}" ]] && return 0
    done
    return 1
}

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
    cat <<EOF | _run oc apply -f -
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
  name: agentic-skills
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
      dockerfilePath: Containerfile
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
  name: agentic-skills
  namespace: ${NS_OPERATOR}
spec:
  output:
    to:
      kind: ImageStreamTag
      name: "agentic-skills:${TAG}"
  source:
    type: Binary
  strategy:
    type: Docker
    dockerStrategy:
      dockerfilePath: Containerfile.dev
EOF
    info "BuildConfigs and ImageStreams ready"
}

# Unified build function — sync (--follow) or async (poll via wait_all_builds).
# Usage: _build sync <bc_name> <from_dir> <label>
#        _build async <bc_name> <from_dir> <label>
PENDING_BUILDS=()
PENDING_LABELS=()
PENDING_BCS=()

_build() {
    local mode="$1" bc_name="$2" from_dir="$3" label="$4"
    if [[ "${SKIP_BUILD}" == "true" ]]; then
        warn "Skipping build of ${label} (--skip-build)"
        return 0
    fi
    if _is_overridden "${bc_name}"; then
        info "Skipping build of ${label} (image overridden via env)"
        return 0
    fi
    _run oc patch "bc/${bc_name}" -n "${NS_OPERATOR}" --type=merge \
        -p "{\"spec\":{\"output\":{\"to\":{\"name\":\"${bc_name}:${TAG}\"}}}}"

    if [[ "${mode}" == "sync" ]]; then
        echo "    Building ${label} on cluster (uploading source)..."
        oc start-build "${bc_name}" -n "${NS_OPERATOR}" \
            --from-dir="${from_dir}" --follow --wait \
            || fail "Failed to build ${label}"
        if [[ "${TAG}" != "latest" ]]; then
            _run oc tag "${NS_OPERATOR}/${bc_name}:${TAG}" "${NS_OPERATOR}/${bc_name}:latest"
        fi
        info "${label} built"
    else
        echo "    Starting ${label} build (uploading source)..."
        local build_output build_name _stderr_file
        _stderr_file=$(mktemp)
        build_output=$(oc start-build "${bc_name}" -n "${NS_OPERATOR}" \
            --from-dir="${from_dir}" -o name 2>"${_stderr_file}") \
            || { echo -e "    ${RED}✗${NC} oc start-build stderr:" >&2; cat "${_stderr_file}" >&2; rm -f "${_stderr_file}"; fail "Failed to start build for ${label}"; }
        rm -f "${_stderr_file}"
        build_name=$(echo "${build_output}" | tail -1)
        build_name="${build_name#build.build.openshift.io/}"
        PENDING_BUILDS+=("${build_name}")
        PENDING_LABELS+=("${label}")
        PENDING_BCS+=("${bc_name}")
        info "${label} build started (${build_name})"
    fi
}

build_on_cluster() { _build sync "$@"; }
start_build_async() { _build async "$@"; }

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
            _run oc tag "${NS_OPERATOR}/${bc_name}:${TAG}" "${NS_OPERATOR}/${bc_name}:latest"
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
    _run oc set image "deployment/${DEPLOY_CONSOLE}" -n "${NS_CONSOLE}" \
        "*=${CONSOLE_IMG}"
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
    _run make manifests kustomize
    _run oc apply -f config/crd/bases/
    bin/kustomize build config/default \
        | oc apply -f - -l app.kubernetes.io/component=rbac --server-side --force-conflicts >/dev/null 2>&1 \
        || warn "RBAC update via kustomize failed — may need full deploy"
    info "CRDs and RBAC updated"
}

install_crds() {
    step "Installing CRDs"
    cd "${OPERATOR_DIR}"
    _run make manifests kustomize
    bin/kustomize build config/crd | _run oc apply -f -
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

    sed -i '' "s|__REPLACE_LIGHTSPEED_OPERATOR__|${OPERATOR_IMG}|g" "${PATCH_FILE}" 2>/dev/null \
        || sed -i "s|__REPLACE_LIGHTSPEED_OPERATOR__|${OPERATOR_IMG}|g" "${PATCH_FILE}"
    sed -i '' "s|__REPLACE_LIGHTSPEED_CONSOLE_PLUGIN__|${CONSOLE_IMG}|g" "${PATCH_FILE}" 2>/dev/null \
        || sed -i "s|__REPLACE_LIGHTSPEED_CONSOLE_PLUGIN__|${CONSOLE_IMG}|g" "${PATCH_FILE}"
    sed -i '' "s|__REPLACE_LIGHTSPEED_AGENTIC_CONSOLE__|${CONSOLE_IMG}|g" "${PATCH_FILE}" 2>/dev/null \
        || sed -i "s|__REPLACE_LIGHTSPEED_AGENTIC_CONSOLE__|${CONSOLE_IMG}|g" "${PATCH_FILE}"
    sed -i '' "s|__REPLACE_LIGHTSPEED_AGENTIC_SANDBOX__|${AGENT_IMG}|g" "${PATCH_FILE}" 2>/dev/null \
        || sed -i "s|__REPLACE_LIGHTSPEED_AGENTIC_SANDBOX__|${AGENT_IMG}|g" "${PATCH_FILE}"

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
        | oc apply -f - >/dev/null 2>&1
    cp "${PATCH_FILE}.bak" "${PATCH_FILE}"
    rm -f "${PATCH_FILE}.bak"
    info "Operator manifests applied"

    step "Ensuring operator cluster-admin"
    oc adm policy add-cluster-role-to-user cluster-admin \
        -z lightspeed-operator-controller-manager -n "${NS_OPERATOR}" >/dev/null 2>&1
    info "Operator cluster-admin bound"
}

ensure_agentic_feature_gate() {
    step "Enabling LightspeedAgents feature gate"
    if ! oc get olsconfig cluster >/dev/null 2>&1; then
        cat <<OLSEOF | _run oc apply -f -
apiVersion: ols.openshift.io/v1alpha1
kind: OLSConfig
metadata:
  name: cluster
spec:
  featureGates:
    - LightspeedAgents
  llm:
    providers:
      - name: placeholder
        type: openai
        credentialsSecretRef:
          name: placeholder
        models:
          - name: placeholder
  ols:
    defaultProvider: placeholder
    defaultModel: placeholder
OLSEOF
        info "OLSConfig created with LightspeedAgents feature gate"
    else
        local current_gates
        current_gates=$(oc get olsconfig cluster -o jsonpath='{.spec.featureGates}' 2>/dev/null || echo "")
        if echo "${current_gates}" | grep -q "LightspeedAgents"; then
            info "LightspeedAgents feature gate already enabled"
        else
            oc patch olsconfig cluster --type=merge \
                -p '{"spec":{"featureGates":["LightspeedAgents"]}}' 2>&1 \
                || fail "Could not enable LightspeedAgents feature gate"
            info "LightspeedAgents feature gate enabled"
        fi
    fi
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
    chmod -R u+w "${ctx}" 2>/dev/null; rm -rf "${ctx}"
}

start_operator_build_async() {
    local ctx
    ctx=$(_make_operator_context)
    start_build_async "${BC_OPERATOR}" "${ctx}" "operator"
    chmod -R u+w "${ctx}" 2>/dev/null; rm -rf "${ctx}"
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

# Locate GCP Application Default Credentials JSON for Vertex AI.
# Uses GOOGLE_APPLICATION_CREDENTIALS if set, otherwise the default gcloud path.
vertex_credentials_file() {
    local creds="${GOOGLE_APPLICATION_CREDENTIALS:-${HOME}/.config/gcloud/application_default_credentials.json}"
    if [[ ! -f "${creds}" ]]; then
        fail "No GCP credentials found at ${creds} — run 'gcloud auth application-default login' first"
    fi
    echo "${creds}"
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
#     3. ApprovalPolicy CR      — Approval policy and concurrency (cluster-scoped singleton)
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
    # Approval behavior and concurrency limits.
    ###########################################################################
    cat <<CONFIGEOF | oc apply -f - >/dev/null 2>&1
apiVersion: agentic.openshift.io/v1alpha1
kind: ApprovalPolicy
metadata:
  name: cluster
spec:
  maxAttempts: 3
  maxConcurrentProposals: 5
  stages:
    - name: Analysis
      approval: Manual
    - name: Execution
      approval: Manual
    - name: Verification
      approval: Manual
CONFIGEOF
    info "ApprovalPolicy applied"

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
  analysisOutput:
    schema:
      type: object
      description: JVM-specific structured data for console rendering
      properties:
        jvmHeapConfig:
          type: object
          description: Current and recommended JVM heap settings
          properties:
            currentXms:
              type: string
              description: "Current -Xms value (e.g., '256m')"
            currentXmx:
              type: string
              description: "Current -Xmx value (e.g., '512m')"
            recommendedXms:
              type: string
              description: "Recommended -Xms value"
            recommendedXmx:
              type: string
              description: "Recommended -Xmx value"
          required: ["currentXms", "currentXmx", "recommendedXms", "recommendedXmx"]
        containerMemory:
          type: object
          description: Container memory limit vs JVM footprint
          properties:
            currentLimit:
              type: string
              description: "Current container memory limit (e.g., '256Mi')"
            estimatedJvmFootprint:
              type: string
              description: "Estimated total JVM memory footprint (heap + metaspace + threads)"
            recommendedLimit:
              type: string
              description: "Recommended container memory limit"
          required: ["currentLimit", "estimatedJvmFootprint", "recommendedLimit"]
        skillToken:
          type: object
          description: Token from find-token skill proving tool execution
          properties:
            token:
              type: string
              description: "The DIAG token returned by find-token.sh. Use the 'find-token' skill."
            generator:
              type: string
              enum: ["find-token.sh", "find-token", "token-generator.sh"]
              description: "The script that generated the token. Use the 'find-token' skill."
          required: ["token", "generator"]
      required: ["jvmHeapConfig", "containerMemory", "skillToken"]
  tools:
    skills:
      # TODO: Replace with Konflux-built image when available for https://github.com/openshift/agentic-skills
      - image: quay.io/harpatil/agentic-skills:latest
        paths:
          - /skills/find-token
  analysis:
    agent: smart
  execution:
    agent: default
  verification:
    agent: fast
PROPOSALEOF
    info "Proposal created (alertmanager-jvm-oomkill)"

    cat <<PROPOSALEOF | oc apply -f - >/dev/null 2>&1
apiVersion: agentic.openshift.io/v1alpha1
kind: Proposal
metadata:
  name: advisory-jvm-oomkill-minimal
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
  analysisOutput:
    mode: Minimal
    schema:
      type: object
      description: JVM memory analysis — lightweight custom output
      properties:
        jvmHeapConfig:
          type: object
          properties:
            currentXmx:
              type: string
              description: "Current -Xmx value (e.g., '512m')"
            recommendedXmx:
              type: string
              description: "Recommended -Xmx value"
          required: ["currentXmx", "recommendedXmx"]
        containerMemoryLimit:
          type: string
          description: "Current container memory limit (e.g., '256Mi')"
        severity:
          type: string
          enum: ["Low", "Medium", "High", "Critical"]
          description: "Severity of the memory misconfiguration"
        skillToken:
          type: string
          description: "The DIAG token returned by find-token.sh. Use the 'find-token' skill."
      required: ["jvmHeapConfig", "severity", "skillToken"]
  tools:
    skills:
      # TODO: Replace with Konflux-built image when available for https://github.com/openshift/agentic-skills
      - image: quay.io/harpatil/agentic-skills:latest
        paths:
          - /skills/find-token
  analysis:
    agent: smart
PROPOSALEOF
    info "Proposal created (advisory-jvm-oomkill-minimal)"
}

verify_deploy() {
    step "Verifying"
    oc get pods -n "${NS_OPERATOR}" --no-headers 2>/dev/null
}
