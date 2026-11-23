---
x-okfw-id: "01a208a2-4104-7ed8-9657-087f326e23c6"
title: "ADR-0010: A path field resolves like a body link, with the v0.2 root reading as a fallback"
type: "ADR"
tags:
  - "adr"
  - "spec"
x-okfw-created: "2026-11-23T07:39:40-06:00"
x-okfw-timestamp: "2026-11-23T07:39:40-06:00"
description: "A bare path in a frontmatter field resolves from its file's directory, as OKF v0.3 section 6 rules; the root reading survives only for v0.2 bundles."
---

# ADR-0010: A path field resolves like a body link, with the v0.2 root reading as a fallback

**Status:** Accepted
**Date:** 2026-11-23
**Source:** PR #15, commit 4500d7d. It reverses commit 06d7d31 (2026-10-10), "read a bare frontmatter path from the bundle root".

## Context
OKF v0.2 section 6.2 did not say how a bare frontmatter path resolves. okfw read it from the bundle root. That rule resolved the examples of the v0.2 spec, and okfw proposed it upstream (#245, #408). OKF v0.3 ruled the other way and lists the change as breaking (section 13.1).

## Decision
- `graph.NormalizeRef` applies one rule to links and path fields. A value with a scheme is external. A leading `/` resolves from the bundle root. Any other value resolves from the directory of its file.
- A bundle can declare `okf_version` 0.2 or no version. There, okfw tries a bare value from the root when it names no file from its directory (section 12.1, `graph.ResolveRef`).
- okfw writers spell an in-bundle path field with a leading `/`, which survives a move of the concept.
- `okfw migrate` prefixes the root-relative paths of a v0.2 bundle with `/` before it stamps `okf_version: "0.3"`.
- Do not bring the root rule back.

## Consequences
- `move` and `export` no longer translate between two spellings.
- Only `okfw migrate` changes a bare path, because the declared version decides how a bare path reads.

## Alternatives considered
- **Keep the root rule.** Rejected: v0.3 rules against it.
