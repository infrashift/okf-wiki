---
x-okfw-id: "01a208a2-4184-7026-8726-0ce102e65de6"
title: "ISSUE-004: okfw site has no graph view"
type: "Issue"
x-okfw-kind: "issue"
tags:
  - "issue"
  - "open"
  - "site"
x-okfw-created: "2026-11-23T07:39:41-06:00"
x-okfw-timestamp: "2026-11-23T07:39:41-06:00"
description: "The site that okfw site builds shows backlinks on each page but has no view of the whole link graph."
---

# ISSUE-004: okfw site has no graph view

**Status:** open

## Description
The OKF reference viewer draws the link graph with Cytoscape, from a CDN. `okfw site` ([ADR-0017](/decisions/adr-0017-okfw-site-needs-bun-and-only-okfw-site-runs-it--01a208a24130.md)) shows each concept's links and backlinks, but no graph of the bundle.

## Acceptance Criteria
- [ ] The site has a graph page that okfw builds from `bundle.LoadGraph`.
- [ ] The page loads no script from a CDN. The template bundles each library through its `bun.lock`.
- [ ] Each node of the graph opens its concept page.
- [ ] The page stays usable on a 10,000-concept bundle, for example with a cap or a filter.

## Related
- [ADR-0017](/decisions/adr-0017-okfw-site-needs-bun-and-only-okfw-site-runs-it--01a208a24130.md) and [ISSUE-005](/issues/issue-005-okfw-site-has-no-watch-mode--01a208a2418b.md).
