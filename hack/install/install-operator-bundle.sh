#!/usr/bin/env bash
# Install the operator via OLM using `operator-sdk run bundle` (Konflux-style).
#
# Required env:
#   BUNDLE_IMAGE   (e.g. quay.io/.../lightspeed-operator-bundle@sha256:...)
#
# Optional env:
#   OLS_NAMESPACE        (default: openshift-lightspeed)
#   OPERATOR_SDK_VERSION (default: 1.36.1)
#   OPERATOR_SDK_BIN     (default: /usr/local/bin/operator-sdk when downloading)
#   BUNDLE_TIMEOUT       (default: 30m) — max time for operator-sdk to wait; OLM still fails the CSV
#                          after ~5m if the manager Deployment never becomes Available (pull/auth/probes).
#   PRE_BUNDLE_IMAGE       If set, `operator-sdk run bundle` is run with this image first,
#                          then again with BUNDLE_IMAGE (upgrade bootstrap / two-step install).
#   SKIP_FINAL_BUNDLE_INSTALL  If non-empty, after PRE_BUNDLE_IMAGE (required), skip the second
#                          `run bundle` with BUNDLE_IMAGE. Used for Konflux upgrade e2e: leave the
#                          cluster on an older catalog bundle, then tests run `bundle-upgrade` to
#                          the snapshot bundle.
#   IMAGE_DIGEST_MIRROR_SET_URL  If set, fetched with curl and applied via `oc apply` before bundle install
#                          (same behavior as Rapidast pipeline; optional elsewhere).
#
# operator-sdk is downloaded if not already on PATH (same as .tekton integration pipelines).
#
# This script does NOT clone git. Pipeline: clone/checkout → ./hack/install/install-operator-bundle.sh → tests
#
# shellcheck disable=SC1091
set -euo pipefail

usage() {
	sed -n '2,/^$/p' "$0" | tail -n +1
	exit "${1:-0}"
}

[[ "${1:-}" == "-h" || "${1:-}" == "--help" ]] && usage 0

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=./_lib.sh
source "${SCRIPT_DIR}/_lib.sh"

if [[ -z "${BUNDLE_IMAGE:-}" ]]; then
	echo "error: BUNDLE_IMAGE must be set" >&2
	usage 1
fi

install::ensure_oc_kubectl
install::ensure_operator_sdk

ns="$(install::default_namespace)"
install::ensure_namespace "$ns"

# Optional cluster pull-through (Rapidast / restricted registries); same as legacy Tekton inline step.
if [[ -n "${IMAGE_DIGEST_MIRROR_SET_URL:-}" ]]; then
	tmp=$(mktemp)
	curl -fsSL -o "$tmp" "${IMAGE_DIGEST_MIRROR_SET_URL}"
	oc apply -f "$tmp"
	rm -f "$tmp"
fi

timeout="${BUNDLE_TIMEOUT:-30m}"

run_bundle() {
	local image="$1"
	echo "operator-sdk run bundle --timeout=${timeout} --namespace ${ns} ${image}"
	if ! operator-sdk run bundle --timeout="${timeout}" --namespace "${ns}" "${image}" --verbose; then
		install::olm_failure_diagnostics "${ns}"
		return 1
	fi
}

if [[ -n "${PRE_BUNDLE_IMAGE:-}" ]]; then
	echo "Installing base bundle: ${PRE_BUNDLE_IMAGE}"
	run_bundle "${PRE_BUNDLE_IMAGE}"
fi

if [[ -n "${SKIP_FINAL_BUNDLE_INSTALL:-}" ]]; then
	if [[ -z "${PRE_BUNDLE_IMAGE:-}" ]]; then
		echo "error: SKIP_FINAL_BUNDLE_INSTALL requires PRE_BUNDLE_IMAGE" >&2
		exit 1
	fi
	echo "SKIP_FINAL_BUNDLE_INSTALL set; skipping second install (BUNDLE_IMAGE=${BUNDLE_IMAGE})"
	echo "Done. Operator deployment:"
	oc get deployment lightspeed-operator-controller-manager -n "$ns" -o wide || true
	exit 0
fi

echo "Installing bundle: ${BUNDLE_IMAGE}"
run_bundle "${BUNDLE_IMAGE}"

echo "Done. Operator deployment:"
oc get deployment lightspeed-operator-controller-manager -n "$ns" -o wide || true
