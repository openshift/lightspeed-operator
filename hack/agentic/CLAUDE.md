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

- `lib.sh` defines `build_on_cluster()` (sequential, streams logs) and
  `start_build_async()` + `wait_all_builds()` (parallel, polls status).
- Each component has a BuildConfig + ImageStream in `openshift-lightspeed`.
- Images are tagged as `wt-<name>` in worktrees, `latest` in main repo.
  Multiple worktrees can deploy to the same cluster without clobbering.
- 4 images total: operator, agent sandbox, console plugin, skills.
- Skills is a single OCI image with all skills. Per-proposal skill selection
  uses `SkillsSource.paths` in the Proposal CRD (no per-profile images needed).
- The operator build constructs a minimal context with just
  `lightspeed-operator/` and `lightspeed-agentic-operator/` (copied to a temp
  dir) — it does NOT upload the entire workspace root.

## Repo path overrides

All repo paths are auto-detected from the workspace layout but can be
overridden via environment variables:

| Variable | Default | Used by |
|---|---|---|
| `AGENTIC_OPERATOR_DIR` | `../lightspeed-agentic-operator` | Operator build (Go types) |
| `AGENT_DIR` | `../lightspeed-agentic-sandbox` | Agent sandbox build |
| `CONSOLE_DIR` | `../lightspeed-agentic-console` | Console plugin build |
| `SKILLS_DIR` | `../lightspeed-skills` | Skills image build |

## Components

| Component | BuildConfig | Build context | Dockerfile |
|---|---|---|---|
| Operator | `lightspeed-operator` | `lightspeed-operator/` + `lightspeed-agentic-operator/` (minimal) | `lightspeed-operator/Dockerfile.dev` |
| Agent sandbox | `lightspeed-agentic-sandbox` | `lightspeed-agentic-sandbox/` | `Containerfile.dev` |
| Console plugin | `lightspeed-console-plugin` | `lightspeed-agentic-console/` | `Dockerfile` |
| Skills | `lightspeed-skills` | `lightspeed-skills/` | `Containerfile` |
