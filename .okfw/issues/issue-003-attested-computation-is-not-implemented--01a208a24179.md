---
x-okfw-id: "01a208a2-4179-7eb7-8990-b7b2e0d2cd5a"
title: "ISSUE-003: Attested Computation is not implemented"
type: "Issue"
x-okfw-kind: "issue"
tags:
  - "issue"
  - "open"
  - "spec"
x-okfw-created: "2026-11-23T07:39:41-06:00"
x-okfw-timestamp: "2026-11-23T07:39:41-06:00"
description: "okfw holds the keys of an OKF Attested Computation concept but does not run, verify or attest a computation."
---

# ISSUE-003: Attested Computation is not implemented

**Status:** open · blocked by the spec

## Description
OKF section 10 defines Attested Computation. The spec still defers its receipt and verdict wire formats, the attester ABI and the sandbox rules (section 12).

okfw declares the keys `runtime`, `parameters`, `computation`, `executor` and `attester` in the schema, so it can hold such a concept. `type: Attested Computation` requires `runtime` (`ValidateBusinessRules`). okfw does nothing else with these concepts.

## Acceptance Criteria
- [ ] The spec defines the receipt and verdict formats and the attester ABI.
- [ ] An ADR decides what okfw implements of section 10.
- [ ] okfw reads and validates a receipt, if that ADR says so.

## Related
- [ADR-0006](/decisions/adr-0006-okfw-is-stricter-than-okf-on-purpose--01a208a240ed.md) and [ADR-0009](/decisions/adr-0009-okfw-targets-okf-v0-3--01a208a240fe.md).
