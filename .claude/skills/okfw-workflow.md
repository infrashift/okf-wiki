---
description: Daily git workflow and session management for the OKF Wiki bundle using okfw hello/goodbye commands
globs:
  - "cli/cmd/okfw/main.go"
---

# Skill: OKF Wiki Git Workflow

The `okfw` CLI provides a branch-per-day git workflow for the OKF Wiki bundle directory.

## Start a Session

```bash
okfw hello
```

This will:
1. Ensure clean working tree (no uncommitted changes)
2. Switch to `main` and pull latest
3. Create a new branch named with today's date (e.g., `20260213`)

## End a Session

```bash
okfw goodbye
```

This will:
1. Stage all changes in the current branch
2. Commit with a message including today's date
3. Switch to `main`
4. Merge the dated branch into `main`
5. Delete the dated branch

## Typical Daily Workflow

```bash
okfw hello                              # Start session, create dated branch
okfw daily                              # Open/create today's daily note
okfw create "Meeting Notes" -T meeting  # Create concepts as needed
okfw index                              # Rebuild search index
okfw graph-sync                         # Update graph database
okfw goodbye                            # Commit, merge to main, cleanup
```

## Important

- Always start with `okfw hello` before creating/editing concepts
- Always end with `okfw goodbye` to merge changes back to main
- If `okfw hello` fails due to uncommitted changes, commit or stash first
- The branch name is `YYYYMMDD` format — one branch per day
