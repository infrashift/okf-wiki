---
title: CLI Flags
description: Every okfw command and its flags, generated from the command tree.
---

<!-- Generated from okfw's command tree by `make docs-flags`. Do not edit: change the flag's help text instead. -->

This page lists each okfw command and its flags. okfw makes it from its command tree, the same source as `okfw <command> --help` and the man pages. So the page cannot list a flag that okfw does not have. For the purpose of each command, with examples, see [CLI Commands](/okf-wiki/reference/cli-commands/).

Each command also takes `--help` (`-h`). Most commands need a wiki. They look for `.okfw/` in the current directory and then in each parent directory. When they find none, they stop with an error that tells you to run `okfw init`.

## Global flags

| Flag | Value | Default | Description |
|---|---|---|---|
| `--version`, `-v` |  |  | Print the version and exit |

## okfw add-source

Add a source to a concept.

```
okfw add-source <id_or_file>
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--author` | `string` |  | Actor who produced the source: `<scheme>:<id>` or `<producer>/<version>` |
| `--id` | `string` |  | Label the body's [^label] footnotes cite this source by |
| `--last-modified` | `string` |  | When the source itself last changed: an RFC 3339 date-time with offset, or a date (midnight UTC) |
| `--resource` | `string` |  | Required. A URL, a bundle path such as /dir/file.md, or a scope descriptor |
| `--title` | `string` |  | Human-readable label for the source |
| `--usage-count` | `int` |  | How often the resource was exercised, within `--usage-window` |
| `--usage-window` | `string` |  | The period `--usage-count` covers, as FROM..TO (two timestamps) |

## okfw add-tags

Add tags to a concept.

```
okfw add-tags [id_or_file] [tags...]
```

No flags.

## okfw backlinks

Find all notes that link to a concept.

```
okfw backlinks [id_or_file]
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--json` |  |  | Print the hits as a JSON array |

## okfw connect

Verify and report the project's OKF wiki.

```
okfw connect
```

No flags.

## okfw create

Create a new concept.

```
okfw create [title]
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--body` | `string` |  | Markdown body, written after the template's body if there is one |
| `--description`, `-d` | `string` |  | One-sentence summary, shown in search hits and index.md listings |
| `--json` |  |  | Print {"id","path"} as JSON instead of prose |
| `--kind` | `string` |  | Kind: note or issue (default: the template's, else note) |
| `--path` | `string` |  | Place the concept in a bundle subdirectory, created if missing (e.g. research/protocols) |
| `--tags` | `strings` |  | Tags to add, comma-separated or repeated |
| `--template`, `-T` | `string` |  | Start from a template; okfw templates lists them |
| `--type` | `string` |  | Free-form OKF type, such as Playbook or Metric (default: the kind) |
| `--project`, `-p` | `string` |  | **Deprecated:** use `--path` `<dir>` to place the concept in a directory. Was: Optional project metadata (also used as the note's folder) |

## okfw daily

Create or open a daily note.

```
okfw daily
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--date` | `string` |  | Date of the note, YYYY-MM-DD (default: today) |
| `--json` |  |  | With `--list`: print JSON |
| `--list` |  |  | List recent daily notes instead of opening one |
| `--month` |  |  | With `--list`: only this month's notes |
| `--week` |  |  | With `--list`: only this week's notes |
| `--yesterday` |  |  | Yesterday's note instead of today's |

## okfw delete

Delete a concept and its index.md entry.

```
okfw delete <id_or_file>
```

Aliases: `rm`.

| Flag | Value | Default | Description |
|---|---|---|---|
| `--force`, `-f` |  |  | Delete without asking for confirmation |
| `--json` |  |  | Output the result as JSON |

## okfw export

Export graph subset as portable markdown to .ephemeral/.

```
okfw export [path]
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--depth` | `int` |  | Maximum BFS depth (hops) from start node (0 = unlimited) |
| `--limit`, `-l` | `int` | `10` | Maximum number of concepts to export |
| `--start` | `string` |  | Concept to start from, by id or path (default: the most connected) |

## okfw graph

Render concept relationships as a Mermaid diagram (or an ASCII tree).

```
okfw graph [path]
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--ascii` |  |  | Render a plain-text tree instead of Mermaid |
| `--depth` | `int` |  | Maximum BFS depth (hops) from start node (0 = unlimited) |
| `--limit`, `-l` | `int` | `10` | Maximum number of nodes to display |
| `--start` | `string` |  | Concept to start from, by id or path (default: the most connected) |

## okfw graph-query backlinks

Find all notes that link to a node.

```
okfw graph-query backlinks <id>
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--json` |  |  | Output as JSON. Applies to every `okfw graph-query` subcommand |

## okfw graph-query cluster

Find connected nodes within N hops.

```
okfw graph-query cluster <id>
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--depth` | `int` |  | Maximum hops from the concept (0 = unlimited) |
| `--json` |  |  | Output as JSON. Applies to every `okfw graph-query` subcommand |
| `--limit`, `-l` | `int` | `10` | Maximum number of concepts |

## okfw graph-query dangling

Find links to notes that do not exist.

```
okfw graph-query dangling
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--json` |  |  | Output as JSON. Applies to every `okfw graph-query` subcommand |

## okfw graph-query tag

Find all notes with a given tag.

```
okfw graph-query tag <tag>
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--json` |  |  | Output as JSON. Applies to every `okfw graph-query` subcommand |

## okfw index-md

Generate OKF index.md directory listings.

```
okfw index-md [path]
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--no-adopt` |  |  | Refresh only the listings okfw already maintains; never take over an unmarked index.md |

## okfw init

Initialize an OKF wiki (.okfw/) for the current project.

```
okfw init
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--description` | `string` |  | One sentence: what the bundle covers and when to use it (required by validate `--all`) |
| `--license` | `string` |  | License of the bundle content: an SPDX expression or a path in the bundle |
| `--title` | `string` |  | Bundle title (default: the project directory's name) |

## okfw install

Install the okfw skill for an AI agent.

```
okfw install
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--force` |  |  | Overwrite an existing skill file |
| `--platform` | `string` | `claude` | Agent platform: claude, gemini or codex |

## okfw log

Generate OKF log.md from git history.

```
okfw log [path]
```

No flags.

## okfw man

Install okfw's man pages.

```
okfw man
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--dir` | `string` |  | Directory to write the pages to (default: $XDG_DATA_HOME/man/man1, else ~/.local/share/man/man1) |

## okfw mcp

Serve okfw over MCP (stdio) for AI agents.

```
okfw mcp
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--read-only` |  |  | Serve only the search, read and graph tools, with shorter instructions; create and update are refused |

## okfw migrate

Bring older concepts up to the current frontmatter shape.

```
okfw migrate [path]
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--dry-run` |  |  | Report what would change without writing |

## okfw move

Move or rename a concept, and repair the links at both ends.

```
okfw move <id_or_file> <destination>
```

Aliases: `rename`, `mv`.

| Flag | Value | Default | Description |
|---|---|---|---|
| `--json` |  |  | Output the result as JSON |

## okfw remove-tags

Remove tags from a concept.

```
okfw remove-tags [id_or_file] [tags...]
```

No flags.

## okfw root

Print the project's OKF wiki root path.

```
okfw root
```

No flags.

## okfw search

Search concepts.

```
okfw search [query]
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--current` |  |  | Current knowledge only: drop deprecated and unknown-status concepts, and replace a deprecated hit with its superseded_by |
| `--json` |  |  | Print the hits as a JSON array |
| `--kind` | `string` |  | Filter by kind: note, daily-note or issue |
| `--limit`, `-l` | `int` | `20` | Maximum number of results |
| `--offset` | `int` |  | Skip this many results, to page past `--limit` |
| `--path` | `string` |  | Search only this bundle subdirectory, recursively (e.g. issues) |
| `--stale` |  |  | Only concepts whose stale_after has passed, or does not parse |
| `--status` | `string` |  | Filter by lifecycle status: draft, stable or deprecated (another value matches as written) |
| `--tag`, `-T` | `strings` |  | Filter by tag; repeat it to require several |
| `--type`, `-t` | `string` |  | Filter by free-form OKF type, such as Playbook |
| `--project`, `-p` | `string` |  | **Deprecated:** use `--path` `<dir>` to search one directory. Was: Filter by project |

## okfw set-bundle-meta

Set the bundle's title, description or license.

```
okfw set-bundle-meta
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--description` | `string` |  | One sentence: what the bundle covers and when to use it (required by validate `--all`) |
| `--license` | `string` |  | SPDX expression, or a path to a license file in the bundle; an empty value removes it |
| `--title` | `string` |  | Bundle title; an empty value removes it |

## okfw set-generated

Record who generated a concept.

```
okfw set-generated <id_or_file>
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--at` | `string` |  | When the content last meaningfully changed: an RFC 3339 date-time or a date (default: now) |
| `--by` | `string` |  | Actor: `human:<id>`, `team:<id>`, `process:<id>` or `<producer>/<version>` (default: `human:<login>`) |

## okfw set-lifecycle

Set a concept's OKF lifecycle status.

```
okfw set-lifecycle <id_or_file> <draft|stable|deprecated>
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--superseded-by` | `string` |  | With deprecated: the concept that replaces this one, by id or path |

## okfw set-resource

Set the URI of the asset a concept describes.

```
okfw set-resource <id_or_file> <uri>
```

No flags.

## okfw set-stale-after

Set the instant after which a concept counts as stale.

```
okfw set-stale-after <id_or_file> [DATETIME]
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--clear` |  |  | Remove stale_after instead of setting it |

## okfw site build

Build the site.

```
okfw site build
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--bun` | `string` |  | Path to the bun binary (default: $OKFW_BUN, then bun on PATH). Applies to every `okfw site` subcommand |
| `--out` | `string` |  | Also copy the built site to this directory |

## okfw site down

Stop the site server and free its port.

```
okfw site down
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--bun` | `string` |  | Path to the bun binary (default: $OKFW_BUN, then bun on PATH). Applies to every `okfw site` subcommand |

## okfw site status

Report bun, the build directory and the site server.

```
okfw site status
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--bun` | `string` |  | Path to the bun binary (default: $OKFW_BUN, then bun on PATH). Applies to every `okfw site` subcommand |

## okfw site up

Build the site and serve it in the background.

```
okfw site up
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--bun` | `string` |  | Path to the bun binary (default: $OKFW_BUN, then bun on PATH). Applies to every `okfw site` subcommand |
| `--no-build` |  |  | Serve the last build without rebuilding |
| `--port` | `int` | `4321` | Port to serve on |

## okfw templates

List available note templates.

```
okfw templates
```

No flags.

## okfw validate

Check a concept's frontmatter, or a whole bundle.

```
okfw validate [id_or_file_or_path]
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--all` |  |  | Validate every concept against the strict okfw schema, and the bundle metadata |
| `--okf` |  |  | Check only OKF v0.3 conformance (section 11.1): permissive, and works on any bundle directory, with or without .okfw |

## okfw verify

Record that someone verified a concept.

```
okfw verify <id_or_file>
```

| Flag | Value | Default | Description |
|---|---|---|---|
| `--at` | `string` |  | When it was verified: an RFC 3339 date-time or a date (default: now) |
| `--by` | `string` |  | Actor: `human:<id>`, `team:<id>`, `process:<id>` or `<producer>/<version>` (default: `human:<login>`) |

## Deprecated commands

These commands still run, and print a warning. `--help` and the man pages do not show them.

- `okfw set-project`: organize concepts by directory instead: move one with okfw move, and create one in place with okfw create `--path`
