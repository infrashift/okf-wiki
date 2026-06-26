---
title: Installation
description: How to install OKF Wiki from source.
---

## Prerequisites

- Go 1.22+
- CUE (for schema validation)

## Build from source

```bash
git clone https://github.com/infrashift/zettelkasten.git
cd zettelkasten
make build
make install  # Installs to $GOPATH/bin
```
