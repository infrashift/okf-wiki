---
x-okfw-id: "01a208a2-419a-726c-b9b8-d3c883fd3c22"
title: "ISSUE-007: Search and the link graph scan the whole bundle"
type: "Issue"
x-okfw-kind: "issue"
tags:
  - "issue"
  - "open"
  - "performance"
x-okfw-created: "2026-11-23T07:39:41-06:00"
x-okfw-timestamp: "2026-11-23T07:39:41-06:00"
description: "Search and graph queries read each concept file on each call, so their cost grows linearly with the bundle; review the design past tens of thousands of concepts."
---

# ISSUE-007: Search and the link graph scan the whole bundle

**Status:** open

## Description
okfw keeps no index ([ADR-0003](/decisions/adr-0003-okfw-keeps-no-search-index-and-no-graph-database--01a208a240dd.md)). Each search and each graph query walks the bundle. On 10,000 concepts, `okfw search` took about 690 ms in v2026.11.2.

## Acceptance Criteria
- [ ] The okfw-benchmark latency section measures `search`, `list` and `validate` at 50,000 concepts.
- [ ] If a real wiki grows past tens of thousands of concepts, an ADR reviews [ADR-0003](/decisions/adr-0003-okfw-keeps-no-search-index-and-no-graph-database--01a208a240dd.md).
- [ ] Any cache that the review adds has one source of truth: the files.

## Related
- [ADR-0003](/decisions/adr-0003-okfw-keeps-no-search-index-and-no-graph-database--01a208a240dd.md) and [ISSUE-002](/issues/issue-002-validate-all-is-slower-than-in-v2026-10-2--01a208a24172.md).
