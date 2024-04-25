# Helper tool to update the catalog artifacts and bundle artifacts
# These genereated artifacts are used to build the catalog image
# Pre-requisites: opm, make
# Usage: ./hack/update_bundle_catalog.sh

#!/bin/bash

set -euo pipefail

# check if opm version is v1.39.0 or exit
if ! opm version | grep -q "v1.39.0"; then
  echo "opm version v1.39.0 is required"
  exit 1
fi

TAG="latest"  # Set the operator version
BUNDLE_TAG="0.0.1"    # Set the bundle version

OPERATOR_IMAGE="quay.io/openshift/lightspeed-operator:${TAG}"
BUNDLE_IMAGE="quay.io/openshift/lightspeed-operator-bundle:v${BUNDLE_TAG}"
CATALOG_FILE="lightspeed-catalog/index.yaml"
CATALOG_INTIAL_FILE="hack/operator.yaml"

# Build the bundle image
echo "Updating bundle artifcts for image ${OPERATOR_IMAGE}"
make bundle VERSION="${BUNDLE_TAG}" IMG="${OPERATOR_IMAGE}"

echo "Adding bundle image to FBC using image ${BUNDLE_IMAGE}" 

#Initialize lightspeed-catalog/index.yaml from hack/operator.yaml
cat "${CATALOG_INTIAL_FILE}" > "${CATALOG_FILE}"

opm render "${BUNDLE_IMAGE}" --output=yaml >> "${CATALOG_FILE}"
cat << EOF >> "${CATALOG_FILE}"
---
schema: olm.channel
package: lightspeed-operator
name: preview
entries:
  - name: lightspeed-operator.v$BUNDLE_TAG
EOF
echo "Finished running updated "
