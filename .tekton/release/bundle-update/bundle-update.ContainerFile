FROM registry.access.redhat.com/ubi9/ubi-minimal

RUN microdnf install -y \
    jq \
    tar \
    git \
    && microdnf clean all
RUN curl -L -o oc.tar.gz https://mirror.openshift.com/pub/openshift-v4/x86_64/clients/ocp/latest-4.19/openshift-client-linux-amd64-rhel9.tar.gz \
    && tar -xvzf oc.tar.gz \
    && chmod +x kubectl oc \
    && mv oc kubectl /usr/local/bin/

