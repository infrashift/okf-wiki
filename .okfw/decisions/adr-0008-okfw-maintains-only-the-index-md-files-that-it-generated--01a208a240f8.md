---
x-okfw-id: "01a208a2-40f8-7dcd-99cd-23cad0c3d3cd"
title: "ADR-0008: okfw maintains only the index.md files that it generated"
type: "ADR"
tags:
  - "adr"
  - "listings"
x-okfw-created: "2026-11-23T07:39:40-06:00"
x-okfw-timestamp: "2026-11-23T07:39:40-06:00"
description: "okfw marks its generated index.md listings and maintains only those, and a listing of more than 50 concepts groups them by type."
---

# ADR-0008: okfw maintains only the index.md files that it generated

**Status:** Accepted
**Date:** 2026-10-09
**Source:** Commit fb3403c, "maintain only the index.md files okfw generated". Commit ebb12d4 (2026-07-27) added the groups by type.

## Context
okfw treated each `index.md` as its own. One `okfw create` replaced a hand-written root listing. The prune step deleted an `index.md` in a directory with no okfw concept, and then the directory. OKF lets an index file exist without a tool.

A generated listing of 900 concepts in one directory was 112 KB, about 28,000 tokens. That preview cost 23 times as much as the concepts it previewed.

## Decision
- A generated listing carries the `IndexMarker` comment. Each write creates each absent listing and rewrites only the marked ones.
- okfw prunes only a marked listing. An unmarked listing belongs to someone else.
- `okfw index-md` and `okfw migrate` adopt an unmarked listing, but only in a directory that holds a concept okfw wrote.
- A listing is not rewritten when it is already current.
- Past 50 concepts (`indexEnumerateLimit`), a listing groups by `type`. Each group names up to 10 concepts, counts the rest and gives the `okfw search --type` command that returns them.
- The root `index.md` keeps its bundle metadata on each rewrite (OKF section 8.1).

## Consequences
- Hand-written listings of foreign concepts survive okfw writes.
- The 900-concept listing went from 112 KB to 8.7 KB. Small directories still list each concept.
- Do not edit a marked `index.md` by hand. The next write replaces it.

## Alternatives considered
- **Treat each `index.md` as okfw's.** Rejected: it deleted hand-written listings.
