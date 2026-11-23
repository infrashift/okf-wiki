---
title: Installation
description: How to install okfw, from a signed release or from source.
---

## Download a release

Each [release](https://github.com/infrashift/okf-wiki/releases) carries static
binaries, with no runtime dependencies, for three platforms:

| Platform | `PLATFORM` |
|---|---|
| Linux on x86_64 | `linux-amd64` |
| Linux on aarch64 | `linux-arm64` |
| macOS on Apple silicon | `darwin-arm64` |

Each archive holds `bin/okfw` and the man pages under `share/man/man1`. Copying
both into `~/.local` installs the command and its manual. `man` searches the
`share/man` beside each `bin` directory on your `PATH`, so `man okfw` works
without setting `MANPATH`.

```bash
VERSION=v2026.11.3
PLATFORM=linux-amd64   # or linux-arm64, darwin-arm64
a=okfw-$VERSION-$PLATFORM
curl -fsSLO https://github.com/infrashift/okf-wiki/releases/download/$VERSION/$a.tar.gz
tar -xzf $a.tar.gz
mkdir -p ~/.local && cp -R $a/bin $a/share ~/.local/
okfw --version   # okfw version v2026.11.3
man okfw
```

Put `~/.local/bin` on your `PATH`. The macOS binary is signed with the
project's cosign key, but it is not notarized by Apple. A download through
`curl` is not quarantined. After a download through a browser, run
`xattr -d com.apple.quarantine ~/.local/bin/okfw`.

A binary from a release before v2026.11.0 has no man pages in its archive, and
`okfw man` installs them for any okfw binary: it writes them to
`~/.local/share/man/man1`, from the commands themselves.

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
make install  # bin/okfw and the man pages, under PREFIX (default: $GOPATH)
make install PREFIX=~/.local   # ~/.local/bin/okfw and ~/.local/share/man/man1
```

[zmake](https://github.com/infrashift/zmake) works too: the repository's
`ZMakefile` holds the same tasks in zmake's syntax, so `zmake build` and
`zmake install` do what `make build` and `make install` do.

`make test` additionally needs the [CUE](https://cuelang.org/docs/introduction/installation/)
CLI, for its schema syntax check.

## Build for a release platform

okfw is pure Go, so it cross-compiles from any host. `make build` targets the
machine you run it on. To build for one of the release platforms from any
machine:

```bash
make build-linux-amd64    # Writes bin/okfw-linux-amd64 (Linux x86_64)
make build-linux-arm64    # Writes bin/okfw-linux-arm64 (Linux aarch64)
make build-darwin-arm64   # Writes bin/okfw-darwin-arm64 (macOS, Apple silicon)
make man                  # Writes the man pages to bin/man/man1
make dist                 # All three, packaged in dist/ (needs syft)
```

`make dist` writes, for each platform, `okfw-<version>-<platform>.tar.gz`
(`bin/okfw`, `share/man/man1/*.1`, LICENSE and README) and an SPDX SBOM of the
binary, `okfw-<version>-<platform>.spdx.json`, plus `SHA256SUMS` over the
tarballs and `SHA256SUMS.sbom` over the SBOMs. Signing them is covered in
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
