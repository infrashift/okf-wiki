---
x-okfw-id: "01a208a2-41a8-7c35-9ae9-7f2e8dece3a4"
title: "ISSUE-009: Deprecate --project and generate the CLI flag reference"
type: "Issue"
x-okfw-kind: "issue"
tags:
  - "issue"
  - "closed"
  - "cli"
x-okfw-created: "2026-11-23T07:39:41-06:00"
x-okfw-timestamp: "2026-11-23T07:39:41-06:00"
description: "Deprecate project in favor of directories, and replace the drifted hand-written flag tables with a page generated from the command tree."
---

# ISSUE-009: Deprecate --project and generate the CLI flag reference

**Status:** closed

## Description
`project` dates from one global wiki ([ADR-0002](/decisions/adr-0002-each-project-has-its-own-wiki-found-by-a-walk-up-from-the-working-directory--01a208a240d7.md)), and no local wiki uses it. The hand-written flag tables of the docs site had drifted from the binary.

## Acceptance Criteria
- [x] `create --project`, `search --project` and `set-project` work and warn. The help does not show them.
- [x] MCP no longer advertises `project` or `set_project`, and still accepts them.
- [x] The Neovim plugin takes `--path` and `opts.path`, and warns once on its project features.
- [x] okfw generates `reference/cli-flags.md`, and a test fails when the page is stale.
- [x] Each flag's help text gives its values and its default.

## Resolution
- PR #27 merged on 2026-11-23 ([ADR-0023](/decisions/adr-0023-project-is-deprecated-in-favor-of-directories--01a208a24156.md), [ADR-0021](/decisions/adr-0021-man-pages-and-the-cli-flag-reference-are-generated-from-the-command-tree--01a208a24149.md)).
- `go test ./...`, `make zmake-check` and `make docs-build` pass. `make e2e` passes all 40 Neovim test groups.
- The Neovim suite also passes inside the dev container image, against its installed okfw and plugin.
- The MCP context is 170 bytes smaller on each turn.

## Related
- [ADR-0021](/decisions/adr-0021-man-pages-and-the-cli-flag-reference-are-generated-from-the-command-tree--01a208a24149.md) and [ADR-0023](/decisions/adr-0023-project-is-deprecated-in-favor-of-directories--01a208a24156.md).
