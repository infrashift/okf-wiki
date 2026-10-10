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
found. `okfw connect` verifies the wiki and prints its concept count; `okfw root`
prints the discovered `.okfw` path. Generated artifacts inside `.okfw/`:
`ephemeral/` (exports) — the only generated directory
— all gitignored; concept markdown is committed with the project.

## Concept Format

Every concept is a markdown file with YAML frontmatter, named
`slug--<first 12 hex digits of the id>.md`. ID format: UUIDv7 (legacy
`YYYYMMDDHHmmss-UUIDv4` still accepted). Commands that take a concept accept
the full id, a unique id prefix of at least 8 hex digits, the OKF concept ID
(bundle-relative path minus `.md`), or a file path.

```yaml
---
id: "019f0494-00a8-7206-b788-c5bbfa2430a8"
title: "My Concept Title"
type: "note"           # OKF free-form concept kind; required, non-empty
kind: "issue"          # note (default, omitted) | daily-note | issue
project: "myproject"   # always optional, never derived from git
tags:
  - "golang"
  - "architecture"
created: "2026-02-13T10:45:00Z"
timestamp: "2026-02-13T10:45:00Z"
status: "deprecated"   # OKF lifecycle: draft | stable (default) | deprecated
stale_after: "2026-12-31T00:00:00Z"
parent: "019f0312-4700-7def-9abc-ffffffffffff"  # optional; the parent's full id
x:                     # your own metadata; the only place custom keys are legal
  jira_epic: "PLAT-1421"
---

# My Concept Title

Body content with vanilla markdown links: [Display Title](/dir/other--019f04a10000.md)
(bundle-relative, recommended) or [Display Title](./other--019f04a10000.md).
```

## Creating Concepts

```bash
# Basic concept (lands at the wiki root, no project metadata)
okfw create "My Concept Title"

# Concept with explicit project
okfw create "Design Doc" --project myproject

# From template (meeting, book-review, snippet, project-idea, user-story, feature, daily, issue)
okfw create "Sprint Planning" --template meeting --project myproject

# With tags
okfw create "Go Patterns" --tags golang,patterns

# List available templates
okfw templates
```

## Searching

No index to build: search scans the concepts and ranks them, so a concept is
findable the moment it is written and gone the moment it is deleted.

```bash
# Full-text search
okfw search "authentication patterns"

# Filter by metadata
okfw search --project myproject
okfw search --status draft              # OKF lifecycle status
okfw search --stale                    # stale_after has passed
okfw search --tag golang --tag api    # AND filter
okfw search --kind issue               # structured kind

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

## Reorganizing and removing

Use these rather than `mv`/`rm`: they repair links at both ends and refresh the
`index.md` listings.

```bash
okfw move <ref> archive/              # into a directory (id is preserved)
okfw move <ref> notes/new-name.md     # new path and name
okfw delete <ref> --force             # file + its index.md line
```

`move` retargets every inbound link and rewrites the moved concept's own relative
links so they still resolve. `delete` reports concepts still linking to the
removed one but does not edit them — OKF requires consumers to tolerate broken
links.

## OKF metadata

okfw has **no task tracking** — `status` is the OKF lifecycle field, not task
progress. Run `okfw migrate` on a wiki written by an older okfw.

```bash
# Lifecycle
okfw set-lifecycle ./note.md deprecated     # draft | stable | deprecated
okfw set-stale-after ./note.md 2026-12-31T00:00:00Z
okfw set-stale-after ./note.md --clear

# Trust: who reviewed this, and when
okfw verify ./note.md                       # defaults to human:<login>
okfw verify ./note.md --by process:nightly-audit
okfw set-generated ./note.md --by human:ryan

# Provenance: where the knowledge came from
okfw add-source ./note.md --resource https://example.com/schema --id ga4
okfw set-resource ./note.md bigquery://project.dataset.orders

# Validation
okfw validate ./note.md      # strict okfw schema, one concept
okfw validate --all          # ...every concept in the wiki
okfw validate --okf          # permissive OKF conformance check instead
```
