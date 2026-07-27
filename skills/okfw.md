---
name: okfw
description: Capture, link, and retrieve project knowledge in the project's OKF wiki (.okfw/). Use when the user wants to save a note, idea, or decision; journal in a daily note; or find/connect existing project knowledge.
---

# OKF Wiki (`okfw`)

The project keeps a local-first knowledge base in `.okfw/` at the project
root: markdown **concepts** with YAML frontmatter, woven together by links.
Search (Bleve), a link graph, and OKF `index.md` listings are maintained
automatically on every change — no bookkeeping commands needed.

## Interface

Prefer the `okfw` MCP tools when connected (`search`, `create`, `read`,
`update`, `graph` — compact JSON, wiki-relative paths). Otherwise use the CLI;
every command is self-documenting (`okfw <cmd> --help`) and read commands
support `--json`:

```bash
okfw init                                  # once per project (no-op if present)
okfw create "Title" --body "..." --json    # returns {"id","path"}
okfw search "query" --json                 # full-text + filters
okfw daily                                 # today's daily note
okfw backlinks <id-or-path> --json
okfw move <id-or-path> archive/             # relocate; repairs links both ways
okfw delete <id-or-path> --force            # destructive; --force skips the prompt
okfw validate --all                        # strict schema check of every concept
```

If a command fails with "no OKF wiki found", run `okfw init` at the project
root first.

## Conventions that matter

1. **Link actively — links are the product.** Connect related concepts with
   plain markdown links using wiki-relative targets:
   `[Display Title](/research/protocols/deep-note--019f23d3.md)` (leading `/`
   = wiki root; `./relative.md` also works). Links power `backlinks` and the
   graph. A new concept should link to at least one existing concept when a
   relation exists — search first to find candidates.
2. **One idea per concept.** Small, atomic, densely linked notes beat long
   documents. Split rather than append unrelated content.
3. **Write a `description`.** The one-sentence description feeds search
   snippets and the auto-generated `index.md` directory listings.
4. **Organize with `dir`/`--path`** (e.g. `research/protocols`) for topical
   subdirectories, and optional `project` metadata for cross-cutting
   filtering. Untouched, notes land at the wiki root — that's fine.
5. **Daily notes are for capture** (`daily/YYYY/MM/`), one per day,
   idempotent. Refine durable ideas out of them into their own concepts, then
   link.
6. **Mark superseded knowledge rather than deleting it.** `status` is the OKF
   lifecycle field — `draft`, `stable` (the default when absent), or
   `deprecated`. Set it with `okfw set-lifecycle`, and `stale_after` with
   `okfw set-stale-after` when a concept has a known shelf life. okfw has **no
   task-tracking fields**; `status` is lifecycle only.
7. **Cite where knowledge came from** with `okfw add-source --resource <uri>`,
   and record review with `okfw verify`. Custom metadata goes under the `x:`
   frontmatter block — it is the only place unrecognized keys are accepted, as
   the schema is otherwise closed and will reject a typo'd field.
8. **Reorganize with `okfw move`, not `mv`.** It keeps the concept's `id`,
   retargets every inbound link, and fixes the moved concept's own relative
   links; moving the file yourself silently breaks both. `okfw delete` likewise
   clears the search index and graph node that a plain `rm` would strand.
   Deleting leaves inbound links dangling on purpose — OKF tolerates them.
9. **Don't edit `index.md` or `log.md`** — they are reserved, generated files
   owned by okfw.

## Workflow

Capture quickly → refine → link. Before creating, `search` to avoid
duplicates and to find concepts the new note should link to. After
substantive file edits made outside okfw tools, `okfw index <file>` (or any
okfw mutation) refreshes the search index; `okfw graph-sync` refreshes the
link graph.
