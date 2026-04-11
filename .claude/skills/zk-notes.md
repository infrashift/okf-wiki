---
description: Create, search, validate, and manage zettelkasten notes using the zk CLI
globs:
  - "cli/**/*.go"
  - "**/*.md"
---

# Skill: Zettelkasten Note Management

Use the `zk` CLI to create, search, and manage notes. The binary must be built first: `go build -o zk ./cli/cmd/zk`

## Zettel Format

Every note is a markdown file with YAML frontmatter. ID format: `YYYYMMDDHHmmss-UUIDv4`.

```yaml
---
id: "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
title: "My Note Title"
type: "note"           # note | todo | daily-note | issue
project: "myproject"   # required for tethered, optional for untethered
category: "untethered" # untethered | tethered
tags:
  - "golang"
  - "architecture"
created: "2026-02-13T10:45:00Z"
parent: "20260212..."  # optional, links to parent zettel ID
---

# My Note Title

Body content with [[20260212...-uuid|Display Title]] style links.
```

## Creating Notes

```bash
# Basic note (untethered, auto-detects project from git context)
zk create "My Note Title"

# Tethered note with explicit project
zk create "Design Doc" --category tethered --project myproject

# From template (meeting, book-review, snippet, project-idea, user-story, feature, daily, todo, issue)
zk create "Sprint Planning" --template meeting --project myproject

# With tags
zk create "Go Patterns" --tags golang,patterns

# List available templates
zk templates
```

## Searching

Requires index: `zk index` or `zk index ~/zk_vault`

```bash
# Full-text search
zk search "authentication patterns"

# Filter by metadata
zk search --project myproject --category tethered
zk search --type todo --status open --priority high
zk search --tag golang --tag api    # AND filter

# Date range (for todos)
zk search --due-before 2026-03-01 --due-after 2026-02-01

# JSON output
zk search "query" --json --limit 50
```

## Validation

```bash
# Validate a single zettel against CUE schema + business rules
zk validate ./path/to/note.md
zk validate 20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee
```

## Note Management

```bash
# Tether an untethered note (requires project)
zk tether ./note.md --project myproject

# Untether a tethered note (removes project, sets category to untethered)
zk untether ./note.md

# Change project
zk set-project ./note.md newproject

# Add tags
zk add-tags ./note.md golang api security
```

## Daily Notes

```bash
zk daily                    # Today's daily note (creates if missing)
zk daily --yesterday        # Yesterday's note
zk daily --date 2026-02-10  # Specific date
zk daily --list             # List recent daily notes
zk daily --list --week      # This week's daily notes
```

## Todos

```bash
# Create
zk todo "Fix login bug" --priority high --due 2026-02-20 --project myproject

# List/search
zk todos                              # All open todos
zk todos --priority high --overdue    # High priority overdue
zk todos --project myproject --today  # Project todos due today
zk todos "fix bug" --json             # Full-text search, JSON output

# Update status
zk set-status ./todo.md in_progress
zk set-status ./todo.md closed

# Generate markdown todo list
zk todo-list --project myproject --output report.md
```
