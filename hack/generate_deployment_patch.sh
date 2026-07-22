#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

RELATED_IMAGES="${RELATED_IMAGES_FILE:-${REPO_ROOT}/related_images.json}"
OUTPUT="${DEPLOYMENT_PATCH_FILE:-${REPO_ROOT}/config/default/deployment-patch.yaml}"
JQ="${JQ:-jq}"

# shellcheck source=image_args_lib.sh
source "${SCRIPT_DIR}/image_args_lib.sh"

if [[ ! -f "${RELATED_IMAGES}" ]]; then
	echo "error: ${RELATED_IMAGES} not found" >&2
	exit 1
fi

image_args::generate_deployment_patch "${RELATED_IMAGES}" "${OUTPUT}" "${JQ}"
echo "Wrote ${OUTPUT}"
