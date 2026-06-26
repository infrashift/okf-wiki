---
title: Plugin Installation
description: Install and configure the okfw NeoVim plugin for seamless Zettelkasten integration.
---

The `okfw` NeoVim plugin provides seamless integration for creating notes directly from your editor.

## Prerequisites

- NeoVim 0.9+
- [plenary.nvim](https://github.com/nvim-lua/plenary.nvim) (required dependency)
- [snacks.nvim](https://github.com/folke/snacks.nvim) (optional, for picker UI)
- `okfw` binary installed and in your `$PATH`

## Installation

### Using lazy.nvim

```lua
{
    "infrashift/zettelkasten",
    dependencies = {
        "nvim-lua/plenary.nvim",
        "folke/snacks.nvim",  -- Optional, for picker UI
    },
    config = function()
        require("okfw").setup({
            bin = "okfw",  -- Path to okfw binary
        })
    end,
}
```

### Using packer.nvim

```lua
use {
    "infrashift/zettelkasten",
    requires = { "nvim-lua/plenary.nvim" },
    config = function()
        require("okfw").setup({
            bin = "okfw",
        })
    end,
}
```

### Using vim-plug

```vim
Plug 'nvim-lua/plenary.nvim'
Plug 'infrashift/zettelkasten'

" In your init.lua or after/plugin:
lua require("okfw").setup({ bin = "okfw" })
```

### Manual Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/infrashift/zettelkasten.git ~/.local/share/nvim/site/pack/plugins/start/zettelkasten
   ```

2. Ensure plenary.nvim is also installed

3. Add to your `init.lua`:
   ```lua
   require("okfw").setup({
       bin = "okfw",
   })
   ```

## Configuration

### Basic Setup

```lua
require("okfw").setup({
    bin = "okfw",  -- Default: "okfw"
})
```

### Custom Binary Path

```lua
require("okfw").setup({
    bin = "/usr/local/bin/okfw",
})
```

### With Development Build

```lua
require("okfw").setup({
    bin = vim.fn.expand("~/projects/zettelkasten/okfw"),
})
```

### Full Setup with Tag Completion

```lua
require("okfw").setup({
    bin = "okfw",
})

-- Enable tag completion in markdown files
require("okfw").setup_tag_completion()

-- Register nvim-cmp source (optional, if using nvim-cmp)
require("okfw").setup_cmp()
```

## Filetype Settings

When editing a concept file (markdown with `id:` in frontmatter), the plugin
automatically provides buffer-local keymaps:

| Keymap | Description |
|--------|-------------|
| `<C-x><C-t>` | Tag completion (insert mode) |
| `<localleader>l` | Insert link |
| `<localleader>L` | Insert link with title |
| `<localleader>b` | Toggle backlinks |
| `<localleader>p` | Set project (note and todo types) |
| `<localleader>t` | Tether / Untether (note and todo types) |
| `<localleader>a` | Add tags (all concept types) |
| `<localleader>v` | Validate frontmatter (all concept types) |
| `<localleader>s` | Set todo status (todo-type only) |

## Help Documentation

Full documentation is available via `:help okfw`.

See [User Commands](/zettelkasten/neovim/user-commands/) for a complete reference of all `:Okfw` commands.

## Keybindings

Add these to your NeoVim configuration:

```lua
-- Quick untethered note (no project)
vim.keymap.set("n", "<leader>zf", function()
    require("okfw").create_note("untethered")
end, { desc = "Create untethered note" })

-- Quick tethered note (will use git project)
vim.keymap.set("n", "<leader>zp", function()
    require("okfw").create_note("tethered")
end, { desc = "Create tethered note" })

-- Tether current note
vim.keymap.set("n", "<leader>zP", function()
    require("okfw").tether_note()
end, { desc = "Tether note" })

-- Set project on current note
vim.keymap.set("n", "<leader>zs", function()
    require("okfw").set_project()
end, { desc = "Set project" })

-- Search with picker
vim.keymap.set("n", "<leader>zz", function()
    require("okfw.picker").search()
end, { desc = "Search concepts" })

-- Live search
vim.keymap.set("n", "<leader>z/", function()
    require("okfw.picker").live_search()
end, { desc = "Live search concepts" })

-- Index current directory
vim.keymap.set("n", "<leader>zi", function()
    require("okfw").index()
end, { desc = "Index concepts" })

-- Generate graph visualization
vim.keymap.set("n", "<leader>zg", function()
    require("okfw").graph()
end, { desc = "Generate graph" })

-- Preview current note in floating window
vim.keymap.set("n", "<leader>zv", function()
    require("okfw").preview_note()
end, { desc = "Preview note" })

-- Preview note by ID
vim.keymap.set("n", "<leader>zV", function()
    require("okfw").preview_by_id()
end, { desc = "Preview by ID" })

-- Insert link (opens picker)
vim.keymap.set("n", "<leader>zl", function()
    require("okfw").link_picker()
end, { desc = "Insert link" })

-- Insert link with title
vim.keymap.set("n", "<leader>zL", function()
    require("okfw").link_picker({ include_title = true })
end, { desc = "Insert link with title" })

-- Toggle backlinks panel
vim.keymap.set("n", "<leader>zb", function()
    require("okfw").toggle_backlinks()
end, { desc = "Toggle backlinks" })

-- Open backlinks in split
vim.keymap.set("n", "<leader>zB", function()
    require("okfw").backlinks_split()
end, { desc = "Backlinks split" })

-- Create note from template (opens picker)
vim.keymap.set("n", "<leader>zt", function()
    require("okfw").template_picker()
end, { desc = "Create from template" })

-- Quick meeting notes
vim.keymap.set("n", "<leader>zm", function()
    require("okfw").create_from_template("meeting")
end, { desc = "Create meeting notes" })

-- Today's daily note
vim.keymap.set("n", "<leader>zd", function()
    require("okfw").daily()
end, { desc = "Today's daily note" })

-- Yesterday's daily note (morning review)
vim.keymap.set("n", "<leader>zD", function()
    require("okfw").daily({ date = "yesterday" })
end, { desc = "Yesterday's daily note" })

-- Browse daily notes
vim.keymap.set("n", "<leader>zw", function()
    require("okfw").daily_picker()
end, { desc = "Browse daily notes" })
```

### With Which-Key

```lua
local wk = require("which-key")
wk.register({
    z = {
        name = "OKF Wiki",
        f = { function() require("okfw").create_note("untethered") end, "Untethered note" },
        p = { function() require("okfw").create_note("tethered") end, "Tethered note" },
        P = { function() require("okfw").tether_note() end, "Tether note" },
        s = { function() require("okfw").set_project() end, "Set project" },
        g = { function() require("okfw").graph() end, "Generate graph" },
        v = { function() require("okfw").preview_note() end, "Preview note" },
        V = { function() require("okfw").preview_by_id() end, "Preview by ID" },
        l = { function() require("okfw").link_picker() end, "Insert link" },
        L = { function() require("okfw").link_picker({ include_title = true }) end, "Insert link with title" },
        b = { function() require("okfw").toggle_backlinks() end, "Toggle backlinks" },
        B = { function() require("okfw").backlinks_split() end, "Backlinks split" },
        t = { function() require("okfw").template_picker() end, "Create from template" },
        m = { function() require("okfw").create_from_template("meeting") end, "Meeting notes" },
        d = { function() require("okfw").daily() end, "Today's daily" },
        D = { function() require("okfw").daily({ date = "yesterday" }) end, "Yesterday's daily" },
        w = { function() require("okfw").daily_picker() end, "Browse daily notes" },
    },
}, { prefix = "<leader>" })
```

## How It Works

1. When you call `create_note()`, NeoVim prompts for a note title
2. The plugin invokes `okfw create "title" --category <category>`
3. The `okfw` binary detects your current git project automatically
4. A confirmation message appears on success

## Troubleshooting

### "okfw: command not found"

Ensure the `okfw` binary is in your `$PATH`:

```bash
# Check if okfw is accessible
which okfw

# If not, add to PATH or specify full path in setup
require("okfw").setup({
    bin = "/full/path/to/okfw",
})
```

### "No module named 'plenary'"

Install plenary.nvim:

```lua
-- lazy.nvim
{ "nvim-lua/plenary.nvim" }

-- packer.nvim
use "nvim-lua/plenary.nvim"
```

### Notes Not Created

1. Check that `okfw create` works from terminal
2. Verify you're in a git repository (for project detection)
3. Check NeoVim messages with `:messages`

## API Reference

### `setup(opts)`

Initialize the plugin with configuration options.

```lua
require("okfw").setup({
    bin = "okfw",  -- Path to okfw binary (default: "okfw")
})
```

### `create_note(note_category, project)`

Create a new note with the specified category.

**Parameters:**
- `note_category` (string): Either `"untethered"` or `"tethered"`
- `project` (string, optional): Project context. If nil, auto-detected from git.

**Behavior:**
1. Prompts for note title via `vim.fn.input()`
2. Executes `okfw create` asynchronously via plenary.job
3. Prints success/failure message

```lua
require("okfw").create_note("untethered")
require("okfw").create_note("tethered", "my-project")
```

### `tether_note(file_path, project)`

Tether an untethered note (promote to tethered).

**Parameters:**
- `file_path` (string, optional): Path to the note. Defaults to current buffer.
- `project` (string, optional): Project context. If nil, auto-detected from git.

**Behavior:**
1. Executes `okfw tether` asynchronously
2. Reloads the buffer if the tethered file is currently open
3. Prints success/failure message

```lua
require("okfw").tether_note()  -- Current file, auto-detect project
require("okfw").tether_note(nil, "my-project")  -- Current file, explicit project
```

### `untether_note(file_path)`

Untether a tethered note (revert to untethered).

**Parameters:**
- `file_path` (string, optional): Path to the note. Defaults to current buffer.

**Behavior:**
1. Executes `okfw untether` asynchronously
2. Reloads the buffer if the untethered file is currently open
3. Prints success/failure message

```lua
require("okfw").untether_note()  -- Current file
require("okfw").untether_note("/path/to/note.md")  -- Specific file
```

### `set_project(file_path, project)`

Set or update the project for a concept.

**Parameters:**
- `file_path` (string, optional): Path to the note. Defaults to current buffer.
- `project` (string, optional): Project name. If nil, prompts for input.

**Behavior:**
1. Prompts for project name if not provided
2. Executes `okfw set-project` asynchronously
3. Reloads the buffer if the modified file is currently open
4. Prints success/failure message

```lua
require("okfw").set_project()  -- Current file, prompt for project
require("okfw").set_project(nil, "my-project")  -- Current file, explicit project
```

### `search(query, opts)`

Search concepts with optional filters.

**Parameters:**
- `query` (string, optional): Full-text search query.
- `opts` (table, optional):
  - `project` (string): Filter by project
  - `category` (string): Filter by category
  - `tags` (table): Filter by tags (AND logic)
  - `limit` (number): Max results
  - `on_results` (function): Callback receiving results array

```lua
require("okfw").search("authentication")
require("okfw").search("query", { project = "my-project", on_results = function(r) ... end })
```

### `index(path)`

Index concepts for searching.

**Parameters:**
- `path` (string, optional): Path to index. Defaults to current directory.

```lua
require("okfw").index()  -- Index cwd
require("okfw").index("~/okfw/")
```

### `graph(opts)`

Generate an ASCII tree visualization of note relationships.

**Parameters:**
- `opts` (table, optional):
  - `path` (string): Path to scan. Defaults to current directory.
  - `limit` (number): Maximum nodes to display. Defaults to 10.

**Behavior:**
1. Executes `okfw graph` asynchronously
2. Opens the ASCII tree output in a vertical split

```lua
require("okfw").graph()  -- Graph cwd with defaults
require("okfw").graph({ limit = 20, path = "~/okfw/" })
```

### `preview_note(file_path)`

Preview a note in a floating window.

**Parameters:**
- `file_path` (string, optional): Path to the note. Defaults to current buffer.

**Behavior:**
1. Reads file content
2. Opens a centered floating window with rounded border
3. Sets up keymaps: `q`/`<Esc>` to close, `<CR>` to open in buffer

**Returns:** Table with `buf` (buffer handle) and `win` (window handle)

```lua
require("okfw").preview_note()  -- Preview current file
require("okfw").preview_note("/path/to/note.md")
```

### `preview_by_id(id)`

Preview a note by its ID (searches the index).

**Parameters:**
- `id` (string, optional): Concept id (UUIDv7, or the concept's bundle-relative path minus `.md`). Prompts for input if nil.

**Behavior:**
1. Searches for note with matching ID
2. Opens floating preview if found

```lua
require("okfw").preview_by_id("202602131045")
require("okfw").preview_by_id()  -- Prompts for ID
```

### Inserting links

Link insertion is normally done through the **link picker**, bound to
`<localleader>l` by default. It lets you search your concepts and insert a
vanilla markdown link to the one you pick — no need to know the concept id.

The inserted link points at the target concept's bundle-relative file path (a
leading `/` means the bundle root), for example:

```markdown
[Title](/infra/note--019f0494.md)
```

To follow a link, place the cursor on it and press `gf`.

#### `link_picker(opts)`

Open a picker to search and insert a markdown link. Requires snacks.nvim. This
is what `<localleader>l` invokes.

**Parameters:**
- `opts` (table, optional):
  - `include_title` (boolean): Default format for the `<CR>` action — `true`
    uses the concept title as the link text, otherwise the path is used
  - `query` (string): Initial search query

**Picker keymaps:**
- `<CR>` - Insert markdown link (link text depends on `include_title` option)
- `<C-t>` - Insert markdown link using the concept title as the link text

```lua
require("okfw").link_picker()  -- Opens picker, <CR> inserts [path](/path.md)
require("okfw").link_picker({ include_title = true })  -- <CR> inserts [title](/path.md)
```

### `get_tags(callback)`

Get all unique tags from indexed concepts (async).

**Parameters:**
- `callback` (function): Called with sorted list of tags

```lua
require("okfw").get_tags(function(tags)
    for _, tag in ipairs(tags) do
        print(tag)
    end
end)
```

### `get_tags_sync()`

Get all tags synchronously. Uses a 60-second cache.

**Returns:** Sorted list of tag strings

```lua
local tags = require("okfw").get_tags_sync()
```

### `refresh_tags()`

Clear the tag cache and reload tags.

```lua
require("okfw").refresh_tags()
```

### `complete_tags()`

Trigger manual tag completion at cursor using vim's completion menu.

```lua
require("okfw").complete_tags()
```

### `setup_tag_completion()`

Set up automatic tag completion for markdown files.

**Behavior:**
1. Sets `omnifunc` for markdown files
2. Adds `<C-x><C-t>` keymap for tag completion
3. Only completes when cursor is in frontmatter `tags:` section

```lua
require("okfw").setup_tag_completion()
```

### `setup_cmp()`

Register nvim-cmp source for tag completion.

**Returns:** `true` if nvim-cmp is available, `false` otherwise

```lua
if require("okfw").setup_cmp() then
    print("nvim-cmp source registered")
end
```

### `get_backlinks(id_or_file, callback)`

Get all notes that link to the specified concept (async).

**Parameters:**
- `id_or_file` (string): Zettel ID or file path
- `callback` (function): Called with list of backlink objects

**Backlink object fields:**
- `id`: Zettel ID
- `title`: Note title
- `project`: Project name
- `category`: "untethered" or "tethered"
- `file_path`: Absolute path to file

```lua
require("okfw").get_backlinks("202602131045", function(backlinks)
    print("Found " .. #backlinks .. " backlinks")
end)
```

### `get_backlinks_sync(id_or_file)`

Get backlinks synchronously.

**Returns:** List of backlink objects

```lua
local backlinks = require("okfw").get_backlinks_sync("202602131045")
```

### `backlinks_panel(opts)`

Open a floating backlinks panel for the current note.

**Parameters:**
- `opts` (table, optional):
  - `id` (string): Zettel ID to show backlinks for
  - `file` (string): File path to show backlinks for

**Panel keymaps:**
- `<CR>` / `o` - Open selected note
- `p` - Preview in floating window
- `q` / `<Esc>` - Close panel

**Returns:** Table with `buf`, `win`, and `backlinks`

```lua
require("okfw").backlinks_panel()  -- Current note
require("okfw").backlinks_panel({ id = "202602131045" })
```

### `backlinks_split(opts)`

Open backlinks in a split window.

**Parameters:**
- `opts` (table, optional):
  - `position` (string): "right" (default), "left", "bottom", or "top"
  - `id` (string): Zettel ID
  - `file` (string): File path

```lua
require("okfw").backlinks_split()
require("okfw").backlinks_split({ position = "bottom" })
```

### `toggle_backlinks(opts)`

Toggle the floating backlinks panel.

```lua
require("okfw").toggle_backlinks()
```

### `templates`

Table containing template metadata. Available templates: `meeting`, `book-review`, `snippet`, `project-idea`, `user-story`, `feature`.

```lua
for name, meta in pairs(require("okfw").templates) do
    print(name .. ": " .. meta.description)
end
```

### `get_template(name)`

Get template metadata by name.

**Parameters:**
- `name` (string): Template name

**Returns:** Table with `name`, `description`, `category`, `tags` or `nil` if not found

```lua
local tmpl = require("okfw").get_template("meeting")
print(tmpl.description)  -- "Meeting notes with attendees and action items"
print(tmpl.category)     -- "untethered"
```

### `create_from_template(template_name, project)`

Create a note from a template.

**Parameters:**
- `template_name` (string): Template name (e.g., `"meeting"`, `"user-story"`)
- `project` (string, optional): Project context. If nil, auto-detected from git.

**Behavior:**
1. Prompts for note title
2. Executes `okfw create --template <name>` asynchronously
3. Prints success/failure message

```lua
require("okfw").create_from_template("meeting")
require("okfw").create_from_template("feature", "my-project")
```

### `template_picker(opts)`

Open a picker to select a template and create a note. Requires snacks.nvim.

**Parameters:**
- `opts` (table, optional):
  - `project` (string): Project context for the new note

**Behavior:**
1. Displays all available templates with descriptions
2. On selection, prompts for note title
3. Creates note using selected template

```lua
require("okfw").template_picker()
require("okfw").template_picker({ project = "my-project" })
```

### `daily(opts)`

Create or open a daily note. Daily notes are idempotent - the same file is returned for the same date.

**Parameters:**
- `opts` (table, optional):
  - `date` (string): Date in `YYYY-MM-DD` format, or `"yesterday"` for yesterday

**Behavior:**
1. Determines target date (defaults to today)
2. Creates daily note if it doesn't exist
3. Opens the daily note in the current buffer

```lua
require("okfw").daily()                       -- Today
require("okfw").daily({ date = "yesterday" }) -- Yesterday
require("okfw").daily({ date = "2026-02-10" }) -- Specific date
```

### `list_daily(opts, callback)`

Get daily notes asynchronously.

**Parameters:**
- `opts` (table, optional):
  - `week` (boolean): Show only this week's notes
  - `month` (boolean): Show only this month's notes
- `callback` (function): Called with list of daily note objects

**Daily note object fields:**
- `date`: Date string (YYYY-MM-DD)
- `title`: Note title
- `file_path`: Absolute path to file

```lua
require("okfw").list_daily({ week = true }, function(notes)
    for _, note in ipairs(notes) do
        print(note.date .. ": " .. note.file_path)
    end
end)
```

### `list_daily_sync(opts)`

Get daily notes synchronously.

**Parameters:**
- `opts` (table, optional): Same as `list_daily`

**Returns:** List of daily note objects

```lua
local notes = require("okfw").list_daily_sync()
local this_week = require("okfw").list_daily_sync({ week = true })
```

### `daily_picker(opts)`

Open a picker to browse daily notes. Requires snacks.nvim.

**Parameters:**
- `opts` (table, optional):
  - `week` (boolean): Show only this week's notes
  - `month` (boolean): Show only this month's notes

```lua
require("okfw").daily_picker()
require("okfw").daily_picker({ week = true })
require("okfw").daily_picker({ month = true })
```

## Picker API

Requires snacks.nvim to be installed.

### `require("okfw.picker").search(opts)`

Open picker with all indexed concepts.

### `require("okfw.picker").live_search(opts)`

Open picker with live search (results update as you type).

### `require("okfw.picker").untethered(opts)`

Browse only untethered notes.

### `require("okfw.picker").tethered(opts)`

Browse only tethered notes.

**Common opts:**
- `project` (string): Filter by project
- `category` (string): Filter by category
- `tags` (table): Filter by tags
- `limit` (number): Max results

### `require("okfw.picker").insert_link(opts)`

Open picker specifically for inserting links. Same as `require("okfw").link_picker(opts)`.

**Opts:**
- `include_title` (boolean): Use the concept title as the link text by default

## Future Features

- Project completion
