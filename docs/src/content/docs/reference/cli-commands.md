---
title: CLI Commands
description: Quick reference for all okfw CLI commands, flags, and workflows.
---

Quick reference for all `okfw` commands.

Every command discovers the wiki (a `.okfw/` directory) by searching upward
from the current working directory, so commands work from any subdirectory of
your project. When no wiki is found, commands exit non-zero with:

```
no OKF wiki found (searched upward from <dir>); run 'okfw init' at your project root to create one
```

---

## Wiki Setup

### Initialize a wiki
```bash
okfw init
```

Creates a `.okfw/` directory at the git repository root (or the current
directory outside git), writes a `.okfw/.gitignore` covering the only
generated directory (`ephemeral/`), and generates the
OKF root `index.md` (with the `okf_version` declaration). Running `okfw init`
again is a no-op that prints `OKF wiki already initialized at <path>`.

### Verify the wiki connection
```bash
okfw connect
```

Verifies the discovered wiki: prints `Connected: <path>` plus a `concepts: N`
line. Exits non-zero with
the `okfw init` advice when no wiki is found.

---

## Note Creation

### Create a note (no project required)
```bash
okfw create "Quick idea about X"
okfw create "My note title"
```

### Create a note with a project
```bash
okfw create "Project idea" --project my-project
okfw create "Project idea" -p my-project
```

### Create a note with a description
```bash
okfw create "OAuth notes" --description "How our OAuth flow works"
okfw create "OAuth notes" -d "How our OAuth flow works"
```

The `-d/--description` value is written to the optional OKF `description`
frontmatter field (a one-sentence summary of the concept).

### Place a note in a specific wiki subdirectory
```bash
okfw create "Spec draft" --path projects/okf
```

`--path <subdir>` places the new concept at an arbitrary path under the wiki
root (here, `.okfw/projects/okf/`). It is sanitized to stay inside the wiki.
With no `--path`, placement falls back to a top-level folder named after the
project, or the wiki root when no project is set.

---

## Note Management

### Set project on a note
```bash
okfw set-project path/to/note.md my-project
```

### Delete a concept
```bash
okfw delete ./notes/scratch--019f0494.md
okfw delete 019f0494 --force        # skip the confirmation
okfw rm 019f0494 -f                 # alias
okfw delete 019f0494 -f --json      # {"id","path","inbound":[...]}
```

Removes the concept's file and its line in the `index.md` listings.
There is no search index or graph database to update: both are computed from
the files on every call. Asks for confirmation unless `--force` is given, and
refuses to prompt when stdin is not a terminal — a script that forgot `--force`
stops rather than deleting.

Concepts that still link to the deleted one are **reported, not repaired**. OKF
requires consumers to tolerate a link whose target does not exist, so a dangling
link is not malformed; rewriting someone's prose to tidy one would destroy
authored content.

### Move or rename a concept
```bash
okfw move 019f0494 archive/                        # into a directory
okfw move ./scratch--019f0494.md notes/design.md   # new path and name
okfw rename 019f0494 notes/better-name.md          # alias
okfw move 019f0494 archive/ --json                 # {"from","to","rewrote":[...]}
```

The destination works like `mv`: a path ending in `.md` is the concept's new full
path, anything else is a directory to move into under the current filename. Both
are bundle-relative and cannot escape the bundle or name one of its internal
dot-directories.

The concept's `id` does not change, so hierarchies built with `parent` survive.
Links are repaired at **both** ends, because a move breaks two different sets:

- Other concepts pointing at the old path are retargeted, keeping whichever form
  they used — a bundle-absolute link stays absolute, a relative one stays relative.
- The moved concept's own *relative* links are rewritten so they still resolve
  from the new directory. Bundle-absolute links need no fixing, which is exactly
  why OKF recommends them.

The moved concept's `timestamp` is bumped, since a concept's OKF identity is its
path within the bundle. Concepts whose links were merely retargeted are not
stamped: their knowledge did not change, only the spelling of a pointer.

---

## Tags

### Add tags to a concept
```bash
okfw add-tags path/to/note.md golang
okfw add-tags path/to/note.md golang api security
```

Tags are appended to the existing `tags:` list in the concept's YAML frontmatter. Duplicate tags are ignored.

---

## Validation

There are two different checks, because okfw's schema is deliberately stricter
than OKF requires (see [Note Format](/okf-wiki/getting-started/note-format/#okf-conformance-and-divergences)).

### Validate a concept against okfw's schema
```bash
okfw validate path/to/note.md
okfw validate 019f0494                 # by id
```

Applies the full CUE `#Concept` schema: the six required fields, the declared
value formats, and no undeclared top-level keys.

### Validate every concept in the wiki
```bash
okfw validate --all                 # strict check of the discovered wiki
okfw validate --all path/to/.okfw/  # ...or an explicit path
```

The same strict check over the whole bundle, reporting each failure. Exits
non-zero if any concept is invalid.

### Validate OKF conformance
```bash
okfw validate --okf                 # Validate the discovered wiki
okfw validate --okf path/to/.okfw/  # Validate an explicit wiki path
```

Checks Open Knowledge Format conformance (spec §11) and nothing more: every
non-reserved `.md` file has parseable frontmatter with a non-empty `type`, and
the reserved `index.md` and `log.md` files follow §8 and §9. Permissive by
design, so a bundle okfw did not author can still pass. Broken links are
reported as warnings — OKF requires consumers to tolerate them.

### Migrate concepts from an older okfw
```bash
okfw migrate --dry-run   # report what would change
okfw migrate             # apply
```

Rewrites concepts so they satisfy the current schema. okfw removed its
task-tracking fields, because OKF v0.2 spec'd `status` as the lifecycle field
and okfw was using it for todo progress. This drops `due`, `completed`,
`priority` and `kind: todo`, and rewrites a legacy `status`:

| Before | After |
|---|---|
| `status: closed` | `status: deprecated` |
| `status: open` or `status: in_progress` | removed (absent means `stable`) |

It also upgrades bare dates in `stale_after`, `sources[].last_modified` and
`usage_window` to ISO 8601 datetimes (`2026-12-31` becomes
`2026-12-31T00:00:00Z`, the reading okfw already gave it), and reports any
timestamp it cannot safely upgrade for you to fix by hand.

Safe to re-run; a second pass reports nothing to do. The free-form `type` is left
alone, since OKF does not register type values.

---

## OKF Wiki

Commands for keeping a wiki conformant with the Open Knowledge Format (OKF).

### Print the wiki root
```bash
okfw root
```

Prints the discovered `.okfw` path (found by searching upward from the current
directory). Script-friendly; used by the Neovim plugin. Errors when no wiki
exists.

### Serve the wiki to AI agents (MCP)
```bash
okfw mcp                         # MCP stdio server (run from the project)
```

Runs a Model Context Protocol server on stdin/stdout, exposing the wiki
through five consolidated tools — advertised as `okfw_search`, `okfw_create`,
`okfw_read`, `okfw_update`, `okfw_graph` (the bare names still dispatch) —
instead of one tool per CLI command. Concept bodies are writable
through the tools (`create` takes `body`; `update` supports `append` and
`set_body`), so filesystem-less MCP clients get full read/write access. Tool responses are compact
JSON with wiki-relative paths, keeping agent token usage low (the full `tools/list`
payload is about 3.9 KB). The wiki is discovered from the server's
working directory, so launch it from inside the project.

Client registration (Claude Code):

```bash
claude mcp add okfw -- okfw mcp
```

### Install the agent skill
```bash
okfw install                     # Claude Code: skill + CLAUDE.md block
okfw install --platform gemini   # GEMINI.md block
okfw install --platform codex    # AGENTS.md block
okfw install --force             # overwrite an existing skill file
```

Registers okfw with an AI coding assistant so it captures and links project
knowledge in the wiki. The managed marker block keeps re-installs idempotent and
is namespaced, so it leaves the rest of the file (and blocks other tools
maintain) alone. Run from anywhere inside the project.

### Generate `index.md` directory listings
```bash
okfw index-md                    # Discovered wiki
okfw index-md path/to/.okfw/     # Explicit path
okfw index-md --no-adopt         # Refresh only listings okfw already maintains
```

Generates OKF `index.md` files (directory listings for progressive disclosure)
in each directory of the wiki. The root `index.md` carries the wiki's
`okf_version: "0.2"`; all other index files have no frontmatter, and entries
include each concept's description.

You rarely need to run this by hand: creating or updating a note keeps every
`index.md` fresh automatically — including a new `index.md` for any
subdirectory a `--path`/`--project` note just created. okfw marks the listings
it generates with an `<!-- okfw:generated ... -->` comment and maintains only
those; hand edits to a marked listing will not survive. `okfw index-md` also
adopts an unmarked `index.md` in a directory holding okfw concepts (that is how
listings from before the marker come back under maintenance), but never deletes
one. `--no-adopt` skips the adoption and refreshes only marked listings, which is
what every okfw write does.

### Generate `log.md` from git history
```bash
okfw log                         # Discovered wiki
okfw log path/to/.okfw/          # Explicit path
```

Generates an OKF `log.md` update history from the project's git log, grouped by
date (newest first), with Creation / Update / Move / Removal entries using each
commit's subject. Pass a directory inside the bundle to give it its own
`log.md`.

---

## Searching

### Full-text search
```bash
okfw search "authentication patterns"
okfw search "golang error handling"
```

### Filter by project
```bash
okfw search --project my-project
okfw search -p my-project
okfw search "query" --project my-project  # Combined
```

### Filter by tags
```bash
okfw search --tag golang
okfw search --tag golang --tag api  # AND (must have both)
okfw search -T golang -T testing
```

### Filter by structured kind
```bash
okfw search --kind daily-note
okfw search --kind issue
```

`--kind` filters the structured `kind` field (`note`/`daily-note`/`issue`)
that drives daily-note and issue behavior.

### Filter by OKF type
```bash
okfw search --type note
okfw search -t book
```

`--type` filters the free-form OKF `type` field. (Previously `--type` filtered
note/daily-note/issue — that behavior is now `--kind`.)

### Filter by lifecycle status
```bash
okfw search --status draft        # not yet reviewed
okfw search --status deprecated   # superseded, kept for links and history
```

Concepts with no `status` are `stable`, and are returned by `--status stable`.

### Find stale concepts
```bash
okfw search --stale
```

Returns concepts whose `stale_after` instant has been reached (now on or after
it). Set it with `okfw set-stale-after`.

### Limit and page results
```bash
okfw search --limit 10
okfw search -l 50
okfw search "auth" --limit 10 --offset 10   # second page
```

Results are ranked (title over tags over description over body). Search scans
the files on every call, so there is no index to build or keep in sync.

### JSON output (for scripting/tooling)
```bash
okfw search --json
okfw search "query" --json | jq '.[] | .title'
```

### Combined filters
```bash
okfw search "authentication" --project my-project --tag security
okfw search --kind issue --status draft --project my-project
```

---

## Graph Visualization

### Generate a graph from a directory
```bash
okfw graph .okfw/
okfw graph .
```

### Limit the number of nodes
```bash
okfw graph . --limit 20
okfw graph . -l 5
```

### Start from a specific note
```bash
okfw graph . --start 019f0494                           # id prefix
okfw graph . --start infra/my-first-idea--019f049400a8  # or OKF concept ID
```

### Control traversal depth
```bash
okfw graph . --depth 3
```

### Render a plain-text tree
```bash
okfw graph --ascii
```

`okfw graph` writes **Mermaid flowchart syntax** to stdout by default, suitable
for pasting into markdown; `--ascii` renders a plain-text tree that reads
directly in a terminal. A summary line (`N nodes, M edges`) goes to stderr, so
piped output stays clean. The graph is built from the concepts on disk each
time, so it always matches them.

---

## Export

### Export a portable subset of the wiki
```bash
okfw export                              # Export from the wiki root
okfw export path/to/dir                  # Export a specific directory
okfw export --start 019f0494 --depth 2   # Export a neighbourhood
okfw export --limit 50
```

Writes a self-contained copy of a connected subset of concepts into
`.okfw/ephemeral/`, one file per concept named `<id>.md`, with links rewritten to
match (an in-bundle link becomes `<id>.md`). Useful for handing a slice of the wiki to something that has no access to
the bundle. `ephemeral/` is gitignored and safe to delete.

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--start` | - | most connected | Concept id to start from |
| `--depth` | - | unlimited | Maximum hops from `--start` |
| `--limit` | `-l` | `10` | Maximum number of concepts |
| `--help` | `-h` | - | Show help |

---

## Graph Queries

`okfw graph-query` answers structural questions about the link graph. Like
`okfw graph`, it builds the graph from the files on every call — there is no
graph database to sync.

### Query backlinks
```bash
okfw graph-query backlinks <id>
okfw graph-query backlinks <id> --json
```

### Query connected cluster (N-depth BFS)
```bash
okfw graph-query cluster <id>
okfw graph-query cluster <id> --depth 2 --limit 20
okfw graph-query cluster <id> --json
```

### Query by tag
```bash
okfw graph-query tag golang
okfw graph-query tag golang --json
```

### Find dangling edges
```bash
okfw graph-query dangling
okfw graph-query dangling --json
```

`--json` emits the graph's node records with Go field names (`ID`, `Title`,
`FilePath`, `Project`, `Parent`, `Children`, `Links`, `Tags`); `cluster` wraps
them as `{"nodes":[...],"edges":[{"From","To","Label"}],"total":N}`, adding
`"truncated":true` when `--limit` withheld some of the `total` concepts in
reach (the text output ends `… N more in reach — raise --limit`). An empty
result is `null`.

---

## Note Templates

List and use note templates for structured content.

### List available templates
```bash
okfw templates
```

### Create a note from a template
```bash
okfw create "Sprint Planning Q1" --template meeting
okfw create "Clean Code Review" --template book-review
okfw create "OAuth Implementation" --template feature
okfw create "Add Login Flow" --template user-story
okfw create "CLI Tool Idea" --template project-idea
okfw create "Go Error Handling" --template snippet
```

Available templates:
- `meeting` - Meeting notes with attendees and action items
- `book-review` - Book review with rating and key takeaways
- `snippet` - Code snippet with context and explanation
- `project-idea` - Project idea with goals and next steps
- `user-story` - User story in standard format with acceptance criteria
- `feature` - Feature specification with requirements and design notes
- `daily` - Daily note for thoughts, tasks, and reflections
- `issue` - Issue tracking like GitHub (bug, enhancement, question)

---

## Backlinks

Find all notes that link to a specific concept.

### Find backlinks by ID
```bash
okfw backlinks 202602131045
```

When given an id, `okfw backlinks` (like every command that takes a concept)
searches the whole wiki, so a concept is found regardless of which folder it
lives in. A concept can be named by its full `id`, a unique id prefix of at
least 8 hex digits (`019f0494`), its OKF concept ID
(`infra/my-note--019f049400a8`, optionally with `.md` or a leading `/`), or a
file path.

### Find backlinks by file path
```bash
okfw backlinks ./notes/202602131045.md
```

### JSON output
```bash
okfw backlinks 202602131045 --json
okfw backlinks 202602131045 --json | jq '.[].title'
```

---

## Daily Notes

Create and manage daily notes for capturing thoughts, tasks, and reflections.

### Create or open today's daily note
```bash
okfw daily
```

### Open yesterday's daily note
```bash
okfw daily --yesterday
```

### Open a specific date's daily note
```bash
okfw daily --date 2026-02-10
```

### List recent daily notes
```bash
okfw daily --list              # Last 14 days
okfw daily --list --week       # This week
okfw daily --list --month      # This month
okfw daily --list --json       # JSON output
```

Daily notes are stored inside the wiki at `.okfw/daily/YYYY/MM/YYYY-MM-DD-<weekday>--<shortid>.md`
(e.g. `daily/2026/06/2026-06-26-friday--019f04a10000.md`).

See [Daily Notes Workflow](/okf-wiki/neovim/daily-notes-workflow/) for a comprehensive guide to daily note workflows.

---

## OKF Metadata

okfw implements OKF v0.2's lifecycle, trust, and provenance families. Each has a
writer, so no field is set-able only by hand-editing YAML.

### Lifecycle
```bash
okfw set-lifecycle 019f0494 deprecated   # draft | stable | deprecated
okfw set-stale-after 019f0494 2026-12-31T00:00:00Z
okfw set-stale-after 019f0494 2026-12-31          # stored as 2026-12-31T00:00:00Z
okfw set-stale-after 019f0494 --clear
```

An absent `status` means `stable`. A concept is stale once now is on or after
`stale_after` — an absolute instant, not a TTL. OKF requires an ISO 8601
datetime with a UTC offset; a bare date is accepted and stored as midnight UTC.

### Trust
```bash
okfw verify 019f0494                              # defaults to human:<your login>
okfw verify 019f0494 --by process:nightly-audit
okfw verify 019f0494 --by human:ryan --at 2026-07-20T10:00:00Z
okfw set-generated 019f0494 --by process:finance-nightly
```

`verify` appends to the concept's `verified` list. Consumers derive a trust tier
from it: no entries means `unverified`, non-human actors mean
`machine-confirmed`, and any `human:` actor means `human-reviewed`. Running
`verify` by hand is a human confirmation, hence the default actor.

`generated.at` is not set directly — it mirrors `timestamp`, and okfw keeps the
two equal.

### Provenance
```bash
okfw add-source 019f0494 --resource https://example.com/schema --id ga4-schema
okfw add-source 019f0494 --resource "all queries in project X" \
    --usage-count 412 --usage-window 2026-01-01T00:00:00Z..2026-06-30T00:00:00Z
```

`--resource` is required, and names either something a consumer can follow or a
scope descriptor it cannot. `--id` is the join key for `[^id]` body footnotes,
so a specific claim can cite a specific source.

### The described asset
```bash
okfw set-resource 019f0494 bigquery://project.dataset.orders
```

Sets OKF's `resource` field: a URI identifying the underlying asset this concept
describes. Absent for concepts about abstract ideas.

See [Note Format](/okf-wiki/getting-started/note-format/) for the full field
reference.

:::note
okfw has **no task tracking**. `okfw todo`, `todos`, `todo-list`, and `set-status`
were removed along with the `todo` kind: OKF v0.2 spec'd `status` as the
lifecycle field, and okfw is a knowledge format rather than a task tracker. Run
`okfw migrate` to clean up concepts written before the removal.
:::

---

## Help

### General help
```bash
okfw --help
okfw -h
```

### Command-specific help
```bash
okfw create --help
okfw search --help
```

---

## Flags Reference

All commands (other than `okfw init`) require a discovered wiki; they exit
non-zero with the `okfw init` advice when none is found.

### `okfw init`

Usage: `okfw init`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--help` | `-h` | - | Show help |

### `okfw connect`

Usage: `okfw connect`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--help` | `-h` | - | Show help |

### `okfw create`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--body` | - | - | Markdown body content for the note |
| `--json` | - | `false` | Print `{"id","path"}` as JSON instead of prose |
| `--project` | `-p` | - | Optional project metadata (also used as the note's folder) |
| `--description` | `-d` | - | One-sentence OKF description (written to frontmatter) |
| `--path` | - | - | Place the concept in an arbitrary wiki subdirectory (e.g. `projects/okf`) |
| `--tags` | - | - | Additional tags (comma-separated) |
| `--template` | `-T` | - | Use a note template (e.g., `meeting`, `user-story`) |
| `--help` | `-h` | - | Show help |

### `okfw templates`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--help` | `-h` | - | Show help |

### `okfw set-project`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--help` | `-h` | - | Show help |

### `okfw add-tags`

Usage: `okfw add-tags <id_or_file> <tag1> [tag2...]`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--help` | `-h` | - | Show help |

### `okfw validate`

Usage: `okfw validate <id_or_file>`, `okfw validate --all [path]`, or `okfw validate --okf [path]`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--all` | - | `false` | Apply the strict okfw schema to every concept in the bundle |
| `--okf` | - | `false` | Validate the whole wiki for OKF §11 conformance: every concept has parseable frontmatter with a non-empty `type`, every `index.md` follows §8 (no frontmatter except `okf_version` at the root) and every `log.md` follows §9 (`## YYYY-MM-DD` headings, newest first). Broken links, listing entries that are not links, and a declared `okf_version` other than okfw's are reported as warnings |
| `--help` | `-h` | - | Show help |

### `okfw mcp`

Usage: `okfw mcp`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--help` | `-h` | - | Show help |

### `okfw install`

Usage: `okfw install [--platform claude|gemini|codex]`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--platform` | - | `claude` | Agent platform: `claude`, `gemini`, or `codex` |
| `--force` | - | `false` | Overwrite an existing skill file |
| `--help` | `-h` | - | Show help |

### `okfw index-md`

Usage: `okfw index-md [path]`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--no-adopt` | - | `false` | Refresh only listings okfw already maintains; never take over an unmarked `index.md` |
| `--help` | `-h` | - | Show help |

### `okfw log`

Usage: `okfw log [path]`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--help` | `-h` | - | Show help |

### `okfw root`

Usage: `okfw root`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--help` | `-h` | - | Show help |

### `okfw search`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--project` | `-p` | - | Filter by project |
| `--tag` | `-T` | - | Filter by tag (repeatable, AND logic) |
| `--kind` | - | - | Filter by structured kind (`note`/`daily-note`/`issue`) |
| `--type` | `-t` | - | Filter by the free-form OKF `type` field |
| `--status` | - | - | Filter by OKF lifecycle status (`draft`/`stable`/`deprecated`) |
| `--stale` | - | `false` | Only concepts whose `stale_after` instant has been reached |
| `--limit` | `-l` | `20` | Maximum number of results |
| `--offset` | - | `0` | Skip this many results (pagination) |
| `--json` | - | `false` | Output as JSON |
| `--help` | `-h` | - | Show help |

### `okfw graph`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--ascii` | - | `false` | Render a plain-text tree instead of Mermaid |
| `--limit` | `-l` | `10` | Maximum number of nodes to display |
| `--start` | | - | Start tree from a specific note ID |
| `--depth` | | `0` (unlimited) | Maximum tree depth |
| `--help` | `-h` | - | Show help |

### `okfw graph-query`

Subcommands: `backlinks <id>`, `cluster <id>`, `tag <tag>`, `dangling`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--json` | | `false` | Output as JSON |
| `--depth` | | `0` (unlimited) | Maximum BFS depth (cluster only) |
| `--limit` | `-l` | `10` | Maximum number of nodes (cluster only) |
| `--help` | `-h` | - | Show help |

### `okfw backlinks`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--json` | - | `false` | Output as JSON |
| `--help` | `-h` | - | Show help |

### `okfw daily`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--date` | - | - | Target date (YYYY-MM-DD format) |
| `--yesterday` | - | `false` | Use yesterday's date |
| `--list` | - | `false` | List recent daily notes |
| `--week` | - | `false` | Show this week's notes (with --list) |
| `--month` | - | `false` | Show this month's notes (with --list) |
| `--json` | - | `false` | Output as JSON (with --list) |
| `--help` | `-h` | - | Show help |

### `okfw delete`

Usage: `okfw delete <id_or_file>` (alias: `rm`)

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--force` | `-f` | `false` | Delete without confirming |
| `--json` | - | `false` | Output the result as JSON |
| `--help` | `-h` | - | Show help |

### `okfw move`

Usage: `okfw move <id_or_file> <destination>` (aliases: `rename`, `mv`)

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--json` | - | `false` | Output the result as JSON |
| `--help` | `-h` | - | Show help |

### `okfw set-lifecycle`

Usage: `okfw set-lifecycle <id_or_file> <draft|stable|deprecated>`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--help` | `-h` | - | Show help |

### `okfw set-stale-after`

Usage: `okfw set-stale-after <id_or_file> [DATETIME]`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--clear` | - | `false` | Remove `stale_after` instead of setting it |
| `--help` | `-h` | - | Show help |

### `okfw verify`

Usage: `okfw verify <id_or_file>`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--by` | - | `human:<login>` | Actor: `human:<id>`, `process:<id>`, or `<producer>/<version>` |
| `--at` | - | now | RFC 3339 datetime |
| `--help` | `-h` | - | Show help |

### `okfw set-generated`

Usage: `okfw set-generated <id_or_file>`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--by` | - | `human:<login>` | Actor that produced the content |
| `--help` | `-h` | - | Show help |

### `okfw add-source`

Usage: `okfw add-source <id_or_file> --resource <uri>`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--resource` | - | - | URL, bundle-relative path, or scope descriptor (**required**) |
| `--id` | - | - | Join key for `[^id]` body footnotes |
| `--title` | - | - | Human-readable label |
| `--author` | - | - | Actor who produced the source |
| `--usage-count` | - | - | How often the resource was exercised |
| `--last-modified` | - | - | When the source itself last changed (ISO 8601 datetime; a bare date means midnight UTC) |
| `--usage-window` | - | - | Datetime range framing `--usage-count`, as `FROM..TO` |
| `--help` | `-h` | - | Show help |

### `okfw set-resource`

Usage: `okfw set-resource <id_or_file> <uri>`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--help` | `-h` | - | Show help |

### `okfw migrate`

Usage: `okfw migrate [path]`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--dry-run` | - | `false` | Report what would change without writing |
| `--help` | `-h` | - | Show help |

---

## Note ID and Filename Format

Notes use a UUIDv7 `id`, for example `019f0494-00a8-7206-b788-c5bbfa2430a8`.
Legacy `YYYYMMDDHHmmss-UUID` IDs are still accepted.

Concept files are named `slug--shortid.md` (e.g. `my-note-title--019f049400a8.md`),
where the short id is the first 12 hex digits of the UUID (extended on a
collision). A concept's **OKF id is its path within the wiki minus `.md`** — so
`infra/my-note-title--019f049400a8.md` has the id
`infra/my-note-title--019f049400a8`. Links between concepts are
vanilla markdown links pointing at these paths.

---

## Frontmatter Template

```yaml
---
id: "019f0494-00a8-7206-b788-c5bbfa2430a8"
title: "Note Title"
type: "note"            # required, free-form OKF kind; defaults to the kind value
# kind: "note"          # structured kind; omitted when it equals "note"
description: "..."      # optional one-sentence OKF summary
project: "project-name" # optional
tags:
  - "tag1"
  - "tag2"
created: "YYYY-MM-DDTHH:MM:SSZ"
timestamp: "YYYY-MM-DDTHH:MM:SSZ"   # OKF last meaningful change
parent: "019f0312-4700-7def-9abc-ffffffffffff"  # optional; the parent's full id
---
```

See [Note Format](/okf-wiki/getting-started/note-format/) for the full field
reference, including the `type` vs `kind` distinction.

---

## Directory Structure

Concepts are organized by topic and project. With no `--path`, a concept is
placed in a top-level folder named after its project (or the wiki root when no
project is set); `--path <subdir>` overrides that.

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

`index.md` and `log.md` are **reserved filenames** carrying no concept
frontmatter.

---

## Environment

`okfw` automatically detects:
- **The wiki**: Searches upward from the current directory for `.okfw/` (like git finds `.git`)
- **The project root**: `okfw init` places `.okfw/` at the git repository root, or the current directory outside git
- **Working directory**: For relative paths

`project` is **never** derived from git. It is optional metadata: pass
`-p/--project` when creating a concept, or set it later with `okfw set-project`.

---

## Common Workflows

### Capture an idea quickly (no project context)
```bash
okfw create "Random thought while commuting"
```

### Capture while working on a project
```bash
cd ~/projects/my-project
okfw create "Interesting pattern in auth flow"
```

### Add project context later
```bash
okfw set-project 019f0494 my-project
```

### Build a note chain
```bash
# Create parent
okfw create "Main concept" -p my-project --json
# Note its full frontmatter id, e.g. 019f0494-00a8-7206-b788-c5bbfa2430a8

# Create child (manually add parent field to frontmatter)
okfw create "Sub-concept" -p my-project
# Edit to add: parent: "019f0494-00a8-7206-b788-c5bbfa2430a8"
# (parent must be the full id, not a path or prefix)
```

---

## Makefile Targets

```bash
make build            # Build binary
make test             # Run Go tests + CUE validation
make test-race        # Go tests under the race detector
make lint             # Run go vet + staticcheck + gofmt check
make fmt              # Format Go + CUE files
make install          # Install to $GOPATH/bin
make clean            # Remove the binary
make tidy             # Run go mod tidy
make ui-check         # Lint Lua plugin
make integration-test # Test NeoVim integration
make e2e              # CLI + NeoVim suite inside a container
make docs-dev         # Run the docs site locally
make build-image      # Build the devcontainer image
make run-container    # Start the devcontainer
make stop-container   # Stop and remove the devcontainer
```
