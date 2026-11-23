---
x-okfw-id: "01a208a2-418b-7c73-8655-9ac011daf490"
title: "ISSUE-005: okfw site has no watch mode"
type: "Issue"
x-okfw-kind: "issue"
tags:
  - "issue"
  - "open"
  - "site"
x-okfw-created: "2026-11-23T07:39:41-06:00"
x-okfw-timestamp: "2026-11-23T07:39:41-06:00"
description: "okfw site up serves the last build, so a change to a concept shows only after the next build."
---

# ISSUE-005: okfw site has no watch mode

**Status:** open

## Description
`okfw site up` builds the site and serves it with `astro preview` ([ADR-0017](/decisions/adr-0017-okfw-site-needs-bun-and-only-okfw-site-runs-it--01a208a24130.md)). A later `okfw site build` replaces the served files without a restart. No command rebuilds the site when a concept changes.

## Acceptance Criteria
- [ ] `okfw site up --watch` rebuilds the site when a concept file in the bundle changes.
- [ ] The rebuild touches no file inside the bundle.
- [ ] `okfw site down` stops the watch and frees the port.

## Related
- [ADR-0017](/decisions/adr-0017-okfw-site-needs-bun-and-only-okfw-site-runs-it--01a208a24130.md) and [ISSUE-004](/issues/issue-004-okfw-site-has-no-graph-view--01a208a24184.md).
