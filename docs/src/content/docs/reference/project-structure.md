---
title: Project Structure
description: Development commands and source code layout for the Zettelkasten CLI.
---

## Development

```bash
# Run tests
make test

# Run linter
make lint

# Format code
make fmt

# Run NeoVim plugin integration tests
make integration-test

# Check Lua syntax
make ui-check
```

## Source Layout

```
zettelkasten-cli/
├── cli/
│   ├── cmd/zk/          # CLI entry point (Cobra commands)
│   ├── internal/
│   │   ├── config/      # CUE schemas and config loading
│   │   ├── graph/       # In-memory graph (BFS, ASCII tree, Mermaid)
│   │   ├── graphdb/     # Persistent graph store (bbolt)
│   │   ├── index/       # Bleve full-text search index
│   │   ├── templates/   # Embedded note templates
│   │   └── zettel/      # Note utilities (git context)
│   └── testdata/        # Test fixtures
├── lua/zk/              # NeoVim plugin (snacks-based pickers)
├── plugin/              # NeoVim autoload
├── after/               # NeoVim ftplugin
├── go.mod, go.sum       # Go module (stays at root)
└── Makefile
```
