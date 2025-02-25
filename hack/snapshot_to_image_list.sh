#!/usr/bin/env bash

usage() {
    echo "Usage: $0 -s <snapshot-ref> -b <bundle-snapshot-ref> -o <output-file> -p"
    echo "  -s snapshot-ref: required, the snapshot's references, example: ols-cq8sl"
    echo "  -b bundle-snapshot-ref: required, the ols-bundle snapshot's references, example: ols-bundle-wf8st"
    echo "  -o output-file: optional, the catalog index file to update, default is empty (output to stdout)"
    echo "  -p: optional, transform URIs to production registry"
    echo "  -h: Show this help message"
    echo "Example: $0 -s ols-cq8sl -b ols-bundle-wf8st -o related_images.json"
}

if [ $# == 0 ]; then
    usage
    exit 1
fi

SNAPSHOT_REF=""
OUTPUT_FILE=""
USE_PRODUCTION_REGISTRY="false"

while getopts ":s:b:o:ph" argname; do
    case "$argname" in
    "s")
        SNAPSHOT_REF=${OPTARG}
        ;;
    "b")
        BUNDLE_SNAPSHOT_REF=${OPTARG}
        ;;
    "o")
        OUTPUT_FILE=${OPTARG}
        ;;
    "p")
        USE_PRODUCTION_REGISTRY="true"
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

if [ -z "${BUNDLE_SNAPSHOT_REF}" ]; then
    echo "bundle-snapshot-ref is required"
    usage
    exit 1
fi

: ${JQ:=$(command -v jq)}
# check if jq exists
if [ -z "${JQ}" ]; then
    echo "jq is required"
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# temporary file for snapshot info from Konflux
TMP_SNAPSHOT_JSON=$(mktemp)
TMP_BUNDLE_SNAPSHOT_JSON=$(mktemp)

cleanup() {
    # remove temporary snapshot file
    if [ -n "${TMP_SNAPSHOT_JSON}" ]; then
        rm -f "${TMP_SNAPSHOT_JSON}"
    fi
    if [ -n "${TMP_BUNDLE_SNAPSHOT_JSON}" ]; then
        rm -f "${TMP_BUNDLE_SNAPSHOT_JSON}"
    fi
}

trap cleanup EXIT

# cache the snapshot from Konflux
oc get snapshot ${SNAPSHOT_REF} -o json >"${TMP_SNAPSHOT_JSON}"

if [ $? -ne 0 ]; then
    echo "Failed to get snapshot ${SNAPSHOT_REF}"
    echo "Please make sure the snapshot exists and the snapshot name is correct"
    echo "Need to login Konflux through oc login, proxy command to be found here: https://registration-service-toolchain-host-operator.apps.stone-prd-host1.wdlc.p1.openshiftapps.com/"
    exit 1
fi

oc get snapshot ${BUNDLE_SNAPSHOT_REF} -o json >"${TMP_BUNDLE_SNAPSHOT_JSON}"
if [ $? -ne 0 ]; then
    echo "Failed to get snapshot ${BUNDLE_SNAPSHOT_REF}"
    echo "Please make sure the bundle snapshot exists and the snapshot name is correct"
    exit 1
fi

cp ${TMP_SNAPSHOT_JSON} snapshot.json
cp ${TMP_BUNDLE_SNAPSHOT_JSON} bundle_snapshot.json

BUNDLE_IMAGE=$(${JQ} -r '.spec.components[]| select(.name=="ols-bundle") | .containerImage' "${TMP_BUNDLE_SNAPSHOT_JSON}")
OPERATOR_IMAGE=$(${JQ} -r '.spec.components[]| select(.name=="lightspeed-operator") | .containerImage' "${TMP_SNAPSHOT_JSON}")
CONSOLE_IMAGE=$(${JQ} -r '.spec.components[]| select(.name=="lightspeed-console") | .containerImage' "${TMP_SNAPSHOT_JSON}")
SERVICE_IMAGE=$(${JQ} -r '.spec.components[]| select(.name=="lightspeed-service") | .containerImage' "${TMP_SNAPSHOT_JSON}")
if [ "${USE_PRODUCTION_REGISTRY}" = "true" ]; then
    BUNDLE_IMAGE_BASE="registry.redhat.io/openshift-lightspeed-tech-preview/lightspeed-operator-bundle"
    OPERATOR_IMAGE_BASE="registry.redhat.io/openshift-lightspeed-tech-preview/lightspeed-rhel9-operator"
    CONSOLE_IMAGE_BASE="registry.redhat.io/openshift-lightspeed-tech-preview/lightspeed-console-plugin-rhel9"
    SERVICE_IMAGE_BASE="registry.redhat.io/openshift-lightspeed-tech-preview/lightspeed-service-api-rhel9"

    BUNDLE_IMAGE=$(sed 's|quay\.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols-bundle|'"${BUNDLE_IMAGE_BASE}"'|g' <<<${BUNDLE_IMAGE})
    OPERATOR_IMAGE=$(sed 's|quay\.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols/lightspeed-operator|'"${OPERATOR_IMAGE_BASE}"'|g' <<<${OPERATOR_IMAGE})
    CONSOLE_IMAGE=$(sed 's|quay\.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols/lightspeed-console|'"${CONSOLE_IMAGE_BASE}"'|g' <<<${CONSOLE_IMAGE})
    SERVICE_IMAGE=$(sed 's|quay\.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols/lightspeed-service|'"${SERVICE_IMAGE_BASE}"'|g' <<<${SERVICE_IMAGE})

fi

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

if [ -n "${OUTPUT_FILE}" ]; then
    ${JQ} <<<$RELATED_IMAGES >${OUTPUT_FILE}
else
    ${JQ} <<<$RELATED_IMAGES
fi
