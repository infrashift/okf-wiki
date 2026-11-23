---
title: Daily Notes Workflow
description: A comprehensive guide to managing daily notes with okfw directly from NeoVim.
---

This guide shows how to keep daily notes with `okfw` from the editor. It covers the daily, weekly and monthly reviews, with the NeoVim commands and keybindings for them.

The project needs a wiki. To create the `.okfw/` directory, run `:OkfwInit` (or `okfw init` in a shell) one time at the project root. The plugin finds the wiki with `okfw root`, from the current working directory of NeoVim. So all the steps below work from any subdirectory of the project.

---

## What Are Daily Notes?

A daily note is a dated entry: the first place where you capture ideas on that day. Do not ask "what type of note is this?" for each thought. Write in the daily note of today first. Later, review it and move the refined ideas into their own notes.

**Key characteristics:**
- **One per day** - Each date has exactly one daily note
- **Idempotent** - When you run `:OkfwDaily` two times, you get the same file
- **Time line** - The notes record when each idea occurred
- **Easy capture** - You do not choose a category when you capture
- **Kind: daily-note** - A distinct structured kind for daily captures (`kind: "daily-note"`)

---

## Why Daily Notes Matter

### 1. Reduces Cognitive Overhead

When an idea occurs, you do not want to stop and answer these questions:
- Is this worth a standalone note?
- What project does this belong to?
- What title do I give it?

With daily notes, you do not answer them. Write first. Sort later.

### 2. Creates a Personal Changelog

Daily notes become a history of work and thoughts that you can search:
- "What was I working on last Tuesday?"
- "When did I first encounter this bug?"
- "What did I learn during that project?"

### 3. Enables Spaced Repetition

The daily review practice naturally resurfaces ideas:
- Morning: Review yesterday's note
- Extract anything valuable into standalone notes
- Unprocessed ideas get a second chance

### 4. Supports GTD-Style Workflows

Daily notes are an "inbox", as in the GTD method of David Allen:
- Capture everything during the day
- Process and organize during review
- Refine valuable insights into permanent notes

### 5. Feeds Durable Concepts

Daily notes are a first stop. Refine each durable idea into its own concept, and link back. Then the capture of the day becomes part of the graph.

---

## The Daily Note Structure

`okfw` creates daily notes with this template:

```markdown
---
x-okfw-id: "019f0494-00a8-7206-b788-c5bbfa2430a8"
title: "2026-02-13 Friday"
type: "daily-note"
x-okfw-kind: "daily-note"
tags:
  - "daily"
x-okfw-created: "2026-02-13T08:12:30-06:00"
x-okfw-timestamp: "2026-02-13T08:12:30-06:00"
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
| **Tasks** | Today's action items |
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
" Browse the last 14 days of daily notes with picker
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
   :vsplit | OkfwDaily yesterday
   ```
   Or use the picker to see this week:
   ```vim
   :OkfwDailyList!
   ```

3. **Extract insights from yesterday:**
   - Find the ideas that are worth a standalone note
   - Create new notes with `:OkfwNote` or `:OkfwTemplate`
   - Link back to the daily note if relevant

4. **Set today's intentions:**
   - Fill in the Morning section
   - List 3-5 tasks

### Throughout the Day

Capture everything in the daily note. Set a keymap that opens it immediately:

```lua
vim.keymap.set("n", "<leader>zd", "<cmd>OkfwDaily<cr>", { desc = "Today's daily" })
```

### End of Day (5 minutes)

1. Review the Notes section
2. Fill in the End of Day section
3. List any notes you created in Links Created
4. Mark completed tasks

---

## Refining Daily Captures into Concepts

A thought goes into the daily note first, but it must not stay there. When an
idea in the note of today is important, promote it:

```vim
:OkfwNote
```

and enter the title at the prompt (`Note Title: Authentication token lifetime`).

Then add links in the two directions. Link the concept back to its daily note,
and link the daily note to the concept. Then you can always find the origin of
an idea.

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
   - Which topics occurred many times?
   - What questions remain unanswered?
   - What projects progressed? Stalled?
3. **Create synthesis notes:**
   - Weekly summary note
   - Standalone notes for themes that occur again
4. **Review drafts and stale concepts** via `:OkfwSearch --status draft` and `:OkfwSearch --stale`

---

## Monthly Review Workflow

Use the monthly review for strategy.

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
   - What do we stop?
4. **Review what you deprecated** via `:OkfwSearch --status deprecated`
5. **Set next month's themes**

---

## Recommended Keymaps

Add these to the NeoVim configuration:

```lua
-- Daily notes
vim.keymap.set("n", "<leader>zd", "<cmd>OkfwDaily<cr>", { desc = "Today's daily" })
vim.keymap.set("n", "<leader>zD", "<cmd>OkfwDaily yesterday<cr>", { desc = "Yesterday's daily" })
vim.keymap.set("n", "<leader>zw", "<cmd>OkfwDailyList!<cr>", { desc = "This week's dailies" })
```

---

## File Organization

okfw keeps daily notes in the wiki of the project, in a tree of directories.
The wiki is the `.okfw/` directory that `okfw init` creates, and okfw finds it
from any subdirectory:

```
.okfw/
└── daily/
    └── 2026/
        ├── 01/
        │   ├── 2026-01-01-thursday--019f049400a8.md
        │   ├── 2026-01-02-friday--019f04a10000.md
        │   └── ...
        └── 02/
            ├── 2026-02-01-sunday--019f04b80000.md
            └── ...
```

This structure:
- Is at the wiki root, apart from the topic directories
- Groups notes by year and month
- Uses human-readable filenames (`YYYY-MM-DD-<weekday>--<shortid>.md`)
- Is easy to browse by hand
- Supports standard file system tools

---

## Integration with OKF Wiki

Daily notes add to the knowledge base:

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
3. **Do not fill in old days** - If you miss a day, start again on the next day
4. **Use the search** - You can search the daily notes, so trust the system
5. **Try changes** - Change the template until it fits

---

## Related Documentation

- [Notes Workflow](/okf-wiki/neovim/notes-workflow/) - The general workflow for notes
- [CLI Commands](/okf-wiki/reference/cli-commands/) - Complete CLI reference
