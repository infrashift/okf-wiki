---
x-okfw-id: "01a208a2-410b-7625-8595-a5d0f063618c"
title: "ADR-0011: generated.at records the last meaningful change, not the last write"
type: "ADR"
tags:
  - "adr"
  - "trust"
x-okfw-created: "2026-11-23T07:39:40-06:00"
x-okfw-timestamp: "2026-11-23T07:39:40-06:00"
description: "Only a body write and set-generated move generated.at; tag, lifecycle, source and verify edits bump only x-okfw-timestamp."
---

# ADR-0011: generated.at records the last meaningful change, not the last write

**Status:** Accepted
**Date:** 2026-11-23
**Source:** PR #15, commit 4500d7d.

## Context
okfw v0.2 kept `generated.at` equal to `timestamp` on each write. OKF v0.3 counts a review only when it is at or after `generated.at`. With the two coupled, a new tag demoted a human review to unverified.

## Decision
- A body write through okfw moves `generated.at`: the MCP `append` and `set_body` actions (`TouchContentChange`).
- `okfw set-generated` moves it, and its `--at` flag sets the value.
- Tag, lifecycle, source and verify edits, and the link repair of `okfw move`, bump only `x-okfw-timestamp`.
- Do not couple the two fields again.

## Consequences
- A review stays current through metadata edits.
- A rewritten concept reads as unverified until someone verifies it again ([ADR-0012](/decisions/adr-0012-the-trust-tier-counts-only-current-reviews-and-only-the-human-scheme-is-human--01a208a24111.md)).

## Alternatives considered
- **Keep them equal.** Rejected: under v0.3 it demotes reviews.
