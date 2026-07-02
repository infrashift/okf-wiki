---
title: Quick Start
description: Get up and running with OKF Wiki in minutes.
---

```bash
# Initialize a wiki in your project (once per project)
cd your-project
okfw init                       # Creates .okfw/ at the project root

# Every command discovers the wiki automatically from any subdirectory
okfw connect                    # Verify: prints the wiki path and a summary

# Create a note (project context optional)
okfw create "My first idea"

# Create a note with an explicit project
okfw create "Project-specific idea" --project my-project

# Create with a one-sentence OKF description
okfw create "OAuth notes" -d "How our OAuth flow works"

# Place a note in a specific subdirectory of the wiki
okfw create "Spec draft" --path projects/okf

# Set project on an existing note
okfw set-project path/to/note.md my-project

# Add tags to a concept
okfw add-tags path/to/note.md golang          # Add a single tag
okfw add-tags path/to/note.md golang api      # Add multiple tags

# Validate frontmatter against CUE schema
okfw validate path/to/note.md
okfw validate --okf                       # Validate whole-wiki OKF conformance

# Index notes for searching
okfw index path/to/notes/       # Index a directory
okfw index path/to/note.md      # Index a single file

# OKF wiki maintenance
okfw root                                 # Print the discovered .okfw path
okfw index-md                             # Generate OKF index.md directory listings
okfw log                                  # Generate OKF log.md from git history

# Search notes
okfw search "authentication"              # Full-text search
okfw search --project my-project          # Filter by project
okfw search --tag golang --tag api        # Filter by tags
okfw search --kind todo                   # Filter by structured kind
okfw search --type note                   # Filter by the free-form OKF type
okfw search "auth" --project my-project   # Combined search
okfw search --json                        # JSON output for tooling

# Generate graph visualization
okfw graph path/to/notes/                 # Show ASCII tree graph
okfw graph . --limit 20                   # Custom node limit
okfw graph . --start <note-id>            # Start from a specific note

# Find backlinks (notes that link to a concept)
okfw backlinks 202602131045               # By ID
okfw backlinks path/to/note.md            # By file path
okfw backlinks 202602131045 --json        # JSON output

# Create notes from templates
okfw templates                            # List available templates
okfw create "Sprint Planning" --template meeting
okfw create "Login Feature" --template user-story
okfw create "OAuth2 Integration" --template feature
okfw create "Login broken on mobile" --template issue

# Daily notes
okfw daily                                # Today's daily note
okfw daily --yesterday                    # Yesterday (for morning review)
okfw daily --date 2026-02-10              # Specific date
okfw daily --list                         # List recent daily notes

# Todo management
okfw todo "Fix login bug"                 # Create a todo
okfw todo "Update docs" --due 2026-02-20  # With due date
okfw todo "Critical fix" --priority high  # With priority
okfw todos                                # List open todos
okfw todos --project my-project           # Filter by project
okfw todos --overdue                      # Show overdue todos
okfw todos --today                        # Due today
okfw todos --this-week                    # Due this week
okfw todos --closed                       # Show closed todos
okfw set-status 202602131045 closed       # Mark todo as closed
okfw set-status 202602131045 in_progress  # Set to in progress
okfw set-status 202602131045 open         # Reopen a todo
okfw todo-list                            # Generate todo list markdown
okfw todo-list --project my-project       # Project-specific list

# Git workflow (dated branches)
```
