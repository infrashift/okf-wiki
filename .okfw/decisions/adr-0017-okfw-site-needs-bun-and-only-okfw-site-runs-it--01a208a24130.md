---
x-okfw-id: "01a208a2-4130-71a8-8817-0ff09150fd7a"
title: "ADR-0017: okfw site needs bun, and only okfw site runs it"
type: "ADR"
tags:
  - "adr"
  - "site"
x-okfw-created: "2026-11-23T07:39:40-06:00"
x-okfw-timestamp: "2026-11-23T07:39:40-06:00"
description: "okfw site builds an Astro and Starlight site with bun, checks bun before it writes anything, and keeps its build tree outside the bundle."
---

# ADR-0017: okfw site needs bun, and only okfw site runs it

**Status:** Accepted
**Date:** 2026-11-23
**Source:** PR #16, commit abfe5d3.

## Context
People want to browse a wiki and search it in a browser. Astro and Starlight build such a site, and Pagefind gives it search. These tools run on bun. The rest of okfw is one pure Go binary.

## Decision
- Each bun call lives in `cli/internal/site`. `TestOnlySiteRunsBun` keeps it there.
- Each `okfw site` subcommand runs `site.Preflight` before it writes anything. Preflight finds bun (`--bun`, then `OKFW_BUN`, then `PATH`), runs `bun --version` under a timeout and requires `MinBunVersion`.
- An explicit bun path never falls back to `PATH`.
- The embedded template goes into a workdir in the user cache directory, never into the bundle. OKF v0.3 section 3.2 counts each non-dot path under the root.
- `okfw site up` serves the site with `astro preview`, because Starlight disables Pagefind under `astro dev`.
- `okfw site down` frees the port with no bun. It stops the PID in Astro's lockfile only after it attributes that PID to this workdir.
- CI installs bun and sets `OKFW_REQUIRE_BUN`, so the real build test cannot skip there.

## Consequences
- Each other okfw command and each MCP tool works without bun.
- An absent or old bun gives an error that names what okfw found and how to fix it.
- [ISSUE-004](/issues/issue-004-okfw-site-has-no-graph-view--01a208a24184.md) and [ISSUE-005](/issues/issue-005-okfw-site-has-no-watch-mode--01a208a2418b.md) track a graph view and a watch mode.

## Alternatives considered
- **A site generator outside okfw.** Not recorded as considered.
