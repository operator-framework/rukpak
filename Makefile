
.PHONY: help
help: ## Show this help screen
	@echo 'Usage: make <OPTIONS> ... <TARGETS>'
	@echo ''
	@echo 'Available targets are:'
	@echo ''
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

# Code management
.PHONY: lint format tidy clean generate

PKGS = $(shell go list ./...)

lint: golangci-lint
	$(Q)$(GOLANGCI_LINT) run

format: ## Format the source code
	$(Q)go fmt ./...

tidy: ## Update dependencies
	$(Q)go mod tidy

generate: controller-gen ## Generate code and manifests
	$(Q)$(CONTROLLER_GEN) crd:crdVersions=v1 output:crd:dir=./manifests paths=./api/...
	$(Q)$(CONTROLLER_GEN) schemapatch:manifests=./manifests output:dir=./manifests paths=./api/...
	$(Q)$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths=./api/...

# Static tests.
.PHONY: test test-unit verify build bin/k8s bin/kuberpak

test: test-unit test-e2e ## Run the tests

UNIT_TEST_DIRS=$(shell go list ./... | grep -v /test/)
test-unit: ## Run the unit tests
	$(Q)go test -count=1 -short $(UNIT_TEST_DIRS)

test-e2e: ginkgo ## Run the e2e tests
	$(GINKGO) run test/e2e

verify: tidy generate format ## Verify the current code generation and lint
	git diff --exit-code

install: generate
	# TODO(tflannag): Introduce kuberpak manifests
	kubectl apply -f manifests
	kubectl apply -f provisioner/k8s/manifests

# Binary builds
GO_BUILD := $(Q)go build

build: bin/k8s bin/kuberpak

bin/k8s:
	CGO_ENABLED=0 go build -o $@ ./provisioner/k8s

bin/kuberpak:
	CGO_ENABLED=0 go build -o $@ ./provisioner/kuberpak


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
	CGO_ENABLED=0 go build -o $@ ./provisioner/kuberpak
