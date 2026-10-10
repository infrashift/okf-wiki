package bundle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"okfw/cli/internal/config"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"My OKF Project Idea!": "my-okf-project-idea",
		"  Trim   Me  ":        "trim-me",
		"a/b/c":                "a-b-c",
		"under_score and-dash": "under-score-and-dash",
		"":                     "note",
		"index":                "index-note",
		"log":                  "log-note",
		"2026-06-26 Friday":    "2026-06-26-friday",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

// Non-ASCII titles have no transliteration, so they collapse to the "note"
// fallback. Pinning this documents that filenames for such titles carry no
// information beyond the short id — the frontmatter title is what survives.
func TestSlugifyDropsNonASCII(t *testing.T) {
	for _, in := range []string{"日本語", "Ελληνικά", "Привет"} {
		if got := Slugify(in); got != "note" {
			t.Errorf("Slugify(%q) = %q, want the \"note\" fallback", in, got)
		}
	}
	// Accented letters are dropped outright rather than transliterated, so
	// neighbouring fragments run together: "Café résumé" loses each "é".
	if got := Slugify("Café résumé"); got != "caf-rsum" {
		t.Errorf("Slugify(%q) = %q", "Café résumé", got)
	}
}

func TestShortID(t *testing.T) {
	id := "019680f0-7e00-7abc-8def-1234567890ab"
	if got := ShortID(id, 8); got != "019680f0" {
		t.Errorf("ShortID 8 = %q", got)
	}
	if got := ShortID(id, 12); got != "019680f07e00" {
		t.Errorf("ShortID 12 = %q", got)
	}
	// Asking for more than the id holds must clamp, not panic.
	if got := ShortID(id, 999); len(got) != 32 {
		t.Errorf("ShortID(999) = %q, want the full 32 hex chars", got)
	}
}

func TestNoteFilenameCollision(t *testing.T) {
	dir := t.TempDir()
	id := "019680f0-7e00-7abc-8def-1234567890ab"
	first := NoteFilename(dir, "Alpha", id)
	if first != "alpha--019680f07e00.md" {
		t.Fatalf("first filename = %q", first)
	}
	if err := os.WriteFile(filepath.Join(dir, first), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	second := NoteFilename(dir, "Alpha", id)
	if second == first {
		t.Fatalf("expected a different filename on collision, got %q", second)
	}
	if !strings.HasPrefix(second, "alpha--019680f0") {
		t.Fatalf("collision filename should extend the id, got %q", second)
	}
}

func TestSanitizeSubdir(t *testing.T) {
	ok := map[string]string{
		"projects/okf": "projects/okf",
		"/projects/x":  "projects/x",
		"a/./b":        "a/b",
	}
	for in, want := range ok {
		if got := SanitizeSubdir(in); got != want {
			t.Errorf("SanitizeSubdir(%q) = %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{"", ".", "..", "../etc", "../../x"} {
		if got := SanitizeSubdir(bad); got != "" {
			t.Errorf("SanitizeSubdir(%q) should be rejected, got %q", bad, got)
		}
	}
}

// A concept written into one of the bundle's own dot-directories is invisible to
// every walk (SkipWalkDir skips dot-dirs), so it never reaches search or
// index.md. Escaping the bundle was already rejected; landing inside its
// internals was not.
func TestSanitizeSubdirRejectsDotDirectories(t *testing.T) {
	for _, bad := range []string{
		".index",
		".todos",
		".graph.db",
		".index/nested",
		"/.index",
		"./.index",
		".hidden/sub",
	} {
		if got := SanitizeSubdir(bad); got != "" {
			t.Errorf("SanitizeSubdir(%q) should be rejected, got %q", bad, got)
		}
	}
	// A dot deeper in the path is just as invisible to the walk.
	for _, bad := range []string{"notes/.hidden", "a/b/.c"} {
		if got := SanitizeSubdir(bad); got != "" {
			t.Errorf("SanitizeSubdir(%q) should be rejected, got %q", bad, got)
		}
	}
	// Dots inside a segment name are fine.
	if got := SanitizeSubdir("v1.2/notes"); got != "v1.2/notes" {
		t.Errorf("SanitizeSubdir(%q) = %q", "v1.2/notes", got)
	}
}

func TestIsReservedBase(t *testing.T) {
	if !IsReservedBase("index.md") || !IsReservedBase("log.md") {
		t.Error("index.md/log.md should be reserved")
	}
	if IsReservedBase("note--abcd.md") {
		t.Error("regular concept should not be reserved")
	}
}

func TestConceptDir(t *testing.T) {
	root := t.TempDir()
	cfg := config.ConfigFor(root)

	if got, err := ConceptDir(cfg, "", ""); err != nil || got != root {
		t.Errorf("ConceptDir(no path, no project) = %q, %v; want %q", got, err, root)
	}
	// An explicit --path wins over the project name.
	want := filepath.Join(root, "custom", "here")
	if got, err := ConceptDir(cfg, "custom/here", "proj"); err != nil || got != want {
		t.Errorf("ConceptDir(--path) = %q, %v; want %q", got, err, want)
	}
	// Otherwise the project name becomes a top-level folder.
	want = filepath.Join(root, "proj")
	if got, err := ConceptDir(cfg, "", "proj"); err != nil || got != want {
		t.Errorf("ConceptDir(project) = %q, %v; want %q", got, err, want)
	}
	if _, err := ConceptDir(cfg, "../escape", ""); err == nil {
		t.Error("ConceptDir should reject a --path that escapes the bundle")
	}
	if _, err := ConceptDir(cfg, ".index", ""); err == nil {
		t.Error("ConceptDir should reject a --path inside the bundle's internals")
	}
	// A project name that sanitizes away falls back to the root rather than erroring.
	if got, err := ConceptDir(cfg, "", ".index"); err != nil || got != root {
		t.Errorf("ConceptDir(bad project) = %q, %v; want the root %q", got, err, root)
	}
}

func TestRelDir(t *testing.T) {
	cases := map[string]string{
		"a/b/c.md": "a/b",
		"a/b.md":   "a",
		"top.md":   "",
	}
	for in, want := range cases {
		if got := RelDir(in); got != want {
			t.Errorf("RelDir(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSkipWalkDir(t *testing.T) {
	root := "/bundle"
	if SkipWalkDir(root, root, ".okfw") {
		t.Error("the root itself must never be skipped, even though it is a dot-dir")
	}
	for _, name := range []string{".index", ".todos", "ephemeral", ".git"} {
		if !SkipWalkDir(root, filepath.Join(root, name), name) {
			t.Errorf("SkipWalkDir should skip %q", name)
		}
	}
	if SkipWalkDir(root, filepath.Join(root, "notes"), "notes") {
		t.Error("SkipWalkDir should not skip a normal directory")
	}
	if SkipWalkDir(root, filepath.Join(root, "foo", "ephemeral"), "ephemeral") {
		t.Error("only the root's ephemeral/ is okfw's; a nested one is part of the bundle")
	}
}
