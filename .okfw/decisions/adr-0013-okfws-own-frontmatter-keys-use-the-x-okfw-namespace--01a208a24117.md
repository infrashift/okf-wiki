---
x-okfw-id: "01a208a2-4117-7c4b-a05b-5f47f384ed9e"
title: "ADR-0013: okfw's own frontmatter keys use the x-okfw- namespace"
type: "ADR"
tags:
  - "adr"
  - "schema"
x-okfw-created: "2026-11-23T07:39:40-06:00"
x-okfw-timestamp: "2026-11-23T07:39:40-06:00"
description: "okfw writes id, kind, project, parent, created and timestamp as x-okfw-* keys, reads both spellings, and okfw migrate renames the old ones."
---

# ADR-0013: okfw's own frontmatter keys use the x-okfw- namespace

**Status:** Accepted
**Date:** 2026-11-23
**Source:** PR #18, commit 18aa43e.

## Context
OKF v0.3 section 4.1 promises that no OKF version defines an `x-` key, and asks producers for `x-<producer>-<name>`. okfw's bare keys could collide with a future spec. `timestamp` already collided with the OKF v0.1 key of the same name, which gave the time that a producer generated the concept (Appendix B).

## Decision
- The keys are `x-okfw-id`, `x-okfw-kind`, `x-okfw-project`, `x-okfw-parent`, `x-okfw-created` and `x-okfw-timestamp` (`config.Key*`).
- Reads accept both spellings. The new key wins.
- Writes and strict validation use only the new names. A concept with an old key gets a "run `okfw migrate`" error.
- `okfw migrate` and each okfw edit rename the keys in place, but only in a concept that okfw wrote. Another producer's bare `id` stays its own.
- `x-okfw-*` belongs to okfw. Each other `x-` key and `x:` belong to the user.

## Consequences
- A future OKF version cannot collide with okfw's keys.
- The Neovim plugin reads `x-okfw-id` and falls back to `id`.
- Each older wiki needs one `okfw migrate`.

## Alternatives considered
- **Keep the bare keys.** Rejected: they can collide, and one already did.
- **An `okfw:` block or an `okfw-` prefix.** Rejected in [ADR-0005](/decisions/adr-0005-okfw-has-no-task-tracking--01a208a240e8.md): both read as "the OKF fields".
