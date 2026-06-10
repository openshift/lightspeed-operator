---
name: investigate-pr-failures
description: >-
  Investigates a GitHub PR's failing CI checks using git (same conventions as
  review-pr: upstream, upstream/main) and curl against the GitHub REST API for
  check runs and Actions job logs. Use when the user gives a PR number, mentions
  failing checks, red CI, or asks to debug workflow or test failures for this
  repository.
---

# Investigate PR failures

When asked about why a test or verification failed in a PR, follow this structured approach.

This repository is **public** on GitHub. Prefer **unauthenticated** `curl` first; add `GITHUB_TOKEN` or `GH_TOKEN` only if you hit rate limits (HTTP 403 with `rate_limit`) or an endpoint refuses anonymous access.

**Never** print a token, log it, or paste it into chat. Redact obvious secrets when quoting log lines.

## Preconditions

- **Shell**: `git`, `curl`, and `python3` (for JSON parsing if `jq` is missing).
- **Workspace**: Run from the **lightspeed-operator** repo root.

## Git conventions (match [review-pr](../review-pr/SKILL.md))

Use the **`upstream`** remote for PR refs and diffs, same as review-pr. If `upstream` is not configured, use **`origin`** for both fetch and API `OWNER/REPO` resolution—but PR numbers are scoped to one repo; if `origin` is a fork, configure `upstream` to the canonical repo before treating PR `N` as the team’s PR `N`.

## 1. Fetch Latest Changes

**Always** fetch the latest PR state before investigating (stale local refs miss new failures).

```bash
git fetch upstream pull/<PR_NUMBER>/head:pr-<PR_NUMBER>
git log pr-<PR_NUMBER> --oneline -10
git diff upstream/main...pr-<PR_NUMBER> --stat
```

Re-fetch after new pushes:

```bash
git fetch upstream pull/<PR_NUMBER>/head:pr-<PR_NUMBER> --force
```

## 2. Resolve OWNER/REPO for the API

Use the **same** remote as for `git fetch` (`upstream`, or `origin` if no upstream):

```bash
REMOTE=upstream
python3 -c "
import subprocess, re, sys
r = subprocess.check_output(['git', 'remote', 'get-url', sys.argv[1]], text=True).strip()
m = re.search(r'github\.com[:/]([^/]+)/([^/.]+)', r)
assert m, 'could not parse owner/repo from remote URL'
print(f'{m.group(1)}/{m.group(2)}')
" "$REMOTE"
```

Use the printed `OWNER/REPO` in API URLs below.

## 3. Head SHA for the PR (checks attach to this commit)

Replace `OWNER`, `REPO`, `PR_NUMBER`:

```bash
curl_api "https://api.github.com/repos/OWNER/REPO/pulls/PR_NUMBER" \
| python3 -c "import json,sys; d=json.load(sys.stdin); print(d['head']['sha'])"
```

Use that value as `SHA` below. If this returns 404, `OWNER/REPO` or `PR_NUMBER` is wrong for the remote you chose.

## 4. List failing check runs

```bash
curl_api "https://api.github.com/repos/OWNER/REPO/commits/${SHA}/check-runs?per_page=100" \
| python3 -c "
import json, sys
data = json.load(sys.stdin)
for r in data.get('check_runs', []):
    name, status, conclusion = r.get('name'), r.get('status'), r.get('conclusion')
    if status == 'completed' and conclusion not in ('success', 'skipped', 'neutral'):
        print(conclusion or 'unknown', name, r.get('html_url',''))
"
```

Treat `failure`, `cancelled`, `timed_out`, and `action_required` as worth investigating.

## 5. Actions: runs for that commit, then failed job logs

```bash
curl_api "https://api.github.com/repos/OWNER/REPO/actions/runs?head_sha=${SHA}&per_page=30" \
| python3 -c "import json,sys; d=json.load(sys.stdin);
[print(r['id'], r.get('conclusion'), r.get('name','')) for r in d.get('workflow_runs',[])]"
```

For each non-success run, list jobs:

```bash
RUN_ID="<id>"
curl_api "https://api.github.com/repos/OWNER/REPO/actions/runs/${RUN_ID}/jobs" \
| python3 -c "import json,sys; d=json.load(sys.stdin);
[print(j['id'], j.get('conclusion'), j.get('name','')) for j in d.get('jobs',[])]"
```

Download a **failed** job log (`-L` follows redirects; response is plain text):

```bash
JOB_ID="<id>"
curl -sSL -H "Accept: application/vnd.github+json" \
  ${TOKEN:+-H "Authorization: Bearer $TOKEN"} \
  "https://api.github.com/repos/OWNER/REPO/actions/jobs/${JOB_ID}/logs" | head -n 400
```

Increase or drop `head` for more context; for huge logs use `rg 'FAIL|panic|Error:|--- FAIL'` on saved output.

**Non-GitHub Actions checks**: use the check run's `html_url`; git + API steps still give the correct SHA and local diff.

## 6. OpenShift CI logs (non-Konflux checks)

For checks that are **NOT** prefixed with "Red Hat Konflux", the check run's `html_url` typically ends with a numeric job ID (e.g., `2046991349567197184`). This ID is crucial for accessing detailed test artifacts.

### Constructing the artifacts URL

Extract the job ID from the check run's `html_url` (the number at the end), then construct the gcsweb artifacts URL:

```
https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/test-platform-results/pr-logs/pull/OWNER_REPO/PR_NUMBER/JOB_NAME/JOB_ID/
```

**Example:**
```
https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/test-platform-results/pr-logs/pull/openshift_lightspeed-operator/1431/pull-ci-openshift-lightspeed-operator-main-bundle-e2e-4-21/2046991349567197184/
```

### Accessing test artifacts

Navigate to the artifacts subdirectories to find test-specific logs organized by test case:

```
<BASE_URL>/artifacts/<TEST_SUITE>/e2e-test/artifacts/openai/
<BASE_URL>/artifacts/<TEST_SUITE>/e2e-test/artifacts/azure_openai/
```

**For the `bundle-e2e-4-21` example above:**
- OpenAI tests: `https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/test-platform-results/pr-logs/pull/openshift_lightspeed-operator/1431/pull-ci-openshift-lightspeed-operator-main-bundle-e2e-4-21/2046991349567197184/artifacts/bundle-e2e-4-21/e2e-test/artifacts/openai/`
- Azure OpenAI tests: `https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/test-platform-results/pr-logs/pull/openshift_lightspeed-operator/1431/pull-ci-openshift-lightspeed-operator-main-bundle-e2e-4-21/2046991349567197184/artifacts/bundle-e2e-4-21/e2e-test/artifacts/azure_openai/`

Each directory contains:
- **Pod logs** (`*.txt`) - Kubernetes pod output for each test
- **Resource manifests** (`*.yaml`) - OLSConfig, Deployments, Services, etc. captured during test execution
- **Test-specific artifacts** - Organized by individual test cases

**These artifacts are crucial for identifying root causes**, especially for environment-specific failures, resource issues, or configuration problems that don't appear in the main job logs.

## 7. Correlate logs with the repository

- Map paths and line numbers from output to files in the workspace (use `pr-<PR_NUMBER>` as the fetched ref).
- For Go in this operator: run **`make test`**, not raw `go test`, when reproducing locally (`AGENTS.md` / `CLAUDE.md`).
- If the failure is environmental (no cluster, e2e-only), state that clearly.

## 7. Report back

1. **PR and SHA** — commit that was red.
2. **Failing checks** — names, conclusions, `html_url` links.
3. **Evidence** — short log excerpts + repo file/line references.
4. **Likely cause** — primary hypothesis tied to evidence.
5. **Next steps** — fix or validation command; note if a flake re-run is plausible.

## Related skills

- [review-pr](../review-pr/SKILL.md) — same `git fetch` / `upstream/main` workflow.
- [go-code-review](../go-code-review/SKILL.md), [go-testing-code-review](../go-testing-code-review/SKILL.md) — after the failure is understood.
