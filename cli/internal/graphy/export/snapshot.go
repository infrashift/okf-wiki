package export

import (
	"sort"

	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/store"
)

// Graph is a materialized, deterministically ordered snapshot of a store.
//
// Every whole-graph exporter — graph.json, GraphML, JGF, Cypher — needs the
// same three things (all nodes, all edges, each node's community) in a stable
// order, so they share one loader. The ordering is not cosmetic: these
// documents are meant to be diffed and committed, and output that depended on
// bbolt's iteration order would churn on every run even when the graph is
// unchanged.
type Graph struct {
	Root        string
	Nodes       []model.Node
	Edges       []model.Edge
	Communities []store.Community // ascending by id, members sorted
	Files       int

	memberOf map[string]int // node id → community id
	labels   map[int]string // community id → label
}

// NewGraph assembles a snapshot from data already in memory, sorting it into
// the canonical order.
func NewGraph(root string, nodes []model.Node, edges []model.Edge, communities []store.Community, files int) *Graph {
	g := &Graph{
		Root:        root,
		Nodes:       nodes,
		Edges:       edges,
		Communities: communities,
		Files:       files,
		memberOf:    make(map[string]int, len(nodes)),
		labels:      make(map[int]string, len(communities)),
	}
	sort.Slice(g.Nodes, func(i, j int) bool { return g.Nodes[i].ID < g.Nodes[j].ID })
	sort.Slice(g.Edges, func(i, j int) bool {
		a, b := g.Edges[i], g.Edges[j]
		switch {
		case a.Source != b.Source:
			return a.Source < b.Source
		case a.Relation != b.Relation:
			return a.Relation < b.Relation
		case a.Target != b.Target:
			return a.Target < b.Target
		case a.File != b.File:
			// Parallel edges differing only in call site still need a total
			// order, or two runs can emit them in either sequence.
			return a.File < b.File
		default:
			return a.Line < b.Line
		}
	})
	sort.Slice(g.Communities, func(i, j int) bool { return g.Communities[i].ID < g.Communities[j].ID })
	for i := range g.Communities {
		c := &g.Communities[i]
		sort.Strings(c.NodeIDs)
		g.labels[c.ID] = c.Label
		for _, id := range c.NodeIDs {
			g.memberOf[id] = c.ID
		}
	}
	return g
}

// Load reads a whole store into a snapshot.
func Load(root string, s *store.Store) (*Graph, error) {
	var nodes []model.Node
	if err := s.EachNode(func(n model.Node) error {
		nodes = append(nodes, n)
		return nil
	}); err != nil {
		return nil, err
	}
	var edges []model.Edge
	if err := s.EachEdge(func(e model.Edge) error {
		edges = append(edges, e)
		return nil
	}); err != nil {
		return nil, err
	}
	var communities []store.Community
	if err := s.EachCommunity(func(c store.Community) error {
		c.NodeIDs = append([]string(nil), c.NodeIDs...)
		communities = append(communities, c)
		return nil
	}); err != nil {
		return nil, err
	}
	files := 0
	if err := s.EachFile(func(store.FileRecord) error {
		files++
		return nil
	}); err != nil {
		return nil, err
	}
	return NewGraph(root, nodes, edges, communities, files), nil
}

// Community reports which community a node belongs to.
//
// Membership is read from the communities bucket rather than from
// Node.Community, which cannot answer the question on its own: community ids
// start at 0 and an unclustered node's field is also 0, so the field alone
// cannot distinguish "in the first community" from "never clustered". The
// exporters therefore omit the attribute entirely rather than claim every node
// belongs to community 0.
func (g *Graph) Community(nodeID string) (id int, label string, ok bool) {
	id, ok = g.memberOf[nodeID]
	if !ok {
		return 0, "", false
	}
	return id, g.labels[id], true
}

// Stats summarizes the snapshot.
func (g *Graph) Stats() model.Stats {
	return model.Stats{
		Nodes:       len(g.Nodes),
		Edges:       len(g.Edges),
		Communities: len(g.Communities),
		Files:       g.Files,
	}
}
