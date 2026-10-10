---
title: Note Format
description: YAML frontmatter schema and fields for OKF Wiki concepts.
---

Concepts are markdown files with YAML frontmatter, validated against a CUE schema. The schema implements the [Open Knowledge Format](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md) **v0.2**, with two deliberate divergences described in [OKF conformance](#okf-conformance-and-divergences) below.

A plain note looks like this:

```markdown
---
id: "019f0494-00a8-7206-b788-c5bbfa2430a8"
title: "My Note Title"
type: "note"
description: "A one-sentence summary of the concept."  # Optional
project: "my-project"
tags:
  - "idea"
  - "research"
created: "2026-02-13T10:45:00Z"
timestamp: "2026-02-13T10:45:00Z"
parent: "019f0312-4700-7def-9abc-ffffffffffff"  # Optional
---

# My Note Title

Your note content here...
```

## `type` vs `kind`

OKF Wiki separates two concerns that used to live in a single `type` field:

- **`type`** is the OKF concept kind. It is **required**, non-empty, and free-form — you can use it to describe what a concept *is* in your own vocabulary (`note`, `Reference`, `BigQuery Table`, whatever fits). It is always written to frontmatter. By default it is set to the value of `kind`, so a plain note is written as `type: "note"`.
- **`kind`** is a structured enum (`note` | `daily-note` | `issue`) that drives the CLI's daily-note and issue behavior. It is **omitted from frontmatter when it equals `note`** (the default), which keeps plain notes clean.

If you previously saw a `type:` field holding `note`/`daily-note`/`issue`, that behavior now lives in `kind:`. The `type:` field is the free-form OKF kind.

## Core fields

| Field | Required | Format | Description |
|-------|----------|--------|-------------|
| `id` | Yes | UUIDv7 | Unique identifier (e.g. `019f0494-00a8-7206-b788-c5bbfa2430a8`). Legacy `YYYYMMDDHHmmss-UUID` IDs are still accepted. An okfw extension — see [Concept IDs](#concept-ids-and-file-paths). |
| `title` | Yes | Non-empty string | Concept title |
| `type` | Yes | Non-empty string | OKF concept kind (free-form). Always written; defaults to the `kind` value. |
| `kind` | No | `note`, `daily-note`, or `issue` | Structured kind that drives behavior. Omitted when it equals `note` (default). |
| `description` | No | One-sentence string | OKF summary, used in `index.md` listings and search snippets |
| `resource` | No | URI | OKF reference to the underlying asset the concept describes. Set with `okfw set-resource`. |
| `project` | No | Non-empty string | Optional project metadata (set with `--project`). Never derived from git. |
| `tags` | Yes | List of non-empty strings | Categorization tags |
| `created` | Yes | RFC3339 timestamp | Creation time |
| `timestamp` | Yes | RFC3339 timestamp | OKF "last meaningful change". Updated by the CLI on every mutation. |
| `parent` | No | Concept id (UUID) | Parent concept for hierarchies |
| `x` | No | Mapping | Your own metadata. The only place custom keys are accepted — see [Custom fields](#custom-fields). |

## Lifecycle fields

OKF spec §5.4–5.5. An absent `status` means `stable`.

| Field | Format | Description |
|-------|--------|-------------|
| `status` | `draft`, `stable`, `deprecated` | `draft`: not yet reviewed, possibly incomplete. `stable`: ready for consumption (the default). `deprecated`: kept for links and history, no longer current. Set with `okfw set-lifecycle`. |
| `stale_after` | ISO 8601 datetime with offset | A concept is **stale** once now is on or after this instant. An absolute instant rather than a TTL, so staleness is a plain comparison that needs no record of when the concept was read. Set with `okfw set-stale-after`; find them with `okfw search --stale`. |

```yaml
status: "deprecated"
stale_after: "2026-12-31T00:00:00Z"
```

Every timestamp in OKF — `stale_after`, `generated.at`, `verified[].at`,
`sources[].last_modified` and `usage_window` — is an ISO 8601 datetime with an
explicit UTC offset. A bare `2026-12-31` names a different instant in every
timezone, so `okfw validate` rejects it; the writers accept one and store it as
midnight UTC, and `okfw migrate` upgrades the bare dates older okfw versions
wrote.

## Trust fields

OKF spec §5.2–5.3. Both are optional; a concept with no trust metadata is perfectly consumable.

| Field | Format | Description |
|-------|--------|-------------|
| `generated` | `{by, at}` | Who produced the content and when. `by` is required within the block. `at` mirrors `timestamp` — okfw keeps them equal and rejects a disagreement, since both record the same event. Set `by` with `okfw set-generated`. |
| `verified` | List of `{by, at}` | Verification events. Multiple entries capture independent checks; "how recently" is the latest `at`. Append with `okfw verify`. |

Actors follow the OKF convention (§7): `human:<id>` for a person, `process:<id>` for an automated process, `<producer>/<version>` for an agent or tool. `okfw validate` holds `generated.by` and every `verified[].by` to exactly those three forms, prefix in lowercase, and `verify`/`set-generated` refuse anything else. The trust tier below keys on the exact prefix `human:`, so `Human:ryan` or `humna:ryan` would otherwise record a human review that reads back as machine-confirmed. `sources[].author` is not checked: the spec's own examples use `team:` there, and it does not feed the trust tier.

```yaml
generated:
  by: "human:ryan"
  at: "2026-07-25T12:00:00Z"
verified:
  - by: "process:nightly-audit"
    at: "2026-07-20T10:00:00Z"
  - by: "human:ryan"
    at: "2026-07-25T12:00:00Z"
```

A single verifier may be written as a bare mapping without the list dash; okfw reads that as a one-element list, as the spec requires.

**Trust tiers** are derived from `verified`, and key strictly on the `human:` prefix:

| `verified` contains | Tier |
|---|---|
| nothing (key absent) | `unverified` |
| only non-`human:` actors | `machine-confirmed` |
| any `human:<id>` actor | `human-reviewed` |

A concept from another tool may carry an actor outside the convention, such as `team:data-platform`. okfw still reads it, and it is *not* a `human:` actor, so it yields `machine-confirmed`; `okfw validate` reports it.

## Provenance fields

OKF spec §5.1. Append entries with `okfw add-source`.

| Field | Format | Description |
|-------|--------|-------------|
| `sources` | List | Where the concept's knowledge came from |
| `sources[].resource` | String | **Required within an entry.** Either a concrete artifact a consumer can follow (a URL or bundle-relative path) or a scope descriptor it cannot, such as `all queries in project X`. A path naming another concept is a link: it shows in backlinks and the graph, `okfw move` keeps it pointing at the concept, and `gf` follows it. A path starting `./` or `../` resolves from the citing concept's directory. Any other path resolves from the bundle root, including a bare `policies/refunds.md`. In a body link, a bare path is relative. This rule applies to every path-valued field (`resource`, `computation`, `executor.resource`, `attester.resource`), and it matches the spec's own examples. |
| `sources[].id` | String | Join key for body footnotes — see below |
| `sources[].title` | String | Human-readable label |
| `sources[].author` | Actor | Who or what produced the source |
| `sources[].usage_count` | Integer | How often the resource was exercised over `usage_window`. Read it as liveness and trend, not as a score. |
| `sources[].last_modified` | ISO 8601 datetime with offset | When the source itself last changed — distinct from `generated.at`, which is when the concept was written |
| `usage_window` | `{from, to}` | Written once as a sibling of `sources`, framing every `usage_count`. A single entry may carry its own to override it. |

```yaml
usage_window:
  from: "2026-01-01T00:00:00Z"
  to: "2026-06-30T00:00:00Z"
sources:
  - resource: "https://example.com/schema"
    id: "ga4-schema"
    title: "GA4 BigQuery Export schema"
    usage_count: 412
```

To attribute a **specific claim**, use a markdown footnote whose label is a `sources[].id`:

```markdown
The `events_` table is sharded daily as `events_YYYYMMDD`.[^ga4-schema]

[^ga4-schema]: GA4 BigQuery Export schema
```

Labels are keyed rather than positional because reordering the `sources` list would otherwise silently misattribute every claim. For the same reason `okfw validate` rejects two sources sharing an `id` (markdown matches footnote labels case-insensitively, so `GA4` and `ga4` collide), and a `[^label]` that matches neither a `sources[].id` nor a `[^label]:` definition, since that claim cites nothing. An ordinary footnote with its own definition is fine.

## Custom fields

The schema is closed, so an unrecognized top-level key is an error — that is what catches `descrption:` before it becomes a silently ignored field. Put your own metadata under `x:`, which accepts anything:

```yaml
x:
  review_cycle: "quarterly"
  jira_epic: "PLAT-1421"
  owners:
    - "alice"
    - "bob"
```

## OKF conformance and divergences

okfw knowingly breaks two of the spec's consumer rules, in exchange for a hard guarantee about the bundles it writes:

1. **okfw requires six fields** — `id`, `title`, `type`, `tags`, `created`, `timestamp`. OKF §11 requires only a non-empty `type`, and says consumers MUST NOT reject a concept for missing optional fields.
2. **The schema is closed.** Undeclared top-level keys are rejected, where OKF §4.1 says consumers MUST NOT reject documents with unrecognized fields. The `x:` block above is the escape hatch.

The consequence: `okfw validate` will reject a *conformant third-party* bundle. Two commands exist for the two questions:

- `okfw validate <concept>` and `okfw validate --all` apply okfw's strict schema.
- `okfw validate --okf` applies the permissive OKF §11 conformance check — parseable frontmatter with a non-empty `type` — and nothing else.

**okfw has no task-tracking fields.** It once carried `status`, `due`, `completed`, and `priority` for a `todo` kind, where `status` collided with the field OKF v0.2 spec'd for lifecycle. The feature was removed rather than namespaced. Run `okfw migrate` on a bundle written by an older okfw: it drops the retired fields and rewrites `status: closed` to `status: deprecated`.

Attested Computation (spec §10) is accepted but not executed: the schema declares its keys (`runtime`, `parameters`, `computation`, `executor`, `attester`) so a bundle can hold such a concept, and a concept of that `type` must carry `runtime`. okfw runs and attests nothing — the spec still defers its receipt formats and attester ABI to a future revision.

## Concept IDs and file paths

OKF derives a concept's identity from the filesystem: its **id is its path within the wiki (relative to `.okfw/`) minus `.md`**. Concept files are named `slug--shortid.md` (for example `my-note-title--019f049400a8.md`), where the short id is the first 12 hex digits of the UUID (extended on a collision). So a note stored at `.okfw/infra/my-note-title--019f049400a8.md` has the OKF concept id `infra/my-note-title--019f049400a8`.

The `id:` field in frontmatter is a separate, okfw-specific identifier: a UUIDv7 that stays stable when a file moves. `parent:` refers to that full UUID, not to a path or a short prefix.

Every command that takes a concept accepts any of: the full frontmatter `id`, a unique id prefix of at least 8 hex digits (`019f0494`), the OKF concept ID (`infra/my-note-title--019f049400a8`, optionally with `.md` or a leading `/`), or a file path.

Links between concepts are vanilla markdown links pointing at file paths. See the [Notes Workflow](/okf-wiki/neovim/notes-workflow/) for how linking works.

:::note
Notes can be created without a project context for quick idea capture. Assign a project later with `okfw set-project`.
:::
