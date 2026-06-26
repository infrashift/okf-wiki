---
title: Note Format
description: YAML frontmatter schema and fields for OKF Wiki concepts.
---

Concepts are markdown files with YAML frontmatter, validated against a CUE schema. The schema conforms to the Open Knowledge Format (OKF).

A plain note looks like this:

```markdown
---
id: "019f0494-00a8-7206-b788-c5bbfa2430a8"
title: "My Note Title"
type: "note"
description: "A one-sentence summary of the concept."  # Optional
project: "my-project"
category: "untethered"
tags:
  - "idea"
  - "research"
created: "2026-02-13T10:45:00Z"
timestamp: "2026-02-13T10:45:00Z"
parent: "infra/some-parent--019f0312"  # Optional
---

# My Note Title

Your note content here...
```

## `type` vs `kind`

OKF Wiki separates two concerns that used to live in a single `type` field:

- **`type`** is the OKF concept kind. It is **required**, non-empty, and free-form — you can use it to describe what a concept *is* in your own vocabulary. It is always written to frontmatter. By default it is set to the value of `kind`, so a plain note is written as `type: "note"`.
- **`kind`** is a structured enum (`note` | `todo` | `daily-note` | `issue`) that drives the CLI's todo/daily/issue behavior. It is **omitted from frontmatter when it equals `note`** (the default), which keeps plain notes clean. When a concept is a todo, daily note, or issue, both `kind` and `type` are written (e.g. a todo carries `kind: "todo"` and `type: "todo"`).

If you previously saw a `type:` field holding `note`/`todo`/`daily-note`/`issue`, that behavior now lives in `kind:`. The `type:` field is the free-form OKF kind.

## Frontmatter Fields

| Field | Required | Format | Description |
|-------|----------|--------|-------------|
| `id` | Yes | UUIDv7 | Unique identifier (e.g. `019f0494-00a8-7206-b788-c5bbfa2430a8`). Legacy `YYYYMMDDHHmmss-UUID` IDs are still accepted. |
| `title` | Yes | Non-empty string | Concept title |
| `type` | Yes | Non-empty string | OKF concept kind (free-form). Always written; defaults to the `kind` value (so a plain note is `type: "note"`). |
| `kind` | No | `note`, `todo`, `daily-note`, or `issue` | Structured kind that drives behavior. Omitted from frontmatter when it equals `note` (default). |
| `description` | No | One-sentence string | Optional OKF summary of the concept |
| `resource` | No | URI | Optional OKF reference to an underlying asset (e.g. a file or URL the concept describes) |
| `project` | Untethered: No, Tethered: Yes | Non-empty string | Project context (auto-detected from git) |
| `category` | Yes | `untethered` or `tethered` | Concept category |
| `tags` | Yes | List of non-empty strings | Categorization tags |
| `created` | Yes | RFC3339 timestamp | Creation time |
| `timestamp` | Yes | RFC3339 timestamp | OKF "last meaningful change". Updated by the CLI on every mutation. |
| `parent` | No | Concept id | Parent concept for hierarchies. A concept's OKF id is its bundle-relative path minus `.md`. |

## Todo-specific fields

When `kind: "todo"`:

| Field | Required | Format | Description |
|-------|----------|--------|-------------|
| `status` | Yes | `open`, `in_progress`, `closed` | Task status |
| `due` | No | `YYYY-MM-DD` | Due date |
| `completed` | No | `YYYY-MM-DD` | Completion date (set automatically) |
| `priority` | No | `high`, `medium`, `low` | Task priority |

## Concept IDs and file paths

A concept's OKF **id is its bundle-relative path minus `.md`**. Concept files are named `slug--shortid.md` (for example `my-note-title--019f0494.md`), where the short id is the leading hex of the UUID. So a note stored at `infra/my-note-title--019f0494.md` has the OKF id `infra/my-note-title--019f0494`.

Links between concepts are vanilla markdown links that point at these paths. See the [Notes Workflow](/zettelkasten/neovim/notes-workflow/) for how linking works.

:::note
Untethered notes can be created without a project context for quick idea capture. When tethering to a tethered note, a project is required.
:::
</content>
</invoke>
