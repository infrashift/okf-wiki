---
x-okfw-id: "01a208a2-411e-7193-bd82-fc11a349a937"
title: "ADR-0014: The MCP server has five consolidated tools, and each result is capped"
type: "ADR"
tags:
  - "adr"
  - "mcp"
x-okfw-created: "2026-11-23T07:39:40-06:00"
x-okfw-timestamp: "2026-11-23T07:39:40-06:00"
description: "okfw mcp serves okfw_search, okfw_create, okfw_read, okfw_update and okfw_graph, with compact JSON and a cap on every result that says so."
---

# ADR-0014: The MCP server has five consolidated tools, and each result is capped

**Status:** Accepted
**Date:** 2026-07-27
**Source:** Commit 029253c (2026-07-02, the five tools). Commits 8714c84 and 1a8a5f1 (2026-07-27, the names and the caps).

## Context
An agent pays for each tool definition on each turn, and for each byte that a tool returns. One tool per command costs more definitions. An uncapped `read` returned a whole file, and `graph` passed its limit to only one of four actions.

## Decision
- Five tools: `okfw_search`, `okfw_create`, `okfw_read`, `okfw_update` and `okfw_graph`. The bare names still dispatch. `read` and `search` are not distinct names in a client that merges servers.
- The server returns compact JSON and has no protocol dependency.
- The server caps each result and says so: `truncated`, `next_offset` or a count of the rest. `read` returns 200 lines by default.
- `TestEveryMCPToolIsCappedAndSaysSo` runs on the tool list, so a new tool cannot escape it.
- The `update` actions share the CLI mutators. `TestUpdateToolMatchesTheCLIWriters` keeps them at parity.
- `initialize` returns short instructions. `TestMCPInstructionsStayShort` holds them to a byte budget.

## Consequences
- The tool definitions stay small. [ADR-0015](/decisions/adr-0015-okfw-mcp-read-only-serves-only-the-read-tools--01a208a24124.md) cuts them further for agents that only read.
- An agent learns from the result when it saw only part of the answer.

## Alternatives considered
- **One tool per command.** Rejected: more definitions in each turn.
