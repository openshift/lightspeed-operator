# Helper tool to update the catalog artifacts and bundle artifacts
# These genereated artifacts are used to build the catalog image
# Pre-requisites: opm, make
# Usage: ./hack/update_bundle_catalog.sh

#!/bin/bash

set -euo pipefail

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

: "${OPERATOR_IMAGE:=quay.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols/lightspeed-operator@sha256:256e653c620a2e1ef3d9ef7024da5326af551f5fe11a5e2307e4bafd8930e381}"
: "${BUNDLE_IMAGE:=quay.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols/bundle@sha256:84a332cd52eb53888da7916d3bc7cfad32322c714dd6e69d025b8c2df882321e}"

CATALOG_FILE="lightspeed-catalog/index.yaml"
CATALOG_INITIAL_FILE="hack/operator.yaml"
CSV_FILE="bundle/manifests/lightspeed-operator.clusterserviceversion.yaml"

BUNDLE_DOCKERFILE="bundle.Dockerfile"

# if RELATED_IMAGES is not defined, extract related images or use default values
:  "${RELATED_IMAGES:=$(${YQ} ' .spec.relatedImages' -ojson ${CSV_FILE})}"
if [ -z "${RELATED_IMAGES}" ]; then
  RELATED_IMAGES=$(cat <<-EOF
[
  {
    "name": "lightspeed-service-api",
    "image": "quay.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols/lightspeed-service@sha256:d60104e8eef06e68107b127f1f19c6e3028bd6d7c36b263dddb89cbb7f5008ee"
  },
  {
    "name": "lightspeed-console-plugin",
    "image": "quay.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols/lightspeed-console@sha256:f939011899542db36baa4826b62671ed08d598dbc22ee5d4a62da6d78a80cda2"
  },
  {
    "name": "lightspeed-operator",
    "image": "quay.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/ols/lightspeed-operator@sha256:256e653c620a2e1ef3d9ef7024da5326af551f5fe11a5e2307e4bafd8930e381"
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
: "${BASE_IMAGE:=registry.access.redhat.com/ubi9/ubi-minimal}"
sed -i 's@^FROM scratch@FROM '"${BASE_IMAGE}"'@' ${BUNDLE_DOCKERFILE}

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

${OPM} render "${BUNDLE_IMAGE}" --output=yaml >>"${CATALOG_FILE}"
cat <<EOF >>"${CATALOG_FILE}"
---
schema: olm.channel
package: lightspeed-operator
name: preview
entries:
  - name: lightspeed-operator.v$BUNDLE_TAG
EOF
echo "Finished running $(basename "$0")"
