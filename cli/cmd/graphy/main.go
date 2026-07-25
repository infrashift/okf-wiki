// graphy is a code knowledge-graph CLI: it extracts a typed, confidence-tagged
// graph from a repository (tree-sitter AST + markdown links), detects
// communities, answers path/explain/NL queries, and exports concepts into an
// OKF Wiki bundle. Sibling binary to okfw in the same module.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"okfw/cli/internal/gitctx"
	"okfw/cli/internal/graphy/build"
	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/store"
	"okfw/cli/internal/graphy/workspace"
)

var version = "0.1.0-dev"

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "graphy",
		Short: "Generate and query a knowledge graph of a code repository",
		Long: `graphy builds a typed knowledge graph from a code repository using
deterministic AST extraction (tree-sitter), detects communities, and answers
graph queries. The graph can be exported into an OKF Wiki (.okfw) bundle.`,
		SilenceUsage: true,
	}
	root.AddCommand(initCmd(), buildCmd(), updateCmd(), exportCmd(), versionCmd())
	return root
}

func buildCmd() *cobra.Command {
	var full bool
	var langs []string
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Extract the repository into the knowledge graph",
		Long: `Runs the pipeline: detect files, extract changed ones (tree-sitter AST +
markdown links), resolve cross-file references into confidence-tagged edges,
and refresh graph.json. Extraction is incremental; use --full to rebuild.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBuild(build.Options{Full: full, Langs: langs})
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "wipe the extraction cache and re-extract everything")
	cmd.Flags().StringSliceVar(&langs, "langs", nil, "restrict to these languages (default: all supported)")
	return cmd
}

func updateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Incrementally refresh the graph (alias of build)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBuild(build.Options{})
		},
	}
}

func runBuild(opts build.Options) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	s, err := store.Open(w.DBPath())
	if err != nil {
		return err
	}
	defer s.Close()

	stats, err := build.Run(w, s, opts)
	if err != nil {
		return err
	}
	if err := writeGraphJSON(w, s, w.JSONPath()); err != nil {
		return err
	}
	fmt.Printf("Scanned %d files (%d extracted, %d purged)\n",
		stats.FilesScanned, stats.FilesExtracted, stats.FilesPurged)
	fmt.Printf("Graph: %d nodes, %d edges (%d refs resolved, %d ambiguous, %d dropped)\n",
		stats.Nodes, stats.Edges, stats.RefsResolved, stats.RefsAmbiguous, stats.RefsDropped)
	fmt.Printf("Wrote %s\n", w.JSONPath())
	return nil
}

// openWorkspace discovers the workspace from the cwd.
func openWorkspace() (*workspace.Workspace, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return workspace.Discover(cwd)
}

// openStore discovers the workspace and opens its graph database.
func openStore() (*workspace.Workspace, *store.Store, error) {
	w, err := openWorkspace()
	if err != nil {
		return nil, nil, err
	}
	if _, err := os.Stat(w.DBPath()); err != nil {
		return nil, nil, fmt.Errorf("no graph database at %s (run `graphy build`)", w.DBPath())
	}
	s, err := store.Open(w.DBPath())
	if err != nil {
		return nil, nil, err
	}
	return w, s, nil
}

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init [directory]",
		Short: "Create a .graphy workspace at the repository root",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := ""
			if len(args) == 1 {
				target = args[0]
			} else {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				if gitRoot := gitctx.GetGitRoot(cwd); gitRoot != "" {
					target = gitRoot
				} else {
					target = cwd
				}
			}
			w, err := workspace.Init(target)
			if err != nil {
				return err
			}
			fmt.Printf("Initialized graphy workspace at %s\n", w.Dir())
			fmt.Println("Next: run `graphy build` to extract the graph.")
			return nil
		},
	}
}

func exportCmd() *cobra.Command {
	export := &cobra.Command{
		Use:   "export",
		Short: "Export the graph to another format",
	}
	export.AddCommand(exportJSONCmd())
	return export
}

func exportJSONCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "json",
		Short: "Write the portable graph.json",
		RunE: func(cmd *cobra.Command, args []string) error {
			w, s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()
			path := out
			if path == "" {
				path = w.JSONPath()
			}
			if err := writeGraphJSON(w, s, path); err != nil {
				return err
			}
			fmt.Printf("Wrote %s\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "output path (default .graphy/graph.json)")
	return cmd
}

// writeGraphJSON dumps the store into a deterministic graph.json file.
func writeGraphJSON(w *workspace.Workspace, s *store.Store, path string) error {
	var nodes []model.Node
	if err := s.EachNode(func(n model.Node) error {
		nodes = append(nodes, n)
		return nil
	}); err != nil {
		return err
	}
	var edges []model.Edge
	if err := s.EachEdge(func(e model.Edge) error {
		edges = append(edges, e)
		return nil
	}); err != nil {
		return err
	}
	communities := 0
	if err := s.EachCommunity(func(store.Community) error {
		communities++
		return nil
	}); err != nil {
		return err
	}
	files := 0
	if err := s.EachFile(func(store.FileRecord) error {
		files++
		return nil
	}); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return model.EncodeGraphJSON(f, w.Root, nodes, edges, communities, files)
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the graphy version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("graphy", version)
		},
	}
}
