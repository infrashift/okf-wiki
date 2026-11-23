---
x-okfw-id: "01a208a2-4192-7e4a-b600-196cfd20dcf9"
title: "ISSUE-006: The macOS binary is not notarized by Apple"
type: "Issue"
x-okfw-kind: "issue"
tags:
  - "issue"
  - "open"
  - "release"
x-okfw-created: "2026-11-23T07:39:41-06:00"
x-okfw-timestamp: "2026-11-23T07:39:41-06:00"
description: "The darwin-arm64 release is signed with the project's cosign key, but Apple does not notarize it, so Gatekeeper quarantines a browser download."
---

# ISSUE-006: The macOS binary is not notarized by Apple

**Status:** open

## Description
Releases cover darwin-arm64 ([ADR-0019](/decisions/adr-0019-releases-cover-exactly-linux-amd64-linux-arm64-and-darwin-arm64--01a208a2413c.md)). The project's cosign key signs the binary. Gatekeeper does not know that signature, so a file from a browser gets the quarantine attribute. The release notes give `xattr -d com.apple.quarantine`, and an install with `curl` and `tar` avoids the attribute.

## Acceptance Criteria
- [ ] An ADR decides whether to sign with an Apple Developer ID and to notarize.
- [ ] If the ADR says yes, `make sign` signs and notarizes the darwin-arm64 binary, and the release docs say so.

## Related
- [ADR-0018](/decisions/adr-0018-releases-are-calver-tags-built-and-signed-offline-on-a-maintainers-machine--01a208a24136.md) and [ADR-0019](/decisions/adr-0019-releases-cover-exactly-linux-amd64-linux-arm64-and-darwin-arm64--01a208a2413c.md).
