# Development Skills & Rules

## Go Architecture
- Use the **Internal Pattern**: Logic lives in `cli/internal/`, binaries in `cli/cmd/`.
- **CUE Validation**: All YAML frontmatter must be unified with the `#Concept` schema in `cli/internal/config`.
- **No derived stores**: Search (`cli/internal/search`) and the link graph (`bundle.LoadGraph`) are computed from the files on every call. Do not add a cache or index that can drift from the bundle.
- **Shared write path**: Concept creation, frontmatter edits, moves, deletes and `index.md` listings go through `cli/internal/bundle`.

## Neovim Integration
- **Async Only**: Never block the UI. Use `plenary.job`.
- **snacks.nvim**: Feed the CLI's `--json` output to the snacks.nvim picker.
- **Ephemeral Files**: Exported files are written to the git-ignored `ephemeral/` directory at the bundle root.
