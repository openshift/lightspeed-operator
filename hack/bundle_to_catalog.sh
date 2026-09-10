#!/usr/bin/env bash

usage() {
  echo "Usage: $0 -b <bundle-snapshot-ref> -t v1|v2 -i <images-file> -c <catalog-file> -n <channel-names> -m"
  echo "  -b bundle-snapshot-ref: required, the bunlde snapshot references, example: ols-bundle-2dhtr"
  echo "  -i images-file: required, json file containing related images, at least operands, default related_images.json"
  echo "  -c catalog-file: the catalog index file to update, default: lightspeed-catalog-4.16/index.yaml"
  echo "  -n channel-names: the channel names to update, default: alpha"
  echo "  -t bundle variant: v1 (1.x classic) or v2 (2.x full), required"
  echo "  -m migrate: migrate the bundle object to csv metadata, required for OCP 4.17+, default: false"
  echo "Example: $0 -s ols-cq8sl -b ols-bundle-2dhtr -c lightspeed-catalog-4.16/index.yaml"
}

if [ $# == 0 ]; then
  usage
  exit 1
fi

version_gt() {
  test "$(printf '%s\n' "$@" | sort -V | tail -n 1)" != "$1"
}

KONFLUX_NAMESPACE="crt-nshift-lightspeed-tenant"
CATALOG_FILE="lightspeed-catalog-4.16/index.yaml"
CHANNEL_NAMES="alpha"
MIGRATE=""
RELATED_IMAGES_FILE="related_images.json"
BUNDLE_VARIANT=""

while getopts ":b:i:c:n:t:mh" argname; do
  case "$argname" in
  "i")
    RELATED_IMAGES_FILE=${OPTARG}
    ;;
  "b")
    BUNDLE_SNAPSHOT_REF=${OPTARG}
    ;;
  "c")
    CATALOG_FILE=${OPTARG}
    ;;
  "n")
    CHANNEL_NAMES=${OPTARG}
    ;;
  "t")
    BUNDLE_VARIANT=${OPTARG}
    ;;
  "m")
    MIGRATE="true"
    echo "migrate is activated, bundle object will be migrated to csv metadata"
    ;;
  "h")
    usage
    exit 0
    ;;
  "?")
    echo "Unknown option $OPTARG"
    usage
    exit 1
    ;;
  *)
    echo "Unknown error while processing options"
    exit 1
    ;;
  esac
done

if [ -z "${BUNDLE_SNAPSHOT_REF}" ] || [ -z "${BUNDLE_VARIANT}" ]; then
  echo "bundle snapshot reference and variant are required"
  usage
  exit 1
fi
if [[ "${BUNDLE_VARIANT}" != "v1" && "${BUNDLE_VARIANT}" != "v2" ]]; then
  echo "bundle variant must be v1 or v2"
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CATALOG_INITIAL_FILE="${SCRIPT_DIR}/operator.yaml"

: ${OPM:=$(command -v opm)}
echo "using opm from ${OPM}"
# check if opm version is greater than v1.46.0 or exit
OPM_VERSION=$(${OPM} version | grep -Eo 'OpmVersion:"[^"]*"' | grep -Eo '[0-9]*\.[0-9]*\.[0-9]*')
if version_gt ${OPM_VERSION} 1.46.0; then
  echo "opm version > 1.46.0 is required, current version is ${OPM_VERSION}"
  exit 1
fi

: ${YQ:=$(command -v yq)}
echo "using yq from ${YQ}"
# check if yq exists
if [ -z "${YQ}" ]; then
  echo "yq is required"
  exit 1
fi

: ${JQ:=$(command -v jq)}
echo "using jq from ${JQ}"
# check if jq exists
if [ -z "${JQ}" ]; then
  echo "jq is required"
  exit 1
fi

# temporary file for snapshot info from Konflux
TMP_BUNDLE_SNAPSHOT_JSON=$(mktemp)
# temporary file for rendering the bundle part of the catalog
TEMP_BUNDLE_FILE=$(mktemp)

cleanup() {
  # remove temporary snapshot file
  if [ -n "${TMP_BUNDLE_SNAPSHOT_JSON}" ]; then
    rm -f "${TMP_BUNDLE_SNAPSHOT_JSON}"
  fi

  # remove temporary bundle file
  if [ -n "${TEMP_BUNDLE_FILE}" ]; then
    rm -f "${TEMP_BUNDLE_FILE}"
  fi

}

trap cleanup EXIT

#Initialize catalog file from hack/operator.yaml
DEFAULT_CHANNEL_NAME=$(cut -d ',' -f 1 <<<${CHANNEL_NAMES})
sed "s/defaultChannel: alpha/defaultChannel: ${DEFAULT_CHANNEL_NAME}/" ${CATALOG_INITIAL_FILE} >"${CATALOG_FILE}"

# get bundle image on konflux workspace, replace CI registry with the stable registry
oc get -n ${KONFLUX_NAMESPACE} snapshot ${BUNDLE_SNAPSHOT_REF} -o json >"${TMP_BUNDLE_SNAPSHOT_JSON}"
BUNDLE_IMAGE_ORIGIN=$(${JQ} -r '.spec.components[]| select(.name=="ols-bundle") | .containerImage' "${TMP_BUNDLE_SNAPSHOT_JSON}")
BUNDLE_REVISION=$(${JQ} -r '.spec.components[]| select(.name=="ols-bundle") | .source.git.revision' "${TMP_BUNDLE_SNAPSHOT_JSON}")
BUNDLE_IMAGE_BASE="registry.redhat.io/openshift-lightspeed/lightspeed-operator-bundle"
BUNDLE_IMAGE=$(sed 's|quay\.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols-bundle|'"${BUNDLE_IMAGE_BASE}"'|g' <<<${BUNDLE_IMAGE_ORIGIN})

# Update or add lightspeed-operator-bundle in RELATED_IMAGES
RELATED_IMAGES=$(${JQ} --arg img "$BUNDLE_IMAGE" --arg rev "$BUNDLE_REVISION" '
  if map(select(.name == "lightspeed-operator-bundle")) | length > 0 then
    map(if .name == "lightspeed-operator-bundle" then .image = $img | .revision = $rev else . end)
  else
    . + [{"name":"lightspeed-operator-bundle","image":$img,"revision":$rev}]
  end
' <${RELATED_IMAGES_FILE})
# save the bundle image to the related images file
${JQ} <<<${RELATED_IMAGES} >"${RELATED_IMAGES_FILE}"
# remove revision from each element
RELATED_IMAGES=$(${JQ} <<<${RELATED_IMAGES} 'map(del(.revision))')
echo "Catalog will use the following images: ${RELATED_IMAGES}"

OPM_ARGS=""
if [ -n "${MIGRATE}" ]; then
  OPM_ARGS="--migrate-level=bundle-object-to-csv-metadata"
fi
${OPM} render ${BUNDLE_IMAGE_ORIGIN} --output=yaml ${OPM_ARGS} >"${TEMP_BUNDLE_FILE}"
BUNDLE_VERSION=$(${YQ} eval '.properties[]| select(.type=="olm.package")| select(.value.packageName=="lightspeed-operator") |.value.version' ${TEMP_BUNDLE_FILE})
echo "Bundle version is ${BUNDLE_VERSION} (${BUNDLE_VARIANT})"
if [[ "${BUNDLE_VARIANT}" == "v1" && "${BUNDLE_VERSION}" != 1.* ]] ||
   [[ "${BUNDLE_VARIANT}" == "v2" && "${BUNDLE_VERSION}" != 2.* ]]; then
  echo "bundle version ${BUNDLE_VERSION} does not match ${BUNDLE_VARIANT}" >&2
  exit 1
fi
# restore bundle image to the bundle file
${YQ} eval -i '.image='"\"${BUNDLE_IMAGE}\"" "${TEMP_BUNDLE_FILE}"
# restore bundle related images and the bundle itself to the bundle file
${YQ} eval -i '.relatedImages='"${RELATED_IMAGES}" "${TEMP_BUNDLE_FILE}"

# Write bundle to its own file
BUNDLE_FILE="$(dirname "${CATALOG_FILE}")/bundle-v${BUNDLE_VERSION}.yaml"
echo "Writing bundle to ${BUNDLE_FILE}"
cat ${TEMP_BUNDLE_FILE} >"${BUNDLE_FILE}"

# Collect all bundle versions from existing bundle-v*.yaml files in the catalog directory
CATALOG_DIR="$(dirname "${CATALOG_FILE}")"
echo "Scanning for existing bundle files in ${CATALOG_DIR}"
ALL_BUNDLE_VERSIONS=()
for bundle_file in "${CATALOG_DIR}"/bundle-v*.yaml; do
  if [ -f "$bundle_file" ]; then
    # Extract version from filename (bundle-v1.0.8.yaml -> 1.0.8)
    version=$(basename "$bundle_file" | sed 's/bundle-v\(.*\)\.yaml/\1/')
    if [[ "${BUNDLE_VARIANT}" == "v1" && "${version}" != 1.* ]] ||
       [[ "${BUNDLE_VARIANT}" == "v2" && "${version}" != 2.* ]]; then
      # A catalog is version-partitioned, not merely channel-partitioned.
      rm -f "${bundle_file}"
      continue
    fi
    ALL_BUNDLE_VERSIONS+=("$version")
  fi
done

# Sort versions to ensure correct order
IFS=$'\n' ALL_BUNDLE_VERSIONS=($(sort -V <<<"${ALL_BUNDLE_VERSIONS[*]}"))
unset IFS

echo "Found bundle versions from files: ${ALL_BUNDLE_VERSIONS[@]}"

echo "Channel names are ${CHANNEL_NAMES}"
for CHANNEL_NAME in $(echo ${CHANNEL_NAMES} | tr "," "\n"); do
  echo "Add channel ${CHANNEL_NAME} in catalog ${CATALOG_FILE}"
  cat <<EOF >>"${CATALOG_FILE}"
---
schema: olm.channel
package: lightspeed-operator
name: ${CHANNEL_NAME}
entries:
EOF
  PREV_VERSION=""
  for BUNDLE_VER in ${ALL_BUNDLE_VERSIONS[@]}; do
    cat <<EOF >>"${CATALOG_FILE}"
  - name: lightspeed-operator.v${BUNDLE_VER}
EOF
    if [ "${BUNDLE_VARIANT}" = "v2" ] && [ -z "${PREV_VERSION}" ]; then
      cat <<EOF >>"${CATALOG_FILE}"
    skipRange: ">=1.0.0 <2.0.0"
EOF
    elif [ -z "${PREV_VERSION}" ]; then
      cat <<EOF >>"${CATALOG_FILE}"
    skipRange: ">=0.1.0 <${BUNDLE_VER}"
EOF
    else
      cat <<EOF >>"${CATALOG_FILE}"
    replaces: lightspeed-operator.v${PREV_VERSION}
EOF
    fi
    PREV_VERSION=${BUNDLE_VER}
  done
done

${OPM} validate "$(dirname "${CATALOG_FILE}")"
if [ $? -ne 0 ]; then
  echo "Validation failed for ${CATALOG_FILE}"
  exit 1
else
  echo "Validation passed for ${CATALOG_FILE}"
fi
