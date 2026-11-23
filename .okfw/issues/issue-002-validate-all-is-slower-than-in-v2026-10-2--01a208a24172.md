---
x-okfw-id: "01a208a2-4172-7851-b529-f90fb7f628a2"
title: "ISSUE-002: validate --all is slower than in v2026.10.2"
type: "Issue"
x-okfw-kind: "issue"
tags:
  - "issue"
  - "open"
  - "performance"
x-okfw-created: "2026-11-23T07:39:41-06:00"
x-okfw-timestamp: "2026-11-23T07:39:41-06:00"
description: "On a 10,000-concept bundle, okfw validate --all takes about 1.04 s, against 0.90 s in v2026.10.2."
---

# ISSUE-002: validate --all is slower than in v2026.10.2

**Status:** open

## Description
Idle medians on a 10,000-concept synthetic bundle, measured on 2026-11-23 on the maintainer's laptop:

| Version | `validate --all` |
|---|---|
| v2026.10.2 | 899 ms |
| v2026.11.0 | 1,521 ms |
| v2026.11.1 | 1,014 ms |
| v2026.11.2 | 1,036 ms |

v2026.11.1 removed most of the v2026.11.0 slowdown. Strict validation now parses each file once. The OKF v0.3 checks ([ADR-0009](/decisions/adr-0009-okfw-targets-okf-v0-3--01a208a240fe.md)) add work, so part of the gap can be the cost of new rules.

## Acceptance Criteria
- [ ] A profile of `validate --all` on the 10,000-concept bundle names the cost of each check.
- [ ] A change removes each cost that the v0.3 rules do not need, or the issue records why it stays.
- [ ] The okfw-benchmark latency section shows the result.

## Related
- [ADR-0007](/decisions/adr-0007-reads-are-lenient-and-writes-are-strict--01a208a240f3.md) and [ADR-0009](/decisions/adr-0009-okfw-targets-okf-v0-3--01a208a240fe.md).
