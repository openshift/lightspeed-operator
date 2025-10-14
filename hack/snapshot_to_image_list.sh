#!/usr/bin/env bash

usage() {
    echo "Usage: $0 -s <snapshot-ref> -b <bundle-snapshot-ref> -o <output-file> -p"
    echo "  -s snapshot-ref: required, the snapshot's references, example: ols-cq8sl"
    echo "  -b bundle-snapshot-ref: optional, the ols-bundle snapshot's references, example: ols-bundle-wf8st"
    echo "  -o output-file: optional, the catalog index file to update, default is empty (output to stdout)"
    echo "  -r: optional, use which registry: stable, preview, ci"
    echo "  -h: Show this help message"
    echo "Example: $0 -s ols-cq8sl -b ols-bundle-wf8st -o related_images.json"
}

if [ $# == 0 ]; then
    usage
    exit 1
fi

SNAPSHOT_REF=""
OUTPUT_FILE=""
USE_REGISTRY="ci"
KONFLUX_NAMESPACE="crt-nshift-lightspeed-tenant"

while getopts ":s:b:o:r:h" argname; do
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
    "r")
        USE_REGISTRY=${OPTARG}
        if [[ "${USE_REGISTRY}" != "stable" && "${USE_REGISTRY}" != "preview" && "${USE_REGISTRY}" != "ci" ]]; then
            echo "Invalid registry option: ${USE_REGISTRY}. Use 'stable', 'preview', or 'ci'."
            usage
            exit 1
        fi
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
    echo "bundle-snapshot-ref is not specified, will not update bundle image"
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
oc get -n ${KONFLUX_NAMESPACE} snapshot ${SNAPSHOT_REF} -o json >"${TMP_SNAPSHOT_JSON}"

if [ $? -ne 0 ]; then
    echo "Failed to get snapshot ${SNAPSHOT_REF}"
    echo "Please make sure the snapshot exists and the snapshot name is correct"
    echo "Need to login Konflux through oc login, proxy command to be found here: https://registration-service-toolchain-host-operator.apps.stone-prd-host1.wdlc.p1.openshiftapps.com/"
    exit 1
fi

if [ -n "${BUNDLE_SNAPSHOT_REF}" ]; then
    oc get -n ${KONFLUX_NAMESPACE} snapshot ${BUNDLE_SNAPSHOT_REF} -o json >"${TMP_BUNDLE_SNAPSHOT_JSON}"
    if [ $? -ne 0 ]; then
        echo "Failed to get snapshot ${BUNDLE_SNAPSHOT_REF}"
        echo "Please make sure the bundle snapshot exists and the snapshot name is correct"
        exit 1
    fi
fi

cp ${TMP_SNAPSHOT_JSON} snapshot.json
cp ${TMP_BUNDLE_SNAPSHOT_JSON} bundle_snapshot.json

if [ -n "${BUNDLE_SNAPSHOT_REF}" ]; then
    BUNDLE_IMAGE=$(${JQ} -r '.spec.components[]| select(.name=="ols-bundle") | .containerImage' "${TMP_BUNDLE_SNAPSHOT_JSON}")
    BUNDLE_REVISION=$(${JQ} -r '.spec.components[]| select(.name=="ols-bundle") | .source.git.revision' "${TMP_BUNDLE_SNAPSHOT_JSON}")
fi
OPERATOR_IMAGE=$(${JQ} -r '.spec.components[]| select(.name=="lightspeed-operator") | .containerImage' "${TMP_SNAPSHOT_JSON}")
OPERATOR_REVISION=$(${JQ} -r '.spec.components[]| select(.name=="lightspeed-operator") | .source.git.revision' "${TMP_SNAPSHOT_JSON}")
CONSOLE_IMAGE=$(${JQ} -r '.spec.components[]| select(.name=="lightspeed-console") | .containerImage' "${TMP_SNAPSHOT_JSON}")
CONSOLE_REVISION=$(${JQ} -r '.spec.components[]| select(.name=="lightspeed-console") | .source.git.revision' "${TMP_SNAPSHOT_JSON}")
SERVICE_IMAGE=$(${JQ} -r '.spec.components[]| select(.name=="lightspeed-service") | .containerImage' "${TMP_SNAPSHOT_JSON}")
SERVICE_REVISION=$(${JQ} -r '.spec.components[]| select(.name=="lightspeed-service") | .source.git.revision' "${TMP_SNAPSHOT_JSON}")
OPENSHIFT_MCP_SERVER_IMAGE=$(${JQ} -r '.spec.components[]| select(.name=="openshift-mcp-server") | .containerImage' "${TMP_SNAPSHOT_JSON}")
OPENSHIFT_MCP_SERVER_REVISION=$(${JQ} -r '.spec.components[]| select(.name=="openshift-mcp-server") | .source.git.revision' "${TMP_SNAPSHOT_JSON}")
DATAVERSE_EXPORTER_IMAGE=$(${JQ} -r '.spec.components[]| select(.name=="lightspeed-to-dataverse-exporter") | .containerImage' "${TMP_SNAPSHOT_JSON}")
DATAVERSE_EXPORTER_REVISION=$(${JQ} -r '.spec.components[]| select(.name=="lightspeed-to-dataverse-exporter") | .source.git.revision' "${TMP_SNAPSHOT_JSON}")
if [ "${USE_REGISTRY}" = "preview" ]; then
    OPERATOR_IMAGE_BASE="registry.redhat.io/openshift-lightspeed-tech-preview/lightspeed-rhel9-operator"
    CONSOLE_IMAGE_BASE="registry.redhat.io/openshift-lightspeed-tech-preview/lightspeed-console-plugin-rhel9"
    SERVICE_IMAGE_BASE="registry.redhat.io/openshift-lightspeed-tech-preview/lightspeed-service-api-rhel9"
    OPENSHIFT_MCP_SERVER_IMAGE_BASE="registry.redhat.io/openshift-lightspeed-tech-preview/openshift-mcp-server-rhel9"
    DATAVERSE_EXPORTER_IMAGE_BASE="registry.redhat.io/openshift-lightspeed-tech-preview/lightspeed-to-dataverse-exporter-rhel9"

    OPERATOR_IMAGE=$(sed 's|quay\.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols/lightspeed-operator|'"${OPERATOR_IMAGE_BASE}"'|g' <<<${OPERATOR_IMAGE})
    CONSOLE_IMAGE=$(sed 's|quay\.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols/lightspeed-console|'"${CONSOLE_IMAGE_BASE}"'|g' <<<${CONSOLE_IMAGE})
    SERVICE_IMAGE=$(sed 's|quay\.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols/lightspeed-service|'"${SERVICE_IMAGE_BASE}"'|g' <<<${SERVICE_IMAGE})
    OPENSHIFT_MCP_SERVER_IMAGE=$(sed 's|quay\.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/openshift-mcp-server|'"${OPENSHIFT_MCP_SERVER_IMAGE_BASE}"'|g' <<<${OPENSHIFT_MCP_SERVER_IMAGE})
    DATAVERSE_EXPORTER_IMAGE=$(sed 's|quay\.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/lightspeed-to-dataverse-exporter|'"${DATAVERSE_EXPORTER_IMAGE_BASE}"'|g' <<<${DATAVERSE_EXPORTER_IMAGE})
    POSTGRES_IMAGE=$(sed "s|quay\.io.*/lightspeed-postgresql|registry.redhat.io/rhel9/postgresql-16|g" <<<"${POSTGRES_IMAGE}")

    if [ -n "${BUNDLE_SNAPSHOT_REF}" ]; then
        BUNDLE_IMAGE_BASE="registry.redhat.io/openshift-lightspeed-tech-preview/lightspeed-operator-bundle"
        BUNDLE_IMAGE=$(sed 's|quay\.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols-bundle|'"${BUNDLE_IMAGE_BASE}"'|g' <<<${BUNDLE_IMAGE})
    fi
fi

if [ "${USE_REGISTRY}" = "stable" ]; then
    OPERATOR_IMAGE_BASE="registry.redhat.io/openshift-lightspeed/lightspeed-rhel9-operator"
    CONSOLE_IMAGE_BASE="registry.redhat.io/openshift-lightspeed/lightspeed-console-plugin-rhel9"
    SERVICE_IMAGE_BASE="registry.redhat.io/openshift-lightspeed/lightspeed-service-api-rhel9"
    OPENSHIFT_MCP_SERVER_IMAGE_BASE="registry.redhat.io/openshift-lightspeed/openshift-mcp-server-rhel9"
    DATAVERSE_EXPORTER_IMAGE_BASE="registry.redhat.io/openshift-lightspeed/lightspeed-to-dataverse-exporter-rhel9"

    OPERATOR_IMAGE=$(sed 's|quay\.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols/lightspeed-operator|'"${OPERATOR_IMAGE_BASE}"'|g' <<<${OPERATOR_IMAGE})
    CONSOLE_IMAGE=$(sed 's|quay\.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols/lightspeed-console|'"${CONSOLE_IMAGE_BASE}"'|g' <<<${CONSOLE_IMAGE})
    SERVICE_IMAGE=$(sed 's|quay\.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols/lightspeed-service|'"${SERVICE_IMAGE_BASE}"'|g' <<<${SERVICE_IMAGE})
    OPENSHIFT_MCP_SERVER_IMAGE=$(sed 's|quay\.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/openshift-mcp-server|'"${OPENSHIFT_MCP_SERVER_IMAGE_BASE}"'|g' <<<${OPENSHIFT_MCP_SERVER_IMAGE})
    DATAVERSE_EXPORTER_IMAGE=$(sed 's|quay\.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/lightspeed-to-dataverse-exporter|'"${DATAVERSE_EXPORTER_IMAGE_BASE}"'|g' <<<${DATAVERSE_EXPORTER_IMAGE})
    POSTGRES_IMAGE=$(sed "s|quay\.io.*/lightspeed-postgresql|registry.redhat.io/rhel9/postgresql-16|g" <<<"${POSTGRES_IMAGE}")

    if [ -n "${BUNDLE_SNAPSHOT_REF}" ]; then
        BUNDLE_IMAGE_BASE="registry.redhat.io/openshift-lightspeed/lightspeed-operator-bundle"
        BUNDLE_IMAGE=$(sed 's|quay\.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols-bundle|'"${BUNDLE_IMAGE_BASE}"'|g' <<<${BUNDLE_IMAGE})
    fi
fi

if [ -z "${POSTGRES_IMAGE}" ] || [ "${POSTGRES_IMAGE}" == "null" ]; then
    if [ -f "${OUTPUT_FILE}" ]; then
        POSTGRES_IMAGE=$(jq -r '.[] | select(.name == "lightspeed-postgresql") | .image' "${OUTPUT_FILE}")
    fi
fi

if [ -z "${POSTGRES_IMAGE}" ] || [ "${POSTGRES_IMAGE}" == "null" ]; then
    DEFAULT_POSTGRES_IMAGE=$(grep -o 'PostgresServerImageDefault = "registry[^"]*"' "${SCRIPT_DIR}/../internal/controller/constants.go" | sed 's/PostgresServerImageDefault = "\(.*\)"/\1/')
    POSTGRES_IMAGE="${DEFAULT_POSTGRES_IMAGE}"
fi

RELATED_IMAGES=$(
    cat <<-EOF
[
  {
    "name": "lightspeed-service-api",
    "image": "${SERVICE_IMAGE}",
    "revision": "${SERVICE_REVISION}"
  },
  {
    "name": "lightspeed-console-plugin",
    "image": "${CONSOLE_IMAGE}",
    "revision": "${CONSOLE_REVISION}"
  },
  {
    "name": "lightspeed-operator",
    "image": "${OPERATOR_IMAGE}",
    "revision": "${OPERATOR_REVISION}"
  },
  {
    "name": "openshift-mcp-server",
    "image": "${OPENSHIFT_MCP_SERVER_IMAGE}",
    "revision": "${OPENSHIFT_MCP_SERVER_REVISION}"
  },
  {
    "name": "lightspeed-to-dataverse-exporter",
    "image": "${DATAVERSE_EXPORTER_IMAGE}",
    "revision": "${DATAVERSE_EXPORTER_REVISION}"
  },
  {
    "name": "lightspeed-postgresql",
    "image": "${POSTGRES_IMAGE}"
  }
]
EOF
)

if [ -n "${BUNDLE_IMAGE}" ]; then
    RELATED_IMAGES=$(echo "${RELATED_IMAGES}" | ${JQ} \
        --arg img "${BUNDLE_IMAGE}" \
        --arg rev "${BUNDLE_REVISION}" \
        '. += [{"name":"lightspeed-operator-bundle","image":$img,"revision":$rev}]')
fi

if [ -n "${OUTPUT_FILE}" ]; then
    ${JQ} <<<$RELATED_IMAGES >${OUTPUT_FILE}
else
    ${JQ} <<<$RELATED_IMAGES
fi