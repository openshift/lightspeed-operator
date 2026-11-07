# Build the manager binary
FROM registry.redhat.io/ubi9/go-toolset:9.8-1793736700 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/

# this directory is checked by ecosystem-cert-preflight-checks task in Konflux
COPY LICENSE /licenses/

USER 0

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
# Build with Go's native FIPS 140-3 crypto provider (pure Go, no CGo/OpenSSL).
# GOFIPS140=certified selects the certified FIPS module; the binary defaults
# to fips140=on (see the //go:debug directive in cmd/fips.go), keeping
# FIPS-approved crypto always active (matching the previous
# strictfipsruntime behavior).
# GOAMD64 is only consulted when GOARCH=amd64; it is silently ignored for
# other target architectures (arm64 etc.), so multi-arch builds are unaffected.
RUN CGO_ENABLED=0 GOFIPS140=certified GOAMD64=v2 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager ./cmd/
# Verify manager is built for at most x86-64-v2 (on amd64 only; check is a no-op elsewhere)
RUN go build -o check-isa-level ./cmd/check-isa-level && ./check-isa-level ./manager


FROM registry.access.redhat.com/ubi9-micro:9.8-1791765224@sha256:ff30602252a3e92706585c7c34cd3c74028eaac4ab815e6b19f8b27562bdd7e3

WORKDIR /
COPY --from=builder /workspace/manager .
RUN mkdir /licenses
COPY LICENSE /licenses/.
LABEL name="openshift-lightspeed/lightspeed-rhel9-operator" \
      com.redhat.component="openshift-lightspeed" \
      cpe="cpe:/a:redhat:openshift_lightspeed:2::el9" \
      io.k8s.display-name="OpenShift Lightspeed Operator" \
      summary="OpenShift Lightspeed Operator manages the AI-powered OpenShift Assistant Service." \
      description="OpenShift Lightspeed Operator manages the AI-powered OpenShift Assistant Service and Openshift Console plugin extention." \
      io.k8s.description="OpenShift Lightspeed Operator is a component of OpenShift Lightspeed, that  manages the AI-powered OpenShift Assistant Service and Openshift Console plugin extention." \
      io.openshift.tags="openshift-lightspeed,ols"
USER 65532:65532

ENTRYPOINT ["/manager"]
