all: images
.PHONY: all

# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/build-machinery-go/make/, \
	golang.mk \
	targets/openshift/images.mk \
)

IMAGE_REGISTRY :=registry.ci.openshift.org

# This will call a macro called "build-image" which will generate image specific targets based on the parameters:
# $0 - macro name
# $1 - target name
# $2 - image ref
# $3 - Dockerfile path
# $4 - context directory for image build
$(call build-image,ocp-must-gather,$(IMAGE_REGISTRY)/ocp/4.17:ocp-must-gather, ./Dockerfile.ocp,.)

$(call verify-golang-versions,Dockerfile.ocp)

# Allow users to pass in an authentication file when building with podman
# An absolute path needs to be passed in here
# This file must container credentials for registry.ci.openshift.org
AUTH_FILE ?=

# Target for building the image using Podman
.PHONY: podman-image
podman-image:
	podman build \
		$(if $(AUTH_FILE),--authfile=$(AUTH_FILE)) \
		-t $(IMAGE_REGISTRY)/ocp/must-gather \
		-f ./Dockerfile.ocp .

# Ensure the podman-image target depends on verifying Golang versions
image-with-podman: verify-golang-versions podman-image
