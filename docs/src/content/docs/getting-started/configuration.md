---
title: Configuration
description: Configure the Zettelkasten CLI with a CUE config file.
---

Create a config file at `~/.config/zk/config.cue`:

```cue
root_path:  "~/zk_vault"
index_path: ".zk_index"
graph_path: ".zk_graph.db"
editor:     "nvim"
folders: {
    untethered: "untethered"
    tethered:   "tethered"
    tmp:        "tmp"
}
```

## Configuration Options

| Option | Default | Description |
|--------|---------|-------------|
| `root_path` | `~/zk_vault` | Root directory for all notes |
| `index_path` | `.zk_index` | Location of Bleve search index (relative to root) |
| `graph_path` | `.zk_graph.db` | Location of persistent graph database (relative to root) |
| `todos_path` | `.zk_todos` | Directory for generated todo lists (relative to root) |
| `editor` | `nvim` | Editor for opening notes |
| `folders.untethered` | `untethered` | Subdirectory for untethered notes |
| `folders.tethered` | `tethered` | Subdirectory for tethered notes |
| `folders.tmp` | `tmp` | Subdirectory for temporary notes |
