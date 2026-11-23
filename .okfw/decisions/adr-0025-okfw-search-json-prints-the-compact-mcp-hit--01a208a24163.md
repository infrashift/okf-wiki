---
x-okfw-id: "01a208a2-4163-7dd8-8e1c-4bd5d79c4539"
title: "ADR-0025: okfw search --json prints the compact MCP hit"
type: "ADR"
tags:
  - "adr"
  - "search"
x-okfw-created: "2026-11-23T07:39:41-06:00"
x-okfw-timestamp: "2026-11-23T07:39:41-06:00"
description: "okfw search --json prints the same compact hit as the MCP search tool, with a bundle-relative path, at about half the size."
---

# ADR-0025: okfw search --json prints the compact MCP hit

**Status:** Accepted
**Date:** 2026-11-23
**Source:** PR #21, commit 9cda719.

## Context
Agents that follow the okfw skill call `okfw search --json`. That output had indents and carried an absolute `file_path`, a `score`, the `description` beside the snippet and empty fields. It cost about 2,900 tokens for each search, where the MCP tool cost about 1,400.

## Decision
- `okfw search --json` prints a compact array of `{id, title, path, kind, type, project, tags, snippet, trust, status, stale, superseded_by, replaces}`.
- The output leaves out empty fields. `status` shows only when it is not `stable`, and `stale` only when it is true.
- `path` is relative to `.okfw/`, and each command accepts it.

## Consequences
- The output is about half the size.
- Scripts that read `file_path`, `score`, `description` or `stale_after` broke. The release notes said so.
- The Neovim plugin resolves `path` itself, and `entry_file(hit)` does the same for scripts.

## Alternatives considered
- **Two JSON shapes.** Not recorded as considered.
