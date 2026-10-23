---
title: Verifying Releases
description: Check a downloaded okfw release against the project's signing key, its signatures and its checksums.
---

Every okfw release is signed with the project's release key. Verifying a
download answers two questions: are these the files the project published, and
were they changed after signing? Both rest on one thing you check first: that
the public key you verify with is the project's key.

## What a release contains

| File | What it is |
| --- | --- |
| `okfw-<version>-linux-<arch>.tar.gz` | The binary, LICENSE and README, for `amd64` (x86_64) and `arm64` (aarch64) |
| `okfw-<version>-linux-<arch>.spdx.json` | An SPDX SBOM of that binary: every Go module linked into it, and the Go version |
| `SHA256SUMS` | SHA-256 checksums of the tarballs |
| `SHA256SUMS.sbom` | SHA-256 checksums of the SBOMs |
| `<file>.sigstore.json` | A cosign signature bundle for each of the six files above |
| `cosign.pub` | The public half of the release key |

You need `sha256sum` (on macOS, `shasum -a 256`) and
[cosign](https://docs.sigstore.dev/cosign/system_config/installation/) v2.6 or
later (checked with v2.6.5, v3.0.4 and v3.1.4).

## 1. Download

```bash
VERSION=v2026.10.1
mkdir okfw-$VERSION && cd okfw-$VERSION
gh release download $VERSION -R infrashift/okf-wiki
```

Without `gh`, download the files from the
[release page](https://github.com/infrashift/okf-wiki/releases), or with
`curl -fsSLO https://github.com/infrashift/okf-wiki/releases/download/$VERSION/<file>`.

## 2. Check the release key

A signature check against the `cosign.pub` inside the release only proves
that the files agree with each other: anyone who could replace the tarballs
could replace that key too. So first confirm it is the project's key, using
copies published elsewhere.

### The release key

| Releases | Key | SHA-256 of `cosign.pub` |
| --- | --- | --- |
| v2026.10.0 onward | ECDSA P-256 | `f912afb812fabb332536ced8efac95664de9e4519ead41613ddd944161484289` |

```bash
sha256sum cosign.pub
```

The output must match the table. The key is also committed to the repository
as [`release/cosign.pub`](https://github.com/infrashift/okf-wiki/blob/main/release/cosign.pub),
so you can compare against the git history as well:

```bash
curl -fsSL https://raw.githubusercontent.com/infrashift/okf-wiki/main/release/cosign.pub \
  | diff - cosign.pub && echo "release key matches the repository"
```

If either check fails, stop: do not install the binary.

## 3. Check the signatures

Verify the signature on each file you intend to use. For a tarball and the
checksum file that covers it:

```bash
cosign verify-blob --key cosign.pub --insecure-ignore-tlog=true \
  --bundle SHA256SUMS.sigstore.json SHA256SUMS

cosign verify-blob --key cosign.pub --insecure-ignore-tlog=true \
  --bundle okfw-$VERSION-linux-amd64.tar.gz.sigstore.json okfw-$VERSION-linux-amd64.tar.gz
```

Each prints `Verified OK`. Any other output means the file is not what was
signed. To check everything in the release at once:

```bash
for f in *.tar.gz *.spdx.json SHA256SUMS SHA256SUMS.sbom; do
  cosign verify-blob --key cosign.pub --insecure-ignore-tlog=true \
    --bundle "$f.sigstore.json" "$f" >/dev/null 2>&1 \
    && echo "signature OK  $f" || echo "SIGNATURE FAILED  $f"
done
```

`--insecure-ignore-tlog` is required, not a shortcut: okfw releases are signed
offline, so there is no transparency-log entry to check. See
[What the signatures prove](#what-the-signatures-prove).

## 4. Check the checksums

With `SHA256SUMS` itself verified, the checksums confirm the tarballs:

```bash
sha256sum --ignore-missing -c SHA256SUMS
sha256sum --ignore-missing -c SHA256SUMS.sbom
```

On macOS, use `shasum -a 256 --ignore-missing -c SHA256SUMS`.
`--ignore-missing` skips the architectures you did not download. Every line
must end in `OK`.

Checksums alone, without step 3, detect a corrupted download but not a
replaced one: whoever replaced a tarball could replace `SHA256SUMS` too.

## 5. Check the binary

Unpack the tarball and confirm the binary is the version you verified:

```bash
tar -xzf okfw-$VERSION-linux-amd64.tar.gz
okfw-$VERSION-linux-amd64/okfw --version   # okfw version v2026.10.1
```

With Go installed, `go version -m` prints what the binary records about its
own build. `vcs.revision` is the commit it was built from, which should be the
tagged commit, and `vcs.modified=false` means the tree was clean:

```bash
go version -m okfw-$VERSION-linux-amd64/okfw | grep -E 'vcs\.(revision|modified)'
git ls-remote https://github.com/infrashift/okf-wiki.git "refs/tags/$VERSION^{}"
```

## Using the SBOM

The SPDX SBOM lists every module compiled into the binary, with versions, and
the Go standard library version. Once its signature is verified, it can feed a
vulnerability scanner without scanning the binary yourself, for example with
[grype](https://github.com/anchore/grype):

```bash
grype sbom:okfw-$VERSION-linux-amd64.spdx.json
```

## What the signatures prove

A verified signature proves the file was signed by whoever holds the release
key's private half, and has not changed since. It does not prove *when* it was
signed: offline signing records no timestamp and no transparency-log entry.
That keeps signing self-contained, at the cost of the public, append-only
record a logged signature would give. Trust therefore rests entirely on the
release key, which is why step 2 comes first.

If the key is ever rotated, the new key is committed to `release/cosign.pub`,
added to the table above, and announced in the release notes. Older releases
stay verifiable against the key that signed them.
