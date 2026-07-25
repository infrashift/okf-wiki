package store

import (
	"path/filepath"
	"testing"

	"okfw/cli/internal/graphy/model"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func node(id, label, lang string, kind model.Kind, file string) model.Node {
	return model.Node{ID: id, Label: label, Language: lang, Kind: kind, File: file}
}

func TestPutFileAndGetNode(t *testing.T) {
	s := openTemp(t)
	n := node("go:function:a.go#Foo", "Foo", "go", model.KindFunction, "a.go")
	rec := FileRecord{Path: "a.go", SHA256: "abc", NodeIDs: []string{n.ID}}
	if err := s.PutFile(rec, []model.Node{n}); err != nil {
		t.Fatalf("PutFile: %v", err)
	}

	got, err := s.GetNode(n.ID)
	if err != nil || got == nil {
		t.Fatalf("GetNode: %v %v", got, err)
	}
	if got.Label != "Foo" {
		t.Errorf("Label = %q, want Foo", got.Label)
	}

	ids, err := s.LookupSymbol("go", "Foo")
	if err != nil || len(ids) != 1 || ids[0] != n.ID {
		t.Errorf("LookupSymbol = %v, %v", ids, err)
	}

	frec, err := s.GetFile("a.go")
	if err != nil || frec == nil || frec.SHA256 != "abc" {
		t.Errorf("GetFile = %+v, %v", frec, err)
	}
}

func TestPurgeFileRemovesNodesAndSymbols(t *testing.T) {
	s := openTemp(t)
	n := node("go:function:a.go#Foo", "Foo", "go", model.KindFunction, "a.go")
	if err := s.PutFile(FileRecord{Path: "a.go", NodeIDs: []string{n.ID}}, []model.Node{n}); err != nil {
		t.Fatalf("PutFile: %v", err)
	}
	if err := s.PurgeFile("a.go"); err != nil {
		t.Fatalf("PurgeFile: %v", err)
	}
	if got, _ := s.GetNode(n.ID); got != nil {
		t.Errorf("node survived purge: %+v", got)
	}
	if ids, _ := s.LookupSymbol("go", "Foo"); len(ids) != 0 {
		t.Errorf("symbol survived purge: %v", ids)
	}
	if rec, _ := s.GetFile("a.go"); rec != nil {
		t.Errorf("file record survived purge: %+v", rec)
	}
}

func TestReextractionIsIdempotent(t *testing.T) {
	s := openTemp(t)
	old := node("go:function:a.go#Old", "Old", "go", model.KindFunction, "a.go")
	if err := s.PutFile(FileRecord{Path: "a.go", NodeIDs: []string{old.ID}}, []model.Node{old}); err != nil {
		t.Fatalf("PutFile: %v", err)
	}
	fresh := node("go:function:a.go#New", "New", "go", model.KindFunction, "a.go")
	if err := s.PutFile(FileRecord{Path: "a.go", NodeIDs: []string{fresh.ID}}, []model.Node{fresh}); err != nil {
		t.Fatalf("PutFile 2: %v", err)
	}
	if got, _ := s.GetNode(old.ID); got != nil {
		t.Errorf("stale node survived re-extraction")
	}
	if ids, _ := s.LookupSymbol("go", "Old"); len(ids) != 0 {
		t.Errorf("stale symbol survived re-extraction: %v", ids)
	}
	if got, _ := s.GetNode(fresh.ID); got == nil {
		t.Errorf("fresh node missing")
	}
}

func TestEdgesAndReverseIndex(t *testing.T) {
	s := openTemp(t)
	edges := []model.Edge{
		{Source: "A", Target: "B", Relation: model.RelCalls, Confidence: model.ConfExtracted},
		{Source: "A", Target: "C", Relation: model.RelImports, Confidence: model.ConfInferred},
		{Source: "C", Target: "B", Relation: model.RelCalls, Confidence: model.ConfAmbiguous},
	}
	if err := s.ReplaceEdges(edges); err != nil {
		t.Fatalf("ReplaceEdges: %v", err)
	}

	out, err := s.OutEdges("A")
	if err != nil || len(out) != 2 {
		t.Fatalf("OutEdges(A) = %v, %v", out, err)
	}
	in, err := s.InEdges("B")
	if err != nil || len(in) != 2 {
		t.Fatalf("InEdges(B) = %v, %v", in, err)
	}
	for _, e := range in {
		if e.Target != "B" {
			t.Errorf("in-edge target = %q, want B", e.Target)
		}
	}

	// Replacement is wholesale: shrinking works.
	if err := s.ReplaceEdges(edges[:1]); err != nil {
		t.Fatalf("ReplaceEdges shrink: %v", err)
	}
	if in, _ := s.InEdges("B"); len(in) != 1 {
		t.Errorf("stale rev entries after ReplaceEdges: %v", in)
	}
	if out, _ := s.OutEdges("C"); len(out) != 0 {
		t.Errorf("stale edges after ReplaceEdges: %v", out)
	}
}

func TestRecomputeDegrees(t *testing.T) {
	s := openTemp(t)
	a := node("A", "A", "go", model.KindFunction, "a.go")
	b := node("B", "B", "go", model.KindFunction, "b.go")
	if err := s.PutFile(FileRecord{Path: "a.go", NodeIDs: []string{a.ID}}, []model.Node{a}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutFile(FileRecord{Path: "b.go", NodeIDs: []string{b.ID}}, []model.Node{b}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceEdges([]model.Edge{{Source: "A", Target: "B", Relation: model.RelCalls, Confidence: model.ConfExtracted}}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecomputeDegrees(); err != nil {
		t.Fatalf("RecomputeDegrees: %v", err)
	}
	got, _ := s.GetNode("A")
	if got.Degree != 1 {
		t.Errorf("A degree = %d, want 1", got.Degree)
	}
}

func TestCommunities(t *testing.T) {
	s := openTemp(t)
	a := node("A", "A", "go", model.KindFunction, "a.go")
	b := node("B", "B", "go", model.KindFunction, "b.go")
	if err := s.PutFile(FileRecord{Path: "a.go", NodeIDs: []string{a.ID, b.ID}}, []model.Node{a, b}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCommunities(map[string]int{"A": 0, "B": 1}, map[int]string{0: "alpha", 1: "beta"}); err != nil {
		t.Fatalf("SetCommunities: %v", err)
	}
	got, _ := s.GetNode("B")
	if got.Community != 1 {
		t.Errorf("B community = %d, want 1", got.Community)
	}
	count := 0
	labels := map[int]string{}
	if err := s.EachCommunity(func(c Community) error {
		count++
		labels[c.ID] = c.Label
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count != 2 || labels[0] != "alpha" {
		t.Errorf("communities = %d %v", count, labels)
	}
}

func TestExportsAndMeta(t *testing.T) {
	s := openTemp(t)
	if err := s.PutExport("A", ExportRecord{ConceptID: "id-1", RelPath: "graphy/p/a.md"}); err != nil {
		t.Fatal(err)
	}
	rec, err := s.GetExport("A")
	if err != nil || rec == nil || rec.ConceptID != "id-1" {
		t.Fatalf("GetExport = %+v, %v", rec, err)
	}
	if err := s.DeleteExport("A"); err != nil {
		t.Fatal(err)
	}
	if rec, _ := s.GetExport("A"); rec != nil {
		t.Errorf("export survived delete")
	}

	if err := s.PutMeta("seed", "42"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.GetMeta("seed"); v != "42" {
		t.Errorf("meta = %q, want 42", v)
	}
}
