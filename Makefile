# kernel-style V=1 build verbosity
ifeq ("$(origin V)", "command line")
  BUILD_VERBOSE = $(V)
endif

ifeq ($(BUILD_VERBOSE),1)
  Q =
else
  Q = @
endif


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

lint:
	$(Q)go run github.com/golangci/golangci-lint/cmd/golangci-lint run

format: ## Format the source code
	$(Q)go fmt ./...

tidy: ## Update dependencies
	$(Q)go mod tidy

CONTROLLER_GEN=$(Q)go run sigs.k8s.io/controller-tools/cmd/controller-gen

generate: ## Generate code and manifests
	$(Q)go generate ./...

# Static tests.
.PHONY: test test-unit verify build bin/k8s

test: test-unit test-e2e ## Run the tests

UNIT_TEST_DIRS=$(shell go list ./... | grep -v /test/)
test-unit: ## Run the unit tests
	$(Q)go test -count=1 -short $(UNIT_TEST_DIRS)

test-e2e: ## Run the e2e tests
	go run "github.com/onsi/ginkgo/ginkgo" run test/e2e

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
	$(GO_BUILD) -o $@ ./provisioner/k8s

bin/kuberpak:
	$(GO_BUILD) -o $@ ./provisioner/kuberpak
