
.PHONY: help
help: ## Show this help screen
	@echo 'Usage: make <OPTIONS> ... <TARGETS>'
	@echo ''
	@echo 'Available targets are:'
	@echo ''
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

# Container build options
IMAGE_REPO=quay.io/operator-framework/plain-v0-provisioner
IMAGE_TAG=latest
IMAGE=$(IMAGE_REPO):$(IMAGE_TAG)

KIND_CLUSTER_NAME := rukpak-e2e
KIND := kind

# Code management
.PHONY: lint tidy clean generate

PKGS = $(shell go list ./...)

lint: golangci-lint
	$(Q)$(GOLANGCI_LINT) run

tidy: ## Update dependencies
	$(Q)go mod tidy

generate: controller-gen ## Generate code and manifests
	$(Q)$(CONTROLLER_GEN) crd:crdVersions=v1 output:crd:dir=./manifests paths=./api/...
	$(Q)$(CONTROLLER_GEN) schemapatch:manifests=./manifests output:dir=./manifests paths=./api/...
	$(Q)$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths=./api/...


## --------------------------------------
## Testing and Verification
## --------------------------------------
# Static tests.
.PHONY: test test-unit verify

test: test-unit test-e2e ## Run the tests

UNIT_TEST_DIRS=$(shell go list ./... | grep -v /test/)
test-unit: ## Run the unit tests
	$(Q)go test -count=1 -short $(UNIT_TEST_DIRS)

test-e2e: ginkgo ## Run the e2e tests
	$(GINKGO) -v -trace -progress test/e2e

verify: tidy generate ## Verify the current code generation and lint
	git diff --exit-code

## --------------------------------------
## Install and Run
## --------------------------------------
install-apis: generate ## Install the core rukpak CRDs
	kubectl apply -f manifests

install-plain-v0: install-apis ## Install the rukpak CRDs and the plain-v0 provisioner
	kubectl apply -f provisioner/plain-v0/manifests

install: install-plain-v0 ## Install all rukpak core CRDs and provisioners

deploy: install-apis ## Deploy the operator to the current cluster
	kubectl apply -f provisioner/plain-v0/manifests

run: build-local-container kind-load deploy ## Build image and run operator in-cluster

## --------------------------------------
## Build and Load
## --------------------------------------
.PHONY: build bin/plain-v0 bin/unpack

# Binary builds
GO_BUILD := $(Q)go build
build: bin/plain-v0 bin/unpack

bin/plain-v0:
	CGO_ENABLED=0 go build -o $@ ./provisioner/plain-v0

bin/unpack:
	CGO_ENABLED=0 go build -o $@ ./cmd/unpack/...

build-container: ## Builds provisioner container image locally
	docker build -f Dockerfile -t $(IMAGE) .

build-local-container: export GOOS=linux
build-local-container: build ## Builds the provisioner container image using locally built binaries
	docker build -f Dockerfile.local -t $(IMAGE) .

kind-load: ## Load-image loads the currently constructed image onto the cluster
	${KIND} load docker-image $(IMAGE) --name $(KIND_CLUSTER_NAME)

kind-cluster: ## Standup a kind cluster for e2e testing usage
	${KIND} delete cluster --name ${KIND_CLUSTER_NAME}
	${KIND} create cluster --name ${KIND_CLUSTER_NAME}

e2e: build-container kind-cluster kind-load deploy test-e2e ## Run e2e tests against a kind cluster

## --------------------------------------
## Hack / Tools
## --------------------------------------
BIN_DIR := bin
TOOLS_DIR := hack/tools
TOOLS_BIN_DIR := $(TOOLS_DIR)/bin

##@ hack/tools:

.PHONY: golangci-lint ginkgo controller-gen

GOLANGCI_LINT := $(abspath $(TOOLS_BIN_DIR)/golangci-lint)
GINKGO := $(abspath $(TOOLS_BIN_DIR)/ginkgo)
CONTROLLER_GEN := $(abspath $(TOOLS_BIN_DIR)/controller-gen)

controller-gen: $(CONTROLLER_GEN) ## Build a local copy of controller-gen
ginkgo: $(GINKGO) ## Build a local copy of ginkgo
golangci-lint: $(GOLANGCI_LINT) ## Build a local copy of golangci-lint

$(CONTROLLER_GEN): $(TOOLS_DIR)/go.mod # Build controller-gen from tools folder.
	cd $(TOOLS_DIR); go build -tags=tools -o $(BIN_DIR)/controller-gen sigs.k8s.io/controller-tools/cmd/controller-gen
$(GINKGO): $(TOOLS_DIR)/go.mod # Build ginkgo from tools folder.
	cd $(TOOLS_DIR); go build -tags=tools -o $(BIN_DIR)/ginkgo github.com/onsi/ginkgo/ginkgo
$(GOLANGCI_LINT): $(TOOLS_DIR)/go.mod # Build golangci-lint from tools folder.
	cd $(TOOLS_DIR); go build -tags=tools -o $(BIN_DIR)/golangci-lint github.com/golangci/golangci-lint/cmd/golangci-lint
