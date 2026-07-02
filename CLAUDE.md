# OKF Wiki (`okfw`)

A Go CLI for managing a local-first, file-based knowledge base that conforms to
the [Open Knowledge Format (OKF)](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md).
Concepts are markdown files with YAML frontmatter, stored in a per-project OKF
**bundle**: a `.okfw/` directory at the project root, created by `okfw init`
and discovered by every command walking upward from the cwd (like `.git`).
There is no config file — storage is standardized. (Formerly the Zettelkasten
CLI `zk`.)

## Project Layout

```
cli/cmd/okfw/main.go         — Cobra CLI entry point (all commands)
cli/internal/config/         — .okfw discovery, CUE schema, concept parsing
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

Placement: an explicit `--path` wins, else an explicit `--project` name as a
top-level folder, else the wiki root. Concepts may live in any directory and
are still found by id (lookup scans the whole bundle). `project` is optional
metadata, never auto-derived.
Kinds (`kind`): `note` (default), `todo`, `daily-note`, `issue`. OKF's required
free-form `type` field is also written on every concept.
Links: vanilla markdown links to bundle-relative (`/dir/file.md`) or relative
(`./file.md`) paths.

## Wiki Layout (fixed, inside `.okfw/`)

- Concept markdown at the top level (subdirs via `--path` or an explicit `--project`); `daily/YYYY/MM/` for daily notes
- `.index/` (Bleve search index), `.graph.db` (bbolt graph), `.todos/` (generated todo lists), `ephemeral/` (export scratch) — all gitignored via `.okfw/.gitignore` written by `okfw init`
- `okfw connect` / `okfw root` verify or print the discovered wiki; commands error with "run `okfw init`" advice when none is found

## Key Conventions

- CUE schemas enforce all validation (concept frontmatter, graph edges)
- bbolt (BoltDB) is the persistent graph store — single-file, read-optimized, pure Go
- Bleve is the full-text search engine — separate from the graph store
- The graph store is optional — commands fall back to filesystem scanning when `.graph.db` doesn't exist
- Run `okfw graph-sync` to build/update the persistent graph for O(1) backlink lookups
- Reserved bundle filenames carry no concept frontmatter: `index.md` (directory listing, auto-maintained by okfw on every note change; root copy carries `okf_version`) and `log.md` (update history, generated from git)
- The Neovim plugin uses snacks.nvim for pickers (not telescope)
- `okfw mcp` serves an MCP stdio server (cli/cmd/okfw/mcp.go): five consolidated tools (search/create/read/update/graph), compact JSON, no protocol dependency
