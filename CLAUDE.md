# OKF Wiki (`okfw`)

A Go CLI for managing a local-first, file-based knowledge base that conforms to
the [Open Knowledge Format (OKF)](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md).
Concepts are markdown files with YAML frontmatter, stored in an OKF **bundle** at
`~/okfw` by default. (Formerly the Zettelkasten CLI `zk`.)

## Project Layout

```
cli/cmd/okfw/main.go         — Cobra CLI entry point (all commands)
cli/internal/config/         — Config loading, CUE schemas, concept parsing
cli/internal/graph/          — In-memory graph + rendering (ASCII tree, Mermaid)
cli/internal/graphdb/        — Persistent graph store (bbolt)
cli/internal/index/          — Bleve full-text search index
cli/internal/templates/      — Embedded note templates
cli/internal/gitctx/         — Git project-context helper
lua/okfw/                    — Neovim plugin (snacks-based pickers)
plugin/okfw.lua              — Neovim plugin auto-load
```

## Build & Test

```bash
make build           # Builds ./okfw binary
make test            # Runs Go tests + CUE schema validation
make install         # Installs to $GOPATH/bin
go test ./cli/... -count=1   # Run all tests
```

## Data Model

Concepts are markdown files with YAML frontmatter. A concept's OKF ID is its file
path within the bundle, minus `.md`. The stable internal `id` is a UUIDv7 (RFC
9562); legacy notes may use `YYYYMMDDHHmmss-UUIDv4`; both formats are accepted.

Categories: `untethered` (quick captures, project optional) or `tethered`
(refined, project required) — an OKF extension and **metadata only**; it does
not dictate file location. Placement: an explicit `--path` wins, else the
project name as a top-level folder, else the bundle root. Concepts may live in
any directory and are still found by id (lookup scans the whole bundle).
Kinds (`kind`): `note` (default), `todo`, `daily-note`, `issue`. OKF's required
free-form `type` field is also written on every concept.
Links: vanilla markdown links to bundle-relative (`/dir/file.md`) or relative
(`./file.md`) paths.

## Key Conventions

- CUE schemas enforce all validation (config, concept frontmatter, graph edges)
- bbolt (BoltDB) is the persistent graph store — single-file, read-optimized, pure Go
- Bleve is the full-text search engine — separate from the graph store
- The graph store is optional — commands fall back to filesystem scanning when `.okfw_graph.db` doesn't exist
- Run `okfw graph-sync` to build/update the persistent graph for O(1) backlink lookups
- Reserved bundle filenames carry no concept frontmatter: `index.md` (directory listing) and `log.md` (update history, generated from git)
- The Neovim plugin uses snacks.nvim for pickers (not telescope)
