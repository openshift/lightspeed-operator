# Helper tool to build bundle and catalog image and push it to the quay.io registry
# for given tag of the operator and operands.
# Example , 
# Pre-requisites: opm, podman, make

#!/bin/bash

usage() {
  echo "Usage:
  -o, --org:
    Specify the quay org the bundle+catalog images will be pushed to (default: openshift)
  -t, --target-tag:
    Specify the image tag for the bundle+catalog images (default: internal-preview) 
  --ols-image: 
    The full pull spec of the ols api server image (default: quay.io/openshift/lightspeed-service-api:internal-preview)
  --console-image:
    The full pull spec of the console image (default: quay.io/openshift/lightspeed-console-plugin:internal-preview)
  --operator-image:
    The full pull spec of the operator image (default: quay.io/openshift/lightspeed-operator:internal-preview)
  --re-generate:
    If set, the bundle and catalog artifacts will be regenerated (default: false)  
  -p, --publish:
    If set, the bundle+catalog images will be pushed to quay (default: false)
  -v, --version:
    The version identifier to use for the bundle (default: 0.0.1)

"
}
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
        -r | --re-generate )    REGENERATE="true"
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
  if [[ $1 == "true" ]]; then
    cp "${KUSTOMIZATION_FILE}" backup/kustomization.yaml.bak
    cp "${OLS_CONFIG_FILE}" backup/ols.openshift.io_olsconfigs.yaml.bak
  fi
}

# Reverse backup files
restore() {
  echo "Restoring files"
  cp backup/lightspeed-operator.clusterserviceversion.yaml.bak "${CSV_FILE}"
  cp backup/index.yaml.bak "${CATALOG_FILE}"
    if [[ $1 == "true" ]]; then
        cp backup/kustomization.yaml.bak "${KUSTOMIZATION_FILE}"
        cp backup/ols.openshift.io_olsconfigs.yaml.bak "${OLS_CONFIG_FILE}"
    fi
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
QUAY_ORG=${QUAY_ORG:-"openshift"}
TARGET_TAG=${TARGET_TAG:-"internal-preview"}  # Set the target tag for the bundle and catalog image

# Set the images for the operator and operands
OLS_IMAGE=${OLS_IMAGE:-"quay.io/openshift/lightspeed-service-api:internal-preview"}
CONSOLE_IMAGE=${CONSOLE_IMAGE:-"quay.io/openshift/lightspeed-console-plugin:internal-preview"}
OPERATOR_IMAGE=${OPERATOR_IMAGE:-"quay.io/openshift/lightspeed-operator:internal-preview"}
PUBLISH=${PUBLISH:-"false"}
REGENERATE=${REGENERATE:-"false"}

echo "====== inputs ======="
echo "OLS_IMAGE=${OLS_IMAGE}"
echo "CONSOLE_IMAGE=${OLS_IMAGE}"
echo "OPERATOR_IMAGE=${OLS_IMAGE}"
echo ""
echo "====== outputs ======="
echo "VERSION=${VERSION}"
echo "QUAY_ORG=${QUAY_ORG}"
echo "TARGET_TAG=${TARGET_TAG}"
echo "PUBLISH=${PUBLISH}"
echo "REGENERATE=${REGENERATE}"


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

backup "${REGENERATE}"
trap 'restore "${REGENERATE}"' EXIT



OPERANDS="lightspeed-service=${OLS_IMAGE},console-plugin=${CONSOLE_IMAGE}"
#replace the operand images in the CSV file
sed -i "s|--images=.*|--images=${OPERANDS}|g" "${CSV_FILE}"

#Replace operator in CSV file
sed -i "s|image: quay.io/openshift/lightspeed-operator:latest|image: ${OPERATOR_IMAGE}|g" "${CSV_FILE}"

#Replace version in CSV file
sed -i "s|0.0.1|${VERSION}|g" "${CSV_FILE}"

if [[ ${REGENERATE} == "true" ]]; then
  rm -rf ./bundle
  make bundle VERSION="${VERSION}" BUNDLE_IMG="${TARGET_BUNDLE_IMAGE}"
fi

make bundle-build VERSION="${VERSION}" BUNDLE_IMG="${TARGET_BUNDLE_IMAGE}"

if [[ ${PUBLISH} == "false" ]] ; then
  echo "Bundle image ${TARGET_BUNDLE_IMAGE} built successfully , skipping the push to quay.io"
  exit 0
else
  echo "Pushing bundle image ${TARGET_BUNDLE_IMAGE}"
  podman push "${TARGET_BUNDLE_IMAGE}"
  echo "Pushed image ${TARGET_BUNDLE_IMAGE} pushed successfully"
fi


echo "Building catalog image with  ${TARGET_BUNDLE_IMAGE}"

#Append ./hack/operator.yaml to lightspeed-catalog/index.yaml
cat "${CATALOG_INTIAL_FILE}" > "${CATALOG_FILE}"

opm render "${TARGET_BUNDLE_IMAGE}" --output=yaml >> "${CATALOG_FILE}"
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
echo "Catalog image ${TARGET_CATALOG_IMAGE} pushed successfully"
