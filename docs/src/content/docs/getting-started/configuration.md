---
title: Configuration
description: Configure OKF Wiki with a CUE config file.
---

Create a config file at `~/.config/okfw/config.cue`:

```cue
root_path:  "~/okfw"
index_path: ".okfw_index"
graph_path: ".okfw_graph.db"
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
| `root_path` | `~/okfw` | Root directory for all notes |
| `index_path` | `.okfw_index` | Location of Bleve search index (relative to root) |
| `graph_path` | `.okfw_graph.db` | Location of persistent graph database (relative to root) |
| `todos_path` | `.okfw_todos` | Directory for generated todo lists (relative to root) |
| `editor` | `nvim` | Editor for opening notes |
| `folders.untethered` | `untethered` | Legacy field; does **not** control placement. Concepts are placed by `--path` or project, and `category` is metadata only. |
| `folders.tethered` | `tethered` | Legacy field; does **not** control placement (see above). |
| `folders.tmp` | `tmp` | Subdirectory for temporary notes |
