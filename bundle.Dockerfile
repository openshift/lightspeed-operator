FROM registry.redhat.io/ubi9/ubi-minimal:9.7 as builder
ARG RELATED_IMAGE_FILE=related_images.json
ARG CSV_FILE=bundle/manifests/lightspeed-operator.clusterserviceversion.yaml
# CSV contains __REPLACE_*__ placeholders; substitute from related_images.json (see hack/image_placeholders.json).

RUN microdnf install -y jq

COPY ${CSV_FILE} /manifests/lightspeed-operator.clusterserviceversion.yaml
COPY ${RELATED_IMAGE_FILE} /${RELATED_IMAGE_FILE}
COPY hack/image_placeholders.json /image_placeholders.json

# Substitute placeholders using image_placeholders.json and related_images.json
RUN set -e && \
    jq -r '.[] | "\(.name)|\(.placeholder)"' /image_placeholders.json | while IFS='|' read -r name placeholder; do \
      img=$(jq -r --arg n "$$name" '.[] | select(.name==$n) | .image' /${RELATED_IMAGE_FILE}) && \
      if [ -n "$$img" ] && [ "$$img" != "null" ]; then \
        sed -i "s|$$placeholder|$$img|g" /manifests/lightspeed-operator.clusterserviceversion.yaml || exit 1; \
      fi; \
    done


FROM registry.redhat.io/ubi9/ubi-minimal:9.6

# Core bundle labels.
LABEL operators.operatorframework.io.bundle.mediatype.v1=registry+v1
LABEL operators.operatorframework.io.bundle.manifests.v1=manifests/
LABEL operators.operatorframework.io.bundle.metadata.v1=metadata/
LABEL operators.operatorframework.io.bundle.package.v1=lightspeed-operator
LABEL operators.operatorframework.io.bundle.channels.v1=alpha
LABEL operators.operatorframework.io.bundle.channel.default.v1=alpha
LABEL operators.operatorframework.io.metrics.builder=operator-sdk-v1.33.0
LABEL operators.operatorframework.io.metrics.mediatype.v1=metrics+v1
LABEL operators.operatorframework.io.metrics.project_layout=go.kubebuilder.io/v4

# Labels for testing.
LABEL operators.operatorframework.io.test.mediatype.v1=scorecard+v1
LABEL operators.operatorframework.io.test.config.v1=tests/scorecard/

# Copy files to locations specified by labels.
COPY bundle/manifests /manifests/
COPY bundle/metadata /metadata/
COPY bundle/tests/scorecard /tests/scorecard/

# Copy the CSVfile with replaced images references
COPY --from=builder manifests/lightspeed-operator.clusterserviceversion.yaml /manifests/lightspeed-operator.clusterserviceversion.yaml

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
LABEL name="openshift-lightspeed/lightspeed-operator-bundle"
LABEL cpe="cpe:/a:redhat:openshift_lightspeed:1::el9"
LABEL release=1.0.9
LABEL url="https://github.com/openshift/lightspeed-operator"
LABEL vendor="Red Hat, Inc."
LABEL version=1.0.9
LABEL summary="Red Hat OpenShift Lightspeed"

# OCP compatibility labels
LABEL com.redhat.openshift.versions=v4.16-v4.20

# Set user to non-root for security reasons.
USER 1001
