#!/usr/bin/env bash
# Install the operator from this repo using kustomize (same path as `make install` + `make deploy`).
# Intended for Konflux / CI and for developers who want in-cluster manager instead of `make run`.
#
# Prerequisites: oc, make, kustomize/jq via Makefile bootstrap, repo root with related_images.json.
# Required env: IMG (operator image to run, e.g. quay.io/.../lightspeed-operator@sha256:...)
#
# Optional env:
#   OLS_NAMESPACE   (default: openshift-lightspeed)
#   KUBECONFIG      (standard kubeconfig path)
#   KUBECTL         (default: kubectl)
#   SKIP_IDMS=1     On Hypershift / HostedCluster, admission often blocks applying ImageDigestMirrorSet
#                   from this manifest stream. When set, the script applies the same resources as
#                   `make deploy` except ImageDigestMirrorSet (kustomize output filtered with yq).
#                   Normal clusters: omit (default) and use `make deploy` unchanged.
#
# This script does NOT clone git; run from a checked-out repo (Tekton checks out first).
# Pipeline layout: clone/checkout → ./hack/install/install-operator-direct.sh → make test-e2e
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

if [[ -z "${IMG:-}" ]]; then
	echo "error: IMG must be set to the operator image (registry/image@digest or :tag)" >&2
	usage 1
fi

install::require_cmd make
install::ensure_oc_kubectl

ns="$(install::default_namespace)"
install::ensure_namespace "$ns"

REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "$REPO_ROOT"

echo "Applying CRDs (make install)..."
make install

# Same substitution logic as Makefile `deploy` (keep in sync); apply step varies when SKIP_IDMS=1.
# Mutates only a temp copy of config/ so the repo's deployment-patch.yaml is never touched.
direct_deploy_skip_idms() {
	local localbin="${REPO_ROOT}/bin"
	# IMG is validated non-empty at script entry
	local operator_img="$IMG"
	local kustomize="${localbin}/kustomize"
	local jqbin="${localbin}/jq"
	local kubectlbin="${KUBECTL:-kubectl}"
	local yq_cmd

	[[ -f "${REPO_ROOT}/related_images.json" ]] || {
		echo "error: related_images.json not found" >&2
		exit 1
	}
	[[ -f "${REPO_ROOT}/hack/image_placeholders.json" ]] || {
		echo "error: hack/image_placeholders.json not found" >&2
		exit 1
	}
	[[ -f "${REPO_ROOT}/config/default/deployment-patch.yaml" ]] || {
		echo "error: ${REPO_ROOT}/config/default/deployment-patch.yaml not found" >&2
		exit 1
	}

	yq_cmd=$(command -v yq 2>/dev/null || true)
	if [[ -z "$yq_cmd" && -x "${localbin}/yq" ]]; then
		yq_cmd="${localbin}/yq"
	fi
	if [[ -z "$yq_cmd" ]]; then
		echo "SKIP_IDMS=1 requires yq; running: make yq"
		make -s yq
		yq_cmd=$(command -v yq 2>/dev/null || echo "${localbin}/yq")
	fi
	if ! "$yq_cmd" --version >/dev/null 2>&1; then
		echo "error: yq not working at ${yq_cmd} (install yq or ensure make yq succeeds)" >&2
		exit 1
	fi

	(
		tmpcfg=$(mktemp -d)
		trap 'rm -rf "${tmpcfg}"' EXIT
		mkdir -p "${tmpcfg}/cfg"
		cp -a "${REPO_ROOT}/config/." "${tmpcfg}/cfg/"
		patch_file="${tmpcfg}/cfg/default/deployment-patch.yaml"

		sed -i "s|__REPLACE_LIGHTSPEED_OPERATOR__|${operator_img}|g" "$patch_file"
		sed -i "/path: \/spec\/template\/spec\/containers\/0\/image/{n;s|value: .*|value: ${operator_img}|}" "$patch_file"
		while IFS='|' read -r name placeholder; do
			if [[ "$name" != "lightspeed-operator" ]]; then
				img=$("${jqbin}" -r --arg n "$name" '.[] | select(.name==$n) | .image' "${REPO_ROOT}/related_images.json")
				if [[ -n "$img" && "$img" != "null" ]]; then
					sed -i "s|${placeholder}|${img}|g" "$patch_file"
				fi
			fi
		done < <("${jqbin}" -r '.[] | "\(.name)|\(.placeholder)"' "${REPO_ROOT}/hack/image_placeholders.json")

		cd "${tmpcfg}/cfg/default"
		"${kustomize}" build . | "${yq_cmd}" ea 'select(.kind != "ImageDigestMirrorSet")' - | "${kubectlbin}" apply -f -
	)
}

if [[ "${SKIP_IDMS:-}" == "1" ]]; then
	echo "Deploying operator manager (SKIP_IDMS=1: same as make deploy but omit ImageDigestMirrorSet; IMG=${IMG})..."
	make manifests kustomize jq
	direct_deploy_skip_idms
else
	echo "Deploying operator manager (make deploy IMG=${IMG})..."
	make deploy IMG="${IMG}"
fi

if git rev-parse --git-dir >/dev/null 2>&1; then
	git checkout -- config/default/deployment-patch.yaml 2>/dev/null || true
fi

echo "Done. Operator deployment:"
oc get deployment lightspeed-operator-controller-manager -n "$ns" -o wide || true
