#!/usr/bin/env bash

usage() {
  echo "Usage: $0 -s <snapshot-refs> -c <catalog-file> -n <channel-names> -m"
  echo "  -s snapshot-refs: required, the snapshots' references to use, ordered by versions ascending, example: ols-cq8sl,ols-mdc8x"
  echo "  -c catalog-file: the catalog index file to update, default: lightspeed-catalog-4.16/index.yaml"
  echo "  -n channel-names: the channel names to update, default: alpha"
  echo "  -m migrate: migrate the bundle object to csv metadata, required for OCP 4.17+, default: false"
  echo "Example: $0 -s ols-cq8sl,ols-mdc8x -c lightspeed-catalog-4.16/index.yaml"
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

while getopts ":s:c:n:mh" argname; do
  case "$argname" in
  "s")
    SNAPSHOT_REFS=${OPTARG}
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

for SNAPSHOT_REF in $(echo ${SNAPSHOT_REFS} | tr "," "\n"); do
  echo "Update catalog ${CATALOG_FILE} from snapshot ${SNAPSHOT_REF}"
  # cache the snapshot from Konflux
  oc get snapshot ${SNAPSHOT_REF} -o json >"${TMP_SNAPSHOT_JSON}"
  if [ $? -ne 0 ]; then
    echo "Failed to get snapshot ${SNAPSHOT_REF}"
    echo "Please make sure the snapshot exists and the snapshot name is correct"
    echo "Need to login Konflux through oc login, proxy command to be found here: https://registration-service-toolchain-host-operator.apps.stone-prd-host1.wdlc.p1.openshiftapps.com/"
    exit 1
  fi
  BUNDLE_IMAGE_BASE="registry.redhat.io/openshift-lightspeed-tech-preview/lightspeed-operator-bundle"
  OPERATOR_IMAGE_BASE="registry.redhat.io/openshift-lightspeed-tech-preview/lightspeed-rhel9-operator"
  CONSOLE_IMAGE_BASE="registry.redhat.io/openshift-lightspeed-tech-preview/lightspeed-console-plugin-rhel9"
  SERVICE_IMAGE_BASE="registry.redhat.io/openshift-lightspeed-tech-preview/lightspeed-service-api-rhel9"

  BUNDLE_IMAGE_ORIGIN=$(jq -r '.spec.components[]| select(.name=="bundle") | .containerImage' "${TMP_SNAPSHOT_JSON}")
  BUNDLE_IMAGE=$(sed 's|quay\.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols/bundle|'"${BUNDLE_IMAGE_BASE}"'|g' <<<${BUNDLE_IMAGE_ORIGIN})

  OPERATOR_IMAGE=$(jq -r '.spec.components[]| select(.name=="lightspeed-operator") | .containerImage' "${TMP_SNAPSHOT_JSON}")
  OPERATOR_IMAGE=$(sed 's|quay\.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols/lightspeed-operator|'"${OPERATOR_IMAGE_BASE}"'|g' <<<${OPERATOR_IMAGE})

  CONSOLE_IMAGE=$(jq -r '.spec.components[]| select(.name=="lightspeed-console") | .containerImage' "${TMP_SNAPSHOT_JSON}")
  CONSOLE_IMAGE=$(sed 's|quay\.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols/lightspeed-console|'"${CONSOLE_IMAGE_BASE}"'|g' <<<${CONSOLE_IMAGE})

  SERVICE_IMAGE=$(jq -r '.spec.components[]| select(.name=="lightspeed-service") | .containerImage' "${TMP_SNAPSHOT_JSON}")
  SERVICE_IMAGE=$(sed 's|quay\.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols/lightspeed-service|'"${SERVICE_IMAGE_BASE}"'|g' <<<${SERVICE_IMAGE})

  RELATED_IMAGES=$(
    cat <<-EOF
[
  {
    "name": "lightspeed-service-api",
    "image": "${SERVICE_IMAGE}"
  },
  {
    "name": "lightspeed-console-plugin",
    "image": "${CONSOLE_IMAGE}"
  },
  {
    "name": "lightspeed-operator",
    "image": "${OPERATOR_IMAGE}"
  },
  { "name": "lightspeed-operator-bundle",
    "image": "${BUNDLE_IMAGE}"
  }
]
EOF
  )
  echo "Catalog will use the following images:"
  echo "BUNDLE_IMAGE=${BUNDLE_IMAGE}"
  echo "OPERATOR_IMAGE=${OPERATOR_IMAGE}"
  echo "CONSOLE_IMAGE=${CONSOLE_IMAGE}"
  echo "SERVICE_IMAGE=${SERVICE_IMAGE}"

  echo BUNDLE_IMAGE_ORIGIN=${BUNDLE_IMAGE_ORIGIN}
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
