---
title: Installation
description: How to install OKF Wiki from source.
---

## Prerequisites

- Go 1.24+
- CUE (for schema validation)

## Build from source

```bash
git clone https://github.com/infrashift/okf-wiki.git
cd okf-wiki
make build
make install  # Installs to $GOPATH/bin
```

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
`SHA256SUMS.sbom` over the SBOMs. `<version>` is the git tag on HEAD, or the
short commit when there is none; override it with `make dist VERSION=v1.2.3`.

## Sign a distribution (optional)

`make sign` runs `make dist` and signs every file it wrote with
[cosign](https://docs.sigstore.dev/cosign/system_config/installation/), writing a
`<file>.sigstore.json` bundle beside each one and the public key as
`dist/cosign.pub`, then verifies them. Signing is offline: nothing is uploaded to
the public Rekor transparency log.

```bash
make signing-key         # Optional: makes ~/.config/cosign/okfw.{key,pub} (ECDSA P-256)
make sign                # Asks for the key's password once (or set COSIGN_PASSWORD)
make sign COSIGN_KEY=path/to/cosign.key COSIGN_PUB=path/to/cosign.pub   # Your own key
```

To verify a download against the published key:

```bash
cosign verify-blob --key cosign.pub --insecure-ignore-tlog=true \
  --bundle okfw-<version>-linux-amd64.tar.gz.sigstore.json okfw-<version>-linux-amd64.tar.gz
```

`--insecure-ignore-tlog` is needed because the signature was deliberately never
logged; trust rests on obtaining `cosign.pub` from a source you trust.

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
