# OKF Wiki (`okfw`)

A Go CLI for managing a local-first, file-based knowledge base that conforms to
the [Open Knowledge Format (OKF)](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md).
Concepts are markdown files with YAML frontmatter, stored in a per-project OKF
**bundle**: a `.okfw/` directory at the project root, created by `okfw init`
and discovered by every command walking upward from the cwd (like `.git`).
There is no config file — storage is standardized. (Formerly the Zettelkasten
CLI `zk`.)

The repo also incubates **`graphy`** (`cli/cmd/graphy`), a sibling binary that
builds a code knowledge graph of a repository (graphify-inspired, tree-sitter
AST extraction) and exports it into an okfw bundle. okfw stays a pure OKF-spec
implementation; graphy owns the enhanced graph capabilities.

## Project Layout

```
cli/cmd/okfw/main.go         — okfw Cobra CLI entry point (all commands)
cli/cmd/graphy/              — graphy CLI (build/query/path/explain/report/export/install/mcp)
cli/internal/config/         — .okfw discovery, CUE schema, concept parsing
cli/internal/bundle/         — Shared bundle write-path (concept creation, frontmatter,
                               index/graph sync) used by BOTH binaries
cli/internal/graph/          — In-memory graph + rendering (ASCII tree, Mermaid)
cli/internal/graphdb/        — okfw persistent graph store (bbolt, 4 predicates)
cli/internal/index/          — Bleve full-text search index
cli/internal/templates/      — Embedded note templates
cli/internal/gitctx/         — Git project-context helper
cli/internal/graphy/         — graphy packages: model, store (own bbolt schema with
                               typed+confidence edges), detect, extract (tree-sitter,
                               CGo), build, cluster (Leiden + Louvain), analyze, report, query,
                               llm (Claude/Gemini/OpenAI REST),
                               export (okfw bundle + GraphML/JGF/Cypher-for-AGE),
                               skills, workspace (.graphy/ discovery)
lua/okfw/                    — Neovim plugin (snacks-based pickers)
plugin/okfw.lua              — Neovim plugin auto-load
```

## Build & Test

```bash
make build           # Builds ./okfw binary (CGO_ENABLED=0 — keep okfw pure Go)
make build-graphy    # Builds ./graphy binary (CGO_ENABLED=1 — tree-sitter needs a C toolchain)
make build-all       # Both
make test            # Runs Go tests + CUE schema validation
make install         # Installs okfw to $GOPATH/bin (install-graphy for graphy)
go test ./cli/... -count=1   # Run all tests
```

graphy conventions: node ids are deterministic strings
(`<lang>:<kind>:<relfile>#<qualified-name>`); edges carry
EXTRACTED/INFERRED/AMBIGUOUS confidence; extraction is incremental
(sha256-diffed) while resolution/clustering rerun globally each build; extractor
golden fixtures (`cli/internal/graphy/extract/testdata/`, regenerate with
`go test -update`) double as tree-sitter grammar-upgrade canaries;
`graphy export okfw` is idempotent via the store's exports bucket, recoverable
from the `<!-- graphy:node … -->` body markers if `.graphy/` is lost, and
stamps exported concepts with `type: code-<entity>`, `resource:` (source
location, commit-pinned permalink with `--repo-url`), and the optional
`graphy:` frontmatter extension block (node_id/language/entity/commit/generator
— declared in concept-schema.cue alongside okfw's other OKF extensions).
No-op re-exports write nothing (content compared with previous
timestamp/commit stamps, so `timestamp` means "last meaningful change"), and
renamed/moved entities keep their concept id: unique same-name-different-file
or 1:1 same-file-same-kind pairings adopt the old concept and rename its file
(rationale is excluded from the same-file pairing — see below). Community
concepts are keyed in the exports bucket by their **label slug**, not their
detection id: ids are ordered by each community's smallest member, so one added
node renumbers most of them, and integer keys silently *repurposed* concepts
(a stable concept id whose subject quietly became a different subsystem). When
the label itself changes, the concept is adopted by **membership overlap**: the
export record stores the community's membership as sorted 64-bit hashes of its
node ids (the `communities` bucket is rebuilt every build, so the previous
partition is gone by export time — and hashing keeps exact set semantics, which
the unambiguity proof below depends on, at a sixth of the size), and a
successor sharing more than half the union inherits it. More-than-half is not a
tuned threshold — successive partitions are disjoint, so at most one candidate
on either side can exceed it, which makes the match unambiguous by
construction; an even split or a merge therefore adopts nothing and starts
fresh. Bundles exported under the old integer keys are re-keyed in place on the
next export, matched by concept title, one record per same-titled community.
The membership refresh has to run for *every* community, including those whose
concept was already up to date (the live-record path skips the concept write, and
a stale membership breaks the next relabelling) — so the digest is canonical and
records already holding it are skipped, keeping a no-op export write-free; the
rest go out in one `PutExports` transaction. Records predating the digest decode
with an empty one (the JSON key changed with the type) and are backfilled on the
next export.

Besides the bundle, the whole graph exports to four **interchange formats**
(`graphy export json|graphml|jgf|cypher`), all fed by one sorted snapshot
(`export.Load`) so they cannot disagree or churn: none stamps a timestamp, so
an unchanged graph re-exports byte for byte and the file can be committed and
diffed. Community membership in these comes from the communities bucket, never
from `Node.Community` — ids start at 0 and so does the field's zero value, so
the field alone cannot tell "in the first community" from "never clustered",
and the attribute is omitted rather than defaulted. GraphML declares its
attribute vocabulary as `<key>` elements (a `<data>` with an undeclared key, or
one declared `int` carrying an empty string, is what makes Gephi and networkx
reject a file). JGF is v2 (jsongraphformat.info): nodes keyed by node id, edges
with source/target/relation, everything graphy-specific under `metadata`, and
HTML escaping off so signatures stay readable.

`graphy export cypher` targets **Apache AGE**, which consumes Cypher only
inside SQL `cypher('<graph>', $tag$ … $tag$) AS (v agtype)` calls — so the
output is a `.sql` psql script, not `.cypher`, and it was verified end to end
against AGE 1.7. Three AGE constraints shape it: a vertex takes exactly one
label (so the label is the entity kind, CamelCased, with the kind repeated as a
property), a pattern's label must be a literal (so edges are grouped by
relation *and* by which site properties they carry, which also keeps every
UNWIND row the same shape — no null padding), and endpoint lookup by id is a
sequential scan until indexed (so per-label `id` btree indexes go in between
the vertex and edge statements, each guarded by an exception handler because
the property-index syntax is version-sensitive and a lost index is not a lost
load). The script is a full snapshot: it creates the graph if missing and
clears it first, so a reload after a rebuild cannot strand vertices for deleted
code (`--no-truncate` appends instead). Two escaping layers have to hold at
once — Cypher literals are backslash-escaped (`\'`, `\\`, `\n`; other control
characters are dropped) while the SQL dollar-quote tag is chosen to appear in
no statement body, since a doc comment containing `$graphy$` would otherwise
close the block and spill the rest of the query into psql.

Community detection is **Leiden** by default (pure Go, in `cluster/`; Louvain
via `--cluster-algorithm louvain`, plus `--cluster-resolution`/`--cluster-seed`
on both `build` and `update`). Leiden guarantees internally-connected
communities, which Louvain does not, and that matters because communities
become subsystem concepts in the bundle. Both are deterministic given a seed
and number communities by their lexicographically-smallest member. The
algorithm/resolution/seed are recorded in the store's meta bucket and rendered
into GRAPH_REPORT.md. Two mutations silently degrade Leiden into Louvain —
skipping refinement, or aggregating the coarse partition instead of the
refined one — so both are pinned by tests (`TestRefineSplitsDisconnectedCommunity`,
`TestLevelAggregatesTheRefinedPartition`); keep them passing.

**Rationale nodes** (`kind: rationale`, `explains` edges) record *why* code is
the way it is, from marked comments (WHY/RATIONALE/DECISION/HACK/FIXME/TODO)
and ADR files. Extraction is a Session-level post-pass in `extract/rationale.go`
so it covers every language, including the pure-Go config extractors; it tracks
string-literal state so a marker inside a string is not a comment, and a marker
must be the leading word of its line. Node identity is
`<ownerQual>@<MARKER>@<ordinal>` — no line numbers, no content hash — so it
survives both rewording and code moving down the file. Exported as
`type: code-rationale` concepts parented to the entity they explain, exempt
from `--min-degree`, and filtered by `--rationale-markers` (default
WHY/RATIONALE/DECISION/ADR — narrower than extraction, so a repo full of TODOs
does not flood the wiki).

Two invariants worth knowing before touching extraction: rationale nodes are
excluded from clustering input (they are degree-2 leaves, and perturbing the
partition rewrites `parent:` on every exported concept), and any change to
extraction output must bump `build.extractorVersion` — the per-file cache keys
on content sha256 alone, so new extraction capability would otherwise apply
only to files that happen to change next.

## Data Model

Concepts are markdown files with YAML frontmatter. A concept's OKF ID is its file
path within the bundle, minus `.md`. The stable internal `id` is a UUIDv7 (RFC
9562); legacy notes may use `YYYYMMDDHHmmss-UUIDv4`; both formats are accepted.

Placement: an explicit `--path` wins, else an explicit `--project` name as a
top-level folder, else the wiki root. Concepts may live in any directory and
are still found by id (lookup scans the whole bundle). `project` is optional
metadata, never auto-derived.
Kinds (`kind`): `note` (default), `todo`, `daily-note`, `issue`. OKF's required
free-form `type` field is also written on every concept.
Links: vanilla markdown links to bundle-relative (`/dir/file.md`) or relative
(`./file.md`) paths.

## Wiki Layout (fixed, inside `.okfw/`)

- Concept markdown at the top level (subdirs via `--path` or an explicit `--project`); `daily/YYYY/MM/` for daily notes
- `.index/` (Bleve search index), `.graph.db` (bbolt graph), `.todos/` (generated todo lists), `ephemeral/` (export scratch) — all gitignored via `.okfw/.gitignore` written by `okfw init`
- `okfw connect` / `okfw root` verify or print the discovered wiki; commands error with "run `okfw init`" advice when none is found

## Key Conventions

- CUE schemas enforce all validation (concept frontmatter, graph edges)
- bbolt (BoltDB) is the persistent graph store — single-file, read-optimized, pure Go
- Bleve is the full-text search engine — separate from the graph store
- The graph store is optional — commands fall back to filesystem scanning when `.graph.db` doesn't exist
- Run `okfw graph-sync` to build/update the persistent graph for O(1) backlink lookups
- Reserved bundle filenames carry no concept frontmatter: `index.md` (directory listing, auto-maintained by okfw on every note change; root copy carries `okf_version`) and `log.md` (update history, generated from git)
- The Neovim plugin uses snacks.nvim for pickers (not telescope)
- `okfw mcp` serves an MCP stdio server (cli/cmd/okfw/mcp.go): five consolidated tools (search/create/read/update/graph), compact JSON, no protocol dependency
