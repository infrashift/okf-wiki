# okfw - OKF Wiki

A fast, opinionated command-line tool for managing a local-first personal knowledge base. Built with Go, validated with CUE schemas, and integrated with NeoVim. Concepts are stored as plain markdown in a per-project `.okfw/` wiki that conforms to the Open Knowledge Format (OKF).

---

## Features

- **CUE-validated schemas** - Frontmatter is validated against strict CUE schemas
- **Full-text search** - Scans and ranks the concepts; no index to build, so results always match the files
- **Graph visualization** - Mermaid diagram (or an ASCII tree with `--ascii`) of concept relationships
- **Backlinks discovery** - Find all concepts that link to any concept
- **Note templates** - Built-in templates for meetings, user stories, features, and more
- **Daily notes** - Idempotent daily capture with review workflows
- **OKF metadata** - Lifecycle status, staleness, verification trail, and provenance sources
- **NeoVim integration** - Lua plugin for seamless note creation from your editor
- **AI-agent ready** - `--json` output, self-documenting help, a token-lean MCP server (`okfw mcp`), and an agent skill installed with `okfw install`
- **Hierarchical notes** - Support for parent-child relationships between concepts
- **Universal linking** - Link any concept to any other concept (notes, daily notes, issues)

---

## Getting Started

Follow these steps to transition from theory to a fully functional, containerized OKF Wiki environment.

### 1. Research & Methodology

Before diving into the code, understand the "why" behind the workflow:

* [Why OKF Wiki?](https://infrashift.github.io/okf-wiki/methodology/why-okf-wiki/) – Core philosophy and benefits.
* [The Tutorial](https://infrashift.github.io/okf-wiki/tutorial/) – A deep dive into the specific implementation used here.

### 2. Quick Start

1. **Install the CLI:**
```bash
git clone https://github.com/infrashift/okf-wiki.git && cd okf-wiki
make install  # Installs okfw to $GOPATH/bin
```

2. **Initialize a wiki in your project:**
```bash
cd your-project
okfw init     # Creates .okfw/ at the project root (no-op if it exists)
```

3. **Create and search notes:**
```bash
okfw create "My first idea"
okfw search "idea"
```

Every command discovers the `.okfw/` wiki automatically from any subdirectory
of your project. Prefer a ready-made environment? Build and run the OCI
container — it comes pre-configured with `tmux`, `LazyVim`, the OKF Wiki CLI,
and the `okfw` Neovim plugin — and follow the interactive workflows in the
tutorial.

---

> [!TIP]
> **The Default Workspace:** > The container launches a `tmux` session partitioned into three panes: **LazyVim**, a standard **Terminal**, and **Claude Code**.
> To ease the transition from IDEs like VSCode, mouse support is enabled by default in LazyVim. While this helps you get moving immediately, the true power of this stack is unlocked through **Vim motions**—it's worth the investment to learn them!

---

## Installation

### Prerequisites

- Go 1.24+
- CUE (for schema validation)

### Build from source

```bash
git clone https://github.com/infrashift/okf-wiki.git
cd okf-wiki
make build
make install  # Installs to $GOPATH/bin
```

### Build for a specific Linux architecture

okfw is pure Go, so it cross-compiles from any host. `make build` targets the
machine you run it on; to build for Linux x86_64 or aarch64 from any machine:

```bash
make build-linux-amd64   # Writes bin/okfw-linux-amd64 (x86_64)
make build-linux-arm64   # Writes bin/okfw-linux-arm64 (aarch64)
make dist                # Both, packaged for distribution in dist/ (needs syft)
```

`make dist` writes, for each architecture, `okfw-<version>-linux-<arch>.tar.gz`
(the binary, LICENSE and README) and an SPDX SBOM of the binary,
`okfw-<version>-linux-<arch>.spdx.json`, plus `SHA256SUMS` over the tarballs and
`SHA256SUMS.sbom` over the SBOMs. `<version>` is the git tag on HEAD, or the
short commit when there is none; override it with `make dist VERSION=v2026.10.0`.

### Versions

`okfw --version` prints the version `make` stamped into the binary: the git tag
on HEAD, `<tag>-<n>-g<commit>` for a build past one, or the short commit when
the repository has no tags. A binary built with plain `go build` reports the
pseudo-version Go recorded from git instead.

Releases are tagged in CalVer, `vYYYY.M.N`: the year, the month, and a counter
within the month, with no leading zeros (`v2026.1.0`, not `v2026.01.0`), which
also keeps them valid semver. To cut one:

```bash
git tag v2026.10.0
make sign                # or make dist, unsigned
```

### Sign a distribution (optional)

`make sign` runs `make dist` and signs every file it wrote with
[cosign](https://docs.sigstore.dev/cosign/system_config/installation/), writing a
`<file>.sigstore.json` bundle beside each one and the public key as
`dist/cosign.pub`, then verifies them. Signing is offline: nothing is uploaded to
the public Rekor transparency log.

```bash
make signing-key         # Optional: makes ~/.config/cosign/okfw.{key,pub} (ECDSA P-256)
make sign                # Asks for the key's password once (or set COSIGN_PASSWORD)
make sign COSIGN_KEY=path/to/cosign.key COSIGN_PUB=path/to/cosign.pub   # Your own key
```

To verify a download against the published key:

```bash
cosign verify-blob --key cosign.pub --insecure-ignore-tlog=true \
  --bundle okfw-<version>-linux-amd64.tar.gz.sigstore.json okfw-<version>-linux-amd64.tar.gz
```

`--insecure-ignore-tlog` is needed because the signature was deliberately never
logged; trust rests on obtaining `cosign.pub` from a source you trust.

## Zero Configuration

There is no config file. A wiki is a `.okfw/` directory at a project's root,
created once with `okfw init`. Every command discovers it by searching upward
from the current working directory (like git finds `.git`), so `okfw` works
from any subdirectory. `okfw connect` verifies the wiki; `okfw root` prints
its path.

The layout inside `.okfw/` is fixed:

| Path | Description |
|------|-------------|
| `*.md` (top level) | Concept markdown (subdirectories via `--path` or `--project`) |
| `daily/YYYY/MM/` | Daily notes |
| `ephemeral/` | Export scratch dir |
| `.gitignore` | Ignores `ephemeral/` |

Concept markdown is meant to be committed with the project; generated
artifacts are gitignored.

---

## AI Agents

Two complementary integration points:

- **MCP server** — `okfw mcp` serves five consolidated tools (`okfw_search`,
  `okfw_create`, `okfw_read`, `okfw_update`, `okfw_graph`) over stdio with
  compact JSON responses (about 3.9 KB of tool schemas). Register with
  `claude mcp add okfw -- okfw mcp`, launched from the project directory. The
  unprefixed names still dispatch, for configurations that predate the prefix.
- **Agent skill** — `okfw install --platform claude|gemini|codex` writes the
  skill and a managed CLAUDE.md/GEMINI.md/AGENTS.md block teaching agents the
  wiki's conventions (linking syntax, atomic notes, capture → refine → link).
  Re-running it is idempotent, and the block is namespaced so it leaves the
  rest of the file alone. The skill covers the *why and when*; the MCP tools and
  `okfw --help` cover the mechanics.

---

## Development

```bash
# Run tests
make test

# Run linter
make lint

# Format code
make fmt

# Run NeoVim plugin integration tests
make integration-test

# Check Lua syntax
make ui-check
```

---

## Project Structure

```
okf-wiki/
├── cli/
│   ├── cmd/okfw/        # CLI entry point (all commands, MCP server)
│   ├── internal/
│   │   ├── config/      # .okfw discovery and CUE schema validation
│   │   ├── bundle/      # Write path: concept creation, frontmatter, index.md, link graph
│   │   ├── graph/       # In-memory graph + rendering (Mermaid, ASCII tree)
│   │   ├── search/      # Scan-and-rank concept search (no index)
│   │   ├── agentskill/  # Agent-skill installer (okfw install)
│   │   ├── okfwskills/  # okfw's SKILL.md / CLAUDE.md-block templates
│   │   ├── templates/   # Embedded note templates
│   │   └── gitctx/      # Git project-context helper
│   └── testdata/        # Test fixtures
├── lua/okfw/            # NeoVim plugin
├── plugin/              # NeoVim autoload
├── doc/                 # NeoVim help
├── after/               # NeoVim ftplugin
├── go.mod, go.sum       # Go module (stays at root)
└── Makefile
```

---

## License

Apache 2.0
