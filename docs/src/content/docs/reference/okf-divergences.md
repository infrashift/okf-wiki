---
title: "OKF v0.3 and OKF Wiki: where we differ"
description: Where okfw is stricter than, or departs from, the Open Knowledge Format v0.3 specification, why, and what it costs.
---

OKF Wiki implements the [Open Knowledge Format v0.3](https://github.com/open-knowledge-format/open-knowledge-format/blob/main/SPEC.md).
Each bundle that okfw **writes** is a conformant OKF v0.3 bundle. okfw
**reads** each conformant bundle: search, the link graph and
`okfw validate --okf` accept what the spec accepts. okfw's tests include the
four example bundles of the OKF repository. For each of them,
`okfw validate --okf` must report no violation, and each link must resolve.

For the bundles that okfw **authors**, okfw is stricter than the spec on
purpose. The spec sets a minimum that each producer can meet. okfw adds a few
guarantees, because without them, a knowledge base that agents write all the
time becomes worse with no sign. This page gives each guarantee, the rule that
it differs from, the reason, and the result.

## Summary

| okfw | OKF v0.3 | Where it applies |
|---|---|---|
| Six required concept fields | Only `type` is required (§4.1) | `okfw validate`, writers |
| A closed schema: unknown keys are errors, except `x-*` | Consumers MUST NOT reject unknown keys (§4.1, §11.2) | `okfw validate`, writers |
| Bundle `title` and `description` required | Both optional (§8.1) | `okfw validate --all`, `okfw init` |
| Near-miss `human:` actor schemes refused | Any scheme is a valid non-human actor (§7) | `okfw validate`, writers |

None of these applies when okfw **reads**. A concept that breaks okfw's rules
still appears in search, in the graph and in `index.md`. Use
`okfw validate --okf` to judge a bundle on the spec's own terms.

## Six required concept fields

**The spec:** a concept needs only a non-empty `type` (§4.1). `title` and
`tags` are optional. A consumer MUST NOT reject a concept when an optional
field is absent (§11.2).

**okfw:** every concept it writes carries `type`, `title` and `tags`, and
okfw's own `x-okfw-id`, `x-okfw-created` and `x-okfw-timestamp`, and
`okfw validate` rejects one that does not.

**Why:**
- OKF identifies a concept by its path, which changes when the concept moves.
  The UUIDv7 `x-okfw-id` stays the same. So a reference by id continues to
  work when you move concepts. Examples are an `x-okfw-parent:`, a daily-note
  link, or an agent's memory.
- `title` and `tags` are what the `index.md` listings, search ranking and
  pickers show. A concept without them is hard to find.
- `x-okfw-created` and `x-okfw-timestamp` record okfw's own write history, which works in a
  bundle that is not in git.

**Consequences:**
- `okfw validate` and `okfw validate --all` reject a bundle another tool wrote
  even when it is conformant. Use `okfw validate --okf` for those.
- Other OKF consumers ignore these keys, as the spec requires them to.
- okfw's keys are in the `x-` namespace that the spec reserves for producers
  (§4.1). So no future OKF version can redefine one of them.
  Before this release, okfw wrote them bare. But OKF v0.1 defines a bare
  `timestamp` as the time that a producer generated the concept (Appendix B).
  So other tools could read okfw's last-write time incorrectly. `okfw migrate`
  changes the names.

## A closed schema

**The spec:** consumers MUST NOT reject documents with keys they do not know,
at any level (§4.1, §11.2).

**okfw:** the schema rejects each key that it does not declare. There are two
exceptions. The schema accepts extension keys that start with `x-`, at each
level. It also accepts the older `x:` block.

**Why:** under the spec's rule, a misspelt key is not an error. It is only an
unknown key, and consumers ignore it. With `stale_afer:`, the concept never
becomes stale. With `descripton:`, it never gets a summary. In both cases,
nothing tells the user. A closed schema turns those typos into errors when
okfw writes. The spec promises never to use the `x-` namespace. So custom
metadata has a place that a closed schema can safely accept.

**Consequences:**
- Name custom keys `x-<producer>-<name>`, for example `x-acme-owner`.
  `x-okfw-*` is okfw's own; every other `x-` key belongs to you.
- A third-party bundle that uses non-`x-` custom keys fails `okfw validate`,
  but passes `okfw validate --okf`.
- okfw never removes a key that it does not know. It edits frontmatter in
  place, so unknown keys, comments and quoting stay after each okfw write, as
  §4.1 requires.

## Bundle title and description required

**The spec:** a bundle-root `index.md` MAY carry `title`, `description` and
`license` (§8.1).

**okfw:** `okfw validate --all` requires a `title` and a `description`.
`okfw init` sets the title from the project directory. Set both with
`okfw init --title … --description …` or `okfw set-bundle-meta`.

**Why:** the description is the sentence an agent reads to choose between
bundles: what the bundle covers and when to use it. okfw also puts the title
and description into the instructions that its MCP server sends to each
agent. When no rule requires a field, the field stays empty.

**Consequences:** `okfw validate --all` fails on a bundle until it has both.
`okfw validate --okf` does not check them.

## Near-miss `human:` actors refused

**The spec:** an actor is `<scheme>:<id>` or `<producer>/<version>`. A
consumer MUST accept any scheme and treat it as non-human. Only `human:`
gives the human-reviewed trust tier (§7).

**okfw:** `okfw verify`, `okfw set-generated` and `okfw validate` refuse a
scheme that is a near miss of `human`, such as `Human:`, `humna:` or
`humans:`. They also refuse an uppercase scheme.

**Why:** under the spec, `humna:alice` is a valid actor with an unknown
scheme. A review with that typo reads back as machine-confirmed, and nothing
tells why.

**Consequences:** reads accept these actors, as the spec requires, and treat
them as non-human. Only the writers and strict `okfw validate` refuse them.

## What okfw implements that the spec only permits

These are not divergences, but they are important choices:

- **okfw writes path fields with a leading `/`.** Both forms are valid (§6).
  The bundle-absolute form stays correct when the concept that holds it moves.
- **okfw writes timestamps as date-times.** Dates are valid and okfw accepts
  them (§5). okfw writes `generated.at` and `verified[].at` as date-times
  because consumers compare those two values, as §5 recommends. It writes
  every other timestamp the same way, so the format stays uniform.
- **`generated.at` moves only for a content change:** `okfw set-generated`,
  or a body rewrite through okfw. Tag, lifecycle, source and verify edits, and
  link repair, change only okfw's `x-okfw-timestamp`. That way an unrelated edit
  never invalidates a review (§5.2–5.3).
- **`okfw search --current`** excludes deprecated concepts and concepts with an
  unknown status. For a deprecated hit, it gives the concept that
  `superseded_by` names (§5.4).
- **Attested Computation (§10)** concepts: okfw can hold, check and link them,
  but it does not execute or attest them. The spec still defers the receipt
  and verdict formats and the attester interface (§12.2).
- **`index.md` gives a summary of a large directory.** Above 50 concepts, the
  listing puts the concepts in groups by `type`. It shows some concepts of each
  group, a count, and the `okfw search --type` command that lists the rest. A
  full listing of a large directory costs more to read than the concepts that
  it shows.
