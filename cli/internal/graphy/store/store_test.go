package store

import (
	"bytes"
	"encoding/json"
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

// TestPutExportsIsAtomicPerKey covers the batched write path: every record in
// the map lands, and none of the untouched ones are disturbed.
func TestPutExportsIsAtomicPerKey(t *testing.T) {
	s := openTemp(t)
	if err := s.PutExport("keep", ExportRecord{ConceptID: "id-keep"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutExports(nil); err != nil {
		t.Errorf("empty batch: %v", err)
	}
	if err := s.PutExports(map[string]ExportRecord{
		"a": {ConceptID: "id-a", MemberDigest: []byte{1, 2}},
		"b": {ConceptID: "id-b"},
	}); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"a": "id-a", "b": "id-b", "keep": "id-keep"} {
		rec, err := s.GetExport(key)
		if err != nil || rec == nil || rec.ConceptID != want {
			t.Errorf("GetExport(%q) = %+v, %v; want %q", key, rec, err, want)
		}
	}
	if rec, _ := s.GetExport("a"); !bytes.Equal(rec.MemberDigest, []byte{1, 2}) {
		t.Errorf("digest round-trip = %v", rec.MemberDigest)
	}
}

// TestLegacyMembersRecordDecodes pins the reason MemberDigest uses a new JSON
// key: a record written before the digest existed carried a []string under
// "members". Decoding it must yield an empty digest — which the next export
// refreshes — rather than a type error that would fail the whole export.
func TestLegacyMembersRecordDecodes(t *testing.T) {
	legacy := []byte(`{"concept_id":"id-1","rel_path":"graphy/p/a.md","members":["go:function:f.go#A"]}`)
	var rec ExportRecord
	if err := json.Unmarshal(legacy, &rec); err != nil {
		t.Fatalf("legacy record failed to decode: %v", err)
	}
	if rec.ConceptID != "id-1" || rec.RelPath != "graphy/p/a.md" {
		t.Errorf("legacy fields lost: %+v", rec)
	}
	if len(rec.MemberDigest) != 0 {
		t.Errorf("MemberDigest = %v, want empty", rec.MemberDigest)
	}
}

// TestCountsEachFileAndMeta covers four exported methods that no test called:
// Counts (run on every build), EachFile (drives the delete diff and the report
// file tally), DeleteMeta, and the ClusterMeta pair.
func TestCountsEachFileAndMeta(t *testing.T) {
	s := openTemp(t)

	nodes := []model.Node{
		{ID: "go:function:a.go#A", Label: "A", Kind: model.KindFunction, Language: "go", File: "a.go"},
		{ID: "go:function:a.go#B", Label: "B", Kind: model.KindFunction, Language: "go", File: "a.go"},
	}
	if err := s.PutFile(FileRecord{Path: "a.go", SHA256: "sha-a", NodeIDs: idsOf(nodes)}, nodes); err != nil {
		t.Fatal(err)
	}
	other := []model.Node{{ID: "go:function:b.go#C", Label: "C", Kind: model.KindFunction, Language: "go", File: "b.go"}}
	if err := s.PutFile(FileRecord{Path: "b.go", SHA256: "sha-b", NodeIDs: idsOf(other)}, other); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceEdges([]model.Edge{
		{Source: nodes[0].ID, Target: nodes[1].ID, Relation: model.RelCalls, Confidence: model.ConfExtracted},
	}); err != nil {
		t.Fatal(err)
	}

	gotNodes, gotEdges, err := s.Counts()
	if err != nil {
		t.Fatal(err)
	}
	if gotNodes != 3 || gotEdges != 1 {
		t.Errorf("Counts() = %d nodes, %d edges; want 3, 1", gotNodes, gotEdges)
	}

	seen := map[string]string{}
	if err := s.EachFile(func(rec FileRecord) error {
		seen[rec.Path] = rec.SHA256
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || seen["a.go"] != "sha-a" || seen["b.go"] != "sha-b" {
		t.Errorf("EachFile saw %v, want a.go/b.go with their hashes", seen)
	}

	// PurgeFile cascades: the record, its nodes, and its symbol entries.
	if err := s.PurgeFile("a.go"); err != nil {
		t.Fatal(err)
	}
	if gotNodes, _, err = s.Counts(); err != nil {
		t.Fatal(err)
	} else if gotNodes != 1 {
		t.Errorf("after PurgeFile, Counts() = %d nodes, want 1", gotNodes)
	}
	if n, err := s.GetNode(nodes[0].ID); err != nil || n != nil {
		t.Errorf("purged node still present: %+v (%v)", n, err)
	}
	if hits, err := s.LookupSymbol("go", "A"); err != nil || len(hits) != 0 {
		t.Errorf("purged symbol still indexed: %v (%v)", hits, err)
	}

	// Meta round-trip, including the delete the louvain_seed migration needs.
	if err := s.PutMeta("k", "v"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetMeta("k"); err != nil || got != "v" {
		t.Errorf("GetMeta = %q, %v", got, err)
	}
	if err := s.DeleteMeta("k"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetMeta("k"); err != nil || got != "" {
		t.Errorf("after DeleteMeta, GetMeta = %q, %v; want empty", got, err)
	}
	// Deleting an absent key is not an error — the migration runs every build.
	if err := s.DeleteMeta("never-existed"); err != nil {
		t.Errorf("DeleteMeta on a missing key: %v", err)
	}
	// An unset fingerprint reads as "" — the branch that makes the first build full.
	if got, err := s.GetMeta("extract_fingerprint"); err != nil || got != "" {
		t.Errorf("unset meta key = %q, %v; want empty", got, err)
	}
}

func TestClusterMetaRoundTrip(t *testing.T) {
	s := openTemp(t)

	// A store that has never been clustered reads back the zero value, which
	// is what makes the CLI fall back to graphy's defaults.
	got, err := s.GetClusterMeta()
	if err != nil {
		t.Fatalf("GetClusterMeta on a fresh store: %v", err)
	}
	if got != (ClusterMeta{}) {
		t.Errorf("fresh store = %+v, want the zero value", got)
	}

	want := ClusterMeta{Algorithm: "louvain", Resolution: 1.5, Seed: 7}
	if err := s.PutClusterMeta(want); err != nil {
		t.Fatal(err)
	}
	if got, err = s.GetClusterMeta(); err != nil || got != want {
		t.Errorf("GetClusterMeta = %+v, %v; want %+v", got, err, want)
	}

	// An unreadable legacy value degrades to the zero value rather than
	// failing the build.
	if err := s.PutMeta(MetaClusterKey, "not json"); err != nil {
		t.Fatal(err)
	}
	if got, err = s.GetClusterMeta(); err != nil {
		t.Errorf("a corrupt cluster meta value must not error, got %v", err)
	} else if got != (ClusterMeta{}) {
		t.Errorf("corrupt value = %+v, want the zero value", got)
	}
}

func idsOf(nodes []model.Node) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.ID
	}
	return out
}
