all: images
.PHONY: all

# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/build-machinery-go/make/, \
	golang.mk \
	targets/openshift/images.mk \
)

IMAGE_REGISTRY :=registry.ci.openshift.org

# Check which builder to use - default is imagebuilder
ifeq ($(BUILDER), podman)
# Use podman to build the must gather image
# Please ensure you have podman installed on your machine for this
IMAGE_BUILD_BUILDER := podman build
# Clear all build default flags
IMAGE_BUILD_DEFAULT_FLAGS :=
# Set authfile if the user passes another location for the authentication file
ifneq ($(strip $(AUTH_FILE)),)
IMAGE_BUILD_DEFAULT_FLAGS := --authfile=$(AUTH_FILE)
endif
endif


# This will call a macro called "build-image" which will generate image specific targets based on the parameters:
# $0 - macro name
# $1 - target name
# $2 - image ref
# $3 - Dockerfile path
# $4 - context directory for image build
$(call build-image,ocp-must-gather,$(IMAGE_REGISTRY)/ocp/4.17:ocp-must-gather, ./Dockerfile.ocp,.)

$(call verify-golang-versions,Dockerfile.ocp)

.PHONY: lint
lint: shellcheck
	$(SHELLCHECK) -x ./**/*.sh

.PHONY: fmt
fmt: shfmt
	$(SHFMT) -l -w ./**/*.sh

.PHONY: test
test: bats
	$(BATS) tests/*.bats

## Location where binaries are installed
LOCALBIN ?= $(shell pwd)/tmp/bin

## Tool Binaries
BATS ?= $(LOCALBIN)/bats
SHFMT ?= $(LOCALBIN)/shfmt
SHELLCHECK ?= $(LOCALBIN)/shellcheck

# NOTE: please keep this list sorted so that it can be easily searched
TOOLS = bats \
        shfmt \
        shellcheck \

.PHONY: tools
tools:
	./hack/tools.sh

$(TOOLS):
	./hack/tools.sh $@
