# Helper tool to build the File Based Catalog (FBC) for the operator with semantic versioning
# Example , if you want to build catalog image for lishtspeed-operator version 0.0.2, set the
# VERSION variable to 0.0.2 and run the script. The script will build the bundle image and
# catalog image and push it to the quay.io registry.
# Pre-requisites: opm, podman, make
# Usage: ./hack/build_catalog.sh

#!/bin/bash
set -euo pipefail
DEFAULT_PLATFORM="linux/amd64"
export QUAY_USER=""  # Set quay user for personal builds
VERSION="0.0.1"    # Set the operator version
QUAY_USER="${QUAY_USER:-openshift}"

export IMAGE_TAG_BASE="quay.io/${QUAY_USER}/lightspeed-operator"
BUNDLE_IMAGE="quay.io/${QUAY_USER}/lightspeed-operator-bundle:v${VERSION}"
CATALOG_IMAGE="quay.io/${QUAY_USER}/lightspeed-catalog:${VERSION}"

CSV_FILE="bundle/manifests/lightspeed-operator.clusterserviceversion.yaml"
CATALOG_FILE="lightspeed-catalog/operator.yaml"
CATALOG_DOCKER_FILE="lightspeed-catalog.Dockerfile"

echo "Building File Based Catalog (FBC) for ${USER}"
echo "Set IMAGE_TAG_BASE: ${IMAGE_TAG_BASE}"

rm -rf ./bundle
make bundle VERSION="${VERSION}"
sed -i.bak "s/name: lightspeed-operator.v[0-9]+.[0-9]+.[0-9]+/name: lightspeed-operator.v${VERSION}/" "${CSV_FILE}"
sed -i.bak "s/version: [0-9]+.[0-9]+.[0-9]+/version: ${VERSION}/" "${CSV_FILE}"
rm -rf "${CSV_FILE}.bak"
echo "Built bundle image ${BUNDLE_IMAGE}"

make bundle-build bundle-push VERSION="${VERSION}" BUNDLE_IMAGE="${BUNDLE_IMAGE}"

echo "Adding bundle image to FBC using image ${BUNDLE_IMAGE}" 
if [ -f "${CATALOG_FILE}" ]; then
  rm -f "${CATALOG_FILE}"
fi
touch "${CATALOG_FILE}"
cat << EOF >> "${CATALOG_FILE}"
---
defaultChannel: preview
name: lightspeed-operator
schema: olm.package
EOF

opm render "${BUNDLE_IMAGE}" --output=yaml >> "${CATALOG_FILE}"
cat << EOF >> "${CATALOG_FILE}"
---
schema: olm.channel
package: lightspeed-operator
name: preview
entries:
  - name: lightspeed-operator.v$VERSION
EOF
echo "Building catalog image ${CATALOG_IMAGE}"
podman build --platform="${DEFAULT_PLATFORM}" -f "${CATALOG_DOCKER_FILE}" -t "${CATALOG_IMAGE}" .
echo "Pushing catalog image ${CATALOG_IMAGE}"
podman push "${CATALOG_IMAGE}"
