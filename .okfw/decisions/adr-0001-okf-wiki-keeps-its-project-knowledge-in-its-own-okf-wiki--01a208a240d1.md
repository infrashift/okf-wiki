---
x-okfw-id: "01a208a2-40d1-7457-8754-53543d619387"
title: "ADR-0001: okf-wiki keeps its project knowledge in its own OKF wiki"
type: "ADR"
tags:
  - "adr"
  - "meta"
x-okfw-created: "2026-11-23T07:39:40-06:00"
x-okfw-timestamp: "2026-11-23T07:39:40-06:00"
description: "okf-wiki records its decisions and its tracked work in an OKF wiki in .okfw/, managed with the okfw binary of this repository."
---

# ADR-0001: okf-wiki keeps its project knowledge in its own OKF wiki

**Status:** Accepted
**Date:** 2026-11-23
**Source:** User request. lexiscope, zmake and okfw-benchmark use the same setup.

## Context
okf-wiki makes the tool that manages OKF wikis, but it had no wiki of its own. Its decisions are in `CLAUDE.md`, `CHANGELOG.md` and the commit messages. These sources do not link to each other, and a search cannot filter them by topic.

## Decision
- okf-wiki keeps an OKF wiki in `.okfw/`. The okfw binary of this repository manages it.
- `decisions/` holds the ADRs. `issues/` holds the tracked work.
- `.mcp.json` registers `okfw mcp` for Claude Code. `okfw install --platform claude` writes the skill and the okfw block at the end of `CLAUDE.md`.
- The first ADRs record decisions that the code already follows. Each ADR names the commit or the pull request that made the decision.
- The wiki follows the [Wiki conventions](/wiki-conventions--01a208a240cb.md) guide.

## Consequences
- An agent can find a decision with `okfw search` or `okfw_search` before it changes a design.
- `CLAUDE.md` stays the guide for contributors. An ADR keeps the context and the alternatives of one decision.
- A maintainer can try a change to okfw on a real wiki in this repository.

## Alternatives considered
- **Keep the decisions only in `CLAUDE.md` and the commit messages.** Rejected by the user request.
