# PRD: okfw - OKF Wiki for Neovim

## 1. Overview
A high-performance, local-first knowledge base — OKF Wiki — consisting of a Go CLI (`okfw`) and a Neovim integration, managing per-project Open Knowledge Format (OKF) wikis. A wiki is a `.okfw/` directory at a project's root, created by `okfw init` and committed with the project. The system prioritizes data integrity, structured search, and interconnected knowledge.

## 2. Technical Stack
- **CLI**: Go 1.22+ (using `cobra`)
- **Indexing**: Bleve (Full-text + Structured)
- **Validation**: CUE (Schema-first metadata)
- **Integration**: Neovim (Lua), snacks.nvim, Plenary.job
- **Version Control**: Git (Used as the storage and backup engine)

## 3. Core Requirements
- **Per-Project Storage**: Each project owns a `.okfw/` wiki at its root, created once with `okfw init`. Concept markdown is committed with the project; generated artifacts (`.index/` search index, `.graph.db` graph database, `.todos/` todo lists, `ephemeral/` exports) are covered by a generated `.gitignore`.
- **Zero Configuration**: No config file or environment variables. Commands discover the wiki by searching upward from the current working directory (like git finds `.git`) and fail with `okfw init` advice when none exists; `okfw connect` verifies the wiki and `okfw root` prints its path.
- **Automated Metadata**: Extract Git project context during note creation; `project` is always optional and reassignable via `set-project`.
- **Verification**: Use CUE to validate YAML frontmatter before indexing.
- **Search**: Structured search via snacks.nvim picker (e.g., `project:my-repo title:config`).
- **Graphing**: Generate local relationship graphs (Forward/Backlinks) as ASCII tree output.