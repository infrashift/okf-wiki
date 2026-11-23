---
x-okfw-id: "01a208a2-413c-764a-9ccd-ad7f4a94d461"
title: "ADR-0019: Releases cover exactly linux-amd64, linux-arm64 and darwin-arm64"
type: "ADR"
tags:
  - "adr"
  - "release"
x-okfw-created: "2026-11-23T07:39:40-06:00"
x-okfw-timestamp: "2026-11-23T07:39:40-06:00"
description: "make dist builds three platforms, CI cross-compiles darwin-arm64, and the macOS binary is signed with cosign but not notarized by Apple."
---

# ADR-0019: Releases cover exactly linux-amd64, linux-arm64 and darwin-arm64

**Status:** Accepted
**Date:** 2026-11-23
**Source:** PR #17, commit e76da5f. The linux-arm64 target from PR #2 (2026-10-23).

## Context
okfw is pure Go with `CGO_ENABLED=0` ([ADR-0022](/decisions/adr-0022-graphy-is-decommissioned--01a208a24150.md)), so one machine can cross-compile each platform.

## Decision
- `PLATFORMS` in the Makefile is exactly `linux-amd64`, `linux-arm64` and `darwin-arm64`.
- CI cross-compiles darwin-arm64, so code that builds only on Linux fails there.
- Each tarball holds `bin/okfw` and `share/man/man1`.
- The project's cosign key signs the macOS binary. Apple does not notarize it.

## Consequences
- A download from a browser on macOS gets the quarantine attribute. The release notes give the `xattr` command, and an install with `curl` and `tar` avoids it.
- [ISSUE-006](/issues/issue-006-the-macos-binary-is-not-notarized-by-apple--01a208a24192.md) tracks notarization.

## Alternatives considered
- **More platforms.** Not recorded as considered.
