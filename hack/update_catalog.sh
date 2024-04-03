# Helper tool to build the File Based Catalog (FBC) for the operator with semantic versioning
# Example , if you want to build catalog image for bundle image, set the
# VERSION variable to 0.0.2 and run the script. The script will build the bundle image and
# catalog image and push it to the quay.io registry.
# Pre-requisites: opm, podman, make
# Usage: ./hack/update_catalog.sh

#!/bin/bash

# Backup files before running script
backup() {
  echo "Backing up files"
  mkdir -p backup
  cp "${CSV_FILE}" backup/lightspeed-operator.clusterserviceversion.yaml.bak
  cp "${KUSTOMIZATION_FILE}" backup/kustomization.yaml.bak
  cp "${CATALOG_FILE}" backup/operator.yaml.bak
  cp "${OLS_CONFIG_FILE}" backup/ols.openshift.io_olsconfigs.yaml.bak
}


# Reverse backup files
restore() {
  echo "Restoring files"
  cp backup/lightspeed-operator.clusterserviceversion.yaml.bak "${CSV_FILE}"
  cp backup/kustomization.yaml.bak "${KUSTOMIZATION_FILE}"
  cp backup/operator.yaml.bak "${CATALOG_FILE}"
  cp backup/ols.openshift.io_olsconfigs.yaml.bak "${OLS_CONFIG_FILE}"
  rm -rf backup
}

trap restore EXIT

set -euo pipefail
DEFAULT_PLATFORM="linux/amd64"
export QUAY_USER=""  # Set quay user for personal builds
VERSION="0.0.1"    # Set the bundle version
QUAY_USER="${QUAY_USER:-openshift}"

OPERATOR_IMAGE="quay.io/openshift/lightspeed-operator:latest"
BUNDLE_IMAGE="quay.io/${QUAY_USER}/lightspeed-operator-bundle:v${VERSION}"
CATALOG_IMAGE="quay.io/${QUAY_USER}/lightspeed-catalog:${VERSION}"

CSV_FILE="bundle/manifests/lightspeed-operator.clusterserviceversion.yaml"
CATALOG_FILE="lightspeed-catalog/operator.yaml"
CATALOG_DOCKER_FILE="lightspeed-catalog.Dockerfile"
OLS_CONFIG_FILE="bundle/manifests/ols.openshift.io_olsconfigs.yaml"
KUSTOMIZATION_FILE="config/manager/kustomization.yaml"

echo "Backup files before running the script"
backup


echo "Building File Based Catalog (FBC) for ${USER}"

make bundle-build bundle-push  VERSION="${VERSION}" BUNDLE_IMG="${BUNDLE_IMAGE}"

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
echo "Catalog image ${CATALOG_IMAGE} pushed successfully"
