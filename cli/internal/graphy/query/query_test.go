package query

import (
	"errors"
	"path/filepath"
	"testing"

	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/store"
)

// fixture: A -calls-> B -calls-> C, D isolated, plus a doc referencing A.
func fixture(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	nodes := []model.Node{
		{ID: "A", Label: "Alpha", Kind: model.KindFunction, Language: "go", File: "a.go", Doc: "handles auth"},
		{ID: "B", Label: "Beta", Kind: model.KindFunction, Language: "go", File: "b.go"},
		{ID: "C", Label: "Gamma", Kind: model.KindFunction, Language: "go", File: "c.go"},
		{ID: "D", Label: "Delta", Kind: model.KindFunction, Language: "go", File: "d.go"},
		{ID: "DOC", Label: "docs/auth.md", Kind: model.KindDoc, Language: "markdown", File: "docs/auth.md"},
	}
	if err := s.PutFile(store.FileRecord{Path: "all", NodeIDs: ids(nodes)}, nodes); err != nil {
		t.Fatal(err)
	}
	edges := []model.Edge{
		{Source: "A", Target: "B", Relation: model.RelCalls, Confidence: model.ConfExtracted},
		{Source: "B", Target: "C", Relation: model.RelCalls, Confidence: model.ConfInferred},
		{Source: "DOC", Target: "A", Relation: model.RelReferences, Confidence: model.ConfExtracted},
	}
	if err := s.ReplaceEdges(edges); err != nil {
		t.Fatal(err)
	}
	if err := s.RecomputeDegrees(); err != nil {
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

func TestResolveEntity(t *testing.T) {
	s := fixture(t)

	if n, err := ResolveEntity(s, "A"); err != nil || n.ID != "A" {
		t.Errorf("by id: %+v %v", n, err)
	}
	if n, err := ResolveEntity(s, "Beta"); err != nil || n.ID != "B" {
		t.Errorf("by label: %+v %v", n, err)
	}
	if n, err := ResolveEntity(s, "gamm"); err != nil || n.ID != "C" {
		t.Errorf("by substring: %+v %v", n, err)
	}
	if _, err := ResolveEntity(s, "nope-nothing"); err == nil {
		t.Errorf("expected miss error")
	}
	// "a" is a substring of several labels → ambiguous.
	_, err := ResolveEntity(s, "a")
	var amb *AmbiguousError
	if !errors.As(err, &amb) {
		t.Errorf("expected AmbiguousError, got %v", err)
	}
}

func TestPathDirectionAware(t *testing.T) {
	s := fixture(t)

	hops, err := Path(s, "A", "C")
	if err != nil || len(hops) != 2 {
		t.Fatalf("A→C hops = %v, %v", hops, err)
	}
	if !hops[0].Forward || !hops[1].Forward {
		t.Errorf("A→C should be all forward: %+v", hops)
	}

	// Reverse direction still finds the path, against the arrows.
	hops, err = Path(s, "C", "A")
	if err != nil || len(hops) != 2 {
		t.Fatalf("C→A hops = %v, %v", hops, err)
	}
	if hops[0].Forward || hops[1].Forward {
		t.Errorf("C→A should traverse backward: %+v", hops)
	}

	// No path to the isolated node.
	hops, err = Path(s, "A", "D")
	if err != nil || hops != nil {
		t.Errorf("A→D = %v, %v, want nil", hops, err)
	}
}

func TestExplain(t *testing.T) {
	s := fixture(t)
	card, err := Explain(s, "B")
	if err != nil {
		t.Fatal(err)
	}
	if len(card.Out) != 1 || card.Out[0].Target != "C" {
		t.Errorf("out = %+v", card.Out)
	}
	if len(card.In) != 1 || card.In[0].Source != "A" {
		t.Errorf("in = %+v", card.In)
	}
}

func TestSubgraph(t *testing.T) {
	s := fixture(t)
	nodes, edges, err := Subgraph(s, []string{"A"}, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, n := range nodes {
		got[n.ID] = true
	}
	for _, want := range []string{"A", "B", "DOC"} {
		if !got[want] {
			t.Errorf("depth-1 subgraph missing %s: %v", want, got)
		}
	}
	if got["C"] || got["D"] {
		t.Errorf("depth-1 subgraph too large: %v", got)
	}
	if len(edges) != 2 {
		t.Errorf("edges = %d, want 2 (%+v)", len(edges), edges)
	}
}

func TestKeywordSearch(t *testing.T) {
	s := fixture(t)
	hits, err := KeywordSearch(s, "auth", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) < 2 {
		t.Fatalf("hits = %+v", hits)
	}
	// The doc file matches on path; Alpha matches on doc text. Both rank.
	found := map[string]bool{}
	for _, h := range hits {
		found[h.ID] = true
	}
	if !found["A"] || !found["DOC"] {
		t.Errorf("keyword hits = %v, want A and DOC", found)
	}
}
