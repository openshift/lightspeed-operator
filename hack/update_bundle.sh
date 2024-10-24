# Helper tool to update the bundle artifacts
# Pre-requisites: opm, make, yq, operator-sdk
# Usage: ./hack/update_bundle.sh

#!/bin/bash

set -euo pipefail

SCRIPT_DIR=$(dirname "$0")

usage() {
  echo "Usage: $0 [-v bundle_version] [-h]"
  echo "  -v bundle_version: The version of the bundle"
  echo "  -i related_images_filename: The JSON file containing the related images"

  echo "  -h: Show this help message"
}

BUNDLE_VERSION=""
RELATED_IMAGES_FILENAME=""
CHANNEL_NAME="alpha"

while getopts ":v:i:h" opt; do
  case "$opt" in
  "v")
    BUNDLE_VERSION=${OPTARG}
    echo "bundle_version is ${BUNDLE_VERSION}"
    ;;
  "i")
    RELATED_IMAGES_FILENAME=${OPTARG}
    if [ ! -f "${RELATED_IMAGES_FILENAME}" ]; then
      echo "related_images_filename ${RELATED_IMAGES_FILENAME} does not exist"
      exit 1
    fi
    echo "related_images from file ${RELATED_IMAGES_FILENAME}"
    ;;
  "c")
    CHANNEL_NAME=${OPTARG}
    echo "channel_name is ${CHANNEL_NAME}"
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

if [ -z "${BUNDLE_VERSION}" ]; then
  echo "bundle_version is required"
  usage
  exit 1
fi

# default flag for bundle generation
: ${BUNDLE_GEN_FLAGS="--channels=${CHANNEL_NAME} --default-channel=${CHANNEL_NAME} -q --overwrite --version ${BUNDLE_VERSION}"}
BUNDLE_GEN_FLAGS="${BUNDLE_GEN_FLAGS} --version ${BUNDLE_VERSION}"

# Tool check
: ${YQ:=$(command -v yq)}
echo "using yq from ${YQ}"
if [ -z "${YQ}" ]; then
  echo "yq is required"
  exit 1
fi

: ${JQ:=$(command -v jq)}
echo "using jq from ${JQ}"
if [ -z "${JQ}" ]; then
  echo "jq is required"
  exit 1
fi

: ${OPERATOR_SDK:=$(command -v operator-sdk)}
echo "using operator-sdk from ${OPERATOR_SDK}"
if [ -z "${OPERATOR_SDK}" ]; then
  echo "operator-sdk is required"
  exit 1
fi

: ${KUSTOMIZE:=$(command -v kustomize)}
echo "using kustomize from ${KUSTOMIZE}"
if [ -z "${KUSTOMIZE}" ]; then
  echo "kustomize is required"
  exit 1
fi

CSV_FILE="bundle/manifests/lightspeed-operator.clusterserviceversion.yaml"
ANNOTATION_FILE="bundle/metadata/annotations.yaml"

BUNDLE_DOCKERFILE="bundle.Dockerfile"

# if RELATED_IMAGES is not defined, extract related images or use default values
if [ -f "${RELATED_IMAGES_FILENAME}" ]; then
  RELATED_IMAGES=$(${JQ} '[ .[] | select(.name == "lightspeed-service-api" or .name == "lightspeed-operator" or .name == "lightspeed-console-plugin") ]' ${RELATED_IMAGES_FILENAME})
elif [ -f "${CSV_FILE}" ]; then
  RELATED_IMAGES=$(${YQ} ' .spec.relatedImages' -ojson ${CSV_FILE})
else
  RELATED_IMAGES=$(
    cat <<EOF
[
  {
      "name": "lightspeed-service-api",
      "image": "quay.io/openshift-lightspeed/lightspeed-service-api:latest"
  },
  {
      "name": "lightspeed-console-plugin",
      "image": "quay.io/openshift-lightspeed/lightspeed-console-plugin:latest"
  },
  {
      "name": "lightspeed-operator",
      "image": "quay.io/openshift-lightspeed/lightspeed-operator:latest"
  }
]
EOF
  )
fi

if [ -z "${RELATED_IMAGES}" ]; then
  echo "RELATED_IMAGES is empty, please provide related images"
  exit 1
fi
OPERATOR_IMAGE=$(${JQ} '.[] | select(.name == "lightspeed-operator") | .image' <<<${RELATED_IMAGES})
SERVICE_IMAGE=$(${JQ} '.[] | select(.name == "lightspeed-service-api") | .image' <<<${RELATED_IMAGES})
CONSOLE_IMAGE=$(${JQ} '.[] | select(.name == "lightspeed-console-plugin") | .image' <<<${RELATED_IMAGES})

# Build the bundle image
echo "Updating bundle artifacts for image ${OPERATOR_IMAGE}"
rm -rf ./bundle

# make bundle VERSION="${BUNDLE_VERSION}"
${OPERATOR_SDK} generate kustomize manifests -q
${KUSTOMIZE} build config/manifests | ${OPERATOR_SDK} generate bundle ${BUNDLE_GEN_FLAGS}
${OPERATOR_SDK} bundle validate ./bundle
# set service and console image for the operator
${YQ} "(.spec.install.spec.deployments[].spec.template.spec.containers[].args[] |= sub(\"quay.io/openshift-lightspeed/lightspeed-service-api:latest\", ${SERVICE_IMAGE}))" -i ${CSV_FILE}
${YQ} "(.spec.install.spec.deployments[].spec.template.spec.containers[].args[] |= sub(\"quay.io/openshift-lightspeed/lightspeed-console-plugin:latest\", ${CONSOLE_IMAGE}))" -i ${CSV_FILE}
${YQ} "(.spec.install.spec.deployments[].spec.template.spec.containers[].image |= sub(\"quay.io/openshift-lightspeed/lightspeed-operator:latest\", ${OPERATOR_IMAGE}))" -i ${CSV_FILE}
# set related images to the CSV file
${YQ} eval -i '.spec.relatedImages='"${RELATED_IMAGES}" ${CSV_FILE}
# add compatibility labels to the annotations file
${YQ} eval -i '.annotations."com.redhat.openshift.versions"="v4.15-v4.17"' ${ANNOTATION_FILE}
${YQ} eval -i '(.annotations."com.redhat.openshift.versions" | key) head_comment="OCP compatibility labels"' ${ANNOTATION_FILE}

# use UBI image as base image for bundle image
: "${BASE_IMAGE:=registry.redhat.io/ubi9/ubi-minimal@sha256:c0e70387664f30cd9cf2795b547e4a9a51002c44a4a86aa9335ab030134bf392}"
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
LABEL release=${BUNDLE_VERSION}
LABEL url="https://github.com/openshift/lightspeed-operator"
LABEL vendor="Red Hat, Inc."
LABEL version=${BUNDLE_VERSION}
LABEL summary="Red Hat OpenShift Lightspeed"

# OCP compatibility labels
LABEL com.redhat.openshift.versions=v4.15-v4.17

# Set user to non-root for security reasons.
USER 1001
EOF

echo "Finished running $(basename "$0")"
