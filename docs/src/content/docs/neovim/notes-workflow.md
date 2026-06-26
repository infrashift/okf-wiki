---
title: Notes Workflow
description: A hands-on tutorial for using okfw to build a personal knowledge base directly from NeoVim.
---

A hands-on tutorial for using `okfw` to build a personal knowledge base directly from your editor.

---

## Why This Matters

You're a developer. You read documentation, solve problems, attend meetings, and have ideas throughout the day. Where does all that knowledge go?

**The problem:**
- Notes scattered across apps, files, and sticky notes
- Good ideas forgotten within days
- No way to connect related thoughts
- Context lost when switching projects

**The solution:**
- One system for capturing everything
- Notes that link to each other
- Searchable, indexed knowledge
- Works from your editor - no context switching

`okfw` brings the Zettelkasten method into NeoVim, so your knowledge base grows while you work.

---

## Before You Begin

**Recommended reading:** [Why Zettelkasten?](/zettelkasten/methodology/why-zettelkasten/)

Understanding the methodology will help you get more value from the tool. The key concepts are:

- **Untethered notes** - Quick captures, unrefined thoughts
- **Tethered notes** - Refined insights, linked to other notes
- **Atomic notes** - One idea per note
- **Links** - Connections between notes create emergent structure

---

## User Story

> *As a developer working on multiple projects, I want to capture ideas and insights as I work, so that I can build a searchable knowledge base without leaving my editor.*

This tutorial will show you how to:
1. Capture a quick idea (untethered note)
2. Refine and tether it (tethered note)
3. Search and connect your notes
4. Use templates for structured content
5. Build daily review habits

---

## Prerequisites

Ensure you have:
- NeoVim 0.9+ with the okfw plugin installed
- The `okfw` CLI binary in your PATH
- A zettelkasten directory (default: `~/okfw/`)

Verify your setup:
```vim
:OkfwTemplate
```

You should see a picker with available templates.

---

## Part 1: Basic Workflow (Without Templates)

### Step 1: Capture an Untethered Note

You're reading code and notice an interesting pattern. Capture it immediately.

**From NeoVim:**
```vim
:OkfwNote untethered
```

Or using the Lua API:
```vim
:lua require("okfw").create_note("untethered")
```

You'll be prompted for a title:
```
Note Title: Interesting error handling pattern in auth module
```

The plugin creates a new note and shows you the path. Files are named `slug--shortid.md`, where the short id is the leading hex of the concept's UUID:
```
Created: ~/okfw/error-handling-pattern--019f0494.md
```

**What just happened:**
- A new markdown file was created with a UUIDv7 `id` and a `slug--shortid.md` filename
- YAML frontmatter was generated with metadata
- The note is categorized as "untethered" (quick capture)
- No project context required - just capture the thought

### Step 2: View Your Note

Open the note to add content:
```vim
:OkfwSearch
```

Find your note in the picker and press `<CR>` to open it.

The file looks like this:
```markdown
---
id: "019f0494-00a8-7206-b788-c5bbfa2430a8"
title: "Interesting error handling pattern in auth module"
type: "note"
category: "untethered"
tags:
  - ""
created: "2026-02-13T14:23:45-06:00"
timestamp: "2026-02-13T14:23:45-06:00"
---

# Interesting error handling pattern in auth module

```

### Step 3: Add Content

Fill in your thoughts while they're fresh:

```markdown
---
id: "019f0494-00a8-7206-b788-c5bbfa2430a8"
title: "Interesting error handling pattern in auth module"
type: "note"
category: "untethered"
tags:
  - "go"
  - "error-handling"
created: "2026-02-13T14:23:45-06:00"
timestamp: "2026-02-13T14:23:45-06:00"
---

# Interesting error handling pattern in auth module

Noticed in the auth service that errors are wrapped with context
at each layer, making debugging much easier.

Instead of just returning the error, each function adds context:

    return fmt.Errorf("authenticate user %s: %w", userID, err)

This creates a chain like:
    "authenticate user 123: validate token: parse claims: invalid signature"

Worth adopting in other services.
```

Save the file (`:w`).

### Step 4: Index Your Notes

Before you can search, index your notes:
```vim
:OkfwIndex ~/okfw/
```

Or index just the current directory:
```vim
:OkfwIndex
```

### Step 5: Tether the Note

A week later, you've used this pattern successfully. Time to tether the note.

Open the untethered note, then press `\t` and select `tether`:
```
\t  →  Pick: tether / untether
```

You can also set a project with `\p`:
```
\p  →  Project: _
```

The note's frontmatter is updated:
```yaml
category: "tethered"
project: "my-project"
```

**Why tether?**
- Tethered notes represent validated knowledge
- They require project context (accountability)
- They're the building blocks of your knowledge base

### Step 6: Link Notes Together

Later, you write another note about Go best practices. Link back to your error handling note.

Links are **vanilla markdown links** that point at a concept's file path (a concept's OKF id is its path minus `.md`). While editing, press `\l` to open the link picker. Find the error handling note, press `<CR>`, and a link is inserted. By default it uses a bundle-relative absolute path — a leading `/` means the bundle root:
```markdown
See also [error wrapping](/error-handling-pattern--019f0494.md) for the pattern.
```

Or press `\L` to use the concept title as the link text:
```markdown
See [Interesting error handling pattern in auth module](/error-handling-pattern--019f0494.md).
```

You can also write relative links by hand:
```markdown
See [the pattern](./error-handling-pattern--019f0494.md).
```

Broken links are tolerated — if a target moves or is removed, the link simply doesn't resolve. To **follow a link**, place your cursor on it and press `gf` (go to file).

### Step 7: Find Backlinks

Open your error handling note and press `\b` to toggle the backlinks panel:
```
\b  →  Backlinks panel (right side)
```

A panel shows all notes that reference this one - your knowledge graph is forming.

---

## Part 2: Working with Templates

Templates provide structure for common note types. They save time and ensure consistency.

### Available Templates

Use `:OkfwTemplate` without arguments to open a picker showing all templates:

`meeting`, `book-review`, `snippet`, `project-idea`, `user-story`, `feature`, `daily`, `todo`, `issue`

### Example: Meeting Notes

You're about to join a sprint planning meeting.

```vim
:OkfwTemplate meeting
```

Enter the title:
```
Note Title: Sprint 14 Planning
```

The note opens with a pre-filled structure:

```markdown
---
id: "019f04a1-2b3c-7456-8def-0123456789ab"
title: "Sprint 14 Planning"
type: "note"
category: "untethered"
tags:
  - "meeting"
created: "2026-02-13T15:00:00-06:00"
timestamp: "2026-02-13T15:00:00-06:00"
---

# Sprint 14 Planning

**Date:** 2026-02-13
**Attendees:**
-

## Agenda

1.

## Discussion



## Action Items

- [ ]

## Next Steps

```

Fill it in during the meeting. You have a consistent format every time.

### Example: User Story

Product wants a new feature. Capture it properly:

```vim
:OkfwTemplate user-story
```

Title: `Add password reset flow`

```markdown
---
id: "019f04b8-7e9a-7012-3456-789abcdef012"
title: "Add password reset flow"
type: "note"
category: "untethered"
tags:
  - "user-story"
  - "requirements"
created: "2026-02-13T15:30:00-06:00"
timestamp: "2026-02-13T15:30:00-06:00"
---

# Add password reset flow

## User Story

**As a** registered user
**I want** to reset my password via email
**So that** I can regain access if I forget my credentials

## Acceptance Criteria

- [ ] Given a valid email, When I request reset, Then I receive an email within 5 minutes
- [ ] Given an invalid token, When I try to reset, Then I see an error message
- [ ] Given a valid token, When I set a new password, Then I can log in immediately

## Priority

- [x] Must Have
- [ ] Should Have
- [ ] Could Have
- [ ] Won't Have (this time)

## Technical Details

- Use existing email service
- Tokens expire after 1 hour
- Rate limit: 3 requests per hour per email

## Related Stories

- [User authentication epic](/auth/user-authentication-epic--019f0312.md)
```

### Example: Code Snippet

You found a useful snippet you want to remember:

```vim
:OkfwTemplate snippet
```

Title: `Go context with timeout pattern`

The template includes sections for:
- The code itself
- Language and context
- When to use it
- Related snippets

### Using the Template Picker

Don't remember template names? Use the picker:

```vim
:OkfwTemplate
```

The picker shows all templates with descriptions. Select one and continue.

---

## Part 3: Search and Discovery

### Full-Text Search

Find notes containing specific terms:
```vim
:OkfwSearch authentication
```

### Live Search

Search as you type:
```vim
:OkfwSearch!
```

### Search with Flags

`:OkfwSearch` supports flags for filtering by any indexed field:

```vim
" Filter by category or project
:OkfwSearch --category tethered
:OkfwSearch --project my-project

" Filter by structured kind (note/todo/daily-note/issue)
:OkfwSearch --kind todo
:OkfwSearch --kind daily-note

" Filter by the free-form OKF type field
:OkfwSearch --type note

" Filter by status and priority (useful for todos)
:OkfwSearch --kind todo --status open
:OkfwSearch --kind todo --priority high

" Filter by due date
:OkfwSearch --due-before 2026-03-01
:OkfwSearch --due-after 2026-02-01

" Combine flags with a text query
:OkfwSearch --project my-project --kind todo authentication

" Combine flags with live search
:OkfwSearch! --kind todo

" Filter by tag
:OkfwSearch --tag golang --tag api
```

Flags support tab completion — press `<Tab>` after typing a flag name to see valid values.

### Graph Visualization

See how your notes connect:
```vim
:OkfwGraph 20
```

This generates an ASCII tree showing up to 20 notes and their relationships.

---

## Part 4: The Tether Workflow

The core Zettelkasten workflow is:

```
Capture (untethered) -> Review -> Refine -> Tether (tethered)
```

### When to Tether

Tether an untethered note when:
- You've validated the idea through use
- You've refined the content for clarity
- It connects meaningfully to other notes
- It's worth keeping long-term

### How to Tether

1. Open the untethered note
2. Review and refine the content
3. Add links to related notes
4. Set the project context with `\p` (if not already set)
5. Press `\t` and select `tether`

### How to Untether

If a tethered note no longer warrants its status, you can revert it:

1. Open the tethered note
2. Press `\t` and select `untether`

The note's `category` changes back from `"tethered"` to `"untethered"`.

### What Changes When Tethering

- `category` changes from `"untethered"` to `"tethered"`
- `project` is set (required for tethered notes)
- The file stays in place (no movement required)

---

## Recommended Keymaps

Add these to your NeoVim configuration:

```lua
-- Daily notes
vim.keymap.set("n", "<leader>zd", "<cmd>OkfwDaily<cr>", { desc = "Today's daily" })
vim.keymap.set("n", "<leader>zD", "<cmd>OkfwDaily yesterday<cr>", { desc = "Yesterday's daily" })

-- Note creation
vim.keymap.set("n", "<leader>zf", function()
    require("okfw").create_note("untethered")
end, { desc = "New untethered note" })

vim.keymap.set("n", "<leader>zt", "<cmd>OkfwTemplate<cr>", { desc = "New from template" })

-- Search
vim.keymap.set("n", "<leader>zz", "<cmd>OkfwSearch<cr>", { desc = "Search notes" })
vim.keymap.set("n", "<leader>z/", "<cmd>OkfwSearch!<cr>", { desc = "Live search" })

-- Graph
vim.keymap.set("n", "<leader>zg", "<cmd>OkfwGraph<cr>", { desc = "Graph" })

-- Management
vim.keymap.set("n", "<leader>zi", "<cmd>OkfwIndex<cr>", { desc = "Index notes" })
```

---

## Quick Reference

| Action | Command | Keymap Suggestion |
|--------|---------|-------------------|
| New untethered note | `:OkfwNote untethered` | `<leader>zf` |
| New from template | `:OkfwTemplate` | `<leader>zt` |
| Today's daily | `:OkfwDaily` | `<leader>zd` |
| Search notes | `:OkfwSearch` | `<leader>zz` |
| Live search | `:OkfwSearch!` | `<leader>z/` |
| Insert link | `\l` / `\L` (buffer-local) | Auto-mapped on concepts |
| Toggle backlinks | `\b` (buffer-local) | Auto-mapped on concepts |
| Set project | `\p` (buffer-local) | Auto-mapped on note/todo |
| Tether / Untether | `\t` (buffer-local) | Auto-mapped on note/todo |
| Add tags | `\a` (buffer-local) | Auto-mapped on all concepts |
| Validate frontmatter | `\v` (buffer-local) | Auto-mapped on all concepts |
| Generate graph | `:OkfwGraph` | `<leader>zg` |
| Index notes | `:OkfwIndex` | `<leader>zi` |

---

## Next Steps

### Build a Daily Habit

Daily notes are the foundation of a sustainable practice. They provide:
- A consistent place to capture thoughts
- A natural review rhythm
- A timeline of your thinking

**Read:** [Daily Notes Workflow](/zettelkasten/neovim/daily-notes-workflow/) for a comprehensive guide to daily, weekly, and monthly review workflows in NeoVim.

### Manage Tasks with Todos

Todos are a special type of concept for actionable tasks. They integrate with daily notes and provide:
- Status tracking (open, in progress, closed)
- Due dates and priorities
- Links to related notes

**Read:** [Todo Workflow](/zettelkasten/neovim/todo-workflow/) for the complete todo workflow guide.

### Master the CLI

The NeoVim plugin wraps the `okfw` CLI. For scripting, automation, or terminal use:

**Read:** [CLI Commands](/zettelkasten/reference/cli-commands/) for the complete command reference.

### Get Help

Full documentation is available in NeoVim:
```vim
:help okfw
```

CLI help:
```bash
okfw --help
okfw create --help
okfw daily --help
```

---

## Summary

You've learned how to:

1. **Capture** - Create untethered notes for quick ideas
2. **Refine** - Add content, tags, and links
3. **Tether** - Elevate validated insights to tethered notes
4. **Untether** - Revert tethered notes when needed
5. **Search** - Find and connect your knowledge
6. **Template** - Use structured formats for consistency

The key is consistency. Capture freely, review regularly, and tether deliberately. Your knowledge base will grow organically as you work.

---

*"The best time to plant a tree was 20 years ago. The second best time is now."*

Start with one note today.
