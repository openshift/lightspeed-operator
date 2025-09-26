# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
VERSION ?= latest

# CHANNELS define the bundle channels used in the bundle.
# Add a new line here if you would like to change its default config. (E.g CHANNELS = "candidate,fast,stable")
# To re-generate a bundle for other specific channels without changing the standard setup, you can:
# - use the CHANNELS as arg of the bundle target (e.g make bundle CHANNELS=candidate,fast,stable)
# - use environment variables to overwrite this value (e.g export CHANNELS="candidate,fast,stable")
CHANNELS=alpha
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif

# DEFAULT_CHANNEL defines the default channel used in the bundle.
# Add a new line here if you would like to change its default config. (E.g DEFAULT_CHANNEL = "stable")
# To re-generate a bundle for any other default channel without changing the default setup, you can:
# - use the DEFAULT_CHANNEL as arg of the bundle target (e.g make bundle DEFAULT_CHANNEL=stable)
# - use environment variables to overwrite this value (e.g export DEFAULT_CHANNEL="stable")
DEFAULT_CHANNEL = "alpha"
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
# This variable is used to construct full image tags for bundle and catalog images.
#
# For example, running 'make bundle-build bundle-push catalog-build catalog-push' will build and push both
# quay.io/openshift-lightspeed/lightspeed-operator-bundle:$VERSION and quay.io/openshift-lightspeed/lightspeed-operator-catalog:$VERSION.
IMAGE_TAG_BASE ?= quay.io/openshift-lightspeed/lightspeed-operator

# BUNDLE_TAG defines the version of the bundle.
# You can use it as an arg. (E.g make update-bundle-catalog BUNDLE_TAG=0.0.1)
BUNDLE_TAG ?= 1.0.4

# set the base image for docker files
# You can use it as an arg.  (E.g make update-bundle-catalog BASE_IMG=registry.redhat.io/ubi9/ubi-minimal)
BASE_IMG ?= registry.redhat.io/ubi9/ubi-minimal

# BUNDLE_IMG defines the image:tag used for the bundle.
# You can use it as an arg. (E.g make bundle-build BUNDLE_IMG=<some-registry>/<project-name-bundle>:<tag>)
BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:v$(VERSION)

# BUNDLE_GEN_FLAGS are the flags passed to the operator-sdk generate bundle command
BUNDLE_GEN_FLAGS ?= -q --overwrite $(BUNDLE_METADATA_OPTS)

# USE_IMAGE_DIGESTS defines if images are resolved via tags or digests
# You can enable this value if you would like to use SHA Based Digests
# To enable set flag to true
USE_IMAGE_DIGESTS ?= false
ifeq ($(USE_IMAGE_DIGESTS), true)
	BUNDLE_GEN_FLAGS += --use-image-digests
endif

# Set the Operator SDK version to use. By default, what is installed on the system is used.
# This is useful for CI or a project to utilize a specific version of the operator-sdk toolkit.
OPERATOR_SDK_VERSION ?= v1.33.0

# Image URL to use all building/pushing image targets
IMG ?= $(IMAGE_TAG_BASE):$(VERSION)
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.27.1

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# By default, it will use podman if available, otherwise falls back to docker.
# Be aware that the target commands are only tested with Docker.
# You can override this by setting CONTAINER_TOOL environment variable.
CONTAINER_TOOL ?= "$(shell which podman >/dev/null 2>&1 && echo podman || echo docker)"

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet envtest test-crds ## Run local tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test ./internal/... -coverprofile cover.out

# Use 4.18 release branch for CRDs in unit tests
OS_CONSOLE_CRD_URL = https://raw.githubusercontent.com/openshift/api/refs/heads/release-4.18/operator/v1/zz_generated.crd-manifests/0000_50_console_01_consoles.crd.yaml
OS_CONSOLE_PLUGIN_CRD_URL = https://raw.githubusercontent.com/openshift/api/refs/heads/release-4.18/console/v1/zz_generated.crd-manifests/90_consoleplugins-Default.crd.yaml
OCP_CLUSTER_VERSION_CRD_URL = https://raw.githubusercontent.com/openshift/api/refs/heads/release-4.18/config/v1/zz_generated.crd-manifests/0000_00_cluster-version-operator_01_clusterversions-Default.crd.yaml
MONITORING_CRD_URL = https://raw.githubusercontent.com/openshift/prometheus-operator/master/bundle.yaml
OCP_APISERVER_CRD_URL = https://raw.githubusercontent.com/openshift/api/refs/heads/release-4.18/payload-manifests/crds/0000_10_config-operator_01_apiservers.crd.yaml
TEST_CRD_DIR = .testcrds
OS_CONSOLE_CRD_FILE = $(TEST_CRD_DIR)/openshift-console-crd.yaml
OS_CONSOLE_PLUGIN_CRD_FILE = $(TEST_CRD_DIR)/openshift-console-plugin-crd.yaml
OCP_CLUSTER_VERSION_CRD_FILE = $(TEST_CRD_DIR)/openshift-config-clusterversion-crd.yaml
MONITORING_CRD_FILE = $(TEST_CRD_DIR)/monitoring-crd.yaml
OCP_APISERVER_CRD_FILE = $(TEST_CRD_DIR)/openshift-apiserver-crd.yaml

.PHONY: test-crds
test-crds: $(TEST_CRD_DIR) $(OS_CONSOLE_CRD_FILE) $(OS_CONSOLE_PLUGIN_CRD_FILE) $(MONITORING_CRD_FILE) $(OCP_CLUSTER_VERSION_CRD_FILE) $(OCP_APISERVER_CRD_FILE) ## Test Dependencies CRDs

$(TEST_CRD_DIR):
	mkdir -p $(TEST_CRD_DIR)

$(OS_CONSOLE_CRD_FILE): $(TEST_CRD_DIR)
	wget -O $(OS_CONSOLE_CRD_FILE) $(OS_CONSOLE_CRD_URL)

$(OS_CONSOLE_PLUGIN_CRD_FILE): $(TEST_CRD_DIR)
	wget -O $(OS_CONSOLE_PLUGIN_CRD_FILE) $(OS_CONSOLE_PLUGIN_CRD_URL)

$(MONITORING_CRD_FILE): $(TEST_CRD_DIR)
	wget -O $(MONITORING_CRD_FILE) $(MONITORING_CRD_URL)

$(OCP_CLUSTER_VERSION_CRD_FILE): $(TEST_CRD_DIR)
	wget -O $(OCP_CLUSTER_VERSION_CRD_FILE) $(OCP_CLUSTER_VERSION_CRD_URL)

$(OCP_APISERVER_CRD_FILE): $(TEST_CRD_DIR)
	wget -O $(OCP_APISERVER_CRD_FILE) $(OCP_APISERVER_CRD_URL)


.PHONY: test-e2e
test-e2e: ## Run e2e tests with an Openshift cluster. Requires KUBECONFIG and LLM_TOKEN environment variables.
ifndef KUBECONFIG
	$(error KUBECONFIG environment variable is not set)
endif
ifndef LLM_TOKEN
	$(error LLM_TOKEN  environment variable is not set)
endif
	go test ./test/e2e -timeout=120m -ginkgo.v -test.v -ginkgo.show-node-events --ginkgo.label-filter="!Rapidast && !Upgrade"

.PHONY: test-upgrade
test-upgrade: ## Run upgrade tests with an Openshift cluster. Requires KUBECONFIG, LLM_TOKEN and BUNDLE_IMAGE environment variables.
ifndef KUBECONFIG
	$(error KUBECONFIG environment variable is not set)
endif
ifndef LLM_TOKEN
	$(error LLM_TOKEN  environment variable is not set)
endif
ifndef BUNDLE_IMAGE
	$(error BUNDLE_IMAGE  environment variable is not set)
endif
	go test ./test/e2e -timeout=120m -ginkgo.v -test.v -ginkgo.show-node-events --ginkgo.label-filter="Upgrade"

.PHONY: test-e2e-local
test-e2e-local: ## Run e2e tests with an Openshift cluster, excluding Database-Persistency test that requires a storage class. Requires KUBECONFIG and LLM_TOKEN environment variables.
ifndef KUBECONFIG
	$(error KUBECONFIG environment variable is not set)
endif
ifndef LLM_TOKEN
	$(error LLM_TOKEN  environment variable is not set)
endif
	go test ./test/e2e -timeout=120m -ginkgo.v -test.v -ginkgo.show-node-events --ginkgo.label-filter="!Rapidast" --ginkgo.label-filter="!Database-Persistency"

.PHONY: lint
lint: ## Run golangci-lint against code.
	golangci-lint run --config=.golangci.yaml

.PHONY: lint-fix
lint-fix: ## Fix found issues (if it's supported by the linter).
	golangci-lint run --fix

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

LIGHTSPEED_SERVICE_IMG ?= quay.io/openshift-lightspeed/lightspeed-service-api:latest
LIGHTSPEED_SERVICE_POSTGRES_IMG ?= registry.redhat.io/rhel9/postgresql-16@sha256:6d2cab6cb6366b26fcf4591fe22aa5e8212a3836c34c896bb65977a8e50d658b
CONSOLE_PLUGIN_IMG ?= quay.io/openshift-lightspeed/lightspeed-console-plugin:latest
OPENSHIFT_MCP_SERVER_IMG ?= quay.io/redhat-user-workloads/crt-nshift-lightspeed-tenant/openshift-mcp-server@sha256:3a035744b772104c6c592acf8a813daced19362667ed6dab73a00d17eb9c3a43
.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
    #TODO: Update DB
	go run ./cmd/main.go --service-image="$(LIGHTSPEED_SERVICE_IMG)" --postgres-image="$(LIGHTSPEED_SERVICE_POSTGRES_IMG)" --console-image="$(CONSOLE_PLUGIN_IMG)" --openshift-mcp-server-image="$(OPENSHIFT_MCP_SERVER_IMG)"

# If you wish built the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64 ). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: test ## Build docker image with the manager.
	$(CONTAINER_TOOL) build --platform linux/amd64 -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

# PLATFORMS defines the target platforms for  the manager image be build to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - able to use docker buildx . More info: https://docs.docker.com/build/buildx/
# - have enable BuildKit, More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image for your registry (i.e. if you do not inform a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To properly provided solutions that supports more than one platform you should use this option.
PLATFORMS ?= linux/amd64
.PHONY: docker-buildx
docker-buildx: test ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name project-v3-builder
	$(CONTAINER_TOOL) buildx use project-v3-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm project-v3-builder
	rm Dockerfile.cross

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize yq ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	$(YQ) e -i ".patches[0].patch |= sub(\"quay.io/openshift-lightspeed/lightspeed-operator:latest\", \"${IMG}\")" config/default/kustomization.yaml
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest

## Tool Versions
KUSTOMIZE_VERSION ?= v5.3.0
CONTROLLER_TOOLS_VERSION ?= v0.16.5
ENVTEST_VERSION ?= release-0.19

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary. If wrong version is installed, it will be removed before downloading.
$(KUSTOMIZE): $(LOCALBIN)
	@if test -x $(LOCALBIN)/kustomize && ! $(LOCALBIN)/kustomize version | grep -q $(KUSTOMIZE_VERSION); then \
		echo "$(LOCALBIN)/kustomize version is not expected $(KUSTOMIZE_VERSION). Removing it before installing."; \
		rm -rf $(LOCALBIN)/kustomize; \
	fi
	test -s $(LOCALBIN)/kustomize || GOBIN=$(LOCALBIN) go install -mod=mod sigs.k8s.io/kustomize/kustomize/v5@$(KUSTOMIZE_VERSION)

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary. If wrong version is installed, it will be overwritten.
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(LOCALBIN)/controller-gen && $(LOCALBIN)/controller-gen --version | grep -q $(CONTROLLER_TOOLS_VERSION) || \
	GOBIN=$(LOCALBIN) go install -mod=mod sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install -mod=mod sigs.k8s.io/controller-runtime/tools/setup-envtest@$(ENVTEST_VERSION)

.PHONY: operator-sdk
OPERATOR_SDK ?= $(LOCALBIN)/operator-sdk
operator-sdk: ## Download operator-sdk locally if necessary.
ifeq (,$(wildcard $(OPERATOR_SDK)))
ifeq (, $(shell which operator-sdk 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPERATOR_SDK)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPERATOR_SDK) https://github.com/operator-framework/operator-sdk/releases/download/$(OPERATOR_SDK_VERSION)/operator-sdk_$${OS}_$${ARCH} ;\
	chmod +x $(OPERATOR_SDK) ;\
	}
else
OPERATOR_SDK = $(shell which operator-sdk)
endif
endif


## Generate bundle manifests and metadata, then validate generated files.
## to set the bundle version, use the BUNDLE_TAG variable
## to set the bundle channels, use the CHANNELS variable
## to set the default channel, use the DEFAULT_CHANNEL variable
## to use image digests instead of version tag, set the USE_IMAGE_DIGESTS variable to true
## to set the related images, use the RELATED_IMAGES_FILE variable
.PHONY: bundle
bundle: manifests kustomize operator-sdk yq jq ## Generate bundle manifests and metadata, then validate generated files.
ifdef RELATED_IMAGES_FILE
	YQ=$(YQ) JQ=$(JQ) BUNDLE_GEN_FLAGS="$(BUNDLE_GEN_FLAGS)" ./hack/update_bundle.sh -v $(BUNDLE_TAG) -i $(RELATED_IMAGES_FILE)
else
	YQ=$(YQ) JQ=$(JQ) BUNDLE_GEN_FLAGS="$(BUNDLE_GEN_FLAGS)" ./hack/update_bundle.sh -v $(BUNDLE_TAG)
endif

parking:
	$(OPERATOR_SDK) generate kustomize manifests -q
	$(KUSTOMIZE) build config/manifests | $(OPERATOR_SDK) generate bundle $(BUNDLE_GEN_FLAGS)
	$(OPERATOR_SDK) bundle validate ./bundle

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	$(CONTAINER_TOOL) build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: bundle-push
bundle-push: ## Push the bundle image.
	$(MAKE) docker-push IMG=$(BUNDLE_IMG)

.PHONY: opm
OPM = ./bin/opm
OPM_VERSION = v1.47.0
opm: ## Download opm locally if necessary.
ifeq (,$(wildcard $(OPM)))
ifeq (,$(shell which opm 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/$(OPM_VERSION)/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM) ;\
	}
else
OPM = $(shell which opm)
endif
endif

.PHONY: yq
YQ = ./bin/yq
yq: ## Download yq locally if necessary.
ifeq (,$(wildcard $(YQ)))
ifeq (,$(shell which yq 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(YQ)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(YQ) https://github.com/mikefarah/yq/releases/download/v4.44.6/yq_$${OS}_$${ARCH} ;\
    chmod +x $(YQ) ;\
	}
else
YQ = $(shell which yq)
endif
endif

.PHONY: jq
JQ = ./bin/jq
jq: ## Download jq locally if necessary.
ifeq (,$(wildcard $(JQ)))
ifeq (,$(shell which jq 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(JQ)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(JQ) https://github.com/jqlang/jq/releases/download/jq-1.7.1/jq-$${OS}-$${ARCH} ;\
    chmod +x $(JQ) ;\
	}
else
JQ = $(shell which jq)
endif
endif

# A comma-separated list of bundle images (e.g. make catalog-build BUNDLE_IMGS=example.com/operator-bundle:v0.1.0,example.com/operator-bundle:v0.2.0).
# These images MUST exist in a registry and be pull-able.
BUNDLE_IMGS ?= $(BUNDLE_IMG)

# The image tag given to the resulting catalog image (e.g. make catalog-build CATALOG_IMG=example.com/operator-catalog:v0.2.0).
CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:v$(VERSION)

# Set CATALOG_BASE_IMG to an existing catalog image tag to add $BUNDLE_IMGS to that image.
ifneq ($(origin CATALOG_BASE_IMG), undefined)
FROM_INDEX_OPT := --from-index $(CATALOG_BASE_IMG)
endif

# Build a catalog image by adding bundle images to an empty catalog using the operator package manager tool, 'opm'.
# This recipe invokes 'opm' in 'semver' bundle add mode. For more information on add modes, see:
# https://github.com/operator-framework/community-operators/blob/7f1438c/docs/packaging-operator.md#updating-your-existing-operator
.PHONY: catalog-build
catalog-build: opm ## Build a catalog image.
	$(OPM) index add --container-tool ${CONTAINER_TOOL} --mode semver --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)

# Push the catalog image.
.PHONY: catalog-push
catalog-push: ## Push a catalog image.
	$(MAKE) docker-push IMG=$(CATALOG_IMG)

# Update bundle and catalog artifacts.
update-bundle-catalog: opm yq
	OPM=$(OPM) YQ=$(YQ) BUNDLE_TAG=$(BUNDLE_TAG) BASE_IMAGE=$(BASE_IMG) hack/update_bundle_catalog.sh

# Genarate release objects for Konflux builds
.PHONY: konflux-release
konflux-release: kustomize yq
	mkdir -p release-konflux
	$(KUSTOMIZE) build hack/release-konflux | $(YQ) -s '"release-konflux/" + .metadata.name'