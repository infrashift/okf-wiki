---
x-okfw-id: "01a208a2-40cb-7f18-b139-7e7ab1f47bc0"
title: "Wiki conventions"
type: "Guide"
tags:
  - "conventions"
  - "meta"
x-okfw-created: "2026-11-23T07:39:40-06:00"
x-okfw-timestamp: "2026-11-23T07:39:40-06:00"
description: "Layout, writing style and authoring rules for the ADRs and issues of this wiki."
---

# Wiki conventions

This guide defines how the okf-wiki OKF wiki (`.okfw/`) stores project knowledge.

- Create concepts with `okfw create` or the okfw MCP `create` tool. Do not write frontmatter by hand.
- Search before you write. Link to related concepts.
- Keep one idea in each concept. Give each concept a one-sentence `description`.
- Do not start a `--body` with `# Title`. okfw writes the H1 from the title.

## Layout
| Path | Type | Contents |
|---|---|---|
| `decisions/` | `ADR` | Architecture decision records. |
| `issues/` | `Issue` (kind `issue`) | Tracked work: features, bugs and design debt. |
| `daily/` | daily note | `okfw daily`: what changed, what you learned, what comes next. |

## ADRs (`decisions/`)
- Title: `ADR-NNNN: <decision>`. Number ADRs in sequence. Do not use a number again.
- Command: `okfw create "ADR-NNNN: <title>" --path decisions --type ADR --tags adr,<topic> -d "<one sentence>" --body "<markdown>"`
- Body: **Status** (Proposed, Accepted, or Superseded by ADR-XXXX), **Date**, **Source**, then **Context**, **Decision**, **Consequences**, **Alternatives considered**.
- **Source** names the commit or the pull request that made the decision. Do not write a reason that no source records.
- To replace an ADR, write the new ADR with a link to the old ADR. Change the Status line of the old ADR. Run `okfw set-lifecycle <old> deprecated --superseded-by <new>`.

## Issues (`issues/`)
- Title: `ISSUE-NNN: <summary>`. Number issues in sequence.
- Command: `okfw create "ISSUE-NNN: <title>" --kind issue --path issues --type Issue --tags issue,open,<area> -d "<summary>" --body "<markdown>"`
- Body: a **Status** line, **Description**, **Acceptance Criteria** (checkboxes), **Related**. Add a **Resolution** section when you close the issue.
- okfw has no status field for issues ([ADR-0005](/decisions/adr-0005-okfw-has-no-task-tracking--01a208a240e8.md)). Tags carry the state: `open`, then `closed`. Change them with `okfw remove-tags` and `okfw add-tags`. Keep the Status line the same as the tags.
- Find open work with `okfw search --path issues --tag open`.

## Writing style
The wiki follows ASD-STE100 Simplified Technical English, as the lexiscope `ste-100` ruleset checks it.

- Use the active voice and the simple present tense.
- Start each procedural step with a verb: Run, Read, Add, Set.
- Keep a procedural step to 20 words or fewer, and a descriptive sentence to 25 words or fewer.
- Use a verb, not a noun form of the verb.
- Do not use the words `your`, `should`, `will`, `would`, `please`, `simply`, `just` or `very`.
- Run `lexiscope scan .okfw` before you commit a change to the wiki.

## Housekeeping
- Use `okfw move` and `okfw delete`. Do not use `mv` or `rm`. The okfw commands repair the links and update the `index.md` listings.
- `status` is the OKF lifecycle field: `draft`, `stable` or `deprecated`. It is not the state of an issue.
- Do not edit `index.md` or `log.md`. okfw writes them.
- Run `okfw validate --all` and `okfw validate --okf` before you commit a change to the wiki.
