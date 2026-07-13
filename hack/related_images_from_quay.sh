#!/usr/bin/env bash
# Refresh related_images.json from Quay using konflux_prefix + revision tags.
# Does not use oc/Konflux snapshots. Requires oras, jq, and curl.
set -euo pipefail

usage() {
	cat <<'EOF'
Usage: related_images_from_quay.sh [-l] [-o <output-file>] [-r ci|stable|preview]

Resolves Konflux-built entries via oras against:
  <konflux_prefix>:<revision>   when revision is set (default)
  <konflux_prefix>:<tag>        otherwise tag from current image (e.g. :main)

With -l (latest), discovers the newest Konflux build on Quay per operand:
  1. Resolve <konflux_prefix>:main when present
  2. Map digest to a git SHA tag via the Quay API
  3. Otherwise use the most recently pushed git SHA tag on Quay

External/manual entries (no konflux_prefix) are unchanged.
EOF
}

OUTPUT_FILE=""
USE_REGISTRY="ci"
ADVANCE_LATEST=false

while getopts ":lo:r:h" argname; do
	case "$argname" in
	l) ADVANCE_LATEST=true ;;
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

: "${JQ:=$(command -v jq)}"
: "${ORAS:=$(command -v oras)}"
: "${CURL:=$(command -v curl)}"
if [[ -z "${JQ}" ]]; then
	echo "jq is required" >&2
	exit 1
fi
if [[ -z "${ORAS}" ]]; then
	echo "oras is required (brew install oras)" >&2
	exit 1
fi
if [[ -z "${CURL}" ]]; then
	echo "curl is required" >&2
	exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INPUT_FILE="${SCRIPT_DIR}/../related_images.json"
if [[ -n "${OUTPUT_FILE}" && -f "${OUTPUT_FILE}" ]]; then
	INPUT_FILE="${OUTPUT_FILE}"
elif [[ ! -f "${INPUT_FILE}" ]]; then
	echo "related images file not found: ${INPUT_FILE}" >&2
	exit 1
fi

map_registry_prefix() {
	local image="$1"
	local konflux_prefix="$2"
	local stable_prefix="$3"
	local target_prefix="${stable_prefix}"

	if [[ "${USE_REGISTRY}" = "preview" ]]; then
		target_prefix="${stable_prefix/openshift-lightspeed\//openshift-lightspeed-tech-preview/}"
	fi
	if [[ "${USE_REGISTRY}" = "stable" || "${USE_REGISTRY}" = "preview" ]]; then
		if [[ -n "${konflux_prefix}" && "${konflux_prefix}" != "null" && -n "${target_prefix}" && "${target_prefix}" != "null" ]]; then
			sed "s|${konflux_prefix}|${target_prefix}|g" <<<"${image}"
			return
		fi
	fi
	printf '%s' "${image}"
}

image_tag_from_ref() {
	local image="$1"
	if [[ "${image}" == *@sha256:* ]]; then
		printf '%s' ""
		return
	fi
	if [[ "${image}" == *:* ]]; then
		printf '%s' "${image##*:}"
		return
	fi
	printf '%s' ""
}

konflux_prefix_to_quay_repo() {
	local prefix="$1"
	prefix="${prefix#quay.io/}"
	printf '%s' "${prefix}"
}

quay_fetch_tags_page() {
	local repo="$1"
	local page="$2"
	"${CURL}" -fsS "https://quay.io/api/v1/repository/${repo}/tag/?page=${page}&limit=100"
}

# Find a 40-char git SHA tag whose manifest matches digest (e.g. sha256:abc...).
quay_find_revision_for_digest() {
	local prefix="$1"
	local digest="$2"
	local repo page=1 json match has_more

	repo="$(konflux_prefix_to_quay_repo "${prefix}")"
	while [[ "${page}" -le 20 ]]; do
		json="$(quay_fetch_tags_page "${repo}" "${page}")"
		match=$("${JQ}" -r --arg d "${digest}" '
			[.tags[] | select(.name | test("^[0-9a-f]{40}$")) | select(.manifest_digest == $d)] | .[0].name // empty
		' <<<"${json}")
		if [[ -n "${match}" ]]; then
			printf '%s' "${match}"
			return 0
		fi
		has_more=$("${JQ}" -r '.has_additional' <<<"${json}")
		[[ "${has_more}" != "true" ]] && break
		page=$((page + 1))
	done
	return 1
}

# Newest git SHA tag by Quay start_ts (fallback when :main is missing or unmapped).
quay_newest_sha_revision() {
	local prefix="$1"
	local repo page=1 json has_more best_ts=0 best_rev="" name ts

	repo="$(konflux_prefix_to_quay_repo "${prefix}")"
	while [[ "${page}" -le 20 ]]; do
		json="$(quay_fetch_tags_page "${repo}" "${page}")"
		while IFS=' ' read -r name ts; do
			[[ -z "${name}" ]] && continue
			if [[ "${ts}" -gt "${best_ts}" ]]; then
				best_ts="${ts}"
				best_rev="${name}"
			fi
		done < <("${JQ}" -r '.tags[] | select(.name | test("^[0-9a-f]{40}$")) | "\(.name) \(.start_ts)"' <<<"${json}")
		has_more=$("${JQ}" -r '.has_additional' <<<"${json}")
		[[ "${has_more}" != "true" ]] && break
		page=$((page + 1))
	done
	if [[ -n "${best_rev}" ]]; then
		printf '%s' "${best_rev}"
		return 0
	fi
	return 1
}

discover_latest_revision() {
	local prefix="$1"
	local digest rev

	if digest=$("${ORAS}" resolve "${prefix}:main" 2>/dev/null); then
		if rev=$(quay_find_revision_for_digest "${prefix}" "${digest}"); then
			printf '%s' "${rev}"
			return 0
		fi
		echo "warning: ${prefix}:main resolved but no matching git SHA tag on Quay; using newest SHA tag" >&2
	fi

	if rev=$(quay_newest_sha_revision "${prefix}"); then
		printf '%s' "${rev}"
		return 0
	fi

	echo "warning: could not discover latest revision for ${prefix}" >&2
	return 1
}

resolve_quay_image() {
	local konflux_prefix="$1"
	local revision="$2"
	local fallback_image="$3"
	local tag ref digest

	if [[ -z "${konflux_prefix}" || "${konflux_prefix}" == "null" ]]; then
		printf '%s' "${fallback_image}"
		return
	fi

	# Already a digest URL with no revision: re-resolve that ref on Quay.
	if [[ ( -z "${revision}" || "${revision}" == "null" ) && "${fallback_image}" == *@sha256:* ]]; then
		if digest=$("${ORAS}" resolve "${fallback_image}" 2>&1); then
			printf '%s' "${konflux_prefix}@${digest}"
			return
		fi
		echo "warning: failed to resolve ${fallback_image}, keeping current image" >&2
		printf '%s' "${fallback_image}"
		return
	fi

	tag="${revision}"
	if [[ -z "${tag}" || "${tag}" == "null" ]]; then
		tag="$(image_tag_from_ref "${fallback_image}")"
	fi
	if [[ -z "${tag}" ]]; then
		tag="main"
	fi

	ref="${konflux_prefix}:${tag}"
	if ! digest=$("${ORAS}" resolve "${ref}" 2>&1); then
		echo "warning: failed to resolve ${ref}, keeping current image (${digest})" >&2
		printf '%s' "${fallback_image}"
		return
	fi
	printf '%s' "${konflux_prefix}@${digest}"
}

postgres_fixup() {
	local name="$1"
	local image="$2"
	if [[ "${name}" == "lightspeed-postgresql" ]]; then
		sed 's|quay\.io.*/lightspeed-postgresql|registry.redhat.io/rhel9/postgresql-16|g' <<<"${image}"
		return
	fi
	printf '%s' "${image}"
}

RESULT='[]'
while IFS= read -r entry; do
	name=$("${JQ}" -r '.name' <<<"${entry}")
	image=$("${JQ}" -r '.image // empty' <<<"${entry}")
	revision=$("${JQ}" -r '.revision // empty' <<<"${entry}")
	konflux_prefix=$("${JQ}" -r '.konflux_prefix // empty' <<<"${entry}")
	stable_prefix=$("${JQ}" -r '.stable_prefix // empty' <<<"${entry}")

	if [[ "${ADVANCE_LATEST}" == "true" && -n "${konflux_prefix}" && "${konflux_prefix}" != "null" ]]; then
		latest_rev=""
		if latest_rev=$(discover_latest_revision "${konflux_prefix}"); then
			if [[ "${latest_rev}" != "${revision}" ]]; then
				echo "info: ${name}: revision ${revision:-<empty>} -> ${latest_rev}" >&2
			fi
			revision="${latest_rev}"
		fi
	fi

	if [[ -n "${konflux_prefix}" && "${konflux_prefix}" != "null" ]]; then
		image=$(resolve_quay_image "${konflux_prefix}" "${revision}" "${image}")
	fi

	image=$(postgres_fixup "${name}" "${image}")
	image=$(map_registry_prefix "${image}" "${konflux_prefix}" "${stable_prefix}")

	if [[ -z "${image}" || "${image}" == "null" ]]; then
		echo "${name} image not found" >&2
		exit 1
	fi

	out_entry=$("${JQ}" --arg image "${image}" --arg revision "${revision}" '
		.image = $image
		| if $revision != "" then .revision = $revision else . end
	' <<<"${entry}")
	RESULT=$("${JQ}" --argjson e "${out_entry}" '. + [$e]' <<<"${RESULT}")
done < <("${JQ}" -c '.[]' "${INPUT_FILE}")

if [[ -n "${OUTPUT_FILE}" ]]; then
	"${JQ}" <<<"${RESULT}" >"${OUTPUT_FILE}"
else
	"${JQ}" <<<"${RESULT}"
fi
