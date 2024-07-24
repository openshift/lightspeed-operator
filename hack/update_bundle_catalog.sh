# Helper tool to update the catalog artifacts and bundle artifacts
# These genereated artifacts are used to build the catalog image
# Pre-requisites: opm, make
# Usage: ./hack/update_bundle_catalog.sh

#!/bin/bash

set -euo pipefail

TEMP_BUNDLE_FILE=""
cleanup() {
  # remove temporary bundle file
  if [ -n "${TEMP_BUNDLE_FILE}" ]; then
    rm -f "${TEMP_BUNDLE_FILE}"
  fi

}

trap cleanup EXIT

: ${OPM:=$(command -v opm)}
echo "using opm from ${OPM}"
# check if opm version is v1.39.0 or exit
if ! ${OPM} version | grep -q "v1.39.0"; then
  echo "opm version v1.39.0 is required"
  exit 1
fi

: ${YQ:=$(command -v yq)}
echo "using yq from ${YQ}"
# check if yq exists
if [ -z "${YQ}" ]; then
  echo "yq is required"
  exit 1
fi

# Set the bundle version
: "${BUNDLE_TAG:=0.0.1}"

: "${OPERATOR_IMAGE:=registry.redhat.io/openshift-lightspeed-beta/lightspeed-rhel9-operator@sha256:5c0fcd208cd93fe6b08f0404a0ae50165973104ebfebe6bdbe30bfa92019eea2}"
: "${BUNDLE_IMAGE:=registry.redhat.io/openshift-lightspeed-beta/lightspeed-operator-bundle@sha256:e46e337502a00282473e083c16a64e6201df3677905e4b056b7f48ef0b8f6e4b}"
: "${CONSOLE_IMAGE:=registry.redhat.io/openshift-lightspeed-beta/lightspeed-console-plugin-rhel9@sha256:4f45c9ba068cf92e592bb3a502764ce6bc93cd154d081fa49d05cb040885155b}"

CATALOG_FILE="lightspeed-catalog/index.yaml"
CATALOG_INITIAL_FILE="hack/operator.yaml"
CSV_FILE="bundle/manifests/lightspeed-operator.clusterserviceversion.yaml"

BUNDLE_DOCKERFILE="bundle.Dockerfile"

# if RELATED_IMAGES is not defined, extract related images or use default values
: "${RELATED_IMAGES:=$(${YQ} ' .spec.relatedImages' -ojson ${CSV_FILE})}"
if [ -z "${RELATED_IMAGES}" ]; then
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
  }
]
EOF
  )
fi

# Build the bundle image
echo "Updating bundle artifacts for image ${OPERATOR_IMAGE}"
rm -rf ./bundle
make bundle VERSION="${BUNDLE_TAG}" IMG="${OPERATOR_IMAGE}"
# restore related images to the CSV file
${YQ} eval -i '.spec.relatedImages='"${RELATED_IMAGES}" ${CSV_FILE}

# use UBI image as base image for bundle image
: "${BASE_IMAGE:=registry.access.redhat.com/ubi9/ubi-minimal@sha256:104cf11d890aeb7dd5728b7d7732e175a0e4018f1bb00d2faebcc8f6bf29bd52}"
sed -i 's|^FROM scratch|FROM '"${BASE_IMAGE}"'|' ${BUNDLE_DOCKERFILE}

# make bundle image comply with enterprise contract requirements
cat <<EOF >>${BUNDLE_DOCKERFILE}

# licenses required by Red Hat certification policy
# refer to https://docs.redhat.com/en/documentation/red_hat_software_certification/2024/html-single/red_hat_openshift_software_certification_policy_guide/index#con-image-content-requirements_openshift-sw-cert-policy-container-images
COPY LICENSE /licenses/

# Labels for enterprise contract
LABEL com.redhat.component=openshift-lightspeed
LABEL description="Red Hat OpenShift Lightspeed - AI assistant for managing OpenShift clusters."
LABEL distribution-scope=public
LABEL io.k8s.description="Red Hat OpenShift Lightspeed - AI assistant for managing OpenShift clusters."
LABEL io.k8s.display-name="Openshift Lightspeed"
LABEL io.openshift.tags="openshift,lightspeed,ai,assistant"
LABEL name=openshift-lightspeed
LABEL release=0.0.1
LABEL url="https://github.com/openshift/lightspeed-operator"
LABEL vendor="Red Hat"
LABEL version=0.0.1
LABEL summary="Red Hat OpenShift Lightspeed"

# Set user to non-root for security reasons.
USER 1001
EOF

echo "Adding bundle image to FBC using image ${BUNDLE_IMAGE}"
#Initialize lightspeed-catalog/index.yaml from hack/operator.yaml
cat "${CATALOG_INITIAL_FILE}" >"${CATALOG_FILE}"

# This bundle image is used to build the catalog image
# Give it a reference in a writable image registry
TEMP_BUNDLE_IMG=${TEMP_BUNDLE_IMG:-}
if [ -z "${TEMP_BUNDLE_IMG}" ]; then
  TEMP_BUNDLE_IMG="quay.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols/bundle@sha256:c83533f0f96a7290886c5b651a3b5c8a6a4dd1058db24861b8f1d3ee3c86eaec"
  echo "No TEMP_BUNDLE_IMG specified. Catalog is built using default bundle image ${TEMP_BUNDLE_IMG}"
  echo "If you have changed the CRD, please specifiy TEMP_BUNDLE_IMG to your writable image registry and re-run the script"
fi

# create a temporary file for the bundle part of the catalog
TEMP_BUNDLE_FILE=$(mktemp)

${OPM} render "${TEMP_BUNDLE_IMG}" --output=yaml >"${TEMP_BUNDLE_FILE}"
# restore bundle image to the catalog file
${YQ} eval -i '.image='"\"${BUNDLE_IMAGE}\"" ${TEMP_BUNDLE_FILE}
# restore bundle related images and the bundle itself to the catalog file
${YQ} eval -i '.relatedImages='"${RELATED_IMAGES}" ${TEMP_BUNDLE_FILE}
${YQ} eval -i '.relatedImages += [{"name": "lightspeed-operator-bundle", "image": "'"${BUNDLE_IMAGE}"'"}]' ${TEMP_BUNDLE_FILE}

cat ${TEMP_BUNDLE_FILE} >>${CATALOG_FILE}

cat <<EOF >>"${CATALOG_FILE}"
---
schema: olm.channel
package: lightspeed-operator
name: preview
entries:
  - name: lightspeed-operator.v$BUNDLE_TAG
EOF
echo "Finished running $(basename "$0")"
