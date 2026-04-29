#!/usr/bin/env bash
# Deploy the full agentic stack on a fresh OpenShift cluster.
# For subsequent iterations, use the redeploy-*.sh scripts instead.
#
# Components deployed:
#   - Operator (lightspeed-operator-controller-manager)
#   - Agent sandbox (lightspeed-chat pod via SandboxTemplate)
#   - Skills OCI images (full + per-profile: design, remediate, escalate, monitor)
#   - Console plugin (lightspeed-agentic-console)
#   - Proposal API chain (LlmProvider → Agent → Workflow CRs)
#
# Usage:
#   KUBECONFIG=/path/to/kubeconfig bash hack/deploy-agentic.sh --provider=vertex
#   KUBECONFIG=/path/to/kubeconfig bash hack/deploy-agentic.sh --provider=bedrock
#   KUBECONFIG=/path/to/kubeconfig bash hack/deploy-agentic.sh --provider=vertex --skip-build
#   KUBECONFIG=/path/to/kubeconfig bash hack/deploy-agentic.sh --provider=vertex --with-demo
#
# Environment variables:
#   KUBECONFIG          - Required. Path to cluster kubeconfig.
#   LLM_PROVIDER        - Alternative to --provider flag (vertex|bedrock).
#
#   Vertex AI:
#     VERTEX_PROJECT    - Required. GCP project with Vertex AI enabled.
#     VERTEX_REGION     - GCP region (default: global).
#     VERTEX_KEY_TTL    - SA key auto-revoke TTL in seconds (default: 64800 = 18h).
#
#   AWS Bedrock:
#     AWS_ACCESS_KEY_ID     - Bedrock access key (or reads from aws cli config).
#     AWS_SECRET_ACCESS_KEY - Bedrock secret key (or reads from aws cli config).
#     AWS_REGION            - Bedrock region (or reads from aws cli config).
#
#   Optional secrets (reads from macOS Keychain if unset):
#     GH_TOKEN              - GitHub API token for agent tools.
#     RH_API_OFFLINE_TOKEN  - Red Hat API offline token for support tools.

show_usage() {
    echo "Usage: KUBECONFIG=<path> bash hack/deploy-agentic.sh --provider=<vertex|bedrock> [--skip-build] [--with-demo]"
    echo ""
    echo "Flags:"
    echo "  --provider=<vertex|bedrock>  LLM provider (required)"
    echo "  --skip-build                 Skip container image builds"
    echo "  --with-demo                  Deploy test fixtures (crash-looping demo app)"
    echo ""
    echo "See hack/AGENTIC_DEPLOY.md for full documentation."
}

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/agentic-lib.sh"
parse_args "$@"

# Provider can come from --provider flag or LLM_PROVIDER env var
LLM_PROVIDER="${LLM_PROVIDER:-}"
if [[ -z "${LLM_PROVIDER}" ]]; then
    fail "LLM provider not set. Use --provider=vertex or --provider=bedrock"
fi
case "${LLM_PROVIDER}" in
    vertex|bedrock) ;;
    *) fail "Unknown provider: ${LLM_PROVIDER}. Supported: vertex, bedrock" ;;
esac

###############################################################################
# Day 0 — Operator Installation (Cluster Admin)
# Timeline ref: gist harche/ac8e8399a9bf69091a38a5cf6e3bc56b
###############################################################################
check_cluster
ensure_registry_route
install_crds
ensure_namespace
deploy_operator_manifests
build_push_operator
install_agent_sandbox_controller
ensure_agent_rbac
ensure_agent_service

###############################################################################
# Day 0, Step 1 — LLM credentials + LLMProvider CRs (Cluster Admin)
###############################################################################
LLM_SECRET="llm-credentials"

if [[ "${LLM_PROVIDER}" == "vertex" ]]; then
    step "Ensuring LLM credentials (Vertex AI)"
    VERTEX_REGION="${VERTEX_REGION:-global}"

    if ! oc get secret "${LLM_SECRET}" -n "${NS_OPERATOR}" >/dev/null 2>&1; then
        SCOPED_KEY_FILE=$(mktemp /tmp/lightspeed-vertex-key-XXXXXX)
        mv "${SCOPED_KEY_FILE}" "${SCOPED_KEY_FILE}.json"
        SCOPED_KEY_FILE="${SCOPED_KEY_FILE}.json"
        ensure_vertex_credentials "${SCOPED_KEY_FILE}"

        oc create secret generic "${LLM_SECRET}" -n "${NS_OPERATOR}" \
            --from-file=credentials.json="${SCOPED_KEY_FILE}" \
            --from-literal=ANTHROPIC_VERTEX_PROJECT_ID="${VERTEX_PROJECT}" \
            --from-literal=CLOUD_ML_REGION="${VERTEX_REGION}" >/dev/null 2>&1
        info "LLM credentials created (scoped SA key, project=${VERTEX_PROJECT}, region=${VERTEX_REGION})"

        rm -f "${SCOPED_KEY_FILE}"
    else
        info "LLM credentials already exist"
    fi

elif [[ "${LLM_PROVIDER}" == "bedrock" ]]; then
    step "Ensuring LLM credentials (Bedrock)"
    BEDROCK_ACCESS_KEY="${AWS_ACCESS_KEY_ID:-$(aws configure get aws_access_key_id 2>/dev/null || true)}"
    BEDROCK_SECRET_KEY="${AWS_SECRET_ACCESS_KEY:-$(aws configure get aws_secret_access_key 2>/dev/null || true)}"
    BEDROCK_REGION="${AWS_REGION:-$(aws configure get region 2>/dev/null || echo us-east-1)}"

    if ! oc get secret "${LLM_SECRET}" -n "${NS_OPERATOR}" >/dev/null 2>&1; then
        if [[ -n "${BEDROCK_ACCESS_KEY}" ]] && [[ -n "${BEDROCK_SECRET_KEY}" ]]; then
            oc create secret generic "${LLM_SECRET}" -n "${NS_OPERATOR}" \
                --from-literal=AWS_ACCESS_KEY_ID="${BEDROCK_ACCESS_KEY}" \
                --from-literal=AWS_SECRET_ACCESS_KEY="${BEDROCK_SECRET_KEY}" \
                --from-literal=AWS_REGION="${BEDROCK_REGION}" >/dev/null 2>&1
            info "LLM credentials created (Bedrock: region=${BEDROCK_REGION})"
        else
            fail "AWS credentials not found — set AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY or configure aws cli"
        fi
    else
        info "LLM credentials already exist"
    fi
fi

###############################################################################
# Day 0, Step 4 — Runtime secrets in operator namespace (Cluster Admin)
# Tool credentials (GitHub, Red Hat API, ACS) for agent sandbox pods.
###############################################################################
ensure_tool_secrets
build_push_agent_and_skills

###############################################################################
# Base SandboxTemplate — provider-agnostic. The operator patches in LLM
# credentials, skills, MCP servers, and phase config from the CRD chain
# (Agent + ComponentTools + LLMProvider) at proposal reconciliation time.
###############################################################################
AGENT_IMAGE="${INTERNAL_REG}/${NS_OPERATOR}/lightspeed-agentic-sandbox:${TAG}"

deploy_base_template <<SANDBOXEOF
apiVersion: extensions.agents.x-k8s.io/v1alpha1
kind: SandboxTemplate
metadata:
  name: lightspeed-agent
  namespace: openshift-lightspeed
spec:
  networkPolicyManagement: Unmanaged
  podTemplate:
    spec:
      serviceAccountName: lightspeed-agent
      automountServiceAccountToken: true
      containers:
      - name: agent
        image: ${AGENT_IMAGE}
        imagePullPolicy: Always
        ports:
          - containerPort: 8080
            protocol: TCP
        env:
          - name: LIGHTSPEED_SKILLS_DIR
            value: /app/skills
        volumeMounts:
          - name: skills
            mountPath: /app/skills
          - name: home
            mountPath: /home/agent
          - name: tmp
            mountPath: /tmp
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10
        resources:
          requests:
            cpu: 500m
            memory: 1Gi
          limits:
            cpu: "4"
            memory: 4Gi
      volumes:
      - name: skills
        image:
          reference: placeholder:latest
          pullPolicy: Always
      - name: home
        emptyDir: {}
      - name: tmp
        emptyDir: {}
SANDBOXEOF

###############################################################################
# Console plugin — built and pushed here, deployed by the operator's
# console reconciler via --agentic-console-image flag.
###############################################################################
build_push_console

###############################################################################
# Day 0, Steps 1-3 — Proposal API chain (Cluster Admin)
# LLMProvider → Agent → ProposalTemplate
# See timeline: gist harche/ac8e8399a9bf69091a38a5cf6e3bc56b
###############################################################################
step "Setting up proposal API chain (Day 0)"

if oc get secret "${LLM_SECRET}" -n "${NS_OPERATOR}" >/dev/null 2>&1; then
    if [[ "${LLM_PROVIDER}" == "vertex" ]]; then
        cat <<LLMEOF | oc apply -f - >/dev/null 2>&1
apiVersion: agentic.openshift.io/v1alpha1
kind: LLMProvider
metadata:
  name: smart
spec:
  type: Vertex
  credentialsSecret:
    name: ${LLM_SECRET}
    namespace: ${NS_OPERATOR}
  model: claude-opus-4-6
---
apiVersion: agentic.openshift.io/v1alpha1
kind: LLMProvider
metadata:
  name: fast
spec:
  type: Vertex
  credentialsSecret:
    name: ${LLM_SECRET}
    namespace: ${NS_OPERATOR}
  model: claude-haiku-4-5
LLMEOF
        info "LLMProvider CRs created (smart=opus-4.6, fast=haiku-4.5 via Vertex)"

    elif [[ "${LLM_PROVIDER}" == "bedrock" ]]; then
        cat <<LLMEOF | oc apply -f - >/dev/null 2>&1
apiVersion: agentic.openshift.io/v1alpha1
kind: LLMProvider
metadata:
  name: smart
spec:
  type: Bedrock
  credentialsSecret:
    name: ${LLM_SECRET}
    namespace: ${NS_OPERATOR}
  model: us.anthropic.claude-opus-4-6-v1
---
apiVersion: agentic.openshift.io/v1alpha1
kind: LLMProvider
metadata:
  name: fast
spec:
  type: Bedrock
  credentialsSecret:
    name: ${LLM_SECRET}
    namespace: ${NS_OPERATOR}
  model: us.anthropic.claude-haiku-4-5-20251001-v1:0
LLMEOF
        info "LLMProvider CRs created (smart=opus-4.6, fast=haiku-4.5 via Bedrock)"
    fi
fi

setup_proposal_agents_and_workflows

if [[ "${WITH_DEMO}" == "true" ]]; then
    deploy_test_fixtures
fi

verify_deploy

echo -e "\n${GREEN}Full agentic stack deployed (provider: ${LLM_PROVIDER}).${NC}"
echo -e "    Day 0 complete: LLMProvider → Agent → ProposalTemplate"
echo -e "    Day 1 (create a proposal):  oc apply -f ../lightspeed-agentic-operator/examples/setup/03-proposals.yaml"
