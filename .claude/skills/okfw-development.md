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
go test ./cli/internal/graphdb/... -v  # Single package, verbose
go build ./cli/cmd/okfw/      # Just compile, no binary output
make lint                     # go vet + staticcheck
make fmt                      # go fmt + cue fmt
```

## Architecture

All CLI commands live in a single file: `cli/cmd/okfw/main.go`. Commands are defined as Cobra commands within `main()`.

Wikis are per-project `.okfw/` directories, created by `okfw init` and
discovered by searching upward from the current working directory (like git
finds `.git`). There is no config file. Fixed artifact names inside `.okfw/`:
`.index/` (Bleve), `.graph.db` (bbolt),
`ephemeral/` (exports). `okfw connect` verifies the wiki; `okfw root` prints
its path.

Internal packages:
- `cli/internal/config/` — Wiki discovery (`config.Discover`) + CUE schema loading + concept frontmatter parsing/validation
- `cli/internal/graph/` — In-memory graph (Node, Edge, BFS, ASCII tree, Mermaid rendering, link extraction)
- `cli/internal/graphdb/` — Persistent graph store (bbolt). Store, Sync, queries (backlinks, cluster, tag, dangling), CUE edge validation
- `cli/internal/index/` — Bleve full-text search (`ConceptDoc` mapping, search with filters)
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

// cli/internal/graphdb/graphdb.go
type NodeRecord struct {  // persisted to bbolt
    ID, Title, FilePath, Project, Parent string
    Tags, Links []string
    ModTime int64
}
type EdgeRecord struct {
    From, Predicate, To string  // predicates: links_to, parent_of, child_of, tagged
}
```

## CUE Schemas

- `cli/internal/config/concept-schema.cue` — #Concept
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

1. Define the command as a `cobra.Command` inside `main()` in `cli/cmd/okfw/main.go`
2. Add flags after the command definition
3. Register with `rootCmd.AddCommand(myCmd)`
4. Discover the wiki with `config.Discover` inside RunE (searches upward from the cwd for `.okfw/`; error out with `okfw init` advice when none is found)
5. For graph operations, open the store at the wiki's `.graph.db` (e.g. `graphdb.Open(filepath.Join(wikiRoot, ".graph.db"))`)

## Testing Pattern

```go
// Use t.TempDir() for test databases and concept files
func TestMyFeature(t *testing.T) {
    dir := t.TempDir()
    store, _ := graphdb.Open(filepath.Join(dir, "test.db"))
    defer store.Close()
    // ... write test concepts, sync, query, assert
}
```
