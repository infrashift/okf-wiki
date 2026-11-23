---
x-okfw-id: "01a208a2-40ed-7955-be66-26494db84e98"
title: "ADR-0006: okfw is stricter than OKF on purpose"
type: "ADR"
tags:
  - "adr"
  - "schema"
x-okfw-created: "2026-11-23T07:39:40-06:00"
x-okfw-timestamp: "2026-11-23T07:39:40-06:00"
description: "okfw requires six fields and keeps a closed CUE schema, so validate rejects some conformant bundles and validate --okf applies the spec's permissive check."
---

# ADR-0006: okfw is stricter than OKF on purpose

**Status:** Accepted
**Date:** 2026-07-27
**Source:** Commit 7134120. Kept for OKF v0.3 in PR #15. `docs/src/content/docs/reference/okf-divergences.md`.

## Context
OKF requires only a non-empty `type` (section 4.1). It also says that a consumer must not reject an unknown key (sections 4.1 and 11.2).

## Decision
- okfw requires six fields: `type`, `title`, `tags`, and its own `x-okfw-id`, `x-okfw-created` and `x-okfw-timestamp`.
- `#Concept` is a closed CUE definition. It rejects an undeclared key.
- Two escape hatches keep the closed schema usable. Each mapping accepts `x-*` keys, and the open `x:` block takes any value.
- `okfw validate` and `validate --all` apply the strict schema and the bundle metadata check. `okfw validate --okf` applies the permissive section 11.1 check and nothing more.
- These divergences bind writes and `validate`, never reads ([ADR-0007](/decisions/adr-0007-reads-are-lenient-and-writes-are-strict--01a208a240f3.md)).
- Do not change them toward the spec. Other rules of okfw depend on them.

## Consequences
- okfw gives a hard guarantee about the bundles that it writes.
- `okfw validate` rejects some conformant third-party bundles. `validate --okf` checks any bundle directory, with or without `.okfw`.
- OKF v0.3 deprecates none of the six required fields. `title` and `tags` went from recommended to optional.

## Alternatives considered
- **Follow the spec exactly.** Rejected: the trade buys the guarantee about the bundles okfw authors.
