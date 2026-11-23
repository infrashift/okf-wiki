---
x-okfw-id: "01a208a2-4136-75b6-956a-671bb35759b5"
title: "ADR-0018: Releases are CalVer tags, built and signed offline on a maintainer's machine"
type: "ADR"
tags:
  - "adr"
  - "release"
x-okfw-created: "2026-11-23T07:39:40-06:00"
x-okfw-timestamp: "2026-11-23T07:39:40-06:00"
description: "A release is a vYYYY.M.N tag; make sign builds, makes SBOMs and signs every file with cosign offline, and no CI workflow cuts releases."
---

# ADR-0018: Releases are CalVer tags, built and signed offline on a maintainer's machine

**Status:** Accepted
**Date:** 2026-10-23
**Source:** PR #4, commit 0595e88 (CalVer). PR #3, commit f145163 (SBOMs and offline signatures). `docs/src/content/docs/reference/releasing.md`.

## Context
okfw needed a version and signed release files. The MCP `serverInfo` reported a fixed "0.2.0" that read like the OKF version. cosign v3 writes to the public Rekor log by default.

## Decision
- Versions are CalVer tags, `vYYYY.M.N`, with no leading zeros, so they are also valid semver.
- `make` stamps the tag into `config.version` with `-ldflags -X`. `config.Version()` is the one reader.
- `make dist` builds the tarballs, an SPDX SBOM for each, `SHA256SUMS` and `SHA256SUMS.sbom`.
- `make sign` signs each file with cosign through `release/cosign-offline-signing-config.json`. That config names no transparency log and no timestamp authority.
- The repository holds the public key as `release/cosign.pub`. `dist/cosign.pub` must match it.
- There is no release workflow in CI. The key stays on the maintainer's machine.

## Consequences
- Go ignores a `v2026.*` tag for a module path without a `/v2026` suffix. The stamp carries the version, and `go version -m` does not show it.
- Nothing goes to the public Rekor log. Users verify against the committed key and its fingerprint.
- A release needs the maintainer, who types the key password.

## Alternatives considered
- **A release workflow in CI.** Rejected: the key must not leave the maintainer's machine.
