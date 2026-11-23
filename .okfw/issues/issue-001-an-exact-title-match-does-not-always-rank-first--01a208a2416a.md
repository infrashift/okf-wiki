---
x-okfw-id: "01a208a2-416a-7d0c-a83a-d6c68619beb2"
title: "ISSUE-001: An exact title match does not always rank first"
type: "Issue"
x-okfw-kind: "issue"
tags:
  - "issue"
  - "open"
  - "search"
x-okfw-created: "2026-11-23T07:39:41-06:00"
x-okfw-timestamp: "2026-11-23T07:39:41-06:00"
description: "In the okfw-benchmark runs, a concept whose title matches the query exactly ranked below a longer title that contains the same words."
---

# ISSUE-001: An exact title match does not always rank first

**Status:** open

## Description
The okfw-benchmark token runs of 2026-11-23 found near-tied rankings ([ADR-0004](/decisions/adr-0004-search-ranks-each-hit-by-a-weighted-field-match--01a208a240e2.md)):

- The query "tag wiki posts" ranks `posts_orphaned_tag_wiki` above `posts_tag_wiki`. The second title is the exact match.
- For query `acme-08`, the best concept ranks third.

Claude Sonnet 5.5 missed these answers in the read-write runs (`results/2026-11-23-tokens-trustfix2/` in okfw-benchmark). A bonus for an exact title match is not tried yet.

## Acceptance Criteria
- [ ] A test in `pkg/bundle` reproduces both rankings.
- [ ] An exact title match, or a title made of exactly the query terms, ranks first in both cases.
- [ ] `TestSearchMatchesThePublicPackage` still passes.
- [ ] A rerun of the okfw-benchmark retrieval section shows no loss in P@5, MRR or nDCG@10.

## Related
- [ADR-0004](/decisions/adr-0004-search-ranks-each-hit-by-a-weighted-field-match--01a208a240e2.md) and [ADR-0016](/decisions/adr-0016-pkg-bundle-is-the-one-public-package-and-the-cli-runs-its-code--01a208a2412a.md).
