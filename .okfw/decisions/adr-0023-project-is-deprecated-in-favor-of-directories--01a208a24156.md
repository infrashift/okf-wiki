---
x-okfw-id: "01a208a2-4156-7805-877f-3f2d18bf015d"
title: "ADR-0023: project is deprecated in favor of directories"
type: "ADR"
tags:
  - "adr"
  - "schema"
x-okfw-created: "2026-11-23T07:39:41-06:00"
x-okfw-timestamp: "2026-11-23T07:39:41-06:00"
description: "create and search --project and set-project still work but warn, MCP no longer advertises project, and a directory (--path) does the job."
---

# ADR-0023: project is deprecated in favor of directories

**Status:** Accepted
**Date:** 2026-11-23
**Source:** PR #27, commit 91a6336.

## Context
`project` dates from the global `~/okfw` bundle, where okfw took it from the git repository to tell repositories apart ([ADR-0002](/decisions/adr-0002-each-project-has-its-own-wiki-found-by-a-walk-up-from-the-working-directory--01a208a240d7.md)). Each repository now has its own wiki. No local wiki of the maintainer set `project` on any concept.

## Decision
- `okfw create --project` (`-p`), `okfw search --project` and `okfw set-project` still work. They print a warning. `--help`, the man pages and the skill do not show them.
- The MCP server no longer advertises `project` or the `set_project` action. It still accepts them from older clients.
- The Neovim plugin takes `--path` and `opts.path`. Its project features warn once in each session.
- Reads keep the `x-okfw-project` key, and search hits still show it.
- Do not promote `project` again.

## Consequences
- A directory organizes a wiki. Each OKF tool reads directories, and each directory gets an `index.md` listing.
- `okfw move <ref> <dir>/` moves a concept and repairs the links to it.
- The MCP context is 170 bytes smaller on each turn.

## Alternatives considered
- **Keep `--project` as a filter across directories.** Rejected: tags do that, and `x-okfw-project` means nothing outside okfw.
