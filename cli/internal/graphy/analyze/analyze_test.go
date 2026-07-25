package analyze

import (
	"path/filepath"
	"testing"

	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/store"
)

// fixture builds a store with a shape that exercises every ranking rule:
// two communities, a file node, a heading, a rationale node, an ADR, an
// orphan, and the synthetic pseudo-file record.
func fixture(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	nodes := []model.Node{
		{ID: "go:file:a.go#", Label: "a.go", Kind: model.KindFile, Language: "go", File: "a.go", Degree: 9},
		{ID: "go:function:a.go#Alpha", Label: "Alpha", Kind: model.KindFunction, Language: "go",
			File: "a.go", Span: model.Span{StartLine: 3}, Degree: 5},
		{ID: "go:function:a.go#Beta", Label: "Beta", Kind: model.KindFunction, Language: "go",
			File: "a.go", Span: model.Span{StartLine: 9}, Degree: 3},
		{ID: "py:function:b.py#gamma", Label: "gamma", Kind: model.KindFunction, Language: "python",
			File: "b.py", Span: model.Span{StartLine: 1}, Degree: 2},
		{ID: "md:heading:c.md#Intro", Label: "Intro", Kind: model.KindHeading, Language: "markdown",
			File: "c.md", Degree: 4},
		{ID: "go:rationale:a.go#Alpha@WHY@0", Label: "WHY: keep it flat", Kind: model.KindRationale,
			Language: "go", File: "a.go", Degree: 8},
		{ID: "md:rationale:docs/adr/0001.md#0001@ADR@0", Label: "ADR: use bbolt",
			Kind: model.KindRationale, Language: "markdown", File: "docs/adr/0001.md", Degree: 1},
		{ID: "go:function:d.go#Lonely", Label: "Lonely", Kind: model.KindFunction, Language: "go",
			File: "d.go", Degree: 0},
	}
	if err := s.PutFile(store.FileRecord{Path: "all.go", NodeIDs: ids(nodes)}, nodes); err != nil {
		t.Fatal(err)
	}
	// The synthetic record holds nodes that belong to no source file. It must
	// not be counted as a file the repository has.
	synthetic := []model.Node{
		{ID: "go:module:ext#ext", Label: "ext", Kind: model.KindModule, Language: "go", Degree: 1},
	}
	if err := s.PutFile(store.FileRecord{Path: store.SyntheticPath, NodeIDs: ids(synthetic)}, synthetic); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceEdges([]model.Edge{
		{Source: nodes[1].ID, Target: nodes[2].ID, Relation: model.RelCalls, Confidence: model.ConfExtracted},
		{Source: nodes[2].ID, Target: nodes[3].ID, Relation: model.RelCalls, Confidence: model.ConfInferred},
		{Source: nodes[5].ID, Target: nodes[1].ID, Relation: model.RelExplains, Confidence: model.ConfAmbiguous},
	}); err != nil {
		t.Fatal(err)
	}
	assign := map[string]int{
		nodes[0].ID: 0, nodes[1].ID: 0, nodes[2].ID: 0, nodes[5].ID: 0,
		nodes[3].ID: 1, nodes[4].ID: 1,
	}
	if err := s.SetCommunities(assign, map[int]string{0: "Alpha", 1: "gamma"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutClusterMeta(store.ClusterMeta{Algorithm: "louvain", Resolution: 1.5, Seed: 7}); err != nil {
		t.Fatal(err)
	}
	return s
}

func ids(nodes []model.Node) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.ID
	}
	return out
}

func TestBuildStats(t *testing.T) {
	r, err := Build(fixture(t), "/repo")
	if err != nil {
		t.Fatal(err)
	}

	if r.Root != "/repo" {
		t.Errorf("Root = %q, want /repo", r.Root)
	}
	if r.Stats.Nodes != 9 {
		t.Errorf("Nodes = %d, want 9", r.Stats.Nodes)
	}
	if r.Stats.Edges != 3 {
		t.Errorf("Edges = %d, want 3", r.Stats.Edges)
	}
	// Two real file records; the synthetic pseudo-file must not be counted.
	if r.Stats.Files != 1 {
		t.Errorf("Files = %d, want 1 (the synthetic record must be excluded)", r.Stats.Files)
	}
	if r.Orphans != 1 {
		t.Errorf("Orphans = %d, want 1", r.Orphans)
	}
	if r.Languages["go"] != 6 || r.Languages["python"] != 1 || r.Languages["markdown"] != 2 {
		t.Errorf("Languages = %v", r.Languages)
	}
	if r.Confidence[model.ConfExtracted] != 1 ||
		r.Confidence[model.ConfInferred] != 1 ||
		r.Confidence[model.ConfAmbiguous] != 1 {
		t.Errorf("Confidence = %v", r.Confidence)
	}
}

// TestBuildGodNodesExcludeScaffoldingAndProse pins the ranking rule: a file
// node and a rationale node both outrank every function by degree here, and
// neither says anything about where a codebase's hot spots are.
func TestBuildGodNodesExcludeScaffoldingAndProse(t *testing.T) {
	r, err := Build(fixture(t), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range r.GodNodes {
		switch n.Kind {
		case model.KindFile, model.KindHeading, model.KindRationale:
			t.Errorf("god nodes must exclude %s, got %s", n.Kind, n.Label)
		}
	}
	if len(r.GodNodes) == 0 {
		t.Fatal("no god nodes")
	}
	if r.GodNodes[0].Label != "Alpha" {
		t.Errorf("god nodes should rank by degree; first = %s, want Alpha", r.GodNodes[0].Label)
	}
	for i := 1; i < len(r.GodNodes); i++ {
		if r.GodNodes[i-1].Degree < r.GodNodes[i].Degree {
			t.Errorf("god nodes are not sorted by descending degree: %v", r.GodNodes)
		}
	}
}

func TestBuildRationaleAndADRs(t *testing.T) {
	r, err := Build(fixture(t), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if r.Rationale["WHY"] != 1 {
		t.Errorf("Rationale[WHY] = %d, want 1 (%v)", r.Rationale["WHY"], r.Rationale)
	}
	if r.Rationale["ADR"] != 1 {
		t.Errorf("Rationale[ADR] = %d, want 1 (%v)", r.Rationale["ADR"], r.Rationale)
	}
	if len(r.ADRs) != 1 {
		t.Fatalf("ADRs = %d, want 1", len(r.ADRs))
	}
	if r.ADRs[0].Label != "ADR: use bbolt" {
		t.Errorf("ADRs[0] = %s", r.ADRs[0].Label)
	}
}

func TestBuildCommunitiesSortedBySize(t *testing.T) {
	r, err := Build(fixture(t), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if r.Stats.Communities != 2 {
		t.Fatalf("Communities = %d, want 2", r.Stats.Communities)
	}
	if len(r.Communities) != 2 {
		t.Fatalf("len(Communities) = %d", len(r.Communities))
	}
	if r.Communities[0].Size < r.Communities[1].Size {
		t.Errorf("communities must be sorted largest first, got %d then %d",
			r.Communities[0].Size, r.Communities[1].Size)
	}
	if r.Communities[0].Label != "Alpha" {
		t.Errorf("largest community label = %q, want Alpha", r.Communities[0].Label)
	}
	for _, c := range r.Communities {
		if len(c.Top) == 0 {
			t.Errorf("community %d has no members listed", c.ID)
		}
		for i := 1; i < len(c.Top); i++ {
			if c.Top[i-1].Degree < c.Top[i].Degree {
				t.Errorf("community %d members not sorted by degree: %v", c.ID, c.Top)
			}
		}
	}
}

// TestBuildSurfacesClusterMeta covers the passthrough that makes
// GRAPH_REPORT.md a visible signal when the cluster flag plumbing breaks.
func TestBuildSurfacesClusterMeta(t *testing.T) {
	r, err := Build(fixture(t), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if r.Cluster.Algorithm != "louvain" || r.Cluster.Resolution != 1.5 || r.Cluster.Seed != 7 {
		t.Errorf("Cluster = %+v, want louvain/1.5/7", r.Cluster)
	}
}

func TestCommunityLabelsPreferCodeOverProse(t *testing.T) {
	nodes := []model.Node{
		{ID: "r", Label: "WHY: a long explanation", Kind: model.KindRationale, Degree: 99},
		{ID: "f", Label: "a.go", Kind: model.KindFile, Degree: 50},
		{ID: "fn", Label: "Handler", Kind: model.KindFunction, Degree: 4},
		{ID: "m", Label: "core", Kind: model.KindModule, Degree: 1},
	}
	assign := map[string]int{"r": 0, "f": 0, "fn": 0, "m": 0}

	// Kind priority beats degree: a module outranks everything even at degree 1.
	if got := CommunityLabels(nodes, assign)[0]; got != "core" {
		t.Errorf("label = %q, want core (kind priority must beat degree)", got)
	}

	// Within one kind, degree breaks the tie.
	two := []model.Node{
		{ID: "a", Label: "Small", Kind: model.KindFunction, Degree: 2},
		{ID: "b", Label: "Big", Kind: model.KindFunction, Degree: 20},
	}
	if got := CommunityLabels(two, map[string]int{"a": 0, "b": 0})[0]; got != "Big" {
		t.Errorf("label = %q, want Big", got)
	}

	// Nodes with no community assignment are ignored.
	if labels := CommunityLabels(nodes, map[string]int{}); len(labels) != 0 {
		t.Errorf("unassigned nodes should produce no labels, got %v", labels)
	}
}
