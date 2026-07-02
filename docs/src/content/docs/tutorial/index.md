---
title: NeoVim okfw Plugin Tutorial
description: Walk through building the devcontainer, connecting via SSH, and using the full OKF Wiki workflow with the NeoVim plugin.
---

This tutorial walks you through building the devcontainer, connecting via SSH,
and using the full OKF Wiki workflow: creating notes, linking them together,
refining quick captures into permanent knowledge, searching, generating
graphs, and managing todos.

## Prerequisites

- [Podman](https://podman.io/) (or Docker, substituting `docker` for `podman`)
- An SSH client
- A terminal emulator (Ghostty recommended for full color support)

## 1. Build the Container

Clone the repository and build the image:

```bash
git clone https://github.com/infrashift/okf-wiki.git
cd okf-wiki

podman build -f Containerfiles/Containerfile -t okfw-devcontainer:latest .
```

The build compiles the `okfw` Go binary, installs NeoVim, Claude Code, tmux, and
all plugins. It takes a few minutes the first time.

## 2. Start the Container

The container runs an SSH server on port 2222. You need to mount an
`authorized_keys` file so you can authenticate.

**Using the included test key** (for local development only):

```bash
podman run -d --name okfw-dev \
  --userns=keep-id \
  -p 2222:2222 \
  -v ./Containerfiles/config/test_ssh_key.pub:/home/user/.ssh/authorized_keys:ro,Z \
  okfw-devcontainer:latest
```

**Using your own SSH key** (recommended):

```bash
podman run -d --name okfw-dev \
  --userns=keep-id \
  -p 2222:2222 \
  -v ~/.ssh/id_ed25519.pub:/home/user/.ssh/authorized_keys:ro,Z \
  okfw-devcontainer:latest
```

Verify the container started:

```bash
podman logs okfw-dev
# Should show: Starting sshd on port 2222...
#              Server listening on 0.0.0.0 port 2222.
```

## 3. Connect via SSH

**With the test key:**

```bash
ssh -p 2222 -i ./Containerfiles/config/test_ssh_key \
  -o StrictHostKeyChecking=no user@localhost
```

**With your own key:**

```bash
ssh -p 2222 user@localhost
```

You land in a **tmux session** with three panes:

```
+---------------------------+------------+
|                           |   bash     |
|         NeoVim            +------------+
|                           |   claude   |
+---------------------------+------------+
```

- **Left pane**: NeoVim (your editor)
- **Top-right pane**: Bash shell
- **Bottom-right pane**: Claude Code AI assistant

Navigate between panes with your mouse (mouse mode is enabled) or with tmux
keybindings (`Ctrl-b` then arrow keys).

If you disconnect and reconnect, SSH reattaches to the same tmux session
automatically. Your editor state, shell history, and Claude conversation are
preserved.

## 4. Where Are Notes Stored?

Each project gets its own wiki: a `.okfw/` directory at the project's root,
created by `okfw init`. All notes live inside `.okfw/`, organized by topic and
project:

```
.okfw/                         Wiki root (at the project root)
  index.md                     Directory listing (reserved; carries okf_version)
  log.md                       Update history (reserved)
  infra/                       A project folder (concepts grouped by project)
    index.md
    my-note-title--019f0494.md
  daily/                       Daily notes, grouped by year/month
    2026/
      06/
        2026-06-26--019f04a1.md
  any-other-folder/            Concepts may live in any directory you choose
    another-concept--019f04b8.md
  ephemeral/                   Export scratch dir (gitignored)
  .index/                      Full-text search index (auto-managed)
  .graph.db                    Persistent graph database (auto-managed)
  .todos/                      Generated todo lists
  .gitignore                   Ignores .index/, .graph.db, .todos/, ephemeral/
```

Every `okfw` command **discovers** the wiki automatically by searching upward
from the current working directory (the way git finds `.git`), so commands
work from any subdirectory of the project. Concept markdown is meant to be
committed with the project; the generated artifacts are gitignored.

Concept files are named `slug--shortid.md`, where the short id is the leading
hex of the concept's UUID. `index.md` and `log.md` are **reserved filenames**
that carry no concept frontmatter: `index.md` is a directory listing (for
progressive disclosure) and `log.md` is an update history generated from git.

Concepts are placed by topic and project. With no `--path`, a concept lands in
a top-level folder named after its project (or the wiki root when there is no
project); `--path <subdir>` overrides that. A project is always optional — quick
captures need nothing more than a title, and you can assign a project later.

**Daily notes** are date-stamped journal entries stored inside the wiki under
`daily/` in a `YYYY/MM/` directory hierarchy.

Each note has a UUIDv7 `id` (e.g. `019f0494-00a8-7206-b788-c5bbfa2430a8`), which
embeds a timestamp for natural sorting while remaining collision-free. Legacy
`YYYYMMDDHHmmss-UUID` IDs are still accepted. A concept's **OKF id is its
path within the wiki minus `.md`** — for example the file
`.okfw/infra/my-note-title--019f0494.md` has the id
`infra/my-note-title--019f0494`.

To **persist notes across container restarts**, mount your project directory:

```bash
podman run -d --name okfw-dev \
  --userns=keep-id \
  -p 2222:2222 \
  -v ~/.ssh/id_ed25519.pub:/home/user/.ssh/authorized_keys:ro,Z \
  -v ~/my-project:/home/user/my-project:Z \
  okfw-devcontainer:latest
```

## 5. Initialize Your Wiki

SSH into the container and switch to the bash pane (click it or press `Ctrl-b`
then right-arrow). Set up a project and initialize the wiki:

```bash
mkdir -p ~/my-project
cd ~/my-project
git init
git config user.email "you@example.com"
git config user.name "Your Name"

okfw init
```

`okfw init` creates `.okfw/` at the git repository root (or the current
directory outside git), writes `.okfw/.gitignore`, and generates the OKF root
`index.md`. Running it again is a no-op. From here on, every `okfw` command
finds the wiki automatically — from the project root or any subdirectory. The
tool creates folders on demand as you add concepts — a top-level folder per
project (or whatever `--path` you pass), plus `daily/` for daily notes.


## 6. Start Your Day with a Daily Note

Switch to the NeoVim pane and create your first daily note:

```vim
:OkfwDaily
```

NeoVim opens today's daily note. It has YAML frontmatter at the top and
sections for your morning plan, tasks, notes, and end-of-day reflection:

```markdown
---
id: "019f0494-00a8-7206-b788-c5bbfa2430a8"
title: "Daily Note - 2026-02-19"
type: "daily-note"
kind: "daily-note"
tags:
  - daily
created: "2026-02-19T14:30:00Z"
timestamp: "2026-02-19T14:30:00Z"
---

# Daily Note - 2026-02-19

## Morning
- [ ] ...

## Tasks
- [ ] ...

## Notes


## End of Day


## Links Created
```

Fill in your morning plan. Add a few tasks. Save the file with `:w`. The note
is automatically indexed for search.

You can also view yesterday's daily note:

```vim
:OkfwDaily yesterday
```

Or browse all daily notes with a picker:

```vim
:OkfwDailyList
```

## 7. Create a Note

Capture a quick idea. In NeoVim:

```vim
:OkfwNote
```

You are prompted for a title. Type `Linked Note-Taking Method` and press
Enter. A new note opens with frontmatter and an empty body. Write
some content:

```markdown
Linked note-taking is a personal knowledge management approach.
Key principles:

- One idea per note (atomicity)
- Write in your own words (elaboration)
- Connect notes to each other (linking)
- Capture quickly, refine later
```

Save with `:w`.

## 8. Create a Note from a Template

Templates give notes structure. Open the template picker:

```vim
:OkfwTemplate
```

Available templates: `meeting`, `book-review`, `snippet`, `project-idea`, `user-story`,
`feature`, `daily`, `todo`, `issue`.

Create a meeting note:

```vim
:OkfwTemplate meeting
```

Enter a title like `Team Standup - Knowledge Management`. The template provides
sections for attendees, agenda, discussion, action items, and next steps. Fill
it in and save.

Create a code snippet note:

```vim
:OkfwTemplate snippet
```

Title it `Bash - Find Files by Extension`. Fill in the code block with a useful
snippet and save.

## 9. Link Notes Together

Notes become powerful when connected. Links are **vanilla markdown links** that
point at a concept's file path — a concept's OKF id is its path minus `.md`.
Open the meeting note you just created, place your cursor where you want a link,
and insert one:

```vim
\l
```

(`\` is the local leader key followed by `l`)

A picker opens showing all your notes. Select `Linked Note-Taking
Method` and press Enter. A markdown link is inserted at your cursor. By default
it uses a wiki-relative absolute path (a leading `/` means the wiki root):

```markdown
[linked-note-taking-method--019f0494](/infra/linked-note-taking-method--019f0494.md)
```

To insert a link that uses the title as link text (more readable):

```vim
\L
```

This inserts:

```markdown
[Linked Note-Taking Method](/infra/linked-note-taking-method--019f0494.md)
```

You can also write relative links by hand, e.g. `[text](./other-note--019f04a1.md)`.
Broken links are tolerated. To **follow a link**, place your cursor on it and
press `gf` (go to file).

You can also insert links from the search picker. Run `:OkfwSearch`,
highlight a note, and press `Ctrl-l` to insert its link at the cursor.

## 10. Create More Related Notes

To build a meaningful graph, create several connected notes. Here is a suggested
set. For each one, use `:OkfwNote`, write content, and link to related notes using
`\l`:

**Note 2: "Atomic Notes"**
```
An atomic note contains exactly one idea, fully developed.
This makes notes reusable across contexts.
```
Link to: `Linked Note-Taking Method`

**Note 3: "Linking as Thinking"**
```
The act of linking notes forces you to articulate the relationship
between ideas. This is where insight happens.
```
Link to: `Linked Note-Taking Method`, `Atomic Notes`

**Note 4: "Progressive Summarization"**
```
Layer highlights on top of notes over time. Bold the most important
passages, then highlight the bold, then write a summary.
```
Link to: `Atomic Notes`

**Note 5: "Capture and Refine"**
```
Capture ideas quickly as raw notes. Later, refine the valuable ones:
rewrite them in your own words with context and connections.
```
Link to: `Linked Note-Taking Method`, `Atomic Notes`

**Note 6: "Graph Thinking"**
```
A knowledge base is a graph of ideas, not a hierarchy.
Any note can connect to any other note.
```
Link to: `Linking as Thinking`, `Capture and Refine`

After creating these, you have a small web of interconnected notes.

## 11. View Backlinks

Open the `Linked Note-Taking Method` note. Several notes link to it. View
its backlinks:

```vim
\b
```

A floating panel appears at the top-right showing every note that references
this one. From the panel:

- Press `Enter` or `o` to open a backlink
- Press `p` to preview it in a floating window
- Press `q` or `Esc` to close the panel

Toggle the panel on and off with `\b`.

## 12. Preview a Note

### Rendered Markdown with Glow

The devcontainer includes [Glow](https://github.com/charmbracelet/glow), a
terminal-based Markdown renderer. With a note open, run:

```vim
:Glow
```

Glow renders the current buffer as styled Markdown in a floating window —
headings, bullet lists, code blocks, and links are all formatted for the
terminal. Press `q` to close the Glow preview.

### Quick preview floating window

To peek at a note's raw content without leaving your current file:

```vim
\p
```

A floating preview window shows the note content. Inside the preview, press
`Enter` to open it for editing or `q` to close.

## 13. Set a Project

The `Linked Note-Taking Method` note has been refined and linked. Give it a
project context so it's easy to find later. With the note open, press `\p`:

```
\p  →  Project: _
```

Enter `knowledge-management`. The note's frontmatter updates: the `project`
field is set. You can set or change a project on any note the same way, or from
the CLI with `okfw set-project`.

## 14. Search Your Notes

### Quick search

```vim
:OkfwSearch linked
```

A picker shows matching notes. Select one and press Enter to open it,
or `Ctrl-p` to preview.

### Live search (updates as you type)

```vim
:OkfwSearch!
```

The `!` (bang) enables live search mode. Start typing and results filter in
real-time.

### CLI search (from the bash pane)

Switch to the bash pane and search from the command line:

```bash
cd ~/my-project    # or any subdirectory — the wiki is discovered automatically

# Full-text search
okfw search "atomic notes"

# JSON output (for scripting)
okfw search --json --limit 5

# Filter by project
okfw search --project knowledge-management

# Filter by tag
okfw search --tag daily
```

## 15. Generate a Graph

Visualize how your notes connect. In NeoVim:

```vim
:OkfwGraph 20
```

This generates a [Mermaid](https://mermaid.js.org/) flowchart showing up to 20
connected notes and opens it in a scratch buffer. Each node is labeled with its
title and every markdown link between concept files becomes an arrow. Copy the output
into [mermaid.live](https://mermaid.live) or any Mermaid renderer to see the
visual graph.

From the CLI:

```bash
okfw graph --limit 20
```

You can also start the tree from a specific note and control depth:

```bash
okfw graph --start infra/linked-note-taking-method--019f0494 --depth 3
```

## 16. Manage Todos

### Create a todo

```vim
:OkfwTodo Buy a notebook for handwritten concept drafts
```

Or with a due date and priority:

```vim
:OkfwTodo Review meeting notes --due 2026-02-21 --priority high
```

### Change todo status

With the todo open in the editor, press `\s` to open the status picker. Choose from:
- `open` — not started
- `in_progress` — being worked on
- `closed` — completed

### Generate a todo list

Create a markdown summary of your todos:

```vim
:OkfwTodoList
```

This generates a markdown summary and opens it in a split.

## 17. Use Templates for Structured Notes

Beyond the meeting and snippet templates shown earlier, try these:

**Book review:**

```vim
:OkfwTemplate book-review --project reading-list
```

Sections: author, rating, summary, key takeaways, favorite quotes.

**Project idea:**

```vim
:OkfwTemplate project-idea
```

Sections: problem statement, proposed solution, goals, non-goals, success
metrics.

**Feature spec:**

```vim
:OkfwTemplate feature
```

Sections: requirements, design, API changes, testing strategy, rollout plan.

**Issue (bug report or enhancement):**

```vim
:OkfwTemplate issue
```

Sections: type (bug/enhancement/question), description, steps to reproduce,
expected vs actual behavior.

## 18. Tag Completion

When editing a note's YAML frontmatter, place your cursor in the `tags:`
section and press `Ctrl-x Ctrl-t` in insert mode. This triggers tag
autocompletion from all tags used across your bundle.

Refresh the tag cache if you've added new tags externally:

```vim
:OkfwRefreshTags
```

## 19. Index Your Notes

Notes are automatically indexed when you save them in NeoVim. To manually
reindex everything:

```vim
:OkfwIndex
```

Or from the CLI:

```bash
okfw index
```

## Quick Reference

### NeoVim Commands

| Command | Description |
|---|---|
| `:OkfwDaily` | Open/create today's daily note |
| `:OkfwDaily yesterday` | Open yesterday's daily note |
| `:OkfwDailyList` | Browse daily notes |
| `:OkfwNote` | Create a new note |
| `:OkfwTemplate [name]` | Create from template |
| `:OkfwSearch [query]` | Search notes |
| `:OkfwSearch!` | Live search (updates as you type) |
| `:Glow` | Render current note as styled Markdown |
| `:OkfwGraph [limit]` | Show Mermaid graph |
| `:OkfwTodo [title]` | Create a todo |
| `:OkfwTodoList` | Generate todo list markdown |
| `:OkfwIndex` | Reindex all notes |
| `:OkfwRefreshTags` | Refresh tag cache |

### Keybindings (in concept markdown files)

| Key | Action |
|---|---|
| `\l` | Insert a markdown link (`[path](/path/to/note.md)`) |
| `\L` | Insert a markdown link using the concept title as link text |
| `gf` | Follow the link under the cursor |
| `\b` | Toggle backlinks panel |
| `\p` | Set project (note and todo types) |
| `\a` | Add tags (all concept types) |
| `\v` | Validate frontmatter (all concept types) |
| `\s` | Set todo status (todo-type only) |
| `Ctrl-x Ctrl-t` | Tag completion (insert mode) |

### Picker Keys

| Key | Action |
|---|---|
| `Enter` | Open selected note |
| `Ctrl-p` | Preview selected note |
| `Ctrl-l` | Insert link to selected note |

### CLI Commands

| Command | Description |
|---|---|
| `okfw init` | Create a `.okfw/` wiki at the project root (no-op if it exists) |
| `okfw connect` | Verify the discovered wiki and print a summary |
| `okfw create [title]` | Create a note |
| `okfw daily` | Create/open daily note |
| `okfw todo [title]` | Create a todo |
| `okfw todos` | List todos |
| `okfw set-status [file] [status]` | Set todo status (open/in_progress/closed) |
| `okfw search [query]` | Search notes |
| `okfw graph [path]` | Show graph tree |
| `okfw set-project [file] [project]` | Set project on a note |
| `okfw backlinks [file]` | Show backlinks |
| `okfw add-tags <file> <tag1> [tag2...]` | Add tags to a concept |
| `okfw validate <file>` | Validate frontmatter against CUE schema |
| `okfw validate --okf [path]` | Validate whole-wiki OKF conformance |
| `okfw index [path]` | Index notes |
| `okfw index-md [path]` | Generate OKF `index.md` directory listings |
| `okfw log [path]` | Generate OKF `log.md` from git history |
| `okfw root` | Print the discovered `.okfw` path |
| `okfw templates` | List templates |

## Container Management

```bash
# Stop the container
podman stop okfw-dev

# Start it again (notes and tmux session preserved)
podman start okfw-dev

# SSH reconnects to the same tmux session
ssh -p 2222 user@localhost

# Remove the container
podman rm -f okfw-dev

# Custom SSH port
podman run -d --name okfw-dev \
  --userns=keep-id \
  -e SSH_PORT=3333 -p 3333:3333 \
  -v ~/.ssh/id_ed25519.pub:/home/user/.ssh/authorized_keys:ro,Z \
  -v ~/my-project:/home/user/my-project:Z \
  okfw-devcontainer:latest
```
