# Changelog

All notable changes to this project are documented here.

## 2026-07-02

### Changed (breaking)
- **Wikis are now per-project.** The global `~/okfw` bundle and the config file are gone. A wiki is a `.okfw/` directory at the project root:
  - `okfw init` creates it (at the git repository root, else the cwd), writes `.okfw/.gitignore` for generated artifacts, and generates the OKF root `index.md`. Re-running `init` is a no-op.
  - Every command discovers the wiki by searching upward from the working directory (like git); commands run from any project subdirectory. Without a wiki they exit with "run `okfw init`" advice.
  - `okfw connect` verifies the wiki (path, concept count, index/graph presence); `okfw root` prints the discovered path.
  - Fixed layout inside `.okfw/`: concepts + `daily/` at the top level, `.index/` (search), `.graph.db` (graph), `.todos/` (todo lists), `ephemeral/` (exports). Concept markdown is meant to be committed with the project; generated artifacts are gitignored.
  - The config file (`~/.config/okfw/config.cue`, `$OKFW_CONFIG`) and all its keys (`root_path`, `index_path`, `graph_path`, `todos_path`, the never-used `editor`) are removed.
  - Neovim: new `:OkfwInit` command; the plugin resolves the wiki through `okfw root` per working directory.
  - **Migrating an existing `~/okfw` bundle:** `cd <project> && okfw init && mv ~/okfw/* .okfw/ && okfw graph-sync && okfw index .okfw`

### Removed (breaking)
- **`okfw migrate-okf` is gone.** It existed to convert pre-OKF vaults (UUID filenames, `[[id]]` wiki links, `category:` fields); with no legacy bundles left to migrate it was pure maintenance surface. The legacy wiki-link helpers in the graph package went with it.
- **`okfw hello` and `okfw goodbye` (dated-branch git workflow) are gone.** With per-project wikis they operated on the *project* repository — `goodbye` ran `git add -A` + commit + merge there, sweeping up unrelated code changes. Manage git however you like; okfw no longer prescribes a workflow.
- **Project auto-detection from git is gone.** `create`/`todo` no longer derive `project` from the repo name (and no longer file notes under a repo-named subfolder). `--project`/`-p` remains as explicit, optional metadata; without it, notes land at the wiki root. The opt-in `okfw log` (log.md from git history) and `init`'s repo-root placement are unaffected.
- **The `tethered`/`untethered` category feature is gone.** Categories were metadata-only and added workflow friction without affecting placement or search quality.
  - `okfw create --category` and `okfw search --category` flags removed — scripts using them now fail with "unknown flag"
  - `okfw tether` and `okfw untether` commands removed — use `okfw set-project <file> <project>` to assign a project
  - `category` is no longer written to frontmatter; `project` is always optional
  - Existing notes that still carry `category:` remain valid (the field is tolerated and ignored)
  - `folders` config section (`untethered`/`tethered`/`tmp`) removed — it was never read by any command
  - NeoVim: `:OkfwNote` no longer takes a category; `tether_note()`/`untether_note()`, the `untethered()`/`tethered()` pickers, and the `<localleader>t` tether keymap removed
  - Search index: the `category` field and filter are gone — run `okfw index <bundle-root>` to rebuild, and `okfw graph-sync` to refresh the graph database

### Added
- **MCP server: `okfw mcp`.** Serves the wiki over the Model Context Protocol (stdio) for AI agents, designed to minimize token usage versus raw CLI access: five consolidated tools (`search`, `create`, `read`, `update`, `graph`) instead of one per command, terse schemas (< 3 KB total), and compact JSON responses with wiki-relative paths. Bodies are fully writable (`create` takes `body`; `update` supports `append`/`set_body`), so filesystem-less MCP clients get complete access. Dependency-free JSON-RPC implementation. Register with `claude mcp add okfw -- okfw mcp`.
- **Agent skill: `skills/okfw.md`.** A drop-in Claude Code skill teaching agents the wiki's conventions — wiki-relative link syntax, atomic notes, descriptions, and the capture → refine → link workflow. Copy it into a project's `.claude/skills/`.
- **`okfw create --body`** writes markdown body content at creation time.
- **Agent/script-friendly help and output.** `okfw --help` now describes the product, the `.okfw/` discovery model, and the machine-readable surfaces; `okfw create --json` returns `{"id","path"}`, and plain create's `Created: <path>` line is documented as a stable contract.
- **OKF `index.md` listings are maintained automatically.** Creating or updating a note regenerates every directory's `index.md` (root with `okf_version`, others frontmatter-free, entries with descriptions) — including a fresh `index.md` for any subdirectory a `--path`/`--project` note just created. `okfw index-md` remains for explicit regeneration; index files are owned by okfw and overwritten.

### Fixed
- **`okfw daily --list` showed no notes** — it tried to parse a date out of `slug--shortid.md` filenames; it now reads the date from frontmatter (`created`, falling back to the id's embedded timestamp).
- **The search index was created relative to the current working directory**, scattering index directories wherever `okfw` was run and making search results depend on your cwd. The index now always lives inside the wiki (`.okfw/.index/`).
- **CRLF line endings broke body extraction** (link scanning/indexing) — frontmatter/body splitting now handles `\r\n`; the duplicated logic was consolidated into `config.ExtractBody`.
- **Corrupt graph-database index data was silently ignored** — backlink/tag index errors now propagate instead of being treated as an empty list.

### Changed
- **Note mutations keep the graph database fresh** — `set-project`, `add-tags`, and note creation now update the graph database (when it exists) alongside the search index.
- **Graph queries use the persistent backlink index** — `okfw graph --start` and `okfw export --start` run the BFS against the graph database's precomputed indexes instead of loading the whole bundle; the in-memory fallback also replaced its O(n²) reverse-link scan with an indexed lookup.

## 2026-06-26

### Changed
- **Product renamed from "Zettelkasten CLI" (`zk`) to "OKF Wiki" (`okfw`)** — the tool now targets the Open Knowledge Format (OKF), and a note/document is referred to as a "concept".
  - Binary/command renamed `zk` → `okfw` (e.g. `okfw create`, `okfw graph-sync`)
  - Default bundle directory renamed `~/zk_vault` → `~/okfw`
  - Dotfiles renamed: `.zk_index` → `.okfw_index`, `.zk_graph.db` → `.okfw_graph.db`, `.zk_todos` → `.okfw_todos`
  - Config directory renamed `~/.config/zk/` → `~/.config/okfw/`
  - NeoVim user commands renamed `:Zk<Name>` → `:Okfw<Name>` (e.g. `:OkfwDaily`, `:OkfwSearch`)
  - Lua module renamed `require("zk")` → `require("okfw")`; plugin dir `lua/zk/` → `lua/okfw/`, `plugin/zk.lua` → `plugin/okfw.lua`
  - Go source dir renamed `cli/cmd/zk/` → `cli/cmd/okfw/`
  - Claude Code skill files renamed `.claude/skills/zk-*.md` → `.claude/skills/okfw-*.md`
  - NeoVim help doc renamed `doc/zk.txt` → `doc/okfw.txt`
  - Container names renamed `zk-devcontainer` → `okfw-devcontainer`, `zk-dev` → `okfw-dev`

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
- **`okfw graph-sync` command** — walk filesystem and synchronize the persistent graph database
  - `--validate` flag runs CUE edge validation and reports integrity issues
- **`okfw graph-query` command** with subcommands:
  - `backlinks <id>` — O(1) indexed backlink lookup
  - `cluster <id> --depth N --limit N` — BFS N-depth traversal over persistent store
  - `tag <tag>` — find all notes with a given tag
  - `dangling` — find edges pointing to non-existent notes
  - All subcommands support `--json` for machine-readable output (Neovim snacks integration)
- **`--sync` flag on `okfw graph`** — force a fresh graph-sync before querying
- **`graph_path` config field** (default: `.okfw_graph.db`) — configurable graph database location
- **Claude Code skills** (`.claude/skills/`) — four skill files for CLI usage, graph queries, git workflow, and development guide
- **`CLAUDE.md`** — project overview for Claude Code and Codex

### Changed
- **`okfw graph` uses persistent store** when `.okfw_graph.db` exists, falls back to filesystem scanning when it doesn't
- **`okfw backlinks` uses indexed lookup** when graph database exists — O(1) vs previous O(N) filesystem scan
- **Default `root_path` changed** from `~/zettelkasten` to `~/zk_vault` (later renamed to `~/okfw`; see top entry)
- **`go.etcd.io/bbolt`** promoted from indirect to direct dependency
- **Documentation updated** across all 9 docs pages for new default path, graph commands, and config fields

### Removed
- **MCP server** (`cli/cmd/zk-mcp/`, the directory name at the time) — removed in favor of Claude Code skills + CLI integration
- **`mcp-go` dependency** removed from `go.mod`
- **MCP documentation** — tutorial and integration pages removed, sidebar cleaned up

## 2026-03-02

### Changed
- **Moved Go CLI source into `cli/` subdirectory** — `cmd/`, `internal/`, and `testdata/` now live under `cli/` to cleanly separate the Go CLI from the NeoVim plugin files at the repo root. `go.mod` remains at root; import paths gain a `cli/` segment.

### Added
- **Graph export to portable markdown** (`okfw export`, `:OkfwExport`)
  - Runs the same BFS query as `okfw graph` (--limit, --depth, --start)
  - Copies notes into `<root>/ephemeral/` with `[[id|title]]` links converted to `[title](id.md)` relative links
  - Ephemeral directory has a `.gitignore` that ignores all exported files
  - `TransformLinks()` function in `internal/graph` with full test coverage
- **Shared graph helpers** (`buildGraphFromPath`, `queryGraph`) extracted from `graphCmd` for reuse by `exportCmd`
- **Ephemeral directory skip** added to all file walkers (index, graph, backlinks, resolveZettelPath)

### Changed
- **Replaced telescope.nvim with snacks.nvim** for picker UI
  - Removed `lua/okfw/telescope.lua`
  - Updated `lua/okfw/init.lua` with snacks.nvim picker integration
  - Updated plugin installation docs and all workflow guides
- **Refactored `graphCmd`** to use shared `buildGraphFromPath`/`queryGraph` helpers
- **Container config** updated for snacks.nvim dependency

### Fixed
- **Docs site 404s**: internal links used `/zettelkasten-cli/` base path instead of `/zettelkasten/` (matching the GitHub repo name)
- **Docs repo references**: clone URLs, plugin specs, and directory names corrected from `zettelkasten-cli` to `zettelkasten`
