FROM registry.access.redhat.com/ubi9/ubi-minimal

ARG OPM_VERSION=v1.47.0
ARG YQ_VERSION=v4.45.4

RUN microdnf install -y \
    jq \
    tar \
    git \
    golang \
    && microdnf clean all
RUN curl -L -o oc.tar.gz https://mirror.openshift.com/pub/openshift-v4/x86_64/clients/ocp/latest-4.19/openshift-client-linux-amd64-rhel9.tar.gz \
    && tar -xvzf oc.tar.gz \
    && chmod +x kubectl oc \
    && mv oc kubectl /usr/local/bin/
RUN OS=$(go env GOOS) && ARCH=$(go env GOARCH) \
	&& curl -sSLo /usr/local/bin/opm https://github.com/operator-framework/operator-registry/releases/download/${OPM_VERSION}/${OS}-${ARCH}-opm \
	&& chmod +x /usr/local/bin/opm
RUN OS=$(go env GOOS) && ARCH=$(go env GOARCH) \
	&& curl -sSLo /usr/local/bin/yq https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_${OS}_${ARCH} \
    && chmod +x /usr/local/bin/yq

LABEL cpe="cpe:/a:redhat:openshift_lightspeed:1::el9"
