FROM registry.redhat.io/ubi9/ubi-minimal@sha256:c0e70387664f30cd9cf2795b547e4a9a51002c44a4a86aa9335ab030134bf392

# Core bundle labels.
LABEL operators.operatorframework.io.bundle.mediatype.v1=registry+v1
LABEL operators.operatorframework.io.bundle.manifests.v1=manifests/
LABEL operators.operatorframework.io.bundle.metadata.v1=metadata/
LABEL operators.operatorframework.io.bundle.package.v1=lightspeed-operator
LABEL operators.operatorframework.io.bundle.channels.v1=preview
LABEL operators.operatorframework.io.bundle.channel.default.v1=preview
LABEL operators.operatorframework.io.metrics.builder=operator-sdk-v1.33.0
LABEL operators.operatorframework.io.metrics.mediatype.v1=metrics+v1
LABEL operators.operatorframework.io.metrics.project_layout=go.kubebuilder.io/v4

# OCP compatibility labels
LABEL com.redhat.openshift.versions=v4.15-v4.16

# Labels for testing.
LABEL operators.operatorframework.io.test.mediatype.v1=scorecard+v1
LABEL operators.operatorframework.io.test.config.v1=tests/scorecard/

# Copy files to locations specified by labels.
COPY bundle/manifests /manifests/
COPY bundle/metadata /metadata/
COPY bundle/tests/scorecard /tests/scorecard/

# replace registry.redhat.io string instances with quay.io
RUN sed -i 's/registry.redhat.io\/openshift-lightspeed-beta\/lightspeed-rhel9-operator/quay.io\/redhat-user-workloads\/crt-nshift-lightspeed-tenant\/ols\/lightspeed-operator/g' /manifests/lightspeed-operator.clusterserviceversion.yaml
RUN sed -i 's/registry.redhat.io\/openshift-lightspeed-beta\/lightspeed-service-api-rhel9/quay.io\/redhat-user-workloads\/crt-nshift-lightspeed-tenant\/ols\/lightspeed-service/g' /manifests/lightspeed-operator.clusterserviceversion.yaml
RUN sed -i 's/registry.redhat.io\/openshift-lightspeed-beta\/lightspeed-console-plugin-rhel9/quay.io\/redhat-user-workloads\/crt-nshift-lightspeed-tenant\/ols\/lightspeed-console/g' /manifests/lightspeed-operator.clusterserviceversion.yaml

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
LABEL release=0.1.4
LABEL url="https://github.com/openshift/lightspeed-operator"
LABEL vendor="Red Hat, Inc."
LABEL version=0.1.4
LABEL summary="Red Hat OpenShift Lightspeed"

# Set user to non-root for security reasons.
USER 1001
