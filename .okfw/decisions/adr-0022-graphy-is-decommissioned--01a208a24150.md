---
x-okfw-id: "01a208a2-4150-708f-903c-342f379e3548"
title: "ADR-0022: graphy is decommissioned"
type: "ADR"
tags:
  - "adr"
  - "scope"
x-okfw-created: "2026-11-23T07:39:41-06:00"
x-okfw-timestamp: "2026-11-23T07:39:41-06:00"
description: "The CGo code-knowledge-graph binary that incubated in this repository is removed, with the okfw code that existed only for it."
---

# ADR-0022: graphy is decommissioned

**Status:** Accepted
**Date:** 2026-10-09
**Source:** Commit ffa4efa, "decommission graphy and the okfw code that served it".

## Context
The repository incubated `graphy`, a CGo binary that built a code knowledge graph and exported it into okfw bundles. It needed a C toolchain, tree-sitter, bbolt, gonum and hcl.

## Decision
- Remove `cli/cmd/graphy`, `cli/internal/graphy`, its make targets, its Containerfile stages and its e2e gate.
- Remove the okfw surface that existed only for graphy: the `graphy:` frontmatter block, `kind: code`, `okfw search --node-id`, the MCP `node_id` field and the after-edit hook of agentskill.
- Do not bring them back.

## Consequences
- okfw builds without a C toolchain. `CGO_ENABLED=0` is the rule.
- Concepts from `graphy export okfw` fail strict validation. Delete the exported directory rather than migrate it.

## Alternatives considered
- **Keep graphy in this repository.** The commit records no other reason.
