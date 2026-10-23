.PHONY: build build-linux-amd64 build-linux-arm64 dist test test-race lint fmt clean install tidy ui-check integration-test e2e docs-dev build-image run-container stop-container

BINARY_NAME=okfw
CMD_PATH=./cli/cmd/okfw
BIN_DIR=bin
DIST_DIR=dist
LINUX_ARCHES=amd64 arm64
# Names the dist tarballs: the tag when HEAD has one, else the commit.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
SHA256SUM=$(shell command -v sha256sum >/dev/null 2>&1 && echo sha256sum || echo shasum -a 256)

# Container settings
CONTAINER_ENGINE=podman
CONTAINER_FILE=Containerfiles/Containerfile
IMAGE_NAME=okfw-devcontainer
IMAGE_TAG=latest
CONTAINER_NAME=okfw-dev
SSH_PORT=2222
SSH_PUBKEY=$(HOME)/.ssh/id_ed25519.pub
USER_UID=$(shell id -u)

build:
	CGO_ENABLED=0 go build -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_PATH)

# okfw is pure Go, so it cross-compiles from any host: each of these builds a
# Linux binary for its architecture (x86_64, aarch64) whatever machine runs it.
# -trimpath keeps the build machine's paths out of a binary meant to be shipped.
build-linux-amd64 build-linux-arm64: build-linux-%:
	CGO_ENABLED=0 GOOS=linux GOARCH=$* go build -trimpath -o $(BIN_DIR)/$(BINARY_NAME)-linux-$* $(CMD_PATH)

# One tarball per Linux architecture, each unpacking to a directory holding
# okfw, LICENSE and README.md, plus SHA256SUMS over the tarballs.
dist: $(addprefix build-linux-,$(LINUX_ARCHES))
	rm -rf $(DIST_DIR) && mkdir -p $(DIST_DIR)
	@for arch in $(LINUX_ARCHES); do \
		name=$(BINARY_NAME)-$(VERSION)-linux-$$arch; \
		mkdir -p $(DIST_DIR)/$$name && \
		cp $(BIN_DIR)/$(BINARY_NAME)-linux-$$arch $(DIST_DIR)/$$name/$(BINARY_NAME) && \
		cp LICENSE README.md $(DIST_DIR)/$$name/ && \
		tar -C $(DIST_DIR) -czf $(DIST_DIR)/$$name.tar.gz $$name && \
		rm -rf $(DIST_DIR)/$$name && \
		echo "$(DIST_DIR)/$$name.tar.gz" || exit 1; \
	done
	cd $(DIST_DIR) && $(SHA256SUM) *.tar.gz > SHA256SUMS

# -count=1 disables the test cache: a cached pass can otherwise hide a
# regression in code the cache key did not notice.
test:
	@echo "Running Go tests..."
	go test -count=1 -cover ./cli/...
	@echo "Validating CUE schema syntax..."
	@echo "(concept conformance is checked by cli/internal/config/fixtures_test.go)"
	cue vet ./cli/internal/config/concept-schema.cue

test-race:
	go test -count=1 -race ./cli/...

lint:
	@echo "Running linters..."
	go vet ./...
	@if command -v staticcheck >/dev/null; then \
		staticcheck ./...; \
	else \
		echo "staticcheck not installed: go install honnef.co/go/tools/cmd/staticcheck@latest"; \
		exit 1; \
	fi
	gofmt -l ./cli
	go mod tidy -diff

fmt:
	go fmt ./...
	@command -v cue >/dev/null && cue fmt ./cli/internal/config/concept-schema.cue || true

install: build
	mv $(BIN_DIR)/$(BINARY_NAME) $(GOPATH)/bin/$(BINARY_NAME)

ui-check:
	@echo "Checking Lua syntax..."
	luacheck lua/ plugin/ test/

# Re-clone when plenary.job is missing, not only when the directory is: an
# emptied or half-cloned checkout passed `test -d` and failed the suite.
PLENARY_DIR := /tmp/nvim-test-plugins/plenary.nvim
integration-test: build
	@echo "Running NeoVim plugin integration tests..."
	@test -f $(PLENARY_DIR)/lua/plenary/job.lua || { rm -rf $(PLENARY_DIR) && git clone --depth 1 https://github.com/nvim-lua/plenary.nvim $(PLENARY_DIR); }
	nvim --headless -u NONE -l test/integration_test.lua

# The e2e image runs its tests as build steps, so a failed assertion fails the
# build. It runs the NeoVim plugin suite against the okfw binary in a clean
# container.
e2e:
	@echo "Building containerized e2e image (runs the tests inside the container)..."
	$(CONTAINER_ENGINE) build -f Containerfiles/Containerfile.e2e -t okfw-e2e:latest .
	@echo "e2e build succeeded — okfw CLI and the NeoVim plugin pass in-container."

clean:
	rm -rf $(BIN_DIR) $(DIST_DIR)
	
tidy:
	go mod tidy

docs-dev:
	cd docs && bun --bun run dev

build-image:
	$(CONTAINER_ENGINE) build --build-arg USER_UID=$(USER_UID) -f $(CONTAINER_FILE) -t $(IMAGE_NAME):$(IMAGE_TAG) .

run-container:
	$(CONTAINER_ENGINE) run -d --name $(CONTAINER_NAME) \
		--userns=keep-id \
		-p $(SSH_PORT):2222 \
		-v $(SSH_PUBKEY):/home/user/.ssh/authorized_keys:ro,Z \
		$(IMAGE_NAME):$(IMAGE_TAG)

stop-container:
	$(CONTAINER_ENGINE) stop $(CONTAINER_NAME) && $(CONTAINER_ENGINE) rm $(CONTAINER_NAME)
