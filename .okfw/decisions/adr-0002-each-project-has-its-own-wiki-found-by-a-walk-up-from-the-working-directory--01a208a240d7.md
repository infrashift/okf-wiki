---
x-okfw-id: "01a208a2-40d7-7926-bed5-b637af960b72"
title: "ADR-0002: Each project has its own wiki, found by a walk up from the working directory"
type: "ADR"
tags:
  - "adr"
  - "storage"
x-okfw-created: "2026-11-23T07:39:40-06:00"
x-okfw-timestamp: "2026-11-23T07:39:40-06:00"
description: "okfw init creates .okfw/ at the project root, and each command finds it by a walk up from the working directory, with no config file."
---

# ADR-0002: Each project has its own wiki, found by a walk up from the working directory

**Status:** Accepted
**Date:** 2026-07-02
**Source:** Commit 029253c, "Per-project wikis".

## Context
Before this change, okfw had one global bundle at `~/okfw`. A config file, `~/.config/okfw/config.cue`, named its root, its index path and its folders. okfw took the `project` field of a note from the git repository of the working directory.

## Decision
- `okfw init` creates `.okfw/` at the project root: the git repository root, else the working directory.
- Each command finds the wiki by a walk up from the working directory, as git finds `.git`.
- There is no config file. okfw fixes the layout inside `.okfw/`.
- `okfw connect` verifies the wiki. `okfw root` prints its path.
- okfw does not take `project` from git. It became explicit metadata, and later a deprecated one ([ADR-0023](/decisions/adr-0023-project-is-deprecated-in-favor-of-directories--01a208a24156.md)).

## Consequences
- The concepts travel with the code that they describe, in the same repository.
- A command outside a wiki stops with an error that tells the user to run `okfw init`.
- The commit removed the tethered and untethered categories and the `hello`/`goodbye` git workflow. With per-project wikis, `goodbye` ran `git add -A` in the project repository.

## Alternatives considered
- **Keep the global bundle.** The commit replaced it. The commit message records no other reason than the new layout.
