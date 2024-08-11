#!/usr/bin/env bash

usage() {
  echo "Usage: $0 -s <snapshot-ref> -c <catalog-file>"
  echo "  snapshot-ref: required, the snapshot reference to use, example: ols-bnxm2"
  echo "  catalog-file: the catalog index file to update, default: lightspeed-catalog-4.16/index.yaml"
}

if [ $# == 0 ]; then
  usage
  exit 1
fi

SNAPSHOT_REF=""
CATALOG_FILE="lightspeed-catalog-4.16/index.yaml"

while getopts ":s:c:h" argname; do
  case "$argname" in
  "s")
    SNAPSHOT_REF=${OPTARG}
    ;;
  "c")
    CATALOG_FILE=${OPTARG}
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

if [ -z "${SNAPSHOT_REF}" ]; then
  echo "snapshot-ref is required"
  usage
  exit 1
fi

echo "Update catalog ${CATALOG_FILE} from snapshot ${SNAPSHOT_REF}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CATALOG_INITIAL_FILE="${SCRIPT_DIR}/operator.yaml"

# cache the snapshot from Konflux
TMP_SNAPSHOT_JSON=$(mktemp)
oc get snapshot ${SNAPSHOT_REF} -o json >"${TMP_SNAPSHOT_JSON}"
if [ $? -ne 0 ]; then
  echo "Failed to get snapshot ${SNAPSHOT_REF}"
  echo "Please make sure the snapshot exists and the snapshot name is correct"
  echo "Need to login Konflux through oc login, proxy command to be found here: https://registration-service-toolchain-host-operator.apps.stone-prd-host1.wdlc.p1.openshiftapps.com/"
  exit 1
fi

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

: ${OPM:=$(command -v opm)}
echo "using opm from ${OPM}"
# check if opm version is v1.39.0 or exit
if ! ${OPM} version | grep -q "v1.27.1"; then
  echo "opm version v1.27.1 is required"
  exit 1
fi

: ${YQ:=$(command -v yq)}
echo "using yq from ${YQ}"
# check if yq exists
if [ -z "${YQ}" ]; then
  echo "yq is required"
  exit 1
fi

BUNDLE_IMAGE_BASE="registry.redhat.io/openshift-lightspeed-beta/lightspeed-operator-bundle"
OPERATOR_IMAGE_BASE="registry.redhat.io/openshift-lightspeed-beta/lightspeed-rhel9-operator"
CONSOLE_IMAGE_BASE="registry.redhat.io/openshift-lightspeed-beta/lightspeed-console-plugin-rhel9"
SERVICE_IMAGE_BASE="registry.redhat.io/openshift-lightspeed-beta/lightspeed-service-api-rhel9"

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
${OPM} render ${BUNDLE_IMAGE_ORIGIN} --output=yaml >"${TEMP_BUNDLE_FILE}"

BUNDLE_VERSION=$(yq '.properties[]| select(.type=="olm.package")| select(.value.packageName=="lightspeed-operator") |.value.version' ${TEMP_BUNDLE_FILE})
echo "Bundle version is ${BUNDLE_VERSION}"

# restore bundle image to the catalog file
${YQ} eval -i '.image='"\"${BUNDLE_IMAGE}\"" "${TEMP_BUNDLE_FILE}"
# restore bundle related images and the bundle itself to the catalog file
${YQ} eval -i '.relatedImages='"${RELATED_IMAGES}" "${TEMP_BUNDLE_FILE}"

#Initialize catalog file from hack/operator.yaml
cat ${CATALOG_INITIAL_FILE} >"${CATALOG_FILE}"

cat ${TEMP_BUNDLE_FILE} >>"${CATALOG_FILE}"

cat <<EOF >>"${CATALOG_FILE}"
---
schema: olm.channel
package: lightspeed-operator
name: preview
entries:
  - name: lightspeed-operator.v${BUNDLE_VERSION}
EOF

${OPM} validate "$(dirname "${CATALOG_FILE}")"
if [ $? -ne 0 ]; then
  echo "Validation failed for ${CATALOG_FILE}"
  exit 1
else
  echo "Validation passed for ${CATALOG_FILE}"
fi
