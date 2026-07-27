package query

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
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

// hubFixture builds a star: one "hub" node with n leaves, every third of them
// declared in a test file. Big enough to trip a result cap, which the small
// fixture above deliberately is not.
func hubFixture(t *testing.T, n int) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	nodes := []model.Node{{
		ID: "hub", Label: "hub", Kind: model.KindFunction,
		Language: "go", File: "hub.go", Doc: "the hub",
	}}
	var edges []model.Edge
	for i := 0; i < n; i++ {
		isTest := i%3 == 0
		file := fmt.Sprintf("leaf%d.go", i)
		if isTest {
			file = fmt.Sprintf("leaf%d_test.go", i)
		}
		id := fmt.Sprintf("leaf%d", i)
		nodes = append(nodes, model.Node{
			ID: id, Label: id, Kind: model.KindFunction,
			Language: "go", File: file, Test: isTest,
		})
		edges = append(edges, model.Edge{
			Source: "hub", Target: id,
			Relation: model.RelCalls, Confidence: model.ConfExtracted,
		})
	}
	if err := s.PutFile(store.FileRecord{Path: "all", NodeIDs: ids(nodes)}, nodes); err != nil {
		t.Fatal(err)
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

// traverseFixture: a small package with a hub, a caller, a test, and a doc
// section, so direction, relation, kind and test filters all have something to
// bite on.
func traverseFixture(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	nodes := []model.Node{
		{ID: "F", Label: "svc/svc.go", Kind: model.KindFile, Language: "go", File: "svc/svc.go",
			Span: model.Span{StartLine: 1}},
		{ID: "HUB", Label: "Serve", Kind: model.KindFunction, Language: "go", File: "svc/svc.go",
			Span: model.Span{StartLine: 10}},
		{ID: "CALLER", Label: "Main", Kind: model.KindFunction, Language: "go", File: "cmd/main.go",
			Span: model.Span{StartLine: 5}},
		{ID: "TYPE", Label: "Server", Kind: model.KindStruct, Language: "go", File: "svc/svc.go",
			Span: model.Span{StartLine: 3}},
		{ID: "TEST", Label: "TestServe", Kind: model.KindFunction, Language: "go",
			File: "svc/svc_test.go", Span: model.Span{StartLine: 8}, Test: true},
		{ID: "WHY", Label: "WHY: serve blocks", Kind: model.KindRationale, Language: "go",
			File: "svc/svc.go", Span: model.Span{StartLine: 9}, Body: "It blocks so callers can wait."},
	}
	if err := s.PutFile(store.FileRecord{Path: "svc/svc.go", NodeIDs: []string{"F", "TYPE", "HUB", "WHY"}},
		[]model.Node{nodes[0], nodes[3], nodes[1], nodes[5]}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutFile(store.FileRecord{Path: "cmd/main.go", NodeIDs: []string{"CALLER"}},
		[]model.Node{nodes[2]}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutFile(store.FileRecord{Path: "svc/svc_test.go", NodeIDs: []string{"TEST"}},
		[]model.Node{nodes[4]}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceEdges([]model.Edge{
		{Source: "F", Target: "HUB", Relation: model.RelContains, Confidence: model.ConfExtracted},
		{Source: "CALLER", Target: "HUB", Relation: model.RelCalls, Confidence: model.ConfInferred},
		{Source: "TEST", Target: "HUB", Relation: model.RelTests, Confidence: model.ConfInferred},
		{Source: "TYPE", Target: "HUB", Relation: model.RelDefines, Confidence: model.ConfExtracted},
		{Source: "WHY", Target: "HUB", Relation: model.RelExplains, Confidence: model.ConfExtracted},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecomputeDegrees(); err != nil {
		t.Fatal(err)
	}
	return s
}

// TestResolveEntityAcceptsAFilePath: an agent is almost always working in a
// file, and that path was the one identifier it held that resolved to nothing.
func TestResolveEntityAcceptsAFilePath(t *testing.T) {
	s := traverseFixture(t)
	for _, arg := range []string{"svc/svc.go", "./svc/svc.go"} {
		n, err := ResolveEntity(s, arg)
		if err != nil {
			t.Fatalf("ResolveEntity(%q): %v", arg, err)
		}
		if n.ID != "F" {
			t.Errorf("ResolveEntity(%q) = %q, want the file node F", arg, n.ID)
		}
	}
}

func TestFileNodesIsOrderedAndBounded(t *testing.T) {
	s := traverseFixture(t)
	res, err := FileNodes(s, "svc/svc.go", 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, n := range res.Nodes {
		got = append(got, n.ID)
	}
	want := []string{"F", "TYPE", "WHY", "HUB"} // lines 1, 3, 9, 10
	if len(got) != len(want) {
		t.Fatalf("FileNodes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("FileNodes = %v, want %v", got, want)
			break
		}
	}
	if _, err := FileNodes(s, "nope.go", 0); err == nil {
		t.Error("want an error for a file that was never extracted")
	}

	// A generated or vendored file can declare hundreds of entities, and this
	// is the tool an agent uses to enter the graph from the file it is editing.
	capped, err := FileNodes(s, "svc/svc.go", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(capped.Nodes) != 2 || !capped.More || capped.Total != 4 {
		t.Errorf("nodes=%d truncated=%v total=%d, want 2/true/4",
			len(capped.Nodes), capped.More, capped.Total)
	}
}

// TestEdgesFiltersByDirectionAndRelation is the impact question: Explain
// returns every edge, so asking "who calls this" meant paying for a hub's
// entire adjacency and filtering it by hand.
func TestEdgesFiltersByDirectionAndRelation(t *testing.T) {
	s := traverseFixture(t)

	// Incoming, all relations, tests excluded by default.
	res, err := Edges(s, "HUB", EdgeQuery{Direction: In})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]model.Relation{}
	for _, h := range res.Hits {
		got[h.Other.ID] = h.Edge.Relation
	}
	for _, id := range []string{"F", "CALLER", "TYPE", "WHY"} {
		if _, ok := got[id]; !ok {
			t.Errorf("incoming edges missing %s: %v", id, got)
		}
	}
	if _, ok := got["TEST"]; ok {
		t.Error("test code should be excluded unless asked for")
	}

	// Narrowed to one relation.
	res, err = Edges(s, "HUB", EdgeQuery{Direction: In, Relations: []model.Relation{model.RelCalls}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 1 || res.Hits[0].Other.ID != "CALLER" {
		t.Errorf("calls-only = %+v, want just CALLER", res.Hits)
	}

	// Tests are reachable when asked for.
	res, err = Edges(s, "HUB", EdgeQuery{Direction: In, Relations: []model.Relation{model.RelTests}, IncludeTests: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 1 || res.Hits[0].Other.ID != "TEST" {
		t.Errorf("tests-only = %+v, want just TEST", res.Hits)
	}

	// The cap is reported rather than silently applied.
	res, err = Edges(s, "HUB", EdgeQuery{Direction: In, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 2 || !res.More || res.Total != 4 {
		t.Errorf("limit 2: hits=%d truncated=%v total=%d, want 2/true/4", len(res.Hits), res.More, res.Total)
	}
}

// TestNeighborsIsDeterministicAndBounded covers the primitive that Subgraph
// could not provide: expansion from a seed the caller chose.
func TestNeighborsIsDeterministicAndBounded(t *testing.T) {
	s := traverseFixture(t)

	res, err := Neighbors(s, []string{"HUB"}, NeighborOptions{Depth: 1})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, n := range res.Nodes {
		got[n.ID] = true
	}
	if !got["HUB"] || !got["CALLER"] || !got["TYPE"] {
		t.Errorf("depth-1 neighbourhood = %v", got)
	}
	if got["TEST"] {
		t.Error("test code should be excluded unless asked for")
	}

	// Kind filter narrows what expansion reaches, never the seed itself.
	res, err = Neighbors(s, []string{"HUB"}, NeighborOptions{Depth: 1, Kinds: []model.Kind{model.KindStruct}})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, n := range res.Nodes {
		ids[n.ID] = true
	}
	if !ids["HUB"] || !ids["TYPE"] || len(ids) != 2 {
		t.Errorf("kind-filtered = %v, want the seed plus TYPE", ids)
	}

	// Direction restricts the walk.
	res, err = Neighbors(s, []string{"HUB"}, NeighborOptions{Depth: 1, Direction: Out})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Nodes) != 1 || res.Nodes[0].ID != "HUB" {
		t.Errorf("outgoing-only from a leaf = %+v, want just the seed", res.Nodes)
	}

	// Two identical calls agree, edge for edge.
	a, _ := Neighbors(s, []string{"HUB"}, NeighborOptions{Depth: 2})
	b, _ := Neighbors(s, []string{"HUB"}, NeighborOptions{Depth: 2})
	if len(a.Nodes) != len(b.Nodes) || len(a.Edges) != len(b.Edges) {
		t.Fatal("Neighbors is not deterministic")
	}
	for i := range a.Nodes {
		if a.Nodes[i].ID != b.Nodes[i].ID {
			t.Fatalf("node order differs at %d: %s vs %s", i, a.Nodes[i].ID, b.Nodes[i].ID)
		}
	}
}

// TestRationaleIsADeterministicFilter: the CLI could previously only reach this
// by phrasing the question in natural language and hoping the planner agreed.
func TestRationaleIsADeterministicFilter(t *testing.T) {
	s := traverseFixture(t)
	res, err := Rationale(s, "HUB", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 1 || res.Hits[0].Other.ID != "WHY" {
		t.Fatalf("Rationale = %+v, want the WHY node", res.Hits)
	}
	if res.Hits[0].Other.Body == "" {
		t.Error("rationale prose should come back with the node")
	}
}

func TestParseVocabularyRejectsUnknownNames(t *testing.T) {
	if _, err := ParseRelations([]string{"calls", "tests"}); err != nil {
		t.Errorf("valid relations rejected: %v", err)
	}
	// A silent empty filter would widen the query to everything and read as
	// "no results" rather than as a typo.
	if _, err := ParseRelations([]string{"callz"}); err == nil {
		t.Error("want an error for an unknown relation")
	}
	if _, err := ParseKinds([]string{"funktion"}); err == nil {
		t.Error("want an error for an unknown kind")
	}
	if _, err := ParseDirection("sideways"); err == nil {
		t.Error("want an error for an unknown direction")
	}
	for _, d := range []string{"", "out", "IN", "both"} {
		if _, err := ParseDirection(d); err != nil {
			t.Errorf("ParseDirection(%q): %v", d, err)
		}
	}
}

// testSeedFixture: a test helper whose label matches the query better than the
// production entity does, which is the situation keyword seeding gets wrong.
func testSeedFixture(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	nodes := []model.Node{
		{ID: "H", Label: "widgetTestHelper", Kind: model.KindFunction,
			Language: "go", File: "widget_test.go", Test: true},
		{ID: "W", Label: "widget", Kind: model.KindFunction,
			Language: "go", File: "widget.go", Doc: "the widget"},
		{ID: "N", Label: "widgetNext", Kind: model.KindFunction,
			Language: "go", File: "next.go"},
	}
	if err := s.PutFile(store.FileRecord{Path: "all", NodeIDs: ids(nodes)}, nodes); err != nil {
		t.Fatal(err)
	}
	edges := []model.Edge{
		{Source: "H", Target: "W", Relation: model.RelCalls, Confidence: model.ConfExtracted},
		{Source: "W", Target: "N", Relation: model.RelCalls, Confidence: model.ConfExtracted},
	}
	if err := s.ReplaceEdges(edges); err != nil {
		t.Fatal(err)
	}
	if err := s.RecomputeDegrees(); err != nil {
		t.Fatal(err)
	}
	return s
}

// TestResolveEntityExplainsAConceptID: `okfw search --node-id` hands agents
// "community:<slug>" and "graph:<slug>" routinely. Neither is in the nodes
// bucket, and "no entity matches" reads as "that subsystem is not in the
// graph" — which is wrong, and leaves the agent with nowhere to go.
func TestResolveEntityExplainsAConceptID(t *testing.T) {
	s := fixture(t)
	for _, tc := range []struct{ arg, want string }{
		{"community:store", "graphy community"},
		{"graph:myproj", "graphy report"},
	} {
		_, err := ResolveEntity(s, tc.arg)
		if err == nil {
			t.Fatalf("%s resolved as an entity", tc.arg)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("error for %s does not name %s: %v", tc.arg, tc.want, err)
		}
	}
}

// TestConceptsForFileReturnsTheEntitiesDocumentedInIt: a file node is never
// exported, so `graphy concept <path>` could only ever fail — and it failed
// advising an export that would never produce a concept for a file.
func TestConceptsForFileReturnsTheEntitiesDocumentedInIt(t *testing.T) {
	s := traverseFixture(t)
	if err := s.PutExport("HUB", store.ExportRecord{
		ConceptID: "019680f0-7e00-7abc-8def-100000000000", RelPath: "graphy/demo/serve--0001.md",
	}); err != nil {
		t.Fatal(err)
	}

	got, err := ConceptsForFile(s, "svc/svc.go", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("concepts = %d, want the one exported entity: %+v", len(got), got)
	}
	if got[0].NodeID != "HUB" || got[0].RelPath != "graphy/demo/serve--0001.md" {
		t.Errorf("concept = %+v", got[0])
	}

	// A file whose entities are all undocumented is empty, not an error: that
	// is the case the caller has to phrase as "below --min-degree".
	got, err = ConceptsForFile(s, "cmd/main.go", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("undocumented file returned %+v", got)
	}
}
