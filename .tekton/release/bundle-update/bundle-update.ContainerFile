FROM registry.access.redhat.com/ubi9/ubi-minimal
ARG KUSTOMIZE_VERSION=v5.3.0
ARG OPERATOR_SDK_VERSION=v1.33.0
ARG YQ_VERSION=v4.44.6

RUN microdnf install -y \
    jq \
    tar \
    git \
    make \
    golang \
    && microdnf clean all
# openshift client
RUN curl -L -o oc.tar.gz https://mirror.openshift.com/pub/openshift-v4/x86_64/clients/ocp/latest-4.19/openshift-client-linux-amd64-rhel9.tar.gz \
    && tar -xvzf oc.tar.gz \
    && chmod +x kubectl oc \
    && mv oc kubectl /usr/local/bin/
# kustomize
RUN go install -mod=mod sigs.k8s.io/kustomize/kustomize/v5@${KUSTOMIZE_VERSION}

# operator-sdk
RUN OPERATOR_SDK=$(go env GOPATH)/bin/operator-sdk && OS=$(go env GOOS) && ARCH=$(go env GOARCH) && \
    curl -sSLo ${OPERATOR_SDK} https://github.com/operator-framework/operator-sdk/releases/download/${OPERATOR_SDK_VERSION}/operator-sdk_${OS}_${ARCH} && \
    chmod +x ${OPERATOR_SDK}

# yq
RUN YQ=$(go env GOPATH)/bin/yq && OS=$(go env GOOS) && ARCH=$(go env GOARCH) && \
    curl -sSLo ${YQ} https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_${OS}_${ARCH} && \
    chmod +x ${YQ}

# set path for go binaries such as kustomize
ENV PATH="/root/go/bin:$PATH"