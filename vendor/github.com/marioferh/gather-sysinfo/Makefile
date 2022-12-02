TARGET_GOOS=linux
TARGET_GOARCH=amd64
TOOLS_BIN_DIR="build/_output/bin"

.PHONY: all
all: dist-gather-sysinfo dist

.PHONY: clean
clean: dist-clean

.PHONY: dist-clean
dist-clean:
	rm -rf build/_output/bin

.PHONY: dist
dist: build-output-dir
	@echo "Building operator binary"
	mkdir -p $(TOOLS_BIN_DIR); \
    LDFLAGS="-s -w "; \
    LDFLAGS+="-X github.com/openshift-kni/performance-addon-operators/version.Version=$(VERSION) "; \
    LDFLAGS+="-X github.com/openshift-kni/performance-addon-operators/version.GitCommit=$(COMMIT) "; \
    LDFLAGS+="-X github.com/openshift-kni/performance-addon-operators/version.BuildDate=$(BUILD_DATE) "; \
	env GOOS=$(TARGET_GOOS) GOARCH=$(TARGET_GOARCH) go build -ldflags="$$LDFLAGS" \
	  -mod=vendor -o $(TOOLS_BIN_DIR)/performance-addon-operators .

.PHONY: build-output-dir
build-output-dir:
	mkdir -p $(TOOLS_BIN_DIR) || :

.PHONY: dist-gather-sysinfo
dist-gather-sysinfo: build-output-dir
	@if [ ! -x $(TOOLS_BIN_DIR)/gather-sysinfo ]; then\
		echo "Building gather-sysinfo helper";\
		env CGO_ENABLED=0 GOOS=$(TARGET_GOOS) GOARCH=$(TARGET_GOARCH) go build -ldflags="-s -w" -mod=vendor -o $(TOOLS_BIN_DIR)/gather-sysinfo ;\
	else \
		echo "Using pre-built gather-sysinfo helper";\
	fi