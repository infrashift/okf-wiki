---
title: Note Format
description: YAML frontmatter schema and fields for OKF Wiki concepts.
---

Concepts are markdown files with YAML frontmatter, and okfw checks them against a CUE schema. The schema implements the [Open Knowledge Format](https://github.com/open-knowledge-format/open-knowledge-format/blob/main/SPEC.md) **v0.3**, with a few deliberate divergences. [OKF conformance](#okf-conformance-and-divergences) below gives a summary, and [OKF v0.3 and OKF Wiki: where we differ](/okf-wiki/reference/okf-divergences/) explains them.

A plain note looks like this:

```markdown
---
x-okfw-id: "019f0494-00a8-7206-b788-c5bbfa2430a8"
title: "My Note Title"
type: "note"
description: "A one-sentence summary of the concept."  # Optional
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

- **`type`** is the OKF concept kind. Each concept **must have** a `type`. It is non-empty and free-form. Use it to tell what a concept *is*, with words that you choose (`note`, `Reference`, `BigQuery Table`, or a different word). okfw always writes it to frontmatter. By default, its value is the value of `x-okfw-kind`, so a plain note has `type: "note"`.
- **`x-okfw-kind`** is a structured enum (`note` | `daily-note` | `issue`) that controls how the CLI treats daily notes and issues. okfw **omits it from frontmatter when it is `note`** (the default), so plain notes stay clean.

In older concepts, a `type:` field can hold `note`, `daily-note` or `issue`. That value is now in `x-okfw-kind:`. The `type:` field is the free-form OKF kind.

## okfw's own keys: `x-okfw-*`

okfw adds these keys for itself: `x-okfw-id`, `x-okfw-kind`, `x-okfw-project`,
`x-okfw-parent`, `x-okfw-created` and `x-okfw-timestamp`. They are in the
extension namespace that OKF v0.3 §4.1 keeps for producers. No future OKF
version can define a key that starts with `x-`. The spec asks each producer to
use names of the form `x-<producer>-<name>`. So no OKF version can redefine one
of okfw's keys. Other OKF tools read them as extensions and do not change them.

:::note[Renamed in this release]
okfw used to write these keys bare: `id`, `kind`, `project`, `parent`,
`created`, `timestamp`. It still reads them, but `okfw validate` tells you to
run `okfw migrate`. That command renames them in each concept that okfw wrote. Any okfw
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

To retire a concept, keep the file. Deprecate it, and name the concept that
replaces it. Links and stored references to it continue to work. A reader that
asks for current knowledge only gets the concept that replaces it.

These fields are OKF **timestamps**: `stale_after`, `generated.at`,
`verified[].at`, `sources[].last_modified` and `usage_window`. A timestamp is
an RFC 3339 date-time with an explicit offset (`"2026-12-31T08:30:00+05:30"`).
It can also be a date (`"2026-12-31"`), which means the start of that day in
UTC. A date-time with
no offset is invalid. okfw's own writers store the date-time form. okfw's
own `x-okfw-created` and `x-okfw-timestamp` always use the date-time form.

## Trust fields

OKF spec §5.2–5.3. Both are optional; a concept with no trust metadata is perfectly consumable.

| Field | Format | Description |
|-------|--------|-------------|
| `generated` | `{by, at}` | Who produced the content, and when it last *meaningfully* changed. `by` is required within the block. Set both with `okfw set-generated` (`--at` defaults to now). A format, whitespace or link-repair edit does not move `at`. |
| `verified` | List of `{by, at}` | Verification events; both keys are required in an event, and one missing either is ignored. Multiple entries capture independent checks; "how recently" is the latest `at`. Append with `okfw verify`. |

**`generated.at` is not `x-okfw-timestamp`.** okfw moves `x-okfw-timestamp` on
each write. It moves `generated.at` only when the content changes. That is
`okfw set-generated`, or a body rewrite through okfw (the `append` and
`set_body` actions of the MCP update tool). These edits do not move it: new tags,
lifecycle changes, new sources and new verifications. The link repair of
`okfw move` in other concepts also does not move it. If they moved it, each of them could
make the reviews below invalid.

Actors follow the OKF rule (§7). An actor is `<scheme>:<id>`, or `<producer>/<version>` for an agent or tool. The scheme can be `human:` for a person, `team:` for a group, `process:` for an automated process, or a different scheme. The scheme is lowercase. Only `human:` gives the human-reviewed tier. So `okfw validate`, `verify` and `set-generated` refuse a scheme that is almost `human` (`Human:`, `humna:`, `humans:`). Such a typo records a human review that reads back as machine-confirmed. `sources[].author` follows the same rule, but okfw only gives a warning about it.

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

A concept can have one verifier as a bare mapping, without the list dash. okfw reads that mapping as a list with one item, as the spec requires.

okfw calculates the **trust tier** from the **current** `verified` events. These are the events at or after `generated.at`, or all events when there is no `generated`. Only the `human:` scheme gives a human review:

| current `verified` events | Tier |
|---|---|
| none | `unverified` |
| only non-`human:` actors (`team:`, `process:`, agents, other schemes) | `machine-confirmed` |
| any `human:<id>` actor | `human-reviewed` |

A review that is older than the last meaningful change stays in the file as history, but it does not count. When an agent rewrites a concept after a human review, the human did not read the new text. Check the concept, then run `okfw verify` again.

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

Footnotes use keys, not positions. With positions, a new order of the `sources` list gives each claim the wrong source, and nothing shows it. For the same reason, `okfw validate` rejects two sources with the same `id`. Markdown matches footnote labels without case, so `GA4` and `ga4` are the same label. It also rejects a `[^label]` that matches no `sources[].id` and no `[^label]:` definition, because that claim cites nothing. An ordinary footnote with its own definition is correct.

## Paths

Body links and path fields — `resource`, `sources[].resource`,
`computation`, `executor.resource`, `attester.resource` and `superseded_by` —
resolve by one rule (OKF §6):

- a value with a scheme (`https://…`) is a URL;
- a value that starts with `/` resolves from the bundle root: `/policies/refunds.md`;
- any other value resolves from the directory of the file it is in:
  `refunds.md`, `./refunds.md`, `../policies/refunds.md`.

okfw writes paths with a leading `/`. Such a path stays correct when the
concept that contains it moves. A space or other reserved character is percent-encoded
(`my%20note.md`). A path that climbs out of the bundle is external, and okfw
never reads it.

:::caution[Changed in OKF v0.3]
Under v0.2, okfw read a bare *frontmatter* path (`policies/refunds.md`) from
the bundle root. v0.3 reads it from the concept's directory. In a bundle that
still declares `okf_version: "0.2"`, or no version, okfw still reads such a
path from the bundle root as a fallback. Run `okfw migrate` to put a leading `/` on such paths and move the
bundle to v0.3. See the [migration guide](/okf-wiki/reference/migrating-to-okf-v0-3/).
:::

## Custom fields

okfw uses a closed schema, so an unknown key is an error. That rule finds `descrption:` before consumers ignore it with no sign. Give custom keys the form `x-<producer>-<name>`. OKF v0.3 promises never to define a key that starts with `x-`. okfw accepts such a key at each level, also in `sources` entries, `generated` and `verified` events:

```yaml
x-acme-review-cycle: "quarterly"
x-acme-jira-epic: "PLAT-1421"
sources:
  - resource: "https://example.com/schema"
    x-acme-confidence: 0.8
```

okfw still accepts the `x:` block of earlier okfw versions, and lets each value in it through. But put new metadata in `x-` keys. The spec does not reserve the name `x`, so a future OKF version can define it.

## OKF conformance and divergences

okfw knowingly breaks two of the spec's consumer rules, in exchange for a hard guarantee about the bundles it writes:

1. **okfw requires six fields and two bundle fields.** The six are `type`, `title` and `tags`, and okfw's `x-okfw-id`, `x-okfw-created` and `x-okfw-timestamp`. The bundle needs a `title` and a `description`. OKF v0.3 requires only a non-empty `type`.
2. **okfw uses a closed schema.** It rejects undeclared keys, except `x-*` extension keys. OKF §4.1 says that consumers MUST NOT reject documents with unknown keys.

The result: `okfw validate` rejects a *conformant third-party* bundle. Two commands answer the two questions:

- `okfw validate <concept>` and `okfw validate --all` apply okfw's strict schema.
- `okfw validate --okf [dir]` applies the permissive OKF §11.1 conformance check — frontmatter that parses as a YAML mapping with no duplicate keys, and well-formed `index.md`/`log.md` — and nothing else. It works on any directory, with or without a `.okfw`.

[OKF v0.3 and OKF Wiki: where we differ](/okf-wiki/reference/okf-divergences/) explains each divergence, why okfw makes it, and what it costs.

A `.md` file with no frontmatter, or no `type`, is not a concept (OKF §3.2) — a `README.md` in the bundle, say. Search, the graph and the listings skip it, and `validate` only warns.

**okfw has no task-tracking fields.** It once had `status`, `due`, `completed`, and `priority` for a `todo` kind. But OKF uses `status` for the lifecycle. The project removed the feature, and did not move it to a namespace. Run `okfw migrate` on a bundle from an older okfw. It removes the old fields and changes `status: closed` to `status: deprecated`.

okfw accepts Attested Computation (spec §10), but it does not execute it. The schema declares its keys (`runtime`, `parameters`, `computation`, `executor`, `attester`), so a bundle can hold such a concept. A concept of that `type` must have `runtime`. okfw runs and attests nothing. The spec does not yet define its receipt formats and attester ABI.

## Concept IDs and file paths

OKF gets the identity of a concept from the filesystem: its **id is its path in the wiki (relative to `.okfw/`), without `.md`**. A concept file has the name `slug--shortid.md`, for example `my-note-title--019f049400a8.md`. The short id is the first 12 hex digits of the UUID. When two short ids are the same, okfw makes the short id longer. So a note at `.okfw/infra/my-note-title--019f049400a8.md` has the OKF concept id `infra/my-note-title--019f049400a8`.

The `x-okfw-id:` field in frontmatter is a separate, okfw-specific identifier: a UUIDv7 that stays stable when a file moves. `x-okfw-parent:` refers to that full UUID, not to a path or a short prefix.

Each command that takes a concept accepts one of these:
- the full `x-okfw-id`;
- a unique id prefix of 8 or more hex digits (`019f0494`);
- the OKF concept ID (`infra/my-note-title--019f049400a8`), with or without `.md` or a leading `/`;
- a file path.

A link between concepts is a vanilla markdown link to a file path. The [notes workflow](/okf-wiki/neovim/notes-workflow/) shows how to add links.

:::note
To capture an idea quickly, create the note at the wiki root. File one in a directory later with `okfw move <note> <dir>/`, which repairs the links to it.
:::
