package main

import (
	"encoding/json"
	"fmt"
	"okfw/cli/internal/bundle"
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

	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

// rootCmd builds the whole command tree.
//
// Extracted from main so tests can enumerate the registered commands. graphy
// has had this shape from the start, which is why its skill could be checked
// against the binary and okfw's could not.
func rootCmd() *cobra.Command {
	var projectFlag string
	var tagsFlag []string
	var limitFlag int
	var offsetFlag int
	var jsonFlag bool
	var graphLimitFlag int
	var templateFlag string
	var descriptionFlag string
	var pathFlag string
	var statusFlag string

	var rootCmd = &cobra.Command{
		Use:          "okfw",
		SilenceUsage: true,
		Short:        "Manage a per-project OKF wiki of linked markdown concepts",
		Long: `okfw manages a local-first knowledge base — an Open Knowledge Format (OKF)
wiki of markdown concepts stored in a .okfw/ directory at your project root.

Run 'okfw init' once per project. Every other command then finds the wiki
automatically by searching upward from the working directory, so okfw works
from any subdirectory. Zero configuration: the search index, graph database,
and index.md listings are maintained automatically inside .okfw/.

For scripts and agents: read commands support --json for structured output,
'okfw create --json' returns the new note's {"id","path"}, and plain create
prints a stable "Created: <path>" line. 'okfw mcp' serves the same operations
as an MCP stdio server.`,
	}

	var createJSONFlag bool
	var bodyFlag string
	var createCmd = &cobra.Command{
		Use:   "create [title]",
		Short: "Create a new concept",
		Long: `Create a new concept with optional template.

Output: prints "Created: <absolute path>" on success (stable, parseable), or
use --json to get {"id": "...", "path": "..."} on stdout instead.

Examples:
  okfw create "My Note"                                      # Basic note
  okfw create "Meeting Notes" --template meeting             # From template
  okfw create "Feature Spec" -T feature -p myproj            # Template with project
  okfw create "Scripted" --json                              # Machine-readable result`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Discover("")
			if err != nil {
				return err
			}

			filePath, id, err := bundle.CreateConcept(cfg, bundle.Spec{
				Title:       args[0],
				Project:     projectFlag,
				Dir:         pathFlag,
				Tags:        tagsFlag,
				Description: descriptionFlag,
				Template:    templateFlag,
				Body:        bodyFlag,
			})
			if err != nil {
				return err
			}

			if createJSONFlag {
				out, _ := json.Marshal(map[string]string{"id": id, "path": filePath})
				fmt.Println(string(out))
			} else {
				fmt.Printf("Created: %s\n", filePath)
			}

			// Auto-index the new note
			if err := bundle.AutoSync(cfg, filePath); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to index note: %v\n", err)
			}

			return nil
		},
	}

	var rootPathCmd = &cobra.Command{
		Use:   "root",
		Short: "Print the project's OKF wiki root path",
		Long: `Print the absolute path of the project's .okfw directory, discovered
by searching upward from the current directory. Exits non-zero when no wiki
is found.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Discover("")
			if err != nil {
				return err
			}
			fmt.Println(cfg.RootPath)
			return nil
		},
	}

	var initCmd = &cobra.Command{
		Use:   "init",
		Short: "Initialize an OKF wiki (.okfw/) for the current project",
		Long: `Create a .okfw/ directory at the project root: the git repository root
when inside a repo, otherwise the current directory.

All other commands find the wiki automatically by searching upward from the
working directory. Generated artifacts (search index, graph database, todo
lists, exports) are gitignored via .okfw/.gitignore; concept markdown is
meant to be committed with the project.

Running init on an already-initialized project is a no-op.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			target := gitctx.GetGitRoot(cwd)
			if target == "" {
				target = cwd
			}
			return runInit(target)
		},
	}

	var connectCmd = &cobra.Command{
		Use:   "connect",
		Short: "Verify and report the project's OKF wiki",
		Long: `Locate the project's .okfw/ directory, searching upward from the current
directory, and report what was found. Exits non-zero with advice when no
wiki exists.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Discover("")
			if err != nil {
				return err
			}

			concepts := 0
			_ = filepath.Walk(cfg.RootPath, func(p string, fi os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				if fi.IsDir() {
					if bundle.SkipWalkDir(cfg.RootPath, p, fi.Name()) {
						return filepath.SkipDir
					}
					return nil
				}
				if strings.HasSuffix(p, ".md") && !bundle.IsReservedBase(fi.Name()) {
					concepts++
				}
				return nil
			})

			indexed := "no"
			if _, err := os.Stat(cfg.IndexPath); err == nil {
				indexed = "yes"
			}
			graphed := "no"
			if _, err := os.Stat(filepath.Join(cfg.RootPath, cfg.GraphPath)); err == nil {
				graphed = "yes"
			}

			fmt.Printf("Connected: %s\n", cfg.RootPath)
			fmt.Printf("  concepts: %d | search index: %s | graph db: %s\n", concepts, indexed, graphed)
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
			cfg, err := config.Discover("")
			if err != nil {
				return err
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
			cfg, err := config.Discover("")
			if err != nil {
				return err
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
				fmt.Printf("  %-15s %s\n", name, tmpl.Description)
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
			cfg, err := config.Discover("")
			if err != nil {
				return err
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
					if fi.IsDir() && bundle.SkipWalkDir(path, p, fi.Name()) {
						return filepath.SkipDir
					}
					if !fi.IsDir() && strings.HasSuffix(p, ".md") && !bundle.IsReservedBase(fi.Name()) {
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
				if err := bundle.IndexFile(idx, filePath); err != nil {
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
	var staleFlag bool
	var nodeIDFlag string

	var searchCmd = &cobra.Command{
		Use:   "search [query]",
		Short: "Search concepts",
		Long: `Search concepts by full-text query and/or field filters.

Examples:
  okfw search "authentication"              # Full-text search
  okfw search --project myproject           # Filter by project
  okfw search --tag golang --tag api        # Filter by tags (AND)
  okfw search --kind note                   # Hand-written concepts only
  okfw search --kind code                   # graphy's exported code graph only
  okfw search --type code-rationale         # Filter by OKF free-form type
  okfw search --status draft                # Filter by lifecycle status
  okfw search --stale                       # Concepts past their stale_after
  okfw search "auth" --project myproject    # Combined search
  okfw search --node-id go:function:a.go#F  # The concept documenting a graphy node`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Discover("")
			if err != nil {
				return err
			}

			idx, err := index.OpenOrCreateIndex(cfg.IndexPath)
			if err != nil {
				return fmt.Errorf("failed to open index: %w", err)
			}
			defer idx.Close()

			opts := index.SearchOptions{
				Project:      projectFlag,
				Type:         typeFlag,
				Kind:         kindFlag,
				Tags:         tagsFlag,
				Limit:        limitFlag,
				Offset:       offsetFlag,
				Status:       statusFlag,
				Stale:        staleFlag,
				GraphyNodeID: nodeIDFlag,
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
				fmt.Print(index.FormatSearchResultsFrom(results, total, offsetFlag))
			}

			return nil
		},
	}

	var graphStartFlag string
	var graphDepthFlag int
	var graphSyncFlag bool
	var graphASCIIFlag bool

	var graphCmd = &cobra.Command{
		Use:   "graph [path]",
		Short: "Render concept relationships as a Mermaid diagram (or an ASCII tree)",
		Long: `Print the relationships between concepts.

Output is Mermaid flowchart syntax by default, suitable for pasting into
markdown. Use --ascii for a plain-text tree that reads directly in a terminal.

The diagram goes to stdout so it can be piped; a summary line
(N nodes, M edges) goes to stderr so it does not pollute piped output.

If no path is given, the discovered wiki root is used.
When a graph database (.graph.db) exists, it is used for fast queries.
Use --sync to force a fresh sync before querying.

Examples:
  okfw graph                             # Mermaid diagram from the wiki root
  okfw graph --ascii                     # Plain-text tree instead
  okfw graph path/to/dir                 # All concepts in a directory
  okfw graph --start 20260213-abc-123    # Centered on a specific concept
  okfw graph --depth 3                   # Traverse up to 3 hops from start
  okfw graph --limit 20                  # Show up to 20 nodes (default: 10)
  okfw graph --sync                      # Sync graph DB before querying`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Discover("")
			if err != nil {
				return err
			}

			path := cfg.RootPath
			if len(args) > 0 {
				path = args[0]
			}

			// Try to use persistent graph store.
			dbPath := filepath.Join(cfg.RootPath, cfg.GraphPath)
			_, dbErr := os.Stat(dbPath)
			useStore := dbErr == nil || graphSyncFlag

			var nodes []*graph.Node
			var edges []graph.Edge
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

				nodes, edges, err = queryStore(store, graphStartFlag, graphLimitFlag, graphDepthFlag)
				if err != nil {
					return err
				}
			} else {
				g, err := buildGraphFromPath(path, cfg.RootPath)
				if err != nil {
					return err
				}
				nodes, err = queryGraph(g, graphStartFlag, graphLimitFlag, graphDepthFlag)
				if err != nil {
					return err
				}
				edges = g.GetEdges(nodes)
			}

			if graphASCIIFlag {
				fmt.Println(graph.GenerateASCIITree(nodes, edges, graphStartFlag))
			} else {
				fmt.Println(graph.GenerateMermaid(nodes, edges))
			}
			fmt.Fprintf(os.Stderr, "%d nodes, %d edges\n", len(nodes), len(edges))
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

			newContent, err := bundle.UpdateFrontmatter(content, map[string]string{
				"project": project,
			})
			if err != nil {
				return fmt.Errorf("failed to update frontmatter: %w", err)
			}

			if err := os.WriteFile(filePath, newContent, 0644); err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}

			fmt.Printf("Updated project to: %s\n", project)

			// Auto-index the updated note (best-effort)
			cfg, cfgErr := config.Discover("")
			if cfgErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: config load failed, skipping index update: %v\n", cfgErr)
			} else {
				if err := bundle.AutoSync(cfg, filePath); err != nil {
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
			cfg, err := config.Discover("")
			if err != nil {
				return err
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
					if fi.IsDir() && bundle.SkipWalkDir(searchPath, p, fi.Name()) {
						return filepath.SkipDir
					}
					if !fi.IsDir() && strings.HasSuffix(p, ".md") && !bundle.IsReservedBase(fi.Name()) {
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

					body := config.ExtractBody(content)
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
						fmt.Printf("  %s: %s\n", bl.ID, bl.Title)
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
  okfw export --limit 5                   # Export 5 closest notes
  okfw export --start 20260213-abc-123    # Export from a specific concept
  okfw export --depth 2                   # Export up to 2 hops from start`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Discover("")
			if err != nil {
				return err
			}

			path := cfg.RootPath
			if len(args) > 0 {
				path = args[0]
			}

			// Use the persistent graph store when it exists (indexed backlink
			// lookups); otherwise fall back to a filesystem walk.
			var nodes []*graph.Node
			dbPath := filepath.Join(cfg.RootPath, cfg.GraphPath)
			if _, dbErr := os.Stat(dbPath); dbErr == nil {
				store, err := graphdb.Open(dbPath)
				if err != nil {
					return err
				}
				defer store.Close()
				nodes, _, err = queryStore(store, exportStartFlag, exportLimitFlag, exportDepthFlag)
				if err != nil {
					return err
				}
			} else {
				g, err := buildGraphFromPath(path, cfg.RootPath)
				if err != nil {
					return err
				}
				nodes, err = queryGraph(g, exportStartFlag, exportLimitFlag, exportDepthFlag)
				if err != nil {
					return err
				}
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
	createCmd.Flags().StringVarP(&projectFlag, "project", "p", "", "Optional project metadata (also used as the note's folder)")
	createCmd.Flags().StringVarP(&templateFlag, "template", "T", "", "Use a template (see 'okfw templates' for list)")
	createCmd.Flags().StringVarP(&descriptionFlag, "description", "d", "", "One-sentence summary (OKF description)")
	createCmd.Flags().StringVar(&pathFlag, "path", "", "Place the concept in a bundle subdirectory (e.g. projects/okf)")
	createCmd.Flags().StringSliceVar(&tagsFlag, "tags", nil, "Additional tags (comma-separated)")
	createCmd.Flags().BoolVar(&createJSONFlag, "json", false, "Print {\"id\",\"path\"} as JSON instead of prose")
	createCmd.Flags().StringVar(&bodyFlag, "body", "", "Markdown body content for the note")

	searchCmd.Flags().StringVarP(&projectFlag, "project", "p", "", "Filter by project")
	searchCmd.Flags().StringVarP(&typeFlag, "type", "t", "", "Filter by OKF type (free-form)")
	searchCmd.Flags().StringVar(&kindFlag, "kind", "", "Filter by structured kind (note/daily-note/issue/code)")
	// Offset has been honoured by index.Search since it was written; nothing
	// ever set it, so pagination was unreachable and the only way past the
	// limit was to raise it.
	searchCmd.Flags().IntVar(&offsetFlag, "offset", 0, "Skip this many results (pagination)")
	searchCmd.Flags().StringSliceVarP(&tagsFlag, "tag", "T", nil, "Filter by tag (can be repeated)")
	searchCmd.Flags().StringVar(&statusFlag, "status", "", "Filter by OKF lifecycle status (draft/stable/deprecated)")
	searchCmd.Flags().BoolVar(&staleFlag, "stale", false, "Only concepts whose stale_after date has passed")
	// The reverse of `graphy concept`: cross from a code-graph node id to the
	// concept documenting it. The two tools otherwise share no identifier.
	searchCmd.Flags().StringVar(&nodeIDFlag, "node-id", "", "Find the concept documenting this graphy code-graph node id")
	searchCmd.Flags().IntVarP(&limitFlag, "limit", "l", 20, "Maximum number of results")
	searchCmd.Flags().BoolVar(&jsonFlag, "json", false, "Output results as JSON")

	graphCmd.Flags().IntVarP(&graphLimitFlag, "limit", "l", 10, "Maximum number of nodes to display")
	graphCmd.Flags().StringVar(&graphStartFlag, "start", "", "Start graph from a specific concept ID")
	graphCmd.Flags().IntVar(&graphDepthFlag, "depth", 0, "Maximum BFS depth (hops) from start node (0 = unlimited)")
	graphCmd.Flags().BoolVar(&graphASCIIFlag, "ascii", false, "Render a plain-text tree instead of Mermaid")
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
			cfg, err := config.Discover("")
			if err != nil {
				return err
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
			path, created, err := createOrOpenDaily(cfg, targetDate)
			if err != nil {
				return err
			}
			if created {
				fmt.Printf("Created daily note: %s\n", path)
			} else {
				fmt.Printf("Daily note: %s\n", path)
			}
			return nil
		},
	}

	dailyCmd.Flags().StringVar(&dateFlag, "date", "", "Target date (YYYY-MM-DD format)")
	dailyCmd.Flags().BoolVar(&yesterdayFlag, "yesterday", false, "Use yesterday's date")
	dailyCmd.Flags().BoolVar(&listFlag, "list", false, "List recent daily notes")
	dailyCmd.Flags().BoolVar(&weekFlag, "week", false, "Show this week's notes (with --list)")
	dailyCmd.Flags().BoolVar(&monthFlag, "month", false, "Show this month's notes (with --list)")
	dailyCmd.Flags().BoolVar(&jsonFlag, "json", false, "Output as JSON (with --list)")

	// Todo-specific flags
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
				cfg, err := config.Discover("")
				if err != nil {
					return err
				}
				filePath, err = bundle.ResolveConceptPath(cfg, args[0])
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

			newContent, err := bundle.AddFrontmatterTags(content, added)
			if err != nil {
				return fmt.Errorf("failed to update frontmatter: %w", err)
			}

			// Bump the OKF timestamp (last meaningful change).
			newContent, err = bundle.UpdateFrontmatter(newContent, nil)
			if err != nil {
				return fmt.Errorf("failed to update timestamp: %w", err)
			}

			if err := os.WriteFile(filePath, newContent, 0644); err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}

			fmt.Printf("Added tags: %s\n", strings.Join(added, ", "))

			// Auto-index (best-effort)
			cfg, cfgErr := config.Discover("")
			if cfgErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: config load failed, skipping index update: %v\n", cfgErr)
			} else {
				if err := bundle.AutoSync(cfg, filePath); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to index: %v\n", err)
				}
			}

			return nil
		},
	}

	var migrateDryRun bool
	var migrateCmd = &cobra.Command{
		Use:   "migrate [path]",
		Short: "Bring older concepts up to the current frontmatter shape",
		Long: `Rewrite concepts written by an earlier okfw so they satisfy the current
schema. Safe to re-run: a second pass reports nothing to do.

okfw removed its task-tracking fields, because OKF v0.2 spec'd 'status' as the
lifecycle field (draft/stable/deprecated) and okfw was using it for todo
progress. This drops 'due', 'completed', 'priority' and 'kind: todo', and
rewrites a legacy 'status':

  status: closed              ->  status: deprecated
  status: open|in_progress    ->  removed (absent means stable)

The free-form OKF 'type' is left alone: the spec does not register type values,
so an existing type: "todo" stays until you change it.

Examples:
  okfw migrate --dry-run     # report what would change
  okfw migrate               # apply`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Discover("")
			if err != nil {
				return err
			}
			root := cfg.RootPath
			if len(args) > 0 {
				root = args[0]
			}
			return runMigrate(cfg, root, migrateDryRun)
		},
	}
	migrateCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "Report what would change without writing")

	var validateOKF bool
	var validateAll bool
	var validateCmd = &cobra.Command{
		Use:   "validate [id_or_file_or_path]",
		Short: "Validate a concept's frontmatter, or a whole bundle",
		Long: `Validate concept frontmatter. There are two different checks, because
okfw's schema is deliberately stricter than OKF requires.

  (default)  Strict okfw validation of one concept: the full CUE #Concept
             schema — required id/title/type/tags/created/timestamp, and no
             undeclared fields.
  --all      The same strict check over every concept in the bundle.
  --okf      OKF conformance only (spec section 11): every non-reserved .md has
             parseable frontmatter with a non-empty 'type'. Permissive by
             design, so a bundle okfw did not author can still pass.

Examples:
  okfw validate ./path/to/note.md
  okfw validate 202602131045
  okfw validate --all                 # strict check of every concept
  okfw validate --okf                 # OKF conformance of the bundle root
  okfw validate --okf path/to/dir     # OKF conformance of a path`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if validateOKF && validateAll {
				return fmt.Errorf("--okf and --all are different checks; run them separately")
			}
			if validateOKF || validateAll {
				cfg, err := config.Discover("")
				if err != nil {
					return err
				}
				root := cfg.RootPath
				if len(args) > 0 {
					root = args[0]
				}
				if validateAll {
					return runValidateAll(cfg, root)
				}
				return runValidateOKF(cfg, root)
			}

			if len(args) == 0 {
				return fmt.Errorf("a concept id or file is required (or use --all / --okf to check the bundle)")
			}
			filePath := args[0]

			// If it looks like an ID (not a file path), resolve it
			if !strings.HasSuffix(filePath, ".md") && !strings.Contains(filePath, "/") {
				cfg, err := config.Discover("")
				if err != nil {
					return err
				}
				filePath, err = bundle.ResolveConceptPath(cfg, args[0])
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
  okfw graph-sync path/to/dir  # Sync from explicit path
  okfw graph-sync --validate         # Sync and validate edge integrity`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Discover("")
			if err != nil {
				return err
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

			// The graph purges deleted concepts; the search index has no
			// equivalent, so prune it here or a removed concept keeps turning
			// up in `okfw search` forever.
			pruned, err := bundle.PruneIndex(cfg)
			if err != nil {
				return fmt.Errorf("failed to prune the search index: %w", err)
			}
			if pruned > 0 {
				fmt.Printf("Search index pruned: %d stale entr%s removed\n",
					pruned, map[bool]string{true: "y", false: "ies"}[pruned == 1])
			}

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
			cfg, err := config.Discover("")
			if err != nil {
				return err
			}

			dbPath := filepath.Join(cfg.RootPath, cfg.GraphPath)
			store, err := graphdb.OpenExisting(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()

			nodes, err := store.GetBacklinks(args[0])
			if err != nil {
				return err
			}

			if graphQueryJSONFlag {
				data, _ := json.Marshal(nodes)
				fmt.Println(string(data))
			} else {
				if len(nodes) == 0 {
					fmt.Println("No backlinks found")
				} else {
					for _, n := range nodes {
						fmt.Printf("  %s: %s\n", n.ID, n.Title)
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
			cfg, err := config.Discover("")
			if err != nil {
				return err
			}

			dbPath := filepath.Join(cfg.RootPath, cfg.GraphPath)
			store, err := graphdb.OpenExisting(dbPath)
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
				data, _ := json.Marshal(result)
				fmt.Println(string(data))
			} else {
				for _, n := range nodes {
					fmt.Printf("  %s: %s\n", n.ID, n.Title)
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
			cfg, err := config.Discover("")
			if err != nil {
				return err
			}

			dbPath := filepath.Join(cfg.RootPath, cfg.GraphPath)
			store, err := graphdb.OpenExisting(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()

			nodes, err := store.GetByTag(args[0])
			if err != nil {
				return err
			}

			if graphQueryJSONFlag {
				data, _ := json.Marshal(nodes)
				fmt.Println(string(data))
			} else {
				if len(nodes) == 0 {
					fmt.Printf("No notes tagged %q\n", args[0])
				} else {
					for _, n := range nodes {
						fmt.Printf("  %s: %s\n", n.ID, n.Title)
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
			cfg, err := config.Discover("")
			if err != nil {
				return err
			}

			dbPath := filepath.Join(cfg.RootPath, cfg.GraphPath)
			store, err := graphdb.OpenExisting(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()

			edges, err := store.GetDanglingEdges()
			if err != nil {
				return err
			}

			if graphQueryJSONFlag {
				data, _ := json.Marshal(edges)
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
	var mcpCmd = &cobra.Command{
		Use:   "mcp",
		Short: "Serve okfw over MCP (stdio) for AI agents",
		Long: `Run an MCP (Model Context Protocol) server on stdin/stdout, exposing the
wiki through five consolidated tools: search, create, read, update, graph.
Tool responses are compact JSON with wiki-relative paths to minimize agent
token usage.

The wiki is discovered per call by searching upward from the server's working
directory, so configure your MCP client to launch okfw from the project root.

Example client configuration (Claude Code):
  claude mcp add okfw -- okfw mcp`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			server := &mcpServer{out: os.Stdout}
			return server.serve(os.Stdin)
		},
	}

	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(connectCmd)
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
	rootCmd.AddCommand(setProjectCmd)
	rootCmd.AddCommand(backlinksCmd)
	rootCmd.AddCommand(dailyCmd)
	rootCmd.AddCommand(addTagsCmd)
	validateCmd.Flags().BoolVar(&validateOKF, "okf", false, "Check the whole bundle for OKF conformance (permissive, spec section 11)")
	validateCmd.Flags().BoolVar(&validateAll, "all", false, "Apply the strict okfw schema to every concept in the bundle")
	for _, c := range fileCommands() {
		rootCmd.AddCommand(c)
	}
	rootCmd.AddCommand(newInstallCmd())
	for _, c := range okfFamilyCommands() {
		rootCmd.AddCommand(c)
	}
	rootCmd.AddCommand(migrateCmd)
	rootCmd.AddCommand(validateCmd)

	return rootCmd
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
	body := config.ExtractBody(content)
	links := config.ResolveBodyLinkIDs(body, absPath, bundleRoot)

	return &graph.Node{
		ID:       z.ID,
		Title:    z.Title,
		FilePath: absPath,
		Project:  z.Project,
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
			if fi.IsDir() && bundle.SkipWalkDir(path, p, fi.Name()) {
				return filepath.SkipDir
			}
			if !fi.IsDir() && strings.HasSuffix(p, ".md") && !bundle.IsReservedBase(fi.Name()) {
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

// runInit creates the .okfw wiki directory under target (a project root),
// with a .gitignore for generated artifacts and an OKF root index.md.
// A no-op with a message when the wiki already exists.
func runInit(target string) error {
	wikiDir := filepath.Join(target, config.WikiDirName)
	if fi, err := os.Stat(wikiDir); err == nil && fi.IsDir() {
		fmt.Printf("OKF wiki already initialized at %s\n", wikiDir)
		return nil
	}

	if err := os.MkdirAll(wikiDir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", wikiDir, err)
	}

	gitignore := "# okfw generated artifacts\n" +
		config.IndexDirName + "/\n" +
		config.GraphDBName + "\n" +
		"ephemeral/\n"
	if err := os.WriteFile(filepath.Join(wikiDir, ".gitignore"), []byte(gitignore), 0644); err != nil {
		return fmt.Errorf("failed to write .gitignore: %w", err)
	}

	// Start OKF-conformant: root index.md with the okf_version declaration.
	cfg := config.ConfigFor(wikiDir)
	if _, err := bundle.WriteIndexFiles(cfg, wikiDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to generate index.md: %v\n", err)
	}

	fmt.Printf("Initialized OKF wiki at %s\n", wikiDir)
	return nil
}

// bundle.SkipWalkDir reports whether a directory met during a bundle walk should be
// skipped: hidden directories (.index, .git, ...) and the ephemeral/ export
// dir — but never the walk root itself, which is the hidden .okfw directory.
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

// queryStore runs the graph query against the persistent store. With a start
// node it uses the store's indexed cluster BFS (backlinks are O(1) lookups and
// only the cluster is loaded); without one it must load the full graph to find
// the most-connected starting node.
func queryStore(store *graphdb.Store, startID string, limit, depth int) ([]*graph.Node, []graph.Edge, error) {
	if startID != "" {
		records, edgeRecords, err := store.GetCluster(startID, depth, limit)
		if err != nil {
			return nil, nil, err
		}
		nodes := make([]*graph.Node, 0, len(records))
		for i := range records {
			nr := &records[i]
			nodes = append(nodes, &graph.Node{
				ID:       nr.ID,
				Title:    nr.Title,
				FilePath: nr.FilePath,
				Project:  nr.Project,
				Parent:   nr.Parent,
				Links:    nr.Links,
			})
		}
		edges := make([]graph.Edge, 0, len(edgeRecords))
		for _, e := range edgeRecords {
			label := "link"
			if e.Predicate == graphdb.PredicateChildOf {
				label = "parent"
			}
			edges = append(edges, graph.Edge{From: e.From, To: e.To, Label: label})
		}
		return nodes, edges, nil
	}

	g, err := store.ToGraph()
	if err != nil {
		return nil, nil, err
	}
	nodes := g.FindAllConnected(limit, depth)
	return nodes, g.GetEdges(nodes), nil
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

// bundle.IndexFile indexes a single markdown file
// runMigrate strips the removed todo feature's frontmatter from every concept in
// the bundle. Idempotent: a second run reports nothing to do.
func runMigrate(cfg *config.Config, root string, dryRun bool) error {
	var scanned, changed int

	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			if bundle.SkipWalkDir(root, p, fi.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".md") || bundle.IsReservedBase(fi.Name()) {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		scanned++

		out, dropped, err := bundle.StripLegacyTodoFields(content)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ! %s: %v\n", rel, err)
			return nil
		}
		if out == nil {
			return nil
		}
		changed++
		fmt.Printf("  %s: dropped %s\n", rel, strings.Join(dropped, ", "))
		if dryRun {
			return nil
		}
		if err := os.WriteFile(p, out, 0644); err != nil {
			return err
		}
		return bundle.AutoSync(cfg, p)
	})
	if err != nil {
		return err
	}

	if changed == 0 {
		fmt.Printf("Nothing to migrate: %d concept(s) already clean.\n", scanned)
		return nil
	}
	if dryRun {
		fmt.Printf("\n%d of %d concept(s) would change. Re-run without --dry-run to apply.\n", changed, scanned)
		return nil
	}
	fmt.Printf("\nMigrated %d of %d concept(s).\n", changed, scanned)
	return nil
}

// runValidateAll applies the strict okfw #Concept schema to every concept in the
// bundle. This is the bulk counterpart to `okfw validate <file>`; runValidateOKF
// deliberately checks far less, because OKF conformance permits concepts okfw
// itself would reject.
func runValidateAll(cfg *config.Config, root string) error {
	var concepts, invalid int
	var problems []string

	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			if bundle.SkipWalkDir(root, p, fi.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".md") || bundle.IsReservedBase(fi.Name()) {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		content, err := os.ReadFile(p)
		if err != nil {
			invalid++
			problems = append(problems, fmt.Sprintf("  ✗ %s: %v", rel, err))
			return nil
		}
		concepts++
		if _, err := config.ParseAndValidate(content); err != nil {
			invalid++
			problems = append(problems, fmt.Sprintf("  ✗ %s: %v", rel, err))
		}
		return nil
	})
	if err != nil {
		return err
	}

	for _, pmsg := range problems {
		fmt.Println(pmsg)
	}
	fmt.Printf("\nokfw schema: %d concept(s), %d invalid.\n", concepts, invalid)
	if invalid > 0 {
		return fmt.Errorf("%d concept(s) failed schema validation", invalid)
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
			if bundle.SkipWalkDir(root, p, fi.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".md") || bundle.IsReservedBase(fi.Name()) {
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
		body := config.ExtractBody(content)
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
	count, err := bundle.WriteIndexFiles(cfg, root)
	if err != nil {
		return err
	}
	fmt.Printf("Wrote %d index.md file(s).\n", count)
	return nil
}

// bundle.WriteIndexFiles (re)generates an OKF index.md in every bundle directory:
// the root index carries the okf_version declaration, all others have no
// frontmatter. index.md files are owned by okfw and overwritten. Returns the
// number of files written.
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
		if !strings.HasSuffix(p, ".md") || bundle.IsReservedBase(filepath.Base(p)) {
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

// bundle.TouchTimestamp ensures the updates map bumps the OKF `timestamp` field to now
// (last meaningful change), unless the caller already set it explicitly.
// bundle.UpdateFrontmatter updates specific fields in YAML frontmatter
// backlinksToJSON converts backlinks slice to JSON
func backlinksToJSON(backlinks interface{}) (string, error) {
	data, err := json.MarshalIndent(backlinks, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// createOrOpenDaily returns the daily note for the given date, creating it
// (and auto-syncing indexes) when it doesn't exist yet.
func createOrOpenDaily(cfg *config.Config, targetDate time.Time) (path string, created bool, err error) {
	id := bundle.NewV7WithTime(targetDate)
	dateStr := targetDate.Format("2006-01-02")
	dayName := targetDate.Format("Monday")
	title := fmt.Sprintf("%s %s", dateStr, dayName)

	// Daily notes go into a "daily" subdirectory of the bundle
	dailyDir := filepath.Join(cfg.RootPath, "daily",
		targetDate.Format("2006"), targetDate.Format("01"))

	// Check if any daily note already exists for this date
	entries, _ := filepath.Glob(filepath.Join(dailyDir, "*.md"))
	if len(entries) > 0 {
		return entries[0], false, nil
	}

	if err := os.MkdirAll(dailyDir, 0755); err != nil {
		return "", false, fmt.Errorf("failed to create directory: %w", err)
	}

	filePath := filepath.Join(dailyDir, bundle.NoteFilename(dailyDir, title, id))

	tmpl, err := templates.Get("daily")
	if err != nil {
		return "", false, fmt.Errorf("failed to get daily template: %w", err)
	}
	content, err := tmpl.GenerateNote(id, title, "", nil)
	if err != nil {
		return "", false, fmt.Errorf("failed to generate daily note: %w", err)
	}
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return "", false, fmt.Errorf("failed to write file: %w", err)
	}

	if err := bundle.AutoSync(cfg, filePath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to index note: %v\n", err)
	}
	return filePath, true, nil
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

		// Filenames are slug--shortid.md, so the date comes from frontmatter:
		// `created` when present, else the creation time encoded in the id.
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		z, err := config.ParseFrontmatter(content)
		if err != nil || z.ID == "" {
			return nil // not a concept
		}

		fileDate := z.Created
		if fileDate.IsZero() {
			fileDate, err = bundle.TimeFromID(z.ID)
			if err != nil {
				return nil
			}
		}

		// Check if within date range
		if fileDate.Before(startDate) {
			return nil
		}

		dateStr := fileDate.Format("2006-01-02")
		title := dateStr + " " + fileDate.Format("Monday")
		if z.Title != "" {
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

// bundle.AutoIndex indexes a single file silently (for auto-indexing on create/update)
// bundle.AutoSync updates the search index and, when a graph database exists, the
// persistent graph for a single created or updated file (best-effort).
// bundle.Spec describes a concept to create.

// bundle.NewV7WithTime constructs a UUIDv7 with a specific timestamp instead of time.Now().
// bundle.TimeFromID extracts creation time from either ID format:
//   - Legacy: "YYYYMMDDHHmmss-UUID" → parse 14-char prefix
//   - UUIDv7: parse first 48 bits as Unix milliseconds
