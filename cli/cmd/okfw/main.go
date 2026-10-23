package main

import (
	"encoding/json"
	"fmt"
	"okfw/cli/internal/bundle"
	"okfw/cli/internal/config"
	"okfw/cli/internal/gitctx"
	"okfw/cli/internal/graph"
	"okfw/cli/internal/search"
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
// Extracted from main so tests can enumerate the registered commands, which is
// what lets the agent skill be checked against the binary.
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
		Version:      config.Version(),
		SilenceUsage: true,
		Short:        "Manage a per-project OKF wiki of linked markdown concepts",
		Long: `okfw manages a local-first knowledge base — an Open Knowledge Format (OKF)
wiki of markdown concepts stored in a .okfw/ directory at your project root.

Run 'okfw init' once per project. Every other command then finds the wiki
automatically by searching upward from the working directory, so okfw works
from any subdirectory. Zero configuration: there is no index or database to
maintain — search and the link graph are computed from the files, and the
index.md listings are kept current on every change.

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
working directory. The one generated directory, ephemeral/ (okfw export's
output), is gitignored via .okfw/.gitignore; concept markdown and the index.md
listings are meant to be committed with the project.

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

			fmt.Printf("Connected: %s\n", cfg.RootPath)
			fmt.Printf("  concepts: %d\n", concepts)
			return nil
		},
	}

	var indexNoAdoptFlag bool
	var indexMdCmd = &cobra.Command{
		Use:   "index-md [path]",
		Short: "Generate OKF index.md directory listings",
		Long: `Generate an OKF index.md in every bundle directory for progressive
disclosure. The root index.md carries the okf_version declaration; all other
index files have no frontmatter. Safe to re-run.

okfw marks the listings it generates and maintains only those on every change.
This command also takes over an unmarked index.md in a directory holding okfw
concepts — which is how listings from before the marker come back under
maintenance — but never deletes one: an index.md okfw did not write is pruned
by nobody but you.

--no-adopt refreshes only the listings okfw already maintains, exactly as every
okfw write does. Editors use it after saving a concept by hand.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Listings are bundle-wide and the root one carries okf_version, so
			// a path argument only selects which bundle, never a subtree.
			cfg, _, err := bundleArg(args)
			if err != nil {
				return err
			}
			return runIndexMd(cfg, cfg.RootPath, !indexNoAdoptFlag)
		},
	}
	indexMdCmd.Flags().BoolVar(&indexNoAdoptFlag, "no-adopt", false, "Refresh only listings okfw already maintains; never take over an unmarked index.md")

	var logCmd = &cobra.Command{
		Use:   "log [path]",
		Short: "Generate OKF log.md from git history",
		Long: `Render an OKF log.md from git history, grouped by date (newest first).
Only history under the directory is logged: the bundle root by default, or a
directory inside it, which gets its own log.md (OKF allows one at any level).
Concept additions, modifications, moves and deletions become Creation, Update,
Move and Removal entries using each commit's subject. Requires the bundle to be
in a git repository.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, root, err := bundleArg(args)
			if err != nil {
				return err
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

	var typeFlag string
	var kindFlag string
	var staleFlag bool

	var searchCmd = &cobra.Command{
		Use:   "search [query]",
		Short: "Search concepts",
		Long: `Search concepts by full-text query and/or field filters.

Examples:
  okfw search "authentication"              # Full-text search
  okfw search --project myproject           # Filter by project
  okfw search --tag golang --tag api        # Filter by tags (AND)
  okfw search --kind note                   # Notes only (no daily notes or issues)
  okfw search --type Playbook               # Filter by OKF free-form type
  okfw search --status draft                # Filter by lifecycle status
  okfw search --stale                       # Concepts past their stale_after
  okfw search "auth" --project myproject    # Combined search`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Discover("")
			if err != nil {
				return err
			}

			opts := search.Options{
				Project: projectFlag,
				Type:    typeFlag,
				Kind:    kindFlag,
				Tags:    tagsFlag,
				Limit:   limitFlag,
				Offset:  offsetFlag,
				Status:  statusFlag,
				Stale:   staleFlag,
			}

			if len(args) > 0 {
				opts.Query = strings.Join(args, " ")
			}

			results, total, err := search.Search(cfg.RootPath, opts)
			if err != nil {
				return fmt.Errorf("search failed: %w", err)
			}

			if jsonFlag {
				jsonOutput, err := search.ResultsToJSON(results)
				if err != nil {
					return fmt.Errorf("failed to format JSON: %w", err)
				}
				fmt.Println(jsonOutput)
			} else {
				fmt.Print(search.FormatResultsFrom(results, total, offsetFlag))
			}

			return nil
		},
	}

	var graphStartFlag string
	var graphDepthFlag int
	var graphASCIIFlag bool

	var graphCmd = &cobra.Command{
		Use:   "graph [path]",
		Short: "Render concept relationships as a Mermaid diagram (or an ASCII tree)",
		Long: `Print the relationships between concepts.

Output is Mermaid flowchart syntax by default, suitable for pasting into
markdown. Use --ascii for a plain-text tree that reads directly in a terminal.

The diagram goes to stdout so it can be piped; a summary line
(N nodes, M edges) goes to stderr so it does not pollute piped output.

If no path is given, the discovered wiki root is used. The graph is built
from the concepts on disk each time, so it always matches them.

Examples:
  okfw graph                             # Mermaid diagram from the wiki root
  okfw graph --ascii                     # Plain-text tree instead
  okfw graph path/to/dir                 # All concepts in a directory
  okfw graph --start 20260213-abc-123    # Centered on a specific concept
  okfw graph --depth 3                   # Traverse up to 3 hops from start
  okfw graph --limit 20                  # Show up to 20 nodes (default: 10)`,
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

			g, err := bundle.LoadGraphFrom(path, cfg.RootPath)
			if err != nil {
				return err
			}
			nodes, err := queryGraph(cfg, g, graphStartFlag, graphLimitFlag, graphDepthFlag)
			if err != nil {
				return err
			}
			edges := g.GetEdges(nodes)

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
		Use:   "set-project [id_or_file] [project]",
		Short: "Set or update the project for a concept",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, filePath, err := resolveRefArg(args[0])
			if err != nil {
				return err
			}
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
			if err := bundle.AutoSync(cfg, filePath); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to refresh index.md listings: %v\n", err)
			}
			return nil
		},
	}

	var backlinksCmd = &cobra.Command{
		Use:   "backlinks [id_or_file]",
		Short: "Find all notes that link to a concept",
		Long: `Find all concepts that contain links to the specified concept.

A link is a markdown link in the body, to a bundle-relative or relative path,
or a path-valued frontmatter field naming the concept (resource,
sources[].resource, computation, executor.resource, attester.resource).

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

			targetID, err := bundle.ConceptKeyOf(cfg, args[0])
			if err != nil {
				return err
			}
			targetTitle := ""

			type Backlink struct {
				ID       string `json:"id"`
				Title    string `json:"title"`
				Project  string `json:"project"`
				FilePath string `json:"file_path"`
			}

			var backlinks []Backlink
			g, err := bundle.LoadGraph(cfg.RootPath)
			if err != nil {
				return err
			}
			for _, n := range g.Backlinks(targetID) {
				backlinks = append(backlinks, Backlink{
					ID:       n.ID,
					Title:    n.Title,
					Project:  n.Project,
					FilePath: n.FilePath,
				})
			}
			if targetTitle == "" {
				if target := g.GetNode(targetID); target != nil {
					targetTitle = target.Title
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
then copies each concept into <root>/ephemeral/ as <id>.md, rewriting links
between exported concepts to point at those files, so the set reads in any
markdown viewer.

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

			g, err := bundle.LoadGraphFrom(path, cfg.RootPath)
			if err != nil {
				return err
			}
			nodes, err := queryGraph(cfg, g, exportStartFlag, exportLimitFlag, exportDepthFlag)
			if err != nil {
				return err
			}

			// Map each exported concept's bundle-relative path to its ID so that
			// in-bundle markdown links can be flattened to <id>.md in the export.
			pathToID := make(map[string]string, len(nodes))
			exported := 0
			for _, n := range nodes {
				if !safeExportName(n.ID) {
					continue
				}
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
				// The id names the output file, and it comes from frontmatter
				// anyone can write: `id: "../../x"` once planted a file outside
				// ephemeral/.
				if !safeExportName(node.ID) {
					fmt.Fprintf(os.Stderr, "Warning: skipping %s: id %q is not a safe file name\n", node.FilePath, node.ID)
					continue
				}
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
				toExported := func(href string) string {
					if id, ok := pathToID[graph.NormalizeTarget(href, fromDir)]; ok {
						return id + ".md"
					}
					return href
				}
				transformed := []byte(graph.RewriteLinks(string(content), toExported))
				// Frontmatter paths are links too, and name files the export renames.
				if out, err := bundle.RewritePathRefs(transformed, fromDir, toExported); err == nil {
					transformed = out
				}
				outPath := filepath.Join(ephemeralDir, node.ID+".md")
				if err := os.WriteFile(outPath, transformed, 0644); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to write %s: %v\n", outPath, err)
					continue
				}
				exported++
			}

			fmt.Fprintf(os.Stderr, "Exported %d files to ephemeral/\n", exported)
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
	searchCmd.Flags().StringVar(&kindFlag, "kind", "", "Filter by structured kind (note/daily-note/issue)")
	// Offset has been honoured by index.Search since it was written; nothing
	// ever set it, so pagination was unreachable and the only way past the
	// limit was to raise it.
	searchCmd.Flags().IntVar(&offsetFlag, "offset", 0, "Skip this many results (pagination)")
	searchCmd.Flags().StringSliceVarP(&tagsFlag, "tag", "T", nil, "Filter by tag (can be repeated)")
	searchCmd.Flags().StringVar(&statusFlag, "status", "", "Filter by OKF lifecycle status (draft/stable/deprecated)")
	searchCmd.Flags().BoolVar(&staleFlag, "stale", false, "Only concepts whose stale_after date has passed")
	searchCmd.Flags().IntVarP(&limitFlag, "limit", "l", 20, "Maximum number of results")
	searchCmd.Flags().BoolVar(&jsonFlag, "json", false, "Output results as JSON")

	graphCmd.Flags().IntVarP(&graphLimitFlag, "limit", "l", 10, "Maximum number of nodes to display")
	graphCmd.Flags().StringVar(&graphStartFlag, "start", "", "Start graph from a specific concept ID")
	graphCmd.Flags().IntVar(&graphDepthFlag, "depth", 0, "Maximum BFS depth (hops) from start node (0 = unlimited)")
	graphCmd.Flags().BoolVar(&graphASCIIFlag, "ascii", false, "Render a plain-text tree instead of Mermaid")

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

	var addTagsCmd = &cobra.Command{
		Use:   "add-tags [id_or_file] [tags...]",
		Short: "Add tags to a concept",
		Long: `Add one or more tags to a concept's frontmatter. Duplicate tags are ignored.

Examples:
  okfw add-tags ./path/to/note.md golang api
  okfw add-tags 202602131045 security review`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, filePath, err := resolveRefArg(args[0])
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

OKF also moved stale_after, sources[].last_modified and usage_window from bare
dates to ISO 8601 datetimes with a UTC offset. A bare date is rewritten as
midnight UTC — the reading okfw already gave it, so staleness does not shift:

  stale_after: 2026-12-31     ->  stale_after: 2026-12-31T00:00:00Z

A timestamp that is neither (say, a datetime with no offset) is reported for
you to fix by hand, since only its author knows which instant it meant.

Examples:
  okfw migrate --dry-run     # report what would change
  okfw migrate               # apply`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, root, err := bundleArg(args)
			if err != nil {
				return err
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
			// A file on disk is validated as given — checking a file outside any
			// wiki is a legitimate, read-only use. Anything else is a concept
			// reference: an id, a unique prefix, or an OKF concept ID.
			filePath := args[0]
			if fi, err := os.Stat(filePath); err != nil || fi.IsDir() {
				cfg, err := config.Discover("")
				if err != nil {
					return err
				}
				if filePath, err = bundle.ResolveConceptPath(cfg, args[0]); err != nil {
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

	// --- graph-query command ---
	var graphQueryDepthFlag int
	var graphQueryLimitFlag int
	var graphQueryJSONFlag bool

	var graphQueryCmd = &cobra.Command{
		Use:   "graph-query <subcommand> <arg>",
		Short: "Query the link graph",
		Long: `Query the link graph for backlinks, clusters, tags, or dangling edges. The
graph is built from the files on each call; there is no database to sync.

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

			key, err := bundle.ConceptKeyOf(cfg, args[0])
			if err != nil {
				return err
			}
			g, err := bundle.LoadGraph(cfg.RootPath)
			if err != nil {
				return err
			}
			nodes := g.Backlinks(key)

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

			key, err := bundle.ConceptKeyOf(cfg, args[0])
			if err != nil {
				return err
			}
			g, err := bundle.LoadGraph(cfg.RootPath)
			if err != nil {
				return err
			}
			if g.GetNode(key) == nil {
				return fmt.Errorf("node %q not found", args[0])
			}
			// Walk all of the cluster, then cap, so the output can say what the
			// limit withheld (see the MCP cluster action).
			nodes := g.FindConnected(key, g.NodeCount(), graphQueryDepthFlag)
			total := len(nodes)
			limit := graphQueryLimitFlag
			if limit <= 0 {
				limit = 10
			}
			nodes = nodes[:min(total, limit)]
			edges := g.GetEdges(nodes)

			if graphQueryJSONFlag {
				result := struct {
					Nodes     []*graph.Node `json:"nodes"`
					Edges     []graph.Edge  `json:"edges"`
					Total     int           `json:"total"`
					Truncated bool          `json:"truncated,omitempty"`
				}{Nodes: nodes, Edges: edges, Total: total, Truncated: total > len(nodes)}
				data, _ := json.Marshal(result)
				fmt.Println(string(data))
			} else {
				for _, n := range nodes {
					fmt.Printf("  %s: %s\n", n.ID, n.Title)
				}
				fmt.Printf("\n%d nodes, %d edges\n", len(nodes), len(edges))
				if total > len(nodes) {
					fmt.Printf("… %d more in reach — raise --limit\n", total-len(nodes))
				}
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

			g, err := bundle.LoadGraph(cfg.RootPath)
			if err != nil {
				return err
			}
			nodes := g.ByTag(args[0])

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

			g, err := bundle.LoadGraph(cfg.RootPath)
			if err != nil {
				return err
			}
			edges := g.Dangling()

			if graphQueryJSONFlag {
				data, _ := json.Marshal(edges)
				fmt.Println(string(data))
			} else {
				if len(edges) == 0 {
					fmt.Println("No dangling edges found")
				} else {
					for _, e := range edges {
						// The source's path, so the link can be opened and fixed;
						// its id says nothing about where it lives.
						from := e.From
						if n := g.GetNode(e.From); n != nil {
							if rel, err := filepath.Rel(cfg.RootPath, n.FilePath); err == nil {
								from = filepath.ToSlash(rel)
							}
						}
						fmt.Printf("  %s -[%s]-> %s (target not found)\n", from, e.Label, e.To)
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
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(graphCmd)
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

// queryGraph runs FindConnected or FindAllConnected based on startID.
func queryGraph(cfg *config.Config, g *graph.Graph, start string, limit, depth int) ([]*graph.Node, error) {
	if start != "" {
		// Accept any concept reference, as every other command does: a full
		// id, a unique prefix, an OKF concept ID or a path.
		key, err := bundle.ConceptKeyOf(cfg, start)
		if err != nil {
			return nil, err
		}
		if g.GetNode(key) == nil {
			return nil, fmt.Errorf("start node %q not found in graph", start)
		}
		return g.FindConnected(key, limit, depth), nil
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

// runMigrate brings older concepts up to the current frontmatter shape: it
// strips the removed todo feature's fields and upgrades bare dates to OKF
// datetimes. Idempotent: a second run reports nothing to do.
func runMigrate(cfg *config.Config, root string, dryRun bool) error {
	var scanned, changed, repair int

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

		var notes []string
		out := content
		if stripped, dropped, err := bundle.StripLegacyTodoFields(out); err != nil {
			fmt.Fprintf(os.Stderr, "  ! %s: %v\n", rel, err)
			return nil
		} else if stripped != nil {
			out = stripped
			notes = append(notes, "dropped "+strings.Join(dropped, ", "))
		}
		if upgraded, keys, err := bundle.UpgradeDateOnlyTimestamps(out); err != nil {
			fmt.Fprintf(os.Stderr, "  ! %s: %v\n", rel, err)
			return nil
		} else if upgraded != nil {
			out = upgraded
			notes = append(notes, "dates to datetimes: "+strings.Join(keys, ", "))
		}
		if bad, err := bundle.TimestampsNeedingRepair(out); err == nil && len(bad) > 0 {
			repair++
			fmt.Fprintf(os.Stderr, "  ! %s: fix by hand, not an ISO 8601 datetime with an offset: %s\n",
				rel, strings.Join(bad, ", "))
		}
		if len(notes) == 0 {
			return nil
		}
		changed++
		fmt.Printf("  %s: %s\n", rel, strings.Join(notes, "; "))
		if dryRun {
			return nil
		}
		return os.WriteFile(p, out, 0644)
	})
	if err != nil {
		return err
	}
	if repair > 0 {
		fmt.Fprintf(os.Stderr, "%d concept(s) carry a timestamp migrate cannot repair.\n", repair)
	}
	if changed > 0 && !dryRun {
		// One refresh for the whole run: the index walk covers the bundle, so
		// refreshing per file made migrating 300 concepts take 28 seconds.
		// Adopting also brings listings written before IndexMarker existed
		// back under maintenance.
		if _, err := bundle.AdoptIndexFiles(cfg, cfg.RootPath); err != nil {
			return err
		}
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
//
// It also reports ids declared by more than one concept — a concept copied as
// a template keeps its id — which no per-file check can see. Lookup by such
// an id refuses rather than guessing, so the copies have to be told apart.
func runValidateAll(cfg *config.Config, path string) error {
	var concepts, invalid int
	var problems []string
	pathsByID := map[string][]string{}

	root, bundleRoot, err := validateScope(cfg, path)
	if err != nil {
		return err
	}
	err = filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			if p != root && bundle.SkipWalkDir(bundleRoot, p, fi.Name()) {
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
		// Leniently, so an otherwise invalid copy still counts as a duplicate.
		if z, err := config.ParseFrontmatter(content); err == nil && z.ID != "" {
			pathsByID[z.ID] = append(pathsByID[z.ID], filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return err
	}

	var duplicated []string
	for id, paths := range pathsByID {
		if len(paths) > 1 {
			duplicated = append(duplicated, id)
		}
	}
	sort.Strings(duplicated)
	for _, id := range duplicated {
		problems = append(problems, fmt.Sprintf("  ✗ id %s is declared by %d concepts: %s",
			id, len(pathsByID[id]), strings.Join(pathsByID[id], ", ")))
	}

	for _, pmsg := range problems {
		fmt.Println(pmsg)
	}
	fmt.Printf("\nokfw schema: %d concept(s), %d invalid, %d duplicated id(s).\n", concepts, invalid, len(duplicated))
	switch {
	case invalid > 0 && len(duplicated) > 0:
		return fmt.Errorf("%d concept(s) failed schema validation and %d id(s) are duplicated", invalid, len(duplicated))
	case invalid > 0:
		return fmt.Errorf("%d concept(s) failed schema validation", invalid)
	case len(duplicated) > 0:
		return fmt.Errorf("%d id(s) are declared by more than one concept", len(duplicated))
	}
	return nil
}

// runValidateOKF checks a bundle for OKF conformance: every non-reserved .md has
// parseable frontmatter with a non-empty `type`. Unresolved links are reported as
// warnings (OKF tolerates broken links). Returns an error if any hard violation
// is found.
func runValidateOKF(cfg *config.Config, path string) error {
	var concepts, violations, brokenLinks, warnings int
	var problems []string

	root, bundleRoot, err := validateScope(cfg, path)
	if err != nil {
		return err
	}
	err = filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			if p != root && bundle.SkipWalkDir(bundleRoot, p, fi.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".md") {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		content, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		// Section 11.3: reserved files must follow sections 8 and 9.
		switch fi.Name() {
		case "index.md":
			v, w := bundle.CheckIndexFile(content, filepath.Dir(p) == bundleRoot)
			for _, m := range v {
				violations++
				problems = append(problems, fmt.Sprintf("  ✗ %s: %s", rel, m))
			}
			for _, m := range w {
				warnings++
				problems = append(problems, fmt.Sprintf("  ⚠ %s: %s", rel, m))
			}
			return nil
		case "log.md":
			for _, m := range bundle.CheckLogFile(content) {
				violations++
				problems = append(problems, fmt.Sprintf("  ✗ %s: %s", rel, m))
			}
			return nil
		}
		// Section 11.1 asks only for parseable YAML frontmatter, so a field of
		// the wrong shape is not a violation here; `okfw validate` is strict.
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
		// Warn on unresolved internal links. They resolve against the bundle
		// root, not the directory being checked, as in every other command.
		body := config.ExtractBody(content)
		fromDir := ""
		if rd, err := filepath.Rel(bundleRoot, filepath.Dir(p)); err == nil {
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
			if _, err := os.Stat(filepath.Join(bundleRoot, filepath.FromSlash(tr))); err != nil {
				brokenLinks++
				problems = append(problems, fmt.Sprintf("  ⚠ %s: link target not found: %s", rel, href))
			}
		}
		// A path-valued field naming a concept is a link as well (section 6.2).
		for _, r := range z.PathRefs() {
			tr := graph.NormalizeRef(r.Value, fromDir)
			if tr == "" {
				continue
			}
			if _, err := os.Stat(filepath.Join(bundleRoot, filepath.FromSlash(tr))); err != nil {
				brokenLinks++
				problems = append(problems, fmt.Sprintf("  ⚠ %s: %s target not found: %s", rel, r.Field, r.Value))
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Section 12: a version okfw does not implement is checked best-effort,
	// not refused.
	if data, err := os.ReadFile(filepath.Join(bundleRoot, "index.md")); err == nil {
		if v := bundle.DeclaredOKFVersion(data); v != "" && v != config.OKFVersion {
			warnings++
			problems = append(problems, fmt.Sprintf(
				"  ⚠ index.md: the bundle declares OKF %s; checked against OKF %s on a best-effort basis", v, config.OKFVersion))
		}
	}

	for _, pmsg := range problems {
		fmt.Println(pmsg)
	}
	fmt.Printf("\nOKF conformance: %d concept(s), %d violation(s), %d broken link(s), %d warning(s).\n",
		concepts, violations, brokenLinks, warnings)
	if violations > 0 {
		return fmt.Errorf("%d conformance violation(s)", violations)
	}
	return nil
}

// validateScope resolves the path given to validate --all/--okf. root is the
// directory to check; bundleRoot is the bundle it belongs to: the discovered
// wiki when the path lies inside it, else the path itself, taken as a bundle of
// its own. Links and the root index.md are judged against bundleRoot, so
// checking a subdirectory reads a bare frontmatter path from the bundle root,
// as every other command does. Both are absolute and clean: a trailing slash
// once made the root unrecognisable, so ephemeral/ was walked and the root
// index.md was held to the rules for every other listing.
func validateScope(cfg *config.Config, path string) (root, bundleRoot string, err error) {
	if root, err = filepath.Abs(path); err != nil {
		return "", "", err
	}
	wiki, err := filepath.Abs(cfg.RootPath)
	if err != nil {
		return "", "", err
	}
	if root == wiki || strings.HasPrefix(root, wiki+string(filepath.Separator)) {
		return root, wiki, nil
	}
	return root, root, nil
}

// safeExportName reports whether a concept id can name a file directly inside
// the export directory.
func safeExportName(id string) bool {
	return id != "" && id != "." && id != ".." && !strings.ContainsAny(id, `/\`)
}

// bundleArg resolves the optional [path] argument of a command that writes to
// the bundle. The path may name the bundle, a directory inside it, or the
// project containing it; the bundle is discovered from there, as it would be
// from the working directory. The returned root is the path itself when it lies
// inside the bundle, otherwise the bundle root — so a write never lands in a
// directory that no discovered bundle contains. `okfw index-md ~/elsewhere`
// used to prune hand-written index.md files there, and delete the directories
// it emptied.
func bundleArg(args []string) (*config.Config, string, error) {
	start := ""
	if len(args) > 0 {
		start = args[0]
		if fi, err := os.Stat(start); err != nil || !fi.IsDir() {
			return nil, "", fmt.Errorf("%s is not a directory", start)
		}
	}
	cfg, err := config.Discover(start)
	if err != nil {
		return nil, "", err
	}
	if start == "" {
		return cfg, cfg.RootPath, nil
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		return nil, "", err
	}
	if bundle.Within(cfg.RootPath, abs) {
		return cfg, abs, nil
	}
	return cfg, cfg.RootPath, nil
}

// runIndexMd generates an OKF index.md in each bundle directory (progressive
// disclosure). The root index.md carries the okf_version declaration; all other
// index files have no frontmatter.
func runIndexMd(cfg *config.Config, root string, adopt bool) error {
	write := bundle.WriteIndexFiles
	if adopt {
		write = bundle.AdoptIndexFiles
	}
	count, err := write(cfg, root)
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
	// Scope to this directory (`-- .`, with paths relative to it) and keep
	// non-ASCII paths unquoted. Without the pathspec this logged the whole
	// repository — README.md, docs/ — under bundle-root links like
	// /.okfw/x.md that resolve to nothing.
	cmd := exec.Command("git", "-c", "core.quotePath=false", "-C", root, "log",
		"--date=short", "--name-status", "--relative", "--pretty=format:\x01%ad\x02%s", "--", ".")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git log failed (is %s in a git repository?): %w", root, err)
	}

	// Links are bundle-absolute; root may be a directory inside the bundle.
	prefix := ""
	if rel, err := filepath.Rel(cfg.RootPath, root); err == nil && rel != "." {
		prefix = filepath.ToSlash(rel) + "/"
	}

	type event struct{ verb, path, subject string }
	byDate := map[string][]event{}
	var curDate, curSubject string

	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "\x01") {
			parts := strings.SplitN(strings.TrimPrefix(line, "\x01"), "\x02", 2)
			curDate = parts[0]
			curSubject = ""
			if len(parts) > 1 {
				curSubject = parts[1]
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
			verb = "Removal"
		case strings.HasPrefix(status, "R"):
			verb = "Move"
		}
		byDate[curDate] = append(byDate[curDate], event{verb, prefix + filepath.ToSlash(p), curSubject})
	}

	// Newest first, as section 9 requires. Commit order is not date order —
	// author dates survive rebases and cherry-picks — so sort the dates.
	dates := make([]string, 0, len(byDate))
	for d := range byDate {
		dates = append(dates, d)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dates)))

	esc := strings.NewReplacer("[", "\\[", "]", "\\]", "<", "&lt;")
	var b strings.Builder
	b.WriteString("# Update Log\n\n")
	for _, d := range dates {
		b.WriteString("## " + d + "\n\n")
		for _, e := range byDate[d] {
			if e.verb == "Removal" {
				// The file is gone, so a link would point at nothing.
				b.WriteString("* **" + e.verb + "**: `/" + e.path + "`")
			} else {
				b.WriteString("* **" + e.verb + "**: [" + esc.Replace(e.path) + "](/" + e.path + ")")
			}
			if e.subject != "" {
				b.WriteString(" — " + esc.Replace(e.subject))
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

	// Reuse the note for this date if there is one. This used to return the
	// first *.md in the month directory — any day's note, or its index.md —
	// so a month held exactly one daily note.
	if existing := findDailyNote(dailyDir, dateStr); existing != "" {
		return existing, false, nil
	}

	if err := os.MkdirAll(dailyDir, 0755); err != nil {
		return "", false, fmt.Errorf("failed to create directory: %w", err)
	}

	tmpl, err := templates.Get("daily")
	if err != nil {
		return "", false, fmt.Errorf("failed to get daily template: %w", err)
	}
	content, err := tmpl.GenerateNote(id, title, "", nil)
	if err != nil {
		return "", false, fmt.Errorf("failed to generate daily note: %w", err)
	}
	filePath, err := bundle.WriteNewConcept(dailyDir, title, id, []byte(content))
	if err != nil {
		return "", false, err
	}

	if err := bundle.AutoSync(cfg, filePath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to index note: %v\n", err)
	}
	return filePath, true, nil
}

// findDailyNote returns the daily note for date (YYYY-MM-DD) in dir, or "".
func findDailyNote(dir, date string) string {
	entries, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	for _, e := range entries {
		if bundle.IsReservedBase(filepath.Base(e)) {
			continue
		}
		content, err := os.ReadFile(e)
		if err != nil {
			continue
		}
		z, err := config.ParseFrontmatter(content)
		if err != nil {
			continue
		}
		if d, ok := dailyDate(z); ok && d.Format("2006-01-02") == date {
			return e
		}
	}
	return ""
}

// dailyDate returns the calendar date a daily note is for. The title is
// "YYYY-MM-DD Weekday" by construction and is the only record of the day the
// note is *for*: a backdated note's created stamp is the day it was written,
// and the time embedded in its id is UTC, which puts an evening note west of
// UTC on the following day.
func dailyDate(z *config.Concept) (time.Time, bool) {
	if len(z.Title) >= 10 {
		if d, err := time.ParseInLocation("2006-01-02", z.Title[:10], time.Local); err == nil {
			return d, true
		}
	}
	if !z.Created.IsZero() {
		return z.Created.In(time.Local), true
	}
	if t, err := bundle.TimeFromID(z.ID); err == nil {
		return t.In(time.Local), true
	}
	return time.Time{}, false
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

		if bundle.IsReservedBase(info.Name()) {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		z, err := config.ParseFrontmatter(content)
		if err != nil || z.ID == "" {
			return nil // not a concept
		}
		fileDate, ok := dailyDate(z)
		if !ok {
			return nil
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
