---
x-okfw-id: "01a208a2-40f3-71c4-899b-c2cc31e722f4"
title: "ADR-0007: Reads are lenient and writes are strict"
type: "ADR"
tags:
  - "adr"
  - "schema"
x-okfw-created: "2026-11-23T07:39:40-06:00"
x-okfw-timestamp: "2026-11-23T07:39:40-06:00"
description: "okfw reads each concept it can parse, field by field, but refuses to write a concept that its schema rejects."
---

# ADR-0007: Reads are lenient and writes are strict

**Status:** Accepted
**Date:** 2026-10-09
**Source:** Commit 1d9f460, "let reads see every concept, keyed by id or concept ID". Write-path validation from commit 7134120.

## Context
Each read path decoded frontmatter strictly. One field with the wrong shape, for example `tags: foo`, dropped the whole concept with no warning. Search, the graph, the listings and the id lookup lost it. The reads also skipped concepts with no okfw id. So a search of a conformant bundle from another producer found nothing.

## Decision
- `ParseFrontmatter` decodes key by key. A bad value costs only its own field. Malformed YAML is still an error.
- okfw keys a concept with no id by its OKF concept ID: its path in the bundle, minus `.md` (`config.ConceptKey`).
- Reads apply the OKF v0.3 value rules: null, `""` and `[]` mean absent, and one item stands for a one-item list.
- `CreateConcept` refuses to write a schema-invalid concept. A mutator refuses an edit that makes a valid concept invalid.
- A mutator still lets an edit through to an already-invalid concept, so the CLI can repair a broken file (`bundle.GuardMutation`).

## Consequences
- `okfw validate` can find and report a malformed concept, because the reads still see it.
- The strict schema ([ADR-0006](/decisions/adr-0006-okfw-is-stricter-than-okf-on-purpose--01a208a240ed.md)) lives in `ParseAndValidate` and on the write path only.

## Alternatives considered
- **Strict reads.** Rejected: the one tool that could report a broken concept could not find it.
