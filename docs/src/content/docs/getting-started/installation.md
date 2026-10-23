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
make dist                # Both, packaged as dist/okfw-<version>-linux-<arch>.tar.gz + SHA256SUMS
```

`<version>` is the git tag on HEAD, or the short commit when there is none.
Override it with `make dist VERSION=v1.2.3`.

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
