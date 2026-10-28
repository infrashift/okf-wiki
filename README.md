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

1. **Install the CLI** from a [release](https://github.com/infrashift/okf-wiki/releases) (see [Installation](#installation)), or from source:
```bash
git clone https://github.com/infrashift/okf-wiki.git && cd okf-wiki
make install  # Builds bin/okfw and moves it to $GOPATH/bin
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
> **The Default Workspace:** The container launches a `tmux` session partitioned into three panes: **LazyVim**, a standard **Terminal**, and **Claude Code**.
> To ease the transition from IDEs like VSCode, mouse support is enabled by default in LazyVim. While this helps you get moving immediately, the true power of this stack is unlocked through **Vim motions**—it's worth the investment to learn them!

---

## Installation

### Download a release

Each [release](https://github.com/infrashift/okf-wiki/releases) carries signed,
static Linux binaries for x86_64 (`amd64`) and aarch64 (`arm64`):

```bash
VERSION=v2026.10.1
ARCH=amd64   # or arm64
curl -fsSLO https://github.com/infrashift/okf-wiki/releases/download/$VERSION/okfw-$VERSION-linux-$ARCH.tar.gz
tar -xzf okfw-$VERSION-linux-$ARCH.tar.gz
install okfw-$VERSION-linux-$ARCH/okfw ~/.local/bin/
okfw --version
```

To check a download against the project's release key, signatures and
checksums before installing it, follow
[Verifying Releases](https://infrashift.github.io/okf-wiki/reference/verifying-releases/).
The release key's public half is committed as [`release/cosign.pub`](release/cosign.pub).

### Build from source

Requires Go 1.24+ (`make test` also needs the [CUE](https://cuelang.org/) CLI).

```bash
git clone https://github.com/infrashift/okf-wiki.git
cd okf-wiki
make build    # Writes bin/okfw
make install  # Moves bin/okfw to $GOPATH/bin
```

okfw is pure Go, so it cross-compiles from any host:

```bash
make build-linux-amd64   # Writes bin/okfw-linux-amd64 (x86_64)
make build-linux-arm64   # Writes bin/okfw-linux-arm64 (aarch64)
make dist                # Both, as tarballs + SPDX SBOMs + SHA256SUMS in dist/ (needs syft)
```

`okfw --version` prints the version `make` stamped in: the git tag on HEAD,
`<tag>-<n>-g<commit>` past one, or the short commit. Releases are CalVer tags,
`vYYYY.M.N` with no leading zeros. Cutting and signing one (`make sign`) is
covered in [Releasing](https://infrashift.github.io/okf-wiki/reference/releasing/).

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
  compact JSON responses (about 5 KB of tool schemas). On connect it sends the
  wiki's conventions and its top-level directories as MCP `instructions`, so
  any client gets them. `okfw_read` opens a directory's `index.md` listing as
  well as a concept (`{"ref":"/"}` for the root), and `okfw_create` sets kind,
  type, lifecycle, staleness, resource and author in one write. Register with
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
│   │   └── gitctx/      # Git root lookup (where okfw init places .okfw/)
│   └── testdata/        # Test fixtures
├── lua/okfw/            # NeoVim plugin
├── plugin/              # NeoVim autoload
├── doc/                 # NeoVim help
├── after/               # NeoVim ftplugin
├── test/                # NeoVim plugin integration suite
├── docs/                # Documentation site (Astro Starlight)
├── Containerfiles/      # Devcontainer and e2e images
├── release/             # cosign offline signing config
├── bin/                 # make build output (gitignored)
├── dist/                # make dist / make sign output (gitignored)
├── go.mod, go.sum       # Go module (stays at root)
└── Makefile
```

---

## License

Apache 2.0
