###########################
# Configuration Variables #
###########################
ORG := github.com/operator-framework
PKG := $(ORG)/rukpak
export IMAGE_REPO ?= quay.io/operator-framework/rukpak
export IMAGE_TAG ?= latest
export GO_BUILD_TAGS ?= upstream
IMAGE?=$(IMAGE_REPO):$(IMAGE_TAG)
KIND_CLUSTER_NAME ?= rukpak
BIN_DIR := bin
TESTDATA_DIR := testdata
VERSION_PATH := $(PKG)/internal/version
GIT_COMMIT ?= $(shell git rev-parse HEAD)
PKGS = $(shell go list ./...)
export CERT_MGR_VERSION ?= v1.9.0
RUKPAK_NAMESPACE ?= rukpak-system

REGISTRY_NAME="docker-registry"
REGISTRY_NAMESPACE=rukpak-e2e
DNS_NAME=$(REGISTRY_NAME).$(REGISTRY_NAMESPACE).svc.cluster.local

CONTAINER_RUNTIME ?= docker
KUBECTL ?= kubectl

# kernel-style V=1 build verbosity
ifeq ("$(origin V)", "command line")
  BUILD_VERBOSE = $(V)
endif

ifeq ($(BUILD_VERBOSE),1)
  Q =
else
  Q = @
endif

###############
# Help Target #
###############
.PHONY: help
help: ## Show this help screen
	@echo 'Usage: make <OPTIONS> ... <TARGETS>'
	@echo ''
	@echo 'Available targets are:'
	@echo ''
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

###################
# Code management #
###################
.PHONY: lint tidy fmt clean generate verify

##@ code management:

lint: golangci-lint ## Run golangci linter
	$(Q)$(GOLANGCI_LINT) run --build-tags $(GO_BUILD_TAGS)

tidy: ## Update dependencies
	$(Q)go mod tidy
	$(Q)(cd $(TOOLS_DIR) && go mod tidy)

fmt: ## Format Go code
	$(Q)go fmt ./...
	$(Q)(cd $(TOOLS_DIR) && go fmt $$(go list -tags=tools ./...))

clean: ## Remove binaries and test artifacts
	@rm -rf bin

generate: controller-gen ## Generate code and manifests
	$(Q)$(CONTROLLER_GEN) crd:crdVersions=v1,generateEmbeddedObjectMeta=true output:crd:dir=./manifests/apis/crds paths=./api/...
	$(Q)$(CONTROLLER_GEN) webhook paths=./api/... paths=./internal/webhook/... output:stdout > ./manifests/apis/webhooks/resources/webhook.yaml
	$(Q)$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths=./api/...
	$(Q)$(CONTROLLER_GEN) rbac:roleName=core-admin \
		paths=./internal/provisioner/bundle/... \
		paths=./internal/provisioner/bundledeployment/... \
		paths=./internal/provisioner/plain/... \
		paths=./internal/provisioner/registry/... \
		paths=./internal/uploadmgr/... \
			output:stdout > ./manifests/core/resources/cluster_role.yaml
	$(Q)$(CONTROLLER_GEN) rbac:roleName=webhooks-admin paths=./internal/webhook/... output:stdout > ./manifests/apis/webhooks/resources/cluster_role.yaml
	$(Q)$(CONTROLLER_GEN) rbac:roleName=helm-provisioner-admin \
		paths=./internal/provisioner/bundle/... \
		paths=./internal/provisioner/bundledeployment/... \
		paths=./internal/provisioner/helm/... \
		    output:stdout > ./manifests/provisioners/helm/resources/cluster_role.yaml

verify: tidy fmt generate ## Verify the current code generation and lint
	git diff --exit-code

###########
# Testing #
###########
.PHONY: test test-unit test-e2e image-registry local-git

##@ testing:

test: test-unit test-e2e ## Run the tests

ENVTEST_VERSION = $(shell go list -m k8s.io/client-go | cut -d" " -f2 | sed 's/^v0\.\([[:digit:]]\{1,\}\)\.[[:digit:]]\{1,\}$$/1.\1.x/')
UNIT_TEST_DIRS=$(shell go list ./... | grep -v /test/)
test-unit: setup-envtest ## Run the unit tests
	eval $$($(SETUP_ENVTEST) use -p env $(ENVTEST_VERSION)) && go test -tags $(GO_BUILD_TAGS) -count=1 -short $(UNIT_TEST_DIRS)

FOCUS := $(if $(TEST),-v --focus "$(TEST)")
E2E_FLAGS ?=
test-e2e: ginkgo ## Run the e2e tests
	$(GINKGO) --tags $(GO_BUILD_TAGS) $(E2E_FLAGS) --trace --progress $(FOCUS) test/e2e

e2e: KIND_CLUSTER_NAME=rukpak-e2e
e2e: rukpakctl run image-registry local-git kind-load-bundles registry-load-bundles test-e2e kind-cluster-cleanup ## Run e2e tests against an ephemeral kind cluster

kind-cluster: kind kind-cluster-cleanup ## Standup a kind cluster
	$(KIND) create cluster --name ${KIND_CLUSTER_NAME} ${KIND_CLUSTER_CONFIG}
	$(KIND) export kubeconfig --name ${KIND_CLUSTER_NAME}

kind-cluster-cleanup: kind ## Delete the kind cluster
	$(KIND) delete cluster --name ${KIND_CLUSTER_NAME}

image-registry: ## Setup in-cluster image registry
	./test/tools/imageregistry/setup_imageregistry.sh ${KIND_CLUSTER_NAME}

local-git: ## Setup in-cluster git repository
	./test/tools/git/setup_git.sh ${KIND_CLUSTER_NAME}

###################
# Install and Run #
###################
.PHONY: install install-manifests wait run debug debug-helper cert-mgr uninstall

##@ install/run:

install: generate cert-mgr install-manifests wait ## Install rukpak

MANIFESTS_DIR ?= manifests
install-manifests:
	$(KUBECTL) apply -k $(MANIFESTS_DIR)

wait:
	$(KUBECTL) wait --for=condition=Available --namespace=$(RUKPAK_NAMESPACE) deployment/core --timeout=60s
	$(KUBECTL) wait --for=condition=Available --namespace=$(RUKPAK_NAMESPACE) deployment/rukpak-webhooks --timeout=60s
	$(KUBECTL) wait --for=condition=Available --namespace=$(RUKPAK_NAMESPACE) deployment/helm-provisioner --timeout=60s
	$(KUBECTL) wait --for=condition=Available --namespace=crdvalidator-system deployment/crd-validation-webhook --timeout=60s

run: build-container kind-cluster kind-load install ## Build image, stop/start a local kind cluster, and run operator in that cluster

debug: MANIFESTS_DIR = test/tools/remotedebug ## Same as 'run' target with the addition of remote debugging available on port 40000
debug: DEBUG_FLAGS = -gcflags="all=-N -l"
debug: KIND_CLUSTER_CONFIG = --config=./test/tools/remotedebug/kind-config.yaml
debug: run debug-helper

debug-helper:
	@echo ''
	@echo "Remote Debugging for '$$($(KUBECTL) -n $(RUKPAK_NAMESPACE) get pods -l app=core -o name)' in namespace '$(RUKPAK_NAMESPACE)' now available through localhost:40000"

cert-mgr: ## Install the certification manager
	$(KUBECTL) apply -f https://github.com/cert-manager/cert-manager/releases/download/$(CERT_MGR_VERSION)/cert-manager.yaml
	$(KUBECTL) wait --for=condition=Available --namespace=cert-manager deployment/cert-manager-webhook --timeout=60s

uninstall: ## Remove all rukpak resources from the cluster
	$(KUBECTL) delete -k $(MANIFESTS_DIR)

##################
# Build and Load #
##################

##@ build/load:

BINARIES=core helm unpack webhooks crdvalidator rukpakctl
LINUX_BINARIES=$(join $(addprefix linux/,$(BINARIES)), )

.PHONY: build $(BINARIES) $(LINUX_BINARIES) build-container kind-load kind-load-bundles kind-cluster registry-load-bundles

# Binary builds
build: $(BINARIES)

$(LINUX_BINARIES):
	CGO_ENABLED=0 GOOS=linux go build $(DEBUG_FLAGS) -tags $(GO_BUILD_TAGS) -o $(BIN_DIR)/$@ ./cmd/$(notdir $@)

$(BINARIES):
	CGO_ENABLED=0 go build $(DEBUG_FLAGS) -tags $(GO_BUILD_TAGS) -o $(BIN_DIR)/$@ ./cmd/$@ $(DEBUG_FLAGS)

build-container: $(LINUX_BINARIES) ## Builds provisioner container image locally
	$(CONTAINER_RUNTIME) build -f Dockerfile -t $(IMAGE) $(BIN_DIR)/linux

kind-load-bundles: kind ## Load the e2e testdata container images into a kind cluster
	$(CONTAINER_RUNTIME) build $(TESTDATA_DIR)/bundles/plain-v0/valid -t localhost/testdata/bundles/plain-v0:valid
	$(CONTAINER_RUNTIME) build $(TESTDATA_DIR)/bundles/plain-v0/dependent -t localhost/testdata/bundles/plain-v0:dependent
	$(CONTAINER_RUNTIME) build $(TESTDATA_DIR)/bundles/plain-v0/provides -t localhost/testdata/bundles/plain-v0:provides
	$(CONTAINER_RUNTIME) build $(TESTDATA_DIR)/bundles/plain-v0/empty -t localhost/testdata/bundles/plain-v0:empty
	$(CONTAINER_RUNTIME) build $(TESTDATA_DIR)/bundles/plain-v0/no-manifests -t localhost/testdata/bundles/plain-v0:no-manifests
	$(CONTAINER_RUNTIME) build $(TESTDATA_DIR)/bundles/plain-v0/invalid-missing-crds -t localhost/testdata/bundles/plain-v0:invalid-missing-crds
	$(CONTAINER_RUNTIME) build $(TESTDATA_DIR)/bundles/plain-v0/invalid-crds-and-crs -t localhost/testdata/bundles/plain-v0:invalid-crds-and-crs
	$(CONTAINER_RUNTIME) build $(TESTDATA_DIR)/bundles/plain-v0/subdir -t localhost/testdata/bundles/plain-v0:subdir
	$(CONTAINER_RUNTIME) build $(TESTDATA_DIR)/bundles/registry/valid -t localhost/testdata/bundles/registry:valid
	$(CONTAINER_RUNTIME) build $(TESTDATA_DIR)/bundles/registry/invalid -t localhost/testdata/bundles/registry:invalid
	$(KIND) load docker-image localhost/testdata/bundles/plain-v0:valid --name $(KIND_CLUSTER_NAME)
	$(KIND) load docker-image localhost/testdata/bundles/plain-v0:dependent --name $(KIND_CLUSTER_NAME)
	$(KIND) load docker-image localhost/testdata/bundles/plain-v0:provides --name $(KIND_CLUSTER_NAME)
	$(KIND) load docker-image localhost/testdata/bundles/plain-v0:empty --name $(KIND_CLUSTER_NAME)
	$(KIND) load docker-image localhost/testdata/bundles/plain-v0:no-manifests --name $(KIND_CLUSTER_NAME)
	$(KIND) load docker-image localhost/testdata/bundles/plain-v0:invalid-missing-crds --name $(KIND_CLUSTER_NAME)
	$(KIND) load docker-image localhost/testdata/bundles/plain-v0:invalid-crds-and-crs --name $(KIND_CLUSTER_NAME)
	$(KIND) load docker-image localhost/testdata/bundles/plain-v0:subdir --name $(KIND_CLUSTER_NAME)
	$(KIND) load docker-image localhost/testdata/bundles/registry:valid --name $(KIND_CLUSTER_NAME)
	$(KIND) load docker-image localhost/testdata/bundles/registry:invalid --name $(KIND_CLUSTER_NAME)

kind-load: kind ## Loads the currently constructed image onto the cluster
	$(KIND) load docker-image $(IMAGE) --name $(KIND_CLUSTER_NAME)

registry-load-bundles: ## Load selected e2e testdata container images created in kind-load-bundles into registry
	$(CONTAINER_RUNTIME) tag localhost/testdata/bundles/plain-v0:valid $(DNS_NAME):5000/bundles/plain-v0:valid
	./test/tools/imageregistry/load_test_image.sh $(KIND) $(KIND_CLUSTER_NAME)

###########
# Release #
###########

##@ release:

export ENABLE_RELEASE_PIPELINE ?= false
release: GORELEASER_ARGS ?= --snapshot --clean
release: goreleaser ## Run goreleaser
	$(GORELEASER) $(GORELEASER_ARGS)

quickstart: VERSION ?= $(shell git describe --abbrev=0 --tags)
quickstart: generate ## Generate the installation release manifests
	$(KUBECTL) kustomize manifests | sed "s/:latest/:$(VERSION)/g" > rukpak.yaml

################
# Hack / Tools #
################
TOOLS_DIR := hack/tools
TOOLS_BIN_DIR := $(TOOLS_DIR)/bin

##@ hack/tools:

.PHONY: golangci-lint ginkgo controller-gen goreleaser kind

GOLANGCI_LINT := $(abspath $(TOOLS_BIN_DIR)/golangci-lint)
GINKGO := $(abspath $(TOOLS_BIN_DIR)/ginkgo)
CONTROLLER_GEN := $(abspath $(TOOLS_BIN_DIR)/controller-gen)
SETUP_ENVTEST := $(abspath $(TOOLS_BIN_DIR)/setup-envtest)
GORELEASER := $(abspath $(TOOLS_BIN_DIR)/goreleaser)
KIND := $(abspath $(TOOLS_BIN_DIR)/kind)

controller-gen: $(CONTROLLER_GEN) ## Build a local copy of controller-gen
ginkgo: $(GINKGO) ## Build a local copy of ginkgo
golangci-lint: $(GOLANGCI_LINT) ## Build a local copy of golangci-lint
setup-envtest: $(SETUP_ENVTEST) ## Build a local copy of envtest
goreleaser: $(GORELEASER) ## Builds a local copy of goreleaser
kind: $(KIND) ## Builds a local copy of kind

$(CONTROLLER_GEN): $(TOOLS_DIR)/go.mod # Build controller-gen from tools folder.
	cd $(TOOLS_DIR); go build -tags=tools -o $(BIN_DIR)/controller-gen sigs.k8s.io/controller-tools/cmd/controller-gen
$(GINKGO): $(TOOLS_DIR)/go.mod # Build ginkgo from tools folder.
	cd $(TOOLS_DIR); go build -tags=tools -o $(BIN_DIR)/ginkgo github.com/onsi/ginkgo/v2/ginkgo
$(GOLANGCI_LINT): $(TOOLS_DIR)/go.mod # Build golangci-lint from tools folder.
	cd $(TOOLS_DIR); go build -tags=tools -o $(BIN_DIR)/golangci-lint github.com/golangci/golangci-lint/cmd/golangci-lint
$(SETUP_ENVTEST): $(TOOLS_DIR)/go.mod # Build setup-envtest from tools folder.
	cd $(TOOLS_DIR); go build -tags=tools -o $(BIN_DIR)/setup-envtest sigs.k8s.io/controller-runtime/tools/setup-envtest
$(GORELEASER): $(TOOLS_DIR)/go.mod # Build goreleaser from tools folder.
	cd $(TOOLS_DIR); go build -tags=tools -o $(BIN_DIR)/goreleaser github.com/goreleaser/goreleaser
$(KIND): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go build -tags=tools -o $(BIN_DIR)/kind sigs.k8s.io/kind
