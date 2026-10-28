---
title: Installation
description: How to install okfw, from a signed release or from source.
---

## Download a release

Each [release](https://github.com/infrashift/okf-wiki/releases) carries static
Linux binaries for x86_64 (`amd64`) and aarch64 (`arm64`), with no runtime
dependencies:

```bash
VERSION=v2026.10.2
ARCH=amd64   # or arm64
curl -fsSLO https://github.com/infrashift/okf-wiki/releases/download/$VERSION/okfw-$VERSION-linux-$ARCH.tar.gz
tar -xzf okfw-$VERSION-linux-$ARCH.tar.gz
install okfw-$VERSION-linux-$ARCH/okfw ~/.local/bin/
okfw --version   # okfw version v2026.10.2
```

Every release is signed. Before installing, you can check that the tarball is
the one the project published; see
[Verifying Releases](/okf-wiki/reference/verifying-releases/).

## Build from source

Requires Go 1.24+. The concept schema is compiled into the binary, so nothing
else is needed at runtime.

```bash
git clone https://github.com/infrashift/okf-wiki.git
cd okf-wiki
make build    # Writes bin/okfw
make install  # Moves bin/okfw to $GOPATH/bin
```

`make test` additionally needs the [CUE](https://cuelang.org/docs/introduction/installation/)
CLI, for its schema syntax check.

## Build for a specific Linux architecture

okfw is pure Go, so it cross-compiles from any host. `make build` targets the
machine you run it on; to build for Linux x86_64 or aarch64 from any machine:

```bash
make build-linux-amd64   # Writes bin/okfw-linux-amd64 (x86_64)
make build-linux-arm64   # Writes bin/okfw-linux-arm64 (aarch64)
make dist                # Both, packaged for distribution in dist/ (needs syft)
```

`make dist` writes, for each architecture, `okfw-<version>-linux-<arch>.tar.gz`
(the binary, LICENSE and README) and an SPDX SBOM of the binary,
`okfw-<version>-linux-<arch>.spdx.json`, plus `SHA256SUMS` over the tarballs and
`SHA256SUMS.sbom` over the SBOMs. Signing them is covered in
[Releasing](/okf-wiki/reference/releasing/).

## Versions

`okfw --version` prints the version `make` stamped into the binary: the git tag
on HEAD, `<tag>-<n>-g<commit>` for a build past one, or the short commit when
the repository has no tags. Override it with `make build VERSION=...`. A binary
built with plain `go build` reports the pseudo-version Go recorded from git
instead.

Releases are tagged in CalVer, `vYYYY.M.N`: the year, the month, and a counter
within the month, with no leading zeros (`v2026.1.0`, not `v2026.01.0`), which
also keeps them valid semver.

## Initialize a wiki

Once installed, create a wiki in the project you want notes for:

```bash
cd your-project
okfw init
```

This creates a `.okfw/` directory at your project's root. Every `okfw` command
finds it automatically from anywhere inside the project — no configuration
needed. See [Configuration](/okf-wiki/getting-started/configuration/) for
details.
