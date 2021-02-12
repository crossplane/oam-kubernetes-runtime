# ====================================================================================
# Setup Project

PLATFORMS := linux_amd64 linux_arm64
include ../../build/makelib/common.mk

ifeq ($(ARCH),ppc64le)
       BUILD_ARGS += -f $(IMAGE_TEMP_DIR)/Dockerfile.ppc64le
endif

# ====================================================================================
#  Options
IMAGE = $(BUILD_REGISTRY)/oam-kubernetes-runtime-$(ARCH)
include ../../build/makelib/image.mk

# ====================================================================================
# Targets

img.build:
	@$(INFO) docker build $(IMAGE)
	@cp -r ../.. $(IMAGE_TEMP_DIR) || $(FAIL)
	@cp $(OUTPUT_DIR)/bin/$(OS)_$(ARCH)/oam-kubernetes-runtime $(IMAGE_TEMP_DIR) || $(FAIL)
	@cd $(IMAGE_TEMP_DIR) || $(FAIL)
	@docker build $(BUILD_ARGS) \
		-t $(IMAGE) \
		$(IMAGE_TEMP_DIR) || $(FAIL)
	@$(OK) docker build $(IMAGE)
