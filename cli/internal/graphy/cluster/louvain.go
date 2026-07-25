package cluster

import (
	"math/rand/v2"

	"gonum.org/v1/gonum/graph/community"
	"gonum.org/v1/gonum/graph/simple"
)

// Louvain runs gonum's Louvain modularization over the undirected projection
// of the graph. The fixed Seed makes assignments reproducible across runs.
//
// Louvain can return communities that are internally disconnected: a node
// that bridged two halves may be moved away, leaving the remainder in pieces
// with no path between them. Leiden exists to fix exactly that, and is the
// default; Louvain is kept because it is what earlier graphs were built with.
type Louvain struct {
	Resolution float64 // 0 → default 1.0
	Seed       uint64
}

// Detect implements Detector. Community ids are renumbered deterministically:
// communities are ordered by their lexicographically-smallest member.
func (l Louvain) Detect(nodeIDs []string, edges [][2]string) (map[string]int, error) {
	sorted := sortUnique(nodeIDs)
	if len(sorted) == 0 {
		return map[string]int{}, nil
	}
	resolution := l.Resolution
	if resolution == 0 {
		resolution = DefaultResolution
	}

	// Deterministic id mapping: sorted node ids → dense int64 indices.
	index := make(map[string]int64, len(sorted))
	for i, id := range sorted {
		index[id] = int64(i)
	}

	g := simple.NewUndirectedGraph()
	for _, id := range sorted {
		g.AddNode(simple.Node(index[id]))
	}
	for _, e := range edges {
		a, aok := index[e[0]]
		b, bok := index[e[1]]
		if !aok || !bok || a == b {
			continue
		}
		g.SetEdge(simple.Edge{F: simple.Node(a), T: simple.Node(b)})
	}

	src := rand.NewPCG(l.Seed, l.Seed)
	reduced := community.Modularize(g, resolution, src)

	member := make([]int32, len(sorted))
	for c, nodes := range reduced.Communities() {
		for _, n := range nodes {
			member[n.ID()] = int32(c)
		}
	}
	return renumber(sorted, member), nil
}
