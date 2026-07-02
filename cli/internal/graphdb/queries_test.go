package graphdb

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestGetBacklinks(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()

	idA := "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	idB := "20260214100000-11111111-2222-3333-4444-555555555555"
	idC := "20260215100000-22222222-3333-4444-5555-666666666666"

	writeConcept(t, dir, idA, "Note A", "", nil, "", []string{idB}, "")
	writeConcept(t, dir, idB, "Note B", "", nil, "", nil, "")
	writeConcept(t, dir, idC, "Note C", "", nil, "", []string{idB}, "")

	s.Sync(dir)

	backlinks, err := s.GetBacklinks(idB)
	if err != nil {
		t.Fatalf("GetBacklinks error: %v", err)
	}
	if len(backlinks) != 2 {
		t.Fatalf("expected 2 backlinks, got %d", len(backlinks))
	}

	ids := make(map[string]bool)
	for _, bl := range backlinks {
		ids[bl.ID] = true
	}
	if !ids[idA] || !ids[idC] {
		t.Errorf("expected backlinks from A and C, got %v", ids)
	}
}

func TestGetBacklinks_None(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()

	id := "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	writeConcept(t, dir, id, "Isolated", "", nil, "", nil, "")
	s.Sync(dir)

	backlinks, err := s.GetBacklinks(id)
	if err != nil {
		t.Fatalf("GetBacklinks error: %v", err)
	}
	if len(backlinks) != 0 {
		t.Errorf("expected 0 backlinks, got %d", len(backlinks))
	}
}

func TestGetCluster(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()

	idA := "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	idB := "20260214100000-11111111-2222-3333-4444-555555555555"
	idC := "20260215100000-22222222-3333-4444-5555-666666666666"
	idD := "20260216100000-33333333-4444-5555-6666-777777777777"

	// A -> B -> C -> D (linear chain)
	writeConcept(t, dir, idA, "A", "", nil, "", []string{idB}, "")
	writeConcept(t, dir, idB, "B", "", nil, "", []string{idC}, "")
	writeConcept(t, dir, idC, "C", "", nil, "", []string{idD}, "")
	writeConcept(t, dir, idD, "D", "", nil, "", nil, "")

	s.Sync(dir)

	// Depth 1 from A: A + B
	nodes, _, err := s.GetCluster(idA, 1, 100)
	if err != nil {
		t.Fatalf("GetCluster error: %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes at depth 1, got %d", len(nodes))
	}

	// Depth 2 from A: A + B + C
	nodes, _, err = s.GetCluster(idA, 2, 100)
	if err != nil {
		t.Fatalf("GetCluster error: %v", err)
	}
	if len(nodes) != 3 {
		t.Errorf("expected 3 nodes at depth 2, got %d", len(nodes))
	}

	// Unlimited depth: all 4
	nodes, edges, err := s.GetCluster(idA, 0, 100)
	if err != nil {
		t.Fatalf("GetCluster error: %v", err)
	}
	if len(nodes) != 4 {
		t.Errorf("expected 4 nodes at unlimited depth, got %d", len(nodes))
	}
	if len(edges) == 0 {
		t.Error("expected edges in result")
	}
}

func TestGetCluster_WithLimit(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()

	idA := "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	idB := "20260214100000-11111111-2222-3333-4444-555555555555"
	idC := "20260215100000-22222222-3333-4444-5555-666666666666"

	writeConcept(t, dir, idA, "A", "", nil, "", []string{idB, idC}, "")
	writeConcept(t, dir, idB, "B", "", nil, "", nil, "")
	writeConcept(t, dir, idC, "C", "", nil, "", nil, "")

	s.Sync(dir)

	nodes, _, err := s.GetCluster(idA, 0, 2)
	if err != nil {
		t.Fatalf("GetCluster error: %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes with limit=2, got %d", len(nodes))
	}
}

func TestGetCluster_ReverseLinks(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()

	idA := "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	idB := "20260214100000-11111111-2222-3333-4444-555555555555"

	// B links to A, so from A we should discover B via backlinks.
	writeConcept(t, dir, idA, "Target", "", nil, "", nil, "")
	writeConcept(t, dir, idB, "Linker", "", nil, "", []string{idA}, "")

	s.Sync(dir)

	nodes, _, err := s.GetCluster(idA, 0, 100)
	if err != nil {
		t.Fatalf("GetCluster error: %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes (A + reverse-linked B), got %d", len(nodes))
	}
}

func TestGetCluster_NotFound(t *testing.T) {
	s := testStore(t)
	_, _, err := s.GetCluster("nonexistent", 0, 10)
	if err == nil {
		t.Error("expected error for nonexistent node")
	}
}

func TestGetByTag(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()

	idA := "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	idB := "20260214100000-11111111-2222-3333-4444-555555555555"
	idC := "20260215100000-22222222-3333-4444-5555-666666666666"

	writeConcept(t, dir, idA, "A", "", []string{"golang", "test"}, "", nil, "")
	writeConcept(t, dir, idB, "B", "", []string{"golang"}, "", nil, "")
	writeConcept(t, dir, idC, "C", "", []string{"rust"}, "", nil, "")

	s.Sync(dir)

	goNodes, err := s.GetByTag("golang")
	if err != nil {
		t.Fatalf("GetByTag error: %v", err)
	}
	if len(goNodes) != 2 {
		t.Errorf("expected 2 nodes tagged 'golang', got %d", len(goNodes))
	}

	rustNodes, _ := s.GetByTag("rust")
	if len(rustNodes) != 1 {
		t.Errorf("expected 1 node tagged 'rust', got %d", len(rustNodes))
	}

	noNodes, _ := s.GetByTag("python")
	if len(noNodes) != 0 {
		t.Errorf("expected 0 nodes tagged 'python', got %d", len(noNodes))
	}
}

func TestGetDanglingEdges(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()

	idA := "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	nonExistent := "20260214100000-11111111-2222-3333-4444-555555555555"

	// A links to a non-existent node.
	writeConcept(t, dir, idA, "A", "", nil, nonExistent, nil, "")

	s.Sync(dir)

	dangling, err := s.GetDanglingEdges()
	if err != nil {
		t.Fatalf("GetDanglingEdges error: %v", err)
	}
	if len(dangling) != 1 {
		t.Fatalf("expected 1 dangling edge, got %d", len(dangling))
	}
	if dangling[0].To != nonExistent {
		t.Errorf("expected dangling edge to %s, got %s", nonExistent, dangling[0].To)
	}
}

func TestToGraph(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()

	idA := "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	idB := "20260214100000-11111111-2222-3333-4444-555555555555"

	writeConcept(t, dir, idA, "Note A", "", nil, "", []string{idB}, "")
	writeConcept(t, dir, idB, "Note B", "", nil, "", nil, "")

	s.Sync(dir)

	g, err := s.ToGraph()
	if err != nil {
		t.Fatalf("ToGraph error: %v", err)
	}
	if g.NodeCount() != 2 {
		t.Errorf("expected 2 nodes in graph, got %d", g.NodeCount())
	}

	nodeA := g.GetNode(idA)
	if nodeA == nil {
		t.Fatal("expected node A in graph")
	}
	if len(nodeA.Links) != 1 || nodeA.Links[0] != idB {
		t.Errorf("expected node A to link to B, got %v", nodeA.Links)
	}
}

func TestExportJSON_Full(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()

	idA := "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	idB := "20260214100000-11111111-2222-3333-4444-555555555555"

	writeConcept(t, dir, idA, "A", "", nil, "", []string{idB}, "")
	writeConcept(t, dir, idB, "B", "", nil, "", nil, "")

	s.Sync(dir)

	var buf bytes.Buffer
	if err := s.ExportJSON(&buf); err != nil {
		t.Fatalf("ExportJSON error: %v", err)
	}

	var result GraphJSON
	if err := json.NewDecoder(&buf).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if len(result.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(result.Nodes))
	}
	if len(result.Edges) == 0 {
		t.Error("expected at least 1 edge")
	}
}
