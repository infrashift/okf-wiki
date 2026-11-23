---
x-okfw-id: "01a208a2-41a1-7717-90fb-3a4b649d80b0"
title: "ISSUE-008: The docs site follows ASD-STE100"
type: "Issue"
x-okfw-kind: "issue"
tags:
  - "issue"
  - "docs"
  - "closed"
x-okfw-created: "2026-11-23T07:39:41-06:00"
x-okfw-timestamp: "2026-11-23T07:56:06-06:00"
description: "The pages of the docs site and the wiki pass a lexiscope ste-100 scan; the baseline was 145 errors and 728 warnings in 19 pages."
---

# ISSUE-008: The docs site follows ASD-STE100

**Status:** closed

## Description
A lexiscope `ste-100` scan of `docs/src/content/docs` on 2026-11-23 found 145 errors and 728 warnings in 19 pages. The errors are long sentences (103), banned words such as `would`, `just` and `should` (41) and one long step. Most warnings are possible passive voice, `-ing` forms, noun forms of verbs and the word `your`.

The generated CLI Flags page ([ADR-0021](/decisions/adr-0021-man-pages-and-the-cli-flag-reference-are-generated-from-the-command-tree--01a208a24149.md)) already has 0 findings, because its text comes from the Go help strings.

## Acceptance Criteria
- [x] A `vocabulary.cue` file lists the technical terms of okfw, such as *provenance* and *conformance*.
- [x] `lexiscope scan docs/src/content/docs` reports 0 errors and 0 warnings with that vocabulary.
- [x] A make target runs the scan, in both the Makefile and the ZMakefile ([ADR-0020](/decisions/adr-0020-a-zmakefile-mirrors-the-makefile--01a208a24142.md)).
- [x] The rewrite keeps each fact of the pages.

## Resolution
- PR #29 rewrote the 19 pages. `make docs-style` reports 0 errors, 0 warnings and 0 notes for the docs site and the concepts of this wiki.
- `vocabulary.cue` holds 27 terms. Each term names a concept of okfw, OKF, a tool or a standard.
- `make docs-style` runs the scan in the Makefile and in the ZMakefile. It skips `index.md` and `log.md`, because okfw generates them. CI does not run it, because lexiscope has no release yet.
- The rewrite also corrected stale text. The CLI Commands page gave the v0.2 rule for a bare frontmatter path. The Note Format page named the wrong version for the v0.2 fallback. Three pages still put concepts in folders by project.
- The rewrite found a bug in the Neovim plugin ([ISSUE-011](/issues/issue-011-the-neovim-plugin-follows-a-bare-frontmatter-path-from-the-wiki-root--01a208b179ad.md)).

## Related
- [ADR-0021](/decisions/adr-0021-man-pages-and-the-cli-flag-reference-are-generated-from-the-command-tree--01a208a24149.md).
