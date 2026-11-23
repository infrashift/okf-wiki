---
x-okfw-id: "01a208a2-4149-7542-b175-6c8b816d8c84"
title: "ADR-0021: Man pages and the CLI flag reference are generated from the command tree"
type: "ADR"
tags:
  - "adr"
  - "docs"
x-okfw-created: "2026-11-23T07:39:41-06:00"
x-okfw-timestamp: "2026-11-23T07:39:41-06:00"
description: "okfw man and the docs site's CLI Flags page come from rootCmd(), so they cannot describe a flag that the binary does not have."
---

# ADR-0021: Man pages and the CLI flag reference are generated from the command tree

**Status:** Accepted
**Date:** 2026-11-23
**Source:** PR #17, commit e76da5f (man pages). PR #27, commit 91a6336 (flag reference).

## Context
The hand-written flag tables of the docs site drifted from the binary. `okfw export` and `okfw man` had no entry. The `site` and `graph-query cluster` flags were on the wrong command. 30 rows only restated `--help`.

## Decision
- `okfw man` writes okfw(1) and one page for each subcommand with `cobra/doc`. `SOURCE_DATE_EPOCH` dates them, so release builds are reproducible.
- `TestEveryCommandHasAManPage` checks that each command gets a page.
- `genFlagReference` writes `docs/src/content/docs/reference/cli-flags.md`. `make docs-flags` regenerates it.
- `TestCLIFlagReferenceIsCurrent` fails when the page is stale.
- `TestEveryFlagHasHelpText` holds the help text to one style: present, with a capital first letter and no final period.
- To change a flag's description, change its help text in the Go source. Do not edit the page.

## Consequences
- `--help`, the man pages and the docs site agree.
- A deprecated flag shows on the page as deprecated ([ADR-0023](/decisions/adr-0023-project-is-deprecated-in-favor-of-directories--01a208a24156.md)).

## Alternatives considered
- **Hand-written tables.** Rejected: they drifted.
