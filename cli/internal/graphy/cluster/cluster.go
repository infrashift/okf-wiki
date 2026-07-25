// Package cluster assigns graph nodes to communities. Detection is LLM-free.
//
// The Detector interface isolates the algorithm choice: today gonum's Louvain
// modularization; Leiden can slot in behind the same interface when a
// credible Go implementation exists.
package cluster

import (
	"math/rand/v2"
	"sort"

	"gonum.org/v1/gonum/graph/community"
	"gonum.org/v1/gonum/graph/simple"
)

// Detector assigns a community id to every node id.
type Detector interface {
	Detect(nodeIDs []string, edges [][2]string) (map[string]int, error)
}

// Louvain runs gonum's Louvain modularization over the undirected projection
// of the graph. The fixed Seed makes assignments reproducible across runs.
type Louvain struct {
	Resolution float64 // 0 → default 1.0
	Seed       uint64
}

// Detect implements Detector. Community ids are renumbered deterministically:
// communities are ordered by their lexicographically-smallest member.
func (l Louvain) Detect(nodeIDs []string, edges [][2]string) (map[string]int, error) {
	if len(nodeIDs) == 0 {
		return map[string]int{}, nil
	}
	resolution := l.Resolution
	if resolution == 0 {
		resolution = 1.0
	}

	// Deterministic id mapping: sorted node ids → dense int64 indices.
	sorted := append([]string(nil), nodeIDs...)
	sort.Strings(sorted)
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

	assign := make(map[string]int, len(sorted))
	type comm struct {
		smallest string
		members  []string
	}
	var comms []comm
	for _, nodes := range reduced.Communities() {
		var members []string
		for _, n := range nodes {
			members = append(members, sorted[n.ID()])
		}
		sort.Strings(members)
		comms = append(comms, comm{smallest: members[0], members: members})
	}
	sort.Slice(comms, func(i, j int) bool { return comms[i].smallest < comms[j].smallest })
	for id, c := range comms {
		for _, member := range c.members {
			assign[member] = id
		}
	}
	return assign, nil
}
