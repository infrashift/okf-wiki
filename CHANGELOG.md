# Changelog

All notable changes to this project are documented here.

## 2026-04-10

### Added
- **Persistent graph database** (`cli/internal/graphdb/`)
  - bbolt (BoltDB) backend — single-file, read-optimized, pure Go
  - Denormalized `backlinks` and `tags` buckets for O(1) indexed lookups
  - Incremental sync: compares file modification times, only processes changed notes
  - Automatic purge of deleted notes and their associated edges
  - CUE schema validation for edge structure (`edge-schema.cue`)
  - Referential integrity checking (dangling edge detection)
  - Full test coverage (25 tests across store, queries, and schema validation)
- **`zk graph-sync` command** — walk filesystem and synchronize the persistent graph database
  - `--validate` flag runs CUE edge validation and reports integrity issues
- **`zk graph-query` command** with subcommands:
  - `backlinks <id>` — O(1) indexed backlink lookup
  - `cluster <id> --depth N --limit N` — BFS N-depth traversal over persistent store
  - `tag <tag>` — find all notes with a given tag
  - `dangling` — find edges pointing to non-existent notes
  - All subcommands support `--json` for machine-readable output (Neovim snacks integration)
- **`--sync` flag on `zk graph`** — force a fresh graph-sync before querying
- **`graph_path` config field** (default: `.zk_graph.db`) — configurable graph database location
- **Claude Code skills** (`.claude/skills/`) — four skill files for CLI usage, graph queries, git workflow, and development guide
- **`CLAUDE.md`** — project overview for Claude Code and Codex

### Changed
- **`zk graph` uses persistent store** when `.zk_graph.db` exists, falls back to filesystem scanning when it doesn't
- **`zk backlinks` uses indexed lookup** when graph database exists — O(1) vs previous O(N) filesystem scan
- **Default `root_path` changed** from `~/zettelkasten` to `~/zk_vault`
- **`go.etcd.io/bbolt`** promoted from indirect to direct dependency
- **Documentation updated** across all 9 docs pages for new default path, graph commands, and config fields

### Removed
- **MCP server** (`cli/cmd/zk-mcp/`) — removed in favor of Claude Code skills + CLI integration
- **`mcp-go` dependency** removed from `go.mod`
- **MCP documentation** — tutorial and integration pages removed, sidebar cleaned up

## 2026-03-02

### Changed
- **Moved Go CLI source into `cli/` subdirectory** — `cmd/`, `internal/`, and `testdata/` now live under `cli/` to cleanly separate the Go CLI from the NeoVim plugin files at the repo root. `go.mod` remains at root; import paths gain a `cli/` segment.

### Added
- **Graph export to portable markdown** (`zk export`, `:ZkExport`)
  - Runs the same BFS query as `zk graph` (--limit, --depth, --start)
  - Copies notes into `<root>/ephemeral/` with `[[id|title]]` links converted to `[title](id.md)` relative links
  - Ephemeral directory has a `.gitignore` that ignores all exported files
  - `TransformLinks()` function in `internal/graph` with full test coverage
- **Shared graph helpers** (`buildGraphFromPath`, `queryGraph`) extracted from `graphCmd` for reuse by `exportCmd`
- **Ephemeral directory skip** added to all file walkers (index, graph, backlinks, resolveZettelPath)

### Changed
- **Replaced telescope.nvim with snacks.nvim** for picker UI
  - Removed `lua/zk/telescope.lua`
  - Updated `lua/zk/init.lua` with snacks.nvim picker integration
  - Updated plugin installation docs and all workflow guides
- **Refactored `graphCmd`** to use shared `buildGraphFromPath`/`queryGraph` helpers
- **Container config** updated for snacks.nvim dependency

### Fixed
- **Docs site 404s**: internal links used `/zettelkasten-cli/` base path instead of `/zettelkasten/` (matching the GitHub repo name)
- **Docs repo references**: clone URLs, plugin specs, and directory names corrected from `zettelkasten-cli` to `zettelkasten`
