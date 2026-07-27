# PRD: okfw - OKF Wiki for Neovim

## 1. Overview
A high-performance, local-first knowledge base — OKF Wiki — consisting of a Go CLI (`okfw`) and a Neovim integration, managing per-project Open Knowledge Format (OKF) wikis. A wiki is a `.okfw/` directory at a project's root, created by `okfw init` and committed with the project. The system prioritizes data integrity, structured search, and interconnected knowledge.

okfw targets **OKF v0.2**. See §4 for the two places it deliberately diverges from
the spec.

## 2. Technical Stack
- **CLI**: Go 1.24+ (using `cobra`)
- **Indexing**: Bleve (Full-text + Structured)
- **Validation**: CUE (Schema-first metadata)
- **Integration**: Neovim (Lua), snacks.nvim, Plenary.job
- **Version Control**: Git (read-only: `okfw log` renders `log.md` from history. okfw never commits, pushes, or backs up on your behalf.)

## 3. Core Requirements
- **Per-Project Storage**: Each project owns a `.okfw/` wiki at its root, created once with `okfw init`. Concept markdown is committed with the project; generated artifacts (`.index/` search index, `.graph.db` graph database, `ephemeral/` exports) are covered by a generated `.gitignore`.
- **Zero Configuration**: No config file or environment variables. Commands discover the wiki by searching upward from the current working directory (like git finds `.git`) and fail with `okfw init` advice when none exists; `okfw connect` verifies the wiki and `okfw root` prints its path.
- **Explicit Metadata**: `project` is optional and never derived from git — pass `-p/--project` on create, or reassign later with `set-project`. Git is consulted only to locate the project root during `okfw init`.
- **Verification**: CUE validates YAML frontmatter on the write path. `okfw create` refuses to write a concept that fails the schema, and an edit that would make a valid concept invalid is rejected before it reaches disk. `okfw validate --all` checks the whole bundle; `okfw validate --okf` checks OKF conformance instead (see §4).
- **Search**: Full-text and field-filtered search via flags (`--project`, `--tag`, `--type`, `--kind`, `--status`, `--stale`), surfaced through a snacks.nvim picker. Query strings are full-text only; there is no `field:value` query syntax.
- **Graphing**: Generate local relationship graphs (forward links and backlinks) as Mermaid diagrams, plus `okfw backlinks` and `okfw graph-query` for structured output.

## 4. OKF conformance and deliberate divergences

okfw implements OKF v0.2, including the lifecycle (`status`, `stale_after`),
trust (`generated`, `verified`), and provenance (`sources`, `usage_window`)
families. **Attested Computation (spec §10) is out of scope**: the spec still
defers its receipt/verdict wire formats, attester ABI, and sandboxing to a future
revision.

Two divergences are intentional. okfw trades tolerance of foreign bundles for a
hard guarantee about the bundles it authors:

1. **okfw requires six fields** — `id`, `title`, `type`, `tags`, `created`,
   `timestamp`. OKF §11 requires only a non-empty `type` and says consumers MUST
   NOT reject a concept for missing optional fields.
2. **The schema is closed** — undeclared top-level keys are rejected, which
   catches typos like `descrption:`. OKF §4.1 says consumers MUST NOT reject
   documents with unrecognized fields. The open `x:` block is the escape hatch:
   anything under it passes through untouched, so custom metadata is still
   possible.

The consequence is that `okfw validate` will reject a conformant third-party
bundle. `okfw validate --okf` exists for exactly that case — it implements the
permissive §11 check (parseable frontmatter, non-empty `type`) and nothing more.

**No task tracking.** okfw once carried `status`/`due`/`completed`/`priority` for
a `todo` kind. OKF v0.2 spec'd `status` as the lifecycle field, and rather than
namespace around the collision the feature was removed: okfw is a knowledge
format, not a task tracker. `okfw migrate` strips those fields from concepts
written before the removal.
