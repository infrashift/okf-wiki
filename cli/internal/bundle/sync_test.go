package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"okfw/cli/internal/config"
	"okfw/cli/internal/graphdb"
	"okfw/cli/internal/index"
)

// fakeIndexer records what IndexFile hands to bleve without building an index.
type fakeIndexer struct {
	docs map[string]*index.ConceptDoc
}

func (f *fakeIndexer) Index(id string, doc interface{}) error {
	if f.docs == nil {
		f.docs = map[string]*index.ConceptDoc{}
	}
	f.docs[id] = doc.(*index.ConceptDoc)
	return nil
}

func writeConcept(t *testing.T, root, rel, content string) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

// --- IndexFile ---

func TestIndexFileMapsFrontmatterToDoc(t *testing.T) {
	root := t.TempDir()
	const id = "019680f0-7e00-7abc-8def-1234567890ab"
	p := writeConcept(t, root, "alpha--0001.md", "---\n"+
		"id: \""+id+"\"\n"+
		"title: \"Alpha\"\n"+
		"type: \"note\"\n"+
		"kind: \"issue\"\n"+
		"description: \"A summary.\"\n"+
		"project: \"proj\"\n"+
		"tags:\n  - \"x\"\n"+
		"created: \"2026-02-13T10:45:00Z\"\n"+
		"timestamp: \"2026-02-13T10:45:00Z\"\n"+
		"status: \"draft\"\n"+
		"stale_after: \"2026-09-01\"\n"+
		"---\n\nBody words here.\n")

	f := &fakeIndexer{}
	if err := IndexFile(f, p); err != nil {
		t.Fatalf("IndexFile: %v", err)
	}
	doc := f.docs[id]
	if doc == nil {
		t.Fatalf("nothing indexed under %q; got %v", id, f.docs)
	}
	if doc.Title != "Alpha" || doc.Type != "note" || doc.Kind != "issue" {
		t.Errorf("title/type/kind = %q/%q/%q", doc.Title, doc.Type, doc.Kind)
	}
	if doc.Description != "A summary." || doc.Project != "proj" {
		t.Errorf("description/project = %q/%q", doc.Description, doc.Project)
	}
	if doc.Status != "draft" || doc.StaleAfter != "2026-09-01" {
		t.Errorf("status/stale_after = %q/%q", doc.Status, doc.StaleAfter)
	}
	if !strings.Contains(doc.Body, "Body words here.") {
		t.Errorf("body = %q", doc.Body)
	}
	if doc.FilePath != p {
		t.Errorf("file path = %q, want %q", doc.FilePath, p)
	}
}

// Notes written before `type` existed carry only `kind`; the indexed type falls
// back to it so they stay filterable.
func TestIndexFileFallsBackToKindForLegacyType(t *testing.T) {
	root := t.TempDir()
	const id = "019680f0-7e00-7abc-8def-1234567890ab"
	p := writeConcept(t, root, "legacy--0001.md", "---\n"+
		"id: \""+id+"\"\n"+
		"title: \"Legacy\"\n"+
		"kind: \"daily-note\"\n"+
		"tags: []\n"+
		"created: \"2026-02-13T10:45:00Z\"\n"+
		"timestamp: \"2026-02-13T10:45:00Z\"\n"+
		"---\n\nBody.\n")
	f := &fakeIndexer{}
	if err := IndexFile(f, p); err != nil {
		t.Fatalf("IndexFile: %v", err)
	}
	if got := f.docs[id].Type; got != "daily-note" {
		t.Errorf("Type = %q, want the kind fallback \"daily-note\"", got)
	}
}

func TestIndexFileRejectsUnparseableFrontmatter(t *testing.T) {
	root := t.TempDir()
	p := writeConcept(t, root, "bad.md", "no frontmatter at all\n")
	if err := IndexFile(&fakeIndexer{}, p); err == nil {
		t.Error("expected an error for a file without frontmatter")
	}
}

// --- WriteIndexFiles ---

func TestWriteIndexFilesStampsOkfVersionAtRootOnly(t *testing.T) {
	root := t.TempDir()
	cfg := config.ConfigFor(root)
	writeConcept(t, root, "top--0001.md", GenerateBasicNote("019680f0-7e00-7abc-8def-100000000000", "Top", "", nil))
	writeConcept(t, root, "area/inner--0002.md", GenerateBasicNote("019680f0-7e00-7abc-8def-200000000000", "Inner", "", nil))

	n, err := WriteIndexFiles(cfg, root)
	if err != nil {
		t.Fatalf("WriteIndexFiles: %v", err)
	}
	if n != 2 {
		t.Errorf("wrote %d index files, want 2 (root + area)", n)
	}

	rootIdx, err := os.ReadFile(filepath.Join(root, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rootIdx), `okf_version: "`+config.OKFVersion+`"`) {
		t.Errorf("root index must declare okf_version, got:\n%s", rootIdx)
	}
	if !strings.Contains(string(rootIdx), "## Subdirectories") || !strings.Contains(string(rootIdx), "[area/](area/)") {
		t.Errorf("root index should list the subdirectory, got:\n%s", rootIdx)
	}
	if !strings.Contains(string(rootIdx), "[Top](top--0001.md)") {
		t.Errorf("root index should link the concept, got:\n%s", rootIdx)
	}

	areaIdx, err := os.ReadFile(filepath.Join(root, "area", "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(string(areaIdx), "---") {
		t.Errorf("non-root index files must carry no frontmatter, got:\n%s", areaIdx)
	}
	if !strings.Contains(string(areaIdx), "[Inner](inner--0002.md)") {
		t.Errorf("area index should link its concept, got:\n%s", areaIdx)
	}
}

func TestWriteIndexFilesIncludesDescriptionAndIsRerunnable(t *testing.T) {
	root := t.TempDir()
	cfg := config.ConfigFor(root)
	note := GenerateBasicNote("019680f0-7e00-7abc-8def-100000000000", "Described", "", nil)
	withDesc, err := UpdateFrontmatter([]byte(note), map[string]string{"description": "The summary."})
	if err != nil {
		t.Fatal(err)
	}
	writeConcept(t, root, "described--0001.md", string(withDesc))

	if _, err := WriteIndexFiles(cfg, root); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(root, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(first), "- The summary.") {
		t.Errorf("index entry should carry the description, got:\n%s", first)
	}
	// Re-running must be a no-op, not an append.
	if _, err := WriteIndexFiles(cfg, root); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(root, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("WriteIndexFiles is not idempotent:\n%s\n---\n%s", first, second)
	}
}

// Deep nesting must produce an index at every level, including directories that
// hold only subdirectories and no concepts of their own.
func TestWriteIndexFilesSynthesizesIntermediateDirs(t *testing.T) {
	root := t.TempDir()
	cfg := config.ConfigFor(root)
	writeConcept(t, root, "a/b/c/deep--0001.md", GenerateBasicNote("019680f0-7e00-7abc-8def-100000000000", "Deep", "", nil))

	if _, err := WriteIndexFiles(cfg, root); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"index.md", "a/index.md", "a/b/index.md", "a/b/c/index.md"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("missing %s: %v", rel, err)
		}
	}
	mid, err := os.ReadFile(filepath.Join(root, "a", "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mid), "[b/](b/)") {
		t.Errorf("intermediate index should link its subdirectory, got:\n%s", mid)
	}
}

func TestWriteIndexFilesIgnoresUnparseableAndHidden(t *testing.T) {
	root := t.TempDir()
	cfg := config.ConfigFor(root)
	writeConcept(t, root, "good--0001.md", GenerateBasicNote("019680f0-7e00-7abc-8def-100000000000", "Good", "", nil))
	writeConcept(t, root, "junk.md", "not a concept\n")
	writeConcept(t, root, ".index/hidden.md", GenerateBasicNote("019680f0-7e00-7abc-8def-300000000000", "Hidden", "", nil))
	writeConcept(t, root, "ephemeral/scratch.md", GenerateBasicNote("019680f0-7e00-7abc-8def-400000000000", "Scratch", "", nil))

	if _, err := WriteIndexFiles(cfg, root); err != nil {
		t.Fatal(err)
	}
	idx, err := os.ReadFile(filepath.Join(root, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(idx)
	if !strings.Contains(s, "[Good]") {
		t.Errorf("valid concept missing:\n%s", s)
	}
	for _, unwanted := range []string{"junk.md", "Hidden", "Scratch", ".index", "ephemeral"} {
		if strings.Contains(s, unwanted) {
			t.Errorf("index should not mention %q, got:\n%s", unwanted, s)
		}
	}
}

// --- AutoSync ---

func TestAutoSyncWithoutGraphDB(t *testing.T) {
	root := t.TempDir()
	cfg := config.ConfigFor(root)
	p := writeConcept(t, root, "alpha--0001.md", GenerateBasicNote("019680f0-7e00-7abc-8def-100000000000", "Alpha", "", nil))

	// No .graph.db exists: AutoSync must index, refresh index.md, and not error.
	if err := AutoSync(cfg, p); err != nil {
		t.Fatalf("AutoSync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "index.md")); err != nil {
		t.Errorf("AutoSync should have written index.md: %v", err)
	}
	if _, err := os.Stat(cfg.IndexPath); err != nil {
		t.Errorf("AutoSync should have created the search index: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, cfg.GraphPath)); err == nil {
		t.Error("AutoSync must not create a graph database that did not exist")
	}
}

func TestAutoSyncUpdatesExistingGraphDB(t *testing.T) {
	root := t.TempDir()
	cfg := config.ConfigFor(root)
	const id = "019680f0-7e00-7abc-8def-100000000000"
	p := writeConcept(t, root, "alpha--0001.md", GenerateBasicNote(id, "Alpha", "", nil))

	// Create the graph database so AutoSync takes the syncing branch.
	store, err := graphdb.Open(filepath.Join(root, cfg.GraphPath))
	if err != nil {
		t.Fatalf("graphdb.Open: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	if err := AutoSync(cfg, p); err != nil {
		t.Fatalf("AutoSync: %v", err)
	}

	store, err = graphdb.Open(filepath.Join(root, cfg.GraphPath))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	node, err := store.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if node == nil || node.Title != "Alpha" {
		t.Errorf("AutoSync did not record the node in the graph: %+v", node)
	}
}

func TestAutoIndexMakesConceptSearchable(t *testing.T) {
	root := t.TempDir()
	cfg := config.ConfigFor(root)
	p := writeConcept(t, root, "findme--0001.md",
		GenerateBasicNote("019680f0-7e00-7abc-8def-100000000000", "Findable Title", "", nil))

	if err := AutoIndex(cfg, p); err != nil {
		t.Fatalf("AutoIndex: %v", err)
	}
	idx, err := index.OpenOrCreateIndex(cfg.IndexPath)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	results, total, err := index.Search(idx, index.SearchOptions{Query: "Findable"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Fatalf("total=%d results=%d, want 1", total, len(results))
	}
	if results[0].Title != "Findable Title" {
		t.Errorf("title = %q", results[0].Title)
	}
}

// Emptying a directory used to leave its generated listing behind, still naming
// the concepts that had been there.
func TestWriteIndexFilesPrunesOrphanedListings(t *testing.T) {
	root := t.TempDir()
	cfg := config.ConfigFor(root)
	p := writeConcept(t, root, "archive/gone--0001.md",
		GenerateBasicNote("019680f0-7e00-7abc-8def-100000000000", "Gone", "", nil))
	writeConcept(t, root, "kept--0002.md",
		GenerateBasicNote("019680f0-7e00-7abc-8def-200000000000", "Kept", "", nil))

	if _, err := WriteIndexFiles(cfg, root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "archive", "index.md")); err != nil {
		t.Fatalf("archive/index.md should exist while the concept does: %v", err)
	}

	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteIndexFiles(cfg, root); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(root, "archive", "index.md")); err == nil {
		t.Error("the orphaned listing should have been removed")
	}
	// The emptied directory goes too, rather than lingering as noise.
	if _, err := os.Stat(filepath.Join(root, "archive")); err == nil {
		t.Error("the emptied directory should have been removed")
	}
	// The root listing and surviving concepts are untouched.
	rootIdx, err := os.ReadFile(filepath.Join(root, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rootIdx), "Kept") {
		t.Errorf("the root listing lost a live concept:\n%s", rootIdx)
	}
	if strings.Contains(string(rootIdx), "archive/") {
		t.Errorf("the root listing still advertises the empty directory:\n%s", rootIdx)
	}
}

// A directory holding other files must survive losing its concepts.
func TestWriteIndexFilesKeepsNonEmptyDirectories(t *testing.T) {
	root := t.TempDir()
	cfg := config.ConfigFor(root)
	p := writeConcept(t, root, "area/gone--0001.md",
		GenerateBasicNote("019680f0-7e00-7abc-8def-100000000000", "Gone", "", nil))
	if err := os.WriteFile(filepath.Join(root, "area", "attachment.png"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := WriteIndexFiles(cfg, root); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteIndexFiles(cfg, root); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(root, "area", "attachment.png")); err != nil {
		t.Errorf("a non-concept file was destroyed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "area")); err != nil {
		t.Errorf("a directory with other content should survive: %v", err)
	}
}

// The bundle-root index.md is never orphaned, even in an empty bundle.
func TestWriteIndexFilesKeepsTheRootListing(t *testing.T) {
	root := t.TempDir()
	cfg := config.ConfigFor(root)
	if _, err := WriteIndexFiles(cfg, root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "index.md")); err != nil {
		t.Errorf("the root listing must always exist: %v", err)
	}
}

// TestIndexFileCarriesTheGraphyNodeID: a generated concept records the code
// entity it documents, and indexing it as a keyword is what makes the reverse
// bridge (concept → graph node) a lookup rather than a full-text guess.
func TestIndexFileCarriesTheGraphyNodeID(t *testing.T) {
	root := t.TempDir()
	p := writeConcept(t, root, "graphy/proj/alpha--019fa4c27a5d.md", `---
id: "019fa4c2-7a5d-7851-9c72-372f3a48473f"
title: "Alpha"
type: "code-function"
tags:
  - "graphy"
created: "2026-07-27T00:00:00Z"
timestamp: "2026-07-27T00:00:00Z"
graphy:
  node_id: "go:function:a.go#Alpha"
  entity: "function"
---

# Alpha
`)
	f := &fakeIndexer{}
	if err := IndexFile(f, p); err != nil {
		t.Fatal(err)
	}
	doc := f.docs["019fa4c2-7a5d-7851-9c72-372f3a48473f"]
	if doc == nil {
		t.Fatalf("concept was not indexed: %#v", f.docs)
	}
	if doc.GraphyNodeID != "go:function:a.go#Alpha" {
		t.Errorf("GraphyNodeID = %q, want the node id from frontmatter", doc.GraphyNodeID)
	}
}

// TestIndexFileLeavesGraphyNodeIDEmptyForHandWrittenConcepts: the field is an
// extension, and an ordinary concept must not acquire one.
func TestIndexFileLeavesGraphyNodeIDEmptyForHandWrittenConcepts(t *testing.T) {
	root := t.TempDir()
	p := writeConcept(t, root, "note.md", `---
id: "019fa4c2-7a5d-7851-9c72-372f3a48473e"
title: "Hand written"
type: "note"
tags: []
created: "2026-07-27T00:00:00Z"
timestamp: "2026-07-27T00:00:00Z"
---

# Hand written
`)
	f := &fakeIndexer{}
	if err := IndexFile(f, p); err != nil {
		t.Fatal(err)
	}
	doc := f.docs["019fa4c2-7a5d-7851-9c72-372f3a48473e"]
	if doc == nil {
		t.Fatalf("concept was not indexed: %#v", f.docs)
	}
	if doc.GraphyNodeID != "" {
		t.Errorf("GraphyNodeID = %q, want empty", doc.GraphyNodeID)
	}
}

// TestIndexFilesIndexesEveryFileInOnePass: the bulk path a large export needs,
// where AutoIndex would reopen Bleve once per concept.
func TestIndexFilesIndexesEveryFileInOnePass(t *testing.T) {
	root := t.TempDir()
	cfg := config.ConfigFor(root)
	var abs, want []string
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("019680f0-7e00-7abc-8def-10000000000%d", i)
		abs = append(abs, writeConcept(t, root, fmt.Sprintf("c%d--000%d.md", i, i),
			GenerateBasicNote(id, fmt.Sprintf("Concept %d", i), "", nil)))
		want = append(want, id)
	}
	if err := IndexFiles(cfg, abs); err != nil {
		t.Fatal(err)
	}
	got, err := IndexedIDs(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range want {
		if !got[id] {
			t.Errorf("concept %s was not indexed", id)
		}
	}
}

// TestIndexedIDsOnABundleWithNoIndexIsEmptyNotAnError: a reconciling caller
// must read "nothing is indexed", and must not create the index on the way
// past — that is the same defect as opening a graph database to read it.
func TestIndexedIDsOnABundleWithNoIndexIsEmptyNotAnError(t *testing.T) {
	cfg := config.ConfigFor(t.TempDir())
	got, err := IndexedIDs(cfg)
	if err != nil {
		t.Fatalf("IndexedIDs on an unindexed bundle errored: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected an empty set, got %d ids", len(got))
	}
	if _, err := os.Stat(cfg.IndexPath); err == nil {
		t.Error("IndexedIDs created the search index it was only meant to read")
	}
}

// TestIndexFileClassifiesAGraphyConceptAsCode: a concept exported before
// `kind: code` existed carries no kind key, and its free-form type
// ("code-function") misses GetKind's legacy switch — so it indexed as "note"
// and `okfw search --kind note` returned the whole code export.
func TestIndexFileClassifiesAGraphyConceptAsCode(t *testing.T) {
	root := t.TempDir()
	p := writeConcept(t, root, "alpha--0001.md", `---
id: "019680f0-7e00-7abc-8def-100000000000"
title: "Alpha"
type: "code-function"
tags: []
created: "2026-02-13T11:00:00Z"
timestamp: "2026-02-13T11:00:00Z"
graphy:
  node_id: "go:function:a.go#Alpha"
---

# Alpha
`)
	f := &fakeIndexer{}
	if err := IndexFile(f, p); err != nil {
		t.Fatal(err)
	}
	doc := f.docs["019680f0-7e00-7abc-8def-100000000000"]
	if doc == nil {
		t.Fatal("nothing was indexed")
	}
	if doc.Kind != "code" {
		t.Errorf("kind = %q, want code", doc.Kind)
	}
	if doc.Type != "code-function" {
		t.Errorf("type = %q, want the free-form type preserved", doc.Type)
	}
}
