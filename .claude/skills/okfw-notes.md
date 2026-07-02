---
description: Create, search, validate, and manage OKF Wiki concepts using the okfw CLI
globs:
  - "cli/**/*.go"
  - "**/*.md"
---

# Skill: OKF Wiki Concept Management

Use the `okfw` CLI to create, search, and manage concepts. The binary must be built first: `go build -o okfw ./cli/cmd/okfw`

## Wiki Location

A wiki is a per-project `.okfw/` directory at the project root, created once
with `okfw init` (no-op if it already exists). There is no config file. Every
command discovers the wiki by searching upward from the current working
directory (like git finds `.git`) and errors with init advice when none is
found. `okfw connect` verifies the wiki and prints a summary; `okfw root`
prints the discovered `.okfw` path. Generated artifacts inside `.okfw/`:
`.index/` (search index), `.graph.db` (graph database), `.todos/` (todo
lists), `ephemeral/` (exports) — all gitignored; concept markdown is
committed with the project.

## Concept Format

Every concept is a markdown file with YAML frontmatter. ID format: `YYYYMMDDHHmmss-UUIDv4`.

```yaml
---
id: "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
title: "My Concept Title"
type: "note"           # note | todo | daily-note | issue
project: "myproject"   # always optional
tags:
  - "golang"
  - "architecture"
created: "2026-02-13T10:45:00Z"
parent: "20260212..."  # optional, links to parent concept ID
---

# My Concept Title

Body content with [[20260212...-uuid|Display Title]] style links.
```

## Creating Concepts

```bash
# Basic concept (lands at the wiki root, no project metadata)
okfw create "My Concept Title"

# Concept with explicit project
okfw create "Design Doc" --project myproject

# From template (meeting, book-review, snippet, project-idea, user-story, feature, daily, todo, issue)
okfw create "Sprint Planning" --template meeting --project myproject

# With tags
okfw create "Go Patterns" --tags golang,patterns

# List available templates
okfw templates
```

## Searching

Requires index: `okfw index` (the wiki is discovered automatically) or `okfw index path/to/.okfw`

```bash
# Full-text search
okfw search "authentication patterns"

# Filter by metadata
okfw search --project myproject
okfw search --type todo --status open --priority high
okfw search --tag golang --tag api    # AND filter

# Date range (for todos)
okfw search --due-before 2026-03-01 --due-after 2026-02-01

# JSON output
okfw search "query" --json --limit 50
```

## Validation

```bash
# Validate a single concept against CUE schema + business rules
okfw validate ./path/to/concept.md
okfw validate 20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee
```

## Concept Management

```bash
# Change project
okfw set-project ./concept.md newproject

# Add tags
okfw add-tags ./concept.md golang api security
```

## Daily Notes

```bash
okfw daily                    # Today's daily note (creates if missing)
okfw daily --yesterday        # Yesterday's note
okfw daily --date 2026-02-10  # Specific date
okfw daily --list             # List recent daily notes
okfw daily --list --week      # This week's daily notes
```

## Todos

```bash
# Create
okfw todo "Fix login bug" --priority high --due 2026-02-20 --project myproject

# List/search
okfw todos                              # All open todos
okfw todos --priority high --overdue    # High priority overdue
okfw todos --project myproject --today  # Project todos due today
okfw todos "fix bug" --json             # Full-text search, JSON output

# Update status
okfw set-status ./todo.md in_progress
okfw set-status ./todo.md closed

# Generate markdown todo list
okfw todo-list --project myproject --output report.md
```
