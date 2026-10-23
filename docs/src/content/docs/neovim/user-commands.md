---
title: User Commands
description: Complete reference for all :Okfw user commands available in NeoVim.
---

The plugin registers user commands that you can run from NeoVim's command line (`:OkfwCommand`). All commands support tab completion for flags and values.

The plugin discovers the wiki by running `okfw root` from NeoVim's current
working directory, which searches upward for a `.okfw/` directory the way git
finds `.git`.

## Setup

### `:OkfwInit`

Initialize an OKF wiki for the current project. Runs `okfw init` in NeoVim's
current working directory, creating `.okfw/` at the git repository root (or
the cwd outside git). Running it again is a no-op.

```vim
:OkfwInit
```

---

## Note Creation

### `:OkfwNote`

Create a new concept. You are prompted for the title, and the new file opens
once it is written.

```vim
" Create a note
:OkfwNote

" Create a note with a project
:OkfwNote --project my-project
```

**Flags:**

| Flag | Values | Description |
|------|--------|-------------|
| `--project` | any string | Optional project metadata |

### `:OkfwTemplate`

Create a note from a template, prompting for the title. With no template name
it opens a picker (snacks.nvim), or prompts for a name without one; `--project`
applies either way.

```vim
" Open template picker
:OkfwTemplate

" Pick a template for a project
:OkfwTemplate --project my-project

" Create meeting notes
:OkfwTemplate meeting

" Create a feature spec for a project
:OkfwTemplate feature --project my-project
```

**Available templates:** `meeting`, `book-review`, `snippet`, `project-idea`, `user-story`, `feature`, `daily`, `issue`

---

## Search

### `:OkfwSearch`

Search concepts. Opens a [snacks.nvim](https://github.com/folke/snacks.nvim)
picker if available, otherwise prints results; every filter below applies either
way. Use the bang (`!`) variant for live search that updates as you type,
starting from any query you give.

```vim
" Search for a term
:OkfwSearch authentication

" Live search (updates as you type)
:OkfwSearch!

" Live search starting from a query
:OkfwSearch! auth

" Live search within a kind
:OkfwSearch! --kind issue

" Filter by project
:OkfwSearch --project my-project

" Filter by structured kind and status
:OkfwSearch --kind issue --status draft

" Find stale concepts
:OkfwSearch --stale

" Filter by tag
:OkfwSearch --tag security

" Combine a query with filters
:OkfwSearch auth --project my-project --tag security
```

**Flags:**

| Flag | Values | Description |
|------|--------|-------------|
| `--project` | any string | Filter by project |
| `--kind` | `note`, `daily-note`, `issue` | Filter by structured kind |
| `--type` | any string | Filter by the free-form OKF `type` field |
| `--status` | `draft`, `stable`, `deprecated` | Filter by OKF lifecycle status |
| `--stale` | - | Only concepts whose `stale_after` has passed |
| `--tag` | any string | Filter by tag (can repeat) |

Hits worth noticing before you open them are badged after the title:
`(reviewed)` for a human-reviewed concept, `(draft)` or `(deprecated)`, and
`(stale)` once `stale_after` has passed. An unverified, stable, current concept
carries no badge.

**Picker keymaps (when available):**

| Key | Action |
|-----|--------|
| `<CR>` | Open note |
| `<C-p>` | Preview in floating window |
| `<C-l>` | Insert a markdown link at cursor (`[file-name](/dir/file-name.md)`) |
| `<C-S-l>` | Insert a markdown link using the concept title as link text |

---

## Graph Visualization

### `:OkfwGraph`

Render note relationships as a [Mermaid](https://mermaid.js.org/) flowchart
(`okfw graph`). Opens in a scratch buffer (filetype `mermaid`); press `q` to
close it. With no `--start`, the graph is centered on the concept in the current
buffer.

```vim
" Default graph (limit 10 nodes)
:OkfwGraph

" Limit number of nodes
:OkfwGraph --limit 20
:OkfwGraph 20

" Start from a specific note
:OkfwGraph --start 019f0494

" Set traversal depth
:OkfwGraph --depth 3

" Combine options
:OkfwGraph --start 019f0494 --depth 3 --limit 50
```

**Flags:**

| Flag | Values | Description |
|------|--------|-------------|
| `--limit` | number | Maximum nodes to display (default: 10) |
| `--start` | concept reference | Start node: full id, unique id prefix (≥ 8 hex digits), or OKF concept ID |
| `--depth` | number | Traversal depth (default: unlimited) |

A bare number is the same as `--limit`.

### `:OkfwExport`

Export a portable subset of the wiki (`okfw export`): the same graph query as
`:OkfwGraph`, with each concept copied into `<wiki>/ephemeral/` as `<id>.md`
and the links between exported concepts rewritten to those files, so the set
reads in any markdown viewer. Links to concepts outside the set are left as
they are. `ephemeral/` is wiped on every export and gitignored. A notification
reports how many files were written.

```vim
" Export up to 10 concepts around the current one
:OkfwExport

" Export 20, at most 2 hops from a given concept
:OkfwExport --start 019f0494 --depth 2 --limit 20
```

Takes the same flags as `:OkfwGraph` (`--limit`, `--start`, `--depth`, or a bare
number for the limit).

---

## Daily Notes

### `:OkfwDaily`

Open or create a daily note. Daily notes are idempotent — running it twice on the same day opens the same file.

```vim
" Today's daily note
:OkfwDaily

" Yesterday's note (for morning review)
:OkfwDaily yesterday

" Specific date
:OkfwDaily 2026-02-10
```

### `:OkfwDailyList`

Browse daily notes in a picker (requires snacks.nvim): the last 14 days. Use
the bang variant to show only this week (from Sunday).

```vim
" Browse the last 14 days of daily notes
:OkfwDailyList

" This week only
:OkfwDailyList!
```

---

## Maintenance

### `:OkfwIndex`

Regenerate the wiki's OKF `index.md` directory listings by running
`okfw index-md`. Search and the graph read the files directly, so the listings
are the only derived state there is to refresh — and saving a concept buffer
already refreshes them (`okfw index-md --no-adopt`). Run it by hand after
editing concepts outside NeoVim.

```vim
" The wiki discovered from the cwd
:OkfwIndex

" Another project's wiki
:OkfwIndex ../other-project
```

### `:OkfwRefreshTags`

Refresh the cached tag list used by tag completion.

---

## Deleting and Moving

Use these rather than shelling out to `rm` / `mv`: they keep the `index.md`
listings in step, and `:OkfwMove` repairs links.

### `:OkfwDelete`

Delete a concept. With no argument it acts on the current buffer; otherwise
name it by file path, id or OKF concept ID, as with the CLI.

```vim
" Delete the concept you are editing
:OkfwDelete

" ...or one by path
:OkfwDelete .okfw/notes/scratch--019f0494.md

" ...or by id
:OkfwDelete 019f0494
```

Asks for confirmation first, defaulting to **No**. On success it removes the
concept's file and its `index.md` line, then wipes the now-orphaned buffer.

Concepts that still link to the deleted one are named in a warning but left
untouched — OKF requires consumers to tolerate a link whose target does not
exist, so a dangling link is not an error to be fixed by editing another note.

### `:OkfwMove`

Move or rename a concept. With one argument it moves the current buffer's
concept; with two, the first is the source, named by file path, id or OKF
concept ID.

```vim
" Move the concept you are editing into a directory
:OkfwMove archive/

" Rename it
:OkfwMove notes/better-name.md

" Move a specific concept, by path or by id
:OkfwMove .okfw/notes/scratch--019f0494.md archive/
:OkfwMove 019f0494 archive/

" Omit the destination to be prompted for one
:OkfwMove
```

The destination is relative to the wiki root, whatever the cwd, and follows
`mv`: a path ending in `.md` is the new full path, anything else is a directory
to move into. The concept keeps its `id`, so
`parent` hierarchies survive, and the buffer follows the concept to its new path
so you can carry on editing.

Links are repaired at **both** ends — inbound links in other concepts, and the
moved concept's own relative links, which resolve against its directory and so
break the moment it changes directory. The notification reports how many
concepts were rewritten.

---

## Quick Reference

| Command | Description |
|---------|-------------|
| `:OkfwInit` | Initialize a `.okfw/` wiki for the current project |
| `:OkfwNote [--project NAME]` | Create new concept |
| `:OkfwTemplate [name] [--project NAME]` | Create from template |
| `:OkfwSearch[!] [flags] [query]` | Search concepts (! for live search) |
| `:OkfwGraph [flags]` | Graph visualization |
| `:OkfwExport [flags]` | Export a portable subset to `ephemeral/` |
| `:OkfwDaily [date]` | Open daily note |
| `:OkfwDailyList[!]` | Browse daily notes (! for this week) |
| `:OkfwDelete [concept]` | Delete a concept (asks first) and its `index.md` entry |
| `:OkfwMove [concept] <dest>` | Move/rename a concept, repairing links |
| `:OkfwIndex [path]` | Regenerate the `index.md` listings (`okfw index-md`) |
| `:OkfwRefreshTags` | Refresh tag cache |
