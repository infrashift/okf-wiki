// graphy is a code knowledge-graph CLI: it extracts a typed, confidence-tagged
// graph from a repository (tree-sitter AST + markdown links), detects
// communities, answers path/explain/NL queries, and exports concepts into an
// OKF Wiki bundle. Sibling binary to okfw in the same module.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"okfw/cli/internal/config"
	"okfw/cli/internal/gitctx"
	"okfw/cli/internal/graphy/build"
	"okfw/cli/internal/graphy/cluster"
	"okfw/cli/internal/graphy/export"
	"okfw/cli/internal/graphy/extract"
	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/skills"
	"okfw/cli/internal/graphy/store"
	"okfw/cli/internal/graphy/workspace"
)

var version = "0.1.0-dev"

func main() {
	// Commands read this via cmd.Context(). Without it an in-flight LLM
	// request ignores Ctrl-C until its own HTTP timeout expires.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := rootCmd().ExecuteContext(ctx); err != nil {
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
		// main prints the error itself; without this cobra prints it too.
		SilenceErrors: true,
	}
	root.AddCommand(initCmd(), buildCmd(), updateCmd(), queryCmd(), pathCmd(), explainCmd(), reportCmd(), exportCmd(), installCmd(), mcpCmd(), versionCmd())
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
	}
	cf := clusterFlags(cmd)
	rf := rationaleFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runBuild(build.Options{Full: full, Langs: langs, Rationale: rf.options()}, cf)
	}
	cmd.Flags().BoolVar(&full, "full", false, "wipe the extraction cache and re-extract everything")
	cmd.Flags().StringSliceVar(&langs, "langs", nil,
		"restrict extraction to these languages; other languages keep their existing nodes")
	return cmd
}

func updateCmd() *cobra.Command {
	var langs []string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Incrementally refresh the graph",
		Long: `Re-extracts changed files and re-resolves the graph. Equivalent to a plain
` + "`graphy build`" + `: the clustering settings the graph was last built with are
inherited, and any of them can be overridden here.`,
	}
	// The clustering flags belong here too: without them there would be no way
	// to change the algorithm during an update. Inheriting the previous run's
	// settings is handled by clusterFlagVals.resolve, not by the defaults.
	cf := clusterFlags(cmd)
	rf := rationaleFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runBuild(build.Options{Langs: langs, Rationale: rf.options()}, cf)
	}
	cmd.Flags().StringSliceVar(&langs, "langs", nil,
		"restrict extraction to these languages; other languages keep their existing nodes")
	return cmd
}

// rationaleFlagVals holds the raw rationale-extraction flag values.
type rationaleFlagVals struct {
	disabled bool
	markers  []string
	adrDirs  []string
}

// rationaleFlags binds the rationale-extraction flags shared by build and
// update. All three feed the extraction fingerprint, so changing any of them
// forces a full re-extract on the next build.
func rationaleFlags(cmd *cobra.Command) *rationaleFlagVals {
	v := &rationaleFlagVals{}
	cmd.Flags().BoolVar(&v.disabled, "no-rationale", false,
		"skip rationale extraction (WHY:/DECISION: comments and ADRs)")
	cmd.Flags().StringSliceVar(&v.markers, "markers", extract.DefaultMarkers,
		"comment markers that become rationale nodes")
	cmd.Flags().StringSliceVar(&v.adrDirs, "adr-dirs", extract.DefaultADRDirs,
		"directory names holding architecture decision records")
	return v
}

func (v *rationaleFlagVals) options() extract.RationaleOptions {
	return extract.RationaleOptions{
		Enabled: !v.disabled,
		Markers: v.markers,
		ADRDirs: v.adrDirs,
	}
}

// clusterFlagVals holds the raw community-detection flag values, plus the flag
// set they were bound to so resolve can tell "the user asked for the default"
// from "the user said nothing".
type clusterFlagVals struct {
	algorithm  string
	resolution float64
	seed       uint64
	flags      *pflag.FlagSet
}

// clusterFlags binds the community-detection flags shared by build and update,
// so the two cannot drift apart.
func clusterFlags(cmd *cobra.Command) *clusterFlagVals {
	v := &clusterFlagVals{flags: cmd.Flags()}
	cmd.Flags().StringVar(&v.algorithm, "cluster-algorithm", string(cluster.DefaultAlgorithm),
		"community detection algorithm: "+strings.Join(cluster.Algorithms(), " or "))
	cmd.Flags().Float64Var(&v.resolution, "cluster-resolution", cluster.DefaultResolution,
		"resolution parameter: higher splits into more, smaller communities")
	cmd.Flags().Uint64Var(&v.seed, "cluster-seed", cluster.DefaultSeed,
		"RNG seed for reproducible community assignments")
	return v
}

// resolve validates the flags and converts them for the build, inheriting any
// flag the user did not pass from how the graph was last clustered.
//
// Binding these flags to `update` is not enough on its own: they default to
// graphy's defaults, so a bare `graphy update` after
// `graphy build --cluster-algorithm louvain` would re-cluster with Leiden —
// the exact silent reversion the flags were added to prevent. Communities are
// numbered by their smallest member and become subsystem concepts, so a
// spurious re-partition rewrites `parent:` on every exported concept.
//
// An explicit flag always wins, including an explicit `--cluster-algorithm
// leiden` on a louvain graph: that is a deliberate switch, not an omission.
func (v *clusterFlagVals) resolve(s *store.Store) (build.ClusterOptions, error) {
	// Validate the flag even when it is about to be overridden by the
	// inherited value, so a typo is still reported.
	algo, err := cluster.ParseAlgorithm(v.algorithm)
	if err != nil {
		return build.ClusterOptions{}, err
	}
	opts := build.ClusterOptions{Algorithm: algo, Resolution: v.resolution, Seed: v.seed}
	if s == nil {
		return opts, nil
	}
	inherited, err := inheritedClusterOptions(s)
	if err != nil {
		return build.ClusterOptions{}, err
	}
	if !v.flags.Changed("cluster-algorithm") {
		opts.Algorithm = inherited.Algorithm
	}
	if !v.flags.Changed("cluster-resolution") {
		opts.Resolution = inherited.Resolution
	}
	if !v.flags.Changed("cluster-seed") {
		opts.Seed = inherited.Seed
	}
	return opts, nil
}

// inheritedClusterOptions returns the settings the graph was last clustered
// with, falling back to graphy's defaults for a store that has never been
// clustered. A meta value that does not parse — a store written by a newer
// graphy, or a corrupted one — degrades to the default rather than refusing to
// build.
func inheritedClusterOptions(s *store.Store) (build.ClusterOptions, error) {
	opts := build.ClusterOptions{
		Algorithm:  cluster.DefaultAlgorithm,
		Resolution: cluster.DefaultResolution,
		Seed:       cluster.DefaultSeed,
	}
	prev, err := s.GetClusterMeta()
	if err != nil {
		return build.ClusterOptions{}, err
	}
	if prev.Algorithm != "" {
		if algo, err := cluster.ParseAlgorithm(prev.Algorithm); err == nil {
			opts.Algorithm = algo
		}
	}
	if prev.Resolution > 0 {
		opts.Resolution = prev.Resolution
	}
	if prev.Seed != 0 {
		opts.Seed = prev.Seed
	}
	return opts, nil
}

// runBuild opens the workspace and runs a build. Cluster options are resolved
// after the store is open, so unset flags can inherit the previous run's
// settings.
func runBuild(opts build.Options, cf *clusterFlagVals) error {
	w, err := openWorkspace()
	if err != nil {
		return err
	}
	s, err := store.Open(w.DBPath())
	if err != nil {
		return err
	}
	defer s.Close()

	if cf != nil {
		co, err := cf.resolve(s)
		if err != nil {
			return err
		}
		opts.Cluster = co
	}

	stats, err := build.Run(w, s, opts)
	if err != nil {
		return err
	}
	if err := writeGraphJSON(w, s, w.JSONPath()); err != nil {
		return err
	}
	cleared := ""
	if stats.CacheCleared > 0 {
		cleared = fmt.Sprintf(", %d cache cleared", stats.CacheCleared)
	}
	failed := ""
	if n := len(stats.Failures); n > 0 {
		failed = fmt.Sprintf(", %d failed", n)
	}
	fmt.Printf("Scanned %d files (%d extracted, %d purged%s%s)\n",
		stats.FilesScanned, stats.FilesExtracted, stats.FilesPurged, cleared, failed)
	for _, fe := range stats.Failures {
		fmt.Fprintf(os.Stderr, "warning: skipped %s\n", fe)
	}
	fmt.Printf("Graph: %d nodes, %d edges, %d communities via %s (%d refs resolved, %d ambiguous, %d dropped)\n",
		stats.Nodes, stats.Edges, stats.Communities, stats.ClusterAlgorithm,
		stats.RefsResolved, stats.RefsAmbiguous, stats.RefsDropped)
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
	export.AddCommand(exportJSONCmd(), exportGraphMLCmd(), exportJGFCmd(), exportCypherCmd(), exportOkfwCmd())
	return export
}

func exportOkfwCmd() *cobra.Command {
	var bundlePath, project, repoURL string
	var minDegree int
	var communitiesOnly, keepStale, noRationale bool
	var rationaleMarkers []string
	cmd := &cobra.Command{
		Use:   "okfw",
		Short: "Export the graph as concepts into an OKF Wiki bundle",
		Long: `Writes communities as parent concepts and significant nodes as child
concepts (vanilla OKF frontmatter + markdown links) into an .okfw bundle,
keeping the bundle's search index and graph database in sync. Re-exports are
idempotent: concept ids and filenames stay stable across runs.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			w, s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()

			var cfg *config.Config
			if bundlePath != "" {
				wikiDir := bundlePath
				if filepath.Base(wikiDir) != config.WikiDirName {
					wikiDir = filepath.Join(wikiDir, config.WikiDirName)
				}
				if fi, err := os.Stat(wikiDir); err != nil || !fi.IsDir() {
					return fmt.Errorf("no %s bundle at %s", config.WikiDirName, bundlePath)
				}
				cfg = config.ConfigFor(wikiDir)
			} else {
				cfg, err = config.Discover("")
				if err != nil {
					return err
				}
			}

			res, err := export.ToOkfw(w.Root, s, export.OkfwOptions{
				Bundle:          cfg,
				Project:         project,
				MinDegree:       minDegree,
				CommunitiesOnly: communitiesOnly,
				KeepStale:       keepStale,
				RepoURL:         repoURL,
				Generator:       "graphy " + version,

				NoRationale:      noRationale,
				RationaleMarkers: rationaleMarkers,
			})
			if err != nil {
				return err
			}
			extra := ""
			if res.Renamed > 0 {
				extra += fmt.Sprintf(", %d renamed", res.Renamed)
			}
			if res.Recovered > 0 {
				extra += fmt.Sprintf(", %d recovered", res.Recovered)
			}
			fmt.Printf("Exported to %s/%s: %d created, %d updated, %d unchanged%s, %d deleted\n",
				cfg.RootPath, res.Dir, res.Created, res.Updated, res.Unchanged, extra, res.Deleted)
			return nil
		},
	}
	cmd.Flags().StringVar(&bundlePath, "bundle", "", "bundle location (default: discover .okfw from cwd)")
	cmd.Flags().StringVar(&project, "project", "", "project folder under graphy/ (default: repo dir name)")
	cmd.Flags().IntVar(&minDegree, "min-degree", 3, "minimum degree for a node to become a concept (does not apply to rationale)")
	cmd.Flags().BoolVar(&communitiesOnly, "communities-only", false, "export only community concepts")
	cmd.Flags().BoolVar(&keepStale, "keep-stale", false, "keep concepts whose graph nodes vanished")
	cmd.Flags().StringVar(&repoURL, "repo-url", "", "web base URL (e.g. https://github.com/org/repo) — turns resource into a commit-pinned permalink")
	cmd.Flags().BoolVar(&noRationale, "no-rationale", false, "skip rationale concepts")
	cmd.Flags().StringSliceVar(&rationaleMarkers, "rationale-markers", export.DefaultRationaleMarkers,
		"which rationale markers become concepts (extraction still captures all of them)")
	return cmd
}

func exportJSONCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "json",
		Short: "Write the portable graph.json",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGraphExport(out, func(w *workspace.Workspace) string { return w.JSONPath() },
				func(f io.Writer, g *export.Graph) error {
					st := g.Stats()
					return model.EncodeGraphJSON(f, g.Root, g.Nodes, g.Edges, st.Communities, st.Files)
				})
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "output path (default .graphy/graph.json)")
	return cmd
}

func exportGraphMLCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "graphml",
		Short: "Write the graph as GraphML",
		Long: `Writes a GraphML document readable by Gephi, yEd, Cytoscape and networkx.
Node and edge attributes (kind, language, source location, community,
relation, confidence) are declared as GraphML keys. The output carries no
timestamp, so an unchanged graph re-exports byte for byte identically.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGraphExport(out, func(w *workspace.Workspace) string { return w.GraphMLPath() },
				func(f io.Writer, g *export.Graph) error {
					return export.WriteGraphML(f, g, "graphy "+version)
				})
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "output path (default .graphy/graph.graphml)")
	return cmd
}

func exportJGFCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "jgf",
		Short: "Write the graph as JSON Graph Format",
		Long: `Writes a JSON Graph Format v2 document (https://jsongraphformat.info):
nodes keyed by graphy node id, edges carrying source/target/relation, and
everything graphy-specific under "metadata". The output carries no timestamp,
so an unchanged graph re-exports byte for byte identically.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGraphExport(out, func(w *workspace.Workspace) string { return w.JGFPath() },
				func(f io.Writer, g *export.Graph) error {
					return export.WriteJGF(f, g, "graphy "+version)
				})
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "output path (default .graphy/graph.jgf.json)")
	return cmd
}

func exportCypherCmd() *cobra.Command {
	var out, graphName string
	var batchSize int
	var noTruncate bool
	cmd := &cobra.Command{
		Use:   "cypher",
		Short: "Write a Cypher load script for Apache AGE (PostgreSQL)",
		Long: `Writes a psql script that loads the graph into Apache AGE. AGE consumes
Cypher only inside SQL cypher() calls, so the output is .sql rather than
.cypher; run it with:

  psql -d <database> -f .graphy/graph.age.sql

Vertices carry one label — the entity kind, since AGE allows a single label
per vertex — and edges are labelled with the relation. The script creates the
graph if it is missing, and by default clears it first so that re-exporting
after a rebuild cannot leave behind vertices for code that no longer exists.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGraphExport(out, func(w *workspace.Workspace) string { return w.CypherPath() },
				func(f io.Writer, g *export.Graph) error {
					name := graphName
					if name == "" {
						name = filepath.Base(g.Root)
					}
					return export.WriteCypher(f, g, export.CypherOptions{
						Generator:  "graphy " + version,
						GraphName:  name,
						BatchSize:  batchSize,
						NoTruncate: noTruncate,
					})
				})
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "output path (default .graphy/graph.age.sql)")
	cmd.Flags().StringVar(&graphName, "graph-name", "", "AGE graph name (default: repo directory name)")
	cmd.Flags().IntVar(&batchSize, "batch-size", export.DefaultCypherBatch, "elements per cypher() statement")
	cmd.Flags().BoolVar(&noTruncate, "no-truncate", false, "append into the existing graph instead of clearing it first")
	return cmd
}

// writeGraphJSON dumps the store into a deterministic graph.json file. Builds
// refresh it directly, without going through the export command.
func writeGraphJSON(w *workspace.Workspace, s *store.Store, path string) error {
	g, err := export.Load(w.Root, s)
	if err != nil {
		return err
	}
	st := g.Stats()
	return workspace.WriteFileAtomic(path, func(f io.Writer) error {
		return model.EncodeGraphJSON(f, g.Root, g.Nodes, g.Edges, st.Communities, st.Files)
	})
}

// runGraphExport loads the store into a snapshot and hands it to a writer. The
// destination is replaced only once the whole file has been written, so
// neither a failed snapshot nor a failed write can damage the previous export.
func runGraphExport(out string, def func(*workspace.Workspace) string, write func(io.Writer, *export.Graph) error) error {
	w, s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()
	g, err := export.Load(w.Root, s)
	if err != nil {
		return err
	}
	path := out
	if path == "" {
		path = def(w)
	}
	if err := workspace.WriteFileAtomic(path, func(f io.Writer) error {
		return write(f, g)
	}); err != nil {
		return err
	}
	st := g.Stats()
	fmt.Printf("Wrote %s (%d nodes, %d edges)\n", path, st.Nodes, st.Edges)
	return nil
}

func installCmd() *cobra.Command {
	var platform string
	var hooks, force bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the graphy skill for an AI coding assistant",
		Long: `Registers graphy with an agent platform so it queries the graph instead of
scanning files. Supported platforms: claude (Claude Code: skill + CLAUDE.md
block, optional PostToolUse hook), gemini (GEMINI.md block), codex
(AGENTS.md block). Managed marker blocks keep re-installs idempotent.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := openWorkspace()
			if err != nil {
				return err
			}
			res, err := skills.Install(skills.Options{
				Root:     w.Root,
				Platform: platform,
				Hooks:    hooks,
				Force:    force,
			})
			if err != nil {
				return err
			}
			for _, f := range res.Written {
				fmt.Println("Wrote", f)
			}
			for _, n := range res.Notes {
				fmt.Println("Note:", n)
			}
			if res.HooksUnsupported {
				fmt.Fprintf(os.Stderr,
					"warning: --hooks installed no hook for %s; the graph will not refresh itself\n", platform)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&platform, "platform", "claude", "target platform: claude, gemini, or codex")
	cmd.Flags().BoolVar(&hooks, "hooks", false,
		"also install an after-edit hook running `graphy update` (claude only; other platforms print manual steps)")
	cmd.Flags().BoolVar(&force, "force", false, "rewrite the skill file even if it exists")
	return cmd
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
