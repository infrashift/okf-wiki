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
make build                    # Build bin/okfw binary
make test                     # Go tests + CUE schema validation
go test ./cli/... -count=1    # All Go tests
go test ./cli/internal/search/... -v  # Single package, verbose
go build -o /dev/null ./cli/cmd/okfw  # Just compile, no binary output
make test-race                # Go tests under the race detector
make lint                     # go vet + staticcheck + gofmt check + go mod tidy -diff
make fmt                      # go fmt + cue fmt
make ui-check                 # luacheck lua/ plugin/ test/
make integration-test         # NeoVim plugin suite (finds the binary at bin/okfw)
make e2e                      # The NeoVim suite inside a container (podman)
```

CI (`.github/workflows/test.yaml`) runs the Go tests with `-race`, `go vet`,
gofmt, `go mod tidy -diff`, staticcheck, a `CGO_ENABLED=0` build and `cue vet`;
luacheck; and the NeoVim suite on v0.9.5 (the plugin's floor) and v0.11.4.
`.github/workflows/devcontainer.yaml` builds the devcontainer natively on
x86_64 and arm64 (`make build-image CONTAINER_ENGINE=docker`), runs its
binaries and checks SSH commands, scp and sftp. It is path-filtered to what the image is
built from, so it is not a required check.

## Versions & Releases

`make` stamps the version from `git describe --tags` via
`-ldflags -X okfw/cli/internal/config.version=$(VERSION)`; `okfw --version`
prints it, and an unstamped `go build` falls back to Go's VCS build info.
Releases are CalVer tags `vYYYY.M.N` (no leading zeros) cut locally — there is
no release workflow:

```bash
make build-linux-amd64        # bin/okfw-linux-amd64 (also build-linux-arm64)
make dist                     # dist/ tarballs + SPDX SBOMs (syft) + SHA256SUMS{,.sbom}
make signing-key              # optional: offline cosign key pair, outside the repo
make sign                     # optional: cosign-sign dist/, then verify-signatures
make clean                    # removes bin/ and dist/
```

## Architecture

CLI commands live in `cli/cmd/okfw/`. The command tree is built by `rootCmd()`
in `main.go` (extracted from `main()` so tests can walk the registered command
set); file-moving commands (`delete`, `move`) are in `filecmds.go`
(`fileCommands()`), the OKF-metadata writers and `install` in `okfcmds.go`
(`okfFamilyCommands()`, `newInstallCmd()`), and the MCP server in `mcp.go`.

Wikis are per-project `.okfw/` directories, created by `okfw init` and
discovered by searching upward from the current working directory (like git
finds `.git`). There is no config file. Fixed artifact names inside `.okfw/`:
`ephemeral/` (exports) — the only generated directory. okfw keeps no derived
state: search and the link graph are computed from the concepts on demand.
`okfw connect` verifies the wiki; `okfw root` prints its path.

Internal packages:
- `cli/internal/config/` — Wiki discovery (`config.Discover`) + CUE schema loading + concept frontmatter parsing/validation
- `cli/internal/bundle/` — The write path: concept creation, `yaml.Node` frontmatter editing, move/delete, `index.md` listings, migrate, OKF conformance, reference resolution (`ResolveConceptPath`), and `LoadGraph`
- `cli/internal/graph/` — In-memory graph (Node, Edge, BFS, Mermaid and ASCII rendering, link extraction) plus the queries: backlinks, tag, dangling. Built by `bundle.LoadGraph`, which scans the bundle
- `cli/internal/search/` — Concept search: scan, filter on frontmatter, rank by weighted field match. No index
- `cli/internal/agentskill/` + `cli/internal/okfwskills/` — `okfw install`: the tool-neutral installer and okfw's SKILL.md / CLAUDE.md-block templates
- `cli/internal/templates/` — Embedded markdown templates via `//go:embed`
- `cli/internal/gitctx/` — Git project-root helper (used by `okfw init` and `okfw log`)

## Key Types

```go
// cli/internal/config/concept.go
type Concept struct {
    Type, Title, Description, Resource string
    Tags []string
    Status, StaleAfter string        // OKF lifecycle
    Generated *Generated              // OKF trust
    Verified  VerifiedList
    Runtime, Computation string       // OKF Attested Computation
    Executor, Attester *ResourceRef
    Sources     []Source              // OKF provenance
    UsageWindow *UsageWindow
    ID, Kind, Project, Parent string  // okfw extensions
    Created, Timestamp time.Time
    X map[string]interface{}          // open x: block
}

// cli/internal/graph/graph.go
type Node struct {
    ID, Title, FilePath, Project, Parent string
    Children, Links, Tags []string
}
```

## CUE Schemas

- `cli/internal/config/concept-schema.cue` — #Concept

Validation pattern: embed CUE file with `//go:embed`, compile it once with `cuecontext.New()` (`LoadConceptSchema`, a process-wide `sync.Once`), unify with data under `schemaMu` (CUE v0.7 is not safe for concurrent unification), validate with `cue.Concrete(true)`.

## Dependencies

okfw carries no database; `make build` uses `CGO_ENABLED=0`, so keep it pure Go.

- `cuelang.org/go` — Schema validation
- `github.com/spf13/cobra` — CLI framework
- `github.com/google/uuid` — UUID generation
- `gopkg.in/yaml.v3` — YAML frontmatter parsing

## Adding a New Command

1. Define the command as a `cobra.Command` — inline in `rootCmd()` in `cli/cmd/okfw/main.go`, or as a `newXxxCmd()` constructor in `filecmds.go`/`okfcmds.go`
2. Add flags after the command definition
3. Register it in `rootCmd()` with `rootCmd.AddCommand(myCmd)` (or add it to `fileCommands()`/`okfFamilyCommands()`)
4. Discover the wiki with `config.Discover` inside RunE (searches upward from the cwd for `.okfw/`; error out with `okfw init` advice when none is found)
5. Resolve a concept argument with `bundle.ResolveConceptPath(cfg, ref)` — it accepts the full id, a unique ≥8-hex-digit id prefix, an OKF concept ID, or a file path
6. For graph operations, build the graph with `bundle.LoadGraph(cfg.RootPath)` — it scans the concepts; there is no store to open
7. If the command belongs in the agent skill or the MCP `update` tool, update those too: `TestSkillDocumentsEveryCommand` / `TestSkillPromisesOnlyRealCommands` and `TestUpdateToolMatchesTheCLIWriters` will fail otherwise

## Testing Pattern

```go
// Use t.TempDir() for the bundle and its concept files
func TestMyFeature(t *testing.T) {
    bundleRoot := t.TempDir()
    // ... write test concepts into bundleRoot
    g, err := bundle.LoadGraph(bundleRoot)
    if err != nil {
        t.Fatal(err)
    }
    // ... query g, assert
}
```
