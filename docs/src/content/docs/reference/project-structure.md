---
title: Project Structure
description: Development commands and source code layout for OKF Wiki.
---

## Development

```bash
# Run tests
make test
make test-race          # under the race detector

# Run linter
make lint

# Format code
make fmt

# Run NeoVim plugin integration tests
make integration-test

# Check Lua syntax
make ui-check

# CLI + NeoVim suite inside a container
make e2e
```

## Source Layout

```
okf-wiki/
├── cli/
│   ├── cmd/okfw/        # CLI entry point (Cobra commands, MCP server)
│   ├── internal/
│   │   ├── config/      # .okfw discovery, CUE schema, concept parsing
│   │   ├── bundle/      # Write path: concept creation, frontmatter, index.md, link graph
│   │   ├── graph/       # In-memory graph (BFS, Mermaid, ASCII tree)
│   │   ├── search/      # Scan-and-rank concept search (no index)
│   │   ├── agentskill/  # Agent-skill installer (okfw install)
│   │   ├── okfwskills/  # okfw's SKILL.md / CLAUDE.md-block templates
│   │   ├── templates/   # Embedded note templates
│   │   └── gitctx/      # Git project-context helper
│   └── testdata/        # Test fixtures
├── lua/okfw/            # NeoVim plugin (snacks-based pickers)
├── plugin/okfw.lua      # NeoVim plugin auto-load
├── doc/okfw.txt         # NeoVim help file
├── docs/                # Documentation site (Astro Starlight)
├── go.mod, go.sum       # Go module (stays at root)
└── Makefile
```

## Wiki Layout

Each project's knowledge base is a `.okfw/` directory at the project's root,
created by `okfw init` and discovered automatically by every command (searching
upward from the current directory, like git finds `.git`). Concepts are
markdown files named `slug--shortid.md`, where the short id is the first 12
hex digits of the concept's UUID. A concept's OKF id is its path within the wiki minus
`.md`.

Concepts are organized by topic and project, not by maturity. With no `--path`,
a concept lands in a top-level folder named after its project (or the wiki
root when there is no project); an explicit `--path projects/okf` overrides
that.

```
.okfw/                             # Wiki root (at the project root)
├── index.md                       # Directory listing (reserved; okf_version)
├── log.md                         # Update history (reserved)
├── infra/                         # A project folder (concepts grouped by project)
│   ├── index.md
│   └── my-first-idea--019f049400a8.md
├── daily/                         # Daily notes, grouped by year/month
│   └── 2026/
│       └── 06/
│           └── 2026-06-26-friday--019f04a10000.md
├── any-other-folder/              # Concepts may live in any directory you choose
│   └── refined-concept--019f04b80000.md
├── ephemeral/                     # Export scratch dir (gitignored)
└── .gitignore                     # Ignores ephemeral/
```

Concept markdown is meant to be **committed with the project**; the only
generated directory, `ephemeral/`, is covered by the `.gitignore` that
`okfw init` writes. There is no search index or graph database: search and the
link graph are computed from the files on every call.

`index.md` and `log.md` are **reserved filenames** that carry no concept
frontmatter. `index.md` is a per-directory listing for progressive disclosure
(the root `index.md` carries `okf_version: "0.2"`), and `log.md` is an update
history generated from git. Generate them with `okfw index-md` and `okfw log`.
