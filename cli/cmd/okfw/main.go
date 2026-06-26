package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"okfw/cli/internal/config"
	"okfw/cli/internal/gitctx"
	"okfw/cli/internal/graph"
	"okfw/cli/internal/graphdb"
	"okfw/cli/internal/index"
	"okfw/cli/internal/templates"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func main() {
	var noteCategory string
	var projectFlag string
	var categoryFlag string
	var tagsFlag []string
	var limitFlag int
	var jsonFlag bool
	var graphLimitFlag int
	var outputFlag string
	var templateFlag string
	var descriptionFlag string
	var pathFlag string
	var statusFlag string
	var priorityFlag string

	var rootCmd = &cobra.Command{Use: "okfw"}

	var createCmd = &cobra.Command{
		Use:   "create [title]",
		Short: "Create a new concept",
		Long: `Create a new concept with optional template.

Examples:
  okfw create "My Note"                                     # Basic untethered note
  okfw create "Meeting Notes" --template meeting             # From template
  okfw create "Feature Spec" -T feature -p myproj            # Template with project
  okfw create "Refined Note" --category tethered -p myproj   # Tethered note`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Validate category
			if noteCategory != "untethered" && noteCategory != "tethered" {
				return fmt.Errorf("invalid category %q: must be \"untethered\" or \"tethered\"", noteCategory)
			}

			cwd, _ := os.Getwd()
			project := gitctx.GetGitContext(cwd)
			if projectFlag != "" {
				project = projectFlag
			}
			title := args[0]

			// Generate ID
			id := uuid.Must(uuid.NewV7()).String()

			var content string
			var category string

			if templateFlag != "" {
				// Use template
				tmpl, err := templates.Get(templateFlag)
				if err != nil {
					return fmt.Errorf("template error: %w", err)
				}

				// Tethered templates require project
				if tmpl.Category == "tethered" && project == "" {
					return fmt.Errorf("template '%s' creates tethered notes which require a project; use --project flag", templateFlag)
				}

				content, err = tmpl.GenerateNote(id, title, project, tagsFlag)
				if err != nil {
					return fmt.Errorf("failed to generate note from template: %w", err)
				}
				category = tmpl.Category

				fmt.Printf("Creating %s note from template '%s': %s", category, templateFlag, title)
			} else {
				// Basic note without template
				category = noteCategory

				// Tethered notes require project
				if category == "tethered" && project == "" {
					return fmt.Errorf("tethered notes require a project; use --project flag or run from a git repository")
				}

				content = generateBasicNote(id, title, project, category, tagsFlag)

				if category == "untethered" && project == "" {
					fmt.Printf("Creating %s note: %s (no project context)", category, title)
				} else {
					fmt.Printf("Creating %s note: %s (Project: %s)", category, title, project)
				}
			}

			// Determine output directory: explicit --path wins, else the project
			// folder, else the bundle root. Category is metadata, not a path.
			outputDir, err := conceptDir(cfg, pathFlag, project)
			if err != nil {
				return err
			}

			// Create directory if it doesn't exist
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}

			// Apply optional OKF description.
			if descriptionFlag != "" {
				updated, derr := updateFrontmatter([]byte(content), map[string]string{"description": descriptionFlag})
				if derr != nil {
					return fmt.Errorf("failed to set description: %w", derr)
				}
				content = string(updated)
			}

			// Write file
			filePath := filepath.Join(outputDir, noteFilename(outputDir, title, id))
			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}

			fmt.Printf("\nCreated: %s\n", filePath)

			// Auto-index the new note
			if err := autoIndex(cfg, filePath); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to index note: %v\n", err)
			}

			return nil
		},
	}

	var migrateDryRun bool
	var migrateCmd = &cobra.Command{
		Use:   "migrate-okf [path]",
		Short: "Migrate a legacy bundle to OKF conventions",
		Long: `Convert a legacy bundle to OKF conventions:
  - relocate ~/zk_vault to the configured bundle root if present
  - rename concept files to hybrid slug--shortid.md names
  - rewrite legacy [[id]] / [[id|title]] links to vanilla markdown links
  - rebuild the graph database and search index

Idempotent and safe to re-run. Use --dry-run to preview changes.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			root := cfg.RootPath
			if len(args) > 0 {
				root = args[0]
			}
			return runMigrate(cfg, root, migrateDryRun)
		},
	}

	var rootPathCmd = &cobra.Command{
		Use:   "root",
		Short: "Print the configured bundle root path",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			fmt.Println(cfg.RootPath)
			return nil
		},
	}

	var indexMdCmd = &cobra.Command{
		Use:   "index-md [path]",
		Short: "Generate OKF index.md directory listings",
		Long: `Generate an OKF index.md in every bundle directory for progressive
disclosure. The root index.md carries the okf_version declaration; all other
index files have no frontmatter. Safe to re-run (overwrites).`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			root := cfg.RootPath
			if len(args) > 0 {
				root = args[0]
			}
			return runIndexMd(cfg, root)
		},
	}

	var logCmd = &cobra.Command{
		Use:   "log [path]",
		Short: "Generate OKF log.md from git history",
		Long: `Render an OKF log.md at the bundle root from git history, grouped by
date (newest first). Concept additions/modifications/deletions become
Creation/Update/Deprecation entries using each commit's message. Requires the
bundle to be a git repository.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			root := cfg.RootPath
			if len(args) > 0 {
				root = args[0]
			}
			return runLog(cfg, root)
		},
	}

	var templatesCmd = &cobra.Command{
		Use:   "templates",
		Short: "List available note templates",
		Long: `List all available note templates.

Templates provide pre-defined structures for common note types like
meeting notes, book reviews, user stories, and more.

Use with: okfw create "title" --template <name>`,
		Run: func(cmd *cobra.Command, args []string) {
			names := templates.List()
			sort.Strings(names)

			fmt.Println("Available templates:")
			fmt.Println()
			for _, name := range names {
				tmpl, _ := templates.Get(name)
				fmt.Printf("  %-15s %s [%s]\n", name, tmpl.Description, tmpl.Category)
			}
			fmt.Println("\nUsage: okfw create \"title\" --template <name>")
		},
	}

	var indexCmd = &cobra.Command{
		Use:   "index [file_path]",
		Short: "Index a concept for searching",
		Long: `Index a single concept file or all concepts in a directory.

If given a file, indexes that file.
If given a directory, indexes all .md files recursively.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			idx, err := index.OpenOrCreateIndex(cfg.IndexPath)
			if err != nil {
				return fmt.Errorf("failed to open index: %w", err)
			}
			defer idx.Close()

			path := args[0]
			info, err := os.Stat(path)
			if err != nil {
				return fmt.Errorf("failed to stat path: %w", err)
			}

			var files []string
			if info.IsDir() {
				err = filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
					if err != nil {
						return err
					}
					if fi.IsDir() && fi.Name() == "ephemeral" {
						return filepath.SkipDir
					}
					if !fi.IsDir() && strings.HasSuffix(p, ".md") && !isReservedBase(fi.Name()) {
						files = append(files, p)
					}
					return nil
				})
				if err != nil {
					return fmt.Errorf("failed to walk directory: %w", err)
				}
			} else {
				files = []string{path}
			}

			indexed := 0
			for _, filePath := range files {
				if err := indexFile(idx, filePath); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to index %s: %v\n", filePath, err)
					continue
				}
				indexed++
			}

			fmt.Printf("Indexed %d file(s)\n", indexed)
			return nil
		},
	}

	var typeFlag string
	var kindFlag string
	var dueBeforeFlag string
	var dueAfterFlag string

	var searchCmd = &cobra.Command{
		Use:   "search [query]",
		Short: "Search concepts",
		Long: `Search concepts by full-text query and/or field filters.

Examples:
  okfw search "authentication"              # Full-text search
  okfw search --project myproject           # Filter by project
  okfw search --category tethered           # Filter by category
  okfw search --tag golang --tag api        # Filter by tags (AND)
  okfw search --type todo                   # Filter by type
  okfw search --status open --priority high # Filter todos by status and priority
  okfw search --due-before 2026-03-01       # Todos due before a date
  okfw search --due-after 2026-02-01        # Todos due after a date
  okfw search "auth" --project myproject    # Combined search`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			idx, err := index.OpenOrCreateIndex(cfg.IndexPath)
			if err != nil {
				return fmt.Errorf("failed to open index: %w", err)
			}
			defer idx.Close()

			opts := index.SearchOptions{
				Project:   projectFlag,
				Category:  categoryFlag,
				Type:      typeFlag,
				Kind:      kindFlag,
				Tags:      tagsFlag,
				Limit:     limitFlag,
				Status:    statusFlag,
				Priority:  priorityFlag,
				DueBefore: dueBeforeFlag,
				DueAfter:  dueAfterFlag,
			}

			if len(args) > 0 {
				opts.Query = strings.Join(args, " ")
			}

			results, total, err := index.Search(idx, opts)
			if err != nil {
				return fmt.Errorf("search failed: %w", err)
			}

			if jsonFlag {
				jsonOutput, err := index.SearchResultsToJSON(results)
				if err != nil {
					return fmt.Errorf("failed to format JSON: %w", err)
				}
				fmt.Println(jsonOutput)
			} else {
				fmt.Print(index.FormatSearchResults(results, total))
			}

			return nil
		},
	}

	var graphStartFlag string
	var graphDepthFlag int
	var graphSyncFlag bool

	var graphCmd = &cobra.Command{
		Use:   "graph [path]",
		Short: "Show an ASCII tree of concept relationships",
		Long: `Print an ASCII tree showing relationships between concepts.

The tree is printed to stdout so it can be viewed directly in the terminal
or piped to other tools. A summary line (N nodes, M edges) is printed to
stderr so it does not pollute piped output.

If no path is given, the configured bundle root directory is used.
When a graph database (.okfw_graph.db) exists, it is used for fast queries.
Use --sync to force a fresh sync before querying.

Examples:
  okfw graph                             # Tree from bundle root
  okfw graph ~/bundle              # Tree of all concepts in directory
  okfw graph --start 20260213-abc-123    # Tree centered on a specific concept
  okfw graph --depth 3                   # Traverse up to 3 hops from start
  okfw graph --limit 20                  # Show up to 20 nodes (default: 10)
  okfw graph --sync                      # Sync graph DB before querying`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			path := cfg.RootPath
			if len(args) > 0 {
				path = args[0]
			}

			// Try to use persistent graph store.
			dbPath := filepath.Join(cfg.RootPath, cfg.GraphPath)
			_, dbErr := os.Stat(dbPath)
			useStore := dbErr == nil || graphSyncFlag

			var g *graph.Graph
			if useStore {
				store, err := graphdb.Open(dbPath)
				if err != nil {
					return err
				}
				defer store.Close()

				if graphSyncFlag || os.IsNotExist(dbErr) {
					result, err := store.Sync(path)
					if err != nil {
						return fmt.Errorf("graph sync failed: %w", err)
					}
					fmt.Fprintf(os.Stderr, "Synced: %d added, %d updated, %d deleted\n", result.Added, result.Updated, result.Deleted)
				}

				g, err = store.ToGraph()
				if err != nil {
					return err
				}
			} else {
				g, err = buildGraphFromPath(path, cfg.RootPath)
				if err != nil {
					return err
				}
			}

			nodes, err := queryGraph(g, graphStartFlag, graphLimitFlag, graphDepthFlag)
			if err != nil {
				return err
			}
			edges := g.GetEdges(nodes)

			fmt.Println(graph.GenerateMermaid(nodes, edges))
			fmt.Fprintf(os.Stderr, "%d nodes, %d edges\n", len(nodes), len(edges))
			return nil
		},
	}

	var tetherCmd = &cobra.Command{
		Use:   "tether [file_path]",
		Short: "Tether an untethered note (set category to tethered)",
		Long: `Tether an untethered note by changing its category to tethered.

Tethered notes require a project context. If the note doesn't have one,
you must provide it via --project flag or be in a git repository.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]

			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}

			z, err := config.ParseFrontmatter(content)
			if err != nil {
				return fmt.Errorf("failed to parse frontmatter: %w", err)
			}

			if z.Category == "tethered" {
				fmt.Println("Note is already tethered")
				return nil
			}

			project := z.Project
			if projectFlag != "" {
				project = projectFlag
			}
			if project == "" {
				cwd, _ := os.Getwd()
				project = gitctx.GetGitContext(cwd)
			}
			if project == "" {
				return fmt.Errorf("tethered notes require a project; use --project flag or run from a git repository")
			}

			newContent, err := updateFrontmatter(content, map[string]string{
				"category": "tethered",
				"project":  project,
			})
			if err != nil {
				return fmt.Errorf("failed to update frontmatter: %w", err)
			}

			if err := os.WriteFile(filePath, newContent, 0644); err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}

			fmt.Printf("Tethered note (Project: %s)\n", project)

			// Auto-index the updated note
			cfg, _ := config.Load("")
			if cfg != nil {
				if err := autoIndex(cfg, filePath); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to index note: %v\n", err)
				}
			}

			return nil
		},
	}

	tetherCmd.Flags().StringVarP(&projectFlag, "project", "p", "", "Project context for the tethered note")

	var untetherCmd = &cobra.Command{
		Use:   "untether [file_path]",
		Short: "Untether a tethered note (set category to untethered)",
		Long:  `Untether a tethered note by changing its category to untethered and clearing the project.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]

			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}

			z, err := config.ParseFrontmatter(content)
			if err != nil {
				return fmt.Errorf("failed to parse frontmatter: %w", err)
			}

			if z.Category == "untethered" {
				fmt.Println("Note is already untethered")
				return nil
			}

			newContent, err := updateFrontmatterRemove(content, map[string]string{
				"category": "untethered",
			}, []string{"project"})
			if err != nil {
				return fmt.Errorf("failed to update frontmatter: %w", err)
			}

			if err := os.WriteFile(filePath, newContent, 0644); err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}

			fmt.Println("Untethered note (project cleared)")

			// Auto-index the updated note
			cfg, _ := config.Load("")
			if cfg != nil {
				if err := autoIndex(cfg, filePath); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to index note: %v\n", err)
				}
			}

			return nil
		},
	}

	var setProjectCmd = &cobra.Command{
		Use:   "set-project [file_path] [project]",
		Short: "Set or update the project for a concept",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			project := args[1]

			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}

			newContent, err := updateFrontmatter(content, map[string]string{
				"project": project,
			})
			if err != nil {
				return fmt.Errorf("failed to update frontmatter: %w", err)
			}

			if err := os.WriteFile(filePath, newContent, 0644); err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}

			fmt.Printf("Updated project to: %s\n", project)

			// Auto-index the updated note
			cfg, _ := config.Load("")
			if cfg != nil {
				if err := autoIndex(cfg, filePath); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to index note: %v\n", err)
				}
			}

			return nil
		},
	}

	var backlinksCmd = &cobra.Command{
		Use:   "backlinks [id_or_file]",
		Short: "Find all notes that link to a concept",
		Long: `Find all concepts that contain links to the specified concept.

Links are detected in the format [[id]] or [[id|title]].
When a graph database exists, uses O(1) indexed lookup instead of filesystem scan.

Examples:
  okfw backlinks 202602131045           # Find backlinks by ID
  okfw backlinks ./notes/202602131045.md  # Find backlinks by file path
  okfw backlinks 202602131045 --json    # Output as JSON`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Determine the target ID
			targetID := args[0]
			targetTitle := ""

			// If it's a file path, extract the ID from frontmatter
			if strings.HasSuffix(targetID, ".md") {
				content, err := os.ReadFile(targetID)
				if err != nil {
					return fmt.Errorf("failed to read file: %w", err)
				}
				z, err := config.ParseFrontmatter(content)
				if err != nil {
					return fmt.Errorf("failed to parse frontmatter: %w", err)
				}
				targetID = z.ID
				targetTitle = z.Title
			}

			type Backlink struct {
				ID       string `json:"id"`
				Title    string `json:"title"`
				Project  string `json:"project"`
				Category string `json:"category"`
				FilePath string `json:"file_path"`
			}

			var backlinks []Backlink

			// Try indexed lookup via graph store.
			dbPath := filepath.Join(cfg.RootPath, cfg.GraphPath)
			if _, err := os.Stat(dbPath); err == nil {
				store, err := graphdb.Open(dbPath)
				if err != nil {
					return err
				}
				defer store.Close()

				nodes, err := store.GetBacklinks(targetID)
				if err != nil {
					return fmt.Errorf("backlink query failed: %w", err)
				}
				for _, n := range nodes {
					backlinks = append(backlinks, Backlink{
						ID:       n.ID,
						Title:    n.Title,
						Project:  n.Project,
						Category: n.Category,
						FilePath: n.FilePath,
					})
				}
				// Get target title if we don't have it yet.
				if targetTitle == "" {
					if target, _ := store.GetNode(targetID); target != nil {
						targetTitle = target.Title
					}
				}
			} else {
				// Fall back to filesystem scan.
				searchPath := cfg.RootPath
				if searchPath == "" {
					searchPath, _ = os.Getwd()
				}
				if _, err := os.Stat(searchPath); os.IsNotExist(err) {
					searchPath, _ = os.Getwd()
				}

				var files []string
				err = filepath.Walk(searchPath, func(p string, fi os.FileInfo, err error) error {
					if err != nil {
						return err
					}
					if fi.IsDir() && fi.Name() == "ephemeral" {
						return filepath.SkipDir
					}
					if !fi.IsDir() && strings.HasSuffix(p, ".md") && !isReservedBase(fi.Name()) {
						files = append(files, p)
					}
					return nil
				})
				if err != nil {
					return fmt.Errorf("failed to walk directory: %w", err)
				}

				for _, filePath := range files {
					content, err := os.ReadFile(filePath)
					if err != nil {
						continue
					}

					body := extractBody(content)
					absPath, _ := filepath.Abs(filePath)
					links := config.ResolveBodyLinkIDs(body, absPath, searchPath)

					for _, linkID := range links {
						if linkID == targetID {
							z, err := config.ParseFrontmatter(content)
							if err != nil {
								continue
							}

							absPath, _ := filepath.Abs(filePath)
							backlinks = append(backlinks, Backlink{
								ID:       z.ID,
								Title:    z.Title,
								Project:  z.Project,
								Category: z.Category,
								FilePath: absPath,
							})
							break
						}
					}
				}
			}

			if jsonFlag {
				output, err := backlinksToJSON(backlinks)
				if err != nil {
					return fmt.Errorf("failed to format JSON: %w", err)
				}
				fmt.Println(output)
			} else {
				if targetTitle != "" {
					fmt.Printf("Backlinks to: %s (%s)\n\n", targetTitle, targetID)
				} else {
					fmt.Printf("Backlinks to: %s\n\n", targetID)
				}

				if len(backlinks) == 0 {
					fmt.Println("No backlinks found")
				} else {
					for _, bl := range backlinks {
						fmt.Printf("  %s: %s [%s]\n", bl.ID, bl.Title, bl.Category)
						fmt.Printf("    %s\n\n", bl.FilePath)
					}
					fmt.Printf("Found %d backlink(s)\n", len(backlinks))
				}
			}

			return nil
		},
	}

	var exportLimitFlag int
	var exportStartFlag string
	var exportDepthFlag int

	var exportCmd = &cobra.Command{
		Use:   "export [path]",
		Short: "Export graph subset as portable markdown to ephemeral/",
		Long: `Export a subset of the bundle as portable markdown files.

Runs the same graph query as 'okfw graph' (BFS, --limit, --depth, --start),
then copies each note into <root>/ephemeral/ with [[id|title]] links
converted to [title](id.md) relative links that work in any markdown viewer.

The ephemeral/ directory has a .gitignore that ignores all its contents.

Examples:
  okfw export                             # Export from bundle root
  okfw export ~/bundle --limit 5    # Export 5 closest notes
  okfw export --start 20260213-abc-123    # Export from a specific concept
  okfw export --depth 2                   # Export up to 2 hops from start`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			path := cfg.RootPath
			if len(args) > 0 {
				path = args[0]
			}

			g, err := buildGraphFromPath(path, cfg.RootPath)
			if err != nil {
				return err
			}

			nodes, err := queryGraph(g, exportStartFlag, exportLimitFlag, exportDepthFlag)
			if err != nil {
				return err
			}

			// Map each exported concept's bundle-relative path to its ID so that
			// in-bundle markdown links can be flattened to <id>.md in the export.
			pathToID := make(map[string]string, len(nodes))
			for _, n := range nodes {
				if rel, err := filepath.Rel(cfg.RootPath, n.FilePath); err == nil {
					pathToID[filepath.ToSlash(rel)] = n.ID
				}
			}

			// Prepare ephemeral directory
			ephemeralDir := filepath.Join(cfg.RootPath, "ephemeral")
			if err := prepareEphemeralDir(ephemeralDir); err != nil {
				return err
			}

			// Export each note with links flattened to the exported file names.
			for _, node := range nodes {
				content, err := os.ReadFile(node.FilePath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to read %s: %v\n", node.FilePath, err)
					continue
				}

				fromDir := ""
				if rel, err := filepath.Rel(cfg.RootPath, filepath.Dir(node.FilePath)); err == nil {
					fromDir = filepath.ToSlash(rel)
					if fromDir == "." {
						fromDir = ""
					}
				}
				transformed := graph.RewriteLinks(string(content), func(href string) string {
					if id, ok := pathToID[graph.NormalizeTarget(href, fromDir)]; ok {
						return id + ".md"
					}
					return href
				})
				outPath := filepath.Join(ephemeralDir, node.ID+".md")
				if err := os.WriteFile(outPath, []byte(transformed), 0644); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to write %s: %v\n", outPath, err)
					continue
				}
			}

			fmt.Fprintf(os.Stderr, "Exported %d files to ephemeral/\n", len(nodes))
			return nil
		},
	}

	// Flags
	createCmd.Flags().StringVar(&noteCategory, "category", "untethered", "Note category (untethered or tethered)")
	createCmd.Flags().StringVarP(&projectFlag, "project", "p", "", "Project context (auto-detected from git if not provided)")
	createCmd.Flags().StringVarP(&templateFlag, "template", "T", "", "Use a template (see 'okfw templates' for list)")
	createCmd.Flags().StringVarP(&descriptionFlag, "description", "d", "", "One-sentence summary (OKF description)")
	migrateCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "Preview changes without modifying the bundle")
	createCmd.Flags().StringVar(&pathFlag, "path", "", "Place the concept in a bundle subdirectory (e.g. projects/okf)")
	createCmd.Flags().StringSliceVar(&tagsFlag, "tags", nil, "Additional tags (comma-separated)")

	searchCmd.Flags().StringVarP(&projectFlag, "project", "p", "", "Filter by project")
	searchCmd.Flags().StringVarP(&categoryFlag, "category", "c", "", "Filter by category (untethered/tethered)")
	searchCmd.Flags().StringVarP(&typeFlag, "type", "t", "", "Filter by OKF type (free-form)")
	searchCmd.Flags().StringVar(&kindFlag, "kind", "", "Filter by structured kind (note/todo/daily-note/issue)")
	searchCmd.Flags().StringSliceVarP(&tagsFlag, "tag", "T", nil, "Filter by tag (can be repeated)")
	searchCmd.Flags().StringVar(&statusFlag, "status", "", "Filter by status (open/in_progress/closed)")
	searchCmd.Flags().StringVar(&priorityFlag, "priority", "", "Filter by priority (high/medium/low)")
	searchCmd.Flags().StringVar(&dueBeforeFlag, "due-before", "", "Filter todos due before date (YYYY-MM-DD)")
	searchCmd.Flags().StringVar(&dueAfterFlag, "due-after", "", "Filter todos due after date (YYYY-MM-DD)")
	searchCmd.Flags().IntVarP(&limitFlag, "limit", "l", 20, "Maximum number of results")
	searchCmd.Flags().BoolVar(&jsonFlag, "json", false, "Output results as JSON")

	graphCmd.Flags().IntVarP(&graphLimitFlag, "limit", "l", 10, "Maximum number of nodes to display")
	graphCmd.Flags().StringVar(&graphStartFlag, "start", "", "Start graph from a specific concept ID")
	graphCmd.Flags().IntVar(&graphDepthFlag, "depth", 0, "Maximum BFS depth (hops) from start node (0 = unlimited)")
	graphCmd.Flags().BoolVar(&graphSyncFlag, "sync", false, "Sync graph database before querying")

	exportCmd.Flags().IntVarP(&exportLimitFlag, "limit", "l", 10, "Maximum number of notes to export")
	exportCmd.Flags().StringVar(&exportStartFlag, "start", "", "Start export from a specific concept ID")
	exportCmd.Flags().IntVar(&exportDepthFlag, "depth", 0, "Maximum BFS depth (hops) from start node (0 = unlimited)")

	backlinksCmd.Flags().BoolVar(&jsonFlag, "json", false, "Output results as JSON")

	// Daily notes flags
	var dateFlag string
	var yesterdayFlag bool
	var listFlag bool
	var weekFlag bool
	var monthFlag bool

	var dailyCmd = &cobra.Command{
		Use:   "daily",
		Short: "Create or open a daily note",
		Long: `Create or open a daily note for a specific date.

Daily notes provide a consistent place for capturing thoughts, tasks, and
reflections throughout the day. Each day has exactly one daily note.

Examples:
  okfw daily                    # Today's daily note
  okfw daily --yesterday        # Yesterday's daily note
  okfw daily --date 2026-02-10  # Specific date
  okfw daily --list             # List recent daily notes
  okfw daily --list --week      # This week's daily notes
  okfw daily --list --month     # This month's daily notes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Determine target date
			targetDate := time.Now()
			if yesterdayFlag {
				targetDate = targetDate.AddDate(0, 0, -1)
			} else if dateFlag != "" {
				parsed, err := time.Parse("2006-01-02", dateFlag)
				if err != nil {
					return fmt.Errorf("invalid date format (use YYYY-MM-DD): %w", err)
				}
				targetDate = parsed
			}

			// Handle --list mode
			if listFlag {
				return listDailyNotes(cfg, weekFlag, monthFlag, jsonFlag)
			}

			// Create or open daily note
			return createOrOpenDaily(cfg, targetDate)
		},
	}

	dailyCmd.Flags().StringVar(&dateFlag, "date", "", "Target date (YYYY-MM-DD format)")
	dailyCmd.Flags().BoolVar(&yesterdayFlag, "yesterday", false, "Use yesterday's date")
	dailyCmd.Flags().BoolVar(&listFlag, "list", false, "List recent daily notes")
	dailyCmd.Flags().BoolVar(&weekFlag, "week", false, "Show this week's notes (with --list)")
	dailyCmd.Flags().BoolVar(&monthFlag, "month", false, "Show this month's notes (with --list)")
	dailyCmd.Flags().BoolVar(&jsonFlag, "json", false, "Output as JSON (with --list)")

	// Todo-specific flags
	var dueFlag string
	var overdueFlag bool
	var closedFlag bool
	var todayFlag bool
	var thisWeekFlag bool

	var todoCmd = &cobra.Command{
		Use:   "todo [title]",
		Short: "Create a new todo",
		Long: `Create a new actionable todo item.

Todos are a special type of concept with status tracking, due dates, and priority.

Examples:
  okfw todo "Fix login bug"                          # Basic todo
  okfw todo "Update docs" --due 2026-02-20           # With due date
  okfw todo "Critical fix" --priority high           # With priority
  okfw todo "Feature work" --project myproj          # With project`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			title := args[0]
			id := uuid.Must(uuid.NewV7()).String()

			cwd, _ := os.Getwd()
			project := gitctx.GetGitContext(cwd)
			if projectFlag != "" {
				project = projectFlag
			}

			// Get the todo template
			tmpl, err := templates.Get("todo")
			if err != nil {
				return fmt.Errorf("failed to get todo template: %w", err)
			}

			// Set todo options
			todoOpts := &templates.TodoOptions{
				Status:   "open",
				Due:      dueFlag,
				Priority: priorityFlag,
			}

			// Generate the note with todo template
			content, err := tmpl.GenerateNoteWithOptions(id, title, project, tagsFlag, todoOpts)
			if err != nil {
				return fmt.Errorf("failed to generate todo: %w", err)
			}

			// Placement: --path wins, else the project folder, else bundle root.
			outputDir, err := conceptDir(cfg, pathFlag, project)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}

			filePath := filepath.Join(outputDir, noteFilename(outputDir, title, id))
			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}

			fmt.Printf("Created todo: %s\n", title)
			if dueFlag != "" {
				fmt.Printf("Due: %s\n", dueFlag)
			}
			fmt.Printf("File: %s\n", filePath)

			// Auto-index
			if err := autoIndex(cfg, filePath); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to index todo: %v\n", err)
			}

			return nil
		},
	}

	todoCmd.Flags().StringVarP(&projectFlag, "project", "p", "", "Project context")
	todoCmd.Flags().StringVar(&pathFlag, "path", "", "Place the todo in a bundle subdirectory (e.g. projects/okf)")
	todoCmd.Flags().StringVar(&dueFlag, "due", "", "Due date (YYYY-MM-DD)")
	todoCmd.Flags().StringVar(&priorityFlag, "priority", "", "Priority (high, medium, low)")
	todoCmd.Flags().StringSliceVar(&tagsFlag, "tags", nil, "Additional tags")

	var setStatusCmd = &cobra.Command{
		Use:   "set-status [id_or_file] [status]",
		Short: "Set the status of a todo",
		Long: `Set the status of a todo to open, in_progress, or closed.

Examples:
  okfw set-status 202602131045 closed          # By ID
  okfw set-status ./path/to/todo.md in_progress # By file path
  okfw set-status ./path/to/todo.md open        # Reopen a closed todo`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			status := args[1]
			validStatuses := map[string]bool{"open": true, "in_progress": true, "closed": true}
			if !validStatuses[status] {
				return fmt.Errorf("invalid status %q: must be open, in_progress, or closed", status)
			}

			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			filePath, err := resolveConceptPath(cfg, args[0])
			if err != nil {
				return err
			}

			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}

			z, err := config.ParseFrontmatter(content)
			if err != nil {
				return fmt.Errorf("failed to parse frontmatter: %w", err)
			}

			if !z.IsTodo() {
				return fmt.Errorf("note is not a todo (type: %s)", z.GetKind())
			}

			if z.Status == status {
				fmt.Printf("Todo is already %s\n", status)
				return nil
			}

			var newContent []byte
			switch status {
			case "closed":
				completedDate := time.Now().Format("2006-01-02")
				newContent, err = updateFrontmatter(content, map[string]string{
					"status":    "closed",
					"completed": completedDate,
				})
				if err != nil {
					return fmt.Errorf("failed to update frontmatter: %w", err)
				}
				fmt.Printf("Closed todo: %s\n", z.Title)
				fmt.Printf("Completed: %s\n", completedDate)
			case "open":
				newContent, err = updateFrontmatterRemove(content, map[string]string{
					"status": "open",
				}, []string{"completed"})
				if err != nil {
					return fmt.Errorf("failed to update frontmatter: %w", err)
				}
				fmt.Printf("Reopened todo: %s\n", z.Title)
			case "in_progress":
				newContent, err = updateFrontmatter(content, map[string]string{
					"status": "in_progress",
				})
				if err != nil {
					return fmt.Errorf("failed to update frontmatter: %w", err)
				}
				fmt.Printf("Set todo to in_progress: %s\n", z.Title)
			}

			if err := os.WriteFile(filePath, newContent, 0644); err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}

			// Auto-index
			if err := autoIndex(cfg, filePath); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to index todo: %v\n", err)
			}

			return nil
		},
	}

	var addTagsCmd = &cobra.Command{
		Use:   "add-tags [id_or_file] [tags...]",
		Short: "Add tags to a concept",
		Long: `Add one or more tags to a concept's frontmatter. Duplicate tags are ignored.

Examples:
  okfw add-tags ./path/to/note.md golang api
  okfw add-tags 202602131045 security review`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]

			// If it looks like an ID (not a file path), resolve it
			if !strings.HasSuffix(filePath, ".md") && !strings.Contains(filePath, "/") {
				cfg, err := config.Load("")
				if err != nil {
					return fmt.Errorf("failed to load config: %w", err)
				}
				filePath, err = resolveConceptPath(cfg, args[0])
				if err != nil {
					return err
				}
			}

			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}

			z, err := config.ParseFrontmatter(content)
			if err != nil {
				return fmt.Errorf("failed to parse frontmatter: %w", err)
			}

			// Deduplicate: only add tags that don't already exist
			existing := make(map[string]bool)
			for _, t := range z.Tags {
				existing[t] = true
			}
			var added []string
			for _, t := range args[1:] {
				if !existing[t] {
					added = append(added, t)
					existing[t] = true
				}
			}

			if len(added) == 0 {
				fmt.Println("All tags already present")
				return nil
			}

			newContent, err := addFrontmatterTags(content, added)
			if err != nil {
				return fmt.Errorf("failed to update frontmatter: %w", err)
			}

			// Bump the OKF timestamp (last meaningful change).
			newContent, err = updateFrontmatter(newContent, nil)
			if err != nil {
				return fmt.Errorf("failed to update timestamp: %w", err)
			}

			if err := os.WriteFile(filePath, newContent, 0644); err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}

			fmt.Printf("Added tags: %s\n", strings.Join(added, ", "))

			// Auto-index (best-effort)
			cfg, _ := config.Load("")
			if cfg != nil {
				if err := autoIndex(cfg, filePath); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to index: %v\n", err)
				}
			}

			return nil
		},
	}

	var validateOKF bool
	var validateCmd = &cobra.Command{
		Use:   "validate [id_or_file_or_path]",
		Short: "Validate a concept's frontmatter, or a bundle's OKF conformance",
		Long: `Validate a concept's frontmatter against the CUE schema and business rules,
or check a whole bundle for OKF conformance with --okf.

Examples:
  okfw validate ./path/to/note.md
  okfw validate 202602131045
  okfw validate --okf                 # conformance check of the bundle root
  okfw validate --okf ~/okfw          # conformance check of a path`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if validateOKF {
				cfg, err := config.Load("")
				if err != nil {
					return fmt.Errorf("failed to load config: %w", err)
				}
				root := cfg.RootPath
				if len(args) > 0 {
					root = args[0]
				}
				return runValidateOKF(cfg, root)
			}

			if len(args) == 0 {
				return fmt.Errorf("a concept id or file is required (or use --okf to check the bundle)")
			}
			filePath := args[0]

			// If it looks like an ID (not a file path), resolve it
			if !strings.HasSuffix(filePath, ".md") && !strings.Contains(filePath, "/") {
				cfg, err := config.Load("")
				if err != nil {
					return fmt.Errorf("failed to load config: %w", err)
				}
				filePath, err = resolveConceptPath(cfg, args[0])
				if err != nil {
					return err
				}
			}

			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}

			z, err := config.ParseAndValidate(content)
			if err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}

			fmt.Printf("Valid %s: %s\n", z.GetKind(), z.Title)
			return nil
		},
	}

	var todosCmd = &cobra.Command{
		Use:   "todos [query]",
		Short: "List and search todos",
		Long: `List and search todos with various filters.

Examples:
  okfw todos                        # All open todos
  okfw todos --closed               # All closed todos
  okfw todos --project myproj       # Filter by project
  okfw todos --priority high        # Filter by priority
  okfw todos --overdue              # Overdue todos
  okfw todos --due today            # Due today
  okfw todos --due this-week        # Due this week
  okfw todos "fix bug"              # Full-text search in todos`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			idx, err := index.OpenOrCreateIndex(cfg.IndexPath)
			if err != nil {
				return fmt.Errorf("failed to open index: %w", err)
			}
			defer idx.Close()

			opts := index.SearchOptions{
				Kind:     "todo",
				Project:  projectFlag,
				Tags:     tagsFlag,
				Limit:    limitFlag,
				Priority: priorityFlag,
			}

			// Status filter
			if closedFlag {
				opts.Status = "closed"
			} else if statusFlag != "" {
				opts.Status = statusFlag
			} else {
				// Default to open todos
				opts.Status = "open"
			}

			// Due date filters
			today := time.Now().Format("2006-01-02")
			if overdueFlag {
				opts.DueBefore = today
				opts.Status = "open" // Overdue only makes sense for open todos
			} else if todayFlag {
				opts.DueAfter = today
				opts.DueBefore = today
			} else if thisWeekFlag {
				opts.DueAfter = today
				endOfWeek := time.Now().AddDate(0, 0, 7-int(time.Now().Weekday()))
				opts.DueBefore = endOfWeek.Format("2006-01-02")
			}

			// Full-text query
			if len(args) > 0 {
				opts.Query = strings.Join(args, " ")
			}

			results, total, err := index.Search(idx, opts)
			if err != nil {
				return fmt.Errorf("search failed: %w", err)
			}

			if jsonFlag {
				jsonOutput, err := index.SearchResultsToJSON(results)
				if err != nil {
					return fmt.Errorf("failed to format JSON: %w", err)
				}
				fmt.Println(jsonOutput)
			} else {
				fmt.Print(index.FormatTodoResults(results, total))
			}

			return nil
		},
	}

	todosCmd.Flags().StringVarP(&projectFlag, "project", "p", "", "Filter by project")
	todosCmd.Flags().StringVar(&statusFlag, "status", "", "Filter by status (open, in_progress, closed)")
	todosCmd.Flags().BoolVar(&closedFlag, "closed", false, "Show closed todos")
	todosCmd.Flags().StringVar(&priorityFlag, "priority", "", "Filter by priority (high, medium, low)")
	todosCmd.Flags().BoolVar(&overdueFlag, "overdue", false, "Show overdue todos")
	todosCmd.Flags().BoolVar(&todayFlag, "today", false, "Show todos due today")
	todosCmd.Flags().BoolVar(&thisWeekFlag, "this-week", false, "Show todos due this week")
	todosCmd.Flags().StringSliceVarP(&tagsFlag, "tag", "T", nil, "Filter by tag")
	todosCmd.Flags().IntVarP(&limitFlag, "limit", "l", 50, "Maximum number of results")
	todosCmd.Flags().BoolVar(&jsonFlag, "json", false, "Output as JSON")

	var todoListCmd = &cobra.Command{
		Use:   "todo-list",
		Short: "Generate a markdown todo list",
		Long: `Generate a markdown file listing todos.

The generated file is placed in the configured todos_path directory.

Examples:
  okfw todo-list                           # All open todos
  okfw todo-list --project myproj          # Project-specific list
  okfw todo-list --today                   # Due today
  okfw todo-list --output my-todos.md      # Custom filename`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			idx, err := index.OpenOrCreateIndex(cfg.IndexPath)
			if err != nil {
				return fmt.Errorf("failed to open index: %w", err)
			}
			defer idx.Close()

			opts := index.SearchOptions{
				Kind:    "todo",
				Project: projectFlag,
				Limit:   200, // Higher limit for list generation
			}

			// Default to open todos
			if closedFlag {
				opts.Status = "closed"
			} else {
				opts.Status = "open"
			}

			// Due date filters
			today := time.Now().Format("2006-01-02")
			if todayFlag {
				opts.DueAfter = today
				opts.DueBefore = today
			} else if thisWeekFlag {
				opts.DueAfter = today
				endOfWeek := time.Now().AddDate(0, 0, 7-int(time.Now().Weekday()))
				opts.DueBefore = endOfWeek.Format("2006-01-02")
			}

			results, _, err := index.Search(idx, opts)
			if err != nil {
				return fmt.Errorf("search failed: %w", err)
			}

			// Generate markdown
			markdown := generateTodoListMarkdown(results, projectFlag, todayFlag, thisWeekFlag)

			// Determine output path
			todosDir := cfg.TodosPath
			if !filepath.IsAbs(todosDir) {
				todosDir = filepath.Join(cfg.RootPath, todosDir)
			}

			if err := os.MkdirAll(todosDir, 0755); err != nil {
				return fmt.Errorf("failed to create todos directory: %w", err)
			}

			// Ensure .gitignore includes todos path
			if err := ensureGitignore(cfg.RootPath, cfg.TodosPath); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not update .gitignore: %v\n", err)
			}

			// Determine output filename
			outputName := outputFlag
			if outputName == "" {
				if projectFlag != "" {
					outputName = fmt.Sprintf("%s-todos.md", projectFlag)
				} else if todayFlag {
					outputName = fmt.Sprintf("todos-%s.md", today)
				} else {
					outputName = "todos.md"
				}
			}
			if !strings.HasSuffix(outputName, ".md") {
				outputName += ".md"
			}

			outputPath := filepath.Join(todosDir, outputName)
			if err := os.WriteFile(outputPath, []byte(markdown), 0644); err != nil {
				return fmt.Errorf("failed to write todo list: %w", err)
			}

			fmt.Printf("Generated todo list with %d items\n", len(results))
			fmt.Printf("Output: %s\n", outputPath)

			return nil
		},
	}

	todoListCmd.Flags().StringVarP(&projectFlag, "project", "p", "", "Filter by project")
	todoListCmd.Flags().BoolVar(&todayFlag, "today", false, "Show todos due today")
	todoListCmd.Flags().BoolVar(&thisWeekFlag, "this-week", false, "Show todos due this week")
	todoListCmd.Flags().BoolVar(&closedFlag, "closed", false, "Include closed todos")
	todoListCmd.Flags().StringVarP(&outputFlag, "output", "o", "", "Output filename")

	// Git workflow commands
	var helloCmd = &cobra.Command{
		Use:   "hello",
		Short: "Start your day by creating a dated branch",
		Long: `Start your bundle session by creating a new branch from main.

The branch name will be today's date in YYYYMMDD format.

This command will:
1. Ensure you're on a clean working tree (no uncommitted changes)
2. Switch to main and pull latest changes
3. Create a new branch named with today's date

Examples:
  okfw hello    # Creates branch like "20260213" from main`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Determine working directory (use bundle root if it exists)
			workDir := cfg.RootPath
			if _, err := os.Stat(workDir); os.IsNotExist(err) {
				workDir, _ = os.Getwd()
			}

			// Check if we're in a git repository
			if !isGitRepo(workDir) {
				return fmt.Errorf("not a git repository: %s", workDir)
			}

			// Check for uncommitted changes
			hasChanges, err := hasUncommittedChanges(workDir)
			if err != nil {
				return fmt.Errorf("failed to check git status: %w", err)
			}
			if hasChanges {
				fmt.Println("Warning: You have uncommitted changes.")
				fmt.Println("")
				fmt.Println("View uncommitted changes with:")
				fmt.Println("  git status          # See changed files")
				fmt.Println("  git diff            # See unstaged changes")
				fmt.Println("  git diff --staged   # See staged changes")
				fmt.Println("")
				fmt.Println("Commit or stash your changes before running 'okfw hello'.")
				return nil
			}

			// Generate branch name from today's date
			branchName := time.Now().Format("20060102")

			// Check if branch already exists
			exists, err := branchExists(workDir, branchName)
			if err != nil {
				return fmt.Errorf("failed to check branch: %w", err)
			}
			if exists {
				fmt.Printf("Warning: Branch '%s' already exists.\n", branchName)
				fmt.Println("")
				fmt.Println("To switch to this branch:")
				fmt.Printf("  git checkout %s\n", branchName)
				return nil
			}

			// Switch to main and pull
			if err := gitCheckout(workDir, "main"); err != nil {
				// Try "master" if "main" doesn't exist
				if err := gitCheckout(workDir, "master"); err != nil {
					return fmt.Errorf("failed to checkout main branch: %w", err)
				}
			}

			if err := gitPull(workDir); err != nil {
				// Pull might fail if no remote, that's okay
				fmt.Fprintf(os.Stderr, "Warning: git pull failed (no remote?): %v\n", err)
			}

			// Create and checkout new branch
			if err := gitCheckoutNewBranch(workDir, branchName); err != nil {
				return fmt.Errorf("failed to create branch: %w", err)
			}

			fmt.Printf("Good morning! Created branch: %s\n", branchName)
			fmt.Println("")
			fmt.Println("You're ready to capture ideas. When you're done, run:")
			fmt.Println("  okfw goodbye")

			return nil
		},
	}

	var goodbyeCmd = &cobra.Command{
		Use:   "goodbye",
		Short: "End your day by committing and merging to main",
		Long: `End your bundle session by committing changes and merging to main.

This command will:
1. Stage all changes in the current branch
2. Commit with a message including today's date
3. Switch to main
4. Merge the dated branch into main
5. Delete the dated branch

Examples:
  okfw goodbye    # Commits changes and merges to main`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Determine working directory
			workDir := cfg.RootPath
			if _, err := os.Stat(workDir); os.IsNotExist(err) {
				workDir, _ = os.Getwd()
			}

			// Check if we're in a git repository
			if !isGitRepo(workDir) {
				return fmt.Errorf("not a git repository: %s", workDir)
			}

			// Get current branch
			currentBranch, err := getCurrentBranch(workDir)
			if err != nil {
				return fmt.Errorf("failed to get current branch: %w", err)
			}

			// Verify we're on a dated branch (YYYYMMDD format)
			if !isDateBranch(currentBranch) {
				fmt.Printf("Warning: Current branch '%s' is not a dated branch (YYYYMMDD format).\n", currentBranch)
				fmt.Println("")
				fmt.Println("The 'goodbye' command is meant to be used with branches created by 'okfw hello'.")
				fmt.Println("")
				fmt.Println("To commit manually:")
				fmt.Println("  git add -A")
				fmt.Println("  git commit -m \"Your message\"")
				return nil
			}

			// Check if there are changes to commit
			hasChanges, err := hasUncommittedChanges(workDir)
			if err != nil {
				return fmt.Errorf("failed to check git status: %w", err)
			}

			if hasChanges {
				// Stage all changes
				if err := gitAddAll(workDir); err != nil {
					return fmt.Errorf("failed to stage changes: %w", err)
				}

				// Commit with dated message
				commitMsg := fmt.Sprintf("Notes for %s", currentBranch)
				if err := gitCommit(workDir, commitMsg); err != nil {
					return fmt.Errorf("failed to commit: %w", err)
				}
				fmt.Printf("Committed changes: %s\n", commitMsg)
			} else {
				fmt.Println("No changes to commit.")
			}

			// Switch to main
			mainBranch := "main"
			if err := gitCheckout(workDir, mainBranch); err != nil {
				// Try "master" if "main" doesn't exist
				mainBranch = "master"
				if err := gitCheckout(workDir, mainBranch); err != nil {
					return fmt.Errorf("failed to checkout main branch: %w", err)
				}
			}

			// Merge the dated branch
			if err := gitMerge(workDir, currentBranch); err != nil {
				return fmt.Errorf("failed to merge branch: %w", err)
			}
			fmt.Printf("Merged '%s' into '%s'\n", currentBranch, mainBranch)

			// Delete the dated branch
			if err := gitDeleteBranch(workDir, currentBranch); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to delete branch '%s': %v\n", currentBranch, err)
			} else {
				fmt.Printf("Deleted branch: %s\n", currentBranch)
			}

			fmt.Println("")
			fmt.Println("Goodbye! Your notes have been merged to main.")

			return nil
		},
	}

	// --- graph-sync command ---
	var graphSyncValidateFlag bool

	var graphSyncCmd = &cobra.Command{
		Use:   "graph-sync [path]",
		Short: "Sync the persistent graph database from filesystem",
		Long: `Walk the bundle directory and synchronize the persistent graph database.

Adds new notes, updates changed notes, and purges deleted notes.
The graph database enables fast O(1) backlink lookups and indexed traversals.

Examples:
  okfw graph-sync                    # Sync from configured root
  okfw graph-sync ~/bundle     # Sync from explicit path
  okfw graph-sync --validate         # Sync and validate edge integrity`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			path := cfg.RootPath
			if len(args) > 0 {
				path = args[0]
			}

			dbPath := filepath.Join(cfg.RootPath, cfg.GraphPath)
			store, err := graphdb.Open(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()

			// Ensure graph DB is gitignored.
			ensureGitignore(cfg.RootPath, cfg.GraphPath)

			result, err := store.Sync(path)
			if err != nil {
				return fmt.Errorf("sync failed: %w", err)
			}

			fmt.Printf("Graph synced: %d total, %d added, %d updated, %d deleted\n",
				result.Total, result.Added, result.Updated, result.Deleted)

			if result.Dangling > 0 {
				fmt.Printf("Warning: %d dangling edge(s) detected\n", result.Dangling)
			}

			if graphSyncValidateFlag {
				integrity, err := store.ValidateIntegrity()
				if err != nil {
					return fmt.Errorf("integrity check failed: %w", err)
				}
				fmt.Print(integrity.String())
			}

			return nil
		},
	}
	graphSyncCmd.Flags().BoolVar(&graphSyncValidateFlag, "validate", false, "Run CUE edge validation and report integrity issues")

	// --- graph-query command ---
	var graphQueryDepthFlag int
	var graphQueryLimitFlag int
	var graphQueryJSONFlag bool

	var graphQueryCmd = &cobra.Command{
		Use:   "graph-query <subcommand> <arg>",
		Short: "Query the persistent graph database",
		Long: `Query the graph database for backlinks, clusters, tags, or dangling edges.

All subcommands support --json for machine-readable output (e.g. for Neovim integration).

Examples:
  okfw graph-query backlinks <id>            # Find backlinks
  okfw graph-query cluster <id> --depth 2    # N-depth cluster
  okfw graph-query tag <tag>                 # Notes by tag
  okfw graph-query dangling                  # Dangling edges`,
	}

	var gqBacklinksCmd = &cobra.Command{
		Use:   "backlinks <id>",
		Short: "Find all notes linking to a node",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			dbPath := filepath.Join(cfg.RootPath, cfg.GraphPath)
			store, err := graphdb.Open(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()

			nodes, err := store.GetBacklinks(args[0])
			if err != nil {
				return err
			}

			if graphQueryJSONFlag {
				data, _ := json.MarshalIndent(nodes, "", "  ")
				fmt.Println(string(data))
			} else {
				if len(nodes) == 0 {
					fmt.Println("No backlinks found")
				} else {
					for _, n := range nodes {
						fmt.Printf("  %s: %s [%s]\n", n.ID, n.Title, n.Category)
					}
					fmt.Printf("\nFound %d backlink(s)\n", len(nodes))
				}
			}
			return nil
		},
	}

	var gqClusterCmd = &cobra.Command{
		Use:   "cluster <id>",
		Short: "Find connected nodes within N hops",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			dbPath := filepath.Join(cfg.RootPath, cfg.GraphPath)
			store, err := graphdb.Open(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()

			nodes, edges, err := store.GetCluster(args[0], graphQueryDepthFlag, graphQueryLimitFlag)
			if err != nil {
				return err
			}

			if graphQueryJSONFlag {
				result := struct {
					Nodes []graphdb.NodeRecord `json:"nodes"`
					Edges []graphdb.EdgeRecord `json:"edges"`
				}{Nodes: nodes, Edges: edges}
				data, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(data))
			} else {
				for _, n := range nodes {
					fmt.Printf("  %s: %s [%s]\n", n.ID, n.Title, n.Category)
				}
				fmt.Printf("\n%d nodes, %d edges\n", len(nodes), len(edges))
			}
			return nil
		},
	}
	gqClusterCmd.Flags().IntVar(&graphQueryDepthFlag, "depth", 0, "Maximum BFS depth (0 = unlimited)")
	gqClusterCmd.Flags().IntVarP(&graphQueryLimitFlag, "limit", "l", 10, "Maximum number of nodes")

	var gqTagCmd = &cobra.Command{
		Use:   "tag <tag>",
		Short: "Find all notes with a given tag",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			dbPath := filepath.Join(cfg.RootPath, cfg.GraphPath)
			store, err := graphdb.Open(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()

			nodes, err := store.GetByTag(args[0])
			if err != nil {
				return err
			}

			if graphQueryJSONFlag {
				data, _ := json.MarshalIndent(nodes, "", "  ")
				fmt.Println(string(data))
			} else {
				if len(nodes) == 0 {
					fmt.Printf("No notes tagged %q\n", args[0])
				} else {
					for _, n := range nodes {
						fmt.Printf("  %s: %s [%s]\n", n.ID, n.Title, n.Category)
					}
					fmt.Printf("\nFound %d note(s) tagged %q\n", len(nodes), args[0])
				}
			}
			return nil
		},
	}

	var gqDanglingCmd = &cobra.Command{
		Use:   "dangling",
		Short: "Find edges pointing to non-existent notes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			dbPath := filepath.Join(cfg.RootPath, cfg.GraphPath)
			store, err := graphdb.Open(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()

			edges, err := store.GetDanglingEdges()
			if err != nil {
				return err
			}

			if graphQueryJSONFlag {
				data, _ := json.MarshalIndent(edges, "", "  ")
				fmt.Println(string(data))
			} else {
				if len(edges) == 0 {
					fmt.Println("No dangling edges found")
				} else {
					for _, e := range edges {
						fmt.Printf("  %s -[%s]-> %s (target not found)\n", e.From, e.Predicate, e.To)
					}
					fmt.Printf("\nFound %d dangling edge(s)\n", len(edges))
				}
			}
			return nil
		},
	}

	graphQueryCmd.PersistentFlags().BoolVar(&graphQueryJSONFlag, "json", false, "Output as JSON")
	graphQueryCmd.AddCommand(gqBacklinksCmd)
	graphQueryCmd.AddCommand(gqClusterCmd)
	graphQueryCmd.AddCommand(gqTagCmd)
	graphQueryCmd.AddCommand(gqDanglingCmd)

	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(migrateCmd)
	rootCmd.AddCommand(rootPathCmd)
	rootCmd.AddCommand(indexMdCmd)
	rootCmd.AddCommand(logCmd)
	rootCmd.AddCommand(templatesCmd)
	rootCmd.AddCommand(indexCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(graphCmd)
	rootCmd.AddCommand(graphSyncCmd)
	rootCmd.AddCommand(graphQueryCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(tetherCmd)
	rootCmd.AddCommand(untetherCmd)
	rootCmd.AddCommand(setProjectCmd)
	rootCmd.AddCommand(backlinksCmd)
	rootCmd.AddCommand(dailyCmd)
	rootCmd.AddCommand(todoCmd)
	rootCmd.AddCommand(setStatusCmd)
	rootCmd.AddCommand(todosCmd)
	rootCmd.AddCommand(todoListCmd)
	rootCmd.AddCommand(addTagsCmd)
	validateCmd.Flags().BoolVar(&validateOKF, "okf", false, "Check the whole bundle for OKF conformance")
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(helloCmd)
	rootCmd.AddCommand(goodbyeCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// parseConceptForGraph parses a concept file and returns a graph node
func parseConceptForGraph(filePath, bundleRoot string) (*graph.Node, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	z, err := config.ParseFrontmatter(content)
	if err != nil {
		return nil, err
	}

	// Validate this is a real concept (must have an ID)
	if z.ID == "" {
		return nil, fmt.Errorf("not a valid concept: no id field")
	}

	absPath, _ := filepath.Abs(filePath)

	// Resolve vanilla markdown links in the body to target concept IDs.
	body := extractBody(content)
	links := config.ResolveBodyLinkIDs(body, absPath, bundleRoot)

	return &graph.Node{
		ID:       z.ID,
		Title:    z.Title,
		FilePath: absPath,
		Project:  z.Project,
		Category: z.Category,
		Parent:   z.Parent,
		Links:    links,
	}, nil
}

// buildGraphFromPath walks the directory for .md files (skipping hidden dirs and ephemeral/),
// parses each as a concept, and returns a fully-built graph.
func buildGraphFromPath(path, bundleRoot string) (*graph.Graph, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat path %q: %w", path, err)
	}

	var files []string
	if info.IsDir() {
		err = filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if fi.IsDir() && (strings.HasPrefix(fi.Name(), ".") && fi.Name() != "." || fi.Name() == "ephemeral") {
				return filepath.SkipDir
			}
			if !fi.IsDir() && strings.HasSuffix(p, ".md") && !isReservedBase(fi.Name()) {
				files = append(files, p)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to walk directory: %w", err)
		}
	} else {
		files = []string{path}
	}

	g := graph.New()
	for _, filePath := range files {
		node, err := parseConceptForGraph(filePath, bundleRoot)
		if err != nil {
			continue
		}
		g.AddNode(node)
	}

	if g.NodeCount() == 0 {
		return nil, fmt.Errorf("no valid concepts found in %s", path)
	}

	g.BuildRelationships()
	return g, nil
}

// queryGraph runs FindConnected or FindAllConnected based on startID.
func queryGraph(g *graph.Graph, startID string, limit, depth int) ([]*graph.Node, error) {
	if startID != "" {
		if g.GetNode(startID) == nil {
			return nil, fmt.Errorf("start node %q not found in graph", startID)
		}
		return g.FindConnected(startID, limit, depth), nil
	}
	return g.FindAllConnected(limit, depth), nil
}

// prepareEphemeralDir wipes and recreates the ephemeral directory with a .gitignore.
func prepareEphemeralDir(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("failed to remove ephemeral dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create ephemeral dir: %w", err)
	}
	gitignore := "# Auto-generated — ignore all exported files\n*\n!.gitignore\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(gitignore), 0644); err != nil {
		return fmt.Errorf("failed to write .gitignore: %w", err)
	}
	return nil
}

// ensureGitignore ensures the .gitignore file includes the graph path
func ensureGitignore(basePath string, graphPath string) error {
	info, err := os.Stat(basePath)
	if err != nil {
		return err
	}

	var gitignorePath string
	if info.IsDir() {
		gitignorePath = filepath.Join(basePath, ".gitignore")
	} else {
		gitignorePath = filepath.Join(filepath.Dir(basePath), ".gitignore")
	}

	// Read existing .gitignore or create new
	var lines []string
	content, err := os.ReadFile(gitignorePath)
	if err == nil {
		lines = strings.Split(string(content), "\n")
	}

	// Check if graph path is already ignored
	graphPattern := graphPath
	if !strings.HasPrefix(graphPattern, "/") {
		graphPattern = "/" + graphPattern
	}
	graphPatternAlt := graphPath + "/"

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == graphPath || trimmed == graphPattern || trimmed == graphPatternAlt {
			return nil // Already ignored
		}
	}

	// Add graph path to .gitignore
	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// Add newline if file doesn't end with one
	if len(content) > 0 && content[len(content)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}

	// Add comment and pattern
	entry := fmt.Sprintf("\n# Generated graph visualizations\n%s/\n", graphPath)
	if _, err := f.WriteString(entry); err != nil {
		return err
	}

	return nil
}

// indexFile indexes a single markdown file
func indexFile(idx interface {
	Index(string, interface{}) error
}, filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	z, err := config.ParseFrontmatter(content)
	if err != nil {
		return err
	}

	body := extractBody(content)

	docKind := z.GetKind()
	docType := z.Type
	if docType == "" {
		docType = docKind // legacy notes: fall back to the structured kind
	}

	doc := &index.ConceptDoc{
		ID:          z.ID,
		Title:       z.Title,
		Type:        docType,
		Kind:        docKind,
		Description: z.Description,
		Project:     z.Project,
		Category:    z.Category,
		Tags:        z.Tags,
		Body:        body,
		FilePath:    filePath,
		Created:     z.Created.Format("2006-01-02T15:04:05Z07:00"),
		Timestamp:   z.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
		Status:      z.Status,
		Due:         z.Due,
		Completed:   z.Completed,
		Priority:    z.Priority,
	}

	return idx.Index(z.ID, doc)
}

// extractBody extracts the markdown body after frontmatter
func extractBody(content []byte) string {
	if !bytes.HasPrefix(content, []byte("---")) {
		return string(content)
	}

	rest := content[3:]
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	}

	endIdx := bytes.Index(rest, []byte("\n---"))
	if endIdx == -1 {
		return ""
	}

	body := rest[endIdx+4:]
	if len(body) > 0 && body[0] == '\n' {
		body = body[1:]
	}

	return string(body)
}

// slugify converts a title into a lowercase, ASCII, hyphen-separated slug
// suitable for a filename. Reserved OKF basenames are guarded.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevHyphen := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevHyphen = false
		case r == ' ' || r == '-' || r == '_' || r == '/' || r == '.':
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "note"
	}
	if slug == "index" || slug == "log" {
		slug += "-note" // never collide with reserved OKF filenames
	}
	return slug
}

// shortID returns the leading hex of a UUID/legacy id for filename disambiguation.
func shortID(id string, n int) string {
	hex := strings.ReplaceAll(id, "-", "")
	if n > len(hex) {
		n = len(hex)
	}
	return hex[:n]
}

// noteFilename builds a hybrid "slug--shortid.md" filename for title/id, unique
// within dir (the short id is extended on collision). Returns the base name only.
func noteFilename(dir, title, id string) string {
	slug := slugify(title)
	for n := 8; n <= 32; n += 2 {
		name := slug + "--" + shortID(id, n) + ".md"
		if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
			return name
		}
	}
	// Fallback: full compact id (effectively always unique).
	return slug + "--" + strings.ReplaceAll(id, "-", "") + ".md"
}

// sanitizeSubdir cleans a user/agent-requested bundle subdirectory and rejects
// any path that would escape the bundle root. Returns "" if invalid.
func sanitizeSubdir(p string) string {
	p = strings.TrimSpace(filepath.ToSlash(p))
	p = strings.TrimPrefix(p, "/")
	cleaned := filepath.ToSlash(filepath.Clean(p))
	if cleaned == "." || cleaned == "" {
		return ""
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return ""
	}
	return cleaned
}

// conceptDir resolves where a new concept should be written: an explicit --path
// (sanitized, in-bundle) takes precedence; otherwise the project name is used as
// a top-level folder; otherwise the bundle root. The untethered/tethered category
// is metadata only and never dictates the path.
func conceptDir(cfg *config.Config, pathFlag, project string) (string, error) {
	if pathFlag != "" {
		sub := sanitizeSubdir(pathFlag)
		if sub == "" {
			return "", fmt.Errorf("invalid --path %q: must be a subdirectory within the bundle", pathFlag)
		}
		return filepath.Join(cfg.RootPath, filepath.FromSlash(sub)), nil
	}
	if project != "" {
		if sub := sanitizeSubdir(project); sub != "" {
			return filepath.Join(cfg.RootPath, filepath.FromSlash(sub)), nil
		}
	}
	return cfg.RootPath, nil
}

// isReservedBase reports whether name is an OKF reserved filename (index.md/log.md).
func isReservedBase(name string) bool {
	return name == "index.md" || name == "log.md"
}

// relDir returns the directory portion of a slash-separated bundle-relative path.
func relDir(rel string) string {
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		return rel[:i]
	}
	return ""
}

type migrateItem struct {
	absPath string
	relPath string // bundle-relative, slash-separated
	id      string
	title   string
	newRel  string // planned bundle-relative path, slash-separated
}

// planName chooses the hybrid slug--shortid.md name for a concept, preserving an
// already-correct name (idempotent) and avoiding in-batch collisions via used.
func planName(dir, oldBase, title, id string, used map[string]bool) string {
	slug := slugify(title)
	key := func(name string) string { return dir + "|" + name }
	// Keep the current name if it is already a valid slug--shortid form for this id.
	for n := 8; n <= 32; n += 2 {
		cand := slug + "--" + shortID(id, n) + ".md"
		if cand == oldBase {
			used[key(cand)] = true
			return cand
		}
	}
	for n := 8; n <= 32; n += 2 {
		cand := slug + "--" + shortID(id, n) + ".md"
		if !used[key(cand)] {
			used[key(cand)] = true
			return cand
		}
	}
	cand := slug + "--" + strings.ReplaceAll(id, "-", "") + ".md"
	used[key(cand)] = true
	return cand
}

// gitMove renames a file with `git mv` (preserving history) when possible,
// falling back to a plain rename outside a git repo.
func gitMove(root, oldAbs, newAbs string) error {
	cmd := exec.Command("git", "-C", root, "mv", "-f", "--", oldAbs, newAbs)
	if err := cmd.Run(); err != nil {
		return os.Rename(oldAbs, newAbs)
	}
	return nil
}

// runValidateOKF checks a bundle for OKF conformance: every non-reserved .md has
// parseable frontmatter with a non-empty `type`. Unresolved links are reported as
// warnings (OKF tolerates broken links). Returns an error if any hard violation
// is found.
func runValidateOKF(cfg *config.Config, root string) error {
	var concepts, violations, brokenLinks int
	var problems []string

	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			if (strings.HasPrefix(fi.Name(), ".") && fi.Name() != ".") || fi.Name() == "ephemeral" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".md") || isReservedBase(fi.Name()) {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		content, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		z, err := config.ParseFrontmatter(content)
		if err != nil {
			violations++
			problems = append(problems, fmt.Sprintf("  ✗ %s: unparseable frontmatter (%v)", rel, err))
			return nil
		}
		concepts++
		if strings.TrimSpace(z.Type) == "" {
			violations++
			problems = append(problems, fmt.Sprintf("  ✗ %s: missing required non-empty `type`", rel))
		}
		// Warn on unresolved internal links.
		absP, _ := filepath.Abs(p)
		body := extractBody(content)
		fromDir := ""
		if rd, err := filepath.Rel(root, filepath.Dir(absP)); err == nil {
			fromDir = filepath.ToSlash(rd)
			if fromDir == "." {
				fromDir = ""
			}
		}
		for _, href := range graph.ExtractLinkTargets(body) {
			tr := graph.NormalizeTarget(href, fromDir)
			if tr == "" {
				continue
			}
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(tr))); err != nil {
				brokenLinks++
				problems = append(problems, fmt.Sprintf("  ⚠ %s: link target not found: %s", rel, href))
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Root index.md should declare okf_version.
	if data, err := os.ReadFile(filepath.Join(root, "index.md")); err == nil {
		if !strings.Contains(string(data), "okf_version") {
			problems = append(problems, "  ⚠ index.md: root index is missing an okf_version declaration")
		}
	}

	for _, pmsg := range problems {
		fmt.Println(pmsg)
	}
	fmt.Printf("\nOKF conformance: %d concept(s), %d violation(s), %d broken link(s).\n", concepts, violations, brokenLinks)
	if violations > 0 {
		return fmt.Errorf("%d conformance violation(s)", violations)
	}
	return nil
}

// runIndexMd generates an OKF index.md in each bundle directory (progressive
// disclosure). The root index.md carries the okf_version declaration; all other
// index files have no frontmatter.
func runIndexMd(cfg *config.Config, root string) error {
	type entry struct{ title, desc, href string }
	concepts := map[string][]entry{}
	dirs := map[string]bool{"": true}

	addDirChain := func(d string) {
		for {
			dirs[d] = true
			if d == "" {
				break
			}
			d = relDir(d)
		}
	}

	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			if (strings.HasPrefix(fi.Name(), ".") && fi.Name() != ".") || fi.Name() == "ephemeral" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".md") || isReservedBase(fi.Name()) {
			return nil
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		z, err := config.ParseFrontmatter(content)
		if err != nil || z.ID == "" {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		d := relDir(rel)
		title := z.Title
		if title == "" {
			title = fi.Name()
		}
		concepts[d] = append(concepts[d], entry{title, z.Description, fi.Name()})
		addDirChain(d)
		return nil
	})
	if err != nil {
		return err
	}

	// Immediate subdirectories per directory.
	subdirs := map[string][]string{}
	for d := range dirs {
		if d == "" {
			continue
		}
		parent := relDir(d)
		name := strings.TrimPrefix(d[len(parent):], "/")
		subdirs[parent] = append(subdirs[parent], name)
	}

	count := 0
	for d := range dirs {
		var b strings.Builder
		if d == "" {
			b.WriteString("---\nokf_version: \"0.1\"\n---\n\n")
		}
		heading := d
		if heading == "" {
			heading = "/"
		}
		b.WriteString("# " + heading + "\n\n")
		if subs := subdirs[d]; len(subs) > 0 {
			sort.Strings(subs)
			b.WriteString("## Subdirectories\n\n")
			for _, s := range subs {
				b.WriteString("* [" + s + "/](" + s + "/)\n")
			}
			b.WriteString("\n")
		}
		if cs := concepts[d]; len(cs) > 0 {
			sort.Slice(cs, func(i, j int) bool { return cs[i].title < cs[j].title })
			b.WriteString("## Concepts\n\n")
			for _, c := range cs {
				line := "* [" + c.title + "](" + c.href + ")"
				if c.desc != "" {
					line += " - " + c.desc
				}
				b.WriteString(line + "\n")
			}
			b.WriteString("\n")
		}
		out := filepath.Join(root, filepath.FromSlash(d), "index.md")
		if err := os.WriteFile(out, []byte(b.String()), 0644); err != nil {
			return err
		}
		count++
	}
	fmt.Printf("Wrote %d index.md file(s).\n", count)
	return nil
}

// runLog renders an OKF log.md at the bundle root from git history (date-grouped,
// newest first). Concept additions/modifications/deletions become
// Creation/Update/Deprecation entries.
func runLog(cfg *config.Config, root string) error {
	cmd := exec.Command("git", "-C", root, "log", "--date=short", "--name-status",
		"--pretty=format:\x01%ad\x02%s")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git log failed (is %s a git repository?): %w", root, err)
	}

	type event struct{ verb, path, subject string }
	byDate := map[string][]event{}
	var dateOrder []string
	seenDate := map[string]bool{}
	var curDate, curSubject string

	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "\x01") {
			parts := strings.SplitN(strings.TrimPrefix(line, "\x01"), "\x02", 2)
			curDate = parts[0]
			curSubject = ""
			if len(parts) > 1 {
				curSubject = parts[1]
			}
			if !seenDate[curDate] {
				seenDate[curDate] = true
				dateOrder = append(dateOrder, curDate)
			}
			continue
		}
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		status := fields[0]
		p := fields[len(fields)-1]
		if !strings.HasSuffix(p, ".md") || isReservedBase(filepath.Base(p)) {
			continue
		}
		verb := "Update"
		switch {
		case strings.HasPrefix(status, "A"):
			verb = "Creation"
		case strings.HasPrefix(status, "D"):
			verb = "Deprecation"
		}
		byDate[curDate] = append(byDate[curDate], event{verb, filepath.ToSlash(p), curSubject})
	}

	var b strings.Builder
	b.WriteString("# Update Log\n\n")
	for _, d := range dateOrder {
		evs := byDate[d]
		if len(evs) == 0 {
			continue
		}
		b.WriteString("## " + d + "\n\n")
		for _, e := range evs {
			b.WriteString("* **" + e.verb + "**: [" + e.path + "](/" + e.path + ")")
			if e.subject != "" {
				b.WriteString(" — " + e.subject)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	outPath := filepath.Join(root, "log.md")
	if err := os.WriteFile(outPath, []byte(b.String()), 0644); err != nil {
		return err
	}
	fmt.Printf("Wrote %s\n", outPath)
	return nil
}

// runMigrate converts a legacy bundle to OKF conventions: relocates ~/zk_vault,
// renames concepts to slug--shortid.md, rewrites [[id]] links to markdown links,
// and rebuilds the graph DB and search index. Idempotent; dryRun previews only.
func runMigrate(cfg *config.Config, root string, dryRun bool) error {
	// Step 0: relocate a legacy ~/zk_vault bundle to the configured root.
	if home, err := os.UserHomeDir(); err == nil {
		legacy := filepath.Join(home, "zk_vault")
		if legacy != root {
			if fi, statErr := os.Stat(legacy); statErr == nil && fi.IsDir() {
				if _, rootErr := os.Stat(root); os.IsNotExist(rootErr) {
					if dryRun {
						fmt.Printf("[dry-run] would relocate %s -> %s\n", legacy, root)
						root = legacy // preview against the legacy contents
					} else {
						if err := os.Rename(legacy, root); err != nil {
							return fmt.Errorf("relocate legacy bundle: %w", err)
						}
						fmt.Printf("Relocated %s -> %s\n", legacy, root)
					}
				}
			}
		}
	}
	if _, err := os.Stat(root); err != nil {
		return fmt.Errorf("bundle root not found: %s", root)
	}

	// Pass 1: collect concepts and plan new filenames.
	var items []migrateItem
	idToNewRel := map[string]string{}
	idToTitle := map[string]string{}
	used := map[string]bool{}

	walkConcepts := func(fn func(p string, fi os.FileInfo)) error {
		return filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if fi.IsDir() {
				if (strings.HasPrefix(fi.Name(), ".") && fi.Name() != ".") || fi.Name() == "ephemeral" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(p, ".md") && !isReservedBase(fi.Name()) {
				fn(p, fi)
			}
			return nil
		})
	}

	err := walkConcepts(func(p string, fi os.FileInfo) {
		content, err := os.ReadFile(p)
		if err != nil {
			return
		}
		z, err := config.ParseFrontmatter(content)
		if err != nil || z.ID == "" {
			return
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		dir := relDir(rel)
		base := planName(dir, fi.Name(), z.Title, z.ID, used)
		newRel := base
		if dir != "" {
			newRel = dir + "/" + base
		}
		items = append(items, migrateItem{absPath: p, relPath: rel, id: z.ID, title: z.Title, newRel: newRel})
		idToNewRel[z.ID] = newRel
		idToTitle[z.ID] = z.Title
	})
	if err != nil {
		return err
	}

	// Pass 2: rewrite links and rename files.
	renamed, rewrote := 0, 0
	for _, it := range items {
		content, err := os.ReadFile(it.absPath)
		if err != nil {
			continue
		}
		newContent := graph.TransformWikiLinks(string(content), func(id, display string) string {
			target, ok := idToNewRel[id]
			if !ok {
				return "" // leave links to unknown ids untouched
			}
			title := display
			if title == "" {
				if t := idToTitle[id]; t != "" {
					title = t
				} else {
					title = id
				}
			}
			return "[" + title + "](/" + target + ")"
		})
		changed := newContent != string(content)
		needRename := it.relPath != it.newRel

		if dryRun {
			if needRename {
				fmt.Printf("[dry-run] rename %s -> %s\n", it.relPath, it.newRel)
				renamed++
			}
			if changed {
				fmt.Printf("[dry-run] rewrite links in %s\n", it.newRel)
				rewrote++
			}
			continue
		}

		targetAbs := it.absPath
		if needRename {
			newAbs := filepath.Join(root, filepath.FromSlash(it.newRel))
			if err := os.MkdirAll(filepath.Dir(newAbs), 0755); err != nil {
				return err
			}
			if err := gitMove(root, it.absPath, newAbs); err != nil {
				return fmt.Errorf("rename %s: %w", it.relPath, err)
			}
			targetAbs = newAbs
			renamed++
		}
		if changed {
			if err := os.WriteFile(targetAbs, []byte(newContent), 0644); err != nil {
				return err
			}
			rewrote++
		}
	}

	if dryRun {
		fmt.Printf("\n[dry-run] %d file(s) would be renamed, %d would have links rewritten.\n", renamed, rewrote)
		return nil
	}

	// Rebuild the graph DB from scratch and reindex.
	_ = ensureGitignore(root, cfg.GraphPath)
	dbPath := filepath.Join(root, cfg.GraphPath)
	_ = os.Remove(dbPath)
	if store, err := graphdb.Open(dbPath); err == nil {
		if _, err := store.Sync(root); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: graph sync failed: %v\n", err)
		}
		store.Close()
	}
	if idx, err := index.OpenOrCreateIndex(cfg.IndexPath); err == nil {
		_ = walkConcepts(func(p string, fi os.FileInfo) { _ = indexFile(idx, p) })
		idx.Close()
	}

	fmt.Printf("Migration complete: %d file(s) renamed, %d had links rewritten.\n", renamed, rewrote)
	return nil
}

// touchTimestamp ensures the updates map bumps the OKF `timestamp` field to now
// (last meaningful change), unless the caller already set it explicitly.
func touchTimestamp(updates map[string]string) map[string]string {
	if updates == nil {
		updates = map[string]string{}
	}
	if _, ok := updates["timestamp"]; !ok {
		updates["timestamp"] = time.Now().Format(time.RFC3339)
	}
	return updates
}

// updateFrontmatter updates specific fields in YAML frontmatter
func updateFrontmatter(content []byte, updates map[string]string) ([]byte, error) {
	if !bytes.HasPrefix(content, []byte("---")) {
		return nil, fmt.Errorf("no frontmatter found")
	}
	updates = touchTimestamp(updates)

	rest := content[3:]
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	}

	endIdx := bytes.Index(rest, []byte("\n---"))
	if endIdx == -1 {
		return nil, fmt.Errorf("frontmatter closing not found")
	}

	frontmatter := rest[:endIdx]
	afterFrontmatter := rest[endIdx+4:]

	var newFrontmatter bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(frontmatter))
	updatedFields := make(map[string]bool)

	for scanner.Scan() {
		line := scanner.Text()
		updated := false

		for field, value := range updates {
			if strings.HasPrefix(line, field+":") {
				newFrontmatter.WriteString(fmt.Sprintf("%s: %q\n", field, value))
				updatedFields[field] = true
				updated = true
				break
			}
		}

		if !updated {
			newFrontmatter.WriteString(line + "\n")
		}
	}

	for field, value := range updates {
		if !updatedFields[field] {
			newFrontmatter.WriteString(fmt.Sprintf("%s: %q\n", field, value))
		}
	}

	var result bytes.Buffer
	result.WriteString("---\n")
	result.Write(newFrontmatter.Bytes())
	result.WriteString("---")
	result.Write(afterFrontmatter)

	return result.Bytes(), nil
}

// backlinksToJSON converts backlinks slice to JSON
func backlinksToJSON(backlinks interface{}) (string, error) {
	data, err := json.MarshalIndent(backlinks, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// createOrOpenDaily creates or opens a daily note for the given date
func createOrOpenDaily(cfg *config.Config, targetDate time.Time) error {
	id := newV7WithTime(targetDate)
	dateStr := targetDate.Format("2006-01-02")
	dayName := targetDate.Format("Monday")
	title := fmt.Sprintf("%s %s", dateStr, dayName)

	// Daily notes go into a "daily" subdirectory within untethered
	dailyDir := filepath.Join(cfg.RootPath, "daily",
		targetDate.Format("2006"), targetDate.Format("01"))

	// Check if any daily note already exists for this date
	entries, _ := filepath.Glob(filepath.Join(dailyDir, "*.md"))
	if len(entries) > 0 {
		fmt.Printf("Daily note: %s\n", entries[0])
		return nil
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(dailyDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	filePath := filepath.Join(dailyDir, noteFilename(dailyDir, title, id))

	// Get the daily template
	tmpl, err := templates.Get("daily")
	if err != nil {
		return fmt.Errorf("failed to get daily template: %w", err)
	}

	// Generate the note with the daily template
	content, err := tmpl.GenerateNote(id, title, "", nil)
	if err != nil {
		return fmt.Errorf("failed to generate daily note: %w", err)
	}

	// Write the file
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("Created daily note: %s\n", filePath)

	// Auto-index the new daily note
	if err := autoIndex(cfg, filePath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to index note: %v\n", err)
	}

	return nil
}

// listDailyNotes lists recent daily notes
func listDailyNotes(cfg *config.Config, weekOnly, monthOnly, asJSON bool) error {
	dailyDir := filepath.Join(cfg.RootPath, "daily")

	// Check if daily directory exists
	if _, err := os.Stat(dailyDir); os.IsNotExist(err) {
		if asJSON {
			fmt.Println("[]")
		} else {
			fmt.Println("No daily notes found")
		}
		return nil
	}

	// Calculate date range
	now := time.Now()
	var startDate time.Time
	if weekOnly {
		// Start of this week (Sunday)
		daysUntilSunday := int(now.Weekday())
		startDate = now.AddDate(0, 0, -daysUntilSunday)
		startDate = time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
	} else if monthOnly {
		// Start of this month
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	} else {
		// Default: last 14 days
		startDate = now.AddDate(0, 0, -14)
	}

	type DailyNote struct {
		Date     string `json:"date"`
		Title    string `json:"title"`
		FilePath string `json:"file_path"`
	}

	var notes []DailyNote

	// Walk the daily directory
	err := filepath.Walk(dailyDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		// Parse date from filename (UUIDv7 or legacy YYYYMMDDHHmmss-UUID format)
		baseName := strings.TrimSuffix(filepath.Base(path), ".md")
		fileDate, err := timeFromID(baseName)
		if err != nil {
			return nil
		}

		// Check if within date range
		if fileDate.Before(startDate) {
			return nil
		}

		// Read title from file
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		z, err := config.ParseFrontmatter(content)
		dateStr := fileDate.Format("2006-01-02")
		title := dateStr + " " + fileDate.Format("Monday")
		if err == nil && z.Title != "" {
			title = z.Title
		}

		absPath, _ := filepath.Abs(path)
		notes = append(notes, DailyNote{
			Date:     dateStr,
			Title:    title,
			FilePath: absPath,
		})

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to list daily notes: %w", err)
	}

	// Sort by date (newest first)
	sort.Slice(notes, func(i, j int) bool {
		return notes[i].Date > notes[j].Date
	})

	if asJSON {
		data, err := json.MarshalIndent(notes, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to format JSON: %w", err)
		}
		fmt.Println(string(data))
	} else {
		if len(notes) == 0 {
			fmt.Println("No daily notes found in the specified range")
			return nil
		}

		rangeName := "recent"
		if weekOnly {
			rangeName = "this week"
		} else if monthOnly {
			rangeName = "this month"
		}
		fmt.Printf("Daily notes (%s):\n\n", rangeName)

		for _, note := range notes {
			fmt.Printf("  %s: %s\n", note.Date, note.Title)
		}
		fmt.Printf("\nFound %d daily note(s)\n", len(notes))
	}

	return nil
}

// autoIndex indexes a single file silently (for auto-indexing on create/update)
func autoIndex(cfg *config.Config, filePath string) error {
	idx, err := index.OpenOrCreateIndex(cfg.IndexPath)
	if err != nil {
		return err
	}
	defer idx.Close()

	return indexFile(idx, filePath)
}

// generateBasicNote creates a basic note without a template
func generateBasicNote(id, title, project, category string, tags []string) string {
	now := time.Now()

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.WriteString(fmt.Sprintf("id: %q\n", id))
	buf.WriteString(fmt.Sprintf("title: %q\n", title))
	buf.WriteString("type: \"note\"\n")

	if project != "" {
		buf.WriteString(fmt.Sprintf("project: %q\n", project))
	}

	buf.WriteString(fmt.Sprintf("category: %q\n", category))

	buf.WriteString("tags:")
	if len(tags) > 0 {
		buf.WriteString("\n")
		for _, tag := range tags {
			buf.WriteString(fmt.Sprintf("  - %q\n", tag))
		}
	} else {
		buf.WriteString(" []\n")
	}

	stamp := now.Format(time.RFC3339)
	buf.WriteString(fmt.Sprintf("created: %q\n", stamp))
	buf.WriteString(fmt.Sprintf("timestamp: %q\n", stamp))
	buf.WriteString("---\n\n")
	buf.WriteString(fmt.Sprintf("# %s\n\n", title))

	return buf.String()
}

// resolveConceptPath resolves a concept ID or file path to an absolute file path
func resolveConceptPath(cfg *config.Config, idOrPath string) (string, error) {
	// If it looks like a file path, use it directly
	if strings.HasSuffix(idOrPath, ".md") || strings.Contains(idOrPath, "/") {
		if _, err := os.Stat(idOrPath); err != nil {
			return "", fmt.Errorf("file not found: %s", idOrPath)
		}
		return filepath.Abs(idOrPath)
	}

	// Otherwise it is an ID: search the whole bundle for a concept whose
	// frontmatter id matches. Concepts may live in any directory (the
	// untethered/tethered layout is not assumed), and filenames are
	// slug--shortid.md rather than <id>.md.
	var found string
	err := filepath.Walk(cfg.RootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if (strings.HasPrefix(info.Name(), ".") && info.Name() != ".") || info.Name() == "ephemeral" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".md") || isReservedBase(info.Name()) {
			return nil
		}
		// Fast path: a legacy <id>.md filename.
		if info.Name() == idOrPath+".md" {
			found = path
			return filepath.SkipAll
		}
		if config.ReadConceptID(path) == idOrPath {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && err != filepath.SkipAll {
		return "", fmt.Errorf("error searching for concept: %w", err)
	}

	if found != "" {
		return filepath.Abs(found)
	}

	return "", fmt.Errorf("concept not found: %s", idOrPath)
}

// updateFrontmatterRemove updates fields and removes specified fields from frontmatter
func updateFrontmatterRemove(content []byte, updates map[string]string, remove []string) ([]byte, error) {
	if !bytes.HasPrefix(content, []byte("---")) {
		return nil, fmt.Errorf("no frontmatter found")
	}
	updates = touchTimestamp(updates)

	rest := content[3:]
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	}

	endIdx := bytes.Index(rest, []byte("\n---"))
	if endIdx == -1 {
		return nil, fmt.Errorf("frontmatter closing not found")
	}

	frontmatter := rest[:endIdx]
	afterFrontmatter := rest[endIdx+4:]

	// Convert remove slice to set for easy lookup
	removeSet := make(map[string]bool)
	for _, field := range remove {
		removeSet[field] = true
	}

	var newFrontmatter bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(frontmatter))
	updatedFields := make(map[string]bool)

	for scanner.Scan() {
		line := scanner.Text()
		skip := false

		// Check if this line should be removed
		for field := range removeSet {
			if strings.HasPrefix(line, field+":") {
				skip = true
				break
			}
		}

		if skip {
			continue
		}

		updated := false
		for field, value := range updates {
			if strings.HasPrefix(line, field+":") {
				newFrontmatter.WriteString(fmt.Sprintf("%s: %q\n", field, value))
				updatedFields[field] = true
				updated = true
				break
			}
		}

		if !updated {
			newFrontmatter.WriteString(line + "\n")
		}
	}

	// Add any fields that weren't already present
	for field, value := range updates {
		if !updatedFields[field] {
			newFrontmatter.WriteString(fmt.Sprintf("%s: %q\n", field, value))
		}
	}

	var result bytes.Buffer
	result.WriteString("---\n")
	result.Write(newFrontmatter.Bytes())
	result.WriteString("---")
	result.Write(afterFrontmatter)

	return result.Bytes(), nil
}

// addFrontmatterTags appends new tags to the tags: YAML list in frontmatter
func addFrontmatterTags(content []byte, tags []string) ([]byte, error) {
	if !bytes.HasPrefix(content, []byte("---")) {
		return nil, fmt.Errorf("no frontmatter found")
	}

	rest := content[3:]
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	}

	endIdx := bytes.Index(rest, []byte("\n---"))
	if endIdx == -1 {
		return nil, fmt.Errorf("frontmatter closing not found")
	}

	frontmatter := rest[:endIdx]
	afterFrontmatter := rest[endIdx+4:]

	var newFrontmatter bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(frontmatter))
	foundTags := false
	inTagsList := false

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "tags:") {
			foundTags = true
			// Check for inline empty array: "tags: []"
			trimmed := strings.TrimSpace(strings.TrimPrefix(line, "tags:"))
			if trimmed == "[]" {
				// Replace inline empty array with multiline list header
				newFrontmatter.WriteString("tags:\n")
				for _, tag := range tags {
					newFrontmatter.WriteString(fmt.Sprintf("  - %q\n", tag))
				}
				// Don't enter inTagsList mode — tags are already written
				continue
			}
			newFrontmatter.WriteString(line + "\n")
			inTagsList = true
			continue
		}

		// If we're in the tags list, look for list items (lines starting with "  - ")
		if inTagsList {
			if strings.HasPrefix(strings.TrimSpace(line), "- ") {
				newFrontmatter.WriteString(line + "\n")
				continue
			}
			// End of tags list - append new tags before this line
			for _, tag := range tags {
				newFrontmatter.WriteString(fmt.Sprintf("  - %q\n", tag))
			}
			inTagsList = false
		}

		newFrontmatter.WriteString(line + "\n")
	}

	// If tags list was the last section in frontmatter
	if inTagsList {
		for _, tag := range tags {
			newFrontmatter.WriteString(fmt.Sprintf("  - %q\n", tag))
		}
	}

	// If no tags: field existed, add one
	if !foundTags {
		newFrontmatter.WriteString("tags:\n")
		for _, tag := range tags {
			newFrontmatter.WriteString(fmt.Sprintf("  - %q\n", tag))
		}
	}

	var result bytes.Buffer
	result.WriteString("---\n")
	result.Write(newFrontmatter.Bytes())
	result.WriteString("---")
	result.Write(afterFrontmatter)

	return result.Bytes(), nil
}

// generateTodoListMarkdown generates a markdown todo list from search results
func generateTodoListMarkdown(results []index.SearchResult, project string, today, thisWeek bool) string {
	var buf bytes.Buffer

	// Title
	title := "Todo List"
	if project != "" {
		title = fmt.Sprintf("Todo List: %s", project)
	} else if today {
		title = fmt.Sprintf("Todos Due: %s", time.Now().Format("2006-01-02"))
	} else if thisWeek {
		title = "Todos Due This Week"
	}

	buf.WriteString(fmt.Sprintf("# %s\n\n", title))
	buf.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().Format("2006-01-02 15:04")))

	if len(results) == 0 {
		buf.WriteString("No todos found.\n")
		return buf.String()
	}

	// Group by priority
	high := make([]index.SearchResult, 0)
	medium := make([]index.SearchResult, 0)
	low := make([]index.SearchResult, 0)
	noPriority := make([]index.SearchResult, 0)

	for _, r := range results {
		switch r.Priority {
		case "high":
			high = append(high, r)
		case "medium":
			medium = append(medium, r)
		case "low":
			low = append(low, r)
		default:
			noPriority = append(noPriority, r)
		}
	}

	writeTodoGroup := func(title string, todos []index.SearchResult) {
		if len(todos) == 0 {
			return
		}
		buf.WriteString(fmt.Sprintf("## %s\n\n", title))
		for _, todo := range todos {
			checkbox := "[ ]"
			if todo.Status == "closed" {
				checkbox = "[x]"
			} else if todo.Status == "in_progress" {
				checkbox = "[~]"
			}
			buf.WriteString(fmt.Sprintf("- %s **%s** [[%s]]\n", checkbox, todo.Title, todo.ID))
			if todo.Due != "" {
				buf.WriteString(fmt.Sprintf("  - Due: %s\n", todo.Due))
			}
			if todo.Project != "" {
				buf.WriteString(fmt.Sprintf("  - Project: %s\n", todo.Project))
			}
		}
		buf.WriteString("\n")
	}

	writeTodoGroup("High Priority", high)
	writeTodoGroup("Medium Priority", medium)
	writeTodoGroup("Low Priority", low)
	writeTodoGroup("Other", noPriority)

	buf.WriteString(fmt.Sprintf("---\n\nTotal: %d todos\n", len(results)))

	return buf.String()
}

// Git helper functions for hello/goodbye commands

// isGitRepo checks if the given path is inside a git repository
func isGitRepo(path string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = path
	return cmd.Run() == nil
}

// hasUncommittedChanges checks if there are uncommitted changes
func hasUncommittedChanges(path string) (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(string(output))) > 0, nil
}

// branchExists checks if a branch exists
func branchExists(path, branch string) (bool, error) {
	cmd := exec.Command("git", "rev-parse", "--verify", branch)
	cmd.Dir = path
	err := cmd.Run()
	if err != nil {
		// Branch doesn't exist
		return false, nil
	}
	return true, nil
}

// getCurrentBranch returns the current branch name
func getCurrentBranch(path string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// isDateBranch checks if a branch name matches YYYYMMDD format
func isDateBranch(branch string) bool {
	if len(branch) != 8 {
		return false
	}
	_, err := time.Parse("20060102", branch)
	return err == nil
}

// gitCheckout switches to an existing branch
func gitCheckout(path, branch string) error {
	cmd := exec.Command("git", "checkout", branch)
	cmd.Dir = path
	return cmd.Run()
}

// gitCheckoutNewBranch creates and switches to a new branch
func gitCheckoutNewBranch(path, branch string) error {
	cmd := exec.Command("git", "checkout", "-b", branch)
	cmd.Dir = path
	return cmd.Run()
}

// gitPull pulls latest changes from remote
func gitPull(path string) error {
	cmd := exec.Command("git", "pull")
	cmd.Dir = path
	return cmd.Run()
}

// gitAddAll stages all changes
func gitAddAll(path string) error {
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = path
	return cmd.Run()
}

// gitCommit creates a commit with the given message
func gitCommit(path, message string) error {
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = path
	return cmd.Run()
}

// gitMerge merges a branch into the current branch
func gitMerge(path, branch string) error {
	cmd := exec.Command("git", "merge", branch, "--no-edit")
	cmd.Dir = path
	return cmd.Run()
}

// gitDeleteBranch deletes a local branch
func gitDeleteBranch(path, branch string) error {
	cmd := exec.Command("git", "branch", "-d", branch)
	cmd.Dir = path
	return cmd.Run()
}

// newV7WithTime constructs a UUIDv7 with a specific timestamp instead of time.Now().
func newV7WithTime(t time.Time) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	ms := t.UnixMilli()
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	b[6] = (b[6] & 0x0F) | 0x70 // version 7
	b[8] = (b[8] & 0x3F) | 0x80 // variant 10
	return uuid.UUID(b).String()
}

// timeFromID extracts creation time from either ID format:
//   - Legacy: "YYYYMMDDHHmmss-UUID" → parse 14-char prefix
//   - UUIDv7: parse first 48 bits as Unix milliseconds
func timeFromID(id string) (time.Time, error) {
	if len(id) > 36 && id[14] == '-' {
		t, err := time.Parse("20060102150405", id[:14])
		if err == nil {
			return t, nil
		}
	}
	u, err := uuid.Parse(id)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid ID: %s", id)
	}
	ms := int64(u[0])<<40 | int64(u[1])<<32 | int64(u[2])<<24 | int64(u[3])<<16 | int64(u[4])<<8 | int64(u[5])
	return time.UnixMilli(ms), nil
}
