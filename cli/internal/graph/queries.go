package graph

import "sort"

// The queries below used to be served from a persisted bbolt store. They are
// plain graph operations and need no storage: the graph they run against is
// rebuilt from the concepts on disk, which is a pure function of the files and
// cannot disagree with them. A cache could, and every rule that existed to stop
// it drifting — prune on delete, sync after a bulk write, open-existing rather
// than create — was a rule about the cache rather than about the knowledge.

// Backlinks returns the nodes whose body links to id, sorted by title so the
// answer does not vary with map iteration order.
func (g *Graph) Backlinks(id string) []*Node {
	var out []*Node
	for _, src := range g.backlinksTo(id) {
		if n := g.nodes[src]; n != nil {
			out = append(out, n)
		}
	}
	SortNodes(out)
	return out
}

// ByTag returns every node carrying tag, matched exactly.
func (g *Graph) ByTag(tag string) []*Node {
	var out []*Node
	for _, n := range g.nodes {
		for _, t := range n.Tags {
			if t == tag {
				out = append(out, n)
				break
			}
		}
	}
	SortNodes(out)
	return out
}

// Dangling returns links and parent references pointing at ids no concept
// declares.
//
// Reported, never repaired: OKF section 6.1 requires consumers to tolerate a
// link whose target does not exist, and `okfw delete` leaves them on purpose.
func (g *Graph) Dangling() []Edge {
	var out []Edge
	for _, n := range g.nodes {
		if n.Parent != "" && g.nodes[n.Parent] == nil {
			out = append(out, Edge{From: n.ID, To: n.Parent, Label: "parent"})
		}
		for _, l := range n.Links {
			if g.nodes[l] == nil {
				out = append(out, Edge{From: n.ID, To: l, Label: "link"})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out
}

// SortNodes orders nodes by title then id, so every listing is stable.
func SortNodes(ns []*Node) {
	sort.Slice(ns, func(i, j int) bool {
		if ns[i].Title != ns[j].Title {
			return ns[i].Title < ns[j].Title
		}
		return ns[i].ID < ns[j].ID
	})
}
