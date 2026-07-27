# okfw - OKF Wiki

A fast, opinionated command-line tool for managing a local-first personal knowledge base. Built with Go, validated with CUE schemas, and integrated with NeoVim. Concepts are stored as plain markdown in a per-project `.okfw/` wiki that conforms to the Open Knowledge Format (OKF).

---

## Features

- **CUE-validated schemas** - Frontmatter is validated against strict CUE schemas
- **Bleve full-text search** - Fast, local search index with structured field queries
- **Graph visualization** - ASCII tree showing note relationships
- **Backlinks discovery** - Find all concepts that link to any concept
- **Note templates** - Built-in templates for meetings, user stories, features, and more
- **Daily notes** - Idempotent daily capture with review workflows
- **OKF metadata** - Lifecycle status, staleness, verification trail, and provenance sources
- **NeoVim integration** - Lua plugin for seamless note creation from your editor
- **AI-agent ready** - `--json` output, self-documenting help, a token-lean MCP server (`okfw mcp`), and a drop-in agent skill ([`skills/okfw.md`](skills/okfw.md))
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

- Go 1.22+
- CUE (for schema validation)

### Build from source

```bash
git clone https://github.com/infrashift/okf-wiki.git
cd okf-wiki
make build
make install  # Installs to $GOPATH/bin
```

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
| `.index/` | Bleve full-text search index (generated) |
| `.graph.db` | Persistent graph database (generated) |
| `ephemeral/` | Export scratch dir |
| `.gitignore` | Ignores `.index/`, `.graph.db`, `ephemeral/` |

Concept markdown is meant to be committed with the project; generated
artifacts are gitignored.

---

## AI Agents

Two complementary integration points:

- **MCP server** — `okfw mcp` serves five consolidated tools (`search`,
  `create`, `read`, `update`, `graph`) over stdio with compact JSON responses
  (< 3 KB of tool schemas). Register with `claude mcp add okfw -- okfw mcp`,
  launched from the project directory.
- **Agent skill** — copy [`skills/okfw.md`](skills/okfw.md) into your
  project's `.claude/skills/` to teach agents the wiki's conventions (linking
  syntax, atomic notes, capture → refine → link workflow). The skill covers
  the *why and when*; the MCP tools and `okfw --help` cover the mechanics.

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
│   ├── cmd/okfw/        # CLI entry point
│   ├── internal/
│   │   ├── config/      # .okfw discovery and CUE schema validation
│   │   ├── graph/       # Concept relationship graph
│   │   ├── graphdb/     # Persistent graph store (bbolt)
│   │   └── index/       # Bleve search index
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
