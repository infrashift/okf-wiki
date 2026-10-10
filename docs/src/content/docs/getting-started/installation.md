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
