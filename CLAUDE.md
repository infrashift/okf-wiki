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
cli/internal/agentskill/     — Shared agent-skill installer (tool-neutral; used by
                               BOTH binaries, so okfw need not import graphy)
cli/internal/okfwskills/     — okfw's SKILL.md / CLAUDE.md-block templates
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
make test-race       # Go tests under the race detector
make install         # Installs okfw to $GOPATH/bin (install-graphy for graphy)
make integration-test # NeoVim plugin suite (sources plugin/okfw.lua for the :Okfw* commands)
make graphy-e2e      # graphy build -> export okfw -> okfw validate, against local binaries
make e2e             # Same checks plus the NeoVim suite, inside a container
go test ./cli/... -count=1   # Run all tests
```

Both Containerfiles build **both** binaries. graphy is the only dynamically
linked artifact (CGo, built on the Debian-based golang image, run on
fedora-minimal) so each image smoke-tests `graphy version` at build time rather
than letting a glibc mismatch surface on first use; keep the runtime no older
than the builder. `Containerfiles/config/graphy-e2e.sh` is the gate for the
graphy/okfw seam — it exports a fixture graph into a bundle and has okfw
validate it, which is the only place the two binaries are checked against one
schema at runtime. `graphy install` is per-project and is deliberately not run
during an image build.

graphy conventions: node ids are deterministic strings
(`<lang>:<kind>:<relfile>#<qualified-name>`); edges carry
EXTRACTED/INFERRED/AMBIGUOUS confidence; extraction is incremental
(sha256-diffed) while resolution/clustering rerun globally each build; a file
that fails extraction is recorded in `Stats.Failures` and skipped rather than
aborting the run (a `--full` build purges the cache before extracting, so
aborting midway used to leave the store gutted); extractor golden fixtures
(`cli/internal/graphy/extract/testdata/`, regenerate with `go test -update`)
are regression tests for all ten languages and **tree-sitter grammar-upgrade
canaries for the four that use tree-sitter** — go, python, javascript,
typescript (incl. a separate `.tsx` grammar, hence its own golden); the other
six are pure-Go parsers (markdown/sql via regex, yaml/json via yaml.v3, cue via
cuelang, terraform via hcl) that a grammar bump cannot break;
`detect` never lists dot-directories — the walk and `git ls-files` share one
ignore filter, so graphy cannot re-ingest its own `.okfw/` export as heading
nodes; the build-output ignores (`dist/`, `build/`, `target/`, `out/`) are
**anchored to the repo root** while `node_modules/`/`vendor/` match at any
depth, because those four are ordinary words that name real source packages —
matching `build/` at any depth silently erased this repo's own
`cli/internal/graphy/build` package from the graph, and invisibly, since a file
`detect` never lists is not a file that *failed* to extract;
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
(rationale is excluded from the same-file pairing — see below).

Adoption resolves **after** every live placement, never during. Both community
matchers can otherwise select a record another item already resolved — a
community that splits leaves a successor overlapping the original by more than
half, and two communities can carry byte-identical titles — and adopting it
renames the file that item sits on and deletes its record, so pass 2 then dies
on `ENOENT` after those writes landed. Renames go through `bundle.MoveConcept`
so inbound links are repaired, and the no-op guard compares the filename's slug
directly: the old guard asked `bundle.NoteFilename`, which skips names that
exist on disk — including the file being renamed — so it never fired and
"re-keyed in place" never was. The record's concept id is a cache of what the
file says; the file wins, or children get a `parent:` pointing at an id that
exists nowhere. A changed `--project` fails loudly rather than splitting the
bundle across two directories, where every `./name.md` link would resolve to
nothing.

**Rendering is budgeted, because the bundle is read one concept at a time.**
Structural `contains` edges naming a file or heading are dropped rather than
rendered as dead text — they restate `resource:` and the outline, and were a
fifth of every bullet. A heading endpoint redirects to its document, which is
what makes a doc→code reference navigable from the code side. Edges group under
`### <relation> (N)` carrying the true count, sorted by confidence then degree,
capped by `--max-edges` (default 25) with the remainder naming the query that
returns it; AMBIGUOUS edges are segregated into a trailing roll-up rather than
interleaved, where they read exactly like resolved ones. Doc concepts render an
**outline** of their headings with what each section references. Community
concepts carry a composition line and cap their member table at 40. A
`type: code-graph` **overview concept** is the bundle's front door — stats,
languages, the cluster meta (algorithm/resolution/seed, otherwise unrecoverable
from a bundle), every subsystem and the top entry points — and communities
parent to it, so the export is a tree rather than a thousand siblings.
`TestExportStaysWithinItsTokenBudget` pins the result: every other test here
asserts something is *present*, so they all push toward rendering more, and
nothing else pushes back.

Community
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
algorithm/resolution/seed are recorded in the store's meta bucket, rendered
into GRAPH_REPORT.md, **and read back as the defaults for the next run**: an
unpassed flag inherits what the graph was last built with (`cmd.Flags().Changed`
decides, so an explicit value always wins, including one that names the
default). Binding the flags to `update` is not enough on its own — they default
to graphy's defaults, so a bare `graphy update` re-clustered a louvain graph
with Leiden, and community renumbering rewrites `parent:` on every exported
concept. The MCP `graphy_update` tool takes no flags and so always inherits.
Two mutations silently degrade Leiden into Louvain —
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

`--langs` scopes which files are *extracted*, never which nodes survive: it
once drove the delete diff too, so `graphy build --langs go` purged every
non-Go node and an unknown name silently wiped the whole graph (names are now
validated against the registry). It also feeds `extractFingerprint`, so
widening the set back re-extracts. `--full` purges are counted as
`Stats.CacheCleared`, kept apart from `FilesPurged` so "purged" keeps meaning
"gone from disk".

**Resolution confidence is a claim about evidence, not a score.** `lookupSymbol`
reports whether it matched the ref's full (possibly qualified) name or only its
bare tail, and the two get different ladders. A full match keeps
same-file→EXTRACTED, unique→INFERRED, 2..5→AMBIGUOUS. A **tail-only** match
resolves at most one candidate and otherwise *drops*: the qualifier that said
which symbol was meant has already been discarded, so the extra candidates are
not rival readings, they are unrelated symbols sharing a method name. Fanning
out over them manufactured edges — every `err.Error()` in this repo landed on
all three error types, making them the largest, most-connected entities in the
graph and the most expensive concepts in the bundle. For Go, a call qualified by
a package the file imports is scoped to that package's directory first
(`goPackageDirs`), which turns a blind tail match into an exact one. Measured
here: AMBIGUOUS fell from 14.1% of edges to 3.7%, while INFERRED *rose*, because
the scoping recovers real cross-package calls.

**A receiver qualifier is recovered the same way, and it is what makes
`graphy callers` answer at all.** A call through a struct field —
`s.engine.Evaluate(...)`, the dominant shape for one subsystem reaching another
— reaches the index as the bare tail `Evaluate` and is dropped whenever a second
type declares that name. That drop lands hardest on the entities worth querying:
central verbs like Evaluate and Validate recur across layers *because* they are
central. Measured on a 24k-LOC Go repo before the fix, `Engine.Evaluate` and
`Engine.Validate` each resolved **zero** callers while single-owner names
resolved 32 and 45 — the flagship impact-analysis query returned nothing for
every entry point anyone would ask about. `goReceiverQualified` rewrites
`recv.field.Method` into `FieldType.Method` using the enclosing method's receiver
type and `Node.Fields` (struct field → bare type name, from the extractor), and
`recv.Method` into `RecvType.Method`. It runs *after* package scoping so an
import qualifier always wins, and it requires the head segment to equal
`Node.Recv` — without that check any `x.engine.Evaluate` would be read as the
enclosing type's field regardless of what `x` is. `recv.Method` also tries the
receiver's **embedded** types, in declaration order, because Go promotion means
`b.Get()` can name `S3Backend.Get` with nothing on the outer type saying so.
Deeper chains (`a.b.c.Method`) are deliberately not followed.

**The other half of a receiver qualifier is a local**, and struct knowledge
cannot reach it: `eng := engine.NewEngine(); eng.Evaluate()`. `Node.Locals` maps
a body's variable names to their types from the three declaration forms that
state one without inference — parameters, `var x T`, and `x := &T{}` — plus the
constructor idiom `x := NewT()`, recorded as the sentinel `()NewT` because the
constructor almost always lives in another file and extraction is per-file.
`goResolveLocals` substitutes the callee's `Node.Returns` (its **first** result
type: Go constructors return `(*T, error)`) once every node is known, and
refuses to guess — an ambiguous or unknown callee leaves the variable untyped,
which merely restores the old drop, whereas guessing would manufacture a
confident wrong edge.

**Locals are scope-ranged, because Go lets an inner scope rebind a name to a
different type.** Each `model.Local` carries the line range over which it is the
binding for its name, and `localTypeAt` takes the innermost declaration covering
the call's line (narrowest range wins, since scopes nest; ties go to the later
declaration). A flat name→type map cannot express this, and "last declaration in
source order wins" is actively wrong: on this repo it typed all three
`err.Error()` calls in a table of `t.Run` subtests as the *last* subtest's error
type, which deduped to **one edge pointing at the wrong type** where the scoped
version emits three correct ones. `func_literal` is a scope kind precisely so
`t.Run(func(t *testing.T){…})` closures separate; `if`/`for`/`switch` are scopes
too, since their initializers bind beyond the braces that follow.

Measured across the changes on one 24k-LOC Go repo (edges / Go methods with ≥1
resolved caller / AMBIGUOUS): baseline **4038 / 138 / 26** → receiver fields +
the same-file fix **4035 / 136 / 26** → local typing **4214 / 144 / 26** →
scope-ranged locals **4216 / 144 / 26**. The second step *lowers* the method
count on purpose — it removes 17 false callers while adding 14 true ones.
AMBIGUOUS never moves at any step, and no true edge is ever lost; every gain
lands in INFERRED, which is what a cross-file inference should be.

One storage note: `Node.Locals` is keyed `local_scopes` in JSON, not `locals`.
An earlier shape wrote an object under `locals`, and `Run` decodes **every**
stored record via `EachFile` *before* a `--full` purge can replace them — so
reusing the key would make an existing store fail to decode instead of
rebuilding itself. An unknown key is ignored; a type mismatch is fatal.

**The same-file shortcut requires a full match**, for the same reason. It used to
run first unconditionally, so a tail-only hit whose bare name happened to be
declared in the same file was stamped EXTRACTED — the *highest* confidence — on
the strength of a qualifier that had already been thrown away. `defer
resp.Body.Close()` in a file declaring a `Close` method became "this method calls
Close, EXTRACTED", and `err.Error()` did the same thing the tail rule exists to
prevent.

**Cross-artifact linking beyond code.** Markdown emits `references` refs for
backticked identifiers in prose, not only for links — links only resolve when a
doc names a *path*, which this repo's 38 documents did exactly once. Refs are
owned by the enclosing H1/H2 rather than the document, carry line numbers, and a
`#L42` anchor resolves to the declaration on that line instead of the file node.
Markdown refs also get the `anyLang` fallback that only ADRs had: every markdown
node kind is non-indexable, so a language-keyed lookup could *never* reach a Go
symbol, which is why doc→code was structurally impossible before. An ADR drops
the `references` refs that duplicate its own `explains` refs — the stronger
relation wins rather than both being recorded.

Three relations join things the graph previously could not: **`implements`** is
inferred for Go by comparing an interface's `Members` against a type's method
set (Go satisfaction is structural, so nothing in the source names the
implementor; empty interfaces and embedded/promoted methods deliberately
under-report rather than guess). **`tests`** points from a test to its subject,
derived from naming convention only — promoting a test's call edges would mark
every assertion helper as under test — with the subject taken from the longest
CamelCase prefix naming a real symbol, lower-cased initial included
(`TestRunMigrateIsIdempotent` → `runMigrate`), floored at 5 characters so short
generic words cannot win. `Node.Test` marks every entity in a test file whether
or not an edge was derived: test files inflate `Degree`, which `--min-degree`
reads as significance, and pull the partition toward mirroring the suite.
**`depends_on`** is read from `go.mod`/`package.json`, since every third-party
call is dropped by resolution; a declared module `contains` the subpackages
actually imported, without which `tree-sitter-go` looks unused while
`tree-sitter-go/bindings/go` looks undeclared.

**Deterministic traversal** (`query.Neighbors`/`Edges`/`FileNodes`/`Rationale`,
surfaced as `graphy neighbors|callers|uses|why|nodes|concept` and the matching
MCP tools) is what an agent uses when it already knows the entity;
`graphy query` is the only non-deterministic command, since its seeds come from
a model or a keyword score. `ResolveEntity` accepts a repo-relative file path as
well as a name, qualified name or id — an agent is almost always working *in* a
file. Everything is capped and says so: a result carrying `truncated`/`… N more`
withheld rows, and an unknown relation/kind/direction is a hard error listing the
vocabulary rather than a silently-ignored filter, which would widen the query to
everything and read as "no matches". Test entities are excluded by default.

**That cap rule now covers the whole MCP surface, in both binaries, and
`TestEveryMCPToolIsCappedAndSaysSo` is what keeps it there** — driven from
`mcpTools()` so a tool added later cannot escape. It exists for the same reason
`TestExportStaysWithinItsTokenBudget` does: every other MCP test asserts a tool
*returns* something, so all of them push toward returning more. `graphy_query`
was the proof — the tool whose own description says to prefer it over reading
files returned 199,703 bytes (~50k tokens) at `depth:2` on this repo's graph,
uncapped, with no `truncated` flag because there was nothing to truncate
against. `query.Subgraph` was deleted rather than capped: `Neighbors` already
did the same expansion *plus* the cap, the test filter and the ordering, and a
second uncapped BFS reachable inside the package is how the defect happened.
Both NL paths now go through one `expand()` helper. Keyword-scored seeds skip
test entities, unlike named ones — `Neighbors` exempts seeds so that naming a
test function still answers, but nobody named a keyword seed, and test helpers
routinely own the best-matching label ("how does the MCP server work" seeded on
`mcpTestServer`).

Two wire shapes follow from that. `mcpEdge` references its endpoints by
**index into the same response's `nodes` array** — the edge block was two thirds
of every subgraph payload and every id in it was already in `nodes`; edges are
induced over the nodes that survived the cap, so an index is always defined. And
an entity card's edges name only their other end, since every `Out` edge's
source is the entity the card is about. `graphy report` with no section returns
the small sections and *names* the large ones (35,132 → ~2,000 bytes here);
`--full` keeps the whole thing.

**The bridge.** `graphy export okfw` records `store.MetaBundleKey` (the bundle
root) alongside each `ExportRecord`, because a record holds a *bundle-relative*
path and nothing else identified the bundle — so the mapping could not be turned
into a path anyone could open. `graphy concept` crosses graph→wiki;
`okfw search --node-id` crosses back, which requires `graphy.node_id` to reach
the Bleve index as a keyword field. Every node graphy's MCP tools return carries
its `concept` path. `cmd/graphy`'s `TestSkillDocumentsEveryCommand` /
`TestSkillPromisesOnlyRealCommands` check the agent skill against the registered
command set in both directions: the skill advertised impact analysis for months
while no command answered it, and that drift is now a test failure. `cmd/okfw`
has the same pair plus `TestUpdateToolMatchesTheCLIWriters`, which is why
`main()`'s command tree is extracted into `rootCmd()`.

**`syncBundle` reconciles, it does not replay.** It takes both what pass 2 wrote
and *every* concept the run placed, and unions in anything the Bleve index or
the graph db is missing. Keying only off what was written meant a lost
`.okfw/.index` was never rebuilt: a re-export finds every concept byte-identical,
writes nothing, and so indexed nothing — leaving `okfw search --node-id` dead
for good. It also **creates** `.graph.db` when absent (with a full walk, so
hand-written concepts are in it too) and says so, because the whole exported
tree — one `parent:` edge per concept — was otherwise invisible to every okfw
graph command until a `graph-sync` nothing mentioned. Conversely
`graphdb.OpenExisting` is what the four `okfw graph-query` subcommands use:
`bolt.Open` *creates* the file, and every command that chooses between the store
and a filesystem scan by whether it exists then answered from the empty store,
permanently, from one read-only query.

`scanExisting` walks the **whole bundle**, and a concept found outside the
export directory is moved back through `bundle.MoveConcept`. `okfw move` cannot
update the exports bucket (cmd/okfw imports no graphy package), so a
directory-scoped scan missed it, `adoptRenames` skipped it, and the export
created a duplicate carrying the same `graphy.node_id`. Recovering it in place is
not an option while `linkTo` emits a bare `./<file>.md`.

Exported concepts carry **`kind: "code"`**. Without it `GetKind` falls through
its legacy `type` switch — `code-function` matches nothing — and returns
`"note"`, so `okfw search --kind note` handed back the entire code export
alongside the notes it was asked for, and `SearchOptions` has no negation with
which to exclude it. `GetKind` also classifies any concept carrying a `graphy`
block as code on read, so bundles exported by an older graphy separate without a
rewrite.

## Data Model

Concepts are markdown files with YAML frontmatter. A concept's OKF ID is its file
path within the bundle, minus `.md`. The stable internal `id` is a UUIDv7 (RFC
9562); legacy notes may use `YYYYMMDDHHmmss-UUIDv4`; both formats are accepted.

Placement: an explicit `--path` wins, else an explicit `--project` name as a
top-level folder, else the wiki root. Concepts may live in any directory and
are still found by id (lookup scans the whole bundle). `project` is optional
metadata, never auto-derived.
Kinds (`kind`): `note` (default), `daily-note`, `issue`, and `code` — the last
written only by `graphy export okfw`, never by `okfw create`. OKF's required
free-form `type` field is also written on every concept; for a code concept it
is the finer `code-<entity>`, which `--type` filters on.
Links: vanilla markdown links to bundle-relative (`/dir/file.md`) or relative
(`./file.md`) paths.

**okfw targets OKF v0.2** (`okf_version: "0.2"`, stamped on the bundle-root
`index.md` only — the sole place the spec permits frontmatter in an index file).
Three optional metadata families are implemented, each with a writer, because a
field nothing can write is indistinguishable from one that does not exist —
`resource` sat modelled, schema'd, and documented for months with no way to set
it, and that is the mistake to avoid repeating:

- **Lifecycle** (§5.4–5.5): `status: draft|stable|deprecated` (absent ⇒ `stable`)
  and `stale_after`, an absolute date; stale means `today >= stale_after`,
  inclusive. Writers: `set-lifecycle`, `set-stale-after`. Filter: `search --stale`.
- **Trust** (§5.2–5.3): `generated{by,at}` and `verified[]{by,at}`. A bare
  `verified` mapping MUST normalize to a one-element list — done in
  `VerifiedList.UnmarshalYAML`, pinned by a test. Trust tier keys **strictly** on
  the `human:` actor prefix, so `team:` is machine-confirmed, not human-reviewed.
  Writers: `verify`, `set-generated`. `generated.at` mirrors `timestamp`; one code
  path writes both (`syncGeneratedAt`) and validation rejects a disagreement.
- **Provenance** (§5.1): `sources[]` (`resource` required in-entry) plus a sibling
  `usage_window`. Writer: `add-source`. `sources[].id` is the join key for
  `[^id]` body footnotes — keyed, not positional, so reordering cannot silently
  misattribute a claim.

Attested Computation (§10) is deliberately out of scope: the spec still defers
its receipt/verdict wire formats, attester ABI, and sandboxing.

**Two deliberate divergences from OKF, both load-bearing — do not "fix" them
toward the spec.** okfw requires six fields (`id`, `title`, `type`, `tags`,
`created`, `timestamp`) where §11 requires only a non-empty `type`, and
`#Concept` is a *closed* CUE definition that rejects undeclared top-level keys
where §4.1 says consumers MUST NOT. The trade buys a hard guarantee about the
bundles okfw authors, and graphy's export relies on closedness to catch bad
extension fields. The open `x:` block is the escape hatch that keeps a closed
schema liveable: anything under `x:` passes through untouched, and it is the only
way to attach custom metadata. The consequence is that `okfw validate` rejects
conformant *third-party* bundles, so there are two distinct checks:
`validate`/`validate --all` apply the strict schema, `validate --okf` implements
the permissive §11 conformance check and nothing more.

**okfw has no task tracking.** It once carried `status`/`due`/`completed`/
`priority` for a `todo` kind, where `status` collided with the field v0.2 spec'd
for lifecycle. The feature was removed rather than namespaced — okfw is a
knowledge format, not a task tracker. `okfw migrate` strips those fields from
older concepts (`status: closed` becomes `deprecated`; open/in_progress lose
`status` entirely, since absent means stable). The free-form `type` is left
alone, since the spec does not register type values. Do not reintroduce todo
fields, an `okfw:` block (the name reads as "the OKF fields" and confuses), or
okfw-owned keys inside `x:` (that block belongs to the user).

## Wiki Layout (fixed, inside `.okfw/`)

- Concept markdown at the top level (subdirs via `--path` or an explicit `--project`); `daily/YYYY/MM/` for daily notes
- `.index/` (Bleve search index), `.graph.db` (bbolt graph), `ephemeral/` (export scratch) — all gitignored via `.okfw/.gitignore` written by `okfw init`
- `okfw connect` / `okfw root` verify or print the discovered wiki; commands error with "run `okfw init`" advice when none is found
- `--path`/`--project` reject any segment starting with `.`: a concept written into `.index/` or `.todos/` would be invisible to every walk, since `SkipWalkDir` skips dot-directories

## Key Conventions

- CUE validates concept frontmatter on the **write path**, not just on demand:
  `CreateConcept` refuses to write a schema-invalid concept, and the frontmatter
  mutators refuse an edit that would make a *valid* concept invalid — while still
  permitting edits to an already-invalid one, or the CLI could never repair a
  broken file. `graphy export okfw` validates its hand-built frontmatter too. The
  compiled schema is a process-wide `sync.Once` value; recompiling per call made
  every concept pay CUE compilation.
- Frontmatter is edited by round-tripping `yaml.Node` (`cli/internal/bundle/yamlfm.go`),
  never by rewriting lines. This preserves key order, comments, unknown keys, and
  each scalar's quoting style, and it is what makes nested writes possible at all.
  The line-oriented editor it replaced could only see top-level keys and scanned
  with a `bufio.Scanner` whose unchecked 64KB line cap silently truncated
  frontmatter and wrote the truncated version back.
- `okfw delete` and `okfw move` (`bundle.DeleteConcept` / `bundle.MoveConcept`)
  own the four things a plain `rm`/`mv` would desynchronize: the file, the search
  index, the graph node, and the `index.md` listings. `move` preserves the
  concept's `id` and repairs links at **both** ends — inbound links in other
  concepts *and* the moved concept's own relative links, which resolve against
  its directory and so break silently when it changes directory. `delete`
  reports inbound links but never rewrites them: OKF section 6.1 requires
  consumers to tolerate a link whose target does not exist.
- `WriteIndexFiles` also prunes listings for directories that no longer hold
  concepts (and the emptied directory), or a delete would leave a generated
  `index.md` still advertising what used to be there.
- `bundle.DeleteConceptFile` is `DeleteConcept` without the `index.md` refresh,
  for callers deleting in bulk — `WriteIndexFiles` walks the whole bundle, so
  running it per deletion turns a sweep into quadratic work. Every such caller
  owes one refresh afterwards (`graphy export okfw`'s stale sweep does this in
  its single `syncBundle` pass).
- The Bleve index is pruned by `bundle.PruneIndex`, wired into `graph-sync`.
  Nothing used to prune it, so a deleted concept stayed in `okfw search` results
  forever — `search` reports index hits without stat-ing the file. Bulk writers
  use `bundle.IndexFiles` (one open/close for the batch) and reconcile against
  `bundle.IndexedIDs`, which must **not** create the index it only reads.
- Search snippets use a registered plain highlighter, not bleve's default. The
  default is `html`, whose formatter runs `html.EscapeString` over the text
  *surrounding* each match — so a snippet of a markdown concept came back with
  `&amp;` and `&lt;` in it. That was corruption, not markup, and stripping the
  `<mark>` tags would not have fixed it. The highlighted field is pinned on the
  request too: it used to be whichever key ranging `hit.Fragments` yielded
  first, which is a Go map.
- CUE schemas enforce all validation (concept frontmatter, graph edges)
- bbolt (BoltDB) is the persistent graph store — single-file, read-optimized, pure Go
- Bleve is the full-text search engine — separate from the graph store
- The graph store is optional — commands fall back to filesystem scanning when `.graph.db` doesn't exist
- Run `okfw graph-sync` to build/update the persistent graph for O(1) backlink lookups
- Reserved bundle filenames carry no concept frontmatter: `index.md` (directory listing, auto-maintained by okfw on every note change; root copy carries `okf_version`) and `log.md` (update history, generated from git)
- The Neovim plugin uses snacks.nvim for pickers (not telescope), and needs `plenary.job` at **runtime**, not only for tests
- `okfw mcp` serves an MCP stdio server (cli/cmd/okfw/mcp.go): five consolidated
  tools, advertised as `okfw_search`/`okfw_create`/`okfw_read`/`okfw_update`/
  `okfw_graph` (`read` and `search` are not distinctive names in a client that
  flattens across servers) with the bare names still dispatching. Compact JSON,
  no protocol dependency. `update` actions must stay at parity with the CLI
  writers — a claim that went unchecked long enough to become false, so
  `TestUpdateToolMatchesTheCLIWriters` now pins it, and the actions share the
  CLI's frontmatter mutators rather than reimplementing them. `read` takes
  `section`/`offset`/`limit` and caps at 200 lines rather than dumping a whole
  concept; `search` takes `node_id` — the wiki→graph half of the bridge, which
  needs `graphy.node_id` indexed as a Bleve keyword field (`bundle.IndexFile`
  populates it from the frontmatter extension block) — plus `type`, `offset`,
  and `kind: "code"` for separating graphy's export from hand-written notes
- Both binaries install an agent skill through `cli/internal/agentskill`
  (`okfw install` / `graphy install`), which owns the platform mechanics —
  SKILL.md placement, the managed CLAUDE.md/GEMINI.md/AGENTS.md block, the
  Claude hook merge. It is deliberately tool-neutral because okfw must not
  import a graphy package, and the marker block is namespaced by tool so both
  can maintain one in the same file. Each tool's package holds only its two
  templates. Both skills are written **dual-surface** — every step gives the CLI
  form and the MCP tool on the same line. A trailing flat list of tool names is
  what let graphy's procedure stay CLI-only while its drift test passed, so an
  MCP agent was reading steps written in flags it cannot pass
- The OKF-family CLI writers use `bundle.GuardMutation`, not
  `ValidateGenerated`. The latter is unconditional, so `okfw set-lifecycle` on a
  malformed concept refused and the CLI had no way to repair a broken file —
  contradicting the invariant stated below
- `cli/internal/bundle` is the shared write path for **both** binaries, so it owns
  its tests. Do not reintroduce the pass-through wrappers that used to sit in
  `cmd/okfw/main.go` (`slugify`, `noteFilename`, …): they existed only so
  `package main` tests could reach the package, which attributed the coverage to
  the wrong place and left graphy's use of `bundle` untested
- `cue vet` on the schema alone type-checks it against no data instance and
  cannot catch a concept that violates `#Concept`. It is a syntax gate only; the
  real check is `cli/internal/config/fixtures_test.go`, which validates the
  `cli/testdata/` corpus against the compiled schema
