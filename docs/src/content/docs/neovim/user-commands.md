---
title: User Commands
description: Complete reference for all :Okfw user commands available in NeoVim.
---

The plugin registers user commands that you can run from NeoVim's command line (`:OkfwCommand`). All commands support tab completion for flags and values.

The plugin finds the wiki with `okfw root`, from the current working directory
of NeoVim. `okfw root` looks for a `.okfw/` directory there and in each parent
directory, as git does to find `.git`.

## Setup

### `:OkfwInit`

Initialize an OKF wiki for the current project. This command runs `okfw init`
in the current working directory of NeoVim. It creates `.okfw/` at the root of
the git repository, or in the cwd outside git. A second run changes nothing.

```vim
:OkfwInit
```

---

## Note Creation

### `:OkfwNote`

Create a new concept. The command asks for the title. Then it writes the new
file and opens it.

```vim
" Create a note
:OkfwNote

" Create a note in a wiki subdirectory
:OkfwNote --path research/auth
```

**Flags:**

| Flag | Values | Description |
|------|--------|-------------|
| `--path` | a wiki-relative directory | Place the concept there (created if missing; default: the wiki root) |

:::note[Deprecated: `--project`]
It still works, but gives a warning one time in each session. Organize concepts
by directory instead: `--path` places and scopes, and `:OkfwMove` refiles a
concept.
:::

### `:OkfwTemplate`

Create a note from a template. The command asks for the title. With no
template name, it opens a picker (snacks.nvim). Without snacks.nvim, it asks
for a name. `--path` applies in both cases.

```vim
" Open template picker
:OkfwTemplate

" Pick a template, placing the note in specs/
:OkfwTemplate --path specs

" Create meeting notes
:OkfwTemplate meeting

" Create a feature spec in specs/
:OkfwTemplate feature --path specs
```

**Available templates:** `meeting`, `book-review`, `snippet`, `project-idea`, `user-story`, `feature`, `daily`, `issue`

---

## Search

### `:OkfwSearch`

Search concepts. Opens a [snacks.nvim](https://github.com/folke/snacks.nvim)
picker if available, otherwise prints results; every filter below applies either
way. Use the bang (`!`) variant for a live search that updates while you type. It
starts from the query that you give, if any.

```vim
" Search for a term
:OkfwSearch authentication

" Live search (updates as you type)
:OkfwSearch!

" Live search starting from a query
:OkfwSearch! auth

" Live search within a kind
:OkfwSearch! --kind issue

" Search one directory, recursively
:OkfwSearch --path research

" Filter by structured kind and status
:OkfwSearch --kind issue --status draft

" Find stale concepts
:OkfwSearch --stale

" Filter by tag
:OkfwSearch --tag security

" Combine a query with filters
:OkfwSearch auth --path research --tag security
```

**Flags:**

| Flag | Values | Description |
|------|--------|-------------|
| `--path` | a wiki-relative directory | Search only this directory, recursively |
| `--kind` | `note`, `daily-note`, `issue` | Filter by structured kind |
| `--type` | any string | Filter by the free-form OKF `type` field |
| `--status` | `draft`, `stable`, `deprecated` | Filter by OKF lifecycle status |
| `--stale` | - | Only concepts whose `stale_after` has passed |
| `--tag` | any string | Filter by tag (can repeat) |

:::note[Deprecated: `--project`]
It still works, but gives a warning one time in each session. Organize concepts
by directory instead: `--path` places and scopes, and `:OkfwMove` refiles a
concept.
:::

Some hits get a badge after the title, so you see them before you open them.
The badges are `(reviewed)` for a human-reviewed concept, `(draft)` or
`(deprecated)`, and `(stale)` after `stale_after` has passed. An unverified, stable, current concept
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
close it. With no `--start`, the graph starts at the concept in the current
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

Export a portable subset of the wiki (`okfw export`). It uses the same graph
query as `:OkfwGraph`. It copies each concept into `<wiki>/.ephemeral/` as
`<id>.md`. It also changes the links between exported concepts to point at
those files, so you can read the set in any markdown viewer. It does not change
links to concepts outside the set. Each export first empties `.ephemeral/`, and
the `.gitignore` excludes it. A message tells how many files the export wrote.

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

Open or create a daily note. Daily notes are idempotent: when you run the command two times on the same day, it opens the same file.

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

Regenerate the OKF `index.md` directory listings of the wiki with
`okfw index-md`. Search and the graph read the files directly. So the listings
are the only derived state to refresh. When you save a concept buffer, the
plugin already refreshes them (`okfw index-md --no-adopt`). Run the command by
hand after you edit concepts outside NeoVim.

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

Use these commands, not `rm` / `mv` in a shell. They keep the `index.md`
listings correct, and `:OkfwMove` repairs links.

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

The command asks first, and the default answer is **No**. Then it removes the
file of the concept and its `index.md` line, and closes the buffer of the
deleted file.

A warning names the concepts that still link to the deleted one, but the
command does not change them. OKF requires consumers to accept a link to a
target that does not exist. So a dangling link is not an error, and you do not
have to edit another note.

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

The target path is relative to the wiki root, from any cwd, and follows the
rules of `mv`. A path that ends in `.md` is the new full path. Each other path
is a directory to move into. The concept keeps its `x-okfw-id`, so
`x-okfw-parent` hierarchies stay correct. The buffer follows the concept to its
new path, so you can continue to edit.

The command repairs links at **both** ends. It repairs the inbound links in
other concepts. It also repairs the relative links of the moved concept: they
resolve from its directory, so they break when the directory changes. A message
tells how many concepts the command changed.

---

## Quick Reference

| Command | Description |
|---------|-------------|
| `:OkfwInit` | Initialize a `.okfw/` wiki for the current project |
| `:OkfwNote [--path DIR]` | Create new concept |
| `:OkfwTemplate [name] [--path DIR]` | Create from template |
| `:OkfwSearch[!] [flags] [query]` | Search concepts (! for live search) |
| `:OkfwGraph [flags]` | Graph visualization |
| `:OkfwExport [flags]` | Export a portable subset to `.ephemeral/` |
| `:OkfwDaily [date]` | Open daily note |
| `:OkfwDailyList[!]` | Browse daily notes (! for this week) |
| `:OkfwDelete [concept]` | Delete a concept (asks first) and its `index.md` entry |
| `:OkfwMove [concept] <dest>` | Move/rename a concept, repairing links |
| `:OkfwIndex [path]` | Regenerate the `index.md` listings (`okfw index-md`) |
| `:OkfwRefreshTags` | Refresh tag cache |
