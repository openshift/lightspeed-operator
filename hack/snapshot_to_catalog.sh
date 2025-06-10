#!/usr/bin/env bash

usage() {
  echo "Usage: $0 -s <snapshot-refs> -b <bundle-snapshot-refs> -c <catalog-file> -n <channel-names> -m"
  echo "  -s snapshot-refs: required, the snapshot' references to use"
  echo "  -b bundle-snapshot-refs: required, the snapshot' references to use"
  echo "  -c catalog-file: the catalog index file to update, default: lightspeed-catalog-4.16/index.yaml"
  echo "  -n channel-names: the channel names to update, default: alpha"
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

SNAPSHOT_REFS=""
CATALOG_FILE="lightspeed-catalog-4.16/index.yaml"
CHANNEL_NAMES="alpha"
MIGRATE=""

while getopts ":s:b:c:n:mh" argname; do
  case "$argname" in
  "s")
    SNAPSHOT_REFS=${OPTARG}
    ;;
  "b")
    BUNDLE_SNAPSHOT_REFS=${OPTARG}
    ;;
  "c")
    CATALOG_FILE=${OPTARG}
    ;;
  "n")
    CHANNEL_NAMES=${OPTARG}
    ;;
  "m")
    MIGRATE="true"
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

echo migrate ${MIGRATE}

if [ -z "${SNAPSHOT_REFS}" ]; then
  echo "snapshot-refs is required"
  usage
  exit 1
fi

if [ -z "${BUNDLE_SNAPSHOT_REFS}" ]; then
  echo "bundle-snapshot-refs is required"
  usage
  exit 1
fi

#array from comma separated string of snapshot-refs and bundle-snapshot-refs
SNAPSHOT_REFS=$(echo ${SNAPSHOT_REFS} | tr "," "\n")
BUNDLE_SNAPSHOT_REFS=$(echo ${BUNDLE_SNAPSHOT_REFS} | tr "," "\n")


if [  ${#SNAPSHOT_REFS[@]} -ne ${#BUNDLE_SNAPSHOT_REFS[@]} ]; then
  echo "The count of snapshot-refs and bundle-snapshot-refs should be the same"
  usage
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

# temporary file for snapshot info from Konflux
TMP_SNAPSHOT_JSON=$(mktemp)
# temporary file for rendering the bundle part of the catalog
TEMP_BUNDLE_FILE=$(mktemp)

cleanup() {
  # remove temporary snapshot file
  if [ -n "${TMP_SNAPSHOT_JSON}" ]; then
    rm -f "${TMP_SNAPSHOT_JSON}"
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

# array to store the bundle versions
BUNDLE_VERSIONS=()
GET_RELATED_IMAGES="${SCRIPT_DIR}/snapshot_to_image_list.sh"
# todo: allow different registry for different snapshots
REGISTRY="stable"
for snapshot in "${!SNAPSHOT_REFS[@]}"; do
  SNAPSHOT_REF=${SNAPSHOT_REFS[snapshot]}
  BUNDLE_SNAPSHOT_REF=${BUNDLE_SNAPSHOT_REFS[snapshot]}
  echo "Update catalog ${CATALOG_FILE} from snapshot ${SNAPSHOT_REF}"
  echo "Update catalog ${CATALOG_FILE} from bundle snapshot ${BUNDLE_SNAPSHOT_REF}"
  # get bundle image on konflux workspace
  RELATED_IMAGES=$(${GET_RELATED_IMAGES} -s ${SNAPSHOT_REF} -b ${BUNDLE_SNAPSHOT_REF})
  BUNDLE_IMAGE_ORIGIN=$(jq -r '.[] | select(.name=="lightspeed-operator-bundle") | .image' <<<${RELATED_IMAGES})
  # get image list in production registry
  RELATED_IMAGES=$(${GET_RELATED_IMAGES} -s ${SNAPSHOT_REF} -b ${BUNDLE_SNAPSHOT_REF} -r ${REGISTRY})
  BUNDLE_IMAGE=$(jq -r '.[] | select(.name=="lightspeed-operator-bundle") | .image' <<<${RELATED_IMAGES})
  echo "Catalog will use the following images:"
  echo "${RELATED_IMAGES}"

  OPM_ARGS=""
  if [ -n "${MIGRATE}" ]; then
    OPM_ARGS="--migrate-level=bundle-object-to-csv-metadata"
  fi
  ${OPM} render ${BUNDLE_IMAGE_ORIGIN} --output=yaml ${OPM_ARGS} >"${TEMP_BUNDLE_FILE}"
  BUNDLE_VERSION=$(yq '.properties[]| select(.type=="olm.package")| select(.value.packageName=="lightspeed-operator") |.value.version' ${TEMP_BUNDLE_FILE})
  echo "Bundle version is ${BUNDLE_VERSION}"
  BUNDLE_VERSIONS+=("${BUNDLE_VERSION}")
  # restore bundle image to the catalog file
  ${YQ} eval -i '.image='"\"${BUNDLE_IMAGE}\"" "${TEMP_BUNDLE_FILE}"
  # restore bundle related images and the bundle itself to the catalog file
  ${YQ} eval -i '.relatedImages='"${RELATED_IMAGES}" "${TEMP_BUNDLE_FILE}"

  cat ${TEMP_BUNDLE_FILE} >>"${CATALOG_FILE}"

done
echo "Bundle versions are ${BUNDLE_VERSIONS[@]}"

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
  for BUNDLE_VERSION in ${BUNDLE_VERSIONS[@]}; do
    cat <<EOF >>"${CATALOG_FILE}"
  - name: lightspeed-operator.v${BUNDLE_VERSION}
EOF
    if [ -z "${PREV_VERSION}" ]; then
      cat <<EOF >>"${CATALOG_FILE}"
    skipRange: ">=0.1.0 <${BUNDLE_VERSION}"
EOF
    else
      cat <<EOF >>"${CATALOG_FILE}"
    replaces: lightspeed-operator.v${PREV_VERSION}
EOF
    fi
    PREV_VERSION=${BUNDLE_VERSION}
  done

done

${OPM} validate "$(dirname "${CATALOG_FILE}")"
if [ $? -ne 0 ]; then
  echo "Validation failed for ${CATALOG_FILE}"
  exit 1
else
  echo "Validation passed for ${CATALOG_FILE}"
fi
