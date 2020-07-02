# ====================================================================================
# Setup Project
IMG ?= controller:latest

PROJECT_NAME := oam-kubernetes-runtime
PROJECT_REPO := github.com/crossplane/$(PROJECT_NAME)

PLATFORMS ?= linux_amd64 linux_arm64
# -include will silently skip missing files, which allows us
# to load those files with a target in the Makefile. If only
# "include" was used, the make command would fail and refuse
# to run a target until the include commands succeeded.
-include build/makelib/common.mk

# ====================================================================================
# Setup Images

# even though this repo doesn't build images (note the no-op img.build target below),
# some of the init is needed for the cross build container, e.g. setting BUILD_REGISTRY
-include build/makelib/image.mk
img.build:

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

GO_LDFLAGS += -X $(GO_PROJECT)/pkg/version.Version=$(VERSION)
GO_SUBDIRS += pkg apis
GO111MODULE = on
-include build/makelib/golang.mk

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

define CROSSPLANE_RUNTIME_HELP
Crossplane Runtime Targets:
    cobertura          Generate a coverage report for cobertura applying exclusions on generated files.
    reviewable         Ensure a PR is ready for review.
    submodules         Update the submodules, such as the common build scripts.

endef
export CROSSPLANE_RUNTIME_HELP

oam-runtime.help:
	@echo "$$CROSSPLANE_RUNTIME_HELP"

help-special: oam-runtime.help

.PHONY: oam-runtime.help help-special docker-build docker-push kind-load e2e-setup e2e-test e2e-cleanup

# Build the docker image
docker-build:
	docker build . -t $(IMG)

# Push the docker image
docker-push:
	docker push ${IMG}

# load docker image to the kind cluster
kind-load:
	kind load docker-image $(IMG) || { echo >&2 "kind not installed or error loading image: $(IMG)"; exit 1; }

e2e-setup: docker-build kind-load
	kubectl create namespace oam-system
	helm install e2e ./charts/oam-core-runtime/ -n oam-system --set image.repository=$(IMG) --wait \
		|| { echo >&2 "helm install timeout"; \
		kubectl logs `kubectl get pods -n oam-system -l "app.kubernetes.io/name=oam-core-runtime,app.kubernetes.io/instance=e2e" -o jsonpath="{.items[0].metadata.name}"` -c e2e; \
		helm uninstall e2e -n oam-system; exit 1;}
	kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=oam-core-runtime -n oam-system --timeout=300s

e2e-test:
	ginkgo -v ./test/e2e-test

e2e-cleanup:
	helm uninstall e2e -n oam-system
	kubectl delete namespace oam-system --wait