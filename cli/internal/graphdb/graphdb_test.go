package graphdb

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// helper: create a temp DB, return store + cleanup func
func testStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// helper: write a concept file to disk
func writeConcept(t *testing.T, dir, id, title, project string, tags []string, parent string, links []string, body string) string {
	t.Helper()
	filename := id + ".md"
	path := filepath.Join(dir, filename)

	fm := "---\n"
	fm += "id: \"" + id + "\"\n"
	fm += "title: \"" + title + "\"\n"
	if project != "" {
		fm += "project: \"" + project + "\"\n"
	}
	if len(tags) > 0 {
		fm += "tags:\n"
		for _, t := range tags {
			fm += "  - \"" + t + "\"\n"
		}
	} else {
		fm += "tags: []\n"
	}
	fm += "created: \"2026-02-13T10:45:00Z\"\n"
	if parent != "" {
		fm += "parent: \"" + parent + "\"\n"
	}
	fm += "---\n\n"

	content := fm + "# " + title + "\n\n" + body
	for _, l := range links {
		// Vanilla markdown link to the target concept's file (flat test layout).
		content += "\nSee [link](" + l + ".md) for details.\n"
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write concept: %v", err)
	}
	return path
}

func TestOpenAndClose(t *testing.T) {
	s := testStore(t)
	count, err := s.NodeCount()
	if err != nil {
		t.Fatalf("NodeCount error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 nodes, got %d", count)
	}
}

func TestSyncFile(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()

	id := "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	writeConcept(t, dir, id, "Test Note", "myproject", []string{"go", "test"}, "", nil, "Hello world")

	path := filepath.Join(dir, id+".md")
	if err := s.SyncFile(path, dir); err != nil {
		t.Fatalf("SyncFile error: %v", err)
	}

	node, err := s.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode error: %v", err)
	}
	if node == nil {
		t.Fatal("expected node to exist")
	}
	if node.Title != "Test Note" {
		t.Errorf("expected title 'Test Note', got %q", node.Title)
	}
	if node.Project != "myproject" {
		t.Errorf("expected project 'myproject', got %q", node.Project)
	}
	if len(node.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(node.Tags))
	}
}

func TestSyncFull(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()

	id1 := "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	id2 := "20260214100000-11111111-2222-3333-4444-555555555555"

	writeConcept(t, dir, id1, "Note A", "", []string{"alpha"}, "", []string{id2}, "Links to B")
	writeConcept(t, dir, id2, "Note B", "", []string{"beta"}, "", nil, "Standalone")

	result, err := s.Sync(dir)
	if err != nil {
		t.Fatalf("Sync error: %v", err)
	}
	if result.Added != 2 {
		t.Errorf("expected 2 added, got %d", result.Added)
	}
	if result.Total != 2 {
		t.Errorf("expected 2 total, got %d", result.Total)
	}

	// Verify nodes exist.
	n1, _ := s.GetNode(id1)
	n2, _ := s.GetNode(id2)
	if n1 == nil || n2 == nil {
		t.Fatal("expected both nodes to exist")
	}
}

func TestSyncIncremental(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()

	id := "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	writeConcept(t, dir, id, "Original Title", "", nil, "", nil, "")

	r1, err := s.Sync(dir)
	if err != nil {
		t.Fatalf("first sync error: %v", err)
	}
	if r1.Added != 1 {
		t.Errorf("expected 1 added, got %d", r1.Added)
	}

	// Second sync without changes — should skip.
	r2, err := s.Sync(dir)
	if err != nil {
		t.Fatalf("second sync error: %v", err)
	}
	if r2.Added != 0 || r2.Updated != 0 {
		t.Errorf("expected no changes on second sync, got added=%d updated=%d", r2.Added, r2.Updated)
	}
}

func TestSyncDeletesRemovedFiles(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()

	id1 := "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	id2 := "20260214100000-11111111-2222-3333-4444-555555555555"

	writeConcept(t, dir, id1, "Note A", "", nil, "", nil, "")
	writeConcept(t, dir, id2, "Note B", "", nil, "", nil, "")

	s.Sync(dir)

	// Delete one file.
	os.Remove(filepath.Join(dir, id2+".md"))

	result, err := s.Sync(dir)
	if err != nil {
		t.Fatalf("Sync error: %v", err)
	}
	if result.Deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", result.Deleted)
	}

	n2, _ := s.GetNode(id2)
	if n2 != nil {
		t.Error("expected deleted node to be gone")
	}
}

func TestPurgeNode(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()

	id1 := "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	id2 := "20260214100000-11111111-2222-3333-4444-555555555555"

	writeConcept(t, dir, id1, "Note A", "", []string{"go"}, "", []string{id2}, "Links to B")
	writeConcept(t, dir, id2, "Note B", "", nil, "", nil, "")

	s.Sync(dir)

	if err := s.PurgeNode(id1); err != nil {
		t.Fatalf("PurgeNode error: %v", err)
	}

	n1, _ := s.GetNode(id1)
	if n1 != nil {
		t.Error("expected purged node to be gone")
	}

	// Backlinks for id2 should no longer include id1.
	backlinks, _ := s.GetBacklinks(id2)
	for _, bl := range backlinks {
		if bl.ID == id1 {
			t.Error("expected backlink from purged node to be cleaned up")
		}
	}
}

func TestParentChildEdges(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()

	parentID := "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	childID := "20260214100000-11111111-2222-3333-4444-555555555555"

	writeConcept(t, dir, parentID, "Parent", "", nil, "", nil, "")
	writeConcept(t, dir, childID, "Child", "", nil, parentID, nil, "")

	s.Sync(dir)

	// Child should show up in backlinks of parent.
	backlinks, err := s.GetBacklinks(parentID)
	if err != nil {
		t.Fatalf("GetBacklinks error: %v", err)
	}
	if len(backlinks) != 1 || backlinks[0].ID != childID {
		t.Errorf("expected child in parent's backlinks, got %v", backlinks)
	}

	// Children should be discoverable via edges.
	children, err := s.getChildIDs(parentID)
	if err != nil {
		t.Fatalf("getChildIDs error: %v", err)
	}
	if len(children) != 1 || children[0] != childID {
		t.Errorf("expected child ID, got %v", children)
	}
}

func TestExportJSON(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()

	id := "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	writeConcept(t, dir, id, "Test", "", []string{"go"}, "", nil, "")
	s.Sync(dir)

	var buf struct {
		Nodes []NodeRecord `json:"nodes"`
		Edges []EdgeRecord `json:"edges"`
	}

	r, w, _ := os.Pipe()
	go func() {
		s.ExportJSON(w)
		w.Close()
	}()

	if err := json.NewDecoder(r).Decode(&buf); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if len(buf.Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(buf.Nodes))
	}
}
