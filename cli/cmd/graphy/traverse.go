package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/query"
)

// Deterministic traversal commands. Everything here answers a question exactly,
// with no model in the loop and a bound on the answer's size — which is what
// separates them from `graphy query`, whose seeds come from an LLM plan or a
// keyword score and whose result is therefore neither reproducible nor bounded.

// traverseFlags are the filters every traversal command shares, so they cannot
// drift apart between commands or between the CLI and the MCP server.
type traverseFlags struct {
	relations []string
	kinds     []string
	limit     int
	tests     bool
	asJSON    bool
}

func (t *traverseFlags) bind(cmd *cobra.Command, withKinds bool) {
	cmd.Flags().StringSliceVar(&t.relations, "rel", nil,
		"restrict to these relations ("+strings.Join(model.RelationNames(), ", ")+")")
	if withKinds {
		cmd.Flags().StringSliceVar(&t.kinds, "kind", nil,
			"restrict to these node kinds ("+strings.Join(model.KindNames(), ", ")+")")
	}
	cmd.Flags().IntVar(&t.limit, "limit", query.DefaultLimit, "maximum results")
	cmd.Flags().BoolVar(&t.tests, "tests", false, "include entities declared in test files")
	cmd.Flags().BoolVar(&t.asJSON, "json", false, "emit JSON")
}

func (t *traverseFlags) parse() ([]model.Relation, []model.Kind, error) {
	rels, err := query.ParseRelations(t.relations)
	if err != nil {
		return nil, nil, err
	}
	kinds, err := query.ParseKinds(t.kinds)
	if err != nil {
		return nil, nil, err
	}
	return rels, kinds, nil
}

func neighborsCmd() *cobra.Command {
	var tf traverseFlags
	var depth int
	var direction string
	cmd := &cobra.Command{
		Use:   "neighbors <entity>",
		Short: "Expand the neighbourhood of an entity you already know",
		Long: `Returns the entities around one you name, to a bounded depth, filtered by
relation and kind. Unlike ` + "`graphy query`" + `, the starting point is yours and the
result is deterministic — use this when you already know what you care about.

The entity may be a node id, a name, or a repo-relative file path.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rels, kinds, err := tf.parse()
			if err != nil {
				return err
			}
			dir, err := query.ParseDirection(direction)
			if err != nil {
				return err
			}
			_, s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()

			n, err := query.ResolveEntity(s, args[0])
			if err != nil {
				return err
			}
			res, err := query.Neighbors(s, []string{n.ID}, query.NeighborOptions{
				Depth: depth, Direction: dir, Relations: rels, Kinds: kinds,
				Limit: tf.limit, IncludeTests: tf.tests,
			})
			if err != nil {
				return err
			}
			if tf.asJSON {
				return json.NewEncoder(os.Stdout).Encode(res)
			}
			fmt.Printf("%s (%s) — %d entities within depth %d\n", n.Label, n.Kind, res.Total, max(depth, 1))
			for _, m := range res.Nodes {
				if m.ID == n.ID {
					continue
				}
				fmt.Printf("  %-40s %-10s %s\n", truncate(m.Label, 40), m.Kind, location(m))
			}
			if res.More {
				fmt.Printf("  … %d more (raise --limit)\n", res.Total-len(res.Nodes))
			}
			return nil
		},
	}
	tf.bind(cmd, true)
	cmd.Flags().IntVar(&depth, "depth", 1, fmt.Sprintf("expansion depth 1-%d", query.MaxDepth))
	cmd.Flags().StringVar(&direction, "direction", string(query.Both), "out, in, or both")
	return cmd
}

func callersCmd() *cobra.Command {
	var tf traverseFlags
	cmd := &cobra.Command{
		Use:   "callers <entity>",
		Short: "What depends on an entity (impact analysis)",
		Long: `Lists the incoming edges of an entity: what calls it, imports it, implements
it, tests it, or documents it. This is the impact question — "what breaks if I
change this" — answered directly rather than by filtering a full entity card.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rels, _, err := tf.parse()
			if err != nil {
				return err
			}
			return runEdges(args[0], query.EdgeQuery{
				Direction: query.In, Relations: rels,
				Limit: tf.limit, IncludeTests: tf.tests,
			}, tf.asJSON, "depends on")
		},
	}
	tf.bind(cmd, false)
	return cmd
}

func usesCmd() *cobra.Command {
	var tf traverseFlags
	cmd := &cobra.Command{
		Use:   "uses <entity>",
		Short: "What an entity depends on",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rels, _, err := tf.parse()
			if err != nil {
				return err
			}
			return runEdges(args[0], query.EdgeQuery{
				Direction: query.Out, Relations: rels,
				Limit: tf.limit, IncludeTests: tf.tests,
			}, tf.asJSON, "used by")
		},
	}
	tf.bind(cmd, false)
	return cmd
}

// runEdges is the shared body of callers and uses.
func runEdges(entity string, q query.EdgeQuery, asJSON bool, verb string) error {
	_, s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()

	n, err := query.ResolveEntity(s, entity)
	if err != nil {
		return err
	}
	res, err := query.Edges(s, n.ID, q)
	if err != nil {
		return err
	}
	if asJSON {
		return json.NewEncoder(os.Stdout).Encode(res)
	}
	if len(res.Hits) == 0 {
		fmt.Printf("%s (%s): nothing %s it\n", n.Label, n.Kind, verb)
		return nil
	}
	fmt.Printf("%s (%s) — %d\n", n.Label, n.Kind, res.Total)
	for _, h := range res.Hits {
		fmt.Printf("  %s %-38s %-10s %s (%s)\n", padRelation(h.Edge.Relation),
			truncate(h.Other.Label, 38), h.Other.Kind, location(h.Other), h.Edge.Confidence)
	}
	if res.More {
		fmt.Printf("  … %d more (raise --limit)\n", res.Total-len(res.Hits))
	}
	return nil
}

func whyCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "why <entity>",
		Short: "The recorded reasoning behind an entity",
		Long: `Shows the WHY/DECISION/RATIONALE comments and architecture decision records
attached to an entity. Read this before changing unfamiliar code.

This is a deterministic filter on the graph's ` + "`explains`" + ` edges — the MCP server
has always exposed it, while the CLI could only reach it by phrasing the
question in natural language and hoping the planner picked the right seeds.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()

			n, err := query.ResolveEntity(s, args[0])
			if err != nil {
				return err
			}
			res, err := query.Rationale(s, n.ID)
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(os.Stdout).Encode(res)
			}
			if len(res.Hits) == 0 {
				fmt.Printf("%s (%s): no recorded rationale\n", n.Label, n.Kind)
				return nil
			}
			fmt.Printf("%s (%s)\n", n.Label, n.Kind)
			for _, h := range res.Hits {
				fmt.Printf("\n  %s\n", h.Other.Label)
				if h.Other.File != "" {
					fmt.Printf("  %s\n", location(h.Other))
				}
				body := h.Other.Body
				if body == "" {
					body = h.Other.Doc
				}
				for _, line := range strings.Split(strings.TrimRight(body, "\n"), "\n") {
					fmt.Printf("    %s\n", line)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON")
	return cmd
}

func nodesCmd() *cobra.Command {
	var file string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "nodes --file <path>",
		Short: "List the entities declared in a file",
		Long: `Maps a repo-relative file path to the entities extracted from it, in source
order. This is the entry point when you are working in a file and want into the
graph without guessing a symbol name.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return fmt.Errorf("--file is required")
			}
			_, s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()

			nodes, err := query.FileNodes(s, file)
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(os.Stdout).Encode(nodes)
			}
			for _, n := range nodes {
				fmt.Printf("  %-6d %-10s %-44s %s\n", n.Span.StartLine, n.Kind, truncate(n.Label, 44), n.ID)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "repo-relative file path")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON")
	return cmd
}

func conceptCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "concept <entity>",
		Short: "The wiki concept documenting an entity",
		Long: `Resolves a code entity to the OKF concept ` + "`graphy export okfw`" + ` wrote for it,
so you can read its documentation, its rationale, and the prose around it.

This is the crossing point between the two tools: the graph knows node ids and
the wiki knows concept ids, and the export records the correspondence. The
reverse direction is ` + "`okfw search --node-id <id>`" + `.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()

			n, err := query.ResolveEntity(s, args[0])
			if err != nil {
				return err
			}
			c, err := query.ConceptFor(s, n.ID)
			if err != nil {
				return err
			}
			if c == nil {
				return fmt.Errorf("%s has no exported concept (run `graphy export okfw`; entities below --min-degree are not exported)", n.Label)
			}
			if asJSON {
				return json.NewEncoder(os.Stdout).Encode(c)
			}
			fmt.Printf("%s\n", n.Label)
			fmt.Printf("  node:    %s\n", c.NodeID)
			fmt.Printf("  concept: %s\n", c.ConceptID)
			if c.Path != "" {
				fmt.Printf("  path:    %s\n", c.Path)
			} else {
				fmt.Printf("  path:    %s (bundle unknown — re-run the export)\n", c.RelPath)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON")
	return cmd
}

func location(n model.Node) string {
	if n.File == "" {
		return ""
	}
	if n.Span.StartLine > 0 {
		return fmt.Sprintf("%s:%d", n.File, n.Span.StartLine)
	}
	return n.File
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
