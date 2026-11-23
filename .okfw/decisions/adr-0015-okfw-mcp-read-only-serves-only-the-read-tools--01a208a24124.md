---
x-okfw-id: "01a208a2-4124-72e2-8f16-f42291a2a01b"
title: "ADR-0015: okfw mcp --read-only serves only the read tools"
type: "ADR"
tags:
  - "adr"
  - "mcp"
x-okfw-created: "2026-11-23T07:39:40-06:00"
x-okfw-timestamp: "2026-11-23T07:39:40-06:00"
description: "With --read-only, okfw mcp serves search, read and graph with shorter instructions, and refuses create and update."
---

# ADR-0015: okfw mcp --read-only serves only the read tools

**Status:** Accepted
**Date:** 2026-11-23
**Source:** PR #22, commit d5d5239. The instruction fix in PR #25, commit 3a4b87b.

## Context
The okfw-benchmark token runs put the tool definitions and the instructions at 60 to 80 percent of okfw's input tokens. The write tools are nearly half of the definitions. Many agents only look things up.

## Decision
- `okfw mcp --read-only` (`mcpServer.readOnly`) serves `okfw_search`, `okfw_read` and `okfw_graph`, with `mcpReadOnlyInstructions`.
- It refuses `create` and `update` under either name.
- `TestMCPReadOnly` requires a cut of at least 35 percent.
- The read-only instructions name the concept path form, `/dir/file.md` ([ISSUE-010](/issues/issue-010-read-only-claude-sonnet-dropped-correct-answers--01a208a241b0.md)).

## Consequences
- The read-only server costs 42 percent less context on each turn: 3,720 bytes against 6,417.
- With Claude Haiku 5.5, `--read-only` was the cheapest setup that the benchmark measured, at about $0.0007 for each correct answer.

## Alternatives considered
- **A tool filter in the client.** Not recorded as considered.
