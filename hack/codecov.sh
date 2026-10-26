#!/usr/bin/env bash
# Upload a Go coverage profile to codecov.io from prow CI.
#
# Usage: hack/codecov.sh <report-file> [flag]
#
# The upload SOFT-FAILS by design: codecov problems must never block CI, so
# this script always exits 0. The upload state is always clearly logged:
#   - "[codecov] upload succeeded"              report is on codecov.io
#   - "[codecov] CODECOV UPLOAD FAILED ..."     upload was attempted and failed
#   - "[codecov] upload skipped: <reason>"      upload was not attempted
#
# Environment (set by prow):
#   JOB_TYPE                    presubmit | postsubmit | batch | local
#   PULL_NUMBER                 PR number (presubmit only)
#   PULL_PULL_SHA               PR head commit SHA (presubmit only)
#   PULL_BASE_SHA / PULL_BASE_REF  base commit SHA and branch
#   REPO_OWNER / REPO_NAME      repository slug parts
#   CODECOV_TOKEN               repository upload token (prow secret)
#   ARTIFACT_DIR                writable directory for the upload log
#
# The codecov CLI is a static binary downloaded from GitHub releases because
# the CI build root is a pure Go image without Python.

set -o nounset
set -o pipefail

CODECOV_CLI_VERSION="11.3.1"
MAX_ATTEMPTS="${CODECOV_MAX_ATTEMPTS:-2}"
RETRY_SLEEP_SECONDS="${CODECOV_RETRY_SLEEP_SECONDS:-15}"
UPLOAD_TIMEOUT_SECONDS="${CODECOV_TIMEOUT_SECONDS:-300}"

log() {
    echo "[codecov] $*"
}

report_not_uploaded() {
    log "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"
    log "!! CODECOV UPLOAD FAILED - coverage was NOT uploaded to codecov.io"
    log "!! CI is not blocked (soft-fail by design), but coverage data is"
    log "!! missing and codecov.io / dashboards will not update."
    log "!! Reason: $*"
    log "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"
    exit 0
}

skip_upload() {
    log "upload skipped: $* (coverage was NOT uploaded to codecov.io)"
    exit 0
}

report_file="${1:-}"
flag="${2:-}"

if [ -z "$report_file" ]; then
    skip_upload "no report file given (usage: codecov.sh <report-file> [flag])"
fi
if [ ! -f "$report_file" ]; then
    report_not_uploaded "report file '$report_file' does not exist"
fi

case "${JOB_TYPE:-local}" in
    presubmit)
        sha="${PULL_PULL_SHA:-}"
        branch="${PULL_BASE_REF:-}"
        pr="${PULL_NUMBER:-}"
        ;;
    postsubmit | batch)
        sha="${PULL_BASE_SHA:-}"
        branch="${PULL_BASE_REF:-}"
        pr=""
        ;;
    *)
        skip_upload "JOB_TYPE '${JOB_TYPE:-local}' does not upload (local run)"
        ;;
esac

if [ -z "${CODECOV_TOKEN:-}" ]; then
    report_not_uploaded "CODECOV_TOKEN is not set (check the CI secret)"
fi
if [ -z "$sha" ]; then
    report_not_uploaded "commit SHA is not set"
fi
if [ -z "$branch" ]; then
    report_not_uploaded "branch is not set"
fi

cli_args=(
    upload-process
    --git-service github
    --slug "${REPO_OWNER:-openshift}/${REPO_NAME:-lightspeed-operator}"
    --token "$CODECOV_TOKEN"
    --file "$report_file"
    --sha "$sha"
    --branch "$branch"
    --fail-on-error
)
if [ -n "$flag" ]; then
    cli_args+=(--flag "$flag")
fi
if [ -n "$pr" ]; then
    cli_args+=(--pr "$pr")
fi

resolve_cli() {
    if command -v codecovcli > /dev/null 2>&1; then
        CLI=(codecovcli)
        return 0
    fi
    case "$(uname -m)" in
        x86_64) asset="codecovcli_linux" ;;
        aarch64 | arm64) asset="codecovcli_linux_arm64" ;;
        *)
            report_not_uploaded "unsupported architecture $(uname -m) for the codecov CLI"
            ;;
    esac
    local cli_bin
    cli_bin="${TMPDIR:-/tmp}/codecovcli-${CODECOV_CLI_VERSION}"
    if [ ! -x "$cli_bin" ]; then
        log "codecovcli not found on PATH, downloading ${asset} v${CODECOV_CLI_VERSION}"
        if ! curl -fsSL --retry 3 -o "$cli_bin" \
            "https://github.com/codecov/codecov-cli/releases/download/v${CODECOV_CLI_VERSION}/${asset}"; then
            report_not_uploaded "failed to download the codecov CLI"
        fi
        chmod +x "$cli_bin"
    fi
    CLI=("$cli_bin")
}

upload_log="${ARTIFACT_DIR:-.}/codecov_upload.log"
pr_note=""
if [ -n "$pr" ]; then
    pr_note=", pr: ${pr}"
fi

resolve_cli

attempt=1
while [ "$attempt" -le "$MAX_ATTEMPTS" ]; do
    log "upload attempt ${attempt}/${MAX_ATTEMPTS} (flag: ${flag:-none}, branch: ${branch}${pr_note})"
    if timeout "$UPLOAD_TIMEOUT_SECONDS" "${CLI[@]}" "${cli_args[@]}" > "$upload_log" 2>&1; then
        cat "$upload_log"
        log "upload succeeded - report available on codecov.io"
        exit 0
    fi
    cat "$upload_log"
    log "upload attempt ${attempt}/${MAX_ATTEMPTS} failed"
    attempt=$((attempt + 1))
    if [ "$attempt" -le "$MAX_ATTEMPTS" ]; then
        sleep "$RETRY_SLEEP_SECONDS"
    fi
done

report_not_uploaded "codecov CLI exited non-zero after ${MAX_ATTEMPTS} attempt(s), see ${upload_log} above"
