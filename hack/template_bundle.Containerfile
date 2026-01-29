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

##__GENERATED_CONTAINER_FILE__##

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
LABEL release={BUNDLE_VERSION}
LABEL url="https://github.com/openshift/lightspeed-operator"
LABEL vendor="Red Hat, Inc."
LABEL version={BUNDLE_VERSION}
LABEL summary="Red Hat OpenShift Lightspeed"

# OCP compatibility labels
LABEL com.redhat.openshift.versions=v4.16-v4.20

# Set user to non-root for security reasons.
USER 1001
