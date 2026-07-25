package report

import (
	"strings"
	"testing"

	"okfw/cli/internal/graphy/analyze"
	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/store"
)

func sampleReport() *analyze.Report {
	alpha := model.Node{ID: "go:function:a.go#Alpha", Label: "Alpha", Kind: model.KindFunction,
		Language: "go", File: "a.go", Span: model.Span{StartLine: 12}, Degree: 5}
	beta := model.Node{ID: "go:function:b.go#Beta", Label: "Beta", Kind: model.KindFunction,
		Language: "go", File: "b.go", Degree: 3} // no line number
	return &analyze.Report{
		Root:       "/repo",
		Stats:      model.Stats{Nodes: 9, Edges: 3, Files: 2, Communities: 2},
		Languages:  map[string]int{"go": 6, "python": 1},
		Confidence: map[model.Confidence]int{model.ConfExtracted: 1, model.ConfInferred: 1, model.ConfAmbiguous: 1},
		GodNodes:   []model.Node{alpha, beta},
		Communities: []analyze.CommunitySummary{
			{ID: 0, Label: "Alpha", Size: 4, Top: []model.Node{alpha}},
			{ID: 1, Label: "gamma", Size: 2, Top: []model.Node{beta}},
		},
		Orphans:   1,
		Rationale: map[string]int{"WHY": 2, "ADR": 1, "DECISION": 3},
		// The template renders an ADR's Doc, which is where the rationale
		// extractor puts the record's summary.
		ADRs: []model.Node{{
			Label: "ADR: use bbolt",
			Doc:   "Use bbolt for the graph store",
			File:  "docs/adr/0001.md",
		}},
		Cluster: store.ClusterMeta{Algorithm: "louvain", Resolution: 1.5, Seed: 7},
	}
}

func render(t *testing.T, r *analyze.Report) string {
	t.Helper()
	var sb strings.Builder
	if err := Render(&sb, r); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return sb.String()
}

func TestRenderIncludesEverySection(t *testing.T) {
	out := render(t, sampleReport())

	for _, want := range []string{
		"# Graph Report",
		"/repo",
		"Alpha",                         // god node
		"a.go:12",                       // loc with a line number
		"gamma",                         // community label
		"Use bbolt for the graph store", // ADR summary
		"louvain",                       // cluster meta
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report is missing %q\n---\n%s", want, out)
		}
	}
	// A node with no line number renders the bare path, not "b.go:0".
	if strings.Contains(out, "b.go:0") {
		t.Errorf("a node without a line number must render the bare path\n---\n%s", out)
	}
	if !strings.Contains(out, "b.go") {
		t.Errorf("report is missing the file for a line-less node\n---\n%s", out)
	}
}

// TestRenderIsDeterministic guards the report against map-iteration churn: it
// is written to disk on every build, so an unstable ordering would produce a
// spurious diff each time.
func TestRenderIsDeterministic(t *testing.T) {
	first := render(t, sampleReport())
	for i := 0; i < 20; i++ {
		if got := render(t, sampleReport()); got != first {
			t.Fatalf("render is not deterministic; run %d differs:\n%s\n---\n%s", i, first, got)
		}
	}
}

// TestRenderMarkersAreSorted is the specific ordering the markers helper
// exists to guarantee.
func TestRenderMarkersAreSorted(t *testing.T) {
	out := render(t, sampleReport())
	adr, decision, why := strings.Index(out, "1 ADR"), strings.Index(out, "3 DECISION"), strings.Index(out, "2 WHY")
	if adr < 0 || decision < 0 || why < 0 {
		t.Fatalf("marker tally missing from report\n---\n%s", out)
	}
	if !(adr < decision && decision < why) {
		t.Errorf("markers must render in sorted order (ADR, DECISION, WHY), got offsets %d/%d/%d", adr, decision, why)
	}
}

func TestRenderEmptyGraph(t *testing.T) {
	out := render(t, &analyze.Report{
		Root:       "/empty",
		Languages:  map[string]int{},
		Confidence: map[model.Confidence]int{},
		Rationale:  map[string]int{},
	})
	if !strings.Contains(out, "# Graph Report") {
		t.Errorf("an empty graph should still render a report\n---\n%s", out)
	}
}

// TestSuggestionsUseTheGraphsOwnHubs covers the starter-query generator.
func TestSuggestionsUseTheGraphsOwnHubs(t *testing.T) {
	got := suggestions(sampleReport())
	if len(got) != 3 {
		t.Fatalf("suggestions = %v, want 3", got)
	}
	if !strings.Contains(got[0], `"Alpha"`) {
		t.Errorf("first suggestion should explain the top hub, got %q", got[0])
	}
	if !strings.Contains(got[1], `"Alpha"`) || !strings.Contains(got[1], `"Beta"`) {
		t.Errorf("second suggestion should path between the top two hubs, got %q", got[1])
	}

	// With no god nodes, only the generic suggestion survives — no panic on
	// GodNodes[0] or GodNodes[1].
	if got := suggestions(&analyze.Report{}); len(got) != 1 {
		t.Errorf("an empty graph should yield exactly the generic suggestion, got %v", got)
	}
	// One god node: explain is possible, path is not.
	one := &analyze.Report{GodNodes: []model.Node{{Label: "Solo"}}}
	if got := suggestions(one); len(got) != 2 {
		t.Errorf("a single hub should yield explain + generic, got %v", got)
	}
}
