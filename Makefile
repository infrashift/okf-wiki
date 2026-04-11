.PHONY: build test lint fmt clean install tidy ui-check integration-test docs-dev image run-dev stop-dev

BINARY_NAME=zk
CMD_PATH=./cli/cmd/zk

# Container settings
CONTAINER_ENGINE=podman
CONTAINER_FILE=Containerfiles/Containerfile
IMAGE_NAME=zk-devcontainer
IMAGE_TAG=latest
CONTAINER_NAME=zk-dev
SSH_PORT=2222
SSH_PUBKEY=$(HOME)/.ssh/id_ed25519.pub
ZK_VAULT=$(HOME)/zk_vault
USER_UID=$(shell id -u)

build:
	go build -o $(BINARY_NAME) $(CMD_PATH)

test:
	@echo "Running Go tests..."
	go test -v -cover ./cli/...
	@echo "Validating CUE schemas..."
	cue vet ./cli/internal/config/config-schema.cue
	cue vet ./cli/internal/config/zettel-schema.cue

lint:
	@echo "Running linters..."
	go vet ./...
	@command -v staticcheck >/dev/null && staticcheck ./... || true

fmt:
	go fmt ./...
	@command -v cue >/dev/null && cue fmt ./cli/internal/config/*.cue || true

install: build
	mv $(BINARY_NAME) $(GOPATH)/bin/$(BINARY_NAME)

ui-check:
	@echo "Checking Lua syntax..."
	luacheck lua/

integration-test: build
	@echo "Running NeoVim plugin integration tests..."
	@test -d /tmp/nvim-test-plugins/plenary.nvim || git clone --depth 1 https://github.com/nvim-lua/plenary.nvim /tmp/nvim-test-plugins/plenary.nvim
	nvim --headless -u NONE -c "luafile test/integration_test.lua"

clean:
	rm -f $(BINARY_NAME)
	rm -rf .zk_index/

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
		-v $(ZK_VAULT):/home/user/zk_vault:Z \
		$(IMAGE_NAME):$(IMAGE_TAG)

stop-container:
	$(CONTAINER_ENGINE) stop $(CONTAINER_NAME) && $(CONTAINER_ENGINE) rm $(CONTAINER_NAME)
