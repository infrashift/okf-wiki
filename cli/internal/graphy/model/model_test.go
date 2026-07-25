package model

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestRationaleQualRoundTrips(t *testing.T) {
	for _, tc := range []struct {
		owner, marker string
		ord           int
	}{
		{"Store.PutFile", "WHY", 0},
		{"Store.PutFile", "TODO", 3},
		{"", "ADR", 0},                      // file-level rationale
		{"pkg.Type.Method", "DECISION", 12}, // dotted owner
	} {
		qual := RationaleQual(tc.owner, tc.marker, tc.ord)
		owner, marker, ord, ok := ParseRationaleQual(qual)
		if !ok {
			t.Fatalf("ParseRationaleQual(%q) not ok", qual)
		}
		if owner != tc.owner || marker != tc.marker || ord != tc.ord {
			t.Errorf("round trip of %q gave %q/%q/%d, want %q/%q/%d",
				qual, owner, marker, ord, tc.owner, tc.marker, tc.ord)
		}
	}
}

// TestRationaleQualSurvivesNodeIDParsing guards the reason the separator is
// '@' and not '#': node ids are parsed by splitting on the LAST '#', so a '#'
// inside the qualname would silently truncate the identity.
func TestRationaleQualSurvivesNodeIDParsing(t *testing.T) {
	qual := RationaleQual("Store.PutFile", "WHY", 0)
	id := NodeID("go", KindRationale, "cli/store.go", qual)
	want := "go:rationale:cli/store.go#Store.PutFile@WHY@0"
	if id != want {
		t.Fatalf("NodeID = %q, want %q", id, want)
	}
	for _, r := range qual {
		if r == '#' {
			t.Fatalf("rationale qual %q contains '#', which breaks node id parsing", qual)
		}
	}
}

func TestParseRationaleQualRejectsMalformed(t *testing.T) {
	for _, bad := range []string{"", "Store.PutFile", "Store.PutFile@WHY", "Store.PutFile@WHY@x", "@@0"} {
		if _, _, _, ok := ParseRationaleQual(bad); ok {
			t.Errorf("ParseRationaleQual(%q) reported ok, want rejection", bad)
		}
	}
}

func TestIndexableExcludesProseAndScaffolding(t *testing.T) {
	for kind, want := range map[Kind]bool{
		KindFunction:  true,
		KindMethod:    true,
		KindStruct:    true,
		KindResource:  true,
		KindFile:      false,
		KindDoc:       false,
		KindHeading:   false,
		KindConfig:    false,
		KindExternal:  false,
		KindRationale: false, // prose labels must never resolve a code reference
	} {
		if got := Indexable(Node{Kind: kind, Label: "x"}); got != want {
			t.Errorf("Indexable(%s) = %v, want %v", kind, got, want)
		}
	}
	if Indexable(Node{Kind: KindFunction, Label: "x", External: true}) {
		t.Error("external nodes must not be indexable")
	}
}

// TestEncodeGraphJSON covers the graph.json serializer. It had no test despite
// being the production path for `graphy build` and `graphy export json`.
func TestEncodeGraphJSON(t *testing.T) {
	nodes := []Node{
		{ID: "z", Label: "Zed", Kind: KindFunction, Language: "go"},
		{ID: "a", Label: "Ay", Kind: KindFunction, Language: "go"},
		{ID: "m", Label: "Em", Kind: KindRationale, Language: "go"},
	}
	edges := []Edge{
		{Source: "z", Target: "a", Relation: RelCalls, Confidence: ConfExtracted},
		{Source: "a", Target: "z", Relation: RelCalls, Confidence: ConfInferred},
		{Source: "a", Target: "m", Relation: RelExplains, Confidence: ConfExtracted},
	}

	var buf bytes.Buffer
	if err := EncodeGraphJSON(&buf, "/repo", nodes, edges, 2, 4); err != nil {
		t.Fatal(err)
	}

	var doc GraphJSON
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if doc.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", doc.SchemaVersion, SchemaVersion)
	}
	if doc.Root != "/repo" {
		t.Errorf("Root = %q", doc.Root)
	}
	if doc.GeneratedAt == "" {
		t.Error("GeneratedAt is empty")
	}
	// Stats are derived from the slices, not from the caller's counts.
	if doc.Stats.Nodes != 3 || doc.Stats.Edges != 3 {
		t.Errorf("Stats = %+v, want 3 nodes / 3 edges", doc.Stats)
	}
	if doc.Stats.Communities != 2 || doc.Stats.Files != 4 {
		t.Errorf("Stats = %+v, want 2 communities / 4 files", doc.Stats)
	}

	// Sorting is the contract that lets the document be committed and diffed.
	gotNodes := []string{doc.Nodes[0].ID, doc.Nodes[1].ID, doc.Nodes[2].ID}
	if !slices.IsSorted(gotNodes) {
		t.Errorf("nodes are not sorted by id: %v", gotNodes)
	}
	for i := 1; i < len(doc.Edges); i++ {
		prev, cur := doc.Edges[i-1], doc.Edges[i]
		if prev.Source > cur.Source ||
			(prev.Source == cur.Source && prev.Relation > cur.Relation) ||
			(prev.Source == cur.Source && prev.Relation == cur.Relation && prev.Target > cur.Target) {
			t.Errorf("edges are not sorted by (source, relation, target): %+v then %+v", prev, cur)
		}
	}
}

// TestEncodeGraphJSONIsOrderIndependent pins the property the sort exists for:
// the same graph supplied in a different order produces identical bytes, apart
// from the timestamp.
func TestEncodeGraphJSONIsOrderIndependent(t *testing.T) {
	a := []Node{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	b := []Node{{ID: "c"}, {ID: "a"}, {ID: "b"}}
	e1 := []Edge{{Source: "a", Target: "b", Relation: RelCalls}, {Source: "b", Target: "c", Relation: RelCalls}}
	e2 := []Edge{{Source: "b", Target: "c", Relation: RelCalls}, {Source: "a", Target: "b", Relation: RelCalls}}

	var first, second bytes.Buffer
	if err := EncodeGraphJSON(&first, "/repo", a, e1, 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := EncodeGraphJSON(&second, "/repo", b, e2, 1, 1); err != nil {
		t.Fatal(err)
	}

	strip := func(s string) []string {
		var out []string
		for _, line := range strings.Split(s, "\n") {
			if !strings.Contains(line, `"generated_at"`) {
				out = append(out, line)
			}
		}
		return out
	}
	if got, want := strip(second.String()), strip(first.String()); !slices.Equal(got, want) {
		t.Errorf("input order changed the output:\n%s\n---\n%s", first.String(), second.String())
	}
}
