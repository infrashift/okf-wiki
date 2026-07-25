package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"okfw/cli/internal/graphy/llm"
	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/store"
)

// NLOptions tunes a natural-language query.
type NLOptions struct {
	TopN  int // candidate index size sent to the LLM (default 1500)
	Depth int // default expansion depth when the LLM doesn't pick one (default 1)
}

// NLResult is a natural-language query answer.
type NLResult struct {
	Mode  string // "llm:<backend>" or "keyword"
	Seeds []string
	Nodes []model.Node
	Edges []model.Edge
}

const nlSystemPrompt = `You are a retrieval planner over a code knowledge graph.
The user asks a question about a codebase. You receive an index of graph nodes
as TSV lines: id, kind, label, file, community, summary.

Nodes of kind "rationale" are recorded intent — a WHY/DECISION/TODO comment or
an architecture decision record — and carry a summary in the last column. They
are linked to the code they justify by "explains" edges. Prefer them as seeds
when the question asks why something is the way it is.

Reply with ONLY a JSON object, no prose, no markdown fences:
{"node_ids": ["<id>", ...], "depth": 1}

Pick the 3-10 node ids most relevant to the question (seeds for a graph
expansion). "depth" is 1 or 2: use 2 only when the question is about flows or
relationships spanning several steps. Optionally add
"relations": ["calls","imports","inherits","implements","references","contains","defines","explains"]
to filter which edge kinds matter.`

// plan is what the LLM must produce.
type plan struct {
	NodeIDs   []string         `json:"node_ids"`
	Depth     int              `json:"depth"`
	Relations []model.Relation `json:"relations"`
}

// NL answers a question: an LLM (when configured) selects seed nodes from a
// bounded candidate index, then the expansion is a deterministic, auditable
// BFS. Falls back to keyword search with no backend or an unusable reply.
func NL(ctx context.Context, s *store.Store, backend llm.Backend, question string, opts NLOptions) (*NLResult, error) {
	if opts.TopN <= 0 {
		opts.TopN = 1500
	}
	if opts.Depth <= 0 {
		opts.Depth = 1
	}

	if backend == nil {
		return keywordFallback(s, question, opts)
	}

	index, err := candidateIndex(s, opts.TopN)
	if err != nil {
		return nil, err
	}
	user := fmt.Sprintf("Question: %s\n\nNode index:\n%s", question, index)

	p, perr := askForPlan(ctx, s, backend, user)
	if perr != nil {
		if errors.Is(perr, context.Canceled) || errors.Is(perr, context.DeadlineExceeded) {
			return nil, perr
		}
		res, ferr := keywordFallback(s, question, opts)
		if ferr != nil {
			return nil, ferr
		}
		res.Mode += fmt.Sprintf(" (llm unusable: %v)", perr)
		return res, nil
	}

	depth := p.Depth
	if depth < 1 || depth > 2 {
		depth = opts.Depth
	}
	nodes, edges, err := Subgraph(s, p.NodeIDs, depth, p.Relations)
	if err != nil {
		return nil, err
	}
	return &NLResult{
		Mode:  "llm:" + backend.Name(),
		Seeds: p.NodeIDs,
		Nodes: nodes,
		Edges: edges,
	}, nil
}

// askForPlan requests the JSON plan, retrying once on malformed output.
func askForPlan(ctx context.Context, s *store.Store, backend llm.Backend, user string) (*plan, error) {
	reply, err := backend.Complete(ctx, nlSystemPrompt, user)
	if err != nil {
		return nil, err
	}
	p, err := parsePlan(s, reply)
	if err == nil {
		return p, nil
	}
	retry := user + "\n\nYour previous reply was not usable (" + err.Error() +
		"). Reply with ONLY the JSON object described in the instructions."
	reply, rerr := backend.Complete(ctx, nlSystemPrompt, retry)
	if rerr != nil {
		return nil, rerr
	}
	return parsePlan(s, reply)
}

// parsePlan extracts and validates the JSON plan, dropping unknown ids.
func parsePlan(s *store.Store, reply string) (*plan, error) {
	text := strings.TrimSpace(reply)
	// Tolerate fenced or prose-wrapped JSON: use the outermost braces.
	if i := strings.IndexByte(text, '{'); i >= 0 {
		if j := strings.LastIndexByte(text, '}'); j > i {
			text = text[i : j+1]
		}
	}
	var p plan
	if err := json.Unmarshal([]byte(text), &p); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	var valid []string
	for _, id := range p.NodeIDs {
		if n, err := s.GetNode(id); err == nil && n != nil {
			valid = append(valid, id)
		}
	}
	if len(valid) == 0 {
		return nil, fmt.Errorf("no valid node ids in plan")
	}
	p.NodeIDs = valid
	return &p, nil
}

func keywordFallback(s *store.Store, question string, opts NLOptions) (*NLResult, error) {
	hits, err := KeywordSearch(s, question, 8)
	if err != nil {
		return nil, err
	}
	seeds := make([]string, len(hits))
	for i, h := range hits {
		seeds[i] = h.ID
	}
	nodes, edges, err := Subgraph(s, seeds, opts.Depth, nil)
	if err != nil {
		return nil, err
	}
	return &NLResult{Mode: "keyword", Seeds: seeds, Nodes: nodes, Edges: edges}, nil
}

// candidateIndex renders the top-N nodes by degree as compact TSV, keeping
// the LLM prompt bounded regardless of repo size.
// rationaleBudget is the share of the candidate index reserved for rationale
// nodes, as a divisor of topN.
const rationaleBudget = 10

func candidateIndex(s *store.Store, topN int) (string, error) {
	var nodes, rationale []model.Node
	if err := s.EachNode(func(n model.Node) error {
		switch n.Kind {
		case model.KindHeading:
			return nil
		case model.KindRationale:
			rationale = append(rationale, n)
		default:
			nodes = append(nodes, n)
		}
		return nil
	}); err != nil {
		return "", err
	}
	byDegree := func(ns []model.Node) {
		sort.Slice(ns, func(i, j int) bool {
			if ns[i].Degree != ns[j].Degree {
				return ns[i].Degree > ns[j].Degree
			}
			return ns[i].ID < ns[j].ID
		})
	}
	byDegree(nodes)
	if len(nodes) > topN {
		nodes = nodes[:topN]
	}
	// Rationale nodes are degree-2 leaves and would be crowded out of a purely
	// degree-ranked index — yet they are exactly what a "why" question needs
	// as a seed. Reserve a slice of the budget for them.
	sort.Slice(rationale, func(i, j int) bool { return rationale[i].ID < rationale[j].ID })
	if cap := topN / rationaleBudget; len(rationale) > cap {
		rationale = rationale[:cap]
	}

	var b strings.Builder
	for _, n := range nodes {
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%d\t\n", n.ID, n.Kind, n.Label, n.File, n.Community)
	}
	for _, n := range rationale {
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%d\t%s\n",
			n.ID, n.Kind, n.Label, n.File, n.Community, truncate(n.Doc, 40))
	}
	return b.String(), nil
}

// truncate shortens a summary so the prompt stays bounded.
func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\t", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
