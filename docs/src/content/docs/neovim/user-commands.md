---
title: User Commands
description: Complete reference for all :Okfw user commands available in NeoVim.
---

The plugin registers user commands that you can run from NeoVim's command line (`:OkfwCommand`). All commands support tab completion for flags and values.

## Note Creation

### `:OkfwNote`

Create a new concept. Defaults to untethered if no category is given.

```vim
" Create an untethered note (default)
:OkfwNote

" Create an untethered note (explicit)
:OkfwNote --category untethered

" Create a tethered note with a project
:OkfwNote --category tethered --project my-project
```

**Flags:**

| Flag | Values | Description |
|------|--------|-------------|
| `--category` | `untethered`, `tethered` | Note category (default: untethered) |
| `--project` | any string | Project name (auto-detected from git if omitted) |

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

" Filter by category and project
:OkfwSearch --category tethered --project my-project

" Filter by structured kind and status
:OkfwSearch --kind todo --status open

" Filter by priority and due date
:OkfwSearch --priority high --due-before 2026-03-01

" Filter by tag
:OkfwSearch --tag security

" Combine a query with filters
:OkfwSearch auth --category tethered --tag security
```

**Flags:**

| Flag | Values | Description |
|------|--------|-------------|
| `--category` | `untethered`, `tethered` | Filter by category |
| `--project` | any string | Filter by project |
| `--kind` | `note`, `todo`, `daily-note`, `issue` | Filter by structured kind |
| `--type` | any string | Filter by the free-form OKF `type` field |
| `--status` | `open`, `in_progress`, `closed` | Filter by status |
| `--priority` | `high`, `medium`, `low` | Filter by priority |
| `--due-before` | `YYYY-MM-DD` | Due before date |
| `--due-after` | `YYYY-MM-DD` | Due after date |
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

## Todos

### `:OkfwTodo`

Create a new todo. The title is everything that isn't a flag.

```vim
" Simple todo
:OkfwTodo Buy a notebook for handwritten concept drafts

" With due date and priority
:OkfwTodo Review meeting notes --due 2026-02-21 --priority high

" With project
:OkfwTodo Fix login bug --project my-project --priority medium
```

**Flags:**

| Flag | Values | Description |
|------|--------|-------------|
| `--due` | `YYYY-MM-DD` | Due date |
| `--priority` | `high`, `medium`, `low` | Priority level |
| `--project` | any string | Project name |

### `:OkfwTodoList`

Generate a markdown summary of todos and open it in a split.

```vim
" All open todos
:OkfwTodoList

" Due today
:OkfwTodoList today

" Due this week
:OkfwTodoList week

" Project-specific list
:OkfwTodoList my-project
```

---

## Index & Tags

### `:OkfwIndex`

Index concepts for full-text search. Indexes the current directory by default.

```vim
" Index current directory
:OkfwIndex

" Index specific path
:OkfwIndex ~/okfw/
```

### `:OkfwRefreshTags`

Clear the tag cache and reload tags from the index.

```vim
:OkfwRefreshTags
```

---

## Quick Reference

| Command | Description |
|---------|-------------|
| `:OkfwNote` | Create new concept |
| `:OkfwTemplate [name]` | Create from template |
| `:OkfwSearch[!] [query]` | Search concepts (! for live search) |
| `:OkfwGraph` | Graph visualization |
| `:OkfwDaily [date]` | Open daily note |
| `:OkfwDailyList[!]` | Browse daily notes (! for this week) |
| `:OkfwTodo [title]` | Create a todo |
| `:OkfwTodoList [filter]` | Generate todo list markdown |
| `:OkfwIndex [path]` | Index concepts |
| `:OkfwRefreshTags` | Refresh tag cache |
