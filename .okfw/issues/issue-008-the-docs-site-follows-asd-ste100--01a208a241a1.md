---
x-okfw-id: "01a208a2-41a1-7717-90fb-3a4b649d80b0"
title: "ISSUE-008: The docs site follows ASD-STE100"
type: "Issue"
x-okfw-kind: "issue"
tags:
  - "issue"
  - "open"
  - "docs"
x-okfw-created: "2026-11-23T07:39:41-06:00"
x-okfw-timestamp: "2026-11-23T07:39:41-06:00"
description: "The pages of the docs site do not yet pass a lexiscope ste-100 scan; the baseline was 145 errors and 728 warnings in 19 pages."
---

# ISSUE-008: The docs site follows ASD-STE100

**Status:** open · in progress on the branch `docs/ste100`

## Description
A lexiscope `ste-100` scan of `docs/src/content/docs` on 2026-11-23 found 145 errors and 728 warnings in 19 pages. The errors are long sentences (103), banned words such as `would`, `just` and `should` (41) and one long step. Most warnings are possible passive voice, `-ing` forms, noun forms of verbs and the word `your`.

The generated CLI Flags page ([ADR-0021](/decisions/adr-0021-man-pages-and-the-cli-flag-reference-are-generated-from-the-command-tree--01a208a24149.md)) already has 0 findings, because its text comes from the Go help strings.

## Acceptance Criteria
- [ ] A `vocabulary.cue` file lists the technical terms of okfw, such as *provenance* and *conformance*.
- [ ] `lexiscope scan docs/src/content/docs` reports 0 errors and 0 warnings with that vocabulary.
- [ ] A make target runs the scan, in both the Makefile and the ZMakefile ([ADR-0020](/decisions/adr-0020-a-zmakefile-mirrors-the-makefile--01a208a24142.md)).
- [ ] The rewrite keeps each fact of the pages.

## Related
- [ADR-0021](/decisions/adr-0021-man-pages-and-the-cli-flag-reference-are-generated-from-the-command-tree--01a208a24149.md).
