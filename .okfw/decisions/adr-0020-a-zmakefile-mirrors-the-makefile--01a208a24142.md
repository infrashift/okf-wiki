---
x-okfw-id: "01a208a2-4142-7c93-9567-49bc8ea572fb"
title: "ADR-0020: A ZMakefile mirrors the Makefile"
type: "ADR"
tags:
  - "adr"
  - "build"
x-okfw-created: "2026-11-23T07:39:40-06:00"
x-okfw-timestamp: "2026-11-23T07:39:40-06:00"
description: "The ZMakefile holds the Makefile's tasks in zmake's modern syntax, and a test and make zmake-check keep the two the same."
---

# ADR-0020: A ZMakefile mirrors the Makefile

**Status:** Accepted
**Date:** 2026-11-23
**Source:** PR #17, commit e76da5f.

## Context
zmake reads a `ZMakefile` before a `Makefile`. Both GNU Make and zmake build okf-wiki.

## Decision
- `ZMakefile` holds each Makefile task in zmake's own syntax: `@name:` tasks and no `.PHONY`.
- `TestZMakefileMatchesMakefile` (`internal/buildfiles`) requires the same task set and a complete `ZMAKE_CHECK_TASKS`.
- `make zmake-check` dry-runs each task under both tools and compares the recipes. CI installs a pinned, checksummed zmake to run it.
- Change both files in the same commit.

## Consequences
- Recipes avoid make-only flags. zmake has no `--no-print-directory`, so recipes use `-s`.
- Recipes avoid a backslash-newline inside `$$(...)`, which the two tools print in different ways under `-n`.

## Alternatives considered
- **Only a Makefile.** Not recorded as considered.
