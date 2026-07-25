package export

import (
	"testing"

	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/store"
)

// formatFixture is the graph the GraphML/JGF/Cypher tests share: two
// communities, an external node, a rationale explaining a function, an
// unclustered node, and edges with and without a call site.
//
// Node order is deliberately scrambled — every writer is supposed to emit in
// the snapshot's canonical order, not in the order it was handed.
func formatFixture() *Graph {
	nodes := []model.Node{
		{
			ID: "go:function:b.go#Handle", Label: "Handle", Kind: model.KindFunction,
			Language: "go", File: "b.go", Span: model.Span{StartLine: 30, EndLine: 44},
			Signature: "func Handle(c <-chan T) error", Doc: "Handle drains c.", Degree: 4,
		},
		{
			ID: "go:external:#net/http", Label: "net/http", Kind: model.KindExternal,
			Language: "go", External: true, Degree: 1,
		},
		{
			ID: "go:function:a.go#Run", Label: "Run", Kind: model.KindFunction,
			Language: "go", File: "a.go", Span: model.Span{StartLine: 10, EndLine: 20},
			Signature: "func Run() error", Doc: "Run starts it.", Degree: 3,
		},
		{
			ID: "go:rationale:a.go#Run@WHY@0", Label: "WHY: retries are capped", Kind: model.KindRationale,
			Language: "go", File: "a.go", Span: model.Span{StartLine: 12},
			Body: "WHY: retries are capped at 3 because\nthe upstream rate-limits.", Degree: 2,
		},
	}
	edges := []model.Edge{
		{
			Source: "go:function:a.go#Run", Target: "go:function:b.go#Handle",
			Relation: model.RelCalls, Confidence: model.ConfExtracted, File: "a.go", Line: 14,
		},
		{
			Source: "go:function:a.go#Run", Target: "go:external:#net/http",
			Relation: model.RelImports, Confidence: model.ConfInferred,
		},
		{
			Source: "go:rationale:a.go#Run@WHY@0", Target: "go:function:a.go#Run",
			Relation: model.RelExplains, Confidence: model.ConfExtracted, File: "a.go", Line: 12,
		},
	}
	communities := []store.Community{
		// Community 0 exists and has members: an exporter that treats 0 as
		// "unclustered" would drop a real subsystem.
		{ID: 0, Label: "core", NodeIDs: []string{"go:function:a.go#Run", "go:rationale:a.go#Run@WHY@0"}},
		{ID: 1, Label: "transport", NodeIDs: []string{"go:function:b.go#Handle"}},
	}
	return NewGraph("/repo/myproj", nodes, edges, communities, 2)
}

func TestNewGraphOrdersNodesEdgesAndCommunities(t *testing.T) {
	g := formatFixture()
	var ids []string
	for _, n := range g.Nodes {
		ids = append(ids, n.ID)
	}
	want := []string{
		"go:external:#net/http",
		"go:function:a.go#Run",
		"go:function:b.go#Handle",
		"go:rationale:a.go#Run@WHY@0",
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("node order = %v, want %v", ids, want)
		}
	}
	// Edges sort by source, then relation, then target.
	if g.Edges[0].Relation != model.RelCalls || g.Edges[1].Relation != model.RelImports {
		t.Errorf("edge order = %v", g.Edges)
	}
	if g.Communities[0].ID != 0 || g.Communities[1].ID != 1 {
		t.Errorf("community order = %v", g.Communities)
	}
}

// TestGraphCommunityDistinguishesZeroFromUnclustered pins the reason
// membership is read from the communities bucket rather than Node.Community:
// the field's zero value is indistinguishable from real membership in
// community 0.
func TestGraphCommunityDistinguishesZeroFromUnclustered(t *testing.T) {
	g := formatFixture()
	if id, label, ok := g.Community("go:function:a.go#Run"); !ok || id != 0 || label != "core" {
		t.Errorf("Run: got (%d, %q, %v), want (0, core, true)", id, label, ok)
	}
	// Never assigned, and its Node.Community field is the same zero value.
	if id, label, ok := g.Community("go:external:#net/http"); ok {
		t.Errorf("external node reported community (%d, %q); it was never clustered", id, label)
	}
	if _, _, ok := g.Community("go:function:nope.go#Missing"); ok {
		t.Error("unknown node reported a community")
	}
}

func TestGraphStats(t *testing.T) {
	st := formatFixture().Stats()
	want := model.Stats{Nodes: 4, Edges: 3, Communities: 2, Files: 2}
	if st != want {
		t.Errorf("Stats = %+v, want %+v", st, want)
	}
}

// TestLoadReadsWholeStore checks the loader against a real store, which is the
// only path that exercises bbolt iteration.
func TestLoadReadsWholeStore(t *testing.T) {
	s, _ := partitionFixture(t, 4)
	partition(t, s, map[string][]int{"alpha": {0, 1}, "beta": {2, 3}})
	if err := s.ReplaceEdges([]model.Edge{{
		Source: node(0), Target: node(1), Relation: model.RelCalls, Confidence: model.ConfExtracted,
	}}); err != nil {
		t.Fatal(err)
	}

	g, err := Load("/repo/myproj", s)
	if err != nil {
		t.Fatal(err)
	}
	st := g.Stats()
	if st.Nodes != 4 || st.Edges != 1 || st.Communities != 2 || st.Files != 1 {
		t.Errorf("Stats = %+v", st)
	}
	if _, label, ok := g.Community(node(2)); !ok || label != "beta" {
		t.Errorf("community of node 2 = %q, %v; want beta", label, ok)
	}
}
