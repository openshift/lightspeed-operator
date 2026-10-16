# Per-run test artifact publication

The operator (bundle and direct), all-features, upgrade, and service test pipelines
use this repository-owned StepAction at the tested commit. The upstream publisher
pulls an existing tag into the current workspace before uploading. That can
replace fresh logs with older files, and concurrent updates to a shared tag can
lose an entire run's artifacts.

## Artifact tags

Tags are now `<PipelineRun name>-<optional version prefix><commit>`:

```text
ols-operator-artifacts:bundle-e2e-tests-dxf7t-41946343f23ae122c53447408ae8976b78c1b52dab7
ols-operator-artifacts:operator-e2e-tests-direct-6fx8x-41946343f23ae122c53447408ae8976b78c1b52dab7
```

The PipelineRun name distinguishes the test scenario and each retry. The publisher
never downloads old artifacts. Consumers looking up artifacts by commit alone
must instead select the run-specific tag. OCI tags are limited to 128 characters;
normal generated PipelineRun names (at most 63 characters), a three-character
version prefix, and a 40-character commit fit within this limit. Longer custom
names are rejected rather than silently truncated into potentially colliding tags.

Retained Tekton log archives are separate: `export-logs-for-retention` continues to
use `<PipelineRun name>-<timestamp>-logs` without changes.

## Bounds and diagnostics

- Secret scan (including file removal): **5 minutes**, with pipefail enabled.
- Upload: **3 attempts**, **2 minutes each**, with 5 seconds between attempts.
- Timed-out processes receive a kill signal after an additional 10 seconds.
- Tekton `push-artifacts` step: **15 minutes** as an outer bound.
- Timestamped `[scan]` and `[upload]` messages report start, completion, elapsed
  time, attempt number, and failure exit code. On scan failure, fixed operational
  markers distinguish a started pattern fetch from one with a completion marker;
  no raw scanner text is printed. GNU timeout normally returns 124 for a timeout,
  or 137 if a forced kill was required.
- Scanner output may contain secrets. It is captured outside the artifact
  directory, never printed to public logs or uploaded, and deleted on exit.
- A failed scanner, failed removal command, or exhausted upload retries returns
  nonzero. The pipelines retain `onError: continue` and their final
  `fail-if-any-step-failed` check, allowing diagnostics/cleanup without masking
  publication failures.

## Local regression tests

Requires Python 3, PyYAML, Bash, jq, and GNU coreutils:

```sh
python3 .tekton/integration-tests/scripts/test_artifact_publication.py -v
```

The tests execute the actual StepAction script with isolated stand-ins for the
external scanner/remover/ORAS commands. They cover all four pipeline references,
run-specific tags, scan/removal failures, scan/upload timeouts, bounded retries,
secret-safe pattern-fetch diagnostics, tag length limits, and publication without
an old-artifact download. They do not
contact a registry or require an OpenShift cluster.
