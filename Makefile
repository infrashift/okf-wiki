.PHONY: build build-graphy build-all test test-race graphy-e2e lint fmt clean install install-graphy tidy ui-check integration-test e2e docs-dev build-image run-container stop-container

BINARY_NAME=okfw
CMD_PATH=./cli/cmd/okfw

# graphy — code knowledge-graph sibling binary (tree-sitter requires CGo)
GRAPHY_BINARY=graphy
GRAPHY_CMD_PATH=./cli/cmd/graphy

# Container settings
CONTAINER_ENGINE=podman
CONTAINER_FILE=Containerfiles/Containerfile
IMAGE_NAME=okfw-devcontainer
IMAGE_TAG=latest
CONTAINER_NAME=okfw-dev
SSH_PORT=2222
SSH_PUBKEY=$(HOME)/.ssh/id_ed25519.pub
OKFW_BUNDLE=$(HOME)/okfw
USER_UID=$(shell id -u)

build:
	CGO_ENABLED=0 go build -o $(BINARY_NAME) $(CMD_PATH)

build-graphy:
	CGO_ENABLED=1 go build -o $(GRAPHY_BINARY) $(GRAPHY_CMD_PATH)

build-all: build build-graphy

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
	mv $(BINARY_NAME) $(GOPATH)/bin/$(BINARY_NAME)

install-graphy: build-graphy
	mv $(GRAPHY_BINARY) $(GOPATH)/bin/$(GRAPHY_BINARY)

ui-check:
	@echo "Checking Lua syntax..."
	luacheck lua/ plugin/

integration-test: build
	@echo "Running NeoVim plugin integration tests..."
	@test -d /tmp/nvim-test-plugins/plenary.nvim || git clone --depth 1 https://github.com/nvim-lua/plenary.nvim /tmp/nvim-test-plugins/plenary.nvim
	nvim --headless -u NONE -c "luafile test/integration_test.lua"

# The e2e image runs its tests as build steps, so a failed assertion fails the
# build. It covers what `go test` cannot: graphy is a CGo binary built in one
# distro and run in another, and `graphy export okfw` has to produce concepts the
# okfw binary accepts at runtime.
e2e:
	@echo "Building containerized e2e image (runs the tests inside the container)..."
	$(CONTAINER_ENGINE) build -f Containerfiles/Containerfile.e2e -t okfw-e2e:latest .
	@echo "e2e build succeeded — okfw CLI, graphy, and the NeoVim plugin all pass in-container."

# Run the graphy end-to-end script against locally built binaries, without
# waiting on a container build.
graphy-e2e: build-all
	PATH="$(CURDIR):$$PATH" bash Containerfiles/config/graphy-e2e.sh

clean:
	rm -f $(BINARY_NAME) $(GRAPHY_BINARY)
	
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
		-v $(OKFW_BUNDLE):/home/user/okfw:Z \
		$(IMAGE_NAME):$(IMAGE_TAG)

stop-container:
	$(CONTAINER_ENGINE) stop $(CONTAINER_NAME) && $(CONTAINER_ENGINE) rm $(CONTAINER_NAME)
