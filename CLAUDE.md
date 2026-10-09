# OKF Wiki (`okfw`)

A Go CLI for managing a local-first, file-based knowledge base that conforms to
the [Open Knowledge Format (OKF)](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md).
Concepts are markdown files with YAML frontmatter, stored in a per-project OKF
**bundle**: a `.okfw/` directory at the project root, created by `okfw init`
and discovered by every command walking upward from the cwd (like `.git`).
There is no config file — storage is standardized. (Formerly the Zettelkasten
CLI `zk`.)


## Project Layout

```
cli/cmd/okfw/                — Cobra CLI: rootCmd() in main.go, plus filecmds.go
                               (delete/move), okfcmds.go (OKF writers, install), mcp.go
cli/internal/config/         — .okfw discovery, CUE schema, concept parsing
cli/internal/bundle/         — Bundle write path (concept creation, frontmatter,
                               move/delete, index.md listings, link-graph load)
cli/internal/graph/          — In-memory graph + rendering (ASCII tree, Mermaid)
cli/internal/search/         — Concept search: scan + rank, no index
cli/internal/agentskill/     — Agent-skill installer (SKILL.md placement, managed
                               CLAUDE.md/GEMINI.md/AGENTS.md block)
cli/internal/okfwskills/     — okfw's SKILL.md / CLAUDE.md-block templates
cli/internal/templates/      — Embedded note templates
cli/internal/gitctx/         — Git root lookup
lua/okfw/                    — Neovim plugin (snacks-based pickers)
plugin/okfw.lua              — Neovim plugin auto-load
```

## Build & Test

```bash
make build           # Builds ./okfw (CGO_ENABLED=0 — keep okfw pure Go)
make test            # Runs Go tests + CUE schema validation
make test-race       # Go tests under the race detector
make install         # Installs okfw to $GOPATH/bin
make integration-test # NeoVim plugin suite (sources plugin/okfw.lua for the :Okfw* commands)
make e2e             # The NeoVim suite against the okfw binary, inside a container
go test ./cli/... -count=1   # Run all tests
```

The repo once also incubated `graphy`, a CGo code-knowledge-graph binary that
exported into okfw bundles. It was decommissioned; do not reintroduce its
frontmatter block (`graphy:`), `kind: code`, or `search --node-id`.

## Data Model

Concepts are markdown files with YAML frontmatter. A concept's OKF ID is its file
path within the bundle, minus `.md`. The stable internal `id` is a UUIDv7 (RFC
9562); legacy notes may use `YYYYMMDDHHmmss-UUIDv4`; both formats are accepted.

Placement: an explicit `--path` wins, else an explicit `--project` name as a
top-level folder, else the wiki root. Concepts may live in any directory and
are still found by id (lookup scans the whole bundle). `project` is optional
metadata, never auto-derived.
Kinds (`kind`): `note` (default), `daily-note`, `issue`. OKF's required
free-form `type` field is also written on every concept, and `--type` filters
on it.
Links: vanilla markdown links to bundle-relative (`/dir/file.md`) or relative
(`./file.md`) paths.

**okfw targets OKF v0.2** (`okf_version: "0.2"`, stamped on the bundle-root
`index.md` only — the sole place the spec permits frontmatter in an index file).
Three optional metadata families are implemented, each with a writer, because a
field nothing can write is indistinguishable from one that does not exist —
`resource` sat modelled, schema'd, and documented for months with no way to set
it, and that is the mistake to avoid repeating:

- **Lifecycle** (§5.4–5.5): `status: draft|stable|deprecated` (absent ⇒ `stable`)
  and `stale_after`, an absolute instant; stale means `now >= stale_after`
  (`config.IsStale`, the one implementation). Writers: `set-lifecycle`,
  `set-stale-after`. Filter: `search --stale`.
- **Timestamps** (§5): every timestamp-valued key — `stale_after`,
  `generated.at`, `verified[].at`, `sources[].last_modified`, `usage_window`,
  and okfw's own `created`/`timestamp` — is an ISO 8601 datetime with an
  explicit offset (`#DateTime` in the schema, `dateTimeRe` in Go; keep them in
  step). A bare date is rejected on read and never treated as stale — the spec
  moved off dates because one names a different instant in every timezone, and a
  midnight-UTC fallback would bring that back. Writers accept a bare date and
  store midnight UTC; `okfw migrate` upgrades the ones older okfw wrote.
  Validation decodes frontmatter through `yaml.Node` and keeps timestamp scalars
  as written: decoded into `map[string]interface{}`, yaml.v3 turns an unquoted
  `2026-12-31` into a `time.Time` that reaches CUE as a valid datetime.
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
bundles okfw authors. The open `x:` block is the escape hatch that keeps a closed
schema liveable: anything under `x:` passes through untouched, and it is the only
way to attach custom metadata. The consequence is that `okfw validate` rejects
conformant *third-party* bundles, so there are two distinct checks:
`validate`/`validate --all` apply the strict schema, `validate --okf` implements
the permissive §11 conformance check and nothing more.

**The divergences bind writes and `validate`, never reads.** `ParseFrontmatter`
is lenient — a field with the wrong shape is left at its zero value rather than
failing the concept — and a concept with no `id` is keyed by its OKF concept ID
(`config.ConceptKey`: the bundle-relative path minus `.md`) in search, the link
graph and the listings. Strict decoding on reads made such a concept vanish from
every view, so the one tool that could report it could not find it. Strictness
lives in `ParseAndValidate`. `index-md`/`migrate` adopt an unmarked `index.md`
only in a directory holding a concept okfw wrote (one with an `id`).

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
- `ephemeral/` (export scratch) is the only generated directory, gitignored via `.okfw/.gitignore` written by `okfw init`. There is no search index and no graph database: both were removed, and `okfw init` must not reintroduce them to the ignore list
- `okfw connect` / `okfw root` verify or print the discovered wiki; commands error with "run `okfw init`" advice when none is found
- `--path`/`--project` reject any segment starting with `.`: a concept written into a dot-directory would be invisible to every walk, since `SkipWalkDir` skips dot-directories

## Key Conventions

- CUE validates concept frontmatter on the **write path**, not just on demand:
  `CreateConcept` refuses to write a schema-invalid concept, and the frontmatter
  mutators refuse an edit that would make a *valid* concept invalid — while still
  permitting edits to an already-invalid one, or the CLI could never repair a
  broken file. The compiled schema is a process-wide `sync.Once` value; recompiling per call made
  every concept pay CUE compilation. Its use is serialized by `schemaMu`: CUE
  v0.7 writes to its runtime during unification, so concurrent validations race.
- Frontmatter is edited by round-tripping `yaml.Node` (`cli/internal/bundle/yamlfm.go`),
  never by rewriting lines. This preserves key order, comments, unknown keys, and
  each scalar's quoting style, and it is what makes nested writes possible at all.
  The line-oriented editor it replaced could only see top-level keys and scanned
  with a `bufio.Scanner` whose unchecked 64KB line cap silently truncated
  frontmatter and wrote the truncated version back.
- `okfw delete` and `okfw move` (`bundle.DeleteConcept` / `bundle.MoveConcept`)
  own what a plain `rm`/`mv` would desynchronize: the links and the `index.md`
  listings. `move` preserves the
  concept's `id` and repairs links at **both** ends — inbound links in other
  concepts *and* the moved concept's own relative links, which resolve against
  its directory and so break silently when it changes directory. `delete`
  reports inbound links but never rewrites them: OKF section 6.1 requires
  consumers to tolerate a link whose target does not exist.
- **okfw maintains only the `index.md` files it generated**, identified by
  `IndexMarker` in the body. `WriteIndexFiles` (every write) creates missing
  listings and rewrites marked ones, skipping any that are already current;
  `AdoptIndexFiles` (`index-md`, `migrate`) also takes over unmarked listings in
  directories holding okfw concepts. Only a marked listing is ever pruned (with
  its emptied directory). OKF lets index files exist without a tool, so an
  unmarked one is someone else's — treating every `index.md` as okfw's deleted
  hand-written listings of foreign concepts.
- **A directory listing past `indexEnumerateLimit` (50) groups by `type` instead
  of naming every concept.** OKF section 8 gives `index.md` one job — showing
  what is available *before* opening documents — and an exhaustive listing stops
  doing that job exactly when a directory grows big enough for the question to
  matter. A generated 900-concept directory produced a 112KB, ~28,000-token
  "preview" that cost 23x reading what it previewed; grouped, it was 8.7KB and
  carried the directory's shape as per-type counts. Each group names up to
  `indexPerTypeLimit` (10) concepts, counts what it withheld, and gives the
  `okfw search --type` that returns the rest.
  Small directories still enumerate: for a hand-written `knowledge/` of 18
  concepts, naming them all is the more useful listing.
- `bundle.DeleteConceptFile` is `DeleteConcept` without the `index.md` refresh,
  for callers deleting in bulk — `WriteIndexFiles` walks the whole bundle, so
  running it per deletion turns a sweep into quadratic work. Every such caller
  owes one refresh afterwards.
- **Search and the link graph are computed from the files, not cached.**
  `search.Search` walks the bundle, filters on frontmatter and ranks by a
  weighted field match; `bundle.LoadGraph` builds the link graph the same way.
  Measured on a 918-concept bundle: search ~50ms, a graph query ~300ms, against
  13MB + 4.7MB of derived state that the caches used to cost. The point was
  never the megabytes — it was that a cache is a second source of truth. Every
  rule that once existed to stop it drifting (prune on delete, sync after a bulk
  write, open-existing rather than create, reconcile rather than replay) was a
  rule about the cache, and all of them are gone with it. Scanning is linear in
  bundle size, so this is the first design choice to revisit past roughly tens
  of thousands of concepts.
- Ranking is deliberately kept. The dominant agent pattern is search then read
  under a small result cap, so the caller needs the *best* few hits rather than
  the first few in directory order. `scoreConcept` weights title over tags over
  description over body, damps repeated body matches, and adds a coverage bonus
  for matching every term — the property BM25 saturation buys, obtained by
  rewarding coverage instead of damping frequency.
- Search snippets are plain text. The bleve implementation this replaces used the
  default `html` highlighter, whose formatter ran `html.EscapeString` over the
  text *surrounding* each match, so a snippet of a markdown concept came back
  containing `&amp;` and `&lt;`. That was corruption, not markup. Keep snippets
  as raw body text.
- CUE schemas enforce all frontmatter validation
- okfw carries no database: no search index, no graph store. Do not add one
  without revisiting the "computed from the files" rule above
- Reserved bundle filenames carry no concept frontmatter: `index.md` (directory listing; okfw maintains the ones it marked on every note change; root copy carries `okf_version`) and `log.md` (update history, generated from git)
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
  concept; `search` takes `kind`, `type`, `status`, `stale` and `offset`.
  Every tool result is capped and says so (`truncated`, `next_offset`, `… N
  more`); `TestEveryMCPToolIsCappedAndSaysSo` is driven from the tool list so a
  tool added later cannot escape it, and exists because every other MCP test
  pushes toward returning more
- `okfw install` writes the agent skill through `cli/internal/agentskill`, which
  owns the platform mechanics (SKILL.md placement, the managed
  CLAUDE.md/GEMINI.md/AGENTS.md block, namespaced by tool so it coexists with
  user content); `okfwskills` holds only the two templates.
  `TestSkillDocumentsEveryCommand` / `TestSkillPromisesOnlyRealCommands` check
  the skill against `rootCmd()` in both directions. The skill is written
  **dual-surface** — every step gives the CLI form and the MCP tool on the same
  line, since a trailing flat list of tool names lets a procedure drift to
  CLI-only flags an MCP agent cannot pass
- The OKF-family CLI writers use `bundle.GuardMutation`, not
  `ValidateGenerated`. The latter is unconditional, so `okfw set-lifecycle` on a
  malformed concept refused and the CLI had no way to repair a broken file —
  contradicting the write-path invariant above
- `cli/internal/bundle` owns its tests. Do not reintroduce the pass-through
  wrappers that used to sit in `cmd/okfw/main.go` (`slugify`, `noteFilename`,
  …): they existed only so `package main` tests could reach the package, which
  attributed the coverage to the wrong place
- `cue vet` on the schema alone type-checks it against no data instance and
  cannot catch a concept that violates `#Concept`. It is a syntax gate only; the
  real check is `cli/internal/config/fixtures_test.go`, which validates the
  `cli/testdata/` corpus against the compiled schema
