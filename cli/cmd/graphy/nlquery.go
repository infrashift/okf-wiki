package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"okfw/cli/internal/graphy/llm"
	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/query"
)

func queryCmd() *cobra.Command {
	var topN, depth, limit int
	var noLLM, asJSON, tests bool
	var modelFlag string
	cmd := &cobra.Command{
		Use:   "query <question>",
		Short: "Answer a natural-language question about the codebase",
		Long: `Interprets the question with a configured LLM backend (ANTHROPIC_API_KEY,
GEMINI_API_KEY, or OPENAI_API_KEY — first match wins) which selects seed
nodes; the graph expansion itself is deterministic. Without an API key the
query degrades to keyword search and still works offline.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()

			var backend llm.Backend
			if !noLLM {
				backend, err = llm.FromEnv(modelFlag)
				if err != nil && err != llm.ErrNoBackend {
					return err
				}
			}

			// Leave Depth zero unless the user actually passed --depth, so an
			// untouched flag lets the planner pick 2 for a flow question while
			// an explicit --depth is honoured instead of being overridden.
			opts := query.NLOptions{TopN: topN, Limit: limit, IncludeTests: tests}
			if cmd.Flags().Changed("depth") {
				if depth < 1 || depth > query.MaxDepth {
					return fmt.Errorf("--depth must be between 1 and %d", query.MaxDepth)
				}
				opts.Depth = depth
			}

			res, err := query.NL(cmd.Context(), s, backend, args[0], opts)
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(os.Stdout).Encode(res)
			}
			printNLResult(s, res)
			return nil
		},
	}
	cmd.Flags().IntVar(&topN, "top", 1500, "candidate index size sent to the LLM")
	cmd.Flags().IntVar(&depth, "depth", 1,
		fmt.Sprintf("expansion depth around seed nodes, 1-%d (default: let the planner choose)", query.MaxDepth))
	cmd.Flags().IntVar(&limit, "limit", query.DefaultLimit, "maximum entities returned, seeds first then most connected")
	cmd.Flags().BoolVar(&tests, "tests", false, "include entities declared in test files")
	cmd.Flags().BoolVar(&noLLM, "no-llm", false, "skip the LLM and use keyword search")
	cmd.Flags().StringVar(&modelFlag, "model", "", "override the backend's default model")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the raw subgraph as JSON")
	return cmd
}

func printNLResult(s storeReader, res *query.NLResult) {
	fmt.Printf("mode: %s — %d nodes, %d edges\n\n", res.Mode, len(res.Nodes), len(res.Edges))
	if len(res.Nodes) == 0 {
		fmt.Println("No matches. Try different terms or run `graphy build` first.")
		return
	}

	seeds := map[string]bool{}
	for _, id := range res.Seeds {
		seeds[id] = true
	}
	// Seeds first, then by degree.
	nodes := append([]model.Node(nil), res.Nodes...)
	sort.SliceStable(nodes, func(i, j int) bool {
		si, sj := seeds[nodes[i].ID], seeds[nodes[j].ID]
		if si != sj {
			return si
		}
		return nodes[i].Degree > nodes[j].Degree
	})
	for _, n := range nodes {
		marker := "  "
		if seeds[n.ID] {
			marker = "* "
		}
		loc := ""
		if n.File != "" {
			loc = fmt.Sprintf("  %s:%d", n.File, n.Span.StartLine)
		}
		fmt.Printf("%s%s (%s, degree %d)%s\n", marker, n.Label, n.Kind, n.Degree, loc)
		if n.Doc != "" {
			fmt.Printf("      %s\n", n.Doc)
		}
	}
	if res.More {
		fmt.Printf("  … %d more entities (raise --limit)\n", res.Total-len(res.Nodes))
	}

	if len(res.Edges) > 0 {
		fmt.Println("\nKey relationships:")
		edges := append([]model.Edge(nil), res.Edges...)
		sort.Slice(edges, func(i, j int) bool {
			return edges[i].Source+edges[i].Target < edges[j].Source+edges[j].Target
		})
		max := len(edges)
		if max > 30 {
			max = 30
		}
		for _, e := range edges[:max] {
			fmt.Printf("  %s -[%s]-> %s (%s)\n", labelOf(s, e.Source), e.Relation, labelOf(s, e.Target), e.Confidence)
		}
		if len(edges) > 30 {
			fmt.Printf("  … %d more (use --json for all)\n", len(edges)-30)
		}
	}
}

func labelOf(s storeReader, id string) string {
	if n, _ := s.GetNode(id); n != nil {
		return n.Label
	}
	return id
}
