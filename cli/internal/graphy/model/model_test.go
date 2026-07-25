package model

import "testing"

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
