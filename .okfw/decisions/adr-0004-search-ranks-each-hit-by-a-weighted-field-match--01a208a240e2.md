---
x-okfw-id: "01a208a2-40e2-7817-b396-5cd45119dfa4"
title: "ADR-0004: Search ranks each hit by a weighted field match"
type: "ADR"
tags:
  - "adr"
  - "search"
x-okfw-created: "2026-11-23T07:39:40-06:00"
x-okfw-timestamp: "2026-11-23T07:39:40-06:00"
description: "okfw search returns the best hits first, weighting title over tags over description over body, with a bonus for a match of every term."
---

# ADR-0004: Search ranks each hit by a weighted field match

**Status:** Accepted
**Date:** 2026-07-27
**Source:** Commit ebb12d4. The `CLAUDE.md` note that keeps the ranking.

## Context
The usual agent pattern is a search, then a read, under a small result cap. The caller needs the best few hits, not the first few in directory order. okfw removed Bleve ([ADR-0003](/decisions/adr-0003-okfw-keeps-no-search-index-and-no-graph-database--01a208a240dd.md)), and with it the Bleve ranking.

## Decision
- `scoreConcept` weights the title over the tags, the tags over the description, and the description over the body.
- Repeated body matches count less each time.
- A hit that matches every term of the query gets a coverage bonus. BM25 damps the term frequency to get a similar result. okfw rewards coverage instead.
- `pkg/bundle` owns the ranking code, and `okfw search` calls it ([ADR-0016](/decisions/adr-0016-pkg-bundle-is-the-one-public-package-and-the-cli-runs-its-code--01a208a2412a.md)).
- Snippets are raw body text, with no HTML escapes.

## Consequences
- A capped search can return the answer in its first hits.
- Near-tied rankings remain. [ISSUE-001](/issues/issue-001-an-exact-title-match-does-not-always-rank-first--01a208a2416a.md) records a case where an exact title match ranks second.

## Alternatives considered
- **Directory order with no ranking.** Rejected: under a small cap, the answer can fall past the cap.
