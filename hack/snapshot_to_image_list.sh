#!/usr/bin/env bash
# Refresh related_images.json image/revision fields from Konflux snapshots.
# Component wiring (snapshot name, Konflux quay prefix, product registry prefix, operator_arg) lives in
# related_images.json optional fields; this script only loops those entries and updates image/revision.
set -euo pipefail

usage() {
	echo "Usage: $0 -s <snapshot-ref> [-b <bundle-snapshot-ref>] [-o <output-file>] [-r stable|preview|ci]"
	echo "Example: $0 -s ols-cq8sl -b ols-bundle-wf8st -o related_images.json -r stable"
}

KONFLUX_NAMESPACE="crt-nshift-lightspeed-tenant"
SNAPSHOT_REF=""
BUNDLE_SNAPSHOT_REF=""
OUTPUT_FILE=""
USE_REGISTRY="ci"

while getopts ":s:b:o:r:h" argname; do
	case "$argname" in
	s) SNAPSHOT_REF=${OPTARG} ;;
	b) BUNDLE_SNAPSHOT_REF=${OPTARG} ;;
	o) OUTPUT_FILE=${OPTARG} ;;
	r)
		USE_REGISTRY=${OPTARG}
		if [[ "${USE_REGISTRY}" != "stable" && "${USE_REGISTRY}" != "preview" && "${USE_REGISTRY}" != "ci" ]]; then
			echo "Invalid registry option: ${USE_REGISTRY}. Use 'stable', 'preview', or 'ci'." >&2
			usage
			exit 1
		fi
		;;
	h)
		usage
		exit 0
		;;
	*)
		echo "Unknown option ${OPTARG:-}" >&2
		usage
		exit 1
		;;
	esac
done

if [ -z "${SNAPSHOT_REF}" ]; then
	echo "snapshot-ref is required" >&2
	usage
	exit 1
fi

: "${JQ:=$(command -v jq)}"
if [ -z "${JQ}" ]; then
	echo "jq is required" >&2
	exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEFAULT_INPUT="${SCRIPT_DIR}/../related_images.json"
if [ -n "${OUTPUT_FILE}" ] && [ -f "${OUTPUT_FILE}" ]; then
	INPUT_FILE="${OUTPUT_FILE}"
elif [ -f "${DEFAULT_INPUT}" ]; then
	INPUT_FILE="${DEFAULT_INPUT}"
else
	echo "related images file not found: set -o to an existing file or add ${DEFAULT_INPUT}" >&2
	exit 1
fi

TMP_OLS_SNAPSHOT=$(mktemp)
TMP_BUNDLE_SNAPSHOT=$(mktemp)
cleanup() {
	rm -f "${TMP_OLS_SNAPSHOT}" "${TMP_BUNDLE_SNAPSHOT}"
}
trap cleanup EXIT

if ! oc get -n "${KONFLUX_NAMESPACE}" snapshot "${SNAPSHOT_REF}" -o json >"${TMP_OLS_SNAPSHOT}"; then
	echo "Failed to get snapshot ${SNAPSHOT_REF}" >&2
	echo "Login to Konflux via oc first." >&2
	exit 1
fi

if [ -n "${BUNDLE_SNAPSHOT_REF}" ]; then
	if ! oc get -n "${KONFLUX_NAMESPACE}" snapshot "${BUNDLE_SNAPSHOT_REF}" -o json >"${TMP_BUNDLE_SNAPSHOT}"; then
		echo "Failed to get bundle snapshot ${BUNDLE_SNAPSHOT_REF}" >&2
		exit 1
	fi
else
	echo "bundle-snapshot-ref is not specified, bundle image entries are preserved"
	: >"${TMP_BUNDLE_SNAPSHOT}"
fi

map_registry_prefix() {
	local image="$1"
	local konflux_prefix="$2"
	local stable_prefix="$3"
	local target_prefix="${stable_prefix}"

	if [ "${USE_REGISTRY}" = "preview" ]; then
		target_prefix="${stable_prefix/openshift-lightspeed\//openshift-lightspeed-tech-preview/}"
	fi
	if [ "${USE_REGISTRY}" = "stable" ] || [ "${USE_REGISTRY}" = "preview" ]; then
		if [ -n "${konflux_prefix}" ] && [ "${konflux_prefix}" != "null" ] && [ -n "${target_prefix}" ] && [ "${target_prefix}" != "null" ]; then
			sed "s|${konflux_prefix}|${target_prefix}|g" <<<"${image}"
			return
		fi
	fi
	printf '%s' "${image}"
}

snapshot_component_image() {
	local snapshot_file="$1"
	local component="$2"
	${JQ} -r --arg c "${component}" '
		.spec.components[] | select(.name == $c) | .containerImage // empty
	' "${snapshot_file}"
}

snapshot_component_revision() {
	local snapshot_file="$1"
	local component="$2"
	${JQ} -r --arg c "${component}" '
		.spec.components[] | select(.name == $c) | .source.git.revision // empty
	' "${snapshot_file}"
}

postgres_default_image() {
	grep -o 'PostgresServerImageDefault = "registry[^"]*"' "${SCRIPT_DIR}/../internal/controller/utils/constants.go" \
		| sed 's/PostgresServerImageDefault = "\(.*\)"/\1/'
}

RESULT='[]'
while IFS= read -r entry; do
	name=$(${JQ} -r '.name' <<<"${entry}")
	image=$(${JQ} -r '.image // empty' <<<"${entry}")
	revision=$(${JQ} -r '.revision // empty' <<<"${entry}")
	component=$(${JQ} -r '.snapshot_component // empty' <<<"${entry}")
	source=$(${JQ} -r '.snapshot_source // "ols"' <<<"${entry}")
	konflux_prefix=$(${JQ} -r '.konflux_prefix // empty' <<<"${entry}")
	stable_prefix=$(${JQ} -r '.stable_prefix // empty' <<<"${entry}")

	if [ -n "${component}" ] && [ "${component}" != "null" ]; then
		snapshot_file="${TMP_OLS_SNAPSHOT}"
		if [ "${source}" = "bundle" ]; then
			if [ -z "${BUNDLE_SNAPSHOT_REF}" ]; then
				component=""
			else
				snapshot_file="${TMP_BUNDLE_SNAPSHOT}"
			fi
		fi
		if [ -n "${component}" ]; then
			snapshot_image=$(snapshot_component_image "${snapshot_file}" "${component}")
			snapshot_revision=$(snapshot_component_revision "${snapshot_file}" "${component}")
			if [ -n "${snapshot_image}" ] && [ "${snapshot_image}" != "null" ]; then
				image="${snapshot_image}"
				if [ -n "${snapshot_revision}" ] && [ "${snapshot_revision}" != "null" ]; then
					revision="${snapshot_revision}"
				fi
			fi
		fi
	fi

	if [ "${name}" = "lightspeed-postgresql" ]; then
		if [ -z "${image}" ] || [ "${image}" = "null" ]; then
			image=$(postgres_default_image)
		fi
		image=$(sed 's|quay\.io.*/lightspeed-postgresql|registry.redhat.io/rhel9/postgresql-16|g' <<<"${image}")
	fi

	image=$(map_registry_prefix "${image}" "${konflux_prefix}" "${stable_prefix}")

	if [ -z "${image}" ] || [ "${image}" = "null" ]; then
		echo "${name} image not found: ensure ${INPUT_FILE} lists a fallback image or snapshot_component metadata." >&2
		exit 1
	fi

	out_entry=$(${JQ} --arg image "${image}" --arg revision "${revision}" '
		.image = $image
		| .revision = $revision
	' <<<"${entry}")
	RESULT=$(${JQ} --argjson e "${out_entry}" '. + [$e]' <<<"${RESULT}")
done < <(${JQ} -c '.[]' "${INPUT_FILE}")

if [ -n "${OUTPUT_FILE}" ]; then
	${JQ} <<<"${RESULT}" >"${OUTPUT_FILE}"
else
	${JQ} <<<"${RESULT}"
fi
