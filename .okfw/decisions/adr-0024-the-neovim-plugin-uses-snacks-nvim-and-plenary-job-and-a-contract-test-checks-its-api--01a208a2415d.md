---
x-okfw-id: "01a208a2-415d-7394-9c49-58cb6f20ac47"
title: "ADR-0024: The Neovim plugin uses snacks.nvim and plenary.job, and a contract test checks its API"
type: "ADR"
tags:
  - "adr"
  - "neovim"
x-okfw-created: "2026-11-23T07:39:41-06:00"
x-okfw-timestamp: "2026-11-23T07:39:41-06:00"
description: "The plugin's pickers use snacks.nvim, it runs okfw through plenary.job, and Test 6 requires each documented function to exist."
---

# ADR-0024: The Neovim plugin uses snacks.nvim and plenary.job, and a contract test checks its API

**Status:** Accepted
**Date:** 2026-10-10
**Source:** Commit 5af6f06 (2026-03-02, snacks.nvim). Commit 9cbdb1e (2026-10-10, the contract test).

## Context
The plugin first used telescope.nvim for its pickers. Its API test checked a hand-kept list of 33 functions with `type(okfw.x) == "function"`. That list could not notice a documented function that the module dropped.

## Decision
- The pickers use snacks.nvim, not telescope.
- The plugin runs okfw through `plenary.job`. plenary is a runtime dependency, not only a test dependency.
- Test 6 of `test/integration_test.lua` reads each `okfw.x(` and `require("okfw").x` in `doc/okfw.txt`, the docs site, the README, `plugin/okfw.lua` and `lua/`, and requires each one to exist.
- The suite stops at Test 4 when plenary is absent. Each test body is a plain `do ... end` block.
- Do not add per-function type checks again.

## Consequences
- A documented function that is gone fails the suite.
- `make e2e` runs the suite in a container. CI runs it on the oldest and the current supported Neovim.

## Alternatives considered
- **A hand-kept list of functions.** Rejected: it missed documented functions.
