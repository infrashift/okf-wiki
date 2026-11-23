# issues

<!-- okfw:generated — maintained by okfw; edits here are overwritten -->

## Concepts

* [ISSUE-001: An exact title match does not always rank first](issue-001-an-exact-title-match-does-not-always-rank-first--01a208a2416a.md) - In the okfw-benchmark runs, a concept whose title matches the query exactly ranked below a longer title that contains the same words.
* [ISSUE-002: validate --all is slower than in v2026.10.2](issue-002-validate-all-is-slower-than-in-v2026-10-2--01a208a24172.md) - On a 10,000-concept bundle, okfw validate --all takes about 1.04 s, against 0.90 s in v2026.10.2.
* [ISSUE-003: Attested Computation is not implemented](issue-003-attested-computation-is-not-implemented--01a208a24179.md) - okfw holds the keys of an OKF Attested Computation concept but does not run, verify or attest a computation.
* [ISSUE-004: okfw site has no graph view](issue-004-okfw-site-has-no-graph-view--01a208a24184.md) - The site that okfw site builds shows backlinks on each page but has no view of the whole link graph.
* [ISSUE-005: okfw site has no watch mode](issue-005-okfw-site-has-no-watch-mode--01a208a2418b.md) - okfw site up serves the last build, so a change to a concept shows only after the next build.
* [ISSUE-006: The macOS binary is not notarized by Apple](issue-006-the-macos-binary-is-not-notarized-by-apple--01a208a24192.md) - The darwin-arm64 release is signed with the project's cosign key, but Apple does not notarize it, so Gatekeeper quarantines a browser download.
* [ISSUE-007: Search and the link graph scan the whole bundle](issue-007-search-and-the-link-graph-scan-the-whole-bundle--01a208a2419a.md) - Search and graph queries read each concept file on each call, so their cost grows linearly with the bundle; review the design past tens of thousands of concepts.
* [ISSUE-008: The docs site follows ASD-STE100](issue-008-the-docs-site-follows-asd-ste100--01a208a241a1.md) - The pages of the docs site and the wiki pass a lexiscope ste-100 scan; the baseline was 145 errors and 728 warnings in 19 pages.
* [ISSUE-009: Deprecate --project and generate the CLI flag reference](issue-009-deprecate-project-and-generate-the-cli-flag-reference--01a208a241a8.md) - Deprecate project in favor of directories, and replace the drifted hand-written flag tables with a page generated from the command tree.
* [ISSUE-010: Read-only Claude Sonnet dropped correct answers](issue-010-read-only-claude-sonnet-dropped-correct-answers--01a208a241b0.md) - With okfw mcp --read-only, Claude Sonnet 5.5 found the right concept but submitted no answer in 7 of 108 benchmark tasks.
* [ISSUE-011: The Neovim plugin follows a bare frontmatter path from the wiki root](issue-011-the-neovim-plugin-follows-a-bare-frontmatter-path-from-the-wiki-root--01a208b179ad.md) - follow_link reads a bare path in a frontmatter field from the wiki root, the OKF v0.2 rule, while the CLI follows the v0.3 rule.

