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
- Searchable knowledge, with no index to maintain
- Works from your editor - no context switching

`okfw` brings OKF Wiki into NeoVim, so your knowledge base grows while you work.

---

## Before You Begin

**Recommended reading:** [Why OKF Wiki?](/okf-wiki/methodology/why-okf-wiki/)

Understanding the methodology will help you get more value from the tool. The key concepts are:

- **Capture** - Get ideas into notes quickly, without judging them
- **Refine** - Sharpen captures into clear, focused insights
- **Atomic notes** - One idea per note
- **Links** - Connections between notes create emergent structure

---

## User Story

> *As a developer working on multiple projects, I want to capture ideas and insights as I work, so that I can build a searchable knowledge base without leaving my editor.*

This tutorial will show you how to:
1. Capture a quick idea
2. Refine and link it
3. Search and connect your notes
4. Use templates for structured content
5. Build daily review habits

---

## Prerequisites

Ensure you have:
- NeoVim 0.9+ with the okfw plugin installed
- The `okfw` CLI binary in your PATH
- An initialized wiki: run `:OkfwInit` (or `okfw init` from a shell) once at
  your project root to create the `.okfw/` directory

The plugin discovers the wiki via `okfw root` from NeoVim's current working
directory, so open NeoVim anywhere inside your project.

Verify your setup:
```vim
:OkfwTemplate
```

You should see a picker with available templates (the picker needs
[snacks.nvim](https://github.com/folke/snacks.nvim)).

---

## Part 1: Basic Workflow (Without Templates)

### Step 1: Capture a Note

You're reading code and notice an interesting pattern. Capture it immediately.

**From NeoVim:**
```vim
:OkfwNote
```

Or using the Lua API:
```vim
:lua require("okfw").create_note()
```

You'll be prompted for a title:
```
Note Title: Interesting error handling pattern in auth module
```

The plugin creates the note and opens it in the current window. Files are named `slug--shortid.md`, where the short id is the first 12 hex digits of the concept's UUID:
```
.okfw/interesting-error-handling-pattern-in-auth-module--019f049400a8.md
```

**What just happened:**
- A new markdown file was created with a UUIDv7 `id` and a `slug--shortid.md` filename
- YAML frontmatter was generated with metadata
- No project context required - just capture the thought

### Step 2: View Your Note

The new note is already open. To find it again later:
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
tags: []
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

### Step 4: Search Your Notes

There is nothing to index: search reads the files directly, so the note is
searchable as soon as it is saved. Saving also refreshes the wiki's `index.md`
listings. If you edited concepts outside NeoVim, regenerate the listings with:
```vim
:OkfwIndex
```

### Step 5: Refine the Note

A week later, you've used this pattern successfully. Time to refine the note: review the content, sharpen the writing, and add any context you were missing at capture time.

You can also set a project with `\p` (`\` is NeoVim's default `<localleader>`;
the mapping exists on plain notes, not daily notes or issues):
```
\p  →  Project: _
```

The note's frontmatter is updated:
```yaml
project: "my-project"
```

**Why refine?**
- Refined notes represent validated knowledge
- Project context makes them easy to find later
- They're the building blocks of your knowledge base

### Step 6: Link Notes Together

Later, you write another note about Go best practices. Link back to your error handling note.

Links are **vanilla markdown links** that point at a concept's file path (a concept's OKF id is its path minus `.md`). While editing, press `\l` to open the link picker. Find the error handling note, press `<CR>`, and a link is inserted. It uses a wiki-relative absolute path — a leading `/` means the wiki root — with the file name as the link text, which you can then reword:
```markdown
See also [interesting-error-handling-pattern-in-auth-module--019f049400a8](/interesting-error-handling-pattern-in-auth-module--019f049400a8.md) for the pattern.
```

Or press `\L` to use the concept title as the link text:
```markdown
See [Interesting error handling pattern in auth module](/interesting-error-handling-pattern-in-auth-module--019f049400a8.md).
```

You can also write relative links by hand:
```markdown
See [the pattern](./interesting-error-handling-pattern-in-auth-module--019f049400a8.md).
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

`meeting`, `book-review`, `snippet`, `project-idea`, `user-story`, `feature`, `daily`, `issue`

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

Filled in (abridged), it reads:

```markdown
---
id: "019f04b8-7e9a-7012-3456-789abcdef012"
title: "Add password reset flow"
type: "note"
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

- [User authentication epic](/auth/user-authentication-epic--019f03124700.md)
```

### Example: Code Snippet

You found a useful snippet you want to remember:

```vim
:OkfwTemplate snippet
```

Title: `Go context with timeout pattern`

The template includes sections for:
- Language and source
- Context: why the snippet is useful
- The code itself, an explanation, and a usage example
- Related notes and citations

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

`:OkfwSearch` supports flags for filtering by frontmatter fields:

```vim
" Filter by project
:OkfwSearch --project my-project

" Filter by structured kind (note/daily-note/issue)
:OkfwSearch --kind issue
:OkfwSearch --kind daily-note

" Filter by the free-form OKF type field
:OkfwSearch --type note

" Filter by OKF lifecycle status
:OkfwSearch --status draft
:OkfwSearch --status deprecated

" Find concepts whose stale_after instant has been reached
:OkfwSearch --stale

" Combine flags with a text query
:OkfwSearch --project my-project --kind issue authentication

" Combine flags with live search
:OkfwSearch! --kind issue

" Filter by tag
:OkfwSearch --tag golang --tag api
```

Flags support tab completion — press `<Tab>` after typing a flag name to see valid values.

### Graph Visualization

See how your notes connect:
```vim
:OkfwGraph 20
```

This generates a Mermaid flowchart of up to 20 notes and their relationships, opened in a scratch buffer.

---

## Part 4: The Refine Workflow

The core workflow is:

```
Capture -> Review -> Refine -> Link
```

### When to Refine

Refine a captured note when:
- You've validated the idea through use
- The content needs sharpening for clarity
- It connects meaningfully to other notes
- It's worth keeping long-term

### How to Refine

1. Open the note
2. Review and sharpen the content
3. Add links to related notes with `\l`
4. Set the project context with `\p` (if not already set)
5. Add tags with `\a`

---

## Recommended Keymaps

Add these to your NeoVim configuration:

```lua
-- Daily notes
vim.keymap.set("n", "<leader>zd", "<cmd>OkfwDaily<cr>", { desc = "Today's daily" })
vim.keymap.set("n", "<leader>zD", "<cmd>OkfwDaily yesterday<cr>", { desc = "Yesterday's daily" })

-- Note creation
vim.keymap.set("n", "<leader>zf", function()
    require("okfw").create_note()
end, { desc = "New note" })

vim.keymap.set("n", "<leader>zt", "<cmd>OkfwTemplate<cr>", { desc = "New from template" })

-- Search
vim.keymap.set("n", "<leader>zz", "<cmd>OkfwSearch<cr>", { desc = "Search notes" })
vim.keymap.set("n", "<leader>z/", "<cmd>OkfwSearch!<cr>", { desc = "Live search" })

-- Graph
vim.keymap.set("n", "<leader>zg", "<cmd>OkfwGraph<cr>", { desc = "Graph" })

-- Management
vim.keymap.set("n", "<leader>zi", "<cmd>OkfwIndex<cr>", { desc = "Refresh index.md listings" })
```

---

## Quick Reference

| Action | Command | Keymap Suggestion |
|--------|---------|-------------------|
| New note | `:OkfwNote` | `<leader>zf` |
| New from template | `:OkfwTemplate` | `<leader>zt` |
| Today's daily | `:OkfwDaily` | `<leader>zd` |
| Search notes | `:OkfwSearch` | `<leader>zz` |
| Live search | `:OkfwSearch!` | `<leader>z/` |
| Insert link | `\l` / `\L` (buffer-local) | Auto-mapped on concepts |
| Toggle backlinks | `\b` (buffer-local) | Auto-mapped on concepts |
| Set project | `\p` (buffer-local) | Auto-mapped on plain notes |
| Add tags | `\a` (buffer-local) | Auto-mapped on all concepts |
| Validate frontmatter | `\v` (buffer-local) | Auto-mapped on all concepts |
| Generate graph | `:OkfwGraph` | `<leader>zg` |
| Refresh `index.md` listings | `:OkfwIndex` | `<leader>zi` |

---

## Next Steps

### Build a Daily Habit

Daily notes are the foundation of a sustainable practice. They provide:
- A consistent place to capture thoughts
- A natural review rhythm
- A timeline of your thinking

**Read:** [Daily Notes Workflow](/okf-wiki/neovim/daily-notes-workflow/) for a comprehensive guide to daily, weekly, and monthly review workflows in NeoVim.

### Keep Knowledge Current with OKF Metadata

okfw is a knowledge format, not a task tracker — there are no todo fields. What
it does track is whether a concept can still be trusted:
- Lifecycle status (`draft`, `stable`, `deprecated`)
- A staleness date, so review becomes a query rather than a memory exercise
- A verification trail, and provenance for where the knowledge came from

**Read:** [CLI Commands](/okf-wiki/reference/cli-commands/#okf-metadata) for the OKF metadata commands.

### Master the CLI

The NeoVim plugin wraps the `okfw` CLI. For scripting, automation, or terminal use:

**Read:** [CLI Commands](/okf-wiki/reference/cli-commands/) for the complete command reference.

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

1. **Capture** - Create notes for quick ideas
2. **Refine** - Add content, tags, and project context
3. **Link** - Connect validated insights to related notes
4. **Search** - Find and connect your knowledge
5. **Template** - Use structured formats for consistency

The key is consistency. Capture freely, review regularly, and link deliberately. Your knowledge base will grow organically as you work.

---

*"The best time to plant a tree was 20 years ago. The second best time is now."*

Start with one note today.
