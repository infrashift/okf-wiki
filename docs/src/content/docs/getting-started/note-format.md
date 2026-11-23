---
title: Note Format
description: YAML frontmatter schema and fields for OKF Wiki concepts.
---

Concepts are markdown files with YAML frontmatter, validated against a CUE schema. The schema implements the [Open Knowledge Format](https://github.com/open-knowledge-format/open-knowledge-format/blob/main/SPEC.md) **v0.3**, with a few deliberate divergences summarized in [OKF conformance](#okf-conformance-and-divergences) below and explained in [OKF v0.3 and OKF Wiki: where we differ](/okf-wiki/reference/okf-divergences/).

A plain note looks like this:

```markdown
---
x-okfw-id: "019f0494-00a8-7206-b788-c5bbfa2430a8"
title: "My Note Title"
type: "note"
description: "A one-sentence summary of the concept."  # Optional
x-okfw-project: "my-project"
tags:
  - "idea"
  - "research"
x-okfw-created: "2026-02-13T10:45:00Z"
x-okfw-timestamp: "2026-02-13T10:45:00Z"
x-okfw-parent: "019f0312-4700-7def-9abc-ffffffffffff"  # Optional
---

# My Note Title

Your note content here...
```

## `type` vs `x-okfw-kind`

OKF Wiki separates two concerns that used to live in a single `type` field:

- **`type`** is the OKF concept kind. It is **required**, non-empty, and free-form — you can use it to describe what a concept *is* in your own vocabulary (`note`, `Reference`, `BigQuery Table`, whatever fits). It is always written to frontmatter. By default it is set to the value of `x-okfw-kind`, so a plain note is written as `type: "note"`.
- **`x-okfw-kind`** is a structured enum (`note` | `daily-note` | `issue`) that drives the CLI's daily-note and issue behavior. It is **omitted from frontmatter when it equals `note`** (the default), which keeps plain notes clean.

If you previously saw a `type:` field holding `note`/`daily-note`/`issue`, that behavior now lives in `x-okfw-kind:`. The `type:` field is the free-form OKF kind.

## okfw's own keys: `x-okfw-*`

The keys okfw adds for itself — `x-okfw-id`, `x-okfw-kind`, `x-okfw-project`,
`x-okfw-parent`, `x-okfw-created` and `x-okfw-timestamp` — live in the
extension namespace OKF v0.3 §4.1 sets aside for producers: future OKF
versions will never define a key starting with `x-`, and producers are asked
to name theirs `x-<producer>-<name>`. So no OKF version can ever give one of
okfw's keys a different meaning, and other OKF tools read them as an
extension and leave them alone.

:::note[Renamed in this release]
okfw used to write these keys bare: `id`, `kind`, `project`, `parent`,
`created`, `timestamp`. It still reads them, but `okfw validate` asks you to
run `okfw migrate`, which renames them in every concept okfw wrote. Any okfw
edit of a concept also renames its keys. A bare `id` in a concept another tool
wrote is that tool's, and okfw leaves it alone.
:::

## Core fields

| Field | Required | Format | Description |
|-------|----------|--------|-------------|
| `x-okfw-id` | Yes | UUIDv7 | Unique identifier (e.g. `019f0494-00a8-7206-b788-c5bbfa2430a8`). Legacy `YYYYMMDDHHmmss-UUID` IDs are still accepted. An okfw extension — see [Concept IDs](#concept-ids-and-file-paths). |
| `title` | Yes | Non-empty string | Concept title |
| `type` | Yes | Non-empty string | OKF concept kind (free-form). Always written; defaults to the `x-okfw-kind` value. |
| `x-okfw-kind` | No | `note`, `daily-note`, or `issue` | Structured kind that drives behavior. Omitted when it equals `note` (default). |
| `description` | No | One-sentence string | OKF summary, used in `index.md` listings and search snippets |
| `resource` | No | URI | OKF reference to the underlying asset the concept describes. Set with `okfw set-resource`. |
| `x-okfw-project` | No | Non-empty string | **Deprecated.** Project metadata from the deprecated `--project` flag. Still read and shown; organize by directory (`--path`) instead. |
| `tags` | Yes | List of non-empty strings | Categorization tags |
| `x-okfw-created` | Yes | ISO 8601 datetime with offset | Creation time |
| `x-okfw-timestamp` | Yes | ISO 8601 datetime with offset | okfw's last-write time, updated by the CLI on every mutation. Not the same as `generated.at` — see [Trust fields](#trust-fields). |
| `x-okfw-parent` | No | Concept id (UUID) | Parent concept for hierarchies |
| `x-<producer>-<name>` | No | Anything | Your own metadata, at any level — see [Custom fields](#custom-fields). |

## Lifecycle fields

OKF spec §5.4–5.5. An absent `status` means `stable`.

| Field | Format | Description |
|-------|--------|-------------|
| `status` | `draft`, `stable`, `deprecated` | `draft`: not yet reviewed, possibly incomplete. `stable`: ready for consumption (the default). `deprecated`: kept for links and history, no longer current. Set with `okfw set-lifecycle`. A value outside these three (from another tool) is shown as written and treated as not current. |
| `superseded_by` | Path | The concept that replaces a deprecated one. Meaningful only with `status: deprecated`. Set with `okfw set-lifecycle <ref> deprecated --superseded-by <new>`; `okfw search --current` follows it. It is a link, so `okfw move` keeps it pointing at the replacement. |
| `stale_after` | Timestamp | A concept is **stale** once now is on or after this instant. An absolute instant rather than a TTL, so staleness is a plain comparison that needs no record of when the concept was read. A value that cannot be parsed counts as stale. Set with `okfw set-stale-after`; find them with `okfw search --stale`. |

```yaml
status: "deprecated"
superseded_by: "/metrics/gross-margin.md"
stale_after: "2026-12-31T00:00:00Z"
```

To retire a concept, keep the file, deprecate it and name its replacement:
links and stored references to it keep working, and a reader filtering to
current knowledge is sent on to the replacement.

An OKF **timestamp** — `stale_after`, `generated.at`, `verified[].at`,
`sources[].last_modified` and `usage_window` — is either an RFC 3339
date-time with an explicit offset (`"2026-12-31T08:30:00+05:30"`) or a date
(`"2026-12-31"`), which means the start of that day in UTC. A date-time with
no offset is invalid. okfw's own writers store the date-time form. okfw's
own `x-okfw-created` and `x-okfw-timestamp` always use the date-time form.

## Trust fields

OKF spec §5.2–5.3. Both are optional; a concept with no trust metadata is perfectly consumable.

| Field | Format | Description |
|-------|--------|-------------|
| `generated` | `{by, at}` | Who produced the content, and when it last *meaningfully* changed. `by` is required within the block. Set both with `okfw set-generated` (`--at` defaults to now). A format, whitespace or link-repair edit does not move `at`. |
| `verified` | List of `{by, at}` | Verification events; both keys are required in an event, and one missing either is ignored. Multiple entries capture independent checks; "how recently" is the latest `at`. Append with `okfw verify`. |

**`generated.at` is not `x-okfw-timestamp`.** okfw moves `x-okfw-timestamp` on every write,
but `generated.at` only when the content changes: through `okfw
set-generated`, or a body rewrite made through okfw (the MCP update tool's
`append` and `set_body`). Adding a tag, changing the lifecycle, adding a source,
recording a verification, or the link repair `okfw move` does in other
concepts, leave it alone — otherwise each of those would invalidate the
reviews below.

Actors follow the OKF convention (§7): `<scheme>:<id>` — `human:` for a person, `team:` for a group, `process:` for an automated process, or another scheme — or `<producer>/<version>` for an agent or tool. The scheme is lowercase. Only `human:` gives the human-reviewed tier, so `okfw validate`, `verify` and `set-generated` refuse a scheme that is a near miss of it (`Human:`, `humna:`, `humans:`), which would otherwise record a human review that reads back as machine-confirmed. `sources[].author` follows the same convention, but okfw only warns about it.

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

**Trust tiers** are derived from the **current** `verified` events — those at or after `generated.at` (all of them when there is no `generated`) — and key strictly on the `human:` scheme:

| current `verified` events | Tier |
|---|---|
| none | `unverified` |
| only non-`human:` actors (`team:`, `process:`, agents, other schemes) | `machine-confirmed` |
| any `human:<id>` actor | `human-reviewed` |

A review older than the last meaningful change stays in the file as history but no longer counts: when an agent rewrites a concept after a human review, the human has not read the new text. Run `okfw verify` again after checking it.

## Provenance fields

OKF spec §5.1. Append entries with `okfw add-source`.

| Field | Format | Description |
|-------|--------|-------------|
| `sources` | List | Where the concept's knowledge came from |
| `sources[].resource` | String | **Required within an entry.** Either a concrete artifact a consumer can follow (a URL or a path in the bundle) or a scope descriptor it cannot, such as `all queries in project X`. A path naming another concept is a link: it shows in backlinks and the graph, `okfw move` keeps it pointing at the concept, and `gf` follows it. |
| `sources[].id` | String | Join key for body footnotes — see below |
| `sources[].title` | String | Human-readable label |
| `sources[].author` | Actor | Who or what produced the source (`team:ga4-docs`) |
| `sources[].usage_count` | Integer | How often the resource was exercised over `usage_window`. Read it as liveness and trend, not as a score. |
| `sources[].last_modified` | Timestamp | When the source itself last changed — distinct from `generated.at`, the last meaningful change to the concept |
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

## Paths

Body links and path fields — `resource`, `sources[].resource`,
`computation`, `executor.resource`, `attester.resource` and `superseded_by` —
resolve by one rule (OKF §6):

- a value with a scheme (`https://…`) is a URL;
- a value starting with `/` resolves from the bundle root: `/policies/refunds.md`;
- any other value resolves from the directory of the file it is in:
  `refunds.md`, `./refunds.md`, `../policies/refunds.md`.

okfw writes paths with a leading `/`, which stays correct when the referring
concept moves. A space or other reserved character is percent-encoded
(`my%20note.md`). A path that climbs out of the bundle is external, and okfw
never reads it.

:::caution[Changed in OKF v0.3]
Under v0.2, okfw read a bare *frontmatter* path (`policies/refunds.md`) from
the bundle root. v0.3 reads it from the concept's directory. A bundle that
still declares `okf_version: "0.3"` keeps the old reading as a fallback; run
`okfw migrate` to rewrite such paths with a leading `/` and move the bundle to
v0.3. See the [migration guide](/okf-wiki/reference/migrating-to-okf-v0-3/).
:::

## Custom fields

The schema is closed, so an unrecognized key is an error — that is what catches `descrption:` before it becomes a silently ignored field. Name your own keys `x-<producer>-<name>`: OKF v0.3 promises never to define a key starting with `x-`, and okfw accepts one at any level, including inside `sources` entries, `generated` and `verified` events:

```yaml
x-acme-review-cycle: "quarterly"
x-acme-jira-epic: "PLAT-1421"
sources:
  - resource: "https://example.com/schema"
    x-acme-confidence: 0.8
```

The `x:` block from earlier okfw versions is still accepted and passes anything through, but new metadata belongs in `x-` keys: `x` itself is not a reserved name, so a future OKF version could define it.

## OKF conformance and divergences

okfw knowingly breaks two of the spec's consumer rules, in exchange for a hard guarantee about the bundles it writes:

1. **okfw requires six fields** — `type`, `title` and `tags`, and its own `x-okfw-id`, `x-okfw-created` and `x-okfw-timestamp` — and a bundle `title` and `description`. OKF v0.3 requires only a non-empty `type`.
2. **The schema is closed.** Undeclared keys are rejected, except `x-*` extension keys, where OKF §4.1 says consumers MUST NOT reject documents with unknown keys.

The consequence: `okfw validate` will reject a *conformant third-party* bundle. Two commands exist for the two questions:

- `okfw validate <concept>` and `okfw validate --all` apply okfw's strict schema.
- `okfw validate --okf [dir]` applies the permissive OKF §11.1 conformance check — frontmatter that parses as a YAML mapping with no duplicate keys, and well-formed `index.md`/`log.md` — and nothing else. It works on any directory, with or without a `.okfw`.

[OKF v0.3 and OKF Wiki: where we differ](/okf-wiki/reference/okf-divergences/) explains each divergence, why okfw makes it, and what it costs.

A `.md` file with no frontmatter, or no `type`, is not a concept (OKF §3.2) — a `README.md` in the bundle, say. Search, the graph and the listings skip it, and `validate` only warns.

**okfw has no task-tracking fields.** It once carried `status`, `due`, `completed`, and `priority` for a `todo` kind, where `status` collided with the field OKF spec'd for lifecycle. The feature was removed rather than namespaced. Run `okfw migrate` on a bundle written by an older okfw: it drops the retired fields and rewrites `status: closed` to `status: deprecated`.

Attested Computation (spec §10) is accepted but not executed: the schema declares its keys (`runtime`, `parameters`, `computation`, `executor`, `attester`) so a bundle can hold such a concept, and a concept of that `type` must carry `runtime`. okfw runs and attests nothing — the spec still defers its receipt formats and attester ABI to a future revision.

## Concept IDs and file paths

OKF derives a concept's identity from the filesystem: its **id is its path within the wiki (relative to `.okfw/`) minus `.md`**. Concept files are named `slug--shortid.md` (for example `my-note-title--019f049400a8.md`), where the short id is the first 12 hex digits of the UUID (extended on a collision). So a note stored at `.okfw/infra/my-note-title--019f049400a8.md` has the OKF concept id `infra/my-note-title--019f049400a8`.

The `x-okfw-id:` field in frontmatter is a separate, okfw-specific identifier: a UUIDv7 that stays stable when a file moves. `x-okfw-parent:` refers to that full UUID, not to a path or a short prefix.

Every command that takes a concept accepts any of: the full `x-okfw-id`, a unique id prefix of at least 8 hex digits (`019f0494`), the OKF concept ID (`infra/my-note-title--019f049400a8`, optionally with `.md` or a leading `/`), or a file path.

Links between concepts are vanilla markdown links pointing at file paths. See the [Notes Workflow](/okf-wiki/neovim/notes-workflow/) for how linking works.

:::note
Notes can be created at the wiki root for quick idea capture. File one in a directory later with `okfw move <note> <dir>/`, which repairs the links to it.
:::
