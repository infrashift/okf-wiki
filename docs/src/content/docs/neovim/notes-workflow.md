---
title: Notes Workflow
description: A hands-on tutorial for using okfw to build a personal knowledge base directly from NeoVim.
---

This tutorial shows how to use `okfw` to build a personal knowledge base from the editor.

---

## Why This Matters

You're a developer. You read documentation, solve problems, attend meetings, and have ideas throughout the day. Where does all that knowledge go?

**The problem:**
- Notes scattered across apps, files, and sticky notes
- Good ideas forgotten within days
- No way to connect related thoughts
- Context lost when you change to a different project

**The solution:**
- One system that captures everything
- Notes that link to each other
- Searchable knowledge, with no index to maintain
- Works from the editor, so you stay in one context

`okfw` brings OKF Wiki into NeoVim, so the knowledge base grows while you work.

---

## Before You Begin

**Read first:** [Why OKF Wiki?](/okf-wiki/methodology/why-okf-wiki/)

When you know the method, the tool is more useful. These are the important concepts:

- **Capture** - Get ideas into notes quickly, and do not judge them yet
- **Refine** - Sharpen captures into clear, focused insights
- **Atomic notes** - One idea per note
- **Links** - Connections between notes create emergent structure

---

## User Story

> *As a developer on many projects, I want to capture ideas while I work. Then I build a knowledge base that I can search, and I stay in the editor.*

This tutorial shows how to:
1. Capture a quick idea
2. Refine and link it
3. Search and connect the notes
4. Use templates for structured content
5. Build daily review habits

---

## Prerequisites

Make sure that you have:
- NeoVim 0.9+ with the okfw plugin installed
- The `okfw` CLI binary on the PATH
- A wiki. To create the `.okfw/` directory, run `:OkfwInit` (or `okfw init` in
  a shell) one time at the project root

The plugin finds the wiki with `okfw root`, from the current working directory
of NeoVim. So you can open NeoVim in any directory of the project.

Verify the setup:
```vim
:OkfwTemplate
```

A picker with the available templates opens. The picker needs
[snacks.nvim](https://github.com/folke/snacks.nvim).

---

## Part 1: Basic Workflow (Without Templates)

### Step 1: Capture a Note

You read code and see a useful pattern. Capture it immediately.

**From NeoVim:**
```vim
:OkfwNote
```

Or use the Lua API:
```vim
:lua require("okfw").create_note()
```

The plugin asks for a title:
```
Note Title: Interesting error handling pattern in auth module
```

The plugin creates the note and opens it in the current window. The file name is `slug--shortid.md`. The short id is the first 12 hex digits of the UUID of the concept:
```
.okfw/interesting-error-handling-pattern-in-auth-module--019f049400a8.md
```

**What occurred:**
- okfw created a new markdown file with a UUIDv7 `x-okfw-id` and a `slug--shortid.md` filename
- okfw wrote YAML frontmatter with metadata
- You did not choose a place for it. You only captured the thought

### Step 2: View Your Note

The new note is already open. To find it again later:
```vim
:OkfwSearch
```

Find the note in the picker and push `<CR>` to open it.

The file looks like this:
```markdown
---
x-okfw-id: "019f0494-00a8-7206-b788-c5bbfa2430a8"
title: "Interesting error handling pattern in auth module"
type: "note"
tags: []
x-okfw-created: "2026-02-13T14:23:45-06:00"
x-okfw-timestamp: "2026-02-13T14:23:45-06:00"
---

# Interesting error handling pattern in auth module

```

### Step 3: Add Content

Write down the thoughts while they are fresh:

```markdown
---
x-okfw-id: "019f0494-00a8-7206-b788-c5bbfa2430a8"
title: "Interesting error handling pattern in auth module"
type: "note"
tags:
  - "go"
  - "error-handling"
x-okfw-created: "2026-02-13T14:23:45-06:00"
x-okfw-timestamp: "2026-02-13T14:23:45-06:00"
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

There is nothing to index. Search reads the files directly, so you can find the
note immediately after you save it. A save also refreshes the `index.md`
listings of the wiki. If you edited concepts outside NeoVim, regenerate the
listings with:
```vim
:OkfwIndex
```

### Step 5: Refine the Note

A week later, the pattern worked for you. Now refine the note. Review the content, make the text clear, and add the context that it did not have at capture time.

You can also file it in a directory with `:OkfwMove`, which keeps its id and
repairs every link to it:
```vim
:OkfwMove patterns/
```

(The `\p` mapping that set a project still works, but do not use it. A
directory does the same job, and each OKF tool reads it.)

**Why refine?**
- Refined notes hold knowledge that you checked
- A directory makes them easy to find later (`:OkfwSearch --path patterns`)
- They are the blocks of the knowledge base

### Step 6: Link Notes Together

Later, you write a different note about Go best practices. Link back to the note about Go errors.

Links are **vanilla markdown links** to the file path of a concept. The OKF id of a concept is its path without `.md`. While you edit, push `\l` to open the link picker. Find the note about Go errors, and push `<CR>`. The picker inserts a link with a wiki-relative absolute path. A leading `/` means the wiki root. The link text is the file name, and you can change it:
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

okfw accepts broken links. If a target moves or goes away, the link does not resolve, and that is not an error. To **follow a link**, put the cursor on it and push `gf` (go to file).

### Step 7: Find Backlinks

Open the note about Go errors and push `\b` to toggle the backlinks panel:
```
\b  →  Backlinks panel (right side)
```

A panel shows all notes that refer to this one. The knowledge graph starts to grow.

---

## Part 2: Working with Templates

Templates give a structure to common types of notes. They save time and keep the notes consistent.

### Available Templates

Use `:OkfwTemplate` without arguments to open a picker with all templates:

`meeting`, `book-review`, `snippet`, `project-idea`, `user-story`, `feature`, `daily`, `issue`

### Example: Meeting Notes

You are about to join a sprint meeting.

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
x-okfw-id: "019f04a1-2b3c-7456-8def-0123456789ab"
title: "Sprint 14 Planning"
type: "note"
tags:
  - "meeting"
x-okfw-created: "2026-02-13T15:00:00-06:00"
x-okfw-timestamp: "2026-02-13T15:00:00-06:00"
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
x-okfw-id: "019f04b8-7e9a-7012-3456-789abcdef012"
title: "Add password reset flow"
type: "note"
tags:
  - "user-story"
  - "requirements"
x-okfw-created: "2026-02-13T15:30:00-06:00"
x-okfw-timestamp: "2026-02-13T15:30:00-06:00"
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
- The code itself, what it does, and an example of its use
- Related notes and sources

### Using the Template Picker

Don't remember template names? Use the picker:

```vim
:OkfwTemplate
```

The picker shows all templates with descriptions. Select one and continue.

---

## Part 3: Search and Discovery

### Full-Text Search

Find notes that contain specific terms:
```vim
:OkfwSearch authentication
```

### Live Search

Search as you type:
```vim
:OkfwSearch!
```

### Search with Flags

`:OkfwSearch` has flags that filter by frontmatter fields:

```vim
" Search one directory, recursively
:OkfwSearch --path research

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
:OkfwSearch --path issues --kind issue authentication

" Combine flags with live search
:OkfwSearch! --kind issue

" Filter by tag
:OkfwSearch --tag golang --tag api
```

Flags have tab completion. Type a flag name, then push `<Tab>` to see the valid values.

### Graph Visualization

See how the notes connect:
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
- You used the idea, and it worked
- The content is not yet clear
- It has a real connection to other notes
- You want to keep it for a long time

### How to Refine

1. Open the note
2. Review and sharpen the content
3. Add links to related notes with `\l`
4. Set the project context with `\p` (if not already set)
5. Add tags with `\a`

---

## Recommended Keymaps

Add these to the NeoVim configuration:

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
| Add tags | `\a` (buffer-local) | Auto-mapped on all concepts |
| Validate frontmatter | `\v` (buffer-local) | Auto-mapped on all concepts |
| Generate graph | `:OkfwGraph` | `<leader>zg` |
| Refresh `index.md` listings | `:OkfwIndex` | `<leader>zi` |

---

## Next Steps

### Build a Daily Habit

Daily notes are the base of a practice that lasts. They give:
- A consistent place to capture thoughts
- A natural rhythm for reviews
- A time line of thoughts

**Read:** [Daily Notes Workflow](/okf-wiki/neovim/daily-notes-workflow/) for a comprehensive guide to daily, weekly, and monthly review workflows in NeoVim.

### Keep Knowledge Current with OKF Metadata

okfw is a knowledge format, not a task tracker, so it has no todo fields. It
records whether you can still trust a concept:
- Lifecycle status (`draft`, `stable`, `deprecated`)
- A stale date, so a review is a query, and you do not have to remember it
- A verification trail, and provenance for the origin of the knowledge

**Read:** [CLI Commands](/okf-wiki/reference/cli-commands/#okf-metadata) for the OKF metadata commands.

### Master the CLI

The NeoVim plugin calls the `okfw` CLI. For scripts, automated tasks, or the terminal:

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
2. **Refine** - Add content and tags, and file it in a directory
3. **Link** - Connect checked ideas to related notes
4. **Search** - Find and connect the knowledge
5. **Template** - Use structured formats for consistency

Be consistent. Capture freely, review regularly, and link on purpose. Then the knowledge base grows naturally while you work.

---

*"The best time to plant a tree was 20 years ago. The second best time is now."*

Start with one note today.
