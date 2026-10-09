package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"okfw/cli/internal/config"
)

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

// TestAutoSyncRefreshesListingsOnly: AutoSync used to write a Bleve document
// and a bbolt node as well. Both are gone, so the one thing it still owes the
// bundle is the OKF index.md listing — and it must not create any other state.
func TestAutoSyncRefreshesListingsOnly(t *testing.T) {
	root := t.TempDir()
	cfg := config.ConfigFor(root)
	p := writeConcept(t, root, "alpha--0001.md", GenerateBasicNote("019680f0-7e00-7abc-8def-100000000000", "Alpha", "", nil))

	if err := AutoSync(cfg, p); err != nil {
		t.Fatalf("AutoSync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "index.md")); err != nil {
		t.Errorf("AutoSync should have written index.md: %v", err)
	}
	for _, gone := range []string{".index", ".graph.db"} {
		if _, err := os.Stat(filepath.Join(root, gone)); err == nil {
			t.Errorf("AutoSync created %s; derived stores were removed", gone)
		}
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

// TestWriteIndexFilesSummarizesLargeDirectories: OKF section 8 gives index.md
// one job — showing what is available *before* opening documents — and a
// listing that names every concept stops doing it exactly when the directory
// is big enough for the question to matter. A 901-concept directory once
// produced a 28,000-token "preview" of concepts averaging 1,200 tokens each.
func TestWriteIndexFilesSummarizesLargeDirectories(t *testing.T) {
	root := t.TempDir()
	cfg := config.ConfigFor(root)
	// Two types, both past the per-type cap, and together past the enumerate
	// limit so the grouped form is chosen.
	for i := 0; i < 40; i++ {
		id := fmt.Sprintf("019680f0-7e00-7abc-8def-1000000000%02d", i)
		writeConcept(t, root, fmt.Sprintf("big/fn%02d--%02d.md", i, i),
			conceptWithType(id, fmt.Sprintf("Func %02d", i), "code-function"))
	}
	for i := 0; i < 30; i++ {
		id := fmt.Sprintf("019680f0-7e00-7abc-8def-2000000000%02d", i)
		writeConcept(t, root, fmt.Sprintf("big/st%02d--%02d.md", i, i),
			conceptWithType(id, fmt.Sprintf("Struct %02d", i), "code-struct"))
	}
	if _, err := WriteIndexFiles(cfg, root); err != nil {
		t.Fatal(err)
	}
	got := readIndex(t, root, "big")

	if !strings.Contains(got, "70 concepts, grouped by type") {
		t.Errorf("no summary header:\n%s", got)
	}
	for _, want := range []string{"### code-function (40)", "### code-struct (30)"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing group heading %q:\n%s", want, got)
		}
	}
	// Withheld rows must be counted and must say how to get them.
	for _, want := range []string{
		"… 30 more — `okfw search --type code-function`",
		"… 20 more — `okfw search --type code-struct`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing withheld notice %q:\n%s", want, got)
		}
	}
	if n := strings.Count(got, "\n* ["); n != 2*indexPerTypeLimit {
		t.Errorf("listed %d concepts, want %d (%d per type)", n, 2*indexPerTypeLimit, indexPerTypeLimit)
	}
	// The point of the exercise: the preview must be cheap.
	if len(got) > 8000 {
		t.Errorf("summarized index is %d bytes; it is meant to be a preview", len(got))
	}
}

// TestWriteIndexFilesEnumeratesSmallDirectories: the summary form must not
// kick in for ordinary hand-written directories, where naming every concept is
// the more useful listing.
func TestWriteIndexFilesEnumeratesSmallDirectories(t *testing.T) {
	root := t.TempDir()
	cfg := config.ConfigFor(root)
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("019680f0-7e00-7abc-8def-3000000000%02d", i)
		writeConcept(t, root, fmt.Sprintf("small/n%02d--%02d.md", i, i),
			conceptWithType(id, fmt.Sprintf("Note %02d", i), "note"))
	}
	if _, err := WriteIndexFiles(cfg, root); err != nil {
		t.Fatal(err)
	}
	got := readIndex(t, root, "small")
	if strings.Contains(got, "grouped by type") {
		t.Errorf("small directory was summarized:\n%s", got)
	}
	for i := 0; i < 5; i++ {
		if want := fmt.Sprintf("Note %02d", i); !strings.Contains(got, want) {
			t.Errorf("missing %q from a full listing:\n%s", want, got)
		}
	}
}

func conceptWithType(id, title, typ string) string {
	return "---\nid: \"" + id + "\"\ntitle: \"" + title + "\"\ntype: \"" + typ + "\"\n" +
		"tags: []\ncreated: \"2026-01-01T00:00:00Z\"\ntimestamp: \"2026-01-01T00:00:00Z\"\n" +
		"---\nbody\n"
}

func readIndex(t *testing.T, root, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(dir), "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// --- ownership: okfw maintains only the listings it generated ---

// TestIndexListingsOkfwDidNotWriteSurvive: OKF lets index.md exist without any
// tool, and a bundle can hold listings of concepts okfw does not manage. A
// single `okfw create` used to replace a hand-written root listing, and delete
// every index.md in a directory holding no okfw concept along with the
// directory itself.
func TestIndexListingsOkfwDidNotWriteSurvive(t *testing.T) {
	root := t.TempDir()
	cfg := config.ConfigFor(root)
	const rootListing = "---\nokf_version: \"1.0\"\n---\n\n# Finance\n\n* [Revenue](computations/revenue.md) - Recognized revenue.\n"
	const subListing = "# Computations\n\n* [Revenue](revenue.md) - Recognized revenue.\n"
	writeConcept(t, root, "index.md", rootListing)
	writeConcept(t, root, "computations/index.md", subListing)
	// A foreign concept: spec-conformant, but carrying no okfw id.
	writeConcept(t, root, "computations/revenue.md", "---\ntype: Attested Computation\n---\n\n# Revenue\n")
	writeConcept(t, root, "mine--0001.md", GenerateBasicNote("019680f0-7e00-7abc-8def-100000000000", "Mine", "", nil))

	for _, run := range []func(*config.Config, string) (int, error){WriteIndexFiles, AdoptIndexFiles} {
		if _, err := run(cfg, root); err != nil {
			t.Fatal(err)
		}
		if got := readIndex(t, root, "computations"); got != subListing {
			t.Fatalf("a hand-written listing in a directory without okfw concepts changed:\n%s", got)
		}
	}
}

func TestOnlyExplicitRegenerationAdoptsUnmarkedListings(t *testing.T) {
	root := t.TempDir()
	cfg := config.ConfigFor(root)
	const legacy = "---\nokf_version: \"0.2\"\n---\n\n# /\n\n## Concepts\n\n* [Old](old.md)\n"
	writeConcept(t, root, "index.md", legacy)
	writeConcept(t, root, "mine--0001.md", GenerateBasicNote("019680f0-7e00-7abc-8def-100000000000", "Mine", "", nil))

	if _, err := WriteIndexFiles(cfg, root); err != nil {
		t.Fatal(err)
	}
	if got := readIndex(t, root, ""); got != legacy {
		t.Fatalf("automatic sync overwrote an unmarked listing:\n%s", got)
	}
	if _, err := AdoptIndexFiles(cfg, root); err != nil {
		t.Fatal(err)
	}
	got := readIndex(t, root, "")
	if !IsGeneratedIndex([]byte(got)) || !strings.Contains(got, "[Mine](mine--0001.md)") {
		t.Fatalf("explicit regeneration did not adopt the listing:\n%s", got)
	}
	// Once adopted, ordinary sync maintains it.
	writeConcept(t, root, "more--0002.md", GenerateBasicNote("019680f0-7e00-7abc-8def-200000000000", "More", "", nil))
	if _, err := WriteIndexFiles(cfg, root); err != nil {
		t.Fatal(err)
	}
	if got := readIndex(t, root, ""); !strings.Contains(got, "[More](more--0002.md)") {
		t.Errorf("adopted listing is not maintained:\n%s", got)
	}
}

// A listing that is already current is not rewritten: every concept write runs
// this, and rewriting every index.md each time churned mtimes and git status.
func TestWriteIndexFilesSkipsCurrentListings(t *testing.T) {
	root := t.TempDir()
	cfg := config.ConfigFor(root)
	writeConcept(t, root, "a/one--0001.md", GenerateBasicNote("019680f0-7e00-7abc-8def-100000000000", "One", "", nil))
	if n, err := WriteIndexFiles(cfg, root); err != nil || n != 2 {
		t.Fatalf("first run wrote %d, %v; want 2", n, err)
	}
	if n, err := WriteIndexFiles(cfg, root); err != nil || n != 0 {
		t.Errorf("second run wrote %d, %v; want 0", n, err)
	}
}

func TestIndexEntriesAreEscaped(t *testing.T) {
	root := t.TempDir()
	cfg := config.ConfigFor(root)
	note := GenerateBasicNote("019680f0-7e00-7abc-8def-100000000000", "Weird ] title [x](evil.md)", "", nil)
	withDesc, err := UpdateFrontmatter([]byte(note), map[string]string{"description": "line one\nline two"})
	if err != nil {
		t.Fatal(err)
	}
	writeConcept(t, root, "my notes (draft).md", string(withDesc))
	if _, err := WriteIndexFiles(cfg, root); err != nil {
		t.Fatal(err)
	}
	got := readIndex(t, root, "")
	want := `* [Weird \] title \[x\](evil.md)](my%20notes%20%28draft%29.md) - line one line two` + "\n"
	if !strings.Contains(got, want) {
		t.Errorf("entry not escaped; want %q in:\n%s", want, got)
	}
}
