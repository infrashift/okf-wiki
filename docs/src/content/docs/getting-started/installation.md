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

Each archive holds `bin/okfw` and the man pages under `share/man/man1`. Copy
both into `~/.local` to install the command and its manual. `man` looks in the
`share/man` next to each `bin` directory on the `PATH`, so `man okfw` works
without a `MANPATH`.

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

Put `~/.local/bin` on the `PATH`. The project signs the macOS binary with its
cosign key, but Apple does not notarize it. macOS does not quarantine a
download through `curl`. After a download through a browser, run
`xattr -d com.apple.quarantine ~/.local/bin/okfw`.

The archive of a release before v2026.11.0 has no man pages. `okfw man`
installs them for each okfw binary. It makes them from the commands and writes
them to `~/.local/share/man/man1`.

The project signs each release. Before you install a tarball, you can verify
that the project published it. See
[Verify a release](/okf-wiki/reference/verifying-releases/).

## Build from source

This needs Go 1.24 or later. The binary contains the compiled concept schema,
so it needs nothing else at runtime.

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

For each platform, `make dist` writes `okfw-<version>-<platform>.tar.gz`, with
`bin/okfw`, `share/man/man1/*.1`, LICENSE and README. It also writes an SPDX
SBOM of the binary, `okfw-<version>-<platform>.spdx.json`. `SHA256SUMS` covers
the tarballs, and `SHA256SUMS.sbom` covers the SBOMs. The
[release procedure](/okf-wiki/reference/releasing/) tells how to sign them.

## Versions

`okfw --version` prints the version that `make` puts into the binary. That is
the git tag on HEAD, or `<tag>-<n>-g<commit>` for a later commit. When the
repository has no tags, it is the short commit. To set another version, run
`make build VERSION=...`. A plain `go build` binary reports the pseudo-version
that Go records from git.

Release tags use CalVer, `vYYYY.M.N`: the year, the month, and a counter in
the month. The numbers have no leading zeros (`v2026.1.0`, not `v2026.01.0`),
so each tag is also valid semver.

## Initialize a wiki

Once installed, create a wiki in the project you want notes for:

```bash
cd your-project
okfw init
```

This command creates a `.okfw/` directory at the root of the project. Each
`okfw` command finds it from any directory in the project, with no
configuration. See [Configuration](/okf-wiki/getting-started/configuration/) for
details.
