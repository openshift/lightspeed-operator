---
name: investigate-ci-failure
description: Investigate CI/Prow/Konflux job failures on a GitHub pull request. Use when the user pastes a PR URL and asks about CI failures, red checks, test failures, or wants to understand why a job failed.
disable-model-invocation: true
---

# Investigate CI Failure

Given a PR URL (e.g. `https://github.com/openshift/lightspeed-operator/pull/1653`), diagnose why CI jobs failed.

This repo runs **both** CI systems on openshift PRs (validated against [PR #1641](https://github.com/openshift/lightspeed-operator/pull/1641)):

- **Prow** (`ci/prow/*`) — commit statuses; config lives in [openshift/release](https://github.com/openshift/release), not this repo. Artifacts in GCS (`prow.ci.openshift.org`).
- **Konflux** (`Red Hat Konflux / …`) — GitHub check runs; pipelines in `.tekton/`. Builds images, runs integration tests, FBC catalog updates.

Always start with `gh pr checks {pr} --repo openshift/lightspeed-operator` — a typical PR has **~8 Prow jobs + ~10+ Konflux checks + `tide`**.

## Workflow

### 1. Extract PR info

Parse org, repo, and PR number from the URL. Fetch metadata with `gh`:

```bash
# PR metadata
gh api repos/{org}/{repo}/pulls/{pr} --jq '{title, state, user: .user.login, head_sha: .head.sha}'

# Changed files
gh api repos/{org}/{repo}/pulls/{pr}/files --jq '.[].filename'
```

### 2. Get check statuses

```bash
# All checks at a glance (both Prow and Konflux)
gh pr checks {pr} --repo {org}/{repo}

# Prow failures — commit statuses with Prow URLs (use head SHA from step 1)
gh api repos/{org}/{repo}/statuses/{head_sha} \
  --jq '.[] | select(.state == "failure" or .state == "error") | {context, state, target_url}'

# Konflux failures — check runs (different API)
gh api "repos/{org}/{repo}/commits/{head_sha}/check-runs" \
  --jq '.check_runs[] | select(.conclusion == "failure" or .conclusion == "neutral") | {id, name, conclusion, output_title: .output.title}'
```

This gives you failed Prow jobs (with GCS URLs) and failed Konflux checks
(with check run IDs for drill-down). Route Prow failures to section 3–4,
Konflux failures to the Konflux Failures section.

### 3. Construct GCS artifact URLs

From a Prow `target_url` like:
```
https://prow.ci.openshift.org/view/gs/test-platform-results/pr-logs/pull/{org}_{repo}/{pr}/{job_name}/{build_id}
```

Derive:
- **Directory browser** (for navigating artifact tree):
  `https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/test-platform-results/pr-logs/pull/{org}_{repo}/{pr}/{job_name}/{build_id}/`
- **Raw file content** (for fetching logs and JSON):
  `https://storage.googleapis.com/test-platform-results/pr-logs/pull/{org}_{repo}/{pr}/{job_name}/{build_id}/{path}`

### 4. Triage the failure

For each failed job, fetch artifacts in this order:

#### 4a. Quick status

```
GET storage.googleapis.com/.../finished.json
```

Check `"passed": false` and `"result": "FAILURE"`.

#### 4b. Build log (most useful)

```
GET storage.googleapis.com/.../build-log.txt
```

This is the main ci-operator build log. It can be large (200KB+). Search from the **end** for:
- `failed` / `FAILED` / `error` / `ERROR`
- `step .* failed`
- Go test / Ginkgo failures (`FAIL!`, `Summarizing`, `Expected`, `Timed out`)
- Python tracebacks (service-integration pipelines only — clones lightspeed-service)
- Container crash indicators (`CrashLoopBackOff`, `OOMKilled`, `Error from server`)

#### 4c. Artifact tree exploration

The build log alone often doesn't tell the full story. Browse the GCS artifact directory
to find step-specific logs, cluster state, and pod logs:

```
GET gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/.../artifacts/
```

**Operator repo** (`ci/prow/bundle-e2e-4-XX`) — validated on PR #1641 `bundle-e2e-4-21`:
```
{build_id}/
├── build-log.txt
├── finished.json
├── artifacts/
│   ├── ci-operator.log, junit_operator.xml, metadata.json, …
│   └── bundle-e2e-4-XX/           ← step name matches job (4.16, 4.21, …)
│       ├── ipi-install-rbac/build-log.txt
│       ├── e2e-install/build-log.txt
│       ├── e2e-test/build-log.txt   ← Ginkgo e2e (test/e2e); tail shows PASS/FAIL
│       ├── fips-check-fips-or-die/
│       ├── gather-must-gather/artifacts/  ← camgi.html, must-gather.tar
│       └── gather-extra/
```

`e2e-test/build-log.txt` ends with Ginkgo summary, e.g.
`Ran 19 of 37 Specs` / `SUCCESS!` — search backward for `FAIL!`, `[FAILED]`, `Timed out`.

**lightspeed-service repo** (not operator) uses `e2e-ols-cluster/e2e/` with pytest
`junit_e2e_*.xml` and per-provider `podlogs/` — do not expect that tree on operator Prow jobs.

Konflux integration failures: cluster dumps on Quay
`quay.io/openshift-lightspeed/ols-operator-artifacts:{head_sha}` under `konflux-artifacts/`
(JSON cluster state, `pods/…/*.log.gz`, etc.) — see K6.

**Where to look by failure type (operator):**

| Symptom | Check these artifacts |
|---|---|
| Ginkgo / operator e2e failure | Prow `bundle-e2e-4-XX/e2e-test/build-log.txt`; Konflux `ols-operator-artifacts:{sha}` |
| OLS app-server / postgres / operator crash | Konflux `konflux-artifacts/pods/…` log gzips; Prow `gather-must-gather/` |
| Operator manager `unknown flag` / exit 2 | CSV `containerCommand` vs image built — often bundle/operator mismatch |
| Image build failure | Prow `build-logs/*.log` or Konflux `build-images` task (K2 UI link) |
| Cluster infra | `gather-must-gather/artifacts/camgi.html` |

#### 4d. Downloading artifacts locally

When you need to search across many files or the artifacts are too large
for WebFetch, download them to a temp directory using `gsutil` or `gcloud storage`:

```bash
TMPDIR=$(mktemp -d)
# Operator bundle e2e (adjust 4-XX to match job name)
gcloud storage cp -r \
  gs://test-platform-results/pr-logs/pull/openshift_lightspeed-operator/{pr}/pull-ci-openshift-lightspeed-operator-main-bundle-e2e-4-21/{build_id}/artifacts/bundle-e2e-4-21/e2e-test/ \
  "$TMPDIR/"
```

The GCS bucket path mirrors the Prow URL: strip `https://prow.ci.openshift.org/view/gs/`
and prepend `gs://`.

When multiple jobs have failed, investigate each in a separate subagent (Task tool)
to keep build-log context isolated and run fetches in parallel.

### 5. Cross-reference with PR changes

Compare the failure with the files changed in the PR. Common patterns:

| Failure type | Likely cause |
|---|---|
| Unit/integration test failure | Direct code bug in changed files |
| e2e cluster test failure | Infrastructure issue OR deployment-breaking change |
| Verify/lint failure | Formatting, type errors, or import issues |
| Image build failure | Dependency or Dockerfile issue |
| Flaky (passes on retest) | Known flake, not PR-related |

Check if the same job fails on `main` branch (flaky test) by looking at job history:
```
https://prow.ci.openshift.org/job-history/gs/test-platform-results/pr-logs/directory/{job_name}
```

### 6. Report findings

Summarize:
1. **Which jobs failed** and which passed
2. **Root cause** for each failure (with relevant log excerpts)
3. **Whether it's PR-related or infrastructure/flaky**
4. **Suggested fix** if the failure is caused by the PR changes
5. **Recommended action** — one of:
   - **Retry** — for infra flakes, transient timeouts, EaaS cluster
     issues. Prefer the CI-native retry (Prow rerun button, Konflux
     re-run in UI) over `/retest` PR comment. Ask the user whether
     they want you to post `/retest {job}` to the PR automatically
   - **Fix in PR** — for test failures, scan findings, build errors caused
     by PR changes (include what to fix)
   - **Escalate** — for persistent infra issues, platform-side problems,
     or failures unrelated to the PR that don't resolve on retry

## Konflux Failures

Konflux runs as GitHub check runs (not commit statuses like Prow). All data
is accessible via the GitHub Check Runs API — no Konflux cluster auth needed.

### K1. List Konflux check runs

```bash
gh api "repos/{org}/{repo}/commits/{head_sha}/check-runs" \
  --jq '.check_runs[] | select(.name | test("Konflux"; "i")) | {id, name, status, conclusion, output_title: .output.title}'
```

This returns all Konflux check runs for the commit. Look for `conclusion`
values: `success`, `failure`, `neutral` (warning), `skipped`.

### K2. Get failure details

For each failed or neutral check run, fetch **both** `output.text` and
`output.summary` (validated on [PR #1641](https://github.com/openshift/lightspeed-operator/pull/1641)):

```bash
gh api repos/{org}/{repo}/check-runs/{check_run_id} \
  --jq '{name, conclusion, output_title: .output.title,
         summary_len: (.output.summary|length),
         text_len: (.output.text|length),
         output_summary: .output.summary,
         output_text: .output.text}'
```

**Where the Tekton task table lives** (lightspeed-operator):

| Check type | Task table location | `output.summary` |
|---|---|---|
| Build pipelines (`lightspeed-operator-on-pull-request`, `ols-bundle-on-pull-request`) | **`output.text`** — HTML `<h4>Task Statuses:</h4>` table with 🟢/🔴 and per-task Konflux UI log links | Often short (e.g. "Build pipeline … has passed") |
| Integration tests (`operator-e2e-tests-*`, `upgrade-e2e-tests`, `service-e2e-tests-*`, console tests) | **`output.text`** — markdown table: Task \| Duration \| Status \| Details; pipelinerun link at top | One line: "Integration test for component … has passed/failed" |
| Enterprise Contract / trusted links | `https://red.ht/trusted` in `gh pr checks` — use EC docs or UI; not a task table in the API |

Do **not** rely on `output.summary` alone for task-level pass/fail — parse
`output.text` and follow the Konflux UI log link for the failed task.

### K3. Check for annotations

Failed check runs may include inline annotations with specific details:

```bash
gh api repos/{org}/{repo}/check-runs/{check_run_id}/annotations \
  --jq '.[] | {path, annotation_level, message}'
```

### K4. Triage by task name

Task names vary by pipeline. Common ones seen across repos:

**Build & scan pipeline**:

| Failed task | What it means | Likely cause |
|---|---|---|
| `build-images` | Container image build failed | Dockerfile error, dependency issue, build timeout |
| `clair-scan` | CVE vulnerability found in image | New dependency introduced a known CVE |
| `clamav-scan` | Malware scan failed | Rare; usually a false positive |
| `sast-snyk-check` | Static analysis security finding | Code pattern flagged by Snyk |
| `sast-shell-check` | Shell script linting failure | Shellcheck error in scripts |
| `sast-unicode-check` | Suspicious Unicode characters | Homoglyph or bidirectional text detected |
| `ecosystem-cert-preflight-checks` | Red Hat certification preflight failed | Image metadata, labels, or structure issue |
| `deprecated-base-image-check` | Base image is deprecated | Update base image in Dockerfile |
| `rpms-signature-scan` | RPM signature verification failed | Unsigned or tampered RPM in image |
| `prefetch-dependencies` | Dependency prefetch failed | Network issue or invalid dependency reference |

**Integration test pipeline**:

| Failed task | What it means | Likely cause |
|---|---|---|
| `eaas-provision-space` | EaaS namespace provisioning failed | Infra issue, quota exhausted |
| `provision-cluster` | Test cluster provisioning failed | Infra issue, cluster pool exhausted |
| `ols-install` | OLS operator installation failed | Operator or CRD issue in the PR changes |
| `ols-operator-tests` | e2e operator tests failed or timed out | Test failure or timeout (2h limit) — check artifacts on Quay |
| `export-logs-for-retention` | Log export to Quay failed | Usually infra; artifacts may be missing |

**Separate check**:

| Check | What it means | Likely cause |
|---|---|---|
| Enterprise Contract | Policy violation on built image | Missing signatures, provenance, or policy rules not met |

This list is not exhaustive — new tasks may appear. Use the task name and
the check summary (K2) to understand what failed.

### K5. Cross-reference with PR changes

| Failure type | Likely cause |
|---|---|
| `clair-scan` after dependency bump | New dependency has a CVE — check `go.mod` / `go.sum` changes |
| `build-images` failure | `Dockerfile` or `bundle.Dockerfile` change, or Go build error |
| `ols-bundle-on-pull-request` triggered | `bundle/**`, `bundle.Dockerfile`, or **`related_images.json`** changed (even if `bundle/` matches `main`) |
| `install-operator` / `ols-install` failed | OLM install — bundle CSV args vs operator image mismatch, or `hack/install/install-operator-bundle.sh` |
| `sast-*` failure | New code pattern flagged by static analysis |
| Enterprise Contract warning | Usually not blocking for PRs (runs on merge) — check if `conclusion` is `neutral` vs `failure` |
| All tasks pass but check is `neutral` | EC ran as optional/warning — informational, not blocking |

### K6. Fetching artifacts from Quay (no auth needed)

Konflux uploads scan results as OCI attachments on the built image in
Quay. These are **publicly accessible** — no Konflux auth required.

The Quay image path is Konflux-configured and varies per repo:

| Image | Quay base / tag pattern |
|---|---|
| Operator manager (build) | `quay.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols/lightspeed-operator:on-pr-{head_sha}` |
| OLM bundle (build) | `quay.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols-bundle:on-pr-{head_sha}` |
| E2e / integration artifacts | `quay.io/openshift-lightspeed/ols-operator-artifacts:{head_sha}` (and per-scenario tags) |
| lightspeed-service (integration pipelines only) | `quay.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols/lightspeed-service:on-pr-{head_sha}` |

The `head_sha` comes from step 1 (`gh api .../pulls/{pr}`). Tag patterns
vary per repo — check the STEP-UPLOAD log or try common patterns:

| Tag pattern | Example |
|---|---|
| `on-pr-{head_sha}` | Konflux build images (operator, bundle) — **expires ~5 days**; older PRs return `not found` from Quay |
| `on-pr-{head_sha}-linux-x86-64` | Arch-specific image (Clair/SBOM attachments) |
| `{head_sha}` | **`ols-operator-artifacts`** — e2e cluster dumps; validated on #1641 |

**Retention:** PR build tags (`on-pr-{sha}`) are pruned quickly. For old PRs use
`ols-operator-artifacts:{head_sha}` for e2e dumps and Prow GCS (longer retention).
Scan SARIF/Clair attachments need a still-present `on-pr-{sha}` or digest from the
check run / build log.

This path is fully automated — no browser, no user input, no cluster auth.

**Step 1**: List attachments on the image (try the main tag first):
```bash
oras discover quay.io/{image_base}:{tag}
```

**Step 2**: If arch-specific images exist, check those too (Clair, SBOM):
```bash
oras discover quay.io/{image_base}:{tag}-linux-x86-64
```

Attachment types:

| Media type | File | Contents |
|---|---|---|
| `application/sarif+json` | `shellcheck-results.sarif` | ShellCheck SAST findings |
| `application/sarif+json` | `sast_snyk_check_out.sarif` | Snyk SAST findings |
| `application/sarif+json` | `sast_unicode_check_out.sarif` | Unicode control char findings |
| `application/vnd.clamav` | ClamAV result | Malware scan findings |
| `application/vnd.redhat.clair-report+json` | `clair-report-amd64.json` | Full Clair CVE report (on arch images) |
| SBOM (cosign attachment) | `sbom.json` | SPDX SBOM (tag: `sha256-{digest}.sbom`) |

**Step 3**: Fetch a specific attachment by digest:
```bash
TMPDIR=$(mktemp -d)
oras pull -o "$TMPDIR" quay.io/{image_base}@sha256:{digest}
```

**Step 4**: Parse results.

For SARIF files (SAST scans):
```bash
python3 -c "
import json, sys
d = json.load(open(sys.argv[1]))
for run in d.get('runs', []):
    tool = run['tool']['driver']['name']
    results = run.get('results', [])
    print(f'{tool}: {len(results)} finding(s)')
    for r in results:
        print(f'  [{r[\"level\"]}] {r[\"message\"][\"text\"][:200]}')
" "$TMPDIR"/*.sarif
```

For Clair reports (CVE scan):
```bash
python3 -c "
import json, sys
d = json.load(open(sys.argv[1]))
vulns = d.get('vulnerabilities', {})
pkg_vulns = d.get('package_vulnerabilities', {})
print(f'Total vulnerabilities: {len(vulns)}')
by_sev = {}
for vid, v in vulns.items():
    sev = v.get('normalized_severity', 'Unknown')
    by_sev.setdefault(sev, []).append(v)
for sev in ['Critical', 'High', 'Medium', 'Low', 'Unknown']:
    if sev in by_sev:
        print(f'  {sev}: {len(by_sev[sev])}')
" "$TMPDIR"/clair-report-*.json
```

This is the most reliable path for Konflux scan failures — fully automated,
gives the exact findings without needing Konflux UI access or user input.

Artifact content varies — don't assume a fixed structure. After pulling,
inspect what you got and adapt:

```bash
oras pull -o "$TMPDIR" quay.io/{image_base}:{tag}
find "$TMPDIR" -type f | head -30
```

Known artifact types you may encounter:

| What you find | How to parse |
|---|---|
| `*.sarif` files | SARIF JSON — SAST scan results (see parser above) |
| `clair-report-*.json` | Clair CVE report (see parser above) |
| `konflux-artifacts/` directory | Cluster state dumps (JSON: clusteroperators, events, RBAC, CSVs, etc.) |
| `openai/`, `azure_openai/`, etc. | Per-provider e2e test results |
| `leaktk-scan-*.log` | Leak detection scan logs (plain text) |
| `sbom.json` / `*.sbom` | SPDX SBOM |
| ClamAV results | Malware scan findings |

Explore the content, identify the relevant files, and parse accordingly.

**Limitations** — fall back to K7 (ask user for Konflux UI logs) when:
- **No artifacts on Quay** — some tasks don't upload results (e.g., build
  failures, infra steps)
- **Task timeouts** (`TaskRunTimeout`) — the task was killed before it
  could upload anything. Identifiable from the check run summary (K2):
  `❓ Reason: TaskRunTimeout`. Ask the user to open the Konflux UI log
  link for the timed-out task and paste the output

### K7. Fallback: ask user for Konflux UI logs

Use this only when K6 (Quay artifacts) doesn't have what you need — no
artifacts uploaded, task timed out, or the failure isn't explained by the
scan results.

The full task logs are only available through the **Konflux UI** in a
browser (SSO-authenticated). The Tekton Results API is not externally
accessible, and completed PipelineRuns are pruned from the Kubernetes API.

Provide the user with the direct Konflux UI log link for the failed task.
The URL pattern is in the check summary markdown (parsed in K2), e.g.:
```
https://konflux-ui.apps.stone-prd-rh01.pg1f.p1.openshiftapps.com/ns/crt-nshift-lightspeed-tenant/pipelinerun/<pipelinerun-name>/logs/<task-name>
```

Ask the user to open the link and paste the relevant log output into chat
for further analysis.

### K8. Parsing task logs (when user pastes from UI)

Each Konflux task log is divided into steps with `STEP-*` headers.

For **scan tasks**, the structure is:

```
STEP-USE-TRUSTED-ARTIFACT     ← artifact download (noise)
STEP-<ACTUAL-CHECK>            ← the scan step (look here)
STEP-UPLOAD                    ← result upload (noise)
```

For **e2e integration test tasks**, the structure is:

```
STEP-GET-KUBECONFIG            ← cluster credentials (noise unless it fails)
STEP-RUN-E2E-TESTS             ← the actual test run (look here)
STEP-PUSH-ARTIFACTS            ← upload results to Quay (noise)
```

When the user pastes a task log:

1. **Skip** infra steps (`STEP-USE-TRUSTED-ARTIFACT`, `STEP-UPLOAD`,
   `STEP-GET-KUBECONFIG`, `STEP-PUSH-ARTIFACTS`)
2. **Focus on the main step** — this is where the actual work happens
3. **E2e logs are extremely repetitive** — deployment readiness polls
   repeat every 5 seconds for 15+ minutes per test retry. Deduplicate
   mentally: look for the first occurrence of a failure pattern, then
   skip to the next `[FAILED]` or `[PANICKED]` marker. Key signals:
   - `[FAILED]` / `[PANICKED]` markers with file:line references
   - `Unexpected error:` blocks with the actual error message
   - Transitions like `node.kubernetes.io/unreachable` or
     `no nodes available to schedule pods` (infra failure)
   - `make: *** ... Terminated` (task was killed by timeout)
   - Ginkgo test summary: `X failed, Y passed, Z skipped`
4. **For scan failures**, look for the `TEST_OUTPUT` JSON:
   ```json
   {"result":"SUCCESS","note":"Task ... success: No finding was detected",...}
   ```
   or:
   ```json
   {"result":"FAILURE","note":"Task ... failure: <N> findings detected",...}
   ```

## Known CI jobs for this repo (lightspeed-operator)

Reference PR: [openshift/lightspeed-operator#1641](https://github.com/openshift/lightspeed-operator/pull/1641) (all checks passing).

### Local commands (map to Prow where noted)

| Command | Used by |
|---|---|
| `make test` | **`ci/prow/unit`** (envtest + CRDs; **do not** use bare `go test`) |
| `make generate` + clean git tree | **`ci/prow/generate`** |
| `make lint` | (not always a separate Prow job on every PR) |
| `make test-e2e` | Konflux `operator-e2e-tests-*` / Prow `bundle-e2e-*` (cluster; Ginkgo in `test/e2e/`) |

### Prow (`ci/prow/*`) — commit statuses

Config is in **openshift/release** (`jobs/openshift/lightspeed-operator/`). Typical PR jobs:

| Context | What it does |
|---|---|
| `ci/prow/unit` | `make test` — Go unit tests (Ginkgo/envtest) |
| `ci/prow/generate` | Generated code / manifests must match `make generate` |
| `ci/prow/images` | Build operator (and related) container images |
| `ci/prow/security` | Go security scan |
| `ci/prow/fips-image-scan-operator` | FIPS compliance scan on operator image |
| `ci/prow/ci-index-lightspeed-bundle-test` | OLM bundle index validation |
| `ci/prow/bundle-e2e-4-XX` | Cluster e2e against OLS bundle on OCP 4.XX (GCS artifacts; full stack) |
| `tide` | **Not a test** — merge gate (labels, approvals, rebase). Shows `pending` until merge-ready |

**Not** used on operator (lightspeed-service skill lists these): `ci/prow/integration`, `ci/prow/verify` (pytest/black), `ci/prow/e2e-ols-cluster` as the only e2e path.

Prow job URL pattern:
`https://prow.ci.openshift.org/view/gs/test-platform-results/pr-logs/pull/openshift_lightspeed-operator/{pr}/pull-ci-openshift-lightspeed-operator-main-{job}/…`

Use §3–4 (GCS artifacts) for failed Prow jobs. `bundle-e2e-*` logs include operator + app-server pod logs under e2e step artifacts.

### Konflux (`Red Hat Konflux / …`) — check runs

**Build pipelines** (`.tekton/*-pull-request.yaml`):

| Check name (GitHub) | Triggers when | Output |
|---|---|---|
| `Red Hat Konflux / lightspeed-operator-on-pull-request` | `Dockerfile`, `cmd/**`, `api/**`, `internal/**`, `test/**`, … | `…/ols/lightspeed-operator:on-pr-{sha}` |
| `Red Hat Konflux / ols-bundle-on-pull-request` | `bundle/**`, `bundle.Dockerfile`, **`related_images.json`** | `…/ols-bundle:on-pr-{sha}` |
| `Red Hat Konflux / fbc-v4-XX-on-pull-request` | `lightspeed-catalog-4.XX/**` | FBC catalog image |
| `Red Hat Konflux / auto-labeling-pull-request` | Auto-labeling pipeline | Labels only |

**Integration tests** (snapshots use `ols-bundle` component when bundle built):

| Check name (examples from #1641) | Pipeline file | Notes |
|---|---|---|
| `… / operator-e2e-tests-419 / ols-bundle` | `lightspeed-operator-e2e-test-pipeline-419.yaml` | Operator e2e from **this repo** |
| `… / upgrade-e2e-tests / ols-bundle` | `lightspeed-operator-upgrade-e2e-test-pipeline-*.yaml` | Upgrade path |
| `… / service-e2e-tests-419 / ols-bundle` | `lightspeed-service-integration-test-pipeline-4.19.yaml` | **lightspeed-service** at `related_images.json` revision |
| `… / console-tests-pf5 / ols-bundle` | `lightspeed-console-e2e-test-pipeline-pf5.yaml` | Console plugin Cypress |
| `… / console-tests-pf6 / ols-bundle` | `lightspeed-console-e2e-test-pipeline-pf6.yaml` | Console plugin PF6 |

**Policy / EC:**

| Check | Notes |
|---|---|
| `… / ols-bundle-enterprise-contract / ols-bundle` | EC on bundle component |
| `… / ols-enterprise-contract / lightspeed-operator` | EC on operator (may show `skipping` on PRs) |

Integration flow: `ols-install` → `install-operator` → e2e. Konflux failures: use Konflux section (K1–K8); artifacts on Quay at `quay.io/openshift-lightspeed/ols-operator-artifacts:{head_sha}`.

### Triage order on a failed PR

1. `gh pr checks {pr} --repo openshift/lightspeed-operator` — separate **Prow** vs **Konflux** failures
2. **Prow** → GCS `build-log.txt` (§4)
3. **Konflux** → check run summary → `oras` on Quay (K6) → UI logs (K7)
4. Match failure to changed files (`gh pr diff --name-only`)

Optional jobs and minor versions (4.16–4.22) vary by PR; the table above is illustrative, not exhaustive.

## Skill validation (smoke test)

Run against a known PR (e.g. [#1641](https://github.com/openshift/lightspeed-operator/pull/1641)) to verify `gh` / GCS / Quay paths still work:

```bash
PR=1641 REPO=openshift/lightspeed-operator
SHA=$(gh api repos/$REPO/pulls/$PR --jq .head.sha)
echo "SHA=$SHA"

# Step 1 — both CI systems present
gh pr checks $PR --repo $REPO | grep -E 'ci/prow/|Konflux|tide'

# Prow — unit finished.json + build-log snippet
BASE="https://storage.googleapis.com/test-platform-results/pr-logs/pull/openshift_lightspeed-operator/$PR"
UNIT_ID=$(gh pr checks $PR --repo $REPO 2>/dev/null | awk '/ci\/prow\/unit/{print $NF}' | head -1)
# Or take build_id from prow target_url in statuses API
curl -sf "$BASE/pull-ci-openshift-lightspeed-operator-main-unit/*/finished.json" 2>/dev/null | head -1
# Prefer explicit build_id from gh pr checks URL tail

# Konflux — build check has Task Statuses in .text
BUILD_ID=$(gh api "repos/$REPO/commits/$SHA/check-runs" \
  --jq '.check_runs[] | select(.name | test("ols-bundle-on-pull-request")) | .id')
gh api "repos/$REPO/check-runs/$BUILD_ID" --jq '.output.text' | grep -q "Task Statuses" && echo "build task table OK"

# Konflux — integration check has pipelinerun + task rows in .text
E2E_ID=$(gh api "repos/$REPO/commits/$SHA/check-runs" \
  --jq '.check_runs[] | select(.name | test("operator-e2e-tests")) | .id')
gh api "repos/$REPO/check-runs/$E2E_ID" --jq '.output.text' | grep -q "ols-operator-tests" && echo "e2e task table OK"

# Quay — e2e artifacts by commit sha (longer-lived than on-pr tags)
oras discover "quay.io/openshift-lightspeed/ols-operator-artifacts:$SHA" | head -3
```

Expected on a green PR: ~8 `ci/prow/*` pass, multiple Konflux pass, `tide` pending until lgtm/approve; `oras discover` returns a manifest digest.

## Tool usage notes

- **`gh`** — all GitHub API calls (PR metadata, statuses, checks, comments, files).
- **`oras`** — fetch Konflux scan results and artifacts from Quay (OCI attachments). Required for K6.
- **`WebFetch`** — Prow artifacts from GCS (`gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/...` for browsing, `storage.googleapis.com/test-platform-results/...` for raw content). The Prow dashboard URL itself is JS-rendered and not useful via WebFetch — always use GCS URLs.
- **Prow** failures appear as **commit statuses** (`gh api repos/.../statuses/{sha}`). **Konflux** failures appear as **check runs** (`gh api repos/.../check-runs/{id}`). Use both APIs when listing failures.
- **Konflux triage order**: (1) `gh api` check run **`output.text`** for task table + UI log links, (2) `oras` on `ols-operator-artifacts:{sha}` or `on-pr-{sha}` scan attachments, (3) Konflux UI logs (K7) if Quay/GCS insufficient.
- Build logs can be very large. When fetched via WebFetch, they're saved to a temp file — read from the end to find failures quickly.
