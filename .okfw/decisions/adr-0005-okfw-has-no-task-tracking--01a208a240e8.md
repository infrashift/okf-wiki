---
x-okfw-id: "01a208a2-40e8-705b-89c8-359a638c5779"
title: "ADR-0005: okfw has no task tracking"
type: "ADR"
tags:
  - "adr"
  - "schema"
x-okfw-created: "2026-11-23T07:39:40-06:00"
x-okfw-timestamp: "2026-11-23T07:39:40-06:00"
description: "okfw removed its todo kind and fields rather than namespace them, because OKF uses status for the lifecycle of knowledge."
---

# ADR-0005: okfw has no task tracking

**Status:** Accepted
**Date:** 2026-07-27
**Source:** Commit 7134120, "target OKF v0.2, drop task tracking, add delete/move".

## Context
okfw had a `todo` kind with `status`, `due`, `completed` and `priority` fields. OKF v0.2 defined `status` as the lifecycle field of a concept (`draft`, `stable`, `deprecated`). The two meanings collided.

## Decision
- okfw removes task tracking. okfw is a knowledge format, not a task tracker.
- The four todo fields, `kind: todo`, the todo commands, the todo search flags, the todo template and `.todos/` are gone.
- `okfw migrate` strips the fields from older concepts. `status: closed` becomes `deprecated`. An open or in-progress concept loses `status`, because an absent `status` means `stable`.
- Do not add todo fields, an `okfw:` block or an `okfw-` prefix again.

## Consequences
- `status` is the OKF lifecycle field only.
- Issues keep their state in tags, for example `open` and `closed` ([Wiki conventions](/wiki-conventions--01a208a240cb.md)).

## Alternatives considered
- **Namespace the todo fields.** Rejected in the commit. okfw removed the feature rather than namespace it.
