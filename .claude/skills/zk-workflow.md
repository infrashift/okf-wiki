---
description: Daily git workflow and session management for the zettelkasten using zk hello/goodbye commands
globs:
  - "cli/cmd/zk/main.go"
---

# Skill: Zettelkasten Git Workflow

The `zk` CLI provides a branch-per-day git workflow for the zettelkasten directory.

## Start a Session

```bash
zk hello
```

This will:
1. Ensure clean working tree (no uncommitted changes)
2. Switch to `main` and pull latest
3. Create a new branch named with today's date (e.g., `20260213`)

## End a Session

```bash
zk goodbye
```

This will:
1. Stage all changes in the current branch
2. Commit with a message including today's date
3. Switch to `main`
4. Merge the dated branch into `main`
5. Delete the dated branch

## Typical Daily Workflow

```bash
zk hello                              # Start session, create dated branch
zk daily                              # Open/create today's daily note
zk create "Meeting Notes" -T meeting  # Create notes as needed
zk index                              # Rebuild search index
zk graph-sync                         # Update graph database
zk goodbye                            # Commit, merge to main, cleanup
```

## Important

- Always start with `zk hello` before creating/editing notes
- Always end with `zk goodbye` to merge changes back to main
- If `zk hello` fails due to uncommitted changes, commit or stash first
- The branch name is `YYYYMMDD` format — one branch per day
