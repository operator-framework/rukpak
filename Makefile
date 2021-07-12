# kernel-style V=1 build verbosity
ifeq ("$(origin V)", "command line")
  BUILD_VERBOSE = $(V)
endif

ifeq ($(BUILD_VERBOSE),1)
  Q =
else
  Q = @
endif

PKGS = $(shell go list ./... | grep -v /vendor/)

.PHONY: help
help: ## Show this help screen
	@echo 'Usage: make <OPTIONS> ... <TARGETS>'
	@echo ''
	@echo 'Available targets are:'
	@echo ''
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

# Code management
.PHONY: format tidy clean vendor generate manifests

format: ## Format the source code
	$(Q)go fmt $(PKGS)

tidy: ## Update dependencies
	$(Q)go mod tidy -v

vendor: tidy ## Update vendor directory
	$(Q)go mod vendor

generate: controller-gen  ## Generate code
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths=./...

manifests: controller-gen ## Generate manifests e.g. CRD, RBAC etc
	@# Create CRDs for new APIs
	$(CONTROLLER_GEN) crd:crdVersions=v1 output:crd:dir=./manifests paths=./api/v1alpha1/...

	@# Update existing CRDs from type changes
	$(CONTROLLER_GEN) schemapatch:manifests=./manifests output:dir=./manifests paths=./api/v1alpha1/...

# Static tests.
.PHONY: test test-unit verify

test: test-unit ## Run the tests

test-unit: ## Run the unit tests
	$(Q)go test -count=1 -short ${PKGS}

verify: tidy format manifests generate
	git diff --exit-code

# Utilities.
.PHONY: controller-gen

controller-gen: vendor ## Find or download controller-gen
CONTROLLER_GEN=$(Q)go run -mod=vendor ./vendor/sigs.k8s.io/controller-tools/cmd/controller-gen
