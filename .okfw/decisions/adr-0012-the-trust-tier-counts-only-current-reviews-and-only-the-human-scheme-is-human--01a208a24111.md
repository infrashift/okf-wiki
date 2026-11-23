---
x-okfw-id: "01a208a2-4111-794c-95b8-2d8a40e112ef"
title: "ADR-0012: The trust tier counts only current reviews, and only the human scheme is human"
type: "ADR"
tags:
  - "adr"
  - "trust"
x-okfw-created: "2026-11-23T07:39:40-06:00"
x-okfw-timestamp: "2026-11-23T07:39:40-06:00"
description: "A verified event counts only at or after generated.at, the tier keys strictly on the human: scheme, and okfw refuses a near-miss of human."
---

# ADR-0012: The trust tier counts only current reviews, and only the human scheme is human

**Status:** Accepted
**Date:** 2026-10-10
**Source:** Commit b727923, "hold generated.by and verified[].by to the OKF actor convention". PR #15 (2026-11-23) added the count of current events and the typo guard.

## Context
The trust tier keys on the `human:` prefix (OKF section 5.3). The identity fields took any string, so `Human:ryan`, `humna:ryan` or `ryan` recorded a review that read back as machine-confirmed, with no warning.

## Decision
- An actor is `<scheme>:<id>` with a lowercase scheme, or `<producer>/<version>` (OKF section 7). `actorRe` in Go and `#Actor` in CUE agree, and one test table runs both.
- The tier keys strictly on `human:`. `team:` and other schemes are machine-confirmed.
- `ValidateActor` refuses a scheme within edit distance 2 of `human`, because a misspelt scheme is otherwise valid.
- An event needs both `by` and `at`. The tier ignores an event without them.
- The tier counts only events at or after `generated.at` (`Concept.CurrentVerifications`).
- Reads stay lenient. `sources[].author` earns only a warning.

## Consequences
- A typo cannot silently demote a human review.
- Search hits carry `trust`, so an agent can weigh a hit before it opens it.

## Alternatives considered
- **Free-form actors.** Rejected: a typo demoted reviews silently.
