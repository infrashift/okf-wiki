// Package query answers deterministic graph questions: entity resolution,
// shortest paths, entity cards, neighborhood expansion, and a keyword search
// that doubles as the offline fallback for natural-language queries.
package query

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/store"
)

// AmbiguousError reports an entity string that matched several nodes.
type AmbiguousError struct {
	Arg        string
	Candidates []model.Node
}

func (e *AmbiguousError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%q is ambiguous — candidates:\n", e.Arg)
	for _, n := range e.Candidates {
		fmt.Fprintf(&b, "  %s (%s, %s)\n", n.ID, n.Kind, n.File)
	}
	return strings.TrimRight(b.String(), "\n")
}

// ResolveEntity maps a user-supplied string to one node: exact id, then
// unique exact label, then unique case-insensitive substring of a label.
func ResolveEntity(s *store.Store, arg string) (*model.Node, error) {
	if n, err := s.GetNode(arg); err != nil {
		return nil, err
	} else if n != nil {
		return n, nil
	}
	var exact, fuzzy []model.Node
	lower := strings.ToLower(arg)
	if err := s.EachNode(func(n model.Node) error {
		switch {
		case n.Label == arg:
			exact = append(exact, n)
		case strings.Contains(strings.ToLower(n.Label), lower):
			fuzzy = append(fuzzy, n)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	pick := func(list []model.Node) (*model.Node, error) {
		if len(list) == 1 {
			return &list[0], nil
		}
		sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
		if len(list) > 10 {
			list = list[:10]
		}
		return nil, &AmbiguousError{Arg: arg, Candidates: list}
	}
	if len(exact) > 0 {
		return pick(exact)
	}
	if len(fuzzy) > 0 {
		return pick(fuzzy)
	}
	return nil, fmt.Errorf("no entity matches %q", arg)
}

// Hop is one step along a path.
type Hop struct {
	Edge    model.Edge
	Forward bool // false when the edge is traversed against its direction
}

// Path finds a shortest undirected path between two nodes, reporting each
// hop's true edge direction. Returns nil when no path exists.
func Path(s *store.Store, fromID, toID string) ([]Hop, error) {
	if fromID == toID {
		return []Hop{}, nil
	}
	type visit struct {
		prev string
		hop  Hop
	}
	visited := map[string]visit{fromID: {}}
	frontier := []string{fromID}

	neighbors := func(id string) ([]Hop, error) {
		out, err := s.OutEdges(id)
		if err != nil {
			return nil, err
		}
		in, err := s.InEdges(id)
		if err != nil {
			return nil, err
		}
		hops := make([]Hop, 0, len(out)+len(in))
		for _, e := range out {
			hops = append(hops, Hop{Edge: e, Forward: true})
		}
		for _, e := range in {
			hops = append(hops, Hop{Edge: e, Forward: false})
		}
		return hops, nil
	}

	for len(frontier) > 0 {
		var next []string
		for _, id := range frontier {
			hops, err := neighbors(id)
			if err != nil {
				return nil, err
			}
			for _, h := range hops {
				other := h.Edge.Target
				if !h.Forward {
					other = h.Edge.Source
				}
				if _, seen := visited[other]; seen {
					continue
				}
				visited[other] = visit{prev: id, hop: h}
				if other == toID {
					// Reconstruct.
					var path []Hop
					for cur := toID; cur != fromID; {
						v := visited[cur]
						path = append([]Hop{v.hop}, path...)
						cur = v.prev
					}
					return path, nil
				}
				next = append(next, other)
			}
		}
		frontier = next
	}
	return nil, nil
}

// Card is an entity's full detail for explain output.
type Card struct {
	Node           model.Node
	CommunityLabel string
	Out            []model.Edge
	In             []model.Edge
}

// Explain assembles an entity card.
func Explain(s *store.Store, id string) (*Card, error) {
	n, err := s.GetNode(id)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return nil, fmt.Errorf("no node %q", id)
	}
	card := &Card{Node: *n}
	if card.Out, err = s.OutEdges(id); err != nil {
		return nil, err
	}
	if card.In, err = s.InEdges(id); err != nil {
		return nil, err
	}
	if err := s.EachCommunity(func(c store.Community) error {
		if c.ID == n.Community {
			card.CommunityLabel = c.Label
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return card, nil
}

// Subgraph expands seed nodes by BFS to the given depth, optionally filtered
// to a set of relations, returning the induced nodes and edges.
func Subgraph(s *store.Store, seeds []string, depth int, relations []model.Relation) ([]model.Node, []model.Edge, error) {
	allowed := map[model.Relation]bool{}
	for _, r := range relations {
		allowed[r] = true
	}
	keep := func(r model.Relation) bool { return len(allowed) == 0 || allowed[r] }

	include := map[string]bool{}
	frontier := []string{}
	for _, id := range seeds {
		if n, err := s.GetNode(id); err != nil {
			return nil, nil, err
		} else if n != nil && !include[id] {
			include[id] = true
			frontier = append(frontier, id)
		}
	}
	for d := 0; d < depth; d++ {
		var next []string
		for _, id := range frontier {
			out, err := s.OutEdges(id)
			if err != nil {
				return nil, nil, err
			}
			in, err := s.InEdges(id)
			if err != nil {
				return nil, nil, err
			}
			for _, e := range append(out, in...) {
				if !keep(e.Relation) {
					continue
				}
				for _, other := range []string{e.Source, e.Target} {
					if !include[other] {
						include[other] = true
						next = append(next, other)
					}
				}
			}
		}
		frontier = next
	}

	var nodes []model.Node
	if err := s.EachNode(func(n model.Node) error {
		if include[n.ID] {
			nodes = append(nodes, n)
		}
		return nil
	}); err != nil {
		return nil, nil, err
	}
	var edges []model.Edge
	if err := s.EachEdge(func(e model.Edge) error {
		if include[e.Source] && include[e.Target] && keep(e.Relation) {
			edges = append(edges, e)
		}
		return nil
	}); err != nil {
		return nil, nil, err
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	return nodes, edges, nil
}

// KeywordSearch ranks nodes by a case-insensitive match of the query terms
// over label > file > doc, weighted by connectivity. It is the offline
// fallback when no LLM backend is configured.
func KeywordSearch(s *store.Store, q string, limit int) ([]model.Node, error) {
	terms := strings.Fields(strings.ToLower(q))
	if len(terms) == 0 {
		return nil, nil
	}
	type scored struct {
		n     model.Node
		score float64
	}
	var hits []scored
	err := s.EachNode(func(n model.Node) error {
		label := strings.ToLower(n.Label)
		file := strings.ToLower(n.File)
		doc := strings.ToLower(n.Doc)
		score := 0.0
		for _, t := range terms {
			switch {
			case label == t:
				score += 10
			case strings.Contains(label, t):
				score += 5
			case strings.Contains(file, t):
				score += 2
			case strings.Contains(doc, t):
				score += 1
			}
		}
		if score > 0 {
			score *= 1 + math.Log1p(float64(n.Degree))
			hits = append(hits, scored{n, score})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].n.ID < hits[j].n.ID
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]model.Node, len(hits))
	for i, h := range hits {
		out[i] = h.n
	}
	return out, nil
}
