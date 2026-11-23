---
title: "OKF v0.3 and OKF Wiki: where we differ"
description: Where okfw is stricter than, or departs from, the Open Knowledge Format v0.3 specification, why, and what it costs.
---

OKF Wiki implements the [Open Knowledge Format v0.3](https://github.com/open-knowledge-format/open-knowledge-format/blob/main/SPEC.md).
Everything okfw **writes** is a conformant OKF v0.3 bundle, and okfw **reads**
any conformant bundle: search, the link graph and `okfw validate --okf` accept
what the spec accepts. The four example bundles in the OKF repository are part
of okfw's test suite, and they must validate with no violation and resolve
every link.

okfw is deliberately stricter than the spec about the bundles it **authors**.
The spec sets a floor any producer can meet; okfw adds a few guarantees on top,
because a knowledge base that agents write continuously degrades quietly
without them. Each one is listed below with the rule it departs from, the
reason, and the consequence.

## Summary

| okfw | OKF v0.3 | Where it applies |
|---|---|---|
| Six required concept fields | Only `type` is required (§4.1) | `okfw validate`, writers |
| A closed schema: unknown keys are errors, except `x-*` | Consumers MUST NOT reject unknown keys (§4.1, §11.2) | `okfw validate`, writers |
| Bundle `title` and `description` required | Both optional (§8.1) | `okfw validate --all`, `okfw init` |
| Near-miss `human:` actor schemes refused | Any scheme is a valid non-human actor (§7) | `okfw validate`, writers |

None of these applies when okfw **reads**. A concept that breaks okfw's rules
is still found by search, drawn in the graph and listed in `index.md`. Use
`okfw validate --okf` to judge a bundle on the spec's own terms.

## Six required concept fields

**The spec:** a concept needs only a non-empty `type` (§4.1). `title` and
`tags` are optional, and a consumer MUST NOT reject a concept for a missing
optional field (§11.2).

**okfw:** every concept it writes carries `type`, `title` and `tags`, and
okfw's own `x-okfw-id`, `x-okfw-created` and `x-okfw-timestamp`, and
`okfw validate` rejects one that does not.

**Why:**
- OKF identifies a concept by its path, which changes when the concept moves.
  The UUIDv7 `x-okfw-id` stays the same, so a reference by id, such as an `x-okfw-parent:`, a
  daily-note link or an agent's memory, survives a reorganization.
- `title` and `tags` are what the `index.md` listings, search ranking and
  pickers show. A concept without them is hard to find.
- `x-okfw-created` and `x-okfw-timestamp` record okfw's own write history, which works in a
  bundle that is not in git.

**Consequences:**
- `okfw validate` and `okfw validate --all` reject a bundle another tool wrote
  even when it is conformant. Use `okfw validate --okf` for those.
- Other OKF consumers ignore these keys, as the spec requires them to.
- okfw's keys live in the `x-` namespace the spec reserves for producers
  (§4.1), so no future OKF version can give one of them a meaning of its own.
  Before this release okfw wrote them bare, and OKF v0.1 had already defined a
  bare `timestamp` as the generation time (Appendix B), so other tools could
  misread okfw's last-write time. `okfw migrate` renames them.

## A closed schema

**The spec:** consumers MUST NOT reject documents with keys they do not know,
at any level (§4.1, §11.2).

**okfw:** the schema rejects any key it does not declare. The exceptions are
extension keys starting with `x-`, which are accepted at every level, and the
older `x:` block.

**Why:** under the spec's rule, a misspelt key is not an error. It is just an
unknown key, and it is ignored. A `stale_afer:` means the concept never goes
stale. A `descripton:` means it never gets a summary. Nobody is told in either
case. A closed schema turns those typos into errors at write time. The `x-`
namespace is the one the spec promises never to use, so custom metadata has a
place to live that a closed schema can accept safely.

**Consequences:**
- Name custom keys `x-<producer>-<name>`, for example `x-acme-owner`.
  `x-okfw-*` is okfw's own; every other `x-` key belongs to you.
- A third-party bundle that uses non-`x-` custom keys fails `okfw validate`,
  but passes `okfw validate --okf`.
- okfw never drops a key it does not know. Frontmatter is edited in place, so
  unknown keys, comments and quoting survive every okfw write, as §4.1
  requires.

## Bundle title and description required

**The spec:** a bundle-root `index.md` MAY carry `title`, `description` and
`license` (§8.1).

**okfw:** `okfw validate --all` requires a `title` and a `description`.
`okfw init` sets the title from the project directory. Set both with
`okfw init --title … --description …` or `okfw set-bundle-meta`.

**Why:** the description is the sentence an agent reads to choose between
bundles: what the bundle covers and when to use it. okfw also puts the title
and description into the instructions its MCP server sends every agent. A
field nobody is made to fill in stays empty.

**Consequences:** `okfw validate --all` fails on a bundle until both are set.
`okfw validate --okf` does not check them.

## Near-miss `human:` actors refused

**The spec:** an actor is `<scheme>:<id>` or `<producer>/<version>`. A
consumer MUST accept any scheme and treat it as non-human. Only `human:`
gives the human-reviewed trust tier (§7).

**okfw:** `okfw verify`, `okfw set-generated` and `okfw validate` refuse a
scheme that is a near miss of `human`, such as `Human:`, `humna:` or
`humans:`. They also refuse an uppercase scheme.

**Why:** under the spec, `humna:alice` is a valid actor with an unknown
scheme. A review recorded with that typo would read back as machine-confirmed,
and nothing would say why.

**Consequences:** reads accept these actors, as the spec requires, and treat
them as non-human. Only writes and strict validation refuse them.

## What okfw implements that the spec only permits

These are not divergences, but they are choices worth knowing:

- **Path fields are written with a leading `/`.** Both forms are valid (§6).
  The bundle-absolute form stays correct when the concept that holds it moves.
- **Timestamps are written as date-times.** Dates are valid and okfw accepts
  them (§5). okfw writes `generated.at` and `verified[].at` as date-times
  because consumers compare those two values, as §5 recommends. It writes
  every other timestamp the same way, so the format stays uniform.
- **`generated.at` moves only for a content change:** `okfw set-generated`,
  or a body rewrite through okfw. Tag, lifecycle, source and verify edits, and
  link repair, change only okfw's `x-okfw-timestamp`. That way an unrelated edit
  never invalidates a review (§5.2–5.3).
- **`okfw search --current`** excludes deprecated concepts and concepts with an
  unknown status, and follows `superseded_by` to the replacement (§5.4).
- **Attested Computation (§10)** concepts can be stored, validated and linked,
  but okfw does not execute or attest them. The spec still defers the receipt
  and verdict formats and the attester interface (§12.2).
- **Large directories are summarized in `index.md`.** Past 50 concepts, a
  listing is grouped by `type` and shows a sample of each group, with a count
  and the `okfw search --type` command that lists the rest. An exhaustive
  listing of a large directory would cost more to read than the concepts it
  previews.
