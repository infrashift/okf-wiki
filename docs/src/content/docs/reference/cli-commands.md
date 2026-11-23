---
title: CLI Commands
description: Quick reference for all okfw CLI commands, flags, and workflows.
---

Quick reference for all `okfw` commands.

Each command looks for the wiki (a `.okfw/` directory) in the current working
directory and then in each parent directory. So commands work from any
subdirectory of the project. When a command finds no wiki, it exits non-zero
with this message:

```
no OKF wiki found (searched upward from <dir>); run 'okfw init' at your project root to create one
```

---

## Wiki Setup

### Initialize a wiki
```bash
okfw init
```

Creates a `.okfw/` directory at the root of the git repository, or in the
current directory outside git. It writes a `.okfw/.gitignore` for the only
generated directory (`.ephemeral/`). It also writes the OKF root `index.md`,
with the `okf_version` declaration. A second `okfw init` changes nothing and
prints `OKF wiki already initialized at <path>`.

### Verify the wiki connection
```bash
okfw connect
```

Verifies the wiki that it finds: prints `Connected: <path>` and a
`concepts: N` line. When it finds no wiki, it exits non-zero and tells you to
run `okfw init`.

---

## Note Creation

### Create a note
```bash
okfw create "Quick idea about X"
okfw create "My note title"
```

### Create a note with a description
```bash
okfw create "OAuth notes" --description "How our OAuth flow works"
okfw create "OAuth notes" -d "How our OAuth flow works"
```

okfw writes the `-d/--description` value to the optional OKF `description`
frontmatter field: a one-sentence summary of the concept.

### Create an issue, or set the OKF type
```bash
okfw create "Login fails" --kind issue --path issues   # kind and type "issue"
okfw create "Q3 revenue" --type Metric                 # free-form OKF type
```

`--kind issue` writes `kind: issue` (and `type: issue` unless `--type` says
otherwise) without the issue template's body, so `--body` holds the whole
issue. `--type` overrides the free-form OKF `type`, which otherwise follows the
kind.

### Place a note in a specific wiki subdirectory
```bash
okfw create "Spec draft" --path specs/auth
```

`--path <subdir>` puts the new concept at any path under the wiki root (here,
`.okfw/specs/auth/`). If the directory does not exist, okfw creates it. okfw
refuses a `--path` that goes out of the wiki or names a dot-directory
(`.hidden`, `../out`). Without `--path`, the concept goes to the wiki root.

Directories organize a wiki. Each OKF tool reads them, each directory gets an
`index.md` listing, and `okfw search --path` limits a search to one. Use tags
for topics that are in many directories.

:::note[Deprecated: `--project`]
`okfw create --project <name>` (`-p`) and `okfw search --project` still work,
but they print a warning. Use `--path` instead. They come from a time when one
global wiki held the notes of all repositories, and `project` kept them apart.
Now each repository has its own wiki. A directory does the same job, and each
OKF tool reads it. Outside okfw, `x-okfw-project` means nothing. Concepts that
already have `x-okfw-project` keep it, and search hits still show it.
:::

---

## Note Management

### Delete a concept
```bash
okfw delete ./notes/scratch--019f0494.md
okfw delete 019f0494 --force        # skip the confirmation
okfw rm 019f0494 -f                 # alias
okfw delete 019f0494 -f --json      # {"id","path","inbound":[...]}
```

Removes the file of the concept and its line in the `index.md` listings.
There is no search index or graph database to update. okfw calculates both from
the files on each call. Without `--force`, the command asks first. When stdin
is not a terminal, it does not ask. So a script without `--force` stops, and
deletes nothing.

The command **reports, but does not repair**, the concepts that still link to
the deleted one, from their body or a path-valued frontmatter field. OKF
requires consumers to accept a link to a target that does not exist. So a
dangling link is not malformed. A rewrite of the text of an author to remove
one destroys content. `okfw graph-query dangling` finds them later.

### Move or rename a concept
```bash
okfw move 019f0494 archive/                        # into a directory
okfw move ./scratch--019f0494.md notes/design.md   # new path and name
okfw rename 019f0494 notes/better-name.md          # alias
okfw move 019f0494 archive/ --json                 # {"from","to","rewrote":[...]}
```

The target path works as with `mv`. A path that ends in `.md` is the new full
path of the concept. Each other path is a directory to move into, with the
current filename. Both are bundle-relative. They cannot go out of the bundle or
name one of its internal dot-directories.

The `x-okfw-id` of the concept does not change, so hierarchies with `x-okfw-parent` stay correct.
The command repairs links at **both** ends, because a move breaks two different
sets. Here, a link is a body link, or a path-valued frontmatter field that
names a concept: `resource`, `sources[].resource`, `computation`,
`executor.resource`, `attester.resource` or `superseded_by`. okfw edits the
frontmatter in place, and keeps comments and quoting.

- okfw changes the links in other concepts from the old path to the new path.
  Each link keeps its form: a bundle-absolute link stays absolute, and a
  relative link stays relative.
- okfw rewrites the *relative* links of the moved concept, so they still resolve
  from the new directory. Bundle-absolute links need no repair. That is why OKF
  recommends them.

okfw updates the `x-okfw-timestamp` of the moved concept, because the OKF
identity of a concept is its path in the bundle. okfw does not update the
concepts that only got new link targets. Their knowledge did not change, only
the text of a link.

---

## Tags

### Add tags to a concept
```bash
okfw add-tags path/to/note.md golang
okfw add-tags path/to/note.md golang api security
```

okfw adds the tags to the `tags:` list in the YAML frontmatter of the concept. It ignores a tag that the list already has.

### Remove tags from a concept
```bash
okfw remove-tags path/to/note.md open
okfw remove-tags 019f0494 open triage
```

okfw ignores a tag that the list does not have. The `tags:` list stays, as
`tags: []` after the last tag goes, because each concept must have `tags`. Use
it to change a state tag, for example `open` to `resolved` when an issue closes.

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
value formats, and no undeclared top-level keys. It also checks citations. Two
`sources` entries cannot have the same `id`. The check ignores case, as
markdown does for footnote labels. Each `[^label]` footnote reference in the
body must match a `sources[].id` or a `[^label]:` definition. A reference with
a definition but no source is an ordinary footnote, and it passes. The check
ignores footnote syntax in code, so `` `[^a-z]` `` is not a citation. In prose,
put a regex character class in code.

### Validate every concept in the wiki
```bash
okfw validate --all                 # strict check of the discovered wiki
okfw validate --all path/to/.okfw   # ...or an explicit path
```

The same strict check of the whole bundle, with a report of each failure. It
also reports each `x-okfw-id` that more than one concept declares, with all the
files that declare it. A concept that you copy as a template keeps its id.
Exits non-zero if a concept is invalid or two concepts have the same id.

### Validate OKF conformance
```bash
okfw validate --okf                 # Validate the discovered wiki
okfw validate --okf path/to/.okfw   # Validate an explicit wiki path
```

Checks Open Knowledge Format conformance (spec §11), and nothing more. Each
non-reserved `.md` file has frontmatter that parses, with a non-empty `type`.
The reserved `index.md` and `log.md` files follow §8 and §9. The check is
permissive on purpose, so a bundle that okfw did not author can pass. It
reports broken links as warnings, because OKF requires consumers to accept
them.

### Migrate concepts from an older okfw
```bash
okfw migrate --dry-run   # report what would change
okfw migrate             # apply
```

Rewrites concepts so that they agree with the current schema. okfw removed its
task-tracking fields, because OKF uses `status` for the lifecycle, and okfw
used it for the progress of todos. This command removes `due`, `completed`,
`priority` and `kind: todo`, and rewrites a legacy `status`:

| Before | After |
|---|---|
| `status: closed` | `status: deprecated` |
| `status: open` or `status: in_progress` | removed (absent means `stable`) |

It moves okfw's own keys into the extension namespace of OKF v0.3, in each
concept that okfw wrote. `id`, `kind`, `project`, `parent`, `created` and
`timestamp` become `x-okfw-id`, `x-okfw-kind` and so on. The keys stay in
place, with the same values. The command does not change the bare `id` of a
different tool.

It also moves a bundle from OKF v0.2 to v0.3:
- okfw v0.2 read some bare path fields from the bundle root. Those fields get
  a leading `/`, because v0.3 reads a bare path from the directory of the
  concept.
- The root `index.md` declares `okf_version: "0.3"`, and keeps its other keys.
- The export scratch directory moves from `ephemeral/` to `.ephemeral/`.

It reports these problems for you to fix: no bundle title or description,
`verified` events without an `at`, and timestamps in neither v0.3 form. Bare
dates are valid in v0.3, and the command keeps them as written. See the
[OKF v0.3 migration guide](/okf-wiki/reference/migrating-to-okf-v0-3/).

You can run it again, and a second run reports nothing to do. It does not
change the free-form `type`, because OKF does not register type values.

---

## OKF Wiki

Commands that keep a wiki conformant with the Open Knowledge Format (OKF).

### Print the wiki root
```bash
okfw root
```

Prints the `.okfw` path that it finds in the current directory or a parent
directory. Scripts and the Neovim plugin use it. When there is no wiki, it
exits with an error.

### Serve the wiki to AI agents (MCP)
```bash
okfw mcp                         # MCP stdio server (run from the project)
```

Runs a Model Context Protocol server on stdin/stdout. It gives the wiki through
five consolidated tools, not one tool for each CLI command. Their names are
`okfw_search`, `okfw_create`, `okfw_read`, `okfw_update` and `okfw_graph`, and
the bare names also work. The tools can write concept bodies: `create` takes
`body`, and `update` has `append` and `set_body`. So MCP clients without a
filesystem get full read and write access. Tool responses are compact JSON with
wiki-relative paths, so agents use few tokens. The full `tools/list` payload is
about 5 KB. On each call, the server finds the wiki from its working directory,
so start it in the project.

**Rules on connect.** The `initialize` result has `instructions`, about 1 KB,
sent one time. They tell the agent to search before it derives an answer, and
to browse listings from the top. They also tell it to write one idea for each
concept, with a description and a link. `status` is the lifecycle, and the agent must never
edit `index.md`. The instructions end with the top-level directories of the
wiki, so the agent knows which listing to read first. Each MCP client gets
them, also a client that `okfw install` did not set up.

**Progressive disclosure.** `okfw_read` takes a directory as well as a
concept. `{"ref":"/"}` returns the root `index.md`. `{"ref":"issues/"}` returns
the listing of that directory, with the description of each concept.
`issues/index.md` does the same, and so does a bare `issues` when no concept
has that name. `section`, `offset` and `limit` work on a listing as on a
concept. A ref cannot go out of the bundle through `..` or a symlink, and it
cannot name a dot-directory.

**One write for each concept.** `okfw_create` sets the kind, type, lifecycle,
stale date, resource and author in the write that creates the concept. So an
issue with its metadata is one call, not five. okfw checks each value before it
writes. A refused create leaves nothing.

| Tool | Parameters | Bounds |
|------|------------|--------|
| `okfw_search` | `q`, `tag`, `tags` (all must match), `dir`, `kind`, `type`, `status`, `stale`, `limit`, `offset` | 10 hits by default, at most 100 per page; `truncated` and `next_offset` say there are more |
| `okfw_create` | `kind` (`note`/`issue`/`daily`), `title`, `dir`, `tags`, `description`, `body`, `template`, `date`, and for a note or issue `type`, `status`, `stale_after`, `resource`, `generated_by` | Returns `{id,path}` (a daily note returns `{path}`) |
| `okfw_read` | `ref` (required: a concept, or a directory such as `/` or `issues/`), `section`, `offset`, `limit` | 200 lines by default; narrow with `section` or page with `offset`/`limit` |
| `okfw_update` | `ref`, `action`, `value` (all required; an empty `value` on `verify`/`set_generated` means `human:<login>`) | Actions: `set_lifecycle`, `set_stale_after`, `add_tags`, `remove_tags`, `append`, `set_body`, `verify`, `set_generated`, `set_resource`, `add_source` — the same writers as the CLI |
| `okfw_graph` | `action` (`backlinks`/`cluster`/`tag`/`dangling`), `ref`, `depth`, `limit` | 10 nodes by default; `truncated` means more exist |

Every search hit carries `trust`, plus `status` and `stale` when they are the
exception (see [How far to trust a hit](#how-far-to-trust-a-hit)).

To register the client (Claude Code):

```bash
claude mcp add okfw -- okfw mcp               # all five tools
claude mcp add okfw -- okfw mcp --read-only   # search, read and graph only
```

`--read-only` is for an agent that only looks things up. The model gets the
tool definitions and instructions on each turn of a session. In a benchmark of
agents that answer questions about OKF bundles, they were 60–80% of okfw's
input tokens. The two write tools are almost half of those definitions. So a
read-only server cuts the fixed cost of each turn by about 42%, from 6.4 KB to
3.7 KB. It refuses `okfw_create` and `okfw_update`, and its instructions omit
the rules for writes.

### Install the agent skill
```bash
okfw install                     # Claude Code: skill + CLAUDE.md block
okfw install --platform gemini   # GEMINI.md block
okfw install --platform codex    # AGENTS.md block
okfw install --force             # overwrite an existing skill file
```

Registers okfw with an AI agent, so that it captures and links project
knowledge in the wiki. okfw manages a marker block, so a second install changes
nothing. The block has a namespace, so okfw does not change the rest of the
file, or the blocks of other tools. Run it from any directory in the project.

### Generate `index.md` directory listings
```bash
okfw index-md                    # Discovered wiki
okfw index-md path/to/.okfw/     # Explicit path
okfw index-md --no-adopt         # Refresh only listings okfw already maintains
```

Generates OKF `index.md` files (directory listings for progressive disclosure)
in each directory of the wiki. The root `index.md` carries the wiki's
`okf_version: "0.3"`; all other index files have no frontmatter, and entries
include each concept's description.

You seldom need to run this by hand. When you create or update a note, okfw
keeps each `index.md` current. That includes a new `index.md` for a new
subdirectory from `--path`. okfw marks the listings that it generates with an
`<!-- okfw:generated ... -->` comment, and maintains only those. It replaces
hand edits to a marked listing. `okfw index-md` also takes over an unmarked
`index.md` in a directory with okfw concepts. That is how okfw maintains the
listings from before the marker. It never deletes one. `--no-adopt` does not
take over listings, and refreshes only marked listings. Each okfw write does
the same.

For a directory of up to 50 concepts, the listing names each concept. Above
that, the listing puts the concepts in groups by `type`. Each group gives its
count, names up to 10 concepts, and gives the `okfw search --type <type>` that
returns the rest.

### Generate `log.md` from git history
```bash
okfw log                         # Discovered wiki
okfw log path/to/.okfw/          # Explicit path
```

Generates an OKF `log.md` update history from the git log of the project. The
history has a group for each date, newest first. It has `Creation`, `Update`, `Move`
and `Removal` entries, with the subject of each commit. To give a directory in
the bundle its own `log.md`, give that directory as the argument.

---

## Searching

### Full-text search
```bash
okfw search "authentication patterns"
okfw search "golang error handling"
```

### Filter by tags
```bash
okfw search --tag golang
okfw search --tag golang --tag api  # AND (must have both)
okfw search -T golang -T testing
```

### Search one directory
```bash
okfw search --path issues --tag open      # issues/ and its subdirectories
okfw search "login" --path research/auth
```

`--path` scopes the search to a bundle subdirectory, recursively — the same
directories `okfw create --path` writes to. The walk starts there, so a scoped
search costs the directory, not the bundle.

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

Concepts with no `status` are `stable`, and `--status stable` returns them.

### Find stale concepts
```bash
okfw search --stale
```

Returns concepts whose `stale_after` instant is now or in the past. Set it with
`okfw set-stale-after`.

### Limit and page results
```bash
okfw search --limit 10
okfw search -l 50
okfw search "auth" --limit 10 --offset 10   # second page
```

The ranking puts the title first, then tags, then the description, then the
body. Search reads the files on each call, so there is no index to build or
keep in sync.

### How far to trust a hit
Every hit says how far to trust it, so a stale or never-reviewed concept no
longer looks like the reviewed, current answer:

```
1. Ledger reconciliation
   ID: 019f0494-...
   Trust: human-reviewed | draft | stale since 2026-06-30T00:00:00Z
```

`Trust` is the OKF trust tier (`unverified`, `machine-confirmed` or
`human-reviewed`, from `verified`). The lifecycle status follows only when it is
`draft` or `deprecated`, and `stale since` only once `stale_after` has passed.

### JSON output (for scripting/tooling)
```bash
okfw search --json
okfw search "query" --json | jq '.[] | .title'
okfw search --json | jq '.[] | select(.trust == "human-reviewed" and (.stale | not)) | .title'
```

`--json` prints a compact JSON array of hits, the same hit the MCP
`okfw_search` tool returns:

```json
[{"id":"019f0494-…","title":"Pricing","path":"infra/pricing--019f049400a8.md","kind":"note","type":"note","tags":["pricing"],"snippet":"…","trust":"human-reviewed"}]
```

- `path` is relative to the wiki (`.okfw/`). Each command that takes a
  concept accepts it.
- Each hit has `trust`. A hit has `status` only when it is not `stable`, and
  `stale` only when it is `true`.
- A deprecated hit has `superseded_by`. With `--current`, the hit that takes
  the place of a deprecated one has `replaces`.
- The output omits empty fields.

Its shape is for agents, which pay for each token. Before v2026.11.2, it had
indents, an absolute `file_path`, a `score`, the `description` next to the
snippet, and empty defaults on each hit. That output was about two times as
large for the same results.

### Combined filters
```bash
okfw search "authentication" --path research --tag security
okfw search --kind issue --status draft --path issues
```

---

## Graph Visualization

### Generate a graph from the wiki or a directory
```bash
okfw graph                       # the discovered wiki root
okfw graph .okfw/infra/          # only the concepts in one directory
```

okfw resolves a `path` argument from the current directory, as for other file
paths, not from the wiki root.

### Limit the number of nodes
```bash
okfw graph --limit 20
okfw graph -l 5
```

### Start from a specific note
```bash
okfw graph --start 019f0494                           # id prefix
okfw graph --start infra/my-first-idea--019f049400a8  # or OKF concept ID
```

Without `--start`, the graph starts at the concept with the most links, and
shows its neighbourhood first. After that, it continues from the concept with
the most links that it did not show yet. So with a `--limit` that is high
enough, it reaches each concept, also concepts with no links. When `--limit`
omits concepts, the summary line tells how many, and how to see them.

### Control traversal depth
```bash
okfw graph --start 019f0494 --depth 3
```

### Render a plain-text tree
```bash
okfw graph --ascii
```

By default, `okfw graph` writes **Mermaid flowchart syntax** to stdout, which
you can paste into markdown. `--ascii` writes a plain-text tree that you can
read in a terminal. A summary line goes to stderr, so piped output stays clean.
The line gives `N nodes, M edges`, and how many concepts `--limit` omitted.
okfw builds the graph from the concepts on disk each time, so it always
agrees with them.

---

## Export

### Export a portable subset of the wiki
```bash
okfw export                              # Export from the wiki root
okfw export path/to/dir                  # Export a specific directory
okfw export --start 019f0494 --depth 2   # Export a neighbourhood
okfw export --limit 50
okfw export --limit 1000                 # Every concept, linked or not
```

Writes a self-contained copy of up to `--limit` concepts into
`.okfw/.ephemeral/`, one file for each concept, with the name `<id>.md`. okfw
changes the links to agree: an in-bundle link becomes `<id>.md`. The export
takes concepts in the same order as `okfw graph`: the neighbourhood with the
most links first, then the rest, with or without links. The summary tells how
many concepts `--limit` omitted. Use it to give a part of the wiki to a tool
that cannot read the bundle. The `.gitignore` excludes `.ephemeral/`, and you
can safely delete it.

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

An edge is a body link to another concept. A path-valued frontmatter field
that names a concept is also an edge: `resource`, `sources[].resource`,
`computation`, `executor.resource`, `attester.resource` and `superseded_by`
(OKF §6.2). Body
links and these fields follow one rule (OKF §6). A path that starts with `/`
resolves from the wiki root. Each other path, also a bare `policies/refunds.md`,
resolves from the directory of the concept. A bundle can declare OKF v0.2, or no
version. In such a bundle, when a bare path names no file from the directory,
okfw also tries it from the wiki root. These are not edges: a URL, a scope descriptor
(`all queries in project X`), and a file that is not markdown (an attester's
`.py`).

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

Reports links to `.md` files that do not exist, usually after `okfw delete`.
It also reports `parent` references to ids that no concept declares. The
report names a link target that does not exist by its OKF concept ID (its path
without `.md`). `okfw graph-query backlinks <that path>` lists all the
concepts that still link to it. A link to a file that exists, but is not a
concept, is not dangling.

`--json` writes the node records of the graph, with Go field names (`ID`,
`Title`, `FilePath`, `Project`, `Parent`, `Children`, `Links`, `Tags`).
`cluster` puts them in `{"nodes":[...],"edges":[{"From","To","Label"}],"total":N}`.
It adds `"truncated":true` when `--limit` held back some of the `total`
concepts in reach. Then the text output ends with
`… N more in reach — raise --limit`. `dangling` writes edges,
`[{"From","To","Label"}]`, where `To` is the target that does not exist. An
empty result is `null`.

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
- `snippet` - Code snippet with its context and what it does
- `project-idea` - Project idea with goals and next steps
- `user-story` - User story in standard format with acceptance criteria
- `feature` - Feature spec with requirements and design notes
- `daily` - Daily note for thoughts, tasks, and reflections
- `issue` - An issue as on GitHub (bug, feature, question)

---

## Backlinks

Find all notes that link to a specific concept.

### Find backlinks by ID
```bash
okfw backlinks 202602131045
```

With an id, `okfw backlinks` looks through the whole wiki, as each command that
takes a concept does. So it finds a concept in any directory. You can name a
concept by its full `id`, or by a unique id prefix of 8 or more hex digits
(`019f0494`). You can also use its OKF concept ID
(`infra/my-note--019f049400a8`, with or without `.md` or a leading `/`), or a
file path. okfw refuses an `id` that more than one concept declares, and gives
the paths of all copies. It does not choose the first copy that it finds. Give
the path of the concept that you want, and see `okfw validate --all`.

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

Create and manage daily notes for thoughts, tasks, and reviews.

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

okfw keeps daily notes in the wiki at `.okfw/daily/YYYY/MM/YYYY-MM-DD-<weekday>--<shortid>.md`,
for example `daily/2026/06/2026-06-26-friday--019f04a10000.md`.

See [Daily Notes Workflow](/okf-wiki/neovim/daily-notes-workflow/) for a comprehensive guide to daily note workflows.

---

## Browsing the wiki as a site

```bash
okfw site up               # build, then serve in the background on :4321
okfw site down             # stop the server and free the port
okfw site build --out ./site   # build only, and copy the static site
okfw site status           # bun, the build directory, the server
```

`okfw site` turns the wiki into a static HTML site with full-text search. It
uses Astro and Starlight. The [site page](/okf-wiki/getting-started/site/)
tells what the pages show and how okfw builds them. The command needs bun 1.2
or later. okfw checks for bun before it does anything, and no other part of
okfw needs bun.

---

## OKF Metadata

okfw implements the lifecycle, trust and provenance families of OKF v0.3, and
its bundle metadata. Each has a writer, so you never have to edit YAML by hand
to set a field.

### Bundle metadata
```bash
okfw init --title Billing --description "Runbooks and decisions for billing." --license CC-BY-4.0
okfw set-bundle-meta --description "Runbooks and decisions for billing."
```

The bundle-root `index.md` holds `okf_version`, `title`, `description` and
`license` (OKF §8.1). okfw keeps that frontmatter unchanged when it regenerates
the listing. `okfw validate --all` requires a title and a description. An
agent reads the description to choose a bundle, and the MCP server quotes both
to each agent.

### Lifecycle
```bash
okfw set-lifecycle 019f0494 deprecated --superseded-by metrics/gross-margin.md
okfw set-lifecycle 019f0494 draft                 # draft | stable | deprecated
okfw set-stale-after 019f0494 2026-12-31T00:00:00Z
okfw set-stale-after 019f0494 2026-12-31          # stored as 2026-12-31T00:00:00Z
okfw set-stale-after 019f0494 --clear
okfw search "gross margin" --current              # skip deprecated; follow superseded_by
```

An absent `status` means `stable`. `--superseded-by` names the concept that
replaces a deprecated one, as a bundle-absolute path. When you set a different
status, okfw removes it. A concept is stale at `stale_after` and after it.
`stale_after` is an absolute instant, not a TTL. A value that okfw cannot parse
counts as stale. OKF accepts a date-time with an offset, or a date (the start
of that day in UTC). okfw stores the date-time form.

### Trust
```bash
okfw verify 019f0494                              # defaults to human:<your login>
okfw verify 019f0494 --by process:nightly-audit
okfw verify 019f0494 --by human:ryan --at 2026-07-20T10:00:00Z
okfw set-generated 019f0494 --by process:finance-nightly
okfw set-generated 019f0494 --by agent/1 --at 2026-07-01T00:00:00Z
```

`verify` adds an entry to the `verified` list of the concept. Consumers
calculate a trust tier from the **current** entries: those at or after
`generated.at`. No entries means `unverified`. Only non-human actors means
`machine-confirmed`. A `human:` actor means `human-reviewed`. When a person
runs `verify` by hand, a human verified the concept. That is why the default
actor is a `human:` actor.

`set-generated` records who produced the content and when: `generated.by`, and
`generated.at`. The time is now, unless `--at` gives a different time. This
says that someone produced the content again, so earlier reviews stop to
count. okfw itself moves `generated.at` only for a body rewrite through okfw.
Tag, lifecycle, source and verify edits change only okfw's
`x-okfw-timestamp`.

On both commands, `--by` must be `<scheme>:<id>`, or `<producer>/<version>`.
The scheme is `human:`, `team:`, `process:` or a different lowercase scheme.
okfw refuses a scheme that is almost `human:` (`Human:`, `humna:`). The tier
uses only the exact scheme `human:`. A misspelt scheme records a human review
as machine-confirmed, and nothing tells you.

### Provenance
```bash
okfw add-source 019f0494 --resource https://example.com/schema --id ga4-schema
okfw add-source 019f0494 --resource "all queries in project X" \
    --usage-count 412 --usage-window 2026-01-01T00:00:00Z..2026-06-30T00:00:00Z
```

Each source needs `--resource`. It names a thing that a consumer can follow,
or a scope descriptor that it cannot follow. `--id` is the join key for
`[^id]` body footnotes, so a specific claim can cite a specific source. okfw
refuses an `--id` that the `sources` of the concept already use, in any letter
case. Add the source before the claim that cites it. okfw refuses an edit that
leaves a `[^id]` that matches nothing.

### The described asset
```bash
okfw set-resource 019f0494 bigquery://project.dataset.orders
```

Sets OKF's `resource` field: a URI of the asset that this concept describes.
Concepts about abstract ideas do not have it.

See [Note Format](/okf-wiki/getting-started/note-format/) for the full field
reference.

:::note
okfw has **no task tracking**. The project removed `okfw todo`, `todos`,
`todo-list`, `set-status` and the `todo` kind. OKF uses `status` for the
lifecycle, and okfw is a knowledge format, not a task tracker. Run
`okfw migrate` to clean up concepts from before that change.
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

### Print the version
```bash
okfw --version
okfw -v
```

Prints `okfw version <version>`. Releases are CalVer tags, `vYYYY.M.N`
(`v2026.10.0`). A build from a later commit reports `<tag>-<n>-g<commit>`. A
build from a repository with no tags reports the short commit. To check a
downloaded release, see
[Verify a release](/okf-wiki/reference/verifying-releases/).

---

## Flags Reference

The [CLI Flags](/okf-wiki/reference/cli-flags/) page gives the flags of each
command, with their values and defaults. okfw makes that page from its command
tree, as it does `okfw <command> --help` and the man pages. So the page always
agrees with the binary.

---

## Note ID and Filename Format

Notes use a UUIDv7 `id`, for example `019f0494-00a8-7206-b788-c5bbfa2430a8`.
Legacy `YYYYMMDDHHmmss-UUID` IDs are still accepted.

A concept file has the name `slug--shortid.md`, for example
`my-note-title--019f049400a8.md`. The short id is the first 12 hex digits of
the UUID. When two short ids are the same, okfw makes the short id longer. The
**OKF id of a concept is its path in the wiki, without `.md`**. So
`infra/my-note-title--019f049400a8.md` has the id
`infra/my-note-title--019f049400a8`. Links between concepts are vanilla
markdown links to these paths.

---

## Frontmatter Template

```yaml
---
x-okfw-id: "019f0494-00a8-7206-b788-c5bbfa2430a8"
title: "Note Title"
type: "note"            # required, free-form OKF kind; defaults to the kind value
# kind: "note"          # structured kind; omitted when it equals "note"
description: "..."      # optional one-sentence OKF summary
tags:
  - "tag1"
  - "tag2"
x-okfw-created: "YYYY-MM-DDTHH:MM:SS+HH:MM"     # ISO 8601 with an explicit offset
x-okfw-timestamp: "YYYY-MM-DDTHH:MM:SS+HH:MM"   # okfw's last write (not generated.at)
x-okfw-parent: "019f0312-4700-7def-9abc-ffffffffffff"  # optional; the parent's full id
---
```

The [note format](/okf-wiki/getting-started/note-format/) page describes all
fields, and the difference between `type` and `kind`.

---

## Directory Structure

Directories organize the concepts. Without `--path`, a concept goes to the
wiki root. `--path <subdir>` puts it in a subdirectory.

```
.okfw/                             # Wiki root (at the project root)
├── index.md                       # Directory listing (reserved; okf_version)
├── log.md                         # Update history (reserved)
├── infra/                         # A topic directory (create --path infra)
│   ├── index.md
│   └── my-first-idea--019f049400a8.md
├── daily/                         # Daily notes, grouped by year/month
│   └── 2026/
│       └── 06/
│           └── 2026-06-26-friday--019f04a10000.md
├── any-other-folder/              # Concepts may live in any directory you choose
│   └── refined-concept--019f04b80000.md
├── .ephemeral/                     # Export scratch dir (gitignored)
└── .gitignore                     # Ignores .ephemeral/
```

okfw reserves the filenames `index.md` and `log.md`. They carry no concept
frontmatter.

---

## Environment

`okfw` automatically detects:
- **The wiki**: Searches upward from the current directory for `.okfw/` (like git finds `.git`)
- **The project root**: `okfw init` places `.okfw/` at the git repository root, or the current directory outside git
- **Working directory**: For relative paths

---

## Common Workflows

### Capture an idea quickly
```bash
okfw create "Random thought while commuting"
```

### Capture while working on a project
```bash
cd ~/projects/my-project
okfw create "Interesting pattern in auth flow"
```

### File it in a directory later
```bash
okfw move 019f0494 auth/
```

### Build a note chain
```bash
# Create parent
okfw create "Main concept" --path auth --json
# Note its full frontmatter id, e.g. 019f0494-00a8-7206-b788-c5bbfa2430a8

# Create child (manually add parent field to frontmatter)
okfw create "Sub-concept" --path auth
# Edit to add: parent: "019f0494-00a8-7206-b788-c5bbfa2430a8"
# (parent must be the full id, not a path or prefix)
```

---

## Makefile Targets

```bash
make build            # Build bin/okfw (pure Go, CGO_ENABLED=0)
make build-linux-amd64 # Build bin/okfw-linux-amd64
make build-linux-arm64 # Build bin/okfw-linux-arm64
make dist             # Release tarballs, SBOMs and checksums in dist/ (needs syft)
make signing-key      # Generate a cosign key pair under ~/.config/cosign/
make sign             # make dist, then sign everything in dist/ with cosign
make verify-signatures # Check the signatures in dist/
make test             # Run Go tests + CUE validation
make test-race        # Go tests under the race detector
make lint             # Run go vet + staticcheck + gofmt check
make fmt              # Format Go + CUE files
make install          # Install to $GOPATH/bin
make clean            # Remove bin/ and dist/
make tidy             # Run go mod tidy
make ui-check         # Lint Lua plugin
make integration-test # Test NeoVim integration
make e2e              # CLI + NeoVim suite inside a container
make docs-dev-up      # Start the docs dev server in the background
make docs-dev-down    # Stop it and free the port
make docs-build       # Build the docs site (search works on the build)
make build-image      # Build the devcontainer image
make run-container    # Start the devcontainer
make stop-container   # Stop and remove the devcontainer
```
