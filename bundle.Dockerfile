FROM registry.redhat.io/ubi9/ubi-minimal:9.6 as builder
ARG RELATED_IMAGE_FILE=related_images.json
ARG CSV_FILE=bundle/manifests/lightspeed-operator.clusterserviceversion.yaml
ARG OPERATOR_IMAGE_ORIGINAL=quay.io/openshift-lightspeed/lightspeed-operator:latest
ARG SERVICE_IMAGE_ORIGINAL=quay.io/openshift-lightspeed/lightspeed-service-api:latest
ARG CONSOLE_IMAGE_ORIGINAL=quay.io/openshift-lightspeed/lightspeed-console-plugin:latest
ARG CONSOLE_IMAGE_ORIGINAL_PF5=quay.io/openshift-lightspeed/lightspeed-console-plugin-pf5:latest
ARG OPENSHIFT_MCP_SERVER_IMAGE_ORIGINAL=quay.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/openshift-mcp-server@sha256:3a035744b772104c6c592acf8a813daced19362667ed6dab73a00d17eb9c3a43
ARG DATAVERSE_EXPORTER_IMAGE_ORIGINAL=quay.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/lightspeed-to-dataverse-exporter@sha256:ccb6705a5e7ff0c4d371dc72dc8cf319574a2d64bcc0a89ccc7130f626656722

RUN microdnf install -y jq

COPY ${CSV_FILE} /manifests/lightspeed-operator.clusterserviceversion.yaml
COPY ${RELATED_IMAGE_FILE} /${RELATED_IMAGE_FILE}

RUN OPERATOR_IMAGE=$(jq -r '.[] | select(.name == "lightspeed-operator") | .image' /${RELATED_IMAGE_FILE}) && sed -i "s|${OPERATOR_IMAGE_ORIGINAL}|${OPERATOR_IMAGE}|g" /manifests/lightspeed-operator.clusterserviceversion.yaml
RUN SERVICE_IMAGE=$(jq -r '.[] | select(.name == "lightspeed-service-api") | .image' /${RELATED_IMAGE_FILE}) && sed -i "s|${SERVICE_IMAGE_ORIGINAL}|${SERVICE_IMAGE}|g" /manifests/lightspeed-operator.clusterserviceversion.yaml
RUN CONSOLE_IMAGE=$(jq -r '.[] | select(.name == "lightspeed-console-plugin") | .image' /${RELATED_IMAGE_FILE}) && sed -i "s|${CONSOLE_IMAGE_ORIGINAL}|${CONSOLE_IMAGE}|g" /manifests/lightspeed-operator.clusterserviceversion.yaml
RUN CONSOLE_IMAGE_PF5=$(jq -r '.[] | select(.name == "lightspeed-console-plugin-pf5") | .image' /${RELATED_IMAGE_FILE}) && sed -i "s|${CONSOLE_IMAGE_ORIGINAL_PF5}|${CONSOLE_IMAGE}|g" /manifests/lightspeed-operator.clusterserviceversion.yaml
RUN OPENSHIFT_MCP_SERVER_IMAGE=$(jq -r '.[] | select(.name == "openshift-mcp-server") | .image' /${RELATED_IMAGE_FILE}) && sed -i "s|${OPENSHIFT_MCP_SERVER_IMAGE_ORIGINAL}|${OPENSHIFT_MCP_SERVER_IMAGE}|g" /manifests/lightspeed-operator.clusterserviceversion.yaml
RUN DATAVERSE_EXPORTER_IMAGE=$(jq -r '.[] | select(.name == "lightspeed-to-dataverse-exporter") | .image' /${RELATED_IMAGE_FILE}) && sed -i "s|${DATAVERSE_EXPORTER_IMAGE_ORIGINAL}|${DATAVERSE_EXPORTER_IMAGE}|g" /manifests/lightspeed-operator.clusterserviceversion.yaml


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
LABEL release=1.0.6
LABEL url="https://github.com/openshift/lightspeed-operator"
LABEL vendor="Red Hat, Inc."
LABEL version=1.0.6
LABEL summary="Red Hat OpenShift Lightspeed"

# OCP compatibility labels
LABEL com.redhat.openshift.versions=v4.16-v4.20

# Set user to non-root for security reasons.
USER 1001
