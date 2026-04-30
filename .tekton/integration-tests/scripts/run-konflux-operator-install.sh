#!/usr/bin/env bash
# Konflux: install operator onto the ephemeral cluster after Tekton has cloned
# lightspeed-operator at the snapshot commit (cwd = repo root).
#
# Usage:
#   ./.tekton/integration-tests/scripts/run-konflux-operator-install.sh bundle
#   ./.tekton/integration-tests/scripts/run-konflux-operator-install.sh direct
#
# Required env:
#   SNAPSHOT               — Konflux snapshot JSON
#   KONFLUX_COMPONENT_NAME — component under test (bundle image selector for bundle mode)
#   OLS_NAMESPACE          — target namespace (e.g. openshift-lightspeed)
#   KUBECONFIG             — standard kubeconfig path
#
# Optional (direct mode — hack/install/install-operator-direct.sh):
#   KONFLUX_OPERATOR_IMAGE_COMPONENT — snapshot component name for manager IMG (default: lightspeed-operator)
#
# Optional (bundle mode — hack/install/install-operator-bundle.sh):
#   OPERATOR_SDK_VERSION
#   PRE_BUNDLE_IMAGE             — two-step install (upgrade pipelines)
#   UPGRADE_E2E_INSTALL_OLD_BASE_FROM_CATALOG — e.g. "4.19."; sets PRE_BUNDLE_IMAGE from the second-newest
#                                 semver (latest minus one) in lightspeed-catalog-<minor>/bundle-v*.yaml and
#                                 SKIP_FINAL_BUNDLE_INSTALL (avoids very old bases with known bad upgrade chains)
#   IMAGE_DIGEST_MIRROR_SET_URL — apply ImageDigestMirrorSet before bundle (e.g. Rapidast)

set -euo pipefail

INSTALL_MODE="${1:?usage: $0 <bundle|direct>}"

: "${SNAPSHOT:?SNAPSHOT must be set}"
: "${KONFLUX_COMPONENT_NAME:?KONFLUX_COMPONENT_NAME must be set}"
: "${OLS_NAMESPACE:?OLS_NAMESPACE must be set}"
: "${KUBECONFIG:?KUBECONFIG must be set}"

export OPERATOR_SDK_VERSION="${OPERATOR_SDK_VERSION:-1.36.1}"

install_bundle() {
	if [[ -n "${UPGRADE_E2E_INSTALL_OLD_BASE_FROM_CATALOG:-}" ]]; then
		PV="${UPGRADE_E2E_INSTALL_OLD_BASE_FROM_CATALOG%.}"
		catalog_dir="lightspeed-catalog-${PV}"
		if [[ ! -d "${catalog_dir}" ]]; then
			echo "error: catalog dir not found: ${catalog_dir} (cwd: $(pwd))" >&2
			exit 1
		fi
		shopt -s nullglob
		_bundle_files=("${catalog_dir}"/bundle-v*.yaml)
		shopt -u nullglob
		if [[ "${#_bundle_files[@]}" -lt 2 ]]; then
			echo "error: need at least 2 bundle-v*.yaml in ${catalog_dir} for upgrade base (second-newest / latest-1 semver)" >&2
			exit 1
		fi
		base_ver="$(
			printf '%s\n' "${_bundle_files[@]}" |
				sed -n 's/.*bundle-v\(.*\)\.yaml/\1/p' |
				sort -V |
				tail -n2 |
				head -n1
		)"
		base_file="${catalog_dir}/bundle-v${base_ver}.yaml"
		if [[ ! -f "${base_file}" ]]; then
			echo "error: could not resolve base bundle file ${base_file} in ${catalog_dir}" >&2
			exit 1
		fi
		PRE_BUNDLE_IMAGE="$(yq '.relatedImages[] | select(.name == "lightspeed-operator-bundle") | .image' "${base_file}")"
		export PRE_BUNDLE_IMAGE
		export SKIP_FINAL_BUNDLE_INSTALL=true
		echo "Upgrade e2e base install: second-newest catalog bundle (latest-1) ${base_file}"
		echo "PRE_BUNDLE_IMAGE=${PRE_BUNDLE_IMAGE}"
	fi

	echo "${KONFLUX_COMPONENT_NAME}"
	export BUNDLE_IMAGE="$(
		jq -r --arg component_name "${KONFLUX_COMPONENT_NAME}" \
			'.components[] | select(.name == $component_name) | .containerImage' <<<"${SNAPSHOT}"
	)"
	echo "${BUNDLE_IMAGE}"
	if [[ -n "${PRE_BUNDLE_IMAGE:-}" ]]; then
		echo "Upgrade path: PRE_BUNDLE_IMAGE=${PRE_BUNDLE_IMAGE} -> BUNDLE_IMAGE=${BUNDLE_IMAGE}"
	fi
	if [[ -n "${IMAGE_DIGEST_MIRROR_SET_URL:-}" ]]; then
		echo "IMAGE_DIGEST_MIRROR_SET_URL=${IMAGE_DIGEST_MIRROR_SET_URL}"
	fi
	echo "---------------------------------------------"
	./hack/install/install-operator-bundle.sh
	echo "---------------------------------------------"
	verify_operator_deployment
}

install_direct() {
	local img_component="${KONFLUX_OPERATOR_IMAGE_COMPONENT:-lightspeed-operator}"
	export IMG="$(
		jq -r --arg n "${img_component}" \
			'.components[] | select(.name == $n) | .containerImage' <<<"${SNAPSHOT}"
	)"
	if [[ -z "${IMG}" || "${IMG}" == "null" ]]; then
		echo "error: direct install: no containerImage for snapshot component \"${img_component}\" (set KONFLUX_OPERATOR_IMAGE_COMPONENT or ensure SNAPSHOT lists lightspeed-operator)" >&2
		exit 1
	fi
	echo "Direct install: IMG from snapshot component \"${img_component}\": ${IMG}"
	export SKIP_IDMS=1
	echo "---------------------------------------------"
	./hack/install/install-operator-direct.sh
	echo "---------------------------------------------"
	verify_operator_deployment
}

verify_operator_deployment() {
	echo "Verifying lightspeed-operator-controller-manager in namespace ${OLS_NAMESPACE}..."
	if ! oc get deployment lightspeed-operator-controller-manager -n "${OLS_NAMESPACE}"; then
		echo "error: operator deployment verification failed (not found or unreachable) in ${OLS_NAMESPACE}" >&2
		exit 1
	fi
	echo "OK: operator deployment is present in ${OLS_NAMESPACE}."
}

case "${INSTALL_MODE}" in
bundle) install_bundle ;;
direct) install_direct ;;
*)
	echo "error: unknown install mode: ${INSTALL_MODE} (expected bundle or direct)" >&2
	exit 1
	;;
esac
