---
title: NeoVim okfw Plugin Tutorial
description: Walk through building the devcontainer, connecting via SSH, and using the full OKF Wiki workflow with the NeoVim plugin.
---

In this tutorial, you build the devcontainer and connect to it through SSH.
Then you use the full OKF Wiki workflow. You create notes and link them. You
refine quick captures into permanent knowledge. You search, make graphs, and
record OKF metadata.

## Prerequisites

- [Podman](https://podman.io/), or Docker with `docker` in place of `podman`
- An SSH client
- A terminal emulator. Ghostty has full color support
- An x86_64 or arm64 (aarch64) host: the image builds natively on either

## 1. Build the Container

Clone the repository and build the image:

```bash
git clone https://github.com/infrashift/okf-wiki.git
cd okf-wiki

make build-image
```

That runs:

```bash
podman build --build-arg USER_UID=$(id -u) \
  --build-arg VERSION=$(git describe --tags --always --dirty) \
  -f Containerfiles/Containerfile -t okfw-devcontainer:latest .
```

The build compiles the `okfw` Go binary, and installs NeoVim, Claude Code, tmux
and all plugins. The first build takes a few minutes. `USER_UID` gives the
container's `user` the same UID as you. So under `--userns=keep-id`, that user
owns the `authorized_keys` file that you mount below. sshd refuses a key file
that a different user owns. `VERSION` is the text that `okfw --version`
prints. The build puts it into the binary as `make build` does.

## 2. Start the Container

The container runs an SSH server on port 2222. You need to mount an
`authorized_keys` file so you can authenticate.

**With the included test key** (only for local tests). git does not
keep the permissions of the private key, and ssh refuses a key that other users
can read. So first restrict it:

```bash
chmod 600 Containerfiles/config/test_ssh_key
podman run -d --name okfw-dev \
  --userns=keep-id \
  -p 2222:2222 \
  -v ./Containerfiles/config/test_ssh_key.pub:/home/user/.ssh/authorized_keys:ro,Z \
  okfw-devcontainer:latest
```

**With a personal SSH key** (the better choice, and what `make run-container` runs):

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
ssh -p 2222 -i ./Containerfiles/config/test_ssh_key -o IdentitiesOnly=yes \
  -o StrictHostKeyChecking=no user@localhost
```

`IdentitiesOnly=yes` makes ssh offer only the test key. Without it, ssh first
offers each key in the ssh-agent. When the agent holds many keys, sshd stops at
its limit of tries with `Too many authentication failures`.

**With a personal key:**

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

- **Left pane**: NeoVim, the editor
- **Top-right pane**: Bash shell
- **Bottom-right pane**: Claude Code AI assistant

Move between panes with the mouse (tmux has mouse mode on), or with the tmux
keybindings (`Ctrl-b`, then the arrow keys).

If you disconnect and connect again, SSH attaches to the same tmux session.
The state of the editor, the shell history and the Claude session stay.

Only an interactive login attaches to tmux. Give `ssh` a command and it runs
that instead, so remote commands, `scp` and `sftp` work:

```bash
ssh -p 2222 user@localhost okfw --version
scp -P 2222 notes.md user@localhost:my-project/
scp -P 2222 user@localhost:my-project/.okfw/index.md .
sftp -P 2222 user@localhost
```

`scp` takes the port as `-P`, capital, where `ssh` takes `-p`. With the test
key, add `-i ./Containerfiles/config/test_ssh_key -o IdentitiesOnly=yes` to each.

The CLI is on `PATH` in the container:

```bash
okfw --version
okfw --help
```

Run `okfw install` in a project to register its skill with Claude Code. The
skill is for one project, so the image build does not install it.

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
    my-note-title--019f049400a8.md
  daily/                       Daily notes, grouped by year/month
    2026/
      06/
        2026-06-26-friday--019f04a10000.md
  any-other-folder/            Concepts may live in any directory you choose
    another-concept--019f04b80000.md
  .ephemeral/                   Export scratch dir (gitignored)
  .gitignore                   Ignores .ephemeral/
```

Each `okfw` command **finds** the wiki: it looks in the current working
directory and then in each parent directory, as git does to find `.git`. So
commands work from any subdirectory of the project. Commit the concept markdown
with the project. The `.gitignore` excludes only the `.ephemeral/` export
directory. There is no search index or graph database to maintain. okfw
calculates search and the link graph from the files on each call.

A concept file has the name `slug--shortid.md`. The short id is the first 12
hex digits of the UUID of the concept. okfw reserves the filenames `index.md`
and `log.md`, and they carry no concept frontmatter. `index.md` is a directory
listing, for progressive disclosure. `log.md` is an update history from git.

Directories organize concepts by topic. Without `--path`, a new concept goes to
the wiki root. `--path <subdir>` puts it in that subdirectory. A quick capture
needs only a title, and you can move it into a directory later.

**Daily notes** are date-stamped journal entries stored inside the wiki under
`daily/` in a `YYYY/MM/` directory hierarchy.

Each note has a UUIDv7 `x-okfw-id`, for example
`019f0494-00a8-7206-b788-c5bbfa2430a8`. The id contains a timestamp, so ids
sort by time, and two ids are never the same. okfw still accepts legacy
`YYYYMMDDHHmmss-UUID` IDs. The **OKF id of a concept is its path in the wiki,
without `.md`**. For example, the file
`.okfw/infra/my-note-title--019f049400a8.md` has the id
`infra/my-note-title--019f049400a8`. Commands accept that id, the full UUID, or
a unique prefix of 8 or more hex digits (`019f0494`).

To **keep notes after the container restarts**, mount the project directory:

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

`okfw init` creates `.okfw/` at the root of the git repository, or in the
current directory outside git. It writes `.okfw/.gitignore` and the OKF root
`index.md`. A second run changes nothing. After that, each `okfw` command
finds the wiki from the project root or from any subdirectory. okfw creates
directories when you add concepts: the directory that you give with `--path`,
and `daily/` for daily notes.


## 6. Start Your Day with a Daily Note

Go to the NeoVim pane and create the first daily note:

```vim
:OkfwDaily
```

NeoVim opens the daily note of today. It has YAML frontmatter at the top, and
sections for the morning plan, tasks, notes, and the end of the day:

```markdown
---
x-okfw-id: "019f0494-00a8-7206-b788-c5bbfa2430a8"
title: "2026-02-19 Thursday"
type: "daily-note"
x-okfw-kind: "daily-note"
tags:
  - "daily"
x-okfw-created: "2026-02-19T14:30:00Z"
x-okfw-timestamp: "2026-02-19T14:30:00Z"
---

# 2026-02-19 Thursday

## Morning
- ...

## Tasks
- [ ] ...

## Notes


## End of Day


## Links Created
```

Fill in the morning plan. Add a few tasks. Save the file with `:w`. Search
reads the files directly, so you can find the note immediately. The save also
refreshes the `index.md` listings.

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

The plugin asks for a title. Type `Linked Note-Taking Method` and push
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
`feature`, `daily`, `issue`.

Create a meeting note:

```vim
:OkfwTemplate meeting
```

Type a title such as `Team Standup - Knowledge Management`. The template has
sections for attendees, agenda, topics, action items, and next steps. Fill it
in and save it.

Create a code snippet note:

```vim
:OkfwTemplate snippet
```

Title it `Bash - Find Files by Extension`. Fill in the code block with a useful
snippet and save.

## 9. Link Notes Together

Notes become more useful when you connect them. Links are **vanilla markdown
links** to the file path of a concept. The OKF id of a concept is its path
without `.md`. Open the meeting note that you created. Put the cursor where you
want a link, and insert one:

```vim
\l
```

(`\` is the local leader key followed by `l`)

A picker opens with all the notes. Select `Linked Note-Taking
Method` and push Enter. The plugin inserts a markdown link at the cursor. By
default, the link has a wiki-relative absolute path. A leading `/` means the
wiki root:

```markdown
[linked-note-taking-method--019f049400a8](/linked-note-taking-method--019f049400a8.md)
```

To insert a link that uses the title as link text (more readable):

```vim
\L
```

This inserts:

```markdown
[Linked Note-Taking Method](/linked-note-taking-method--019f049400a8.md)
```

You can also write relative links by hand, for example
`[text](./other-note--019f04a10000.md)`. okfw accepts broken links. To
**follow a link**, put the cursor on it and push `gf` (go to file).

You can also insert links from the search picker. Run `:OkfwSearch`,
highlight a note, and press `Ctrl-l` to insert its link at the cursor.

## 10. Create More Related Notes

To build a useful graph, create some connected notes. Here is a set to try.
For each note, use `:OkfwNote`, write the content, and link to related notes
with `\l`:

**Note 2: `Atomic Notes`**
```
An atomic note contains exactly one idea, fully developed.
This makes notes reusable across contexts.
```
Link to: `Linked Note-Taking Method`

**Note 3: `Linking as Thinking`**
```
The act of linking notes forces you to articulate the relationship
between ideas. This is where insight happens.
```
Link to: `Linked Note-Taking Method`, `Atomic Notes`

**Note 4: `Progressive Summarization`**
```
Layer highlights on top of notes over time. Bold the most important
passages, then highlight the bold, then write a summary.
```
Link to: `Atomic Notes`

**Note 5: `Capture and Refine`**
```
Capture ideas quickly as raw notes. Later, refine the valuable ones:
rewrite them in your own words with context and connections.
```
Link to: `Linked Note-Taking Method`, `Atomic Notes`

**Note 6: `Graph Thinking`**
```
A knowledge base is a graph of ideas, not a hierarchy.
Any note can connect to any other note.
```
Link to: `Linking as Thinking`, `Capture and Refine`

Now you have a small web of connected notes.

## 11. View Backlinks

Open the `Linked Note-Taking Method` note. Several notes link to it. View
its backlinks:

```vim
\b
```

A floating panel opens at the top-right. It shows each note that refers to
this one. In the panel:

- Press `Enter` or `o` to open a backlink
- Press `p` to preview it in a floating window
- Press `q` or `Esc` to close the panel

Toggle the panel on and off with `\b`.

## 12. Preview a Note

To see the raw content of a note and stay in the current file, push `Ctrl-p`
on a note in the `:OkfwSearch` picker. The `:OkfwDailyList` picker shows a
preview pane next to the list. You can also preview the current buffer:

```vim
:lua require("okfw").preview_note()
```

A floating preview window shows the note content. In the preview, push
`Enter` to open the note and edit it, or push `q` to close the preview.

## 13. File the Note in a Directory

You refined and linked the `Linked Note-Taking Method` note. Now file it in a
directory, so you can find it quickly later. With the note open, run:

```vim
:OkfwMove knowledge-management/
```

The note moves to `.okfw/knowledge-management/`, and keeps its id. okfw
changes each link to it to the new path, and the buffer follows the note. The
new directory gets its own `index.md` listing. In the CLI, the same move is
`okfw move <note> knowledge-management/`. `okfw create --path
knowledge-management` creates a note directly in that directory.

## 14. Search Your Notes

### Quick search

```vim
:OkfwSearch linked
```

A picker shows the notes that match. Select one, then push Enter to open it,
or `Ctrl-p` to preview it.

### Live search (updates as you type)

```vim
:OkfwSearch!
```

The `!` (bang) turns on live search. Start to type, and the results change
while you type.

### CLI search (from the bash pane)

Switch to the bash pane and search from the command line:

```bash
cd ~/my-project    # or any subdirectory — the wiki is discovered automatically

# Full-text search
okfw search "atomic notes"

# JSON output (for scripting)
okfw search --json --limit 5

# Search one directory
okfw search --path knowledge-management

# Filter by tag
okfw search --tag daily
```

## 15. Generate a Graph

See how the notes connect. In NeoVim:

```vim
:OkfwGraph 20
```

This command makes a [Mermaid](https://mermaid.js.org/) flowchart of up to 20
connected notes, and opens it in a scratch buffer. The label of each node is
its title, and each markdown link between concept files becomes an arrow. To
see the graph, copy the output into [mermaid.live](https://mermaid.live) or a
different Mermaid renderer.

From the CLI:

```bash
okfw graph --limit 20
```

You can also start the graph from a specific note and control depth:

```bash
okfw graph --start linked-note-taking-method--019f049400a8 --depth 3
```

## 16. Record OKF Metadata

okfw has **no task tracking** — the `status` field is OKF's *lifecycle* status,
not task progress. What it does track is how trustworthy and current a concept
is.

### Mark a concept as superseded

```bash
okfw set-lifecycle infra/old-method--019f0494 deprecated
```

`draft` means that nobody has reviewed it yet. `stable` (the default when the
field is absent) means that it is ready to use. `deprecated` means that okfw
keeps it for its links and history, but it is not current. Find drafts later
with `okfw search --status draft`.

### Give a concept a shelf life

```bash
okfw set-stale-after infra/pricing--019f0494 2026-12-31T00:00:00Z
```

A concept is stale at that instant and after it. A bare date such as
`2026-12-31` means the start of that day in UTC. `okfw search --stale` lists
all the concepts that are stale. So a review is a query, and you do not have to
remember it.

### Record that you reviewed something

```bash
okfw verify infra/pricing--019f0494
```

This command adds an entry to the `verified` list of the concept. The default
actor is `human:<login>`. The tier of a concept with no current entries is `unverified`. When only
automated checks verified it, the tier is `machine-confirmed`. When a human
approved it, the tier is `human-reviewed`. "Current" means that the entry is at
or after the last meaningful change of the concept (`generated.at`). After a
rewrite of the content, an earlier review does not count, because nobody
reviewed the new text.

### Cite where the knowledge came from

```bash
okfw add-source infra/pricing--019f0494 \
    --resource https://example.com/pricing-spec --id spec
```

Then attribute a specific claim in the body with a footnote whose label is that
source id:

```markdown
Enterprise tiers renew annually.[^spec]

[^spec]: Pricing specification
```

### Check the whole wiki

```bash
okfw validate --all    # strict okfw schema
okfw validate --okf    # permissive OKF conformance check
```

If you open a wiki from an older okfw, run `okfw migrate` first. It removes
the old task-tracking fields and moves the bundle to OKF v0.3. See the
[OKF v0.3 migration guide](/okf-wiki/reference/migrating-to-okf-v0-3/).

### Retire a concept instead of deleting it

```bash
okfw set-lifecycle infra/old-method--019f0494 deprecated \
    --superseded-by infra/new-method--019f0512
okfw search "method" --current   # the deprecated one gives way to its replacement
```



## 17. Use Templates for Structured Notes

Beyond the meeting and snippet templates shown earlier, try these:

**Book review:**

```vim
:OkfwTemplate book-review --path reading-list
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

Sections: requirements, design, API changes, test strategy, rollout plan.

**Issue (bug report or feature request):**

```vim
:OkfwTemplate issue
```

Sections: type (bug, feature or question), description, steps to reproduce,
and the expected and actual behavior.

## 18. Tag Completion

When you edit the YAML frontmatter of a note, put the cursor in the `tags:`
section. Then push `Ctrl-x Ctrl-t` in insert mode. This starts tag completion,
from all the tags in the bundle.

Refresh the tag cache if you've added new tags externally:

```vim
:OkfwRefreshTags
```

## 19. Refresh the Directory Listings

There is no search index to maintain, because search and the graph read the
files directly. okfw keeps only the OKF `index.md` directory listings up to
date. When you save a concept in NeoVim, the plugin refreshes them. To
regenerate them all by hand:

```vim
:OkfwIndex
```

Or from the CLI:

```bash
okfw index-md
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
| `:OkfwGraph [limit]` | Show Mermaid graph |
| `:OkfwIndex` | Regenerate `index.md` listings (`okfw index-md`) |
| `:OkfwRefreshTags` | Refresh tag cache |

### Keybindings (in concept markdown files)

| Key | Action |
|---|---|
| `\l` | Insert a markdown link (`[path](/path/to/note.md)`) |
| `\L` | Insert a markdown link using the concept title as link text |
| `gf` | Follow the link under the cursor |
| `\b` | Toggle backlinks panel |
| `\p` | Set project |
| `\a` | Add tags (all concept types) |
| `\v` | Validate frontmatter (all concept types) |
| `Ctrl-x Ctrl-t` | Tag completion (insert mode) |

### Picker Keys

| Key | Action |
|---|---|
| `Enter` | Open selected note |
| `Ctrl-p` | Preview selected note (`:OkfwSearch`) |
| `Ctrl-l` | Insert link to selected note (`:OkfwSearch`) |

### CLI Commands

| Command | Description |
|---|---|
| `okfw init` | Create a `.okfw/` wiki at the project root (no-op if it exists) |
| `okfw connect` | Verify the discovered wiki and print a summary |
| `okfw create [title]` | Create a note |
| `okfw daily` | Create/open daily note |
| `okfw search [query]` | Search notes |
| `okfw graph [path]` | Render a Mermaid diagram (`--ascii` for a text tree) |
| `okfw delete <ref>` | Delete a concept and its `index.md` entry |
| `okfw move <ref> <dest>` | Move or rename a concept, repairing links |
| `okfw backlinks [file]` | Show backlinks |
| `okfw add-tags <file> <tag1> [tag2...]` | Add tags to a concept |
| `okfw remove-tags <file> <tag1> [tag2...]` | Remove tags from a concept |
| `okfw validate <file>` | Validate frontmatter against okfw's CUE schema |
| `okfw validate --all [path]` | Apply that strict check to every concept |
| `okfw validate --okf [path]` | Validate whole-wiki OKF conformance (permissive) |
| `okfw migrate [path]` | Bring a pre-0.2 wiki up to the current schema |
| `okfw set-lifecycle <ref> <status>` | Set draft/stable/deprecated |
| `okfw set-stale-after <ref> <datetime>` | Set the staleness instant |
| `okfw verify <ref>` | Record a verification event |
| `okfw add-source <ref> --resource <uri>` | Append a provenance source |
| `okfw set-resource <ref> <uri>` | Set the described asset's URI |
| `okfw set-generated <ref>` | Record who generated a concept |
| `okfw graph-query <query>` | Query the link graph (`backlinks`, `cluster`, `tag`, `dangling`) |
| `okfw export [path]` | Export a portable subset to `.ephemeral/` |
| `okfw index-md [path]` | Generate OKF `index.md` directory listings |
| `okfw log [path]` | Generate OKF `log.md` from git history |
| `okfw root` | Print the discovered `.okfw` path |
| `okfw templates` | List templates |
| `okfw install` | Install the okfw agent skill (`--platform claude\|gemini\|codex`) |
| `okfw mcp` | Serve the wiki to AI agents over MCP (stdio) |

## Container Management

```bash
# Stop the container
podman stop okfw-dev

# Start it again (notes and tmux session preserved)
podman start okfw-dev

# SSH reconnects to the same tmux session
ssh -p 2222 user@localhost

# Run one command, or copy files, without attaching
ssh -p 2222 user@localhost okfw --version
scp -P 2222 notes.md user@localhost:

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
