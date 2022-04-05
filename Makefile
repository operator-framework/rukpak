###########################
# Configuration Variables #
###########################
ORG := github.com/operator-framework
PKG := $(ORG)/rukpak
IMAGE_REPO=quay.io/operator-framework/plain-provisioner
IMAGE_TAG=latest
IMAGE?=$(IMAGE_REPO):$(IMAGE_TAG)
KIND_CLUSTER_NAME ?= kind
KIND := kind
BIN_DIR := bin
TESTDATA_DIR := testdata
VERSION_PATH := $(PKG)/internal/version
GIT_COMMIT ?= $(shell git rev-parse HEAD)
PKGS = $(shell go list ./...)
CERT_MGR_VERSION=v1.7.1

CONTAINER_RUNTIME ?= docker

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
.PHONY: lint tidy clean generate verify

##@ code management:

lint: golangci-lint ## Run golangci linter
	$(Q)$(GOLANGCI_LINT) run

tidy: ## Update dependencies
	$(Q)go mod tidy

clean: ## Remove binaries and test artifacts
	@rm -rf bin

generate: controller-gen ## Generate code and manifests
	$(Q)$(CONTROLLER_GEN) crd:crdVersions=v1 output:crd:dir=./manifests paths=./api/...
	$(Q)$(CONTROLLER_GEN) schemapatch:manifests=./manifests output:dir=./manifests paths=./api/...
	$(Q)$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths=./api/...

verify: tidy generate ## Verify the current code generation and lint
	git diff --exit-code

###########
# Testing #
###########
.PHONY: test test-unit test-e2e

##@ testing:

test: test-unit test-e2e ## Run the tests

ENVTEST_VERSION = $(shell go list -m k8s.io/client-go | cut -d" " -f2 | sed 's/^v0\.\([[:digit:]]\{1,\}\)\.[[:digit:]]\{1,\}$$/1.\1.x/')
UNIT_TEST_DIRS=$(shell go list ./... | grep -v /test/)
test-unit: setup-envtest ## Run the unit tests
	eval $$($(SETUP_ENVTEST) use -p env $(ENVTEST_VERSION)) && go test -count=1 -short $(UNIT_TEST_DIRS)

FOCUS := $(if $(TEST),-v -focus "$(TEST)")
test-e2e: ginkgo ## Run the e2e tests
	$(GINKGO) -trace -progress $(FOCUS) test/e2e

e2e: KIND_CLUSTER_NAME=rukpak-e2e
e2e: build-container kind-cluster kind-load cert-mgr kind-load-bundles deploy test-e2e ## Run e2e tests against a kind cluster

kind-cluster: ## Standup a kind cluster for e2e testing usage
	${KIND} delete cluster --name ${KIND_CLUSTER_NAME}
	${KIND} create cluster --name ${KIND_CLUSTER_NAME}

###################
# Install and Run #
###################
.PHONY: install-apis install-plain install deploy run

##@ install/run:

install-apis: cert-mgr generate ## Install the core rukpak CRDs
	kubectl apply -f manifests
	kubectl apply -f manifests/bundle-webhook

install-plain: install-apis ## Install the rukpak CRDs and the plain provisioner
	kubectl apply -f internal/provisioner/plain/manifests

install: install-plain ## Install all rukpak core CRDs and provisioners

deploy: install-apis ## Deploy the operator to the current cluster
	kubectl apply -f internal/provisioner/plain/manifests

run: build-container kind-load cert-mgr deploy ## Build image and run operator in-cluster

cert-mgr: ## Install the certification manager
	kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/$(CERT_MGR_VERSION)/cert-manager.yaml
	kubectl wait --for=condition=Available --namespace=cert-manager deployment/cert-manager-webhook --timeout=60s

##################
# Build and Load #
##################
.PHONY: build plain unpack core build-container kind-load kind-load-bundles kind-cluster

##@ build/load:

# Binary builds
VERSION_FLAGS=-ldflags "-X $(VERSION_PATH).GitCommit=$(GIT_COMMIT)"
build: plain unpack core

plain:
	CGO_ENABLED=0 go build $(VERSION_FLAGS) -o $(BIN_DIR)/$@ ./internal/provisioner/plain

unpack:
	CGO_ENABLED=0 go build $(VERSION_FLAGS) -o $(BIN_DIR)/$@ ./cmd/unpack/...

core:
	CGO_ENABLED=0 go build $(VERSION_FLAGS) -o $(BIN_DIR)/$@ ./cmd/core/...

build-container: export GOOS=linux
build-container: BIN_DIR:=$(BIN_DIR)/$(GOOS)
build-container: build ## Builds provisioner container image locally
	$(CONTAINER_RUNTIME) build -f Dockerfile -t $(IMAGE) $(BIN_DIR)

kind-load-bundles: ## Load the e2e testdata container images into a kind cluster
	$(CONTAINER_RUNTIME) build $(TESTDATA_DIR)/bundles/plain-v0/valid -t testdata/bundles/plain-v0:valid
	$(CONTAINER_RUNTIME) build $(TESTDATA_DIR)/bundles/plain-v0/dependent -t testdata/bundles/plain-v0:dependent
	$(CONTAINER_RUNTIME) build $(TESTDATA_DIR)/bundles/plain-v0/provides -t testdata/bundles/plain-v0:provides
	$(CONTAINER_RUNTIME) build $(TESTDATA_DIR)/bundles/plain-v0/empty -t testdata/bundles/plain-v0:empty
	$(CONTAINER_RUNTIME) build $(TESTDATA_DIR)/bundles/plain-v0/no-manifests -t testdata/bundles/plain-v0:no-manifests
	$(CONTAINER_RUNTIME) build $(TESTDATA_DIR)/bundles/plain-v0/invalid-missing-crds -t testdata/bundles/plain-v0:invalid-missing-crds
	$(CONTAINER_RUNTIME) build $(TESTDATA_DIR)/bundles/plain-v0/invalid-crds-and-crs -t testdata/bundles/plain-v0:invalid-crds-and-crs
	$(CONTAINER_RUNTIME) build $(TESTDATA_DIR)/bundles/plain-v0/subdir -t testdata/bundles/plain-v0:subdir
	${KIND} load docker-image testdata/bundles/plain-v0:valid --name $(KIND_CLUSTER_NAME)
	${KIND} load docker-image testdata/bundles/plain-v0:dependent --name $(KIND_CLUSTER_NAME)
	${KIND} load docker-image testdata/bundles/plain-v0:provides --name $(KIND_CLUSTER_NAME)
	${KIND} load docker-image testdata/bundles/plain-v0:empty --name $(KIND_CLUSTER_NAME)
	${KIND} load docker-image testdata/bundles/plain-v0:no-manifests --name $(KIND_CLUSTER_NAME)
	${KIND} load docker-image testdata/bundles/plain-v0:invalid-missing-crds --name $(KIND_CLUSTER_NAME)
	${KIND} load docker-image testdata/bundles/plain-v0:invalid-crds-and-crs --name $(KIND_CLUSTER_NAME)
	${KIND} load docker-image testdata/bundles/plain-v0:subdir --name $(KIND_CLUSTER_NAME)

kind-load: ## Load-image loads the currently constructed image onto the cluster
	${KIND} load docker-image $(IMAGE) --name $(KIND_CLUSTER_NAME)

################
# Hack / Tools #
################
TOOLS_DIR := hack/tools
TOOLS_BIN_DIR := $(TOOLS_DIR)/bin

##@ hack/tools:

.PHONY: golangci-lint ginkgo controller-gen goreleaser

GOLANGCI_LINT := $(abspath $(TOOLS_BIN_DIR)/golangci-lint)
GINKGO := $(abspath $(TOOLS_BIN_DIR)/ginkgo)
CONTROLLER_GEN := $(abspath $(TOOLS_BIN_DIR)/controller-gen)
SETUP_ENVTEST := $(abspath $(TOOLS_BIN_DIR)/setup-envtest)
GORELEASER := $(abspath $(TOOLS_BIN_DIR)/goreleaser)

release: GORELEASER_ARGS ?= --snapshot --rm-dist
release: goreleaser
	$(GORELEASER) $(GORELEASER_ARGS)

controller-gen: $(CONTROLLER_GEN) ## Build a local copy of controller-gen
ginkgo: $(GINKGO) ## Build a local copy of ginkgo
golangci-lint: $(GOLANGCI_LINT) ## Build a local copy of golangci-lint
setup-envtest: $(SETUP_ENVTEST) ## Build a local copy of envtest
goreleaser: $(GORELEASER) ## Builds a local copy of goreleaser

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
