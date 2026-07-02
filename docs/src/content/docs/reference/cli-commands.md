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
directory outside git), writes a `.okfw/.gitignore` covering generated
artifacts (`.index/`, `.graph.db`, `.todos/`, `ephemeral/`), and generates the
OKF root `index.md` (with the `okf_version` declaration). Running `okfw init`
again is a no-op that prints `OKF wiki already initialized at <path>`.

### Verify the wiki connection
```bash
okfw connect
```

Verifies the discovered wiki: prints `Connected: <path>` plus a summary line
(`concepts: N | search index: yes/no | graph db: yes/no`). Exits non-zero with
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

### Validate a concept's frontmatter
```bash
okfw validate path/to/note.md
```

Validates the concept's YAML frontmatter against the CUE schema. Reports any missing or invalid fields.

### Validate whole-wiki OKF conformance
```bash
okfw validate --okf                 # Validate the discovered wiki
okfw validate --okf path/to/.okfw/  # Validate an explicit wiki path
```

Checks the entire wiki for Open Knowledge Format conformance: every concept
must have a non-empty `type` field. Broken links are reported as warnings (they
do not fail validation).

---

## Indexing

### Index a single file
```bash
okfw index path/to/note.md
```

### Index all notes in a directory
```bash
okfw index .okfw/
okfw index .
```

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
through five consolidated tools — `search`, `create`, `read`, `update`,
`graph` — instead of one tool per CLI command. Concept bodies are writable
through the tools (`create` takes `body`; `update` supports `append` and
`set_body`), so filesystem-less MCP clients get full read/write access. Tool responses are compact
JSON with wiki-relative paths, keeping agent token usage low (the full tool
schema payload is under 3 KB). The wiki is discovered from the server's
working directory, so launch it from inside the project.

Client registration (Claude Code):

```bash
claude mcp add okfw -- okfw mcp
```

### Generate `index.md` directory listings
```bash
okfw index-md                    # Discovered wiki
okfw index-md path/to/.okfw/     # Explicit path
```

Generates OKF `index.md` files (directory listings for progressive disclosure)
in each directory of the wiki. The root `index.md` carries the wiki's
`okf_version: "0.1"`; all other index files have no frontmatter, and entries
include each concept's description.

You rarely need to run this by hand: creating or updating a note keeps every
`index.md` fresh automatically — including a new `index.md` for any
subdirectory a `--path`/`--project` note just created. `index.md` files are
owned and overwritten by okfw; hand edits will not survive.

### Generate `log.md` from git history
```bash
okfw log                         # Discovered wiki
okfw log path/to/.okfw/          # Explicit path
```

Generates an OKF `log.md` update history from the project's git log, with
date-grouped Creation / Update / Deprecation entries.

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
okfw search --kind todo
okfw search --kind daily-note
okfw search --kind issue
```

`--kind` filters the structured `kind` field (`note`/`todo`/`daily-note`/`issue`)
that drives todo/daily/issue behavior.

### Filter by OKF type
```bash
okfw search --type note
okfw search -t book
```

`--type` filters the free-form OKF `type` field. (Previously `--type` filtered
note/todo/daily-note/issue — that behavior is now `--kind`.)

### Filter by status and priority
```bash
okfw search --status open
okfw search --priority high
okfw search --kind todo --status open --priority high
```

### Filter by due date
```bash
okfw search --due-before 2026-03-01
okfw search --due-after 2026-02-01
okfw search --due-after 2026-02-01 --due-before 2026-02-28  # Date range
```

### Limit results
```bash
okfw search --limit 10
okfw search -l 50
```

### JSON output (for scripting/tooling)
```bash
okfw search --json
okfw search "query" --json | jq '.[] | .title'
```

### Combined filters
```bash
okfw search "authentication" --project my-project --tag security
okfw search --kind todo --status open --project my-project --priority high
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
okfw graph . --start infra/my-first-idea--019f0494
```

### Control tree depth
```bash
okfw graph . --depth 3
```

### Sync graph database before querying
```bash
okfw graph --sync
```

The graph command outputs Mermaid flowchart syntax to stdout. When a graph database (`.okfw/.graph.db`) exists, it uses indexed data for faster queries. Use `--sync` to force a fresh sync before querying.

---

## Graph Database

The graph database (`.okfw/.graph.db`) enables O(1) indexed backlink lookups and fast traversals. It is optional — all commands fall back to filesystem scanning when the database doesn't exist.

### Sync the graph database
```bash
okfw graph-sync                    # Sync the discovered wiki
okfw graph-sync path/to/.okfw/     # Sync from explicit path
okfw graph-sync --validate         # Sync + validate edge integrity
```

Output shows added, updated, deleted counts. With `--validate`, reports dangling edges (links to non-existent notes) and structurally invalid edges.

### Query backlinks (indexed)
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
- `todo` - Actionable task with status tracking
- `issue` - Issue tracking like GitHub (bug, enhancement, question)

---

## Backlinks

Find all notes that link to a specific concept.

### Find backlinks by ID
```bash
okfw backlinks 202602131045
```

When given an id, `okfw backlinks` (like `validate`, `set-status`, and
`add-tags`) searches the whole wiki, so a concept is found regardless of which
folder it lives in.

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

Daily notes are stored inside the wiki at `.okfw/daily/YYYY/MM/YYYY-MM-DD--<shortid>.md`.

See [Daily Notes Workflow](/okf-wiki/neovim/daily-notes-workflow/) for a comprehensive guide to daily note workflows.

---

## Todo Management

### Create a todo
```bash
okfw todo "Fix login bug"
okfw todo "Update docs" --project my-project
```

### Create with due date and priority
```bash
okfw todo "Critical fix" --due 2026-02-20 --priority high
okfw todo "Nice to have" --priority low
```

### List todos
```bash
okfw todos                        # List open todos
okfw todos --project my-project   # Filter by project
okfw todos --overdue              # Overdue todos
okfw todos --today                # Due today
okfw todos --this-week            # Due this week
okfw todos --closed               # Show closed todos
okfw todos --json                 # JSON output
```

### Manage todo status
```bash
okfw set-status 202602131045 closed       # Mark as closed
okfw set-status 202602131045 in_progress  # Set to in progress
okfw set-status 202602131045 open         # Reopen a closed todo
```

### Generate todo list markdown
```bash
okfw todo-list                    # Generate todo list
okfw todo-list --project my-project
okfw todo-list --output my-todos.md
```

See [Todo Workflow](/okf-wiki/neovim/todo-workflow/) for a comprehensive guide to todo workflows.

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
okfw index --help
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
| `--template` | - | - | Use a note template (e.g., `meeting`, `user-story`) |
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

Usage: `okfw add-tags <file> <tag1> [tag2...]`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--help` | `-h` | - | Show help |

### `okfw validate`

Usage: `okfw validate <file>` or `okfw validate --okf [path]`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--okf` | - | `false` | Validate the whole wiki for OKF conformance (every concept has non-empty `type`; broken links reported as warnings) |
| `--help` | `-h` | - | Show help |

### `okfw mcp`

Usage: `okfw mcp`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--help` | `-h` | - | Show help |

### `okfw index-md`

Usage: `okfw index-md [path]`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
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

### `okfw index`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--help` | `-h` | - | Show help |

### `okfw search`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--project` | `-p` | - | Filter by project |
| `--tag` | `-T` | - | Filter by tag (repeatable, AND logic) |
| `--kind` | - | - | Filter by structured kind (`note`/`todo`/`daily-note`/`issue`) |
| `--type` | `-t` | - | Filter by the free-form OKF `type` field |
| `--status` | - | - | Filter by status (`open`/`in_progress`/`closed`) |
| `--priority` | - | - | Filter by priority (`high`/`medium`/`low`) |
| `--due-before` | - | - | Filter todos due before date (YYYY-MM-DD) |
| `--due-after` | - | - | Filter todos due after date (YYYY-MM-DD) |
| `--limit` | `-l` | `20` | Maximum number of results |
| `--json` | - | `false` | Output as JSON |
| `--help` | `-h` | - | Show help |

### `okfw graph`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--limit` | `-l` | `10` | Maximum number of nodes to display |
| `--start` | | - | Start tree from a specific note ID |
| `--depth` | | `0` (unlimited) | Maximum tree depth |
| `--sync` | | `false` | Sync graph database before querying |
| `--help` | `-h` | - | Show help |

### `okfw graph-sync`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--validate` | | `false` | Run CUE edge validation and report integrity issues |
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

### `okfw todo`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--project` | `-p` | - | Optional project metadata (also used as the note's folder) |
| `--due` | - | - | Due date (YYYY-MM-DD format) |
| `--priority` | - | - | Priority (`high`, `medium`, `low`) |
| `--help` | `-h` | - | Show help |

### `okfw todos`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--project` | `-p` | - | Filter by project |
| `--overdue` | - | `false` | Show overdue todos |
| `--today` | - | `false` | Show todos due today |
| `--this-week` | - | `false` | Show todos due this week |
| `--closed` | - | `false` | Show closed todos |
| `--json` | - | `false` | Output as JSON |
| `--help` | `-h` | - | Show help |

### `okfw set-status`

Usage: `okfw set-status [id_or_file] [status]`

Status must be one of: `open`, `in_progress`, `closed`.

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--help` | `-h` | - | Show help |

### `okfw todo-list`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--project` | `-p` | - | Filter by project |
| `--output` | `-o` | `todos-TIMESTAMP.md` | Output filename |
| `--help` | `-h` | - | Show help |

---

## Note ID and Filename Format

Notes use a UUIDv7 `id`, for example `019f0494-00a8-7206-b788-c5bbfa2430a8`.
Legacy `YYYYMMDDHHmmss-UUID` IDs are still accepted.

Concept files are named `slug--shortid.md` (e.g. `my-note-title--019f0494.md`),
where the short id is the leading hex of the UUID. A concept's **OKF id is its
path within the wiki minus `.md`** — so `infra/my-note-title--019f0494.md`
has the id `infra/my-note-title--019f0494`. Links between concepts are
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
parent: "infra/some-parent--019f0312"  # optional
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
│   └── my-first-idea--019f0494.md
├── daily/                         # Daily notes, grouped by year/month
│   └── 2026/
│       └── 06/
│           └── 2026-06-26--019f04a1.md
├── any-other-folder/              # Concepts may live in any directory you choose
│   └── refined-concept--019f04b8.md
├── ephemeral/                     # Export scratch dir (gitignored)
├── .index/                        # Bleve search index (auto-managed)
├── .graph.db                      # Persistent graph database (auto-managed)
├── .todos/                        # Generated todo lists
└── .gitignore                     # Ignores .index/, .graph.db, .todos/, ephemeral/
```

`index.md` and `log.md` are **reserved filenames** carrying no concept
frontmatter.

---

## Environment

`okfw` automatically detects:
- **The wiki**: Searches upward from the current directory for `.okfw/` (like git finds `.git`)
- **Git project**: Uses repo name as `project` field
- **Working directory**: For relative paths

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
okfw set-project .okfw/infra/202602131045.md my-project
```

### Build a note chain
```bash
# Create parent
okfw create "Main concept" -p my-project
# Note its id (its path minus .md, e.g. my-project/main-concept--019f0494)

# Create child (manually add parent field to frontmatter)
okfw create "Sub-concept" -p my-project
# Edit to add: parent: "my-project/main-concept--019f0494"
```

---

## Makefile Targets

```bash
make build            # Build binary
make test             # Run Go tests + CUE validation
make lint             # Run go vet + staticcheck
make fmt              # Format Go + CUE files
make install          # Install to $GOPATH/bin
make clean            # Remove binary and index
make tidy             # Run go mod tidy
make ui-check         # Lint Lua plugin
make integration-test # Test NeoVim integration
```
