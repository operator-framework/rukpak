# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL := /usr/bin/env bash -o pipefail
.SHELLFLAGS := -ec

###########################
# Configuration Variables #
###########################
ORG := github.com/operator-framework
PKG := $(ORG)/rukpak

ifeq ($(origin IMAGE_REPO), undefined)
IMAGE_REPO := quay.io/operator-framework/rukpak
endif
export IMAGE_REPO
ifeq ($(origin IMAGE_TAG), undefined)
IMAGE_TAG := devel
endif
export IMAGE_TAG
IMAGE := $(IMAGE_REPO):$(IMAGE_TAG)


ifeq ($(origin KIND_CLUSTER_NAME), undefined)
KIND_CLUSTER_NAME := rukpak
endif

export CERT_MGR_VERSION := v1.9.0

BIN_DIR          := bin
VERSION_PATH     := $(PKG)/internal/version
GIT_COMMIT       := $(shell git rev-parse HEAD)
PKGS             := $(shell go list ./...)
RUKPAK_NAMESPACE := rukpak-system

REGISTRY_NAME      := "docker-registry"
REGISTRY_NAMESPACE := rukpak-e2e
DNS_NAME           := $(REGISTRY_NAME).$(REGISTRY_NAMESPACE).svc.cluster.local

ifneq (, $(shell command -v docker 2>/dev/null))
CONTAINER_RUNTIME := docker
else ifneq (, $(shell command -v podman 2>/dev/null))
CONTAINER_RUNTIME := podman
else
$(warning Could not find docker or podman in path! This may result in targets requiring a container runtime failing!)
endif

# By default setup-envtest will write to $XDG_DATA_HOME, or $HOME/.local/share if that is not defined.
# If $HOME is not set, we need to specify a binary directory to prevent an error in setup-envtest.
# Useful for some CI/CD environments that set neither $XDG_DATA_HOME nor $HOME.
SETUP_ENVTEST_BIN_DIR_OVERRIDE :=
ifeq ($(shell [[ $$HOME == "" || $$HOME == "/" ]] && [[ $$XDG_DATA_HOME == "" ]] && echo true ), true)
	SETUP_ENVTEST_BIN_DIR_OVERRIDE += --bin-dir /tmp/envtest-binaries
endif

# bingo manages consistent tooling versions for things like kind, kustomize, etc.
include .bingo/Variables.mk

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

lint: $(GOLANGCI_LINT) ## Run golangci linter
	$(GOLANGCI_LINT) run $(GOLANGCI_LINT_ARGS)

tidy: ## Update dependencies
	go mod tidy

fmt: ## Format Go code
	go fmt ./...

clean: ## Remove binaries and test artifacts
	@rm -rf bin

generate: $(CONTROLLER_GEN) ## Generate code and manifests
	$(CONTROLLER_GEN) crd:crdVersions=v1,generateEmbeddedObjectMeta=true output:crd:dir=./manifests/base/apis/crds paths=./api/...
	$(CONTROLLER_GEN) webhook paths=./api/... paths=./internal/webhook/... output:stdout > ./manifests/base/apis/webhooks/resources/webhook.yaml
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths=./api/...
	$(CONTROLLER_GEN) rbac:roleName=core-admin \
		paths=./internal/controllers/bundledeployment/... \
		paths=./internal/provisioner/plain/... \
		paths=./internal/provisioner/registry/... \
			output:stdout > ./manifests/base/core/resources/cluster_role.yaml
	$(CONTROLLER_GEN) rbac:roleName=webhooks-admin paths=./internal/webhook/... output:stdout > ./manifests/base/apis/webhooks/resources/cluster_role.yaml
	$(CONTROLLER_GEN) rbac:roleName=helm-provisioner-admin \
		paths=./internal/controllers/bundledeployment/... \
		paths=./internal/provisioner/helm/... \
		    output:stdout > ./manifests/base/provisioners/helm/resources/cluster_role.yaml

verify: tidy fmt generate ## Verify the current code generation and lint
	git diff --exit-code

###########
# Testing #
###########
.PHONY: test test-unit test-e2e image-registry local-git

##@ testing:

test: test-unit test-e2e ## Run the tests

ENVTEST_VERSION := $(shell go list -m k8s.io/client-go | cut -d" " -f2 | sed 's/^v0\.\([[:digit:]]\{1,\}\)\.[[:digit:]]\{1,\}$$/1.\1.x/')
UNIT_TEST_DIRS  := $(shell go list ./... | grep -v /test/)
test-unit: $(SETUP_ENVTEST) ## Run the unit tests
	eval $$($(SETUP_ENVTEST) use -p env $(ENVTEST_VERSION) $(SETUP_ENVTEST_BIN_DIR_OVERRIDE)) && go test -count=1 -short $(UNIT_TEST_DIRS) -coverprofile cover.out

FOCUS := $(if $(TEST),-v --focus "$(TEST)")
test-e2e: $(GINKGO) ## Run the e2e tests
	$(GINKGO) $(E2E_FLAGS) --trace $(FOCUS) test/e2e

e2e: KIND_CLUSTER_NAME := rukpak-e2e
e2e: run image-registry secure-image-registry local-git kind-load-bundles registry-load-bundles secure-registry-load-bundles test-e2e kind-cluster-cleanup ## Run e2e tests against an ephemeral kind cluster

kind-cluster: $(KIND) kind-cluster-cleanup ## Standup a kind cluster
	$(KIND) create cluster --name ${KIND_CLUSTER_NAME} ${KIND_CLUSTER_CONFIG}
	$(KIND) export kubeconfig --name ${KIND_CLUSTER_NAME}

kind-cluster-cleanup: $(KIND) ## Delete the kind cluster
	$(KIND) delete cluster --name ${KIND_CLUSTER_NAME}

image-registry: ## Setup in-cluster image registry
	./test/tools/imageregistry/image-registry.sh ${REGISTRY_NAMESPACE} ${REGISTRY_NAME}

secure-image-registry: ## Setup a private in-cluster image registry
	./test/tools/imageregistry/image-registry-secure.sh ${REGISTRY_NAMESPACE} ${REGISTRY_NAME}

local-git: ## Setup in-cluster git repository
	./test/tools/git/setup_git.sh ${KIND_CLUSTER_NAME}

###################
# Install and Run #
###################
.PHONY: install install-manifests wait run debug debug-helper cert-mgr uninstall

##@ install/run:

install: generate cert-mgr install-manifests wait ## Install rukpak

MANIFESTS_DIR := manifests/overlays/cert-manager
install-manifests:
	kubectl apply -k $(MANIFESTS_DIR)

wait:
	kubectl wait --for=condition=Available --namespace=$(RUKPAK_NAMESPACE) deployment/core --timeout=60s
	kubectl wait --for=condition=Available --namespace=$(RUKPAK_NAMESPACE) deployment/rukpak-webhooks --timeout=60s
	kubectl wait --for=condition=Available --namespace=$(RUKPAK_NAMESPACE) deployment/helm-provisioner --timeout=60s
	kubectl wait --for=condition=Available --namespace=crdvalidator-system deployment/crd-validation-webhook --timeout=60s

run: build-container kind-cluster kind-load install ## Build image, stop/start a local kind cluster, and run operator in that cluster

debug: MANIFESTS_DIR       := test/tools/remotedebug ## Same as 'run' target with the addition of remote debugging available on port 40000
debug: DEBUG_FLAGS         := -gcflags="all=-N -l"
debug: KIND_CLUSTER_CONFIG := --config=./test/tools/remotedebug/kind-config.yaml
debug: run debug-helper

debug-helper:
	@echo ''
	@echo "Remote Debugging for '$$(kubectl -n $(RUKPAK_NAMESPACE) get pods -l app=core -o name)' in namespace '$(RUKPAK_NAMESPACE)' now available through localhost:40000"

cert-mgr: ## Install the certification manager
	kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/$(CERT_MGR_VERSION)/cert-manager.yaml
	kubectl wait --for=condition=Available --namespace=cert-manager deployment/cert-manager-webhook --timeout=60s

uninstall: ## Remove all rukpak resources from the cluster
	kubectl delete -k $(MANIFESTS_DIR)

##################
# Build and Load #
##################

##@ build/load:

BINARIES := core helm unpack webhooks crdvalidator
LINUX_BINARIES=$(join $(addprefix linux/,$(BINARIES)), )

.PHONY: build $(BINARIES) $(LINUX_BINARIES) build-container kind-load kind-load-bundles kind-cluster registry-load-bundles

# Binary builds
build: $(BINARIES)

$(LINUX_BINARIES):
	CGO_ENABLED=0 GOOS=linux go build $(DEBUG_FLAGS) -o $(BIN_DIR)/$@ ./cmd/$(notdir $@)

$(BINARIES):
	CGO_ENABLED=0 go build $(DEBUG_FLAGS) -o $(BIN_DIR)/$@ ./cmd/$@ $(DEBUG_FLAGS)

build-container: $(LINUX_BINARIES) ## Builds provisioner container image locally
	$(CONTAINER_RUNTIME) build -f Dockerfile -t $(IMAGE) $(BIN_DIR)/linux

kind-load-bundles: $(KIND) ## Load the e2e testdata container images into a kind cluster
	$(CONTAINER_RUNTIME) build testdata/bundles/plain-v0/valid -t localhost/testdata/bundles/plain-v0:valid
	$(CONTAINER_RUNTIME) build testdata/bundles/plain-v0/dependent -t localhost/testdata/bundles/plain-v0:dependent
	$(CONTAINER_RUNTIME) build testdata/bundles/plain-v0/provides -t localhost/testdata/bundles/plain-v0:provides
	$(CONTAINER_RUNTIME) build testdata/bundles/plain-v0/empty -t localhost/testdata/bundles/plain-v0:empty
	$(CONTAINER_RUNTIME) build testdata/bundles/plain-v0/no-manifests -t localhost/testdata/bundles/plain-v0:no-manifests
	$(CONTAINER_RUNTIME) build testdata/bundles/plain-v0/invalid-missing-crds -t localhost/testdata/bundles/plain-v0:invalid-missing-crds
	$(CONTAINER_RUNTIME) build testdata/bundles/plain-v0/invalid-crds-and-crs -t localhost/testdata/bundles/plain-v0:invalid-crds-and-crs
	$(CONTAINER_RUNTIME) build testdata/bundles/plain-v0/subdir -t localhost/testdata/bundles/plain-v0:subdir
	$(CONTAINER_RUNTIME) build testdata/bundles/registry/valid -t localhost/testdata/bundles/registry:valid
	$(CONTAINER_RUNTIME) build testdata/bundles/registry/invalid -t localhost/testdata/bundles/registry:invalid
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

kind-load: $(KIND) ## Loads the currently constructed image onto the cluster
	$(KIND) load docker-image $(IMAGE) --name $(KIND_CLUSTER_NAME)

registry-load-bundles: ## Load selected e2e testdata container images created in kind-load-bundles into registry
	testdata/bundles/plain-v0/valid/build-push-e2e-bundle.sh ${REGISTRY_NAMESPACE} $(DNS_NAME):5000/bundles/plain-v0:valid
	testdata/bundles/plain-v0/dependent/build-push-e2e-bundle.sh ${REGISTRY_NAMESPACE} $(DNS_NAME):5000/bundles/plain-v0:dependent
	testdata/bundles/plain-v0/provides/build-push-e2e-bundle.sh ${REGISTRY_NAMESPACE} $(DNS_NAME):5000/bundles/plain-v0:provides
	testdata/bundles/plain-v0/empty/build-push-e2e-bundle.sh ${REGISTRY_NAMESPACE} $(DNS_NAME):5000/bundles/plain-v0:empty
	testdata/bundles/plain-v0/no-manifests/build-push-e2e-bundle.sh ${REGISTRY_NAMESPACE} $(DNS_NAME):5000/bundles/plain-v0:no-manifests
	testdata/bundles/plain-v0/invalid-missing-crds/build-push-e2e-bundle.sh ${REGISTRY_NAMESPACE} $(DNS_NAME):5000/bundles/plain-v0:invalid-missing-crds
	testdata/bundles/plain-v0/invalid-crds-and-crs/build-push-e2e-bundle.sh ${REGISTRY_NAMESPACE} $(DNS_NAME):5000/bundles/plain-v0:invalid-crds-and-crs
	testdata/bundles/plain-v0/subdir/build-push-e2e-bundle.sh ${REGISTRY_NAMESPACE} $(DNS_NAME):5000/bundles/plain-v0:subdir
	testdata/bundles/registry/valid/build-push-e2e-bundle.sh ${REGISTRY_NAMESPACE} $(DNS_NAME):5000/bundles/registry:valid
	testdata/bundles/registry/invalid/build-push-e2e-bundle.sh ${REGISTRY_NAMESPACE} $(DNS_NAME):5000/bundles/registry:invalid
	
secure-registry-load-bundles: ## Load selected e2e testdata container images created in kind-load-bundles into private registry
	testdata/bundles/plain-v0/valid/build-push-e2e-bundle-secure.sh ${REGISTRY_NAMESPACE} docker-registry-secure.rukpak-e2e.svc.cluster.local:5000/bundles/plain-v0:valid

###########
# Release #
###########

##@ release:

ifeq ($(origin ENABLE_RELEASE_PIPELINE), undefined)
ENABLE_RELEASE_PIPELINE := false
endif
export ENABLE_RELEASE_PIPELINE

ifeq ($(origin GORELEASER_ARGS), undefined)
release: GORELEASER_ARGS := --snapshot --clean
endif
release: $(GORELEASER) ## Run goreleaser
	$(GORELEASER) $(GORELEASER_ARGS)


ifeq ($(origin VERSION), undefined)
VERSION := $(shell git describe --abbrev=0 --tags)
endif
quickstart: $(KUSTOMIZE) generate ## Generate the installation release manifests
	$(KUSTOMIZE) build manifests/overlays/cert-manager | sed "s/:devel/:$(VERSION)/g" > rukpak.yaml
