.PHONY: build build-linux-amd64 build-linux-arm64 dist signing-key sign verify-signatures test test-race lint fmt clean install tidy ui-check integration-test e2e docs-dev build-image run-container stop-container

BINARY_NAME=okfw
CMD_PATH=./cli/cmd/okfw
BIN_DIR=bin
DIST_DIR=dist
LINUX_ARCHES=amd64 arm64
# Stamped into the binary (`okfw --version`) and naming the dist tarballs: the
# tag when HEAD has one, else the commit. Releases are CalVer tags, vYYYY.M.N
# with no leading zeros (v2026.1.0, not v2026.01.0), so they are also semver.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS=-X okfw/cli/internal/config.version=$(VERSION)
SHA256SUM=$(shell command -v sha256sum >/dev/null 2>&1 && echo sha256sum || echo shasum -a 256)

# Signing (optional: `make dist` never signs). The default key lives outside
# the repository so no build context or `git add -f` can pick it up. Supply
# your own with COSIGN_KEY= (a key file or a KMS URI) and, optionally,
# COSIGN_PUB= (a file; derived from the key when absent).
COSIGN_KEY_PREFIX ?= $(HOME)/.config/cosign/okfw
COSIGN_KEY ?= $(COSIGN_KEY_PREFIX).key
COSIGN_PUB ?= $(COSIGN_KEY_PREFIX).pub
# A signing config naming no transparency log or timestamp authority: signing
# stays offline and never publishes to the public Rekor log, which cosign v3
# does by default.
COSIGN_SIGNING_CONFIG=release/cosign-offline-signing-config.json

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
	CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_PATH)

# okfw is pure Go, so it cross-compiles from any host: each of these builds a
# Linux binary for its architecture (x86_64, aarch64) whatever machine runs it.
# -trimpath keeps the build machine's paths out of a binary meant to be shipped.
build-linux-amd64 build-linux-arm64: build-linux-%:
	CGO_ENABLED=0 GOOS=linux GOARCH=$* go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$(BINARY_NAME)-linux-$* $(CMD_PATH)

# One tarball per Linux architecture, each unpacking to a directory holding
# okfw, LICENSE and README.md, plus SHA256SUMS over the tarballs. Each tarball
# gets an SPDX SBOM made by syft from the binary it ships (Go records its
# modules in the binary, so this is what was linked, not what go.mod lists),
# with SHA256SUMS.sbom over the SBOMs.
dist: $(addprefix build-linux-,$(LINUX_ARCHES))
	@command -v syft >/dev/null || { echo "make dist needs syft for the SBOMs: https://github.com/anchore/syft#installation" >&2; exit 1; }
	rm -rf $(DIST_DIR) && mkdir -p $(DIST_DIR)
	@for arch in $(LINUX_ARCHES); do \
		name=$(BINARY_NAME)-$(VERSION)-linux-$$arch; \
		mkdir -p $(DIST_DIR)/$$name && \
		cp $(BIN_DIR)/$(BINARY_NAME)-linux-$$arch $(DIST_DIR)/$$name/$(BINARY_NAME) && \
		cp LICENSE README.md $(DIST_DIR)/$$name/ && \
		tar -C $(DIST_DIR) -czf $(DIST_DIR)/$$name.tar.gz $$name && \
		rm -rf $(DIST_DIR)/$$name && \
		SYFT_CHECK_FOR_APP_UPDATE=false syft scan -q file:$(BIN_DIR)/$(BINARY_NAME)-linux-$$arch \
			-o spdx-json=$(DIST_DIR)/$$name.spdx.json && \
		echo "$(DIST_DIR)/$$name.tar.gz $(DIST_DIR)/$$name.spdx.json" || exit 1; \
	done
	cd $(DIST_DIR) && $(SHA256SUM) *.tar.gz > SHA256SUMS
	cd $(DIST_DIR) && $(SHA256SUM) *.spdx.json > SHA256SUMS.sbom

# An ECDSA P-256 key pair: what `cosign generate-key-pair` makes, and cosign's
# default signing algorithm. (cosign imports an Ed25519 key but cannot sign a
# blob with one.) cosign asks for a password to encrypt the private key.
signing-key:
	@test ! -e $(COSIGN_KEY) || { echo "$(COSIGN_KEY) already exists; not overwriting it" >&2; exit 1; }
	@mkdir -p $(dir $(COSIGN_KEY_PREFIX)) && chmod 700 $(dir $(COSIGN_KEY_PREFIX))
	cosign generate-key-pair --output-key-prefix $(COSIGN_KEY_PREFIX)

# Signs everything `make dist` writes, each file getting a <file>.sigstore.json
# bundle, and copies the public key in as cosign.pub — COSIGN_PUB, or derived
# from the private key when that file does not exist. The key password is read
# once (or from COSIGN_PASSWORD) rather than once per file.
sign: dist
	@command -v cosign >/dev/null || { echo "make sign needs cosign: https://docs.sigstore.dev/cosign/system_config/installation/" >&2; exit 1; }
	@case "$(COSIGN_KEY)" in *://*) ;; *) test -f $(COSIGN_KEY) || { echo "no signing key at $(COSIGN_KEY): run 'make signing-key' or set COSIGN_KEY=" >&2; exit 1; } ;; esac
	@if [ -z "$${COSIGN_PASSWORD+set}" ]; then \
		printf 'Password for $(COSIGN_KEY): ' >&2; \
		stty -echo 2>/dev/null; read -r COSIGN_PASSWORD; stty echo 2>/dev/null; echo >&2; \
	fi; \
	export COSIGN_PASSWORD; \
	for f in $(DIST_DIR)/*.tar.gz $(DIST_DIR)/SHA256SUMS $(DIST_DIR)/*.spdx.json $(DIST_DIR)/SHA256SUMS.sbom; do \
		if out=$$(cosign sign-blob -y --key $(COSIGN_KEY) --signing-config $(COSIGN_SIGNING_CONFIG) \
			--bundle $$f.sigstore.json $$f 2>&1); \
		then echo "$$f.sigstore.json"; else echo "$$out" >&2; exit 1; fi; \
	done; \
	if [ -f "$(COSIGN_PUB)" ]; then cp $(COSIGN_PUB) $(DIST_DIR)/cosign.pub; \
	else cosign public-key --key $(COSIGN_KEY) --outfile $(DIST_DIR)/cosign.pub 2>/dev/null || exit 1; fi; \
	echo "$(DIST_DIR)/cosign.pub"
	@$(MAKE) --no-print-directory verify-signatures COSIGN_PUB=$(DIST_DIR)/cosign.pub

# Checks every bundle in dist/ against COSIGN_PUB. --insecure-ignore-tlog is
# needed because `make sign` deliberately writes no transparency-log entry.
verify-signatures:
	@ls $(DIST_DIR)/*.sigstore.json >/dev/null 2>&1 || { echo "no signatures in $(DIST_DIR)/: run 'make sign'" >&2; exit 1; }
	@for b in $(DIST_DIR)/*.sigstore.json; do \
		f=$${b%.sigstore.json}; \
		if out=$$(cosign verify-blob --key $(COSIGN_PUB) --bundle $$b --insecure-ignore-tlog=true $$f 2>&1); \
		then echo "verified $$f"; else echo "$$out" >&2; echo "FAILED $$f" >&2; exit 1; fi; \
	done

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
	$(CONTAINER_ENGINE) build --build-arg USER_UID=$(USER_UID) --build-arg VERSION=$(VERSION) -f $(CONTAINER_FILE) -t $(IMAGE_NAME):$(IMAGE_TAG) .

run-container:
	$(CONTAINER_ENGINE) run -d --name $(CONTAINER_NAME) \
		--userns=keep-id \
		-p $(SSH_PORT):2222 \
		-v $(SSH_PUBKEY):/home/user/.ssh/authorized_keys:ro,Z \
		$(IMAGE_NAME):$(IMAGE_TAG)

stop-container:
	$(CONTAINER_ENGINE) stop $(CONTAINER_NAME) && $(CONTAINER_ENGINE) rm $(CONTAINER_NAME)
