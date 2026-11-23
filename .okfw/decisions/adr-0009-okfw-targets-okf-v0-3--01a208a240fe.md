---
x-okfw-id: "01a208a2-40fe-7c15-895a-2872b6193d2d"
title: "ADR-0009: okfw targets OKF v0.3"
type: "ADR"
tags:
  - "adr"
  - "spec"
x-okfw-created: "2026-11-23T07:39:40-06:00"
x-okfw-timestamp: "2026-11-23T07:39:40-06:00"
description: "okfw reads and writes OKF v0.3 bundles, stamps okf_version 0.3 on the root index.md, and migrates v0.2 bundles with okfw migrate."
---

# ADR-0009: okfw targets OKF v0.3

**Status:** Accepted
**Date:** 2026-11-23
**Source:** PR #15, commit 4500d7d, "OKF v0.3 compliance in the read and write paths".

## Context
OKF v0.3 is at `open-knowledge-format/open-knowledge-format`, pinned at `527e81a`. The `knowledge-catalog/okf` copy stays at v0.2. v0.3 has two declared breaking changes and new consumer MUSTs that okfw broke.

## Decision
- okfw stamps `okf_version: "0.3"` on the root `index.md` only, the one index file that can hold frontmatter.
- Path fields resolve like body links ([ADR-0010](/decisions/adr-0010-a-path-field-resolves-like-a-body-link-with-the-v0-2-root-reading-as-a-fallback--01a208a24104.md)).
- The trust tier counts only current reviews ([ADR-0012](/decisions/adr-0012-the-trust-tier-counts-only-current-reviews-and-only-the-human-scheme-is-human--01a208a24111.md)), and `generated.at` records the last meaningful change ([ADR-0011](/decisions/adr-0011-generated-at-records-the-last-meaningful-change-not-the-last-write--01a208a2410b.md)).
- A full date means UTC midnight. An unparseable `stale_after` is stale, never fresh.
- `superseded_by` is a path field and a graph edge. `search --current` shows the concept that replaces a deprecated hit.
- A `.md` file with no `type` is not a concept. okfw refuses duplicate keys.
- Strict validate requires a bundle `title` and `description` in the root `index.md`.
- `okfw migrate` updates a v0.2 bundle.

## Consequences
- The four upstream example bundles are a regression corpus in `cli/testdata/`.
- The okfw-benchmark repository compares okfw with the reference implementation.
- [ADR-0006](/decisions/adr-0006-okfw-is-stricter-than-okf-on-purpose--01a208a240ed.md) still applies: okfw stays stricter than the spec on writes.

## Alternatives considered
- **Stay on v0.2.** Not recorded as considered.
