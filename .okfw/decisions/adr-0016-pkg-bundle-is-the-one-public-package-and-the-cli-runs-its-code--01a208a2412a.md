---
x-okfw-id: "01a208a2-412a-7200-ba45-2be1033d4660"
title: "ADR-0016: pkg/bundle is the one public package, and the CLI runs its code"
type: "ADR"
tags:
  - "adr"
  - "api"
x-okfw-created: "2026-11-23T07:39:40-06:00"
x-okfw-timestamp: "2026-11-23T07:39:40-06:00"
description: "Other modules read and search bundles through pkg/bundle, and okfw search calls the same walk and ranking, so the two cannot differ."
---

# ADR-0016: pkg/bundle is the one public package, and the CLI runs its code

**Status:** Accepted
**Date:** 2026-11-14
**Source:** PR #14, commit 4d480ef.

## Context
HALO downloads bundle archives and must parse them. The parser was internal to the CLI, so HALO had to copy it.

## Decision
- The module is `github.com/infrashift/okf-wiki`, so other modules can import it.
- `pkg/bundle` reads any `fs.FS`. `Walk` and `Load` bound each read by `Limits`, and the zero value of `Limits` is an error.
- `Parse` reads one concept as leniently as okfw does. `Validate` applies the strict check. `Search` ranks as `okfw search` does.
- The code moved, not copied. `cli/internal/search` calls `Walk` and `Search` and adds only its filters, its pages and its output.
- `TestSearchMatchesThePublicPackage` pins that both return the same hits.
- `config` and `graph` sit in the module's `internal/`, because `pkg/` cannot import `cli/internal/`.

## Consequences
- Keep the public API small. When a consumer needs more, move the code into the shared packages. Do not copy it.
- Go cannot use the CalVer tags as module versions ([ADR-0018](/decisions/adr-0018-releases-are-calver-tags-built-and-signed-offline-on-a-maintainers-machine--01a208a24136.md)), so consumers pin a commit.

## Alternatives considered
- **Copy the parser into HALO.** Rejected: two copies can differ.
