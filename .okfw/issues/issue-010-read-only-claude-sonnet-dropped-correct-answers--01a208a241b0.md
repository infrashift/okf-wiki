---
x-okfw-id: "01a208a2-41b0-70c0-990b-094f8c788a4c"
title: "ISSUE-010: Read-only Claude Sonnet dropped correct answers"
type: "Issue"
x-okfw-kind: "issue"
tags:
  - "issue"
  - "closed"
  - "mcp"
x-okfw-created: "2026-11-23T07:39:41-06:00"
x-okfw-timestamp: "2026-11-23T07:39:41-06:00"
description: "With okfw mcp --read-only, Claude Sonnet 5.5 found the right concept but submitted no answer in 7 of 108 benchmark tasks."
---

# ISSUE-010: Read-only Claude Sonnet dropped correct answers

**Status:** closed

## Description
In the okfw-benchmark run on v2026.11.2, read-only Sonnet answered 101 of 108 tasks correctly, against 106 on the read-write server. In 7 tasks it found the right concept, often first, and then submitted an empty answer. The read-only instructions had no example of the concept path form. Only the write instructions had one, and read-only mode drops them.

## Acceptance Criteria
- [x] The read-only instructions name the path form, `/dir/file.md`.
- [x] A test checks that the read-only instructions contain it.
- [x] A benchmark rerun shows no empty answer from read-only Sonnet.

## Resolution
- PR #25 merged, released in v2026.11.3 ([ADR-0015](/decisions/adr-0015-okfw-mcp-read-only-serves-only-the-read-tools--01a208a24124.md)).
- In the rerun, read-only Sonnet answered 107 of 108 correctly, with no empty answer in 168 tasks. Haiku 5.5 stayed at 108 of 108.
- The search instructions also say that trust ranks hits but never rules one out. The benchmark could not show that this sentence changes accuracy by itself.

## Related
- [ADR-0014](/decisions/adr-0014-the-mcp-server-has-five-consolidated-tools-and-each-result-is-capped--01a208a2411e.md) and [ADR-0015](/decisions/adr-0015-okfw-mcp-read-only-serves-only-the-read-tools--01a208a24124.md).
