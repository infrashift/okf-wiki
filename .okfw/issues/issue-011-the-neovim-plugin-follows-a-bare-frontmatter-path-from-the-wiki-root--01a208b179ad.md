---
x-okfw-id: "01a208b1-79ad-792d-816b-ebd3db741c4b"
title: "ISSUE-011: The Neovim plugin follows a bare frontmatter path from the wiki root"
type: "Issue"
x-okfw-kind: "issue"
tags:
  - "issue"
  - "neovim"
  - "closed"
x-okfw-created: "2026-11-23T07:56:18-06:00"
x-okfw-timestamp: "2026-11-23T08:06:55-06:00"
description: "follow_link read a bare path in a frontmatter field from the wiki root, the OKF v0.2 rule, while the CLI follows the v0.3 rule."
---

# ISSUE-011: The Neovim plugin follows a bare frontmatter path from the wiki root

**Status:** closed

## Description
`follow_link` in `lua/okfw/init.lua` reads a bare path in a path-valued frontmatter field, such as `resource: notes/a.md`, from the root of the wiki. That is the OKF v0.2 rule. Since OKF v0.3, the CLI reads a bare path from the directory of the concept. It uses the root only as a fallback, in a bundle that declares `okf_version` "0.2" or no version ([ADR-0010](/decisions/adr-0010-a-path-field-resolves-like-a-body-link-with-the-v0-2-root-reading-as-a-fallback--01a208a24104.md)).

The comment in `follow_link` says that it follows the rule of the CLI (`graph.RefLink`). That comment is now false. In a v0.3 bundle, `gf` on such a field can open a different file than `okfw backlinks` and `okfw move` use, or it can find no file.

The ASD-STE100 pass of the docs site found the bug (PR #29). The plugin page describes the current behavior.

## Acceptance Criteria
- [x] `follow_link` reads a bare frontmatter path from the directory of the concept, as body links do.
- [x] Some bundles declare `okf_version` "0.2" or no version. In such a bundle, a bare path that names no file from the directory falls back to the root, as `graph.ResolveRef` does.
- [x] A test in `test/integration_test.lua` covers both cases.
- [x] The comment in `follow_link` and the plugin page give the v0.3 rule.

## Resolution
- `follow_link` resolves a bare frontmatter path from the directory of the concept. It tries the root only when the bundle declares `okf_version` "0.2" or no version, and only when the directory has no such file. Two helpers read the declared version and apply the rule of `config.LegacyPathFallback`.
- Test 27 of `test/integration_test.lua` covers the v0.3 rule. It also checks that a v0.3 bundle has no fallback, that a v0.2 bundle has one, and the version rule. The old code fails the two v0.3 checks.
- `luacheck` reports 0 warnings. `make e2e` passes all Neovim tests.
- The comment in `follow_link`, `doc/okfw.txt` and the plugin page give the v0.3 rule.

## Related
- [ADR-0010](/decisions/adr-0010-a-path-field-resolves-like-a-body-link-with-the-v0-2-root-reading-as-a-fallback--01a208a24104.md) and [ADR-0024](/decisions/adr-0024-the-neovim-plugin-uses-snacks-nvim-and-plenary-job-and-a-contract-test-checks-its-api--01a208a2415d.md).
