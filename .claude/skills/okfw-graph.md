---
description: Graph traversal, backlinks, and relationship queries over an OKF Wiki's links
globs:
  - "cli/internal/graph/**"
  - "cli/internal/graph/**"
---

# Skill: Graph Database & Relationship Queries

The `okfw` CLI answers relationship queries by building the link graph from the concepts on disk. There is no database and no sync step: the graph always matches the files. Building it costs roughly 300ms on a 900-concept bundle.

## Graph Sync

## Graph Visualization

```bash
# Mermaid flowchart of relationships (output to stdout)
okfw graph

# Center on a specific concept
okfw graph --start 20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee

# Limit traversal depth and node count
okfw graph --depth 3 --limit 20

# Force sync before querying
okfw graph --sync
```

Summary line (nodes, edges) goes to stderr so piped output stays clean.

## Backlinks

```bash
# Find all concepts that link TO a given concept (O(1) with graph DB, O(N) without)
okfw backlinks 20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee
okfw backlinks ./path/to/concept.md
okfw backlinks <id> --json
```

## Graph Queries

All subcommands support `--json` for machine-readable output.

```bash
# Backlinks via graph-query (same as the backlinks command)
okfw graph-query backlinks <id>

# N-depth cluster: find all connected concepts within N hops
okfw graph-query cluster <id> --depth 2 --limit 20

# Find all concepts with a specific tag
okfw graph-query tag golang

# Find dangling edges (links to non-existent concepts)
okfw graph-query dangling
```

## JSON Output Format

All `--json` graph-query output uses this structure:

**Backlinks / tag:** Array of node records:
```json
[
  {
    "id": "20260213...",
    "title": "Concept Title",
    "file_path": "/abs/path/to/concept.md",
    "project": "myproject",
    "tags": ["golang"],
    "parent": "",
    "links": ["20260214..."],
    "mod_time": 1707820000
  }
]
```

**Cluster:** Object with nodes and edges:
```json
{
  "nodes": [/* NodeRecord array */],
  "edges": [
    {
      "from": "20260213...",
      "predicate": "links_to",  // links_to | child_of | parent_of | tagged
      "to": "20260214..."
    }
  ]
}
```

## Relationship Types

- **links_to**: Body `[[id]]` or `[[id|title]]` link between concepts
- **parent_of / child_of**: Hierarchical via `parent` frontmatter field
- **tagged**: Concept-to-tag association (from `tags` frontmatter field)

## Export

```bash
# Export graph subset as portable markdown to ephemeral/
# Converts [[id|title]] links to [title](id.md) relative links
okfw export --start <id> --depth 2 --limit 10
```
