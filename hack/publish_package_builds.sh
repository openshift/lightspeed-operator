# Helper tool to build bundle and catalog image and push it to the quay.io registry
# for given tag of the operator and operands.
# Example , 
# Pre-requisites: opm, podman, make

#!/bin/bash

usage() {
  echo "Usage:
  -o, --org:
    Specify the quay org the bundle+catalog images will be pushed to (default: openshift-lightspeed)
  -t, --target-tag:
    Specify the image tag for the bundle+catalog images (default: internal-preview) 
  --ols-image: 
    The full pull spec of the ols api server image (default: quay.io/openshift-lightspeed/lightspeed-service-api:internal-preview)
  --console-image:
    The full pull spec of the console image (default: quay.io/openshift-lightspeed/lightspeed-console-plugin:internal-preview)
  --operator-image:
    The full pull spec of the operator image (default: quay.io/openshift-lightspeed/lightspeed-operator:internal-preview)
  --build-operator:
    If set, an operator build is built from source (default: false)
  -p, --publish:
    If set, the bundle+catalog images will be pushed to quay (default: false)
  -v, --version:
    The version identifier to use for the bundle (default: 0.0.1)

"
}

# Returns the SHA of the image tag passed as $1
get_image_sha() {
  # Make sure we have the latest version of the image locally because inspect operators on the local images
  ${CONTAINER_TOOL} pull $1
  SHARED_SHA=$(${CONTAINER_TOOL} inspect ${1} | jq -r .[0].RepoDigests[-1])
}
CONTAINER_TOOL="$(shell which podman >/dev/null 2>&1 && echo podman || echo docker)"


while [ "$1" != "" ]; do
    case $1 in
        -o | --org )            shift
                                QUAY_ORG="$1"
                                ;;
        -t | --target-tag )     shift
                                TARGET_TAG="$1"
                                ;;
        --ols-image )           shift
                                OLS_IMAGE="$1"
                                ;;
        --console-image )       shift
                                CONSOLE_IMAGE="$1"
                                ;;
        --operator-image )      shift
                                OPERATOR_IMAGE="$1"
                                ;;
        -b | --build-operator ) REBUILD="true"
                                ;;
        -p | --publish )        PUBLISH="true"
                                ;;
        -v | --version )        shift
                                VERSION="$1"
                                ;;
        * )                     usage
                                exit 1
    esac
    shift
done

# Backup files before running script
backup() {
  echo "Backing up files"
  mkdir -p backup
  cp "${CSV_FILE}" backup/lightspeed-operator.clusterserviceversion.yaml.bak
  cp "${CATALOG_FILE}" backup/index.yaml.bak
}

# Reverse backup files
restore() {
  echo "Restoring files"
  cp backup/lightspeed-operator.clusterserviceversion.yaml.bak "${CSV_FILE}"
  cp backup/index.yaml.bak "${CATALOG_FILE}"
  rm -rf backup
}

set -euo pipefail

# check if opm version is v1.39.0 or exit
if ! opm version | grep -q "v1.39.0"; then
  echo "opm version v1.39.0 is required"
  exit 1
fi

# Begin configuration
VERSION=${VERSION:-"0.0.1"}    # Set the bundle version - currently 0.0.1
QUAY_ORG=${QUAY_ORG:-"openshift-lightspeed"}
TARGET_TAG=${TARGET_TAG:-"internal-preview"}  # Set the target tag for the bundle and catalog image

# Set the images for the operator and operands
OLS_IMAGE=${OLS_IMAGE:-"quay.io/openshift-lightspeed/lightspeed-service-api:internal-preview"}
CONSOLE_IMAGE=${CONSOLE_IMAGE:-"quay.io/openshift-lightspeed/lightspeed-console-plugin:internal-preview"}
OPERATOR_IMAGE=${OPERATOR_IMAGE:-"quay.io/openshift-lightspeed/lightspeed-operator:internal-preview"}
PUBLISH=${PUBLISH:-"false"}
REBUILD=${REBUILD:-"false"}

echo "====== inputs ======="
echo "OLS_IMAGE=${OLS_IMAGE}"
echo "CONSOLE_IMAGE=${CONSOLE_IMAGE}"
echo "OPERATOR_IMAGE=${OPERATOR_IMAGE}"
echo ""
echo "====== outputs ======="
echo "VERSION=${VERSION}"
echo "QUAY_ORG=${QUAY_ORG}"
echo "TARGET_TAG=${TARGET_TAG}"
echo "PUBLISH=${PUBLISH}"
echo "REBUILD=${REBUILD}"


# End configuration


# If first arg --publish is set ,then the script pushes the images to the quay.io registry

DEFAULT_PLATFORM="linux/amd64"
BUNDLE_BASE="quay.io/${QUAY_ORG}/lightspeed-operator-bundle"
CATALOG_BASE="quay.io/${QUAY_ORG}/lightspeed-catalog"
TARGET_BUNDLE_IMAGE="${BUNDLE_BASE}:${TARGET_TAG}"
TARGET_CATALOG_IMAGE="${CATALOG_BASE}:${TARGET_TAG}"

CSV_FILE="bundle/manifests/lightspeed-operator.clusterserviceversion.yaml"
CATALOG_FILE="lightspeed-catalog/index.yaml"
CATALOG_DOCKER_FILE="lightspeed-catalog.Dockerfile"
CATALOG_INTIAL_FILE="hack/operator.yaml"
KUSTOMIZATION_FILE="config/manager/kustomization.yaml"
OLS_CONFIG_FILE="bundle/manifests/ols.openshift.io_olsconfigs.yaml"

echo "Backup files before running the script"

backup
trap restore EXIT


if [[ ${REBUILD} == "true" ]]; then
  make docker-build VERSION="${VERSION}" IMG="${OPERATOR_IMAGE}"
  if [[ ${PUBLISH} == "true" ]] ; then
  make docker-push VERSION="${VERSION}" IMG="${OPERATOR_IMAGE}"
  fi
fi

get_image_sha $OLS_IMAGE
OLS_IMAGE_SHA=$SHARED_SHA

get_image_sha $CONSOLE_IMAGE
CONSOLE_IMAGE_SHA=$SHARED_SHA

get_image_sha $OPERATOR_IMAGE
OPERATOR_IMAGE_SHA=$SHARED_SHA

OPERANDS="lightspeed-service=${OLS_IMAGE_SHA},console-plugin=${CONSOLE_IMAGE_SHA}"
#replace the operand images in the CSV file
sed -i "s|--images=.*|--images=${OPERANDS}|g" "${CSV_FILE}"

#Replace operator in CSV file
sed -i "s|image: quay.io/openshift-lightspeed/lightspeed-operator:latest|image: ${OPERATOR_IMAGE_SHA}|g" "${CSV_FILE}"

#Replace related images in CSV file
sed -i "s|quay.io/openshift-lightspeed/lightspeed-service-api:latest|${OLS_IMAGE_SHA}|g" "${CSV_FILE}"
sed -i "s|quay.io/openshift-lightspeed/lightspeed-console-plugin:latest|${CONSOLE_IMAGE_SHA}|g" "${CSV_FILE}"
sed -i "s|quay.io/openshift-lightspeed/lightspeed-operator:latest|${OPERATOR_IMAGE_SHA}|g" "${CSV_FILE}"

#Replace version in CSV file
sed -i "s|0.0.1|${VERSION}|g" "${CSV_FILE}"

make bundle-build VERSION="${VERSION}" BUNDLE_IMG="${TARGET_BUNDLE_IMAGE}"

if [[ ${PUBLISH} == "false" ]] ; then
  echo "Bundle image ${TARGET_BUNDLE_IMAGE} built successfully , skipping the push to quay.io"
  exit 0
else
  echo "Pushing bundle image ${TARGET_BUNDLE_IMAGE}"
  podman push "${TARGET_BUNDLE_IMAGE}"
  echo "Pushed image ${TARGET_BUNDLE_IMAGE} pushed successfully"
fi

# Get the bundle image sha.  Requires podman as docker doesn't use the same args/formatting

#CONTAINER_TOOL="$(shell which podman >/dev/null 2>&1 && echo podman || echo docker)"
#TARGET_BUNDLE_SHA=$(${CONTAINER_TOOL} inspect ${TARGET_BUNDLE_IMAGE} | jq -r .[0].RepoDigests[-1])
get_image_sha $TARGET_BUNDLE_IMAGE
TARGET_BUNDLE_IMAGE_SHA=$SHARED_SHA
if [ -z $TARGET_BUNDLE_IMAGE_SHA ]; then
  echo "Error getting bundle sha for bundle image ${TARGET_BUNDLE_IMAGE}"
  exit 1
fi

echo "Building catalog image with  ${TARGET_BUNDLE_IMAGE_SHA}"

#Initialize lightspeed-catalog/index.yaml from /hack/operator.yaml
cat "${CATALOG_INTIAL_FILE}" > "${CATALOG_FILE}"

opm render "${TARGET_BUNDLE_IMAGE_SHA}" --output=yaml >> "${CATALOG_FILE}"
cat << EOF >> "${CATALOG_FILE}"
---
schema: olm.channel
package: lightspeed-operator
name: preview
entries:
  - name: lightspeed-operator.v${VERSION}
EOF

echo "Building catalog image ${TARGET_CATALOG_IMAGE}"
podman build --platform="${DEFAULT_PLATFORM}" -f "${CATALOG_DOCKER_FILE}" -t "${TARGET_CATALOG_IMAGE}" .
if [[ ${PUBLISH} == "false" ]] ; then
  echo "Catalog image ${TARGET_CATALOG_IMAGE} built successfully , skipping the push to quay.io"
  exit 0
fi
echo "Pushing catalog image ${TARGET_CATALOG_IMAGE}"
podman push "${TARGET_CATALOG_IMAGE}"

echo
echo
echo "Catalog image ${TARGET_CATALOG_IMAGE} pushed successfully"

echo "Catalog image: ${TARGET_CATALOG_IMAGE}"
echo "Bundle image: ${TARGET_BUNDLE_IMAGE} / ${TARGET_BUNDLE_IMAGE_SHA}"
echo "Operator image: ${OPERATOR_IMAGE} / ${OPERATOR_IMAGE_SHA}"
echo "OLS image: ${OLS_IMAGE} / ${OLS_IMAGE_SHA}"
echo "Console image: ${CONSOLE_IMAGE} / ${CONSOLE_IMAGE_SHA}"
echo "Ensure all image SHAs have an associated tag other than \"latest\" so they are not GCed by quay!"
