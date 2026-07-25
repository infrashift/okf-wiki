package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/query"
)

func pathCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "path <from> <to>",
		Short: "Show a shortest path between two entities",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()

			from, err := query.ResolveEntity(s, args[0])
			if err != nil {
				return err
			}
			to, err := query.ResolveEntity(s, args[1])
			if err != nil {
				return err
			}
			hops, err := query.Path(s, from.ID, to.ID)
			if err != nil {
				return err
			}
			if hops == nil {
				return fmt.Errorf("no path between %s and %s", from.Label, to.Label)
			}
			if asJSON {
				return json.NewEncoder(os.Stdout).Encode(hops)
			}
			fmt.Printf("%s (%d hops)\n", from.Label, len(hops))
			cur := from.ID
			for _, h := range hops {
				next, arrow := h.Edge.Target, fmt.Sprintf("-[%s, %s]->", h.Edge.Relation, h.Edge.Confidence)
				if !h.Forward {
					next, arrow = h.Edge.Source, fmt.Sprintf("<-[%s, %s]-", h.Edge.Relation, h.Edge.Confidence)
				}
				n, _ := s.GetNode(next)
				label := next
				if n != nil {
					label = fmt.Sprintf("%s (%s)", n.Label, n.Kind)
				}
				fmt.Printf("  %s %s\n", arrow, label)
				cur = next
			}
			_ = cur
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit raw hops as JSON")
	return cmd
}

func explainCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "explain <entity>",
		Short: "Show an entity's details and connections",
		Args:  cobra.ExactArgs(1),
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
			card, err := query.Explain(s, n.ID)
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(os.Stdout).Encode(card)
			}
			printCard(s, card)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the card as JSON")
	return cmd
}

func printCard(s storeReader, card *query.Card) {
	n := card.Node
	fmt.Printf("%s  (%s, %s)\n", n.Label, n.Kind, n.Language)
	if n.File != "" {
		fmt.Printf("  location:  %s:%d\n", n.File, n.Span.StartLine)
	}
	if n.Signature != "" {
		fmt.Printf("  signature: %s\n", n.Signature)
	}
	if n.Doc != "" {
		fmt.Printf("  doc:       %s\n", n.Doc)
	}
	if card.CommunityLabel != "" {
		fmt.Printf("  community: %s (#%d)\n", card.CommunityLabel, n.Community)
	}
	fmt.Printf("  degree:    %d\n", n.Degree)
	printEdgeGroup(s, "Outgoing", card.Out, true)
	printEdgeGroup(s, "Incoming", card.In, false)
}

// storeReader is the minimal lookup surface the printers need.
type storeReader interface {
	GetNode(id string) (*model.Node, error)
}

func printEdgeGroup(s storeReader, title string, edges []model.Edge, forward bool) {
	if len(edges) == 0 {
		return
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Relation != edges[j].Relation {
			return edges[i].Relation < edges[j].Relation
		}
		return edges[i].Source+edges[i].Target < edges[j].Source+edges[j].Target
	})
	fmt.Printf("  %s:\n", title)
	for _, e := range edges {
		otherID := e.Target
		if !forward {
			otherID = e.Source
		}
		label := otherID
		if n, _ := s.GetNode(otherID); n != nil {
			label = fmt.Sprintf("%s (%s)", n.Label, n.Kind)
		}
		fmt.Printf("    %s %s (%s)\n", padRelation(e.Relation), label, e.Confidence)
	}
}

func padRelation(r model.Relation) string {
	return fmt.Sprintf("%-11s", string(r))
}

func reportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "report",
		Short: "Print GRAPH_REPORT.md",
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := openWorkspace()
			if err != nil {
				return err
			}
			content, err := os.ReadFile(w.ReportPath())
			if err != nil {
				return fmt.Errorf("no report at %s (run `graphy build`)", w.ReportPath())
			}
			fmt.Print(string(content))
			if !strings.HasSuffix(string(content), "\n") {
				fmt.Println()
			}
			return nil
		},
	}
}
