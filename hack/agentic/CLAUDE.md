# Agentic Deploy Scripts

Scripts for building and deploying the agentic stack on OpenShift. All builds
run on-cluster via OpenShift BuildConfigs (binary source + Docker strategy) —
no local container engine needed.

## Full deploy (fresh cluster)

```bash
KUBECONFIG=/path/to/kubeconfig bash hack/agentic/deploy.sh --provider=vertex
KUBECONFIG=/path/to/kubeconfig bash hack/agentic/deploy.sh --provider=bedrock
KUBECONFIG=/path/to/kubeconfig bash hack/agentic/deploy.sh --provider=vertex --skip-build
KUBECONFIG=/path/to/kubeconfig bash hack/agentic/deploy.sh --provider=vertex --with-demo
```

Deploys: CRDs, namespace, builds (agent + skills in parallel, then console,
then operator), LLMProvider, Agent tiers, ApprovalPolicy, SandboxTemplate.

Required env vars for Vertex: `VERTEX_PROJECT`. For Bedrock: `AWS_ACCESS_KEY_ID`,
`AWS_SECRET_ACCESS_KEY` (or aws cli config).

## Fast iteration (redeploy single component)

```bash
KUBECONFIG=... bash hack/agentic/redeploy-operator.sh     # operator only
KUBECONFIG=... bash hack/agentic/redeploy-agent.sh        # agent sandbox + skills
KUBECONFIG=... bash hack/agentic/redeploy-console.sh      # console plugin only
KUBECONFIG=... bash hack/agentic/redeploy-skills.sh       # skills image only
KUBECONFIG=... bash hack/agentic/redeploy-all.sh          # everything (parallel)
```

All scripts accept `--skip-build` to skip the image build and just rollout.

## Teardown

```bash
KUBECONFIG=... bash hack/agentic/undeploy.sh
KUBECONFIG=... VERTEX_PROJECT=... bash hack/agentic/undeploy.sh  # also cleans GCP SA
```

## How builds work

- `lib.sh` defines `_build sync` (sequential, streams logs) and
  `_build async` + `wait_all_builds()` (parallel, polls status).
  `build_on_cluster` and `start_build_async` are thin wrappers around `_build`.
- Each component has a BuildConfig + ImageStream in `openshift-lightspeed`.
- Images are tagged as `wt-<name>` in worktrees, `latest` in main repo.
  Multiple worktrees can deploy to the same cluster without clobbering.
- 4 images total: operator, agent sandbox, console plugin, skills.
- Skills is a single OCI image with all skills. Per-proposal skill selection
  uses `SkillsSource.paths` in the Proposal CRD (no per-profile images needed).
- The operator build constructs a minimal context with just
  `lightspeed-operator/` and `lightspeed-agentic-operator/` (copied to a temp
  dir) — it does NOT upload the entire workspace root.
- All `oc` commands that suppress output go through `_run()`, which captures
  stdout/stderr to a tempfile and surfaces it only on failure.

## Image overrides (dev scripts only)

These deploy scripts default to on-cluster images (built via BuildConfigs).
In production, Konflux-built images from `related_images.json` are used
instead — the Konflux pipeline substitutes `__REPLACE_*__` placeholders in the
operator CSV at bundle build time (see `hack/image_placeholders.json`).

Override any dev image via env vars to use external images (e.g. Konflux):

| Variable | Default | Description |
|---|---|---|
| `OPERATOR_IMG` | `image-registry.../lightspeed-operator:<tag>` | Operator binary |
| `CONSOLE_IMG` | `image-registry.../lightspeed-console-plugin:<tag>` | Agentic console plugin |
| `AGENT_IMG` | `image-registry.../lightspeed-agentic-sandbox:<tag>` | Agent sandbox |
| `SKILLS_IMG` | `image-registry.../agentic-skills:<tag>` | Skills OCI image |

Example — deploy with Konflux-built sandbox and console (skip their builds):

```bash
KUBECONFIG=... \
AGENT_IMG=$(jq -r '.[] | select(.name=="lightspeed-agentic-sandbox") | .image' related_images.json) \
CONSOLE_IMG=$(jq -r '.[] | select(.name=="lightspeed-agentic-console") | .image' related_images.json) \
bash hack/agentic/deploy.sh --provider=vertex --skip-build
```

`related_images.json` is the source of truth for Konflux image references.
Look up any image by name:

```bash
jq -r '.[] | select(.name=="lightspeed-agentic-sandbox") | .image' related_images.json
jq -r '.[] | select(.name=="lightspeed-agentic-console") | .image' related_images.json
```

In production, these images are wired into the operator deployment via
`hack/image_placeholders.json` + `config/default/deployment-patch.yaml`.
The Konflux build pipeline substitutes the `__REPLACE_*__` placeholders at
bundle build time.

## Repo path overrides

All repo paths are auto-detected from the workspace layout but can be
overridden via environment variables:

| Variable | Default | Used by |
|---|---|---|
| `AGENTIC_OPERATOR_DIR` | `../lightspeed-agentic-operator` | Operator build (Go types) |
| `AGENT_DIR` | `../lightspeed-agentic-sandbox` | Agent sandbox build |
| `CONSOLE_DIR` | `../lightspeed-agentic-console` | Console plugin build |
| `SKILLS_DIR` | `../agentic-skills` | Skills image build |

## Components

| Component | BuildConfig | Build context | Dockerfile |
|---|---|---|---|
| Operator | `lightspeed-operator` | `lightspeed-operator/` + `lightspeed-agentic-operator/` (minimal) | `lightspeed-operator/Dockerfile.dev` |
| Agent sandbox | `lightspeed-agentic-sandbox` | `lightspeed-agentic-sandbox/` | `Containerfile.dev` |
| Console plugin | `lightspeed-console-plugin` | `lightspeed-agentic-console/` | `Dockerfile` |
| Skills | `agentic-skills` | `agentic-skills/` | `Containerfile` |
