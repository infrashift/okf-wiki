---
title: Releasing
description: How a maintainer cuts, signs and publishes an okfw release.
---

okfw has no release workflow in CI. A release is built, signed and published
from a maintainer's machine with `make`, so the signing key never leaves it.

## Requirements

- Go 1.24+
- [syft](https://github.com/anchore/syft#installation), for the SBOMs
- [cosign](https://docs.sigstore.dev/cosign/system_config/installation/) v3, for signing
- [gh](https://cli.github.com/), to publish

## Cut a release

Releases are CalVer tags, `vYYYY.M.N`, with no leading zeros. Tag the commit on
`main` that CI has passed, after renaming the CHANGELOG's `Unreleased` section
to the version:

```bash
git switch main && git pull --ff-only
git tag -a v2026.10.0 -m "okfw v2026.10.0"
git push origin v2026.10.0
```

## Build and sign

```bash
make sign COSIGN_KEY=~/.config/cosign/infrashift/trusted-build-agents/release.key \
  COSIGN_PUB=release/cosign.pub
```

The project's release key is kept at
`~/.config/cosign/infrashift/trusted-build-agents/release.key` on the
maintainer's machine, not at the `make signing-key` default
(`~/.config/cosign/okfw.key`), so `make sign` must be told where it is. Pass
`COSIGN_PUB=release/cosign.pub` as well: the verification at the end of `make
sign` then checks every new signature against the committed public key, so a
wrong key fails there instead of producing a release no one can verify.

`make sign` runs `make dist`, which builds the Linux x86_64 and aarch64
binaries with the version stamped in, packages each as a tarball with an SPDX
SBOM, and writes `SHA256SUMS` and `SHA256SUMS.sbom`. It then reads the key's
password once (or takes `COSIGN_PASSWORD`), writes a `<file>.sigstore.json`
bundle for each of those six files, writes the public key to `dist/cosign.pub`
(from `COSIGN_PUB`, or derived from the key), and verifies every bundle.

Check that every file in `dist/` is named for the tag. A `-dirty` suffix means
the working tree had uncommitted changes, and a `-<n>-g<commit>` suffix means
HEAD is not the tagged commit.

Signing is offline. `make sign` uses a signing config that names no
transparency log or timestamp authority
(`release/cosign-offline-signing-config.json`), so nothing is uploaded to the
public Rekor log, which cosign v3 writes to by default.

## The release key

`dist/cosign.pub` must match the published key, `release/cosign.pub` in the
repository. Its fingerprint is also listed in
[Verifying Releases](/okf-wiki/reference/verifying-releases/#the-release-key).
Users verify against those copies, not just the one inside the release:

```bash
diff dist/cosign.pub release/cosign.pub && echo "release key matches"
```

To rotate the key, commit the new `release/cosign.pub`, add its fingerprint to
the table in Verifying Releases, and say so in the release notes. Older releases
stay verifiable against the key that signed them.

To make a key for a fork or a private build, `make signing-key` writes an
ECDSA P-256 pair to `~/.config/cosign/okfw.{key,pub}`. That location is outside
the repository, so no `git add` or container build context can pick up the
private key.

## Publish

```bash
gh release create v2026.10.0 --verify-tag --title "okfw v2026.10.0" \
  --notes-file notes.md dist/*
```

The notes should carry the CHANGELOG section for the version and the install
and verify steps. Then download the release into an empty directory and
follow [Verifying Releases](/okf-wiki/reference/verifying-releases/) against it,
as a user would.
