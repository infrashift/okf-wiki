# Changelog

All notable changes to this project are documented here.

## Unreleased

### Changed (breaking)
- **Every OKF timestamp is now an ISO 8601 datetime with an explicit offset**, following the spec's 2026-08-21 revision (§5). `stale_after`, `sources[].last_modified` and `usage_window` were bare `YYYY-MM-DD` dates; a bare date names a different instant in every timezone, so the same bundle could be stale in one office and fresh in another. `okfw validate` now rejects bare dates and offset-less datetimes in every timestamp field (`#ISOTime` used to accept anything after the seconds), whether quoted or not. Staleness is `now >= stale_after` compared as instants, and the one implementation (`config.IsStale`) replaces two that disagreed — `search --stale` never reported a spec-form value, and was a day off east of UTC. The writers (`set-stale-after`, `add-source`, and the MCP actions) accept a bare date and store midnight UTC. **Run `okfw migrate`** to upgrade bare dates already in a bundle; it reports, rather than guesses at, any timestamp it cannot repair.

### Fixed
- **okfw can hold an OKF Attested Computation concept.** The closed schema rejected `runtime`, `parameters`, `computation`, `executor` and `attester`, so even the spec's own §10 example failed `okfw validate`, and `x:` cannot carry spec-defined keys. They are now declared and typed, and `type: Attested Computation` requires `runtime` as §10.2 says. okfw still runs and attests nothing.
- **`okfw log` logs the bundle, newest first.** It ran `git log` over the whole repository, so `README.md` and `docs/` appeared in the bundle's log under links like `/.okfw/x.md` that resolve to nothing, and it ordered dates by commit order, which rebases and cherry-picks break — §9 requires newest first. It is now scoped to the bundle (or to the directory given, which gets its own `log.md`), sorts dates, records moves as **Move** and deletions as **Removal** (with no link to the file that no longer exists, rather than **Deprecation**), escapes brackets in commit subjects, and keeps non-ASCII paths readable.
- **`okfw validate --okf` checks what OKF §11 asks and nothing else.** It now checks every `index.md` against §8 (frontmatter only at the bundle root, and only `okf_version`) and every `log.md` against §9 (`## YYYY-MM-DD` headings, newest first), which it never did. It no longer reports a concept with a wrongly-typed optional field as unparseable, no longer warns about a missing `okf_version` (a MAY), and reports a declared version other than 0.2 as checked best-effort. A directory named `ephemeral` below the bundle root is no longer skipped by every command — only the root's is okfw's. okfw's grouped listings end each group with plain text rather than a non-link bullet, so they pass the same check.
- **Concepts no longer vanish from search, the graph and the listings over one bad field or a missing `id`.** Reads decoded frontmatter strictly, so `tags: foo` or an unparseable `created` dropped the whole concept, and every read path skipped concepts without an okfw `id` — a bundle okfw did not write searched as empty and `okfw graph` reported "no valid concepts found". Reads are now lenient (validation and writes are as strict as before), and a concept without an `id` is identified by its OKF concept ID, its path minus `.md`, which every command that takes a concept reference accepts. Building the link graph also opens each file once instead of twice.
- **Links are read by the markdown grammar, not a regular expression.** `okfw move` rewrote link-shaped text inside fenced code and inline code — silently editing examples — while missing links with a title (`[t](x.md "T")`), angle-bracketed destinations, reference definitions (`[r]: x.md`), percent-encoded paths and `.MD`; those broke on every move and were invisible to backlinks, the graph and delete's inbound report. A bundle-absolute link from a concept to itself now follows it when it moves. A link that climbs out of the bundle (`../../x.md` from the root) resolves to nothing instead of being read from outside it.
- **The Neovim plugin works against the current CLI again.** Every `:w` on a concept ran the removed `okfw index` and warned "failed to index"; it now refreshes the `index.md` listings with the new `okfw index-md --no-adopt`, so a title edited by hand reaches them. `:OkfwIndex` runs `okfw index-md`. `<localleader>a` and `<localleader>v` called `add_tags` and `validate` after both had been deleted; they are back, on the CLI's `add-tags` and `validate`. The todo status picker is gone with the todo kind. `preview_by_id` ran a one-result search and looked for the id inside it; it now resolves the id (or an OKF concept ID) directly.
- **okfw no longer deletes or overwrites `index.md` files it did not write.** Every listing okfw generates now carries an `<!-- okfw:generated … -->` marker, and only marked listings are rewritten on each change or pruned when their directory empties. Previously a single `okfw create` replaced a hand-written root listing (rewriting its `okf_version`), and deleted any `index.md` — and the emptied directory — wherever no okfw concept lived. Listings written by an earlier okfw carry no marker: `okfw migrate` or `okfw index-md` adopts them. Listings are also no longer rewritten when already current, and entries now escape `[`/`]` in titles, percent-encode awkward filenames, and keep descriptions on one line.
- **Edits keep a CRLF concept CRLF.** Every frontmatter writer (`set-lifecycle`, `add-tags`, `verify`, …, and their MCP actions) rewrote the block as LF between a CRLF closing delimiter and a CRLF body, and MCP `append` and `set_body` wrote their text as LF too, so a concept saved on Windows came back with mixed line endings. Each writer now uses the line ending the file already has. `set_body` also finds the frontmatter with the strict splitter the other writers use, rather than the first `\n---` in the file.
- **CI runs the Neovim plugin suite**, at Neovim 0.9.5 (the plugin's floor) and 0.11.4. Only luacheck ran before, and it cannot see the plugin calling a command or flag the CLI has dropped — the break `okfw index` caused on save. The suite now runs under `nvim -l`: under `-c "luafile ..."`, an uncaught Lua error left headless nvim waiting for input, so a crash would have hung the job rather than failed it.

### Removed (breaking)
- **graphy is decommissioned.** The code knowledge-graph binary (`cli/cmd/graphy`, `cli/internal/graphy`), its `build-graphy`/`install-graphy`/`graphy-e2e` make targets, its Containerfile stages and the `graphy-e2e.sh` gate are gone, and the build is pure Go again — tree-sitter, bbolt, gonum and hcl left `go.mod`. The okfw surface that existed only to serve it went with it:
  - the `graphy:` frontmatter extension block and `kind: code` are no longer in the concept schema, so concepts written by `graphy export okfw` now fail `okfw validate`. Delete the exported directory (`.okfw/graphy/` by default) rather than migrating it; it was generated from source and carries nothing hand-written.
  - `okfw search --node-id`, and the MCP `okfw_search` `node_id` argument, result field and `kind: "code"` filter.
  - `agentskill`'s after-edit hook support, which only graphy used.

## 2026-07-25

### Changed (breaking)
- **okfw now targets OKF v0.2** (released 2026-07-24). The bundle-root `index.md` declares `okf_version: "0.2"`. Three optional metadata families are implemented, each with a CLI writer:
  - **Lifecycle** (spec §5.4–5.5): `status` (`draft`/`stable`/`deprecated`; absent means `stable`) and `stale_after`, an absolute date — a concept is stale once today is on or after it. `okfw set-lifecycle`, `okfw set-stale-after`, `okfw search --status`, `okfw search --stale`.
  - **Trust** (§5.2–5.3): `generated{by,at}` and `verified[]{by,at}`, with trust tiers derived from `verified` keyed strictly on the `human:` actor prefix. `okfw verify`, `okfw set-generated`. A bare `verified` mapping is read as a one-element list, as the spec requires.
  - **Provenance** (§5.1): `sources[]` (with `resource` required per entry) plus a sibling `usage_window`. `okfw add-source`. `sources[].id` is the join key for `[^id]` body footnotes.
  - Also new: `okfw set-resource` finally writes the `resource` field, which had been modelled and documented but unsettable.
  - Attested Computation (§10) is deliberately not implemented — the spec still defers its receipt formats, attester ABI, and sandboxing.
- **`okfw graph` now documents what it actually emits.** It has always printed Mermaid, while its help promised an ASCII tree. The help is corrected, and the previously-unreachable tree renderer is available as `okfw graph --ascii`.
- **The search index gained `status`/`stale_after` and lost the four todo fields.** Run `rm -rf .okfw/.index && okfw index .okfw` so the new fields become searchable.

### Removed (breaking)
- **Task tracking is gone.** OKF v0.2 spec'd `status` as the lifecycle field, and okfw was using it for todo progress (`open`/`in_progress`/`closed`). Rather than namespace around the collision, the feature was removed: okfw is a knowledge format, not a task tracker.
  - Removed frontmatter: `status` (in its todo sense), `due`, `completed`, `priority`, and `kind: todo`
  - Removed commands: `okfw todo`, `okfw todos`, `okfw todo-list`, `okfw set-status`
  - Removed flags: `okfw search --priority`, `--due-before`, `--due-after` (`--status` survives with its OKF lifecycle meaning)
  - Removed elsewhere: the `todo` template, the `.todos/` directory, MCP's `set_status` action and `kind: "todo"` creation, and the Neovim todo workflow (`:OkfwTodo`, `:OkfwTodoList`)
  - **Migrating:** run `okfw migrate` (`--dry-run` to preview). It drops the retired fields and rewrites `status: closed` to `status: deprecated`; `open`/`in_progress` lose `status` entirely, since absent means `stable`. Idempotent. The free-form `type` is left alone, since OKF does not register type values.
  - MCP gains `set_lifecycle` and `set_stale_after` actions in their place.

### Added
- **`okfw delete <ref>`** (alias `rm`) removes a concept's file, its search-index entry, its graph node, and its line in the `index.md` listings — the four things a plain `rm` leaves desynchronized. It confirms first unless `--force` is given, and refuses to prompt when stdin is not a terminal, so a script that forgot `--force` stops instead of deleting. Concepts still linking to the deleted one are reported but deliberately not rewritten: OKF requires consumers to tolerate a link whose target does not exist, and editing someone's prose to tidy one would destroy authored content.
- **`okfw move <ref> <dest>`** (aliases `rename`, `mv`) relocates a concept, keeping its `id` so `parent` hierarchies survive. The destination follows `mv`: a `.md` path names the file, anything else is a directory to move into. Links are repaired at **both** ends — inbound links in other concepts are retargeted (a bundle-absolute link stays absolute, a relative one stays relative), *and* the moved concept's own relative links are rewritten so they still resolve from the new directory. Missing that second half is what silently breaks a graph: `./sibling.md` quietly points somewhere else the moment a file changes directory.
- **`:OkfwDelete` and `:OkfwMove` in the Neovim plugin.** `:OkfwDelete` acts on the current buffer (or a path), confirms with a No default, and wipes the orphaned buffer afterwards; `:OkfwMove [src] <dest>` moves or renames and the buffer follows the concept to its new path. The confirmation happens in Lua rather than the CLI, because plenary.job hands the child a pipe and `okfw delete` refuses to prompt on anything that is not a terminal — the plugin asks, then passes `--force`.
- **`--json` on `okfw delete` and `okfw move`**, so tooling reads a result object instead of parsing prose. `delete --json` returns `{"id","path","inbound":[…]}`; `move --json` returns `{"from","to","from_rel","to_rel","rewrote":[…],"self_link"}`. `inbound`/`rewrote` are always arrays, never `null`.
- **`okfw validate --all`** applies the strict okfw schema to every concept in the bundle. `okfw validate --okf` remains the permissive OKF §11 conformance check — two questions, two answers, since okfw's schema is deliberately stricter than the spec.
- **An `x:` frontmatter block for custom fields.** The schema is closed, so it rejects a typo'd key like `descrption:` — but that also left no way to attach your own metadata. Anything under `x:` now passes through untouched.
- **`okfw migrate`** brings concepts written by an older okfw up to the current schema.

### Fixed
- **Frontmatter is no longer edited by rewriting lines.** All mutations round-trip through `yaml.Node`, preserving key order, comments, unrecognized keys, and each scalar's quoting style. This fixes a data-loss bug: the previous editor scanned with a `bufio.Scanner` whose 64KB line cap was never checked, so a longer frontmatter line silently truncated the block and the truncated version was written back. It also made nested fields editable at all, which the new families need.
- **Validation now runs on the write path.** `okfw create` refuses to write a concept its own schema rejects, and an edit that would make a *valid* concept invalid is refused before it reaches disk (an already-invalid concept stays editable, or the CLI could never repair one). `graphy export okfw` validates its generated frontmatter too. Previously CUE validation ran only when you asked for it, and the struct-level validator had no callers at all.
- **The search index is pruned.** Deleting a concept used to leave it in `okfw search` results indefinitely — `index.DeleteDocument` had no callers anywhere. `okfw graph-sync` now prunes stale entries.
- **`--path`/`--project` can no longer write into the bundle's internals.** `okfw create X --path .index` resolved inside the Bleve index directory, and because dot-directories are skipped by every walk, the concept was then invisible to search and `index.md`. Any path segment beginning with `.` is now rejected.
- **The compiled CUE schema is cached** instead of being recompiled on every validation.
- **Appended frontmatter fields are written in a stable order** rather than Go's randomized map order.
- **Link rewriting no longer corrupts links whose text equals their target.** `graph.RewriteLinks` replaced the first occurrence of the href within the whole match, so for `[/dir/a.md](/dir/a.md)` — exactly what the Neovim link-insert keymap produces — it rewrote the visible text and left the real target behind. It now splices at the capture group's own offsets. `okfw export` used this path too.
- **Emptying a directory no longer leaves a stale `index.md`** still advertising the concepts that used to be there. `WriteIndexFiles` now prunes listings for directories that hold no concepts, and removes the directory once it is empty (a directory with other files in it is left alone).

### Internal
- **graphy is now in both Containerfiles.** Neither built it before, so the devcontainer shipped without it and the e2e gate never exercised it. The devcontainer installs `graphy` on `PATH` alongside `okfw` and smoke-tests that it starts, which matters because graphy is the only dynamically linked binary in the image — it is built with CGo on Debian and run on Fedora, and a glibc mismatch would otherwise surface as a confusing error on first use. (`graphy install` is per-project and is deliberately not run at build time.)
- **The e2e image now gates the graphy/okfw seam** (`Containerfiles/config/graphy-e2e.sh`, also runnable as `make graphy-e2e`). It builds a graph of a small fixture, exports it into a real bundle, and has okfw validate the result — covering two things `go test` cannot: that graphy's tree-sitter CGo binary runs in the target distro, and that the two binaries agree on the concept schema at runtime, including graphy's hand-built `graphy:` frontmatter block. It also asserts a re-export is a byte-for-byte no-op. Verified to fail as intended when the two disagree.
- The Neovim integration suite now sources `plugin/okfw.lua` explicitly, so the `:Okfw*` user commands are covered for the first time — `nvim -u NONE` implies `--noplugin`, which had made every one of them invisible to the suite. Sourcing it directly keeps the run hermetic, where `-u NORC` would load whatever the developer has installed.
- `cli/internal/bundle` — the write path shared by both binaries — has its own tests for the first time (0% → 89%), and the ~23 pass-through wrappers in `cmd/okfw/main.go` that existed only so `package main` tests could reach it are gone. `cli/internal/gitctx` went from no tests to 100%. Coverage across the okfw internal packages: 55% → 84%.
- CI now runs `-race`, a real (non-`|| true`) staticcheck, `gofmt -l`, `go mod tidy -diff`, and `luacheck`. `cue vet` on the schema alone could never catch a concept that violates `#Concept`; a Go test now validates the fixture corpus against the compiled schema.
- Deleted `cli/internal/config/config.cue` (an empty file) and `templates-schema.cue` (never compiled, and stale).

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
