#!/bin/bash
# Helper tool to update the bundle artifacts
# Pre-requisites: opm, make, yq, operator-sdk
# Usage: ./hack/update_bundle.sh v1|v2 [-v bundle_version]

set -euo pipefail

TEMP_BUNDLE_CONTAINER_FILE=$(mktemp)
TEMP_BUNDLE_CSV_BACKUP="${TEMP_BUNDLE_CONTAINER_FILE}.csv-base"
BUNDLE_DOCKERFILE_BASE="bundle.Dockerfile"
TEMP_BUNDLE_DOCKERFILE_BACKUP="${TEMP_BUNDLE_CONTAINER_FILE}.bundle-dockerfile"
DEPLOYMENT_PATCH_FILE="config/default/deployment-patch.yaml"
DEFAULT_KUSTOMIZATION="config/default/kustomization.yaml"
DEFAULT_KUSTOMIZATION_V1="config/default/kustomization-v1.yaml"
DEFAULT_KUSTOMIZATION_BACKUP="${TEMP_BUNDLE_CONTAINER_FILE}.default-kustomization"
DEPLOYMENT_PATCH_BACKUP=$(mktemp)
cp "${DEPLOYMENT_PATCH_FILE}" "${DEPLOYMENT_PATCH_BACKUP}"
cp "${BUNDLE_DOCKERFILE_BASE}" "${TEMP_BUNDLE_DOCKERFILE_BACKUP}"
CSV_BASE_TEMPLATE="config/manifests/bases/lightspeed-operator.clusterserviceversion.yaml"
MANIFESTS_KUSTOMIZATION="config/manifests/kustomization.yaml"
MANIFESTS_KUSTOMIZATION_BACKUP="${TEMP_BUNDLE_CONTAINER_FILE}.manifests-kustomization"

cleanup() {
  cp "${DEPLOYMENT_PATCH_BACKUP}" "${DEPLOYMENT_PATCH_FILE}"
  if [ -f "${TEMP_BUNDLE_CSV_BACKUP}" ]; then
    mv "${TEMP_BUNDLE_CSV_BACKUP}" "${CSV_BASE_TEMPLATE}"
  fi
  if [ -f "${MANIFESTS_KUSTOMIZATION_BACKUP}" ]; then
    mv "${MANIFESTS_KUSTOMIZATION_BACKUP}" "${MANIFESTS_KUSTOMIZATION}"
  fi
  if [ -f "${DEFAULT_KUSTOMIZATION_BACKUP}" ]; then
    mv "${DEFAULT_KUSTOMIZATION_BACKUP}" "${DEFAULT_KUSTOMIZATION}"
  fi
  if [ -f "${TEMP_BUNDLE_DOCKERFILE_BACKUP}" ]; then
    mv "${TEMP_BUNDLE_DOCKERFILE_BACKUP}" "${BUNDLE_DOCKERFILE_BASE}"
  fi
  rm -f "${TEMP_BUNDLE_CONTAINER_FILE}" "${TEMP_BUNDLE_CONTAINER_FILE}.related_images.json" \
    "${DEPLOYMENT_PATCH_BACKUP}"

}

trap cleanup EXIT

SCRIPT_DIR=$(dirname "$0")

usage() {
  echo "Usage: $0 v1|v2 -i related_images_filename [-v bundle_version] [-c channel_name]"
  echo "  v1|v2: Bundle variant (v1 is classic, v2 includes agentic components)"
  echo "  -v bundle_version: The version of the bundle (defaults to 1.0.0 or 2.0.0)"
  echo "  -i related_images_filename: The JSON file containing the related images"
  echo "  -c channel_name: The bundle channel (defaults to alpha)"
  echo "  -h: Show this help message"
}

if [ "$#" -ge 1 ] && [[ "$1" == "-h" || "$1" == "--help" ]]; then
  usage
  exit 0
fi
if [ "$#" -lt 1 ] || [[ "$1" != "v1" && "$1" != "v2" ]]; then
  echo "a bundle variant (v1 or v2) is required"
  usage
  exit 1
fi
BUNDLE_VARIANT="$1"
shift
BUNDLE_VERSION="${BUNDLE_VERSION:-${BUNDLE_VARIANT/v/}.0.0}"
RELATED_IMAGES_FILENAME=""
CHANNEL_NAME="alpha"

while getopts ":v:i:c:h" opt; do
  case "$opt" in
  "v")
    BUNDLE_VERSION="${OPTARG}"
    echo "bundle_version is ${BUNDLE_VERSION}"
    ;;
  "i")
    RELATED_IMAGES_FILENAME="${OPTARG}"
    if [ ! -f "${RELATED_IMAGES_FILENAME}" ]; then
      echo "related_images_filename ${RELATED_IMAGES_FILENAME} does not exist"
      exit 1
    fi
    echo "related_images from file ${RELATED_IMAGES_FILENAME}"
    ;;
  "c")
    CHANNEL_NAME="${OPTARG}"
    echo "channel_name is ${CHANNEL_NAME}"
    ;;
  "h")
    usage
    exit 0
    ;;
  "?")
    echo "Unknown option ${OPTARG}"
    usage
    exit 1
    ;;
  *)
    echo "Unknown error while processing options"
    exit 1
    ;;
  esac
done

if [[ "${BUNDLE_VARIANT}" == "v1" && "${BUNDLE_VERSION}" != 1.* ]] ||
   [[ "${BUNDLE_VARIANT}" == "v2" && "${BUNDLE_VERSION}" != 2.* ]]; then
  echo "bundle version ${BUNDLE_VERSION} does not match ${BUNDLE_VARIANT}"
  exit 1
fi

# Supplying BUNDLE_GEN_FLAGS replaces the defaults, but the selected version
# must always be passed exactly once.
BUNDLE_GEN_FLAGS="${BUNDLE_GEN_FLAGS:---channels=${CHANNEL_NAME} --default-channel=${CHANNEL_NAME} -q --overwrite} --version ${BUNDLE_VERSION}"

# Tool check
: "${YQ:=$(command -v yq)}"
echo "using yq from ${YQ}"
if [ -z "${YQ}" ]; then
  echo "yq is required"
  exit 1
fi

: "${JQ:=$(command -v jq)}"
echo "using jq from ${JQ}"
if [ -z "${JQ}" ]; then
  echo "jq is required"
  exit 1
fi

: "${OPERATOR_SDK:=$(command -v operator-sdk)}"
echo "using operator-sdk from ${OPERATOR_SDK}"
if [ -z "${OPERATOR_SDK}" ]; then
  echo "operator-sdk is required"
  exit 1
fi

: "${KUSTOMIZE:=$(command -v kustomize)}"
echo "using kustomize from ${KUSTOMIZE}"
if [ -z "${KUSTOMIZE}" ]; then
  echo "kustomize is required"
  exit 1
fi

# Keep both generated variants available for inspection and image builds.
BUNDLE_DIR="bundle-${BUNDLE_VARIANT}"
CSV_FILE="${BUNDLE_DIR}/manifests/lightspeed-operator.clusterserviceversion.yaml"
ANNOTATION_FILE="${BUNDLE_DIR}/metadata/annotations.yaml"

# The v1 template is intentionally separate so it can never inherit a second
# controller deployment. The v2 template (OLS-3188) carries the agentic layer's
# CSV metadata; its deployment, RBAC and owned CRDs come from config/agentic.
CSV_TEMPLATE="config/manifests/bases/lightspeed-operator-${BUNDLE_VARIANT}.clusterserviceversion.yaml"
if [ ! -f "${CSV_TEMPLATE}" ]; then
  echo "missing CSV template for ${BUNDLE_VARIANT}: ${CSV_TEMPLATE}" >&2
  exit 1
fi
cp "${CSV_BASE_TEMPLATE}" "${TEMP_BUNDLE_CSV_BACKUP}"
cp "${CSV_TEMPLATE}" "${CSV_BASE_TEMPLATE}"
# v1 replaces the shared RBAC assembly with its classic-only counterpart.
if [ "${BUNDLE_VARIANT}" = "v1" ]; then
  cp "${DEFAULT_KUSTOMIZATION}" "${DEFAULT_KUSTOMIZATION_BACKUP}"
  cp "${DEFAULT_KUSTOMIZATION_V1}" "${DEFAULT_KUSTOMIZATION}"
fi
# v2 assembly pulls in the agentic controller deployment, RBAC and owned CRDs
# (config/agentic, synced by OLS-3189). operator-sdk then emits the second
# deployment, agentic clusterPermissions and owned agentic CRDs into the v2 CSV.
# The v1 assembly never references config/agentic. Restored by cleanup().
if [ "${BUNDLE_VARIANT}" = "v2" ] && [ -d config/agentic ]; then
  cp "${MANIFESTS_KUSTOMIZATION}" "${MANIFESTS_KUSTOMIZATION_BACKUP}"
  ( cd config/manifests && "${KUSTOMIZE}" edit add resource ../agentic )
fi

BUNDLE_DOCKERFILE="bundle-${BUNDLE_VARIANT}.Dockerfile"

# related_images.json is required because selector metadata is not stored in the CSV.
if [ -n "${RELATED_IMAGES_FILENAME}" ] && [ -f "${RELATED_IMAGES_FILENAME}" ]; then
  echo "using related images from file ${RELATED_IMAGES_FILENAME} for ${BUNDLE_VARIANT}"
  RELATED_IMAGES=$("${JQ}" --arg bundle "${BUNDLE_VARIANT}" '[.[] | select((has("bundles") | not) or (.bundles | index($bundle)))]' "${RELATED_IMAGES_FILENAME}")
else
  echo "error: provide -i related_images.json"
  exit 1
fi

if [ -z "${RELATED_IMAGES}" ] || [ "${RELATED_IMAGES}" = "null" ]; then
  echo "RELATED_IMAGES is empty, please provide related images via -i related_images.json"
  exit 1
fi

OPERATOR_IMAGE=$("${JQ}" -r '.[] | select(.name == "lightspeed-operator") | .image' <<<"${RELATED_IMAGES}")
echo "Updating bundle artifacts for image ${OPERATOR_IMAGE:-<from related_images>}"
rm -rf "./${BUNDLE_DIR}"

FILTERED_RELATED_IMAGES_FILE="${TEMP_BUNDLE_CONTAINER_FILE}.related_images.json"
printf '%s\n' "${RELATED_IMAGES}" > "${FILTERED_RELATED_IMAGES_FILE}"
RELATED_IMAGES_FILE="${FILTERED_RELATED_IMAGES_FILE}" ./hack/generate_deployment_patch.sh

"${OPERATOR_SDK}" generate kustomize manifests -q
"${KUSTOMIZE}" build config/manifests | "${OPERATOR_SDK}" generate bundle ${BUNDLE_GEN_FLAGS} --output-dir "${BUNDLE_DIR}"
# createdAt changes on every generation and is not required in bundle metadata.
"${YQ}" eval -i 'del(.metadata.annotations.createdAt)' "${CSV_FILE}"
# Generate a Dockerfile for the selected variant from the shared template.
cp ./hack/bundle.Dockerfile "${BUNDLE_DOCKERFILE}"
sed -i -e "s|COPY bundle/manifests|COPY ${BUNDLE_DIR}/manifests|" \
       -e "s|COPY bundle/metadata|COPY ${BUNDLE_DIR}/metadata|" \
       -e "s|COPY bundle/tests/scorecard|COPY ${BUNDLE_DIR}/tests/scorecard|" \
       "${BUNDLE_DOCKERFILE}"
if [ "${BUNDLE_VARIANT}" = "v2" ]; then
  sed -i -e 's|LABEL name=.*|LABEL name="openshift-lightspeed/lightspeed-agentic-operator-bundle"|' \
         -e 's|LABEL com.redhat.openshift.versions=.*|LABEL com.redhat.openshift.versions=">=v5.0"|' \
         "${BUNDLE_DOCKERFILE}"
fi
# The Dockerfile template carries the release metadata consumed by the bundle
# image. Keep it aligned with the operator-sdk CSV version for both variants.
BUNDLE_MAJOR_VERSION="${BUNDLE_VERSION%%.*}"
sed -i -e "s|^LABEL release=.*|LABEL release=${BUNDLE_VERSION}|" \
       -e "s|^LABEL version=.*|LABEL version=${BUNDLE_VERSION}|" \
       -e "s|^LABEL cpe=.*|LABEL cpe=\"cpe:/a:redhat:openshift_lightspeed:${BUNDLE_MAJOR_VERSION}::el9\"|" \
       "${BUNDLE_DOCKERFILE}"
# Substitute deployment args and container image from related_images.json (operator_arg / operator_target).
# shellcheck source=image_args_lib.sh
source "${SCRIPT_DIR}/image_args_lib.sh"
while IFS='|' read -r name placeholder target _; do
  IMG=$("${JQ}" -r --arg n "${name}" '.[] | select(.name==$n) | .image' <<<"${RELATED_IMAGES}")
  [ -z "${IMG}" ] || [ "${IMG}" = "null" ] && continue
  IMG_SAFE=$(printf '%s' "${IMG}" | sed 's/"/\\"/g')
  if [ "${target}" = "image" ]; then
    "${YQ}" "(.spec.install.spec.deployments[].spec.template.spec.containers[].image |= sub(\"${placeholder}\", \"${IMG_SAFE}\"))" -i "${CSV_FILE}"
  else
    "${YQ}" "(.spec.install.spec.deployments[].spec.template.spec.containers[].args[] |= sub(\"${placeholder}\", \"${IMG_SAFE}\"))" -i "${CSV_FILE}"
  fi
done < <(image_args::list_patch_entries "${FILTERED_RELATED_IMAGES_FILE}" "${JQ}")

# The agentic controller is a second deployment in the v2 CSV rather than an
# argument of the classic controller. Substitute its image directly; it must
# never retain the development `:latest` placeholder in a released bundle.
if [ "${BUNDLE_VARIANT}" = "v2" ]; then
  AGENTIC_OPERATOR_IMAGE=$("${JQ}" -r '.[] | select(.name == "lightspeed-agentic-operator") | .image' <<<"${RELATED_IMAGES}")
  if [ -z "${AGENTIC_OPERATOR_IMAGE}" ] || [ "${AGENTIC_OPERATOR_IMAGE}" = "null" ]; then
    echo "agentic operator image is required for the v2 bundle" >&2
    exit 1
  fi
  export AGENTIC_OPERATOR_IMAGE
  "${YQ}" eval -i '(.spec.install.spec.deployments[] | select(.name == "lightspeed-agentic-operator-controller-manager").spec.template.spec.containers[].image) = strenv(AGENTIC_OPERATOR_IMAGE)' "${CSV_FILE}"
fi

# Set spec.relatedImages from related_images.json (strip revision and snapshot metadata for OLM CSV).
# The bundle image is only referenced in catalog files, not in the CSV.
RELATED_IMAGES_CSV=$("${JQ}" 'map(select(.snapshot_source != "bundle") | del(.revision, .snapshot_component, .snapshot_source, .konflux_prefix, .stable_prefix, .operator_arg, .operator_target, .bundles))' <<<"${RELATED_IMAGES}")
# set related images to the CSV file
"${YQ}" eval -i '.spec.relatedImages='"${RELATED_IMAGES_CSV}" "${CSV_FILE}"
# v1 must not grant access to agentic API resources. Keep this filtering at
# bundle generation time so it also applies to the CSV permissions generated
# from config/rbac.
if [ "${BUNDLE_VARIANT}" = "v1" ]; then
  "${YQ}" eval -i '(.spec.install.spec.clusterPermissions[].rules, .spec.install.spec.permissions[].rules) |= map(select((.apiGroups // [] | contains(["agentic.openshift.io"])) == false) | select((.resources // [] | map(test("agentic")) | any) == false))' "${CSV_FILE}"
fi
# add compatibility labels to the annotations file
if [ "${BUNDLE_VARIANT}" = "v1" ]; then
  OCP_VERSIONS="v4.16-v4.22"
else
  OCP_VERSIONS=">=v5.0"
fi
"${YQ}" eval -i '.annotations."com.redhat.openshift.versions"="'"${OCP_VERSIONS}"'"' "${ANNOTATION_FILE}"
"${YQ}" eval -i '(.annotations."com.redhat.openshift.versions" | key) head_comment="OCP compatibility labels"' "${ANNOTATION_FILE}"
"${YQ}" eval -i '.annotations."features.operators.openshift.io/fips-compliant"="true"' "${ANNOTATION_FILE}"

# Validate the final artifact, after image, related-image, and annotation updates.
"${OPERATOR_SDK}" bundle validate "./${BUNDLE_DIR}"

echo "Finished running $(basename "$0")"
