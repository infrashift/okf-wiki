# Zettelkasten CLI (`zk`)

A Go CLI for managing a local-first, file-based Zettelkasten knowledge base. Notes are markdown files with YAML frontmatter, stored in `~/zk_vault` by default.

## Project Layout

```
cli/cmd/zk/main.go          — Cobra CLI entry point (all commands)
cli/internal/config/         — Config loading, CUE schemas, zettel parsing
cli/internal/graph/          — In-memory graph + rendering (ASCII tree, Mermaid)
cli/internal/graphdb/        — Persistent graph store (bbolt)
cli/internal/index/          — Bleve full-text search index
cli/internal/templates/      — Embedded note templates
lua/zk/                      — Neovim plugin (snacks-based pickers)
plugin/zk.lua                — Neovim plugin auto-load
```

## Build & Test

```bash
make build           # Builds ./zk binary
make test            # Runs Go tests + CUE schema validation
make install         # Installs to $GOPATH/bin
go test ./cli/... -count=1   # Run all tests
```

## Data Model

Notes are markdown files with YAML frontmatter. ID format: `YYYYMMDDHHmmss-UUIDv4`.

Categories: `untethered` (quick captures, project optional) or `tethered` (refined, project required).
Types: `note` (default), `todo`, `daily-note`, `issue`.
Links: `[[id]]` or `[[id|title]]` syntax in markdown body.

## Key Conventions

- CUE schemas enforce all validation (config, zettel frontmatter, graph edges)
- bbolt (BoltDB) is the persistent graph store — single-file, read-optimized, pure Go
- Bleve is the full-text search engine — separate from the graph store
- The graph store is optional — commands fall back to filesystem scanning when `.zk_graph.db` doesn't exist
- Run `zk graph-sync` to build/update the persistent graph for O(1) backlink lookups
- The Neovim plugin uses snacks.nvim for pickers (not telescope)
