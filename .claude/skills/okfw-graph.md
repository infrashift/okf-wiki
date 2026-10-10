---
description: Graph traversal, backlinks, and relationship queries over an OKF Wiki's links
globs:
  - "cli/internal/graph/**"
  - "cli/internal/bundle/loadgraph.go"
---

# Skill: Graph & Relationship Queries

The `okfw` CLI answers relationship queries by building the link graph from the concepts on disk (`bundle.LoadGraph`). There is no database and no sync step: the graph always matches the files. Building it costs roughly 300ms on a 900-concept bundle.

Concept references (`--start`, `<id>` arguments) accept the full frontmatter id, a unique id prefix of at least 8 hex digits, an OKF concept ID (bundle-relative path minus `.md`), or a file path.

## Graph Visualization

```bash
# Mermaid flowchart of relationships (output to stdout)
okfw graph

# Plain-text tree instead of Mermaid
okfw graph --ascii

# Center on a specific concept
okfw graph --start 019f0494

# Limit traversal depth and node count
okfw graph --depth 3 --limit 20
```

Summary line (nodes, edges) goes to stderr so piped output stays clean.

## Backlinks

```bash
# Find all concepts that link TO a given concept (a scan of the bundle)
okfw backlinks 019f0494
okfw backlinks ./path/to/concept.md
okfw backlinks <id> --json      # [{"id","title","project","file_path"}]
```

## Graph Queries

All subcommands take `--json` for machine-readable output.

```bash
# Backlinks via graph-query (same answer as the backlinks command)
okfw graph-query backlinks <id>

# N-depth cluster: find all connected concepts within N hops
okfw graph-query cluster <id> --depth 2 --limit 20

# Find all concepts with a specific tag
okfw graph-query tag golang

# Find dangling edges: parent references and links naming an id no concept declares
okfw graph-query dangling
```

## JSON Output Format

`graph-query --json` serializes the graph's Go structs directly, so keys are the
Go field names (PascalCase), not snake_case. An empty result is `null`, not `[]`.

**backlinks / tag:** array of `graph.Node`:
```json
[
  {
    "ID": "019f0494-00a8-7206-b788-c5bbfa2430a8",
    "Title": "Concept Title",
    "FilePath": "/abs/path/to/.okfw/concept--019f049400a8.md",
    "Project": "myproject",
    "Parent": "",
    "Children": null,
    "Links": ["019f04a1-..."],
    "Tags": ["golang"]
  }
]
```

**cluster:** object with nodes and edges:
```json
{
  "nodes": [/* graph.Node array, as above */],
  "edges": [
    { "From": "019f04a1-...", "To": "019f0494-...", "Label": "link" }
  ]
}
```

**dangling:** array of `graph.Edge` (`{"From","To","Label"}`).

`okfw backlinks --json` is different: it emits snake_case
`{"id","title","project","file_path"}` records.

## Relationship Types

Edge `Label` values:

- **link**: a vanilla markdown link in the body (`[text](/dir/file.md)` or `[text](./file.md)`) that resolves to another concept
- **parent** / **child**: hierarchy via the `parent` frontmatter field, which holds the parent's full id

Tags are node attributes (`Tags`), queried with `graph-query tag`, not edges.

## Export

```bash
# Export graph subset as portable markdown to ephemeral/
# Each concept is written as <id>.md, with in-bundle links rewritten to <id>.md
okfw export --start <id> --depth 2 --limit 10
```
