---
title: Daily Notes Workflow
description: A comprehensive guide to managing daily notes with okfw directly from NeoVim.
---

A comprehensive guide to managing daily notes with `okfw` directly from your editor. This document covers the daily, weekly, and monthly review workflows with NeoVim-specific commands and keybindings.

Your project needs an initialized wiki: run `:OkfwInit` (or `okfw init` from a shell) once at the project root to create the `.okfw/` directory. The plugin discovers the wiki via `okfw root` from NeoVim's current working directory, so everything below works from any subdirectory of the project.

---

## What Are Daily Notes?

A daily note is a dated entry that serves as your primary capture point for a given day. Rather than deciding "what type of note is this?" every time you have a thought, you write in today's daily note first. Later, you review and extract refined insights.

**Key characteristics:**
- **One per day** - Each date has exactly one daily note
- **Idempotent** - Running `:OkfwDaily` twice returns the same file
- **Temporal anchor** - Creates a timeline of your thinking
- **Low friction** - No categorization decisions needed during capture
- **Kind: daily-note** - A distinct structured kind for daily captures (`kind: "daily-note"`)

---

## Why Daily Notes Matter

### 1. Reduces Cognitive Overhead

When an idea strikes, you don't want to spend mental energy deciding:
- Is this worth a standalone note?
- What project does this belong to?
- What should I title this?

Daily notes eliminate these decisions. Just write. Sort it out later.

### 2. Creates a Personal Changelog

Daily notes become a searchable history of your work and thoughts:
- "What was I working on last Tuesday?"
- "When did I first encounter this bug?"
- "What did I learn during that project?"

### 3. Enables Spaced Repetition

The daily review practice naturally resurfaces ideas:
- Morning: Review yesterday's note
- Extract anything valuable into standalone notes
- Unprocessed ideas get a second chance

### 4. Supports GTD-Style Workflows

Daily notes function as an "inbox" in Getting Things Done methodology:
- Capture everything during the day
- Process and organize during review
- Refine valuable insights into permanent notes

### 5. Feeds Durable Concepts

Daily notes are a staging area: refine what proves durable into its own concept and link back, so the day's capture becomes part of the graph.

---

## The Daily Note Structure

`okfw` creates daily notes with this template:

```markdown
---
id: "019f0494-00a8-7206-b788-c5bbfa2430a8"
title: "2026-02-13 Friday"
type: "daily-note"
kind: "daily-note"
tags:
  - "daily"
created: "2026-02-13T00:00:00Z"
timestamp: "2026-02-13T00:00:00Z"
---

# 2026-02-13 Friday

## Morning

-

## Tasks

- [ ]

## Notes



## End of Day



## Links Created

-
```

### Section Purposes

| Section | Purpose |
|---------|---------|
| **Morning** | Intentions, priorities, energy level, gratitude |
| **Tasks** | Today's action items (can sync with task manager) |
| **Notes** | Free-form capture throughout the day |
| **End of Day** | Reflection, wins, blockers, tomorrow's focus |
| **Links Created** | Track concepts created today for review |

---

## NeoVim Commands

### Create or Open Daily Note

```vim
" Today's daily note
:OkfwDaily

" Yesterday's note (for morning review)
:OkfwDaily yesterday

" Specific date
:OkfwDaily 2026-02-10
```

### Browse Daily Notes

```vim
" Browse recent daily notes with picker
:OkfwDailyList

" Browse this week's notes only
:OkfwDailyList!
```

### Lua API

```lua
-- Open today's daily note
require("okfw").daily()

-- Open yesterday's note
require("okfw").daily({ date = "yesterday" })

-- Open specific date
require("okfw").daily({ date = "2026-02-10" })

-- Browse daily notes with picker
require("okfw").daily_picker()

-- List daily notes this week
require("okfw").daily_picker({ week = true })

-- List daily notes this month
require("okfw").daily_picker({ month = true })

-- Get daily notes synchronously (for scripting)
local notes = require("okfw").list_daily_sync({ week = true })
```

---

## Daily Workflow in NeoVim

### Morning Routine (5-10 minutes)

1. **Open today's note:**
   ```vim
   :OkfwDaily
   ```

2. **Review yesterday (in a split):**
   ```vim
   :OkfwDaily yesterday
   ```
   Or use the picker to see this week:
   ```vim
   :OkfwDailyList!
   ```

3. **Extract insights from yesterday:**
   - Identify anything worth keeping as a standalone note
   - Create new notes with `:OkfwNote` or `:OkfwTemplate`
   - Link back to the daily note if relevant

4. **Set today's intentions:**
   - Fill in the Morning section
   - List 3-5 tasks

### Throughout the Day

Capture everything in your daily note. Press your keymap to jump there instantly:

```lua
vim.keymap.set("n", "<leader>zd", "<cmd>OkfwDaily<cr>", { desc = "Today's daily" })
```

### End of Day (5 minutes)

1. Review the Notes section
2. Fill in the End of Day reflection
3. List any notes you created in Links Created
4. Mark completed tasks

---

## Refining Daily Captures into Concepts

A daily note is where a thought lands first; it is not where it should stay. When
something in today's note turns out to matter, promote it:

```vim
:OkfwNote Authentication token lifetime
```

Then link the two directions — the concept back to the daily note it came from,
and the daily note out to the concept — so the origin of an idea stays
recoverable.

### Suggested rhythm

1. Capture in the daily note: `:OkfwDaily`
2. Promote durable ideas: `:OkfwNote`


---

## Weekly Review Workflow

Weekly reviews help you step back and see patterns.

### List This Week's Notes

```vim
:OkfwDailyList!
```

Or via CLI:
```bash
okfw daily --list --week
```

### Weekly Review in NeoVim

1. **Open the daily picker for this week:**
   ```vim
   :OkfwDailyList!
   ```

2. **Scan each day's notes:**
   - Press `<CR>` to open a note
   - Use `<C-n>` and `<C-p>` to navigate the picker

3. **Create synthesis notes:**
   ```vim
   :OkfwTemplate meeting
   ```
   Use the meeting template for weekly summaries.

### Weekly Review Checklist

1. **Scan all daily notes** from the week
2. **Identify patterns:**
   - What topics kept coming up?
   - What questions remain unanswered?
   - What projects progressed? Stalled?
3. **Create synthesis notes:**
   - Weekly summary note
   - Standalone notes for recurring themes
4. **Review drafts and stale concepts** via `:OkfwSearch --status draft` and `:OkfwSearch --stale`

---

## Monthly Review Workflow

Monthly reviews are for strategic thinking.

### List This Month's Notes

```lua
require("okfw").daily_picker({ month = true })
```

Or via CLI:
```bash
okfw daily --list --month
```

### Monthly Review Checklist

1. **Skim all daily notes** (focus on End of Day sections)
2. **Review weekly summaries** if you created them
3. **Assess projects:**
   - What shipped?
   - What's blocked?
   - What should be abandoned?
4. **Review what you deprecated** via `:OkfwSearch --status deprecated`
5. **Set next month's themes**

---

## Recommended Keymaps

Add these to your NeoVim configuration:

```lua
-- Daily notes
vim.keymap.set("n", "<leader>zd", "<cmd>OkfwDaily<cr>", { desc = "Today's daily" })
vim.keymap.set("n", "<leader>zD", "<cmd>OkfwDaily yesterday<cr>", { desc = "Yesterday's daily" })
vim.keymap.set("n", "<leader>zw", "<cmd>OkfwDailyList!<cr>", { desc = "This week's dailies" })
```

---

## File Organization

Daily notes are stored inside the project's wiki (the `.okfw/` directory
created by `okfw init` and discovered automatically from any subdirectory) in
a structured hierarchy:

```
.okfw/
└── daily/
    └── 2026/
        ├── 01/
        │   ├── 2026-01-01--019f0494.md
        │   ├── 2026-01-02--019f04a1.md
        │   └── ...
        └── 02/
            ├── 2026-02-01--019f04b8.md
            └── ...
```

This structure:
- Lives at the wiki root, separate from project folders
- Groups notes by year and month
- Uses human-readable filenames (YYYY-MM-DD--<shortid>.md)
- Makes manual browsing easy
- Supports standard file system tools

---

## Integration with OKF Wiki

Daily notes complement the wider knowledge base:

| Knowledge Base Concept | Daily Notes Role |
|-----------------------|------------------|
| **Capture** | Daily note = primary inbox |
| **Refined notes** | Extract from daily during review |
| **Links** | Track in "Links Created" section |
| **Promotion** | Refine daily captures into standalone concepts |
| **Projects** | Daily notes capture project-related thoughts |

### The Flow

```
Daily Note (capture)
    |
Morning Review (extract)
    |
Standalone Note (refine)  <->    Linked concepts (the graph)
    |                              |
Graph (connect)             Done (complete)
```

---

## Quick Reference

| Action | Command | Keymap Suggestion |
|--------|---------|-------------------|
| Today's daily | `:OkfwDaily` | `<leader>zd` |
| Yesterday's daily | `:OkfwDaily yesterday` | `<leader>zD` |
| Browse dailies | `:OkfwDailyList` | |
| This week's dailies | `:OkfwDailyList!` | `<leader>zw` |
| Add tags | `\a` (buffer-local) | Auto-mapped on all concepts |
| Validate frontmatter | `\v` (buffer-local) | Auto-mapped on all concepts |

---

## Tips for Success

1. **Lower the bar** - A one-line daily note is still valuable
2. **Time-box reviews** - 5 minutes daily, 15 minutes weekly
3. **Don't backfill** - If you miss a day, just start fresh
4. **Use the search** - Your daily notes are searchable; trust the system
5. **Experiment** - Modify the template to fit your needs

---

## Related Documentation

- [Notes Workflow](/okf-wiki/neovim/notes-workflow/) - General note-taking workflow
- [CLI Commands](/okf-wiki/reference/cli-commands/) - Complete CLI reference
