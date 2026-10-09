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

# OKF wiki maintenance
okfw root                                 # Print the discovered .okfw path
okfw index-md                             # Regenerate OKF index.md directory listings
okfw log                                  # Generate OKF log.md from git history

# Search notes
okfw search "authentication"              # Full-text search
okfw search --project my-project          # Filter by project
okfw search --tag golang --tag api        # Filter by tags
okfw search --kind issue                  # Filter by structured kind
okfw search --type note                   # Filter by the free-form OKF type
okfw search "auth" --project my-project   # Combined search
okfw search --json                        # JSON output for tooling

# Generate graph visualization (search and the graph read the files directly;
# there is no index to build)
okfw graph path/to/notes/                 # Mermaid diagram
okfw graph --ascii                        # Plain-text tree instead
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

# OKF metadata (okfw has no task tracking)
okfw set-lifecycle 019f0494 deprecated    # draft | stable | deprecated
okfw set-stale-after 019f0494 2026-12-31T00:00:00Z  # mark a shelf life
okfw search --status draft                # find unreviewed concepts
okfw search --stale                       # find concepts past stale_after
okfw verify 019f0494                      # record that you reviewed it
okfw add-source 019f0494 --resource https://example.com/spec
okfw validate --all                       # strict schema check of the wiki
okfw migrate                              # clean up a pre-0.2 wiki
```
