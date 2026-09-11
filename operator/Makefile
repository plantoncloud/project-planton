# Image URL to use all building/pushing image targets
IMG ?= planton-operator:latest

# Kind cluster name for local development
KIND_DEV_CLUSTER ?= planton-operator-dev

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
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

# The Helm chart that packages this operator ships controller-gen's output
# verbatim: the CRDs as templates under templates/crds/ (wrapped by
# hack/chartcrds so the chart owns their lifecycle) and the manager ClusterRole
# rules read by templates/clusterrole.yaml from rbac/. One target writes
# both homes so a type or RBAC marker change and its chart change are always
# the same commit; CI regenerates and diffs, so a stale chart fails the build.
CHART_DIR := ../helm/planton-operator
CHART_CRD_TEMPLATES_DIR := $(CHART_DIR)/templates/crds
CHART_RBAC_DIR := $(CHART_DIR)/rbac

.PHONY: manifests
manifests: controller-gen ## Generate ClusterRole and CustomResourceDefinition objects into config/ and the Helm chart.
	"$(CONTROLLER_GEN)" rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	@go run ./hack/chartcrds --in config/crd/bases --out $(CHART_CRD_TEMPLATES_DIR)
	@mkdir -p $(CHART_RBAC_DIR)
	@cp config/rbac/role.yaml $(CHART_RBAC_DIR)/manager-role.yaml

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	"$(CONTROLLER_GEN)" object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet setup-envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell "$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: test-realm-convergence
test-realm-convergence: ## Run the Keycloak realm-convergence + federation suite (requires Docker; boots a real Keycloak and the lab directory).
	go test -tags=requires_docker ./internal/keycloak/ -run 'TestConvergence|TestFederation|TestEmailTheme' -v -timeout 20m

# TODO(user): To use a different vendor for e2e tests, modify the setup under 'tests/e2e'.
# The default setup assumes Kind is pre-installed and builds/loads the Manager Docker image locally.
# CertManager is installed by default; skip with:
# - CERT_MANAGER_INSTALL_SKIP=true
KIND_CLUSTER ?= planton-operator-test-e2e

.PHONY: setup-test-e2e
setup-test-e2e: ## Set up a Kind cluster for e2e tests if it does not exist
	@command -v $(KIND) >/dev/null 2>&1 || { \
		echo "Kind is not installed. Please install Kind manually."; \
		exit 1; \
	}
	@case "$$($(KIND) get clusters)" in \
		*"$(KIND_CLUSTER)"*) \
			echo "Kind cluster '$(KIND_CLUSTER)' already exists. Skipping creation." ;; \
		*) \
			echo "Creating Kind cluster '$(KIND_CLUSTER)'..."; \
			$(KIND) create cluster --name $(KIND_CLUSTER) ;; \
	esac

.PHONY: test-e2e
# The suite builds and side-loads the manager image before its first spec, so a
# cold build cache alone can consume Go's default 10-minute test timeout.
test-e2e: setup-test-e2e manifests generate fmt vet ## Run the e2e tests. Expected an isolated environment using Kind.
	KIND=$(KIND) KIND_CLUSTER=$(KIND_CLUSTER) go test -tags=e2e ./test/e2e/ -v -ginkgo.v -timeout 40m
	$(MAKE) cleanup-test-e2e

.PHONY: cleanup-test-e2e
cleanup-test-e2e: ## Tear down the Kind cluster used for e2e tests
	@$(KIND) delete cluster --name $(KIND_CLUSTER)

# The chart lifecycle suite (test/chartlifecycle) proves what helm/planton-operator
# promises for the definitions it owns -- install, keep, adopt, upgrade -- on its
# own Kind cluster, driving Helm through the SDK against the working-tree chart
# and the last published charts that predate that ownership.
KIND_CHART_CLUSTER ?= planton-operator-test-chart

.PHONY: test-chart-lifecycle
test-chart-lifecycle: manifests generate fmt vet ## Run the chart lifecycle suite on a dedicated Kind cluster.
	@case "$$($(KIND) get clusters)" in \
		*"$(KIND_CHART_CLUSTER)"*) \
			echo "Kind cluster '$(KIND_CHART_CLUSTER)' already exists. Skipping creation." ;; \
		*) \
			echo "Creating Kind cluster '$(KIND_CHART_CLUSTER)'..."; \
			$(KIND) create cluster --name $(KIND_CHART_CLUSTER) ;; \
	esac
	KIND=$(KIND) KIND_CLUSTER=$(KIND_CHART_CLUSTER) go test -tags=e2e ./test/chartlifecycle/ -v -ginkgo.v -timeout 30m
	@$(KIND) delete cluster --name $(KIND_CHART_CLUSTER)

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	"$(GOLANGCI_LINT)" run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	"$(GOLANGCI_LINT)" run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	"$(GOLANGCI_LINT)" config verify

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: ## Build docker image with the manager. OPERATOR_VERSION=vX.Y.Z stamps a release build (the release workflow does); unset, the image is a development build that never judges a platform's operator requirement.
	$(CONTAINER_TOOL) build -t ${IMG} $(if $(OPERATOR_VERSION),--build-arg OPERATOR_VERSION=$(OPERATOR_VERSION),) .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
# The platforms the published operator image carries. The Dockerfile's builder
# stage already pins itself to BUILDPLATFORM and cross-compiles, so no
# Dockerfile rewrite is needed here.
PLATFORMS ?= linux/amd64,linux/arm64
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	- $(CONTAINER_TOOL) buildx create --name planton-operator-builder
	$(CONTAINER_TOOL) buildx use planton-operator-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} .
	- $(CONTAINER_TOOL) buildx rm planton-operator-builder

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && "$(KUSTOMIZE)" edit set image controller=${IMG}
	"$(KUSTOMIZE)" build config/default > dist/install.yaml

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	@out="$$( "$(KUSTOMIZE)" build config/crd 2>/dev/null || true )"; \
	if [ -n "$$out" ]; then echo "$$out" | "$(KUBECTL)" apply -f -; else echo "No CRDs to install; skipping."; fi

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	@out="$$( "$(KUSTOMIZE)" build config/crd 2>/dev/null || true )"; \
	if [ -n "$$out" ]; then echo "$$out" | "$(KUBECTL)" delete --ignore-not-found=$(ignore-not-found) -f -; else echo "No CRDs to delete; skipping."; fi

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && "$(KUSTOMIZE)" edit set image controller=${IMG}
	"$(KUSTOMIZE)" build config/default | "$(KUBECTL)" apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	"$(KUSTOMIZE)" build config/default | "$(KUBECTL)" delete --ignore-not-found=$(ignore-not-found) -f -

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p "$(LOCALBIN)"

## Tool Binaries
KUBECTL ?= kubectl
KIND ?= kind
HELM ?= helm
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint

## Tool Versions
KUSTOMIZE_VERSION ?= v5.8.1
CONTROLLER_TOOLS_VERSION ?= v0.20.1

#ENVTEST_VERSION is the version of controller-runtime release branch to fetch the envtest setup script (i.e. release-0.20)
ENVTEST_VERSION ?= $(shell v='$(call gomodver,sigs.k8s.io/controller-runtime)'; \
  [ -n "$$v" ] || { echo "Set ENVTEST_VERSION manually (controller-runtime replace has no tag)" >&2; exit 1; }; \
  printf '%s\n' "$$v" | sed -E 's/^v?([0-9]+)\.([0-9]+).*/release-\1.\2/')

#ENVTEST_K8S_VERSION is the version of Kubernetes to use for setting up ENVTEST binaries (i.e. 1.31)
ENVTEST_K8S_VERSION ?= $(shell v='$(call gomodver,k8s.io/api)'; \
  [ -n "$$v" ] || { echo "Set ENVTEST_K8S_VERSION manually (k8s.io/api replace has no tag)" >&2; exit 1; }; \
  printf '%s\n' "$$v" | sed -E 's/^v?[0-9]+\.([0-9]+).*/1.\1/')

GOLANGCI_LINT_VERSION ?= v2.8.0
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@"$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
	}

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))
	@test -f .custom-gcl.yml && { \
		echo "Building custom golangci-lint with plugins..." && \
		$(GOLANGCI_LINT) custom --destination $(LOCALBIN) --name golangci-lint-custom && \
		mv -f $(LOCALBIN)/golangci-lint-custom $(GOLANGCI_LINT); \
	} || true

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] && [ "$$(readlink -- "$(1)" 2>/dev/null)" = "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f "$(1)" ;\
GOBIN="$(LOCALBIN)" go install $${package} ;\
mv "$(LOCALBIN)/$$(basename "$(1)")" "$(1)-$(3)" ;\
} ;\
ln -sf "$$(realpath "$(1)-$(3)")" "$(1)"
endef

define gomodver
$(shell go list -m -f '{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}' $(1) 2>/dev/null)
endef

##@ Embedded Manifests

# Pinned Helm chart versions for reproducible manifest generation.
# Bump the version here and run the corresponding generate-manifests-* target
# to update the embedded YAML files committed to the repository.

# CloudNativePG ships a single-file release manifest (like Tekton), vendored
# verbatim. The version pin matches the OSS catalog's kubernetescloudnativepgoperator
# kind (chart 0.29.0 == operator 1.30.0) so both engines stay in lockstep.
CNPG_VERSION ?= 1.30.0
CNPG_MANIFESTS_DIR := internal/resources/manifests/cloudnative-pg

# The Barman Cloud plugin is CloudNativePG's backup engine (WAL archiving,
# base backups, and recovery to an object store) since the in-tree backup
# path was deprecated. It ships as a Helm chart, rendered at runtime into
# CloudNativePG's own namespace -- the operator discovers plugins only there.
# The pin matches the OSS catalog's kubernetescnpgbarmancloudplugin kind
# (chart 0.7.0 == plugin v0.13.0) so a platform-operator cluster and a
# catalog-installed cluster run the same plugin; move the two pins together
# and read the plugin's CloudNativePG floor at the new pin (1.26+ today).
BARMAN_PLUGIN_HELM_CHART_VERSION ?= 0.7.0
BARMAN_PLUGIN_HELM_CHART_REPO_NAME := cnpg
BARMAN_PLUGIN_HELM_CHART_REPO_URL := https://cloudnative-pg.github.io/charts
BARMAN_PLUGIN_CHART_DIR := internal/resources/manifests/barman-plugin-chart

OPENFGA_HELM_CHART_VERSION ?= 0.3.13
OPENFGA_HELM_CHART_REPO_NAME := openfga
OPENFGA_HELM_CHART_REPO_URL := https://openfga.github.io/helm-charts
OPENFGA_CHART_DIR := internal/resources/manifests/openfga-chart

TEMPORAL_HELM_CHART_VERSION ?= 0.62.0
TEMPORAL_HELM_CHART_REPO_NAME := temporal
TEMPORAL_HELM_CHART_REPO_URL := https://go.temporal.io/helm-charts
TEMPORAL_CHART_DIR := internal/resources/manifests/temporal

# The redis-protocol cache role is served by Valkey (BSD-3-Clause), not
# Redis 8+ (RSALv2/SSPLv1/AGPLv3) -- see internal/resources/valkey_helm.go.
# The Bitnami valkey chart is published OCI-only (no classic repo index).
VALKEY_HELM_CHART_VERSION ?= 3.0.31
VALKEY_HELM_CHART_OCI_REF := oci://registry-1.docker.io/bitnamicharts/valkey
VALKEY_CHART_DIR := internal/resources/manifests/valkey-chart



OPENBAO_HELM_CHART_VERSION ?= 0.25.6
OPENBAO_HELM_CHART_REPO_NAME := openbao
OPENBAO_HELM_CHART_REPO_URL := https://openbao.github.io/openbao-helm
OPENBAO_CHART_DIR := internal/resources/manifests/openbao-chart

NEO4J_HELM_CHART_VERSION ?= 2026.1.4
NEO4J_HELM_CHART_REPO_NAME := neo4j
NEO4J_HELM_CHART_REPO_URL := https://helm.neo4j.com/neo4j
NEO4J_CHART_DIR := internal/resources/manifests/neo4j-chart

TEKTON_PIPELINES_VERSION ?= v1.15.0
TEKTON_PIPELINES_MANIFESTS_DIR := internal/resources/manifests/tekton-pipelines

.PHONY: pull-charts
pull-charts: pull-chart-valkey pull-chart-openfga pull-chart-temporal pull-chart-openbao pull-chart-neo4j pull-chart-barman-plugin ## Pull all embedded Helm chart archives (.tgz) for runtime rendering.

.PHONY: generate-manifests
generate-manifests: generate-manifests-cnpg generate-manifests-tekton pull-charts ## Regenerate all embedded third-party manifests and pull all Helm chart archives.

.PHONY: generate-manifests-cnpg
generate-manifests-cnpg: ## Regenerate the vendored CloudNativePG operator release manifest.
	@echo "Downloading CloudNativePG release manifest ($(CNPG_VERSION))..."
	@mkdir -p $(CNPG_MANIFESTS_DIR)
	@curl -sSfL \
		"https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-$(shell echo $(CNPG_VERSION) | cut -d. -f1-2)/releases/cnpg-$(CNPG_VERSION).yaml" \
		-o $(CNPG_MANIFESTS_DIR)/release.yaml
	@echo "Wrote $(CNPG_MANIFESTS_DIR)/release.yaml"

.PHONY: generate-manifests-tekton
generate-manifests-tekton: ## Regenerate the vendored Tekton Pipelines release manifest.
	@echo "Downloading Tekton Pipelines release manifest ($(TEKTON_PIPELINES_VERSION))..."
	@mkdir -p $(TEKTON_PIPELINES_MANIFESTS_DIR)
	@curl -sSfL \
		"https://github.com/tektoncd/pipeline/releases/download/$(TEKTON_PIPELINES_VERSION)/release.yaml" \
		-o $(TEKTON_PIPELINES_MANIFESTS_DIR)/release.yaml
	@echo "Wrote $(TEKTON_PIPELINES_MANIFESTS_DIR)/release.yaml"

.PHONY: pull-chart-openfga generate-manifests-openfga
pull-chart-openfga: ## Download OpenFGA Helm chart tgz for runtime rendering.
	@command -v $(HELM) >/dev/null 2>&1 || { echo "Error: helm is not installed."; exit 1; }
	@echo "Downloading OpenFGA Helm chart (v$(OPENFGA_HELM_CHART_VERSION))..."
	@$(HELM) repo add $(OPENFGA_HELM_CHART_REPO_NAME) $(OPENFGA_HELM_CHART_REPO_URL) 2>/dev/null || true
	@$(HELM) repo update $(OPENFGA_HELM_CHART_REPO_NAME) >/dev/null
	@mkdir -p $(OPENFGA_CHART_DIR)
	@rm -f $(OPENFGA_CHART_DIR)/*.tgz
	@$(HELM) pull $(OPENFGA_HELM_CHART_REPO_NAME)/openfga --version $(OPENFGA_HELM_CHART_VERSION) --destination $(OPENFGA_CHART_DIR)
	@echo "Wrote $(OPENFGA_CHART_DIR)/openfga-$(OPENFGA_HELM_CHART_VERSION).tgz"
generate-manifests-openfga: pull-chart-openfga

.PHONY: pull-chart-temporal generate-manifests-temporal
pull-chart-temporal: ## Download Temporal Helm chart tgz for runtime rendering.
	@command -v $(HELM) >/dev/null 2>&1 || { echo "Error: helm is not installed."; exit 1; }
	@echo "Downloading Temporal Helm chart (v$(TEMPORAL_HELM_CHART_VERSION))..."
	@$(HELM) repo add $(TEMPORAL_HELM_CHART_REPO_NAME) $(TEMPORAL_HELM_CHART_REPO_URL) 2>/dev/null || true
	@$(HELM) repo update $(TEMPORAL_HELM_CHART_REPO_NAME) >/dev/null
	@mkdir -p $(TEMPORAL_CHART_DIR)
	@rm -f $(TEMPORAL_CHART_DIR)/*.tgz
	@$(HELM) pull $(TEMPORAL_HELM_CHART_REPO_NAME)/temporal --version $(TEMPORAL_HELM_CHART_VERSION) --destination $(TEMPORAL_CHART_DIR)
	@echo "Wrote $(TEMPORAL_CHART_DIR)/temporal-$(TEMPORAL_HELM_CHART_VERSION).tgz"
generate-manifests-temporal: pull-chart-temporal

.PHONY: pull-chart-valkey
pull-chart-valkey: ## Download Bitnami Valkey Helm chart tgz for runtime rendering.
	@command -v $(HELM) >/dev/null 2>&1 || { echo "Error: helm is not installed."; exit 1; }
	@echo "Downloading Bitnami Valkey chart (v$(VALKEY_HELM_CHART_VERSION))..."
	@mkdir -p $(VALKEY_CHART_DIR)
	@rm -f $(VALKEY_CHART_DIR)/*.tgz
	@$(HELM) pull $(VALKEY_HELM_CHART_OCI_REF) --version $(VALKEY_HELM_CHART_VERSION) --destination $(VALKEY_CHART_DIR)
	@echo "Wrote $(VALKEY_CHART_DIR)/valkey-$(VALKEY_HELM_CHART_VERSION).tgz"


.PHONY: pull-chart-openbao
pull-chart-openbao: ## Download OpenBAO Helm chart tgz for runtime rendering.
	@command -v $(HELM) >/dev/null 2>&1 || { echo "Error: helm is not installed."; exit 1; }
	@echo "Downloading OpenBAO chart (v$(OPENBAO_HELM_CHART_VERSION))..."
	@$(HELM) repo add $(OPENBAO_HELM_CHART_REPO_NAME) $(OPENBAO_HELM_CHART_REPO_URL) 2>/dev/null || true
	@$(HELM) repo update $(OPENBAO_HELM_CHART_REPO_NAME) >/dev/null
	@mkdir -p $(OPENBAO_CHART_DIR)
	@rm -f $(OPENBAO_CHART_DIR)/*.tgz
	@$(HELM) pull $(OPENBAO_HELM_CHART_REPO_NAME)/openbao --version $(OPENBAO_HELM_CHART_VERSION) --destination $(OPENBAO_CHART_DIR)
	@echo "Wrote $(OPENBAO_CHART_DIR)/openbao-$(OPENBAO_HELM_CHART_VERSION).tgz"

.PHONY: pull-chart-barman-plugin
pull-chart-barman-plugin: ## Download the CloudNativePG Barman Cloud plugin chart tgz for runtime rendering.
	@command -v $(HELM) >/dev/null 2>&1 || { echo "Error: helm is not installed."; exit 1; }
	@echo "Downloading CloudNativePG Barman Cloud plugin chart (v$(BARMAN_PLUGIN_HELM_CHART_VERSION))..."
	@$(HELM) repo add $(BARMAN_PLUGIN_HELM_CHART_REPO_NAME) $(BARMAN_PLUGIN_HELM_CHART_REPO_URL) 2>/dev/null || true
	@$(HELM) repo update $(BARMAN_PLUGIN_HELM_CHART_REPO_NAME) >/dev/null
	@mkdir -p $(BARMAN_PLUGIN_CHART_DIR)
	@rm -f $(BARMAN_PLUGIN_CHART_DIR)/*.tgz
	@$(HELM) pull $(BARMAN_PLUGIN_HELM_CHART_REPO_NAME)/plugin-barman-cloud --version $(BARMAN_PLUGIN_HELM_CHART_VERSION) --destination $(BARMAN_PLUGIN_CHART_DIR)
	@echo "Wrote $(BARMAN_PLUGIN_CHART_DIR)/plugin-barman-cloud-$(BARMAN_PLUGIN_HELM_CHART_VERSION).tgz"

.PHONY: pull-chart-neo4j
pull-chart-neo4j: ## Download Neo4j Helm chart tgz for runtime rendering.
	@command -v $(HELM) >/dev/null 2>&1 || { echo "Error: helm is not installed."; exit 1; }
	@echo "Downloading Neo4j chart (v$(NEO4J_HELM_CHART_VERSION))..."
	@$(HELM) repo add $(NEO4J_HELM_CHART_REPO_NAME) $(NEO4J_HELM_CHART_REPO_URL) 2>/dev/null || true
	@$(HELM) repo update $(NEO4J_HELM_CHART_REPO_NAME) >/dev/null
	@mkdir -p $(NEO4J_CHART_DIR)
	@rm -f $(NEO4J_CHART_DIR)/*.tgz
	@$(HELM) pull $(NEO4J_HELM_CHART_REPO_NAME)/neo4j --version $(NEO4J_HELM_CHART_VERSION) --destination $(NEO4J_CHART_DIR)
	@echo "Wrote $(NEO4J_CHART_DIR)/neo4j-$(NEO4J_HELM_CHART_VERSION).tgz"

##@ Kind Development

.PHONY: kind-create
kind-create: ## Create a Kind cluster for local development.
	@command -v $(KIND) >/dev/null 2>&1 || { echo "Kind is not installed."; exit 1; }
	@case "$$($(KIND) get clusters 2>/dev/null)" in \
		*"$(KIND_DEV_CLUSTER)"*) \
			echo "Kind cluster '$(KIND_DEV_CLUSTER)' already exists." ;; \
		*) \
			echo "Creating Kind cluster '$(KIND_DEV_CLUSTER)'..."; \
			$(KIND) create cluster --name $(KIND_DEV_CLUSTER) --config hack/kind-config.yaml; \
			echo "Kind cluster '$(KIND_DEV_CLUSTER)' created." ;; \
	esac

.PHONY: kind-delete
kind-delete: ## Delete the Kind development cluster.
	@$(KIND) delete cluster --name $(KIND_DEV_CLUSTER) 2>/dev/null || true
	@echo "Kind cluster '$(KIND_DEV_CLUSTER)' deleted."

.PHONY: kind-reset
kind-reset: kind-delete kind-create install ## Delete + recreate Kind cluster and install CRDs.
	@echo "Kind cluster '$(KIND_DEV_CLUSTER)' reset with CRDs installed."

.PHONY: kind-deploy
kind-deploy: docker-build ## Build image and load into the Kind cluster, then deploy.
	$(KIND) load docker-image $(IMG) --name $(KIND_DEV_CLUSTER)
	$(MAKE) deploy

.PHONY: kind-e2e-lite
kind-e2e-lite: kind-create install kind-deploy ## Full lite E2E: create cluster, build, deploy, apply lite CR.
	@echo "Applying lite PlantonPlatform CR..."
	@$(KUBECTL) apply -f config/samples/lite.yaml
	@echo ""
	@echo "Operator deployed and lite CR applied."
	@echo "Monitor with: make kind-logs"
	@echo "Check status: make kind-status"

.PHONY: kind-logs
kind-logs: ## Stream operator controller logs from the Kind cluster.
	@$(KUBECTL) logs -n planton-operator-system deployment/planton-operator-controller-manager -c manager -f

.PHONY: kind-status
kind-status: ## Show PlantonPlatform CR status and component statuses.
	@echo "=== PlantonPlatform Resources ==="
	@$(KUBECTL) get plantonplatform -A -o wide 2>/dev/null || echo "No PlantonPlatform resources found."
	@echo ""
	@echo "=== CR Status (JSON) ==="
	@$(KUBECTL) get plantonplatform planton -o jsonpath='{.status}' 2>/dev/null | python3 -m json.tool 2>/dev/null || echo "CR 'planton' not found or status empty."
	@echo ""
	@echo "=== Conditions ==="
	@$(KUBECTL) get plantonplatform planton -o jsonpath='{range .status.conditions[*]}{.type}{"\t"}{.status}{"\t"}{.reason}{"\t"}{.message}{"\n"}{end}' 2>/dev/null || true
	@echo ""
	@echo "=== Pods ==="
	@$(KUBECTL) get pods -A --sort-by=.metadata.creationTimestamp 2>/dev/null | grep -v kube-system || true
