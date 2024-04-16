# Helper tool to build bundle and catalog image and push it to the quay.io registry
# for given SHA of the operator and operands.
# Example , 
# Pre-requisites: opm, podman, make
# Dry-run Usage: ./hack/publish_catalog_by_sha.sh 
# Publish Usage: ./hack/publish_catalog_by_sha.sh --publish

#!/bin/bash

# Backup files before running script
backup() {
  echo "Backing up files"
  mkdir -p backup
  cp "${CSV_FILE}" backup/lightspeed-operator.clusterserviceversion.yaml.bak
  cp "${CATALOG_FILE}" backup/operator.yaml.bak
}


# Reverse backup files
restore() {
  echo "Restoring files"
  cp backup/lightspeed-operator.clusterserviceversion.yaml.bak "${CSV_FILE}"
  cp backup/operator.yaml.bak "${CATALOG_FILE}"
  rm -rf backup
}

trap restore EXIT

set -euo pipefail

# Begin configuration
export QUAY_USER=""  # Set quay user for personal builds
VERSION="0.0.1"    # Set the bundle version - currently 0.0.1
QUAY_USER="${QUAY_USER:-openshift}"
TARGET_TAG=$(git rev-parse --short HEAD)  # Set the target tag for the bundle and catalog image

# Set the images for the operator and operands
OLS_IMAGE="quay.io/openshift/lightspeed-service-api@sha256:1c209bca3e34797cc417c89c4ed1664f280014ef74f15b12e654681ad6c7d4dd"
CONSOLE_IMAGE="quay.io/openshift/lightspeed-console-plugin@sha256:1fee74eec5c90d9bb2a81dccc6b5dfc8abc13d81ba97b2d464f9be774d482516"
OPERATOR_IMAGE="quay.io/openshift/lightspeed-operator@sha256:e6909a28d4b35f4e242b5dc7498e25d71a1284c0eabe52beef0275cd2c403130"

# End configuration


# If first arg --publish is set ,then the script pushes the images to the quay.io registry
PUBLISH="false"
if [ "$#" -eq 1 ] && [ "$1" == "--publish" ]; then
  PUBLISH="true"
fi
echo "PUBLISH=${PUBLISH}"

DEFAULT_PLATFORM="linux/amd64"
BUNDLE_BASE="quay.io/${QUAY_USER}/lightspeed-operator-bundle"
CATALOG_BASE="quay.io/${QUAY_USER}/lightspeed-catalog"
TARGET_BUNDLE_IMAGE="${BUNDLE_BASE}:${TARGET_TAG}"
TARGET_CATALOG_IMAGE="${CATALOG_BASE}:${TARGET_TAG}"

CSV_FILE="bundle/manifests/lightspeed-operator.clusterserviceversion.yaml"
CATALOG_FILE="lightspeed-catalog/operator.yaml"
CATALOG_DOCKER_FILE="lightspeed-catalog.Dockerfile"

echo "Backup files before running the script"
backup


OPERANDS="lightspeed-service=${OLS_IMAGE},console-plugin=${CONSOLE_IMAGE}"
#replace the operand images in the CSV file
sed -i.bak "s|--images=lightspeed-service=quay.io/openshift/lightspeed-service-api:latest|--images=${OPERANDS}|g" "${CSV_FILE}"

#Replace operator  in CSV file
sed -i.bak "s|image: quay.io/openshift/lightspeed-operator:latest|image: ${OPERATOR_IMAGE}|g" "${CSV_FILE}"
rm bundle/manifests/lightspeed-operator.clusterserviceversion.yaml.bak

make bundle-build   VERSION="${VERSION}" BUNDLE_IMG="${TARGET_BUNDLE_IMAGE}"
BUNDLE_SHA=$(podman inspect --format='{{index .RepoDigests 0}}' "${TARGET_BUNDLE_IMAGE}" | cut -d '@' -f 2)
TARGET_BUNDLE_SHA="quay.io/${QUAY_USER}/lightspeed-operator-bundle@${BUNDLE_SHA}"


if [[ ${PUBLISH} == "false" ]] ; then
  echo "Bundle image ${TARGET_BUNDLE_IMAGE} built successfully , skipping the push to quay.io"
  exit 0
else
  echo "Pushing bundle image ${TARGET_BUNDLE_SHA}"
  # Push the bundle image SHA and tag to fix the SHA from local to quay.io
  podman push "${TARGET_BUNDLE_SHA}"
  echo "Pushed image ${TARGET_BUNDLE_SHA} pushed successfully"
  podman push "${TARGET_BUNDLE_IMAGE}"
  echo "Pushed image ${TARGET_BUNDLE_IMAGE} pushed successfully"
fi


echo "Building catalog image with  ${TARGET_BUNDLE_SHA}"

cat << EOF > "${CATALOG_FILE}"
---
defaultChannel: preview
name: lightspeed-operator
schema: olm.package
EOF

opm render "${TARGET_BUNDLE_SHA}" --output=yaml >> "${CATALOG_FILE}"
cat << EOF >> "${CATALOG_FILE}"
---
schema: olm.channel
package: lightspeed-operator
name: preview
entries:
  - name: lightspeed-operator.v$VERSION
EOF

echo "Building catalog image ${TARGET_CATALOG_IMAGE}"
podman build --platform="${DEFAULT_PLATFORM}" -f "${CATALOG_DOCKER_FILE}" -t "${TARGET_CATALOG_IMAGE}" .
echo "Pushing catalog image ${TARGET_CATALOG_IMAGE}"
podman push "${TARGET_CATALOG_IMAGE}"
echo "Catalog image ${TARGET_CATALOG_IMAGE} pushed successfully"
