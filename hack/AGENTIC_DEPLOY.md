# Agentic Deploy Scripts

Scripts for deploying the full agentic stack to an OpenShift cluster for development and testing.

## Components

| Component | Source Repo | Image | Managed By |
|-----------|-------------|-------|------------|
| Operator + agentic controller | `lightspeed-operator` (imports `lightspeed-agentic-operator`) | `lightspeed-operator` | Deployment |
| Agent sandbox | `lightspeed-agentic-sandbox` | `lightspeed-agentic-sandbox` | SandboxTemplate/SandboxClaim (standalone pod) |
| Skills | `lightspeed-skills` | `lightspeed-skills` (+ per-profile images) | OCI image volumes mounted in agent pod |
| Console plugin | `lightspeed-agentic-console` | `lightspeed-console-plugin` | Deployment (created by operator via OLSConfig) |
| Proposal API | `lightspeed-agentic-operator` (CRD definitions) | N/A | LLMProvider, Agent, ComponentTools, Workflow, Proposal CRs |

The agentic controller (`lightspeed-agentic-operator`) is imported as a Go module into `lightspeed-operator`. It provides the CRDs and the proposal reconciler. The operator image contains both the base OLSConfig controller and the agentic proposal controller.

### Agentic CRD Chain

The agentic API types are defined in `lightspeed-agentic-operator/api/v1alpha1/` and compose as:

```
LLMProvider (cluster) → Agent (cluster) → Workflow (namespaced) → Proposal (namespaced)
                                              ↑
                                    ComponentTools (namespaced)
```

- **LLMProvider** — LLM backend config (type, model, credentials)
- **Agent** — Tier that references an LLMProvider (e.g., "smart" = Opus, "fast" = Haiku)
- **ComponentTools** — Domain-specific tools: skills images, MCP servers, system prompt, required secrets
- **Workflow** — 3-step pipeline template pairing Agent tiers with ComponentTools per step
- **Proposal** — Unit of work referencing a Workflow

Example manifests are in `lightspeed-agentic-operator/examples/setup/`.

## Prerequisites

- **oc** — OpenShift CLI, logged into your target cluster
- **docker** — For building container images (must support `--platform linux/amd64`)
- **skopeo** — For pushing images to the OpenShift internal registry
- **jq** — For JSON processing during image placeholder substitution
- **Vertex AI provider:** `gcloud` CLI, authenticated with a project that has Vertex AI enabled
- **Bedrock provider:** `aws` CLI configured, or `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY` env vars

## Workspace Layout

The scripts expect sibling repos next to `lightspeed-operator/`:

```
workspace-root/
├── lightspeed-operator/          # This repo (hack/ scripts live here)
├── lightspeed-agentic-console/   # Console plugin (React + PatternFly 6)
├── lightspeed-agentic-sandbox/   # Agent sandbox (TypeScript, Claude Agent SDK)
└── lightspeed-skills/            # Skills OCI images
```

Override sibling paths via env vars if your layout differs:
- `CONSOLE_DIR` — path to console plugin repo (default: `../lightspeed-agentic-console`)
- `AGENT_DIR` — path to agent sandbox repo (default: `../lightspeed-agentic-sandbox`)
- `SKILLS_DIR` — path to skills repo (default: `../lightspeed-skills`)

## Full Deploy (Fresh Cluster)

```bash
# Vertex AI
KUBECONFIG=/path/to/kubeconfig \
VERTEX_PROJECT=my-gcp-project \
  bash hack/deploy-agentic.sh --provider=vertex

# AWS Bedrock
KUBECONFIG=/path/to/kubeconfig \
AWS_ACCESS_KEY_ID=... \
AWS_SECRET_ACCESS_KEY=... \
AWS_REGION=us-east-1 \
  bash hack/deploy-agentic.sh --provider=bedrock

# With demo fixtures (crash-looping app + PrometheusRule)
KUBECONFIG=/path/to/kubeconfig \
VERTEX_PROJECT=my-gcp-project \
  bash hack/deploy-agentic.sh --provider=vertex --with-demo
```

This installs everything: CRDs, namespace, operator, agent-sandbox controller, RBAC, LLM credentials, agent pod, skills images, console plugin, and the proposal API chain (LlmProvider + Agent + Workflow CRs).

## Fast Iteration (Redeploy Scripts)

After the initial deploy, use these scripts to rebuild and redeploy individual components:

| Script | What it rebuilds | When to use |
|--------|-----------------|-------------|
| `hack/redeploy-agentic-operator.sh` | Operator only | Changed Go code in the operator |
| `hack/redeploy-agentic-console.sh` | Console plugin only | Changed React/TypeScript in the console |
| `hack/redeploy-agentic-agent.sh` | Agent + skills | Changed agent code or skills |
| `hack/redeploy-agentic-skills.sh` | Skills images only | Changed skill definitions |
| `hack/redeploy-agentic-all.sh` | All components | Changed multiple components |

All redeploy scripts support `--skip-build` to push the last-built images without rebuilding:

```bash
KUBECONFIG=/path/to/kubeconfig bash hack/redeploy-agentic-operator.sh
KUBECONFIG=/path/to/kubeconfig bash hack/redeploy-agentic-agent.sh --skip-build
```

## Worktree Support

Scripts auto-detect git worktrees and tag images accordingly:
- Main repo: images tagged as `:latest`
- Worktree `.worktrees/my-feature/`: images tagged as `:wt-my-feature`

This means multiple worktrees can deploy to the same cluster concurrently without overwriting each other's images.

## Environment Variables

### Required

| Variable | Used By | Description |
|----------|---------|-------------|
| `KUBECONFIG` | All scripts | Path to cluster kubeconfig |
| `VERTEX_PROJECT` | `--provider=vertex` | GCP project ID with Vertex AI enabled |

### Optional

| Variable | Default | Description |
|----------|---------|-------------|
| `VERTEX_REGION` | `us-east5` | GCP region for Vertex AI |
| `VERTEX_KEY_TTL` | `64800` (18h) | SA key auto-revocation TTL in seconds |
| `AWS_ACCESS_KEY_ID` | From `aws configure` | Bedrock access key |
| `AWS_SECRET_ACCESS_KEY` | From `aws configure` | Bedrock secret key |
| `AWS_REGION` | From `aws configure` or `us-east-1` | Bedrock region |
| `GH_TOKEN` | macOS Keychain | GitHub API token for agent tools |
| `RH_API_OFFLINE_TOKEN` | macOS Keychain | Red Hat API token for support tools |
| `CONSOLE_DIR` | `../lightspeed-agentic-console` | Console plugin repo path |
| `AGENT_DIR` | `../lightspeed-agentic-sandbox` | Agent sandbox repo path |
| `SKILLS_DIR` | `../lightspeed-skills` | Skills repo path |

## How It Works

### Image Build/Push Pipeline

1. Images are built locally with `docker build --platform linux/amd64` (cross-compile for ARM Macs)
2. Pushed to the OpenShift internal registry via `skopeo` with short-lived builder tokens (10min TTL)
3. Referenced inside the cluster via the internal registry endpoint (`image-registry.openshift-image-registry.svc:5000`)

### Operator Pause/Resume

The console plugin deployment is managed by the operator's reconciliation loop. When redeploying the console with a custom image, the scripts:
1. Scale the operator to 0 replicas (pause reconciliation)
2. Patch the console deployment with the new image
3. Wait for rollout
4. Scale the operator back to 1 replica

### Vertex AI Credentials

The deploy script creates a scoped GCP service account per cluster (derived from the API server hostname) with only `roles/aiplatform.user`. A fresh key is created and scheduled for auto-revocation after `VERTEX_KEY_TTL` seconds.

To clean up credentials manually:
```bash
gcloud iam service-accounts delete ls-<cluster-id>@<project>.iam.gserviceaccount.com
```

## Troubleshooting

**"Registry route not found"** — Enable the internal registry route:
```bash
oc patch configs.imageregistry.operator.openshift.io/cluster --type merge \
  -p '{"spec":{"defaultRoute":true}}'
```

**"Cannot create builder token"** — The `builder` service account may not exist in the namespace:
```bash
oc get sa builder -n openshift-lightspeed
```

**Console image keeps reverting** — The operator's reconciler is overwriting your image. The redeploy scripts handle this via pause/resume, but if deploying manually, scale the operator to 0 first.

**Skills directory is empty after agent redeploy** — Check that the SandboxTemplate uses an `image` volume (not `emptyDir`). The `redeploy-agentic-agent.sh` script auto-patches this, but verify with:
```bash
oc get sandboxtemplate lightspeed-chat -n openshift-lightspeed \
  -o jsonpath='{.spec.podTemplate.spec.volumes[?(@.name=="skills")]}'
```

**Agent pod not starting** — Check events and logs:
```bash
oc describe pod lightspeed-chat -n openshift-lightspeed
oc logs lightspeed-chat -n openshift-lightspeed -c agent
```
