---
description: Create, search, validate, and manage OKF Wiki concepts using the okfw CLI
globs:
  - "cli/**/*.go"
  - "**/*.md"
---

# Skill: OKF Wiki Concept Management

Use the `okfw` CLI to create, search, and manage concepts. The binary must be built first: `go build -o okfw ./cli/cmd/okfw`

## Concept Format

Every concept is a markdown file with YAML frontmatter. ID format: `YYYYMMDDHHmmss-UUIDv4`.

```yaml
---
id: "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
title: "My Concept Title"
type: "note"           # note | todo | daily-note | issue
project: "myproject"   # required for tethered, optional for untethered
category: "untethered" # untethered | tethered
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
# Basic concept (untethered, auto-detects project from git context)
okfw create "My Concept Title"

# Tethered concept with explicit project
okfw create "Design Doc" --category tethered --project myproject

# From template (meeting, book-review, snippet, project-idea, user-story, feature, daily, todo, issue)
okfw create "Sprint Planning" --template meeting --project myproject

# With tags
okfw create "Go Patterns" --tags golang,patterns

# List available templates
okfw templates
```

## Searching

Requires index: `okfw index` or `okfw index ~/okfw`

```bash
# Full-text search
okfw search "authentication patterns"

# Filter by metadata
okfw search --project myproject --category tethered
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
# Tether an untethered concept (requires project)
okfw tether ./concept.md --project myproject

# Untether a tethered concept (removes project, sets category to untethered)
okfw untether ./concept.md

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
