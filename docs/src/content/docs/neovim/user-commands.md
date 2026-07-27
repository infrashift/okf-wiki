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

Create a new concept.

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

Create a note from a template. Opens a picker if no template name is given.

```vim
" Open template picker
:OkfwTemplate

" Create meeting notes
:OkfwTemplate meeting

" Create a feature spec for a project
:OkfwTemplate feature --project my-project
```

**Available templates:** `meeting`, `book-review`, `snippet`, `project-idea`, `user-story`, `feature`, `daily`

---

## Search

### `:OkfwSearch`

Search concepts. Opens a picker if available, otherwise prints results.
Use the bang (`!`) variant for live search that updates as you type.

```vim
" Search for a term
:OkfwSearch authentication

" Live search (updates as you type)
:OkfwSearch!

" Live search with an initial query
:OkfwSearch! auth

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

**Picker keymaps (when available):**

| Key | Action |
|-----|--------|
| `<CR>` | Open note |
| `<C-p>` | Preview in floating window |
| `<C-l>` | Insert a markdown link at cursor (`[title](/path/to/note.md)`) |
| `<C-S-l>` | Insert a markdown link using the concept title as link text |

---

## Graph Visualization

### `:OkfwGraph`

Generate an ASCII tree visualization of note relationships. Opens in a vertical split.

```vim
" Default graph (limit 10 nodes)
:OkfwGraph

" Limit number of nodes
:OkfwGraph --limit 20
:OkfwGraph 20

" Start from a specific note
:OkfwGraph --start 20260213143000-550e8400-e29b-41d4-a716-446655440000

" Set traversal depth
:OkfwGraph --depth 3

" Combine options
:OkfwGraph --start 20260213143000-550e8400-e29b-41d4-a716-446655440000 --depth 3 --limit 50
```

**Flags:**

| Flag | Values | Description |
|------|--------|-------------|
| `--limit` | number | Maximum nodes to display (default: 10) |
| `--start` | concept ID | Start node for the graph |
| `--depth` | number | Traversal depth |

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

Browse daily notes in a picker. Use the bang variant to show only this week.

```vim
" Browse all daily notes
:OkfwDailyList

" This week only
:OkfwDailyList!
```

---

## Deleting and Moving

Use these rather than shelling out to `rm` / `mv`: they keep the search index,
graph database, and `index.md` listings in step, and repair links.

### `:OkfwDelete`

Delete a concept. With no argument it acts on the current buffer.

```vim
" Delete the concept you are editing
:OkfwDelete

" ...or one by path
:OkfwDelete .okfw/notes/scratch--019f0494.md
```

Asks for confirmation first, defaulting to **No**. On success it removes the
concept's file, its search-index entry, its graph node, and its `index.md` line,
then wipes the now-orphaned buffer.

Concepts that still link to the deleted one are named in a warning but left
untouched — OKF requires consumers to tolerate a link whose target does not
exist, so a dangling link is not an error to be fixed by editing another note.

### `:OkfwMove`

Move or rename a concept. With one argument it moves the current buffer's
concept; with two, the first is the source.

```vim
" Move the concept you are editing into a directory
:OkfwMove archive/

" Rename it
:OkfwMove notes/better-name.md

" Move a specific concept
:OkfwMove .okfw/notes/scratch--019f0494.md archive/

" Omit the destination to be prompted for one
:OkfwMove
```

The destination follows `mv`: a path ending in `.md` is the new full path,
anything else is a directory to move into. The concept keeps its `id`, so
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
| `:OkfwNote` | Create new concept |
| `:OkfwTemplate [name]` | Create from template |
| `:OkfwSearch[!] [query]` | Search concepts (! for live search) |
| `:OkfwGraph` | Graph visualization |
| `:OkfwExport` | Export a portable subset to `ephemeral/` |
| `:OkfwDaily [date]` | Open daily note |
| `:OkfwDailyList[!]` | Browse daily notes (! for this week) |
| `:OkfwDelete [path]` | Delete a concept (asks first) and its index/graph entries |
| `:OkfwMove [src] <dest>` | Move/rename a concept, repairing links |
| `:OkfwIndex [path]` | Index concepts |
| `:OkfwRefreshTags` | Refresh tag cache |
