---
description: Development guide for contributing to the OKF Wiki (okfw) codebase — building, testing, architecture
globs:
  - "cli/**/*.go"
  - "go.mod"
  - "Makefile"
---

# Skill: OKF Wiki Development

## Build & Test

```bash
make build                    # Build ./okfw binary
make test                     # Go tests + CUE schema validation
go test ./cli/... -count=1    # All Go tests
go test ./cli/internal/search/... -v  # Single package, verbose
go build ./cli/cmd/okfw/      # Just compile, no binary output
make lint                     # go vet + staticcheck
make fmt                      # go fmt + cue fmt
```

## Architecture

All CLI commands live in a single file: `cli/cmd/okfw/main.go`. Commands are defined as Cobra commands within `main()`.

Wikis are per-project `.okfw/` directories, created by `okfw init` and
discovered by searching upward from the current working directory (like git
finds `.git`). There is no config file. Fixed artifact names inside `.okfw/`:
`ephemeral/` (exports) — the only generated directory. okfw keeps no derived
state: search and the link graph are computed from the concepts on demand.
`okfw connect` verifies the wiki; `okfw root` prints its path.

Internal packages:
- `cli/internal/config/` — Wiki discovery (`config.Discover`) + CUE schema loading + concept frontmatter parsing/validation
- `cli/internal/graph/` — In-memory graph (Node, Edge, BFS, ASCII tree, Mermaid rendering, link extraction) plus the queries: backlinks, tag, dangling. Built by `bundle.LoadGraph`, which scans the bundle
- `cli/internal/search/` — Concept search: scan, filter on frontmatter, rank by weighted field match. No index
- `cli/internal/templates/` — Embedded markdown templates via `//go:embed`

## Key Types

```go
// cli/internal/config/concept.go
type Concept struct {
    ID, Title, Type, Kind, Description, Resource, Project string
    Tags []string
    Created, Timestamp time.Time
    Parent, Status, Due, Completed, Priority string
}

// cli/internal/graph/graph.go
type Node struct {
    ID, Title, FilePath, Project, Parent string
    Children, Links []string
}

## CUE Schemas

- `cli/internal/config/concept-schema.cue` — #Concept

Validation pattern: embed CUE file with `//go:embed`, compile with `cuecontext.New()`, unify with data, validate with `cue.Concrete(true)`.

## Dependencies

- `go.etcd.io/bbolt` — Persistent graph store (BoltDB fork, pure Go, single-file)
- `github.com/blevesearch/bleve/v2` — Full-text search engine
- `cuelang.org/go` — Schema validation
- `github.com/spf13/cobra` — CLI framework
- `github.com/google/uuid` — UUID generation
- `gopkg.in/yaml.v3` — YAML frontmatter parsing

## Adding a New Command

1. Define the command as a `cobra.Command` inside `main()` in `cli/cmd/okfw/main.go`
2. Add flags after the command definition
3. Register with `rootCmd.AddCommand(myCmd)`
4. Discover the wiki with `config.Discover` inside RunE (searches upward from the cwd for `.okfw/`; error out with `okfw init` advice when none is found)
5. For graph operations, build the graph with `bundle.LoadGraph(cfg.RootPath)` — it scans the concepts; there is no store to open

## Testing Pattern

```go
// Use t.TempDir() for test databases and concept files
func TestMyFeature(t *testing.T) {
    dir := t.TempDir()
    g, _ := bundle.LoadGraph(bundleRoot)
    defer store.Close()
    // ... write test concepts, sync, query, assert
}
```
