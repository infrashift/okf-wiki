---
title: Project Structure
description: Development commands and source code layout for OKF Wiki.
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

## Source Layout

```
okfw/
├── cli/
│   ├── cmd/okfw/        # CLI entry point (Cobra commands)
│   ├── internal/
│   │   ├── config/      # CUE schemas and config loading
│   │   ├── graph/       # In-memory graph (BFS, ASCII tree, Mermaid)
│   │   ├── graphdb/     # Persistent graph store (bbolt)
│   │   ├── index/       # Bleve full-text search index
│   │   ├── templates/   # Embedded note templates
│   │   └── zettel/      # Note utilities (git context)
│   └── testdata/        # Test fixtures
├── lua/okfw/            # NeoVim plugin (snacks-based pickers)
├── plugin/              # NeoVim autoload
├── after/               # NeoVim ftplugin
├── go.mod, go.sum       # Go module (stays at root)
└── Makefile
```

## Bundle Layout

Your knowledge base (the "bundle") lives separately from the source tree, at
`~/okfw` by default. Concepts are markdown files named `slug--shortid.md`, where
the short id is the leading hex of the concept's UUID. A concept's OKF id is its
bundle-relative path minus `.md`.

Concepts are organized by topic and project, not by maturity. A concept's
`category` (`untethered`/`tethered`) is metadata only and does not affect where
the file is placed. With no `--path`, a concept lands in a top-level folder named
after its project (or the bundle root when there is no project); an explicit
`--path projects/okf` overrides that. okfw does not create or require any
`untethered/`/`tethered/` folders.

```
~/okfw/                            # Bundle root
├── index.md                       # Directory listing (reserved; okf_version)
├── log.md                         # Update history (reserved)
├── infra/                         # A project folder (concepts grouped by project)
│   ├── index.md
│   └── my-first-idea--019f0494.md
├── daily/                         # Daily notes, grouped by year/month
│   └── 2026/
│       └── 06/
│           └── 2026-06-26--019f04a1.md
├── any-other-folder/              # Concepts may live in any directory you choose
│   └── refined-concept--019f04b8.md
├── ephemeral/                     # Exports (gitignored)
├── .okfw_index/                   # Bleve full-text search index (auto-managed)
├── .okfw_graph.db                 # Persistent graph database (auto-managed)
└── .okfw_todos/                   # Generated todo lists
```

`index.md` and `log.md` are **reserved filenames** that carry no concept
frontmatter. `index.md` is a per-directory listing for progressive disclosure
(the root `index.md` carries `okf_version: "0.1"`), and `log.md` is an update
history generated from git. Generate them with `okfw index-md` and `okfw log`.

