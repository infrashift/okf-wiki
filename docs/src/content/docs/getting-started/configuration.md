---
title: Configuration
description: OKF Wiki is zero-configuration — one command per project, no config file.
---

OKF Wiki is **zero-configuration**. There is no config file, no environment
variable, and nothing to tune. A wiki is a `.okfw/` directory at your project's
root, and every command finds it automatically.

## Initializing a wiki

Run `okfw init` once per project:

```bash
cd your-project
okfw init
```

This creates `.okfw/` at the git repository root (or the current directory if
you're not in a git repo), writes a `.okfw/.gitignore` covering generated
artifacts, and generates the OKF root `index.md` (with the `okf_version`
declaration). Running `okfw init` again is a no-op that prints
`OKF wiki already initialized at <path>`.

## Automatic discovery

Every command discovers the wiki by searching upward from the current working
directory — the same way git finds `.git`. That means commands work from any
subdirectory of your project. When no wiki is found, commands exit non-zero
with:

```
no OKF wiki found (searched upward from <dir>); run 'okfw init' at your project root to create one
```

Two commands help you verify and script against the discovered wiki:

- `okfw connect` — verifies the wiki and prints `Connected: <path>` plus a
  summary line (`concepts: N | search index: yes/no | graph db: yes/no`).
- `okfw root` — prints the discovered `.okfw` path (script-friendly; used by
  the Neovim plugin).

## The `.okfw/` layout

The layout inside `.okfw/` is fixed:

| Path | Description |
|------|-------------|
| `*.md` (top level) | Concept markdown files (subdirectories via `--path` or `--project`) |
| `index.md` (every directory) | OKF directory listing, auto-maintained on every note change |
| `daily/YYYY/MM/` | Daily notes, grouped by year and month |
| `.index/` | Bleve full-text search index (generated) |
| `.graph.db` | Persistent graph database (generated) |
| `.todos/` | Generated todo lists |
| `ephemeral/` | Export scratch directory |
| `.gitignore` | Ignores `.index/`, `.graph.db`, `.todos/`, `ephemeral/` |

Concept markdown is meant to be **committed with your project**; the generated
artifacts are gitignored.
