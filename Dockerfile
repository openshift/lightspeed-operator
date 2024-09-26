# Build the manager binary
FROM registry.redhat.io/ubi9/go-toolset@sha256:f001ad1001a22fe5f6fc7d876fc172b01c1b7dcd6c498f83a07b425e24275a79 AS builder
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
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/controller/ internal/controller/

# this directory is checked by ecosystem-cert-preflight-checks task in Konflux
COPY LICENSE /licenses/

USER 0

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager cmd/main.go


FROM registry.redhat.io/ubi9/ubi-minimal@sha256:c0e70387664f30cd9cf2795b547e4a9a51002c44a4a86aa9335ab030134bf392

WORKDIR /
COPY --from=builder /workspace/manager .
RUN mkdir /licenses
COPY LICENSE /licenses/.
LABEL name="openshift-lightspeed/lightspeed-rhel9-operator" \
      com.redhat.component="openshift-lightspeed" \
      io.k8s.display-name="OpenShift Lightspeed Operator" \
      summary="OpenShift Lightspeed Operator manages the AI-powered OpenShift Assistant Service." \
      description="OpenShift Lightspeed Operator manages the AI-powered OpenShift Assistant Service and Openshift Console plugin extention." \
      io.k8s.description="OpenShift Lightspeed Operator is a component of OpenShift Lightspeed, that  manages the AI-powered OpenShift Assistant Service and Openshift Console plugin extention." \
      io.openshift.tags="openshift-lightspeed,ols"
USER 65532:65532

ENTRYPOINT ["/manager"]
