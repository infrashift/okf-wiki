package graphdb

import (
	"testing"
)

func TestValidateEdge_Valid(t *testing.T) {
	edges := []EdgeRecord{
		{From: "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", Predicate: PredicateLinksTo, To: "20260214100000-11111111-2222-3333-4444-555555555555"},
		{From: "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", Predicate: PredicateParentOf, To: "20260214100000-11111111-2222-3333-4444-555555555555"},
		{From: "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", Predicate: PredicateChildOf, To: "20260214100000-11111111-2222-3333-4444-555555555555"},
		{From: "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", Predicate: PredicateTagged, To: "golang"},
	}

	for _, e := range edges {
		if err := ValidateEdge(e); err != nil {
			t.Errorf("expected valid edge %s -[%s]-> %s, got error: %v", e.From, e.Predicate, e.To, err)
		}
	}
}

func TestValidateEdge_UUIDv7(t *testing.T) {
	edges := []EdgeRecord{
		{From: "019680f0-7e00-7abc-8def-1234567890ab", Predicate: PredicateLinksTo, To: "01968109-4700-7def-9abc-ffffffffffff"},
		{From: "019680f0-7e00-7abc-8def-1234567890ab", Predicate: PredicateTagged, To: "golang"},
	}

	for _, e := range edges {
		if err := ValidateEdge(e); err != nil {
			t.Errorf("expected valid UUIDv7 edge %s -[%s]-> %s, got error: %v", e.From, e.Predicate, e.To, err)
		}
	}
}

func TestValidateEdge_InvalidFrom(t *testing.T) {
	e := EdgeRecord{From: "not-a-uuid", Predicate: PredicateLinksTo, To: "20260214100000-11111111-2222-3333-4444-555555555555"}
	if err := ValidateEdge(e); err == nil {
		t.Error("expected error for invalid 'from' ID")
	}
}

func TestValidateEdge_InvalidPredicate(t *testing.T) {
	e := EdgeRecord{
		From:      "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		Predicate: "invalid_pred",
		To:        "20260214100000-11111111-2222-3333-4444-555555555555",
	}
	if err := ValidateEdge(e); err == nil {
		t.Error("expected error for invalid predicate")
	}
}

func TestValidateEdge_EmptyTo(t *testing.T) {
	e := EdgeRecord{
		From:      "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		Predicate: PredicateLinksTo,
		To:        "",
	}
	if err := ValidateEdge(e); err == nil {
		t.Error("expected error for empty 'to' field")
	}
}

func TestValidateEdges_Mixed(t *testing.T) {
	edges := []EdgeRecord{
		{From: "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", Predicate: PredicateLinksTo, To: "20260214100000-11111111-2222-3333-4444-555555555555"},
		{From: "bad-id", Predicate: PredicateLinksTo, To: "20260214100000-11111111-2222-3333-4444-555555555555"},
	}

	errs := ValidateEdges(edges)
	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d", len(errs))
	}
}

func TestValidateIntegrity(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()

	idA := "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	nonExistent := "20260214100000-11111111-2222-3333-4444-555555555555"

	writeZettel(t, dir, idA, "A", "", "untethered", nil, "", []string{nonExistent}, "")
	s.Sync(dir)

	result, err := s.ValidateIntegrity()
	if err != nil {
		t.Fatalf("ValidateIntegrity error: %v", err)
	}
	if result.TotalNodes != 1 {
		t.Errorf("expected 1 node, got %d", result.TotalNodes)
	}
	if len(result.DanglingEdges) != 1 {
		t.Errorf("expected 1 dangling edge, got %d", len(result.DanglingEdges))
	}
	if len(result.InvalidEdges) != 0 {
		t.Errorf("expected 0 invalid edges, got %d: %v", len(result.InvalidEdges), result.InvalidEdges)
	}

	// String output should mention dangling.
	s_output := result.String()
	if len(s_output) == 0 {
		t.Error("expected non-empty string output")
	}
}

func TestValidateIntegrity_Clean(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()

	idA := "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	idB := "20260214100000-11111111-2222-3333-4444-555555555555"

	writeZettel(t, dir, idA, "A", "", "untethered", nil, "", []string{idB}, "")
	writeZettel(t, dir, idB, "B", "", "untethered", nil, "", nil, "")
	s.Sync(dir)

	result, err := s.ValidateIntegrity()
	if err != nil {
		t.Fatalf("ValidateIntegrity error: %v", err)
	}
	if len(result.DanglingEdges) != 0 {
		t.Errorf("expected 0 dangling edges, got %d", len(result.DanglingEdges))
	}
	if len(result.InvalidEdges) != 0 {
		t.Errorf("expected 0 invalid edges, got %d", len(result.InvalidEdges))
	}
}
