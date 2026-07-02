---
title: Todo Workflow
description: A comprehensive guide to managing todos with okfw directly from NeoVim.
---

A comprehensive guide to managing todos with `okfw` directly from your editor. Todos are a special type of concept designed for actionable tasks with status tracking, due dates, priorities, and links to other notes.

Your project needs an initialized wiki: run `:OkfwInit` (or `okfw init` from a shell) once at the project root to create the `.okfw/` directory. The plugin discovers the wiki via `okfw root` from NeoVim's current working directory. Generated todo lists land in `.okfw/.todos/` (gitignored).

---

## What Are Todos?

Todos in `okfw` are not just checkboxes - they're full concepts with:
- **Status tracking** - Open, in progress, or closed
- **Due dates** - Target completion dates
- **Priority levels** - High, medium, or low
- **Links** - Connections to other concepts, including daily notes
- **Full-text search** - Todos are indexed and searchable
- **Rich content** - Descriptions, acceptance criteria, notes

**Key characteristics:**
- **Kind: todo** - A distinct structured kind for actionable tasks (`kind: "todo"`)
- **Linkable** - Can link to any other concept (notes, daily notes, other todos)
- **Queryable** - Filter by status, priority, due date, project
- **Persistent** - Never lost, always searchable, even when closed

---

## Why Todos as Concepts?

### Traditional Task Managers

| Feature | Task Managers | okfw Todos |
|---------|--------------|----------|
| Rich descriptions | Limited | Full markdown |
| Links to knowledge | None | Native linking |
| Searchable history | Varies | Always indexed |
| Context preservation | Lost on completion | Permanent record |
| Integration | Separate app | In your editor |

### The okfw Advantage

1. **Context lives with the task** - Add notes, links, and details directly
2. **Tasks link to knowledge** - Connect todos to related notes
3. **History is preserved** - Closed todos remain searchable
4. **No context switching** - Manage tasks in your editor

---

## The Todo Structure

`okfw` creates todos with this frontmatter:

```markdown
---
id: "019f0494-00a8-7206-b788-c5bbfa2430a8"
title: "Fix authentication bug"
type: "todo"
kind: "todo"
project: "my-project"
status: "open"
due: "2026-02-20"
priority: "high"
tags:
  - "todo"
  - "bug"
created: "2026-02-13T10:45:00Z"
timestamp: "2026-02-13T10:45:00Z"
---

# Fix authentication bug

## Description

Users are getting logged out randomly after 10 minutes.

## Acceptance Criteria

- [ ] Identify root cause
- [ ] Write failing test
- [ ] Implement fix
- [ ] Verify in staging

## Notes

Might be related to session timeout configuration.

## Related

- [Session Management Architecture](/auth/session-management-architecture--019f0312.md)
```

A todo carries both `kind: "todo"` (the structured kind that drives status/due/priority behavior) and `type: "todo"` (the required, free-form OKF kind, which defaults to the `kind` value). Links such as the one under `## Related` are vanilla markdown links pointing at a concept's file path.

### Status Values

| Status | Meaning | Icon |
|--------|---------|------|
| `open` | Not started | `[ ]` |
| `in_progress` | Being worked on | `[~]` |
| `closed` | Completed | `[x]` |

### Priority Values

| Priority | When to Use |
|----------|-------------|
| `high` | Urgent, blocking other work |
| `medium` | Important but not urgent |
| `low` | Nice to have, do when possible |

---

## NeoVim Commands

### Creating Todos

```vim
" Basic todo
:OkfwTodo Fix the login bug

" With due date
:OkfwTodo Update documentation --due 2026-02-20

" With priority
:OkfwTodo Critical security fix --priority high

" With project
:OkfwTodo Refactor auth module --project my-project

" Multiple options
:OkfwTodo Fix auth --due 2026-02-20 --priority high
```

### Managing Todo Status

Use the `\s` keymap (buffer-local on todo concepts) to open a status picker:

```
\s  →  Pick: open / in_progress / closed
```

This calls `okfw set-status` under the hood.

### Generating Todo Lists

```vim
" Generate markdown todo list
:OkfwTodoList

" Project-specific list
:OkfwTodoList my-project

" Due today
:OkfwTodoList today

" Due this week
:OkfwTodoList week
```

---

## Lua API

### Creating Todos

```lua
-- Basic todo
require("okfw").todo({ title = "Fix the bug" })

-- With options
require("okfw").todo({
    title = "Update documentation",
    due = "2026-02-20",
    priority = "high",
    project = "my-project",
})

-- Linked to specific concepts
require("okfw").todo({
    title = "Follow up",
    links = { "202602131045", "202602131100" },
})
```

### Managing Status

```lua
-- Set status on current buffer's todo
require("okfw").set_status("closed")
require("okfw").set_status("in_progress")
require("okfw").set_status("open")

-- Set status on a specific file
require("okfw").set_status("closed", "./path/to/todo.md")
```

### Browsing Todos

```lua
-- Open todos (picker)
require("okfw").todo_picker()

-- Closed todos
require("okfw").todo_picker({ closed = true })

-- Filtered
require("okfw").todo_picker({
    project = "my-project",
    priority = "high",
})

-- Due this week
require("okfw").todo_picker({ this_week = true })

-- Overdue
require("okfw").todo_picker({ overdue = true })

-- Get todos synchronously
local todos = require("okfw").todos_sync({ this_week = true })
```

### Generating Lists

```lua
require("okfw").todo_list()
require("okfw").todo_list({ project = "my-project" })
require("okfw").todo_list({ today = true })
```

---

## Daily Workflow

### Morning Review

1. **Open today's daily note:**
   ```vim
   :OkfwDaily
   ```

2. **Review and plan** from your daily note — create todos for tasks.

### Throughout the Day

**Capture task from idea:**
```vim
:OkfwTodo Investigate the memory leak
```

**Start working on a task:**
Open the todo, review context, optionally mark as in_progress.

**Complete a task:**
Press `\s` and select `closed` from the picker.

### End of Day

1. **Update due dates if needed** (edit the frontmatter)

2. **Generate tomorrow's list:**
   ```vim
   :OkfwTodoList today
   ```

---

## Linking Strategies

### Link Todos via Lua API

```lua
require("okfw").todo({
    title = "Update auth",
    links = { "202602101200", "202602111430" },
})
```

### Find Backlinks to Todos

To see what notes link TO a todo, press `\b` to toggle the backlinks panel.

---

## Recommended Keymaps

```lua
-- Todo management
vim.keymap.set("n", "<leader>zt", "<cmd>OkfwTodo<cr>", { desc = "New todo" })
-- \s is automatically mapped on todo concepts (status picker)
```

---

## Integration with Daily Notes

Todos and daily notes work together naturally:

### Capture Pattern

```
Daily Note (capture thoughts)
    |
Identify actionable item
    |
Create todo (:OkfwTodo)
    |
Todo tracks the task
```

### Review Pattern

```
Morning: Open daily note (:OkfwDaily)
    |
Check linked todos from yesterday
    |
Plan the day
```

### The Todo Lifecycle

```
Created (open)
    |
Working (in_progress) [optional]
    |
Completed (closed)
    |
Still searchable (permanent record)
```

---

## Generated Todo Lists

The `:OkfwTodoList` command generates a markdown file you can review or share:

```markdown
# Todo List

Generated: 2026-02-13 15:30

## High Priority

- [ ] **Fix authentication bug** [link](/my-project/fix-authentication-bug--019f0494.md)
  - Due: 2026-02-15
  - Project: my-project

## Medium Priority

- [ ] **Update documentation** [link](/update-documentation--019f04a1.md)
  - Due: 2026-02-20

## Other

- [ ] **Refactor tests** [link](/refactor-tests--019f04b8.md)

---

Total: 3 todos
```

### Generated List Location

---

## Quick Reference

| Action | Command / Keymap | Description |
|--------|-----------------|-------------|
| New todo | `:OkfwTodo [title]` | Create a new todo |
| Set status | `\s` (buffer-local) | Pick open / in_progress / closed |
| Add tags | `\a` (buffer-local) | Prompt for tags and add to frontmatter |
| Validate frontmatter | `\v` (buffer-local) | Validate frontmatter against CUE schema |
| Generate list | `:OkfwTodoList` | Markdown summary of open todos |

---

## Tips for Success

1. **Set due dates** - Helps prioritize and enables filtering
2. **Use priorities sparingly** - Reserve "high" for truly urgent items
3. **Don't delete, close** - Closed todos are a record of accomplishment
4. **Add descriptions** - Your future self will thank you
5. **Link related notes** - Build context around tasks
6. **Review regularly** - Use `:OkfwTodoList` to stay on track

---

## Related Documentation

- [Notes Workflow](/okf-wiki/neovim/notes-workflow/) - General note-taking workflow
- [Daily Notes Workflow](/okf-wiki/neovim/daily-notes-workflow/) - Daily notes workflow
- [CLI Commands](/okf-wiki/reference/cli-commands/) - Complete CLI reference
