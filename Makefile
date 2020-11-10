# ====================================================================================
# Setup Project
PROJECT_NAME := oam-kubernetes-runtime
PROJECT_REPO := github.com/crossplane/$(PROJECT_NAME)

PLATFORMS ?= linux_amd64 linux_arm64
# -include will silently skip missing files, which allows us
# to load those files with a target in the Makefile. If only
# "include" was used, the make command would fail and refuse
# to run a target until the include commands succeeded.
-include build/makelib/common.mk

# ====================================================================================
# Setup Output

S3_BUCKET ?= crossplane.releases/oam
-include build/makelib/output.mk

# ====================================================================================
# Setup Kubernetes tools

-include build/makelib/k8s_tools.mk

# ====================================================================================
# Setup Helm

HELM_BASE_URL = https://charts.crossplane.io
HELM_S3_BUCKET = crossplane.charts
HELM_CHARTS_DIR=$(ROOT_DIR)/charts
HELM_CHART = oam-kubernetes-runtime
LEGACY_HELM_CHART = oam-kubernetes-runtime-legacy
HELM_CHARTS = $(HELM_CHART) $(LEGACY_HELM_CHART)
LEGACY_HELM_CHART_DIR=$(ROOT_DIR)/legacy/charts
HELM_CHART_LINT_ARGS_oam-kubernetes-runtime = --set serviceAccount.name=''
HELM_CHART_LINT_ARGS_oam-kubernetes-runtime-legacy = --set serviceAccount.name='' --set image.tag='master'

-include build/makelib/helm.mk

# ====================================================================================
# Setup Go

# Set a sane default so that the nprocs calculation below is less noisy on the initial
# loading of this file
NPROCS ?= 1

# each of our test suites starts a kube-apiserver and running many test suites in
# parallel can lead to high CPU utilization. by default we reduce the parallelism
# to half the number of CPU cores.
GO_TEST_PARALLEL := $(shell echo $$(( $(NPROCS) / 2 )))

GO_INTEGRATION_TESTS_SUBDIRS = test

GO_STATIC_PACKAGES = $(GO_PROJECT)/cmd/oam-kubernetes-runtime
GO_LDFLAGS += -X $(GO_PROJECT)/pkg/version.Version=$(VERSION)
GO_SUBDIRS += cmd pkg apis
GO111MODULE = on
-include build/makelib/golang.mk

# ====================================================================================
# Setup Images
# Due to the way that the shared build logic works, images should
# all be in folders at the same level (no additional levels of nesting).

DOCKER_REGISTRY = crossplane
IMAGE_DIR=$(ROOT_DIR)/images
IMAGES = oam-kubernetes-runtime
-include build/makelib/image.mk

# ====================================================================================
# Targets

# run `make help` to see the targets and options

# We want submodules to be set up the first time `make` is run.
# We manage the build/ folder and its Makefiles as a submodule.
# The first time `make` is run, the includes of build/*.mk files will
# all fail, and this target will be run. The next time, the default as defined
# by the includes will be run instead.
fallthrough: submodules
	@echo Initial setup complete. Running make again . . .
	@make

# Generate a coverage report for cobertura applying exclusions on
# - generated file
cobertura:
	@cat $(GO_TEST_OUTPUT)/coverage.txt | \
		grep -v zz_generated.deepcopy | \
		$(GOCOVER_COBERTURA) > $(GO_TEST_OUTPUT)/cobertura-coverage.xml

# Ensure a PR is ready for review.
reviewable: generate lint
	@go mod tidy

# Ensure branch is clean.
check-diff: reviewable
	@$(INFO) checking that branch is clean
	@git diff --quiet || $(FAIL)
	@$(OK) branch is clean

# Update the submodules, such as the common build scripts.
submodules:
	@git submodule sync
	@git submodule update --init --recursive

go-integration:
	GO_TEST_FLAGS="-timeout 1h -count=1 -v" GO_TAGS=integration $(MAKE) go.test.integration

.PHONY: cobertura reviewable submodules fallthrough

# ====================================================================================
# Special Targets

define OAM_KUBERNETES_RUNTIME_HELP
OAM Kubernetes Runtime Targets:
    cobertura          Generate a coverage report for cobertura applying exclusions on generated files.
    reviewable         Ensure a PR is ready for review.
    submodules         Update the submodules, such as the common build scripts.
    run                Run oam-k8s-runtime as a local process. Useful for development.
    install-crds       Install crds into clusters for oam-k8s-runtime. Useful for development.
    uninstall-crds     Uninstall crds from clusters for oam-k8s-runtime. Useful for development.
endef
export OAM_KUBERNETES_RUNTIME_HELP

oam-kubernetes-runtime.help:
	@echo "$$OAM_KUBERNETES_RUNTIME_HELP"

help-special: oam-kubernetes-runtime.help

.PHONY: oam-kubernetes-runtime.help help-special kind-load e2e e2e-setup e2e-test e2e-cleanup run install-crds uninstall-crds

# Install CRDs into a cluster. This is for convenience.
install-crds: reviewable
	kubectl apply -f charts/oam-kubernetes-runtime/crds/

# Uninstall CRDs from a cluster. This is for convenience.
uninstall-crds:
	kubectl delete -f charts/oam-kubernetes-runtime/crds/

# This is for running as a local process for convenience.
run: go.build
	@$(INFO) Running OAM Kubernetes Runtime as a local process . . .
	@# To see other arguments that can be provided, run the command with --help instead
	$(GO_OUT_DIR)/$(PROJECT_NAME)

# load docker image to the kind cluster
kind-load:
	docker tag $(BUILD_REGISTRY)/oam-kubernetes-runtime-$(ARCH) crossplane/oam-kubernetes-runtime:$(VERSION)
	kind load docker-image crossplane/oam-kubernetes-runtime:$(VERSION) || { echo >&2 "kind not installed or error loading image: $(IMAGE)"; exit 1; }

e2e-setup: kind-load
	kubectl create namespace oam-system
	helm install e2e ./charts/oam-kubernetes-runtime -n oam-system --set image.pullPolicy='Never' --wait \
		|| { echo >&2 "helm install timeout"; \
		kubectl logs `kubectl get pods -n oam-system -l "app.kubernetes.io/name=oam-kubernetes-runtime,app.kubernetes.io/instance=e2e" -o jsonpath="{.items[0].metadata.name}"` -c e2e; \
		helm uninstall e2e -n oam-system; exit 1;}
	kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=oam-kubernetes-runtime -n oam-system --timeout=300s

e2e-test:
	ginkgo -v ./test/e2e-test

e2e-cleanup:
	helm uninstall e2e -n oam-system
	kubectl delete namespace oam-system --wait

e2e: e2e-setup e2e-test go-integration

prepare-legacy-chart:
	rsync -r $(LEGACY_HELM_CHART_DIR)/$(LEGACY_HELM_CHART) $(HELM_CHARTS_DIR)
	rsync -r $(HELM_CHARTS_DIR)/$(HELM_CHART)/* $(HELM_CHARTS_DIR)/$(LEGACY_HELM_CHART) --exclude=Chart.yaml --exclude=crds
