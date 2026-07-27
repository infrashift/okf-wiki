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
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit raw hops as JSON")
	return cmd
}

func explainCmd() *cobra.Command {
	var tf traverseFlags
	var direction string
	cmd := &cobra.Command{
		Use:   "explain <entity>",
		Short: "Show an entity's details and connections",
		Long: `Prints an entity card: location, signature, doc, community, and its edges.

Use --rel and --direction to narrow the edge list; on a hub entity the full card
is hundreds of edges. For the impact question specifically, ` + "`graphy callers`" + `
is the same query with the direction already chosen.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rels, _, err := tf.parse()
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
			card, err := query.ExplainFiltered(s, n.ID, query.EdgeQuery{
				Direction: dir, Relations: rels, Limit: tf.limit, IncludeTests: tf.tests,
			})
			if err != nil {
				return err
			}
			if tf.asJSON {
				return json.NewEncoder(os.Stdout).Encode(card)
			}
			printCard(s, card)
			return nil
		},
	}
	tf.bind(cmd, false)
	cmd.Flags().StringVar(&direction, "direction", string(query.Both), "which edges to list: out, in, or both")
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
	if n.Body != "" && n.Body != n.Doc {
		for i, line := range strings.Split(n.Body, "\n") {
			label := "  body:    "
			if i > 0 {
				label = "           "
			}
			fmt.Printf("%s  %s\n", label, line)
		}
	}
	if card.CommunityLabel != "" {
		fmt.Printf("  community: %s (#%d)\n", card.CommunityLabel, n.Community)
	}
	fmt.Printf("  degree:    %d\n", n.Degree)
	printEdgeGroup(s, "Outgoing", card.Out, true)
	printEdgeGroup(s, "Incoming", card.In, false)
	if card.Truncated {
		fmt.Printf("  … %d of %d edges shown (raise --limit, or narrow with --rel)\n",
			len(card.Out)+len(card.In), card.Total)
	}
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
	var section string
	var list bool
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Print GRAPH_REPORT.md",
		Long: `Prints the generated graph report. On a large repository the whole document
runs to thousands of tokens, most of them the community listing, so --section
takes just the part you need and --sections lists what is available.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := openWorkspace()
			if err != nil {
				return err
			}
			content, err := os.ReadFile(w.ReportPath())
			if err != nil {
				return fmt.Errorf("no report at %s (run `graphy build`)", w.ReportPath())
			}
			switch {
			case list:
				for _, name := range reportSections(string(content)) {
					fmt.Println(name)
				}
				return nil
			case section != "":
				body, ok := reportSection(string(content), section)
				if !ok {
					return fmt.Errorf("no section %q (try --sections)", section)
				}
				fmt.Print(body)
				return nil
			}
			fmt.Print(string(content))
			if !strings.HasSuffix(string(content), "\n") {
				fmt.Println()
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&section, "section", "", "print only this section, by heading text")
	cmd.Flags().BoolVar(&list, "sections", false, "list the report's section headings")
	return cmd
}

// reportSections lists the report's top-level headings, in document order.
//
// Depth-limited on purpose: the community listing contributes one H3 per
// community, so listing every heading on a real repository is longer than the
// document the flag exists to avoid printing. Those subsections are still
// reachable — asking for "Communities" returns them all.
func reportSections(content string) []string {
	var out []string
	for _, line := range strings.Split(content, "\n") {
		if h, ok := headingText(line); ok && headingLevel(line) <= 2 {
			out = append(out, h)
		}
	}
	return out
}

// reportSection extracts one heading's content, matched case-insensitively,
// through to the next heading of the same or a shallower level.
func reportSection(content, want string) (string, bool) {
	lines := strings.Split(content, "\n")
	want = strings.ToLower(strings.TrimSpace(want))
	start, level := -1, 0
	for i, line := range lines {
		h, ok := headingText(line)
		if !ok {
			continue
		}
		if start < 0 {
			if strings.ToLower(h) == want {
				start, level = i, headingLevel(line)
			}
			continue
		}
		if headingLevel(line) <= level {
			return strings.Join(lines[start:i], "\n"), true
		}
	}
	if start < 0 {
		return "", false
	}
	return strings.Join(lines[start:], "\n"), true
}

func headingLevel(line string) int {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	return n
}

func headingText(line string) (string, bool) {
	n := headingLevel(line)
	if n == 0 || n >= len(line) || line[n] != ' ' {
		return "", false
	}
	return strings.TrimSpace(line[n+1:]), true
}
