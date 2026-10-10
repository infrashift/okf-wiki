# Do graphy + okfw pay for themselves? A three-arm measurement on garmr

Measured 2026-07-27 against `github.com/infrashift/garmr` at `b57efe7` — a Go+CUE
policy engine, 188 tracked files, 74 Go files / 24k LOC, 27 CUE, 58 markdown.

**Headline: no, not at this repo size.** The tools measurably reduced how much
material the agent had to read — by 46% on the change-impact case — but the
fixed cost of *having* 16 MCP tools loaded exceeded that saving on every one of
five use cases. Quality was a tie; the tooled arm never scored higher.

The retrieval idea works. The packaging is too expensive at 24k LOC.

---

## Method

| Arm | Reaches |
|---|---|
| **A — tooled** | graphy + okfw MCP servers (16 tools), installed skills, managed CLAUDE.md, the wiki |
| **B — unaided** | `Read`/`Grep`/`Glob`/`Bash`. Pristine clone, no wiki, no MCP |
| **C — conventional** | Same as B, plus a system-prompt pointer to `go doc`, `go list`, `govulncheck`, `gopls`, `rg`, `syft`, `grype`, `gh` |

5 use cases × 3 arms × 3 reps = **45 runs**, all valid. `claude -p`,
`claude-sonnet-5`, `--effort high`, fresh session per run, arms interleaved so
cache warmth could not favour one. Answer keys written before any run. Every
answer scored **independently and blind** by Opus against its key
(correctness 0-4, coverage 0-3, actionability 0-2, non-padding 0-1; any
known-false claim caps the total at 3). Arm-identifying strings scrubbed first.

Arm C exists because Arm B alone would strawman the baseline: the honest
comparison for a Go repo is against Go's own tooling.

Spend: $18.43 on runs, $5.00 on judging.

## Results

Median of 3 reps.

| Case | Metric | A (tooled) | B (unaided) | C (conventional) |
|---|---|---|---|---|
| **UC1** change impact | cost | $0.564 | **$0.471** | $0.473 |
| | quality | 8 | 8 | **9** |
| | $/quality pt | 0.0690 | 0.0578 | **0.0525** |
| **UC2** orientation | cost | $0.564 | $0.569 | **$0.445** |
| | quality | 10 | 10 | 10 |
| | $/quality pt | 0.0564 | 0.0569 | **0.0445** |
| **UC3** decision history | cost | $0.109 | **$0.076** | $0.200 |
| | quality | 9 | 9 | 9 |
| | $/quality pt | 0.0121 | **0.0085** | 0.0249 |
| **UC4** dependency risk | cost | $0.851 | **$0.699** | $0.762 |
| | quality | 7 | **8** | 7 |
| | $/quality pt | 0.1216 | **0.0874** | 0.1089 |
| **UC5** localized (control) | cost | $0.175 | **$0.126** | $0.139 |
| | quality | 8 | 8 | 6 |
| | $/quality pt | 0.0218 | **0.0158** | 0.0232 |

**Arm A is last or tied on cost-per-quality-point in all five cases.** It never
achieves a higher median quality score than both baselines.

UC5 was pre-registered as a case the tools *should* lose. They lost it, and the
other four as well.

## Why: a 5,231-token tax on every request

Measured with an identical trivial prompt (`"Reply with exactly: OK"`) in each arm:

| | first-request context | tools |
|---|---|---|
| Arm A | **23,943 tokens** | 21 (16 MCP) |
| Arm B | **18,712 tokens** | 5 |

**Δ = 5,231 tokens**, paid on every request — the 16 MCP tool schemas plus the
managed CLAUDE.md blocks and skill listings. Multiplied by turn count it accounts
for most of Arm A's excess consumption:

| Case | A−B extra context | explained by the prefix tax |
|---|---|---|
| UC2 | 117,573 | **93%** |
| UC3 | 40,629 | **90%** |
| UC1 | 199,989 | **78%** |
| UC5 | 83,253 | 57% |
| UC4 | 373,141 | 36% |

On three of five cases, Arm A's entire disadvantage is the cost of *having* the
tools loaded — not of using them badly.

## The tools did work — that is what makes this worth acting on

Bytes of tool output consumed per run:

| Case | A | B | A's reduction |
|---|---|---|---|
| UC1 | **82 KB** | 152 KB | **−46%** |
| UC2 | **119 KB** | 138 KB | −14% |
| UC4 | 80 KB | 78 KB | — |
| UC3 | 18 KB | 9 KB | +100% |
| UC5 | 11 KB | 5 KB | +120% |

On UC1 the tooled agent reached the same answer having read **less than half**
the raw material, and on UC2 it opened 5.7 files against the baselines' 10.0.
The retrieval mechanism does what it claims. It is simply outweighed by a
fixed overhead nine times larger than the saving.

**Crossover estimate.** UC1's 70 KB saving is roughly 18k tokens against a 157k
token prefix tax. The saving scales with repo size; the tax does not. Very
roughly, the sign flips somewhere around **150k–250k LOC** — an order of
magnitude above garmr. This experiment is structurally biased against graphy and
should not be read as a verdict at scale.

## Behavioural findings

**1. The tools were used *in addition to* search, not instead of it.** On UC1
Arm A made 22 MCP calls *and* 40 greps *and* 15 reads. Genuine substitution
appears only on UC2 (28 MCP calls, 3 greps) and UC3 (10 MCP calls, **zero file
reads**).

**2. On UC4 the wiki was bypassed.** Arm A made 6 MCP calls and **60 Bash
commands** — it re-ran dependency scanning itself rather than reading the
briefing written for exactly that question, and scored *lower* than the arm that
computed the answer from scratch. The okfw skill's `description` is phrased as a
write trigger ("save a note, idea, or decision; journal in a daily note"), and
skill descriptions are what the model matches against. On a retrieval question it
does not fire. **This is the cheapest high-value fix these results suggest.**

This was fixed and re-measured — see the addendum below. Cost fell 83% and
quality rose from 7 to a perfect 10.

**3. The graph could not answer the case built to showcase it.** UC1 asked what
calls the policy-evaluation entry point. `graphy callers "Engine.Evaluate"`
returns **zero actual callers** — both real call sites are dropped by resolution
(see below). Arm A's 22 MCP calls returned nothing usable and it grepped anyway.

**4. The baseline never fell into the trap the graph was supposed to catch.**
UC1's answer key flags a grep-conflation error: `cmd/garmr/eval.go:126` and
`internal/client/client.go:147` call the *client's* `Evaluate`, not the engine's.
**No run in any arm made that error** (0 known-false hits across all 9 UC1 runs).
Sonnet with grep was careful enough on its own. The graph's differentiating
advantage never materialised because the baseline did not need it.

---

# Addendum: the skill-description fix, measured

Behavioural finding 2 was acted on immediately. Two changes, no code:

1. **`SKILL.md.tmpl` description** rewritten to lead with retrieval and to name
   the *kinds* of knowledge the wiki holds ("design decisions and the reasoning
   behind them, security and dependency findings, operational context, open
   work"), plus "Check here before deriving an answer from scratch or re-running
   an analysis". A new **"Search before you derive"** section lists the triggers
   — including "run a scan or analysis whose output someone may already have
   recorded *and adjudicated*". The Workflow section now leads with retrieving,
   not capturing.
2. **`instructions.md.tmpl`** — the always-loaded CLAUDE.md block — had exactly
   one mention of search, *"Search before you write"*, framing retrieval purely
   as a precondition for authoring. This was the larger defect, since that text
   is in context on every request. Split into "Search before you derive" (with
   the same trigger list) and a demoted "Search before you write, too".

UC4 re-run, Arm A, 3 reps, everything else identical:

| | before | after |
|---|---|---|
| Median cost | $0.851 | **$0.148** (**−83%**) |
| Median turns | 26 | **10** |
| MCP calls / run | 1–3 | **7–8** |
| Bash commands / run | 17–23 | **1–2** |
| Quality (3 reps) | 7 | **10, 10, 10** — all 10 key items, zero false claims |
| $/quality point | 0.1216 | **0.0148** (**8.2× better**) |

Against the best baseline on this case (Arm B: $0.699 at quality 8), the fixed
Arm A is now **79% cheaper and higher quality**. UC4 flips from Arm A's worst
loss to a decisive win.

The fix generalises to the other wiki-dependent case. UC3 re-run, Arm A:
median cost $0.109 → **$0.082** (−25%), turns 7 → 5, tool calls now
`okfw_search` ×2 + `okfw_read` ×1 rather than a file hunt. That brings it level
with Arm B's $0.076, from 43% behind.

The fixed prefix grew by only **186 tokens** (23,943 → 24,129) — the longer
description costs essentially nothing against what it recovers.

**What this means for the headline result.** The measured verdict above stands
as measured, but one of the five losses was a text defect, not an architectural
one. The corrected picture: okfw wins decisively where the wiki holds knowledge
the repo does not (UC4), ties where the repo already records it (UC3), and the
graph-side cases (UC1, UC2, UC5) are unchanged — still dominated by the
5,231-token MCP prefix tax. **Retrieval tooling is only as good as the sentence
that tells the model when to reach for it.**

---

# Addendum 2: re-measuring after the graphy fixes

UC1 re-run on Arm A with all four resolution fixes, and UC3 re-run after the
skill-description fix. 3 reps each, everything else identical.

| | UC1 before | UC1 after | UC3 before | UC3 after skill fix | UC3 after content fix |
|---|---|---|---|---|---|
| Median cost | $0.564 | $0.631 | $0.109 | $0.110 | $0.129 |
| Median quality | 8 | **9** | 9 | **3** | **10** |
| $/quality point | 0.0705 | 0.0701 | 0.0121 | — | 0.0129 |
| Best baseline | 0.0525 (C) | 0.0525 (C) | 0.0085 (B) | 0.0085 (B) | 0.0085 (B) |

**UC1: better answers, not cheaper ones.** Quality rose 8 → 9, with one rep
hitting all eight key items at full correctness. Cost rose too, and
cost-per-quality is flat. Part of the reason is self-inflicted by the fix:
`callers Engine.Evaluate --tests` now returns **99** callers where it once
returned nothing, so the tool's answers got *bigger*. Variance is wide — the
cheapest rep ($0.527) leaned on the graph (6 `graphy_callers`, 3 greps); the
dearest ($0.680) grepped 13 times anyway. **Arm C still wins this case.**

## The finding worth the whole exercise: a wiki concept became a hallucination vector

UC3's median quality *collapsed* from 9 to **3** after the skill-description fix
— two of three runs tripped the hard gate for inventing rationale.

The cause was **content I wrote**, not tooling. My decision concept ended with:

> "…which would also mean accepting evaluation-time I/O, and with it latency and
> failure modes that the engine currently does not have."

That is my analysis. It appears nowhere in `TODO.md`. The agent read it and
emitted it under a heading titled **"Reasoning:"** — presenting one reader's
inference as the project's recorded rationale — then embellished further
("timeouts, partial data, auth to external systems").

Two things make this worth recording:

1. **The skill fix made it worse.** Before, the agent largely ignored the wiki
   and answered from `TODO.md`, so the invention never happened. Getting
   retrieval to work is what exposed the bad content. Fixing discoverability
   raises the cost of unreliable concepts.
2. **A concept that blends record with inference is indistinguishable from one
   that doesn't** — to the agent and to a human reader.

The fix was editorial: an explicit `## Analysis — not part of the recorded
decision` heading, stating that everything above is from the record and what
follows is one reader's assessment. Applied to three decision concepts. Result:
**quality 10 / 9 / 10, zero gate hits**, up from 3 / 3 / 9.

**Convention to adopt: a concept must not mix what a source says with what its
author concluded, unless the boundary is on the page.** okfw already has the
machinery for the strong version — `sources[]` with `[^id]` body footnotes — and
the CVE concepts here use it. The decision concepts did not, and that is exactly
where the failure landed.

---

# Defects found in graphy and okfw

## Caller recall fails on exactly the methods that matter most — FIXED

A method whose bare name is shared by two or more types in the repo resolves
**zero** callers:

| Method | types owning that bare name | callers resolved |
|---|---|---|
| `Engine.LoadPolicy` | 1 | **32** |
| `NewEngine` (pkg-qualified func) | 1 | **45** |
| `Engine.Evaluate` | 2 (`Engine`, `Client`) | **0** |
| `Engine.Validate` | 4 (`Engine`, `Client`, `Validator`, `QuickValidator`) | **0** |

A call through a struct field (`s.engine.Evaluate(...)`) gives resolution only
the bare tail `Evaluate`. The deliberate tail-drop rule — "resolve at most one
candidate, otherwise drop" — then discards it. The rule is right for the case it
was written for (`err.Error()` fanning out over every error type), but its cost
is that **recall is inversely correlated with architectural importance**: names
like `Evaluate` and `Validate` recur across layers precisely because they are the
central verbs. 74% of refs (7,243 of 9,860) were dropped on this repo.

**Fixed.** The extractor now records struct field types (`Node.Fields`) and
method receiver names (`Node.Recv`); `goReceiverQualified` rewrites
`s.engine.Evaluate` into `Engine.Evaluate` using the enclosing method's receiver
type, mirroring what `goPackageDirs` already did for import qualifiers. It also
follows Go method promotion into embedded types.

Fixing it exposed an adjacent, worse bug: the **same-file shortcut ran before the
tail-drop rule**, so a tail-only hit whose bare name happened to be declared in
the same file was stamped EXTRACTED — the highest confidence — on a qualifier
that had already been discarded. `defer resp.Body.Close()` became
"`Client.Evaluate` calls `Client.Close`, EXTRACTED", and `err.Error()` did
exactly what the tail rule exists to prevent. The shortcut now requires a full
match.

A third change types **local** receivers — `eng := engine.NewEngine();
eng.Evaluate()` — from parameters, `var x T`, `x := &T{}`, and the constructor
idiom `x := NewT()`, the last resolved through the callee's recorded first
result type.

Measured on garmr, three binaries, identical input:

| | baseline | +receiver fields<br>+same-file fix | +local typing | +scope ranges |
|---|---|---|---|---|
| `callers Engine.Evaluate` | **0** | `Server.handleEvaluate`, `Runner.evaluate` | + **99** with `--tests` | ✓ |
| `callers Engine.Validate` | **0** | `Server.handleValidate` | ✓ | ✓ |
| Edges | 4,038 | 4,035 | 4,214 | **4,216** |
| Go methods with ≥1 caller | 138/231 | 136/231 | **144/231** | 144/231 |
| AMBIGUOUS | 26 | 26 | 26 | **26** |

The middle column *lowers* the method count deliberately: it removes 17 false
callers while adding 14 true ones. Every one of both sets was audited against
source — all 14 additions genuine, all 17 removals false positives (six
`resp.Body.Close()`, two `err.Error()`, plus `os.Stat`, `watcher.Close()`,
`object.Close()`, `hh.Handler()`).

Local typing then added **179 edges and removed none**. 15 are in production
code, including `runEval → Client.Evaluate` at `cmd/garmr/eval.go:126` — the
exact decoy UC1's answer key is built around, now correctly attributed to the
*client* rather than conflated with the engine. The other 164 are the test suite
that exercises the engine, which is UC1's key item K6 ("every test that
exercises it").

Scope ranges then fixed **shadowing**, which a flat name→type map gets actively
wrong. garmr contains a textbook case: `internal/storage/backend_test.go`'s
`TestErrorTypes` runs three `t.Run` subtests, each declaring `err :=` with a
different error type. Last-declaration-wins typed all three as the *last*
subtest's type, and because the three calls then deduped onto one edge key the
graph carried **a single edge pointing at the wrong type**. Scope-ranged locals
emit three, each correct:

| | line 83 | line 91 | line 98 |
|---|---|---|---|
| flat | `ErrUnknownBackend` ✗ | — | — |
| scoped | `ErrNotFound` ✓ | `ErrAccessDenied` ✓ | `ErrUnknownBackend` ✓ |

**AMBIGUOUS never moved across any step, and no true edge was lost at any step.**
One apparent regression during development turned out to be Go method promotion
through an embedded field; it is now handled and covered by a test.

## The HTTP route table is invisible

`Server.handleEvaluate` shows 0 callers because
`mux.HandleFunc("/v1/evaluate", s.handleEvaluate)` passes a method *value*, not a
call. No edge of any kind is recorded, so the graph cannot answer "what handles
this route".

## `graphy report`'s "God nodes" table is topped by test helpers

`makePolicy` (degree 91), `loadTestPolicy` (82), `captureOutput` (57),
`newTestServer` (47) — four of the top ten. `Node.Test` exists specifically
because "test files inflate `Degree`, which `--min-degree` reads as
significance", but the report does not apply that filter. The table presenting
itself as "the graph's flow hubs" is mostly the test suite.

## Lockfiles are ingested as architecture

Before exclusion, `docs/pnpm-lock.yaml` contributed 65 nodes and its `packages`
key ranked **4th** among the graph's most-connected entities. Dependency
lockfiles are machine-generated inventory; the default `.graphyignore` should
carry `*-lock.yaml`, `*.lock`, `package-lock.json`.

## okfw forces create-then-repair authoring

`okfw create` appends a UUID to the filename
(`the-go-toolchain-is-the-largest-cve-lever--019fa5a38a50.md`), so a link to a
concept **cannot be written before that concept exists**. Authoring 18
cross-linked concepts required a second pass rewriting 14 link occurrences and a
manual reindex. The okfw SKILL.md instructs agents that "a new concept should
link to at least one existing one" without mentioning this.

## The okfw skill description is write-biased — FIXED

Covered above under behavioural finding 2, and fixed in
`cli/internal/okfwskills/assets/{SKILL.md.tmpl,instructions.md.tmpl}`. It was
the reason the wiki went unread on the one case where it held knowledge nothing
else had. Re-measured: −83% cost, quality 7 → 10. See the addendum.

## The Bleve index is not carrying its weight

13 MB of index for **1.4 MB** of markdown across 905 concepts (~9×). Measured
alternatives on the same bundle:

| Operation | method | time |
|---|---|---|
| full-text scan, whole bundle | `rg` | **17 ms** |
| `kind: "code"` filter (883 hits) | `rg` | **14 ms** |
| tag filter | `rg` | **10 ms** |

OKF §3.1 says a consumer "can synthesize [a tag-browsing view] at consumption
time by scanning frontmatter"; the spec requires no search infrastructure. Ten of
the index's thirteen mapped fields are keyword metadata filters that a scan does
in 10 ms. Only `body`, `title`, `description` need an inverted index, and `body`
is what makes it large.

The index is also a second source of truth that can desynchronize: `PruneIndex`,
`IndexedIDs`, the "must not create the index it only reads" rule, and the
`syncBundle` reconcile-don't-replay fix all exist *only because the index
exists*. Note the inconsistency — `.graph.db` is already optional with a
filesystem fallback; the index is not.

**Suggested:** make search scan-and-score by default and treat the index the way
`.graph.db` is already treated — built on demand past a size threshold, with
commands falling back to scanning. Smaller first step: drop `body` from the
mapping and measure.

---

# When to use what

**Use graphy when** the repo is large enough that the fixed MCP overhead is small
relative to what an agent would otherwise read — on this evidence, well north of
100k LOC — and when the question is genuinely structural (subsystem orientation,
"how do these connect"). UC2 is where substitution actually happened.

**Do not use graphy for** localized questions in a known file. UC5 cost 38% more
for the same answer. One `Read` beats any graph query, and having the graph
available made the agent explore *more*.

**Do not expect graphy to answer impact analysis on Go code** until tail-only
resolution handles struct-field receivers. On this repo it returns nothing for
the entry points anyone would ask about.

**Use okfw for knowledge that is genuinely not in the repo** — the dependency
reachability adjudication was worth having. But note UC3: garmr's `TODO.md`
already recorded every rejected design, and Arm B answered from it for **30%
less** than Arm A spent reading the wiki. **A wiki that restates a
well-maintained in-repo document is a net cost.** Write down what the repo does
not already say.

**For Go dependency risk, use `govulncheck`, not `grype`:**

| | govulncheck | syft + grype |
|---|---|---|
| wall time | **5.1 s** | ~30 s + DB update |
| findings | 27 symbol-reachable, with call traces | 159 |
| false Criticals | 0 | **8** (all `x/crypto`) |

grype matches at module granularity, which is the wrong granularity for Go — a
module is a unit of distribution, not of linkage. Its 8 Critical `x/crypto`
findings are unreachable: only `blake2b` and `argon2` are linked, via `minio-go`.
Also avoid `go mod why` for this — it answers for the bare package path and
reported "main module does not need package golang.org/x/net" while the server
links eight x/net packages. Use `go version -m <binary>` or `go list -deps`.

**For human search, prefer fzf over either.** Emit structured data and let the
picker work: `okfw search --json | fzf --preview`. The Neovim plugin already made
this call with snacks pickers.

---

# What would make these conclusions weak

1. **One repository, one harness, one model.** This is a statement about garmr on
   Claude Code 2.1.220 with Sonnet 5, not about knowledge graphs. 24k LOC is
   comfortably grep-able and the result may invert at 200k.
2. **The wiki was authored by the same person who wrote the prompts.** Mitigated
   by writing all 18 concepts from repo content before writing any prompt, and by
   restricting the strongest claims to graph-derived cases. Not eliminated.
3. **The judge is a Claude model grading Claude output.** No gold-standard
   calibration set and no human spot-check were run, so the quality axis is
   weaker than the token axis. Since quality was a *tie* in 4 of 5 cases and the
   cost result is large and consistent, the conclusion does not rest on it.
4. **3 reps.** Enough to show a consistent direction — Arm A is last in all five
   cases — not enough for a significance claim.
5. **Read-only workload.** `Edit`/`Write` were disabled, so graphy's
   "run `graphy update` after editing" loop is untested.
6. **Arm C's system prompt names `govulncheck`,** which telegraphs UC4. It still
   lost UC4 to plain Arm B.

## Reproducing

Harness, prompts, answer keys, per-run JSON and judgments are under
`/home/ryancraig/_scratch_/ab/`: `run.sh`, `metrics.sh`, `judge.sh`, `valid.sh`,
`prompts/`, `keys/`, `out/{runs,judged,metrics.tsv,scores.tsv}`.

**One harness note worth carrying elsewhere:** a network-interrupted `claude -p`
run reports `subtype: "success"` with `is_error: true` and
`terminal_reason: "api_error"`. Filtering on `subtype` alone admits truncated
2-turn stubs as valid results. Validity requires all three fields
(`valid.sh`).
