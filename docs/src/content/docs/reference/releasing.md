---
title: Releasing
description: How a maintainer cuts, signs and publishes an okfw release.
---

okfw has no release workflow in CI. A maintainer builds, signs and publishes
each release on their own machine with `make`, so the signing key stays on
that machine.

## Requirements

- Go 1.24+
- [syft](https://github.com/anchore/syft#installation), for the SBOMs
- [cosign](https://docs.sigstore.dev/cosign/system_config/installation/) v3, for signing
- [gh](https://cli.github.com/), to publish

## Cut a release

Releases are CalVer tags, `vYYYY.M.N`, with no leading zeros.

1. Change the name of the CHANGELOG's `Unreleased` section to the version.
2. Tag the commit on `main` that passed CI:

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

The maintainer keeps the project's release key at
`~/.config/cosign/infrashift/trusted-build-agents/release.key`. That is not the
`make signing-key` default (`~/.config/cosign/okfw.key`), so give the path to
`make sign`. Also give `COSIGN_PUB=release/cosign.pub`. Then the last step of
`make sign` checks each new signature against the committed public key. With
the wrong key, the command fails at that step. It does not make a release
that no user can verify.

`make sign` runs `make dist`. That target builds the binaries for the three
release platforms: Linux x86_64 and aarch64, and macOS on Apple silicon. Each
binary contains the version. It also makes the man pages, with the date of the
commit. It puts each platform in a tarball (`bin/okfw` and `share/man/man1`)
with an SPDX SBOM. Then it writes `SHA256SUMS` and `SHA256SUMS.sbom`.

Next, `make sign` reads the password of the key one time, or uses
`COSIGN_PASSWORD`. It writes a `<file>.sigstore.json` bundle for each of those
eight files. It writes the public key to `dist/cosign.pub`, from `COSIGN_PUB`
or from the key. Then it verifies each bundle.

Make sure that the name of each file in `dist/` has the tag. A `-dirty` suffix means
the working tree had uncommitted changes, and a `-<n>-g<commit>` suffix means
HEAD is not the tagged commit.

Signing is offline. `make sign` uses a signing config that names no
transparency log or timestamp authority
(`release/cosign-offline-signing-config.json`). So `make sign` sends nothing to
the public Rekor log. cosign v3 writes to that log by default.

## The release key

`dist/cosign.pub` must agree with the published key, `release/cosign.pub` in
the repository. The page
[Verify a release](/okf-wiki/reference/verifying-releases/#the-release-key)
also lists its fingerprint. Users verify against those copies, not only
against the copy in the release:

```bash
diff dist/cosign.pub release/cosign.pub && echo "release key matches"
```

To rotate the key, commit the new `release/cosign.pub`. Add its fingerprint to
the table in [Verify a release](/okf-wiki/reference/verifying-releases/#the-release-key).
Then tell users in the release notes. Older releases
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

The notes must have the CHANGELOG section for the version, and the steps to
install and verify. Then download the release into an empty directory. Do the
steps in [Verify a release](/okf-wiki/reference/verifying-releases/) on it, as
a user does.
