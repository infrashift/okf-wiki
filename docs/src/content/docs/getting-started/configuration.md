---
title: Configuration
description: OKF Wiki is zero-configuration — one command per project, no config file.
---

OKF Wiki is **zero-configuration**. There is no config file, no environment
variable, and nothing to tune. A wiki is a `.okfw/` directory at the root of the
project, and every command finds it automatically.

## Initializing a wiki

Run `okfw init` once per project:

```bash
cd your-project
okfw init
```

This command creates `.okfw/` at the root of the git repository. Outside a git
repository, it creates `.okfw/` in the current directory. It also writes a
`.okfw/.gitignore` for the `.ephemeral/` export directory, and the OKF root
`index.md` with the `okf_version` declaration. When you run `okfw init` again,
it changes nothing and prints `OKF wiki already initialized at <path>`.

## Automatic discovery

Each command looks for the wiki in the current working directory, and then in
each parent directory, as git does to find `.git`. So the commands work from
any subdirectory of the project. When a command finds no wiki, it exits
non-zero with this message:

```
no OKF wiki found (searched upward from <dir>); run 'okfw init' at your project root to create one
```

Two commands help you verify and script against the discovered wiki:

- `okfw connect` — verifies the wiki and prints `Connected: <path>` plus a
  `concepts: N` line.
- `okfw root` — prints the discovered `.okfw` path (script-friendly; used by
  the Neovim plugin).

## The `.okfw/` layout

okfw always uses this layout inside `.okfw/`:

| Path | Description |
|------|-------------|
| `*.md` (top level) | Concept markdown files (subdirectories via `--path`) |
| `index.md` (every directory) | OKF directory listing, auto-maintained on every note change |
| `daily/YYYY/MM/` | Daily notes, grouped by year and month |
| `.ephemeral/` | Export scratch directory (the only generated directory) |
| `.gitignore` | Ignores `.ephemeral/` |

Commit the concept markdown **with the project**. The `.gitignore` excludes
only `.ephemeral/`. There is no search index or graph database to build or
keep in sync. okfw calculates search and the link graph from the files on each
call.
