---
x-okfw-id: "01a208a2-40dd-7232-8b21-098e3429279c"
title: "ADR-0003: okfw keeps no search index and no graph database"
type: "ADR"
tags:
  - "adr"
  - "storage"
x-okfw-created: "2026-11-23T07:39:40-06:00"
x-okfw-timestamp: "2026-11-23T07:39:40-06:00"
description: "Search and the link graph are computed from the concept files on each call; okfw removed Bleve and the bbolt graph store."
---

# ADR-0003: okfw keeps no search index and no graph database

**Status:** Accepted
**Date:** 2026-07-27
**Source:** Commit ebb12d4, "delete the derived stores, conform to OKF disclosure".

## Context
okfw kept a Bleve search index and a bbolt link graph beside the concepts. Each store was a second source of truth that could disagree with the files. Most rules about the stores existed only to stop drift. Examples: prune on delete, sync after a bulk write, and reconcile rather than replay.

## Decision
- okfw removes the Bleve index and the bbolt graph, and the `okfw graph-sync` and `okfw index` commands.
- `search.Search` walks the bundle, filters on frontmatter and ranks the hits ([ADR-0004](/decisions/adr-0004-search-ranks-each-hit-by-a-weighted-field-match--01a208a240e2.md)).
- `bundle.LoadGraph` builds the link graph from the concepts on demand.
- Do not add a database again without a review of this ADR.

## Consequences
- A concept is findable when okfw writes it, and gone when okfw deletes it. No rule about drift remains.
- On a 918-concept bundle, search took about 51 ms and a graph query about 300 ms. The stores had held 13 MB and 4.7 MB of derived state.
- The bundle went from 22 MB to 3.9 MB. The dependencies went from 52 to 27. The binary went from 25.8 MB to 17.8 MB.
- A scan is linear in the bundle size. [ISSUE-007](/issues/issue-007-search-and-the-link-graph-scan-the-whole-bundle--01a208a2419a.md) tracks the review past tens of thousands of concepts.

## Alternatives considered
- **Keep the stores and fix the drift.** Rejected: each fix was a rule about the cache, not about the knowledge.
