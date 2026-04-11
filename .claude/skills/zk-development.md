---
description: Development guide for contributing to the zk CLI codebase — building, testing, architecture
globs:
  - "cli/**/*.go"
  - "go.mod"
  - "Makefile"
---

# Skill: ZK CLI Development

## Build & Test

```bash
make build                    # Build ./zk binary
make test                     # Go tests + CUE schema validation
go test ./cli/... -count=1    # All Go tests
go test ./cli/internal/graphdb/... -v  # Single package, verbose
go build ./cli/cmd/zk/        # Just compile, no binary output
make lint                     # go vet + staticcheck
make fmt                      # go fmt + cue fmt
```

## Architecture

All CLI commands live in a single file: `cli/cmd/zk/main.go`. Commands are defined as Cobra commands within `main()`.

Internal packages:
- `cli/internal/config/` — Config struct + CUE schema loading + zettel frontmatter parsing/validation
- `cli/internal/graph/` — In-memory graph (Node, Edge, BFS, ASCII tree, Mermaid rendering, link extraction)
- `cli/internal/graphdb/` — Persistent graph store (bbolt). Store, Sync, queries (backlinks, cluster, tag, dangling), CUE edge validation
- `cli/internal/index/` — Bleve full-text search (ZettelDoc mapping, search with filters)
- `cli/internal/templates/` — Embedded markdown templates via `//go:embed`

## Key Types

```go
// cli/internal/config/zettel.go
type Zettel struct {
    ID, Title, Type, Project, Category string
    Tags []string
    Created time.Time
    Parent, Status, Due, Completed, Priority string
}

// cli/internal/graph/graph.go
type Node struct {
    ID, Title, FilePath, Project, Category, Parent string
    Children, Links []string
}

// cli/internal/graphdb/graphdb.go
type NodeRecord struct {  // persisted to bbolt
    ID, Title, FilePath, Project, Category, Parent string
    Tags, Links []string
    ModTime int64
}
type EdgeRecord struct {
    From, Predicate, To string  // predicates: links_to, parent_of, child_of, tagged
}
```

## CUE Schemas

- `cli/internal/config/config-schema.cue` — #Config (root_path, index_path, graph_path, etc.)
- `cli/internal/config/zettel-schema.cue` — #Zettel (#UntetheredZettel | #TetheredZettel)
- `cli/internal/graphdb/edge-schema.cue` — #Edge (from UUID, predicate enum, to string)

Validation pattern: embed CUE file with `//go:embed`, compile with `cuecontext.New()`, unify with data, validate with `cue.Concrete(true)`.

## Dependencies

- `go.etcd.io/bbolt` — Persistent graph store (BoltDB fork, pure Go, single-file)
- `github.com/blevesearch/bleve/v2` — Full-text search engine
- `cuelang.org/go` — Schema validation
- `github.com/spf13/cobra` — CLI framework
- `github.com/google/uuid` — UUID generation
- `gopkg.in/yaml.v3` — YAML frontmatter parsing

## Adding a New Command

1. Define the command as a `cobra.Command` inside `main()` in `cli/cmd/zk/main.go`
2. Add flags after the command definition
3. Register with `rootCmd.AddCommand(myCmd)`
4. Load config with `config.Load("")` inside RunE
5. For graph operations, use `graphdb.Open(filepath.Join(cfg.RootPath, cfg.GraphPath))`

## Testing Pattern

```go
// Use t.TempDir() for test databases and zettel files
func TestMyFeature(t *testing.T) {
    dir := t.TempDir()
    store, _ := graphdb.Open(filepath.Join(dir, "test.db"))
    defer store.Close()
    // ... write test zettels, sync, query, assert
}
```
