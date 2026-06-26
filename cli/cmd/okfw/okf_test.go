package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"okfw/cli/internal/config"
)

// --- helpers ---

func testCfg(root string) *config.Config {
	return &config.Config{
		RootPath:  root,
		IndexPath: filepath.Join(root, ".okfw_index"),
		GraphPath: ".okfw_graph.db",
		TodosPath: ".okfw_todos",
		Folders:   config.Folders{Untethered: "untethered", Tethered: "tethered", Tmp: "tmp"},
	}
}

func conceptContent(id, title, typ, body string) string {
	return "---\n" +
		"id: \"" + id + "\"\n" +
		"title: \"" + title + "\"\n" +
		"type: \"" + typ + "\"\n" +
		"category: \"untethered\"\n" +
		"tags: []\n" +
		"created: \"2026-02-13T10:45:00Z\"\n" +
		"timestamp: \"2026-02-13T10:45:00Z\"\n" +
		"---\n\n" + body + "\n"
}

func writeConceptFile(t *testing.T, root, rel, content string) string {
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

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func glob1(t *testing.T, pattern string) string {
	t.Helper()
	m, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 1 {
		t.Fatalf("expected exactly 1 match for %q, got %d: %v", pattern, len(m), m)
	}
	return m[0]
}

// --- slugify / shortID / noteFilename ---

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
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShortID(t *testing.T) {
	id := "019680f0-7e00-7abc-8def-1234567890ab"
	if got := shortID(id, 8); got != "019680f0" {
		t.Errorf("shortID 8 = %q", got)
	}
	if got := shortID(id, 12); got != "019680f07e00" {
		t.Errorf("shortID 12 = %q", got)
	}
}

func TestNoteFilename_Collision(t *testing.T) {
	dir := t.TempDir()
	id := "019680f0-7e00-7abc-8def-1234567890ab"
	first := noteFilename(dir, "Alpha", id)
	if first != "alpha--019680f0.md" {
		t.Fatalf("first filename = %q", first)
	}
	// Create the file; a second call with the same slug+id must extend the short id.
	if err := os.WriteFile(filepath.Join(dir, first), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	second := noteFilename(dir, "Alpha", id)
	if second == first {
		t.Fatalf("expected a different filename on collision, got %q", second)
	}
	if !strings.HasPrefix(second, "alpha--019680f0") {
		t.Fatalf("collision filename should extend the id, got %q", second)
	}
}

// --- sanitizeSubdir ---

func TestSanitizeSubdir(t *testing.T) {
	ok := map[string]string{
		"projects/okf": "projects/okf",
		"/projects/x":  "projects/x",
		"a/./b":        "a/b",
	}
	for in, want := range ok {
		if got := sanitizeSubdir(in); got != want {
			t.Errorf("sanitizeSubdir(%q) = %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{"", ".", "..", "../etc", "../../x"} {
		if got := sanitizeSubdir(bad); got != "" {
			t.Errorf("sanitizeSubdir(%q) should be rejected, got %q", bad, got)
		}
	}
}

func TestIsReservedBase(t *testing.T) {
	if !isReservedBase("index.md") || !isReservedBase("log.md") {
		t.Error("index.md/log.md should be reserved")
	}
	if isReservedBase("note--abcd.md") {
		t.Error("regular concept should not be reserved")
	}
}

func TestTouchTimestamp(t *testing.T) {
	m := touchTimestamp(nil)
	if m["timestamp"] == "" {
		t.Error("touchTimestamp should set a timestamp")
	}
	pre := map[string]string{"timestamp": "preset"}
	if got := touchTimestamp(pre); got["timestamp"] != "preset" {
		t.Error("touchTimestamp should not overwrite an explicit timestamp")
	}
}

// --- runValidateOKF ---

func TestRunValidateOKF(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(root)
	writeConceptFile(t, root, "untethered/alpha--0001.md", conceptContent("019680f0-7e00-7abc-8def-1234567890ab", "Alpha", "note", "# Alpha"))

	if err := runValidateOKF(cfg, root); err != nil {
		t.Fatalf("conformant bundle should pass, got %v", err)
	}

	// Inject a concept with an empty `type` (hard violation).
	writeConceptFile(t, root, "untethered/bad--0002.md", conceptContent("01968109-4700-7def-9abc-ffffffffffff", "Bad", "", "# Bad"))
	if err := runValidateOKF(cfg, root); err == nil {
		t.Fatal("expected a conformance violation for empty type")
	}
}

// --- runIndexMd ---

func TestRunIndexMd(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(root)
	writeConceptFile(t, root, "untethered/alpha--0001.md", conceptContent("019680f0-7e00-7abc-8def-1234567890ab", "Alpha", "note", "# Alpha"))
	writeConceptFile(t, root, "projects/okf/beta--0002.md", conceptContent("01968109-4700-7def-9abc-ffffffffffff", "Beta", "note", "# Beta"))

	if err := runIndexMd(cfg, root); err != nil {
		t.Fatalf("runIndexMd: %v", err)
	}

	rootIdx := readFile(t, filepath.Join(root, "index.md"))
	if !strings.Contains(rootIdx, "okf_version") {
		t.Error("root index.md should declare okf_version")
	}
	leafIdx := readFile(t, filepath.Join(root, "projects", "okf", "index.md"))
	if strings.Contains(leafIdx, "okf_version") || strings.HasPrefix(leafIdx, "---") {
		t.Error("non-root index.md must not carry frontmatter")
	}
	if !strings.Contains(leafIdx, "Beta") {
		t.Error("leaf index.md should list its concept")
	}
}

// --- runMigrate ---

func TestRunMigrate(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(root)
	id1 := "019680f0-7e00-7abc-8def-1234567890ab"
	id2 := "01968109-4700-7def-9abc-ffffffffffff"
	writeConceptFile(t, root, "untethered/"+id1+".md", conceptContent(id1, "Alpha", "note", "Link to [["+id2+"]] and [["+id2+"|Custom]]."))
	writeConceptFile(t, root, "untethered/"+id2+".md", conceptContent(id2, "Beta", "note", "# Beta"))

	if err := runMigrate(cfg, root, false); err != nil {
		t.Fatalf("runMigrate: %v", err)
	}

	// Files renamed to slug--shortid.md; the old UUID names are gone.
	if _, err := os.Stat(filepath.Join(root, "untethered", id1+".md")); !os.IsNotExist(err) {
		t.Error("legacy UUID filename should be gone")
	}
	alpha := glob1(t, filepath.Join(root, "untethered", "alpha--*.md"))
	content := readFile(t, alpha)
	if strings.Contains(content, "[[") {
		t.Error("wiki links should be converted")
	}
	if !strings.Contains(content, "](/untethered/beta--") {
		t.Errorf("expected a markdown link to beta, got:\n%s", content)
	}
	if !strings.Contains(content, "[Custom](/untethered/beta--") {
		t.Error("titled wiki link should preserve its display text")
	}

	// Idempotent: a second run renames/rewrites nothing and still succeeds.
	if err := runMigrate(cfg, root, false); err != nil {
		t.Fatalf("second runMigrate: %v", err)
	}
	if readFile(t, alpha) != content {
		t.Error("migrate should be idempotent")
	}
}

// --- runLog (requires git) ---

func TestRunLog(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	cfg := testCfg(root)
	writeConceptFile(t, root, "untethered/alpha--0001.md", conceptContent("019680f0-7e00-7abc-8def-1234567890ab", "Alpha", "note", "# Alpha"))

	for _, args := range [][]string{
		{"init"},
		{"-c", "user.email=t@t", "-c", "user.name=t", "add", "-A"},
		{"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-m", "Add Alpha concept"},
	} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	if err := runLog(cfg, root); err != nil {
		t.Fatalf("runLog: %v", err)
	}
	logmd := readFile(t, filepath.Join(root, "log.md"))
	if !strings.Contains(logmd, "## 2026-") && !strings.Contains(logmd, "## 20") {
		t.Errorf("log.md should have a date heading:\n%s", logmd)
	}
	if !strings.Contains(logmd, "**Creation**") {
		t.Errorf("log.md should mark the addition as a Creation:\n%s", logmd)
	}
	if !strings.Contains(logmd, "Add Alpha concept") {
		t.Error("log.md should include the commit subject")
	}
}

// --- placement & id resolution (no untethered/tethered enforcement) ---

func TestConceptDir(t *testing.T) {
	cfg := testCfg("/bundle")

	// --path wins.
	if got, _ := conceptDir(cfg, "projects/okf", "infra"); got != filepath.FromSlash("/bundle/projects/okf") {
		t.Errorf("--path: got %q", got)
	}
	// else project folder.
	if got, _ := conceptDir(cfg, "", "infra"); got != filepath.FromSlash("/bundle/infra") {
		t.Errorf("project: got %q", got)
	}
	// else bundle root.
	if got, _ := conceptDir(cfg, "", ""); got != "/bundle" {
		t.Errorf("root: got %q", got)
	}
	// traversal in --path is rejected.
	if _, err := conceptDir(cfg, "../../etc", ""); err == nil {
		t.Error("expected --path traversal to be rejected")
	}
}

func TestResolveConceptPath_ByID_AnywhereInBundle(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(root)
	id := "019680f0-7e00-7abc-8def-1234567890ab"
	// A slug-named concept in an arbitrary subdirectory (not untethered/tethered).
	want := writeConceptFile(t, root, "infra/some-project-note--019680f0.md", conceptContent(id, "Some Project Note", "note", "# x"))

	got, err := resolveConceptPath(cfg, id)
	if err != nil {
		t.Fatalf("resolveConceptPath by id: %v", err)
	}
	if got != want {
		t.Errorf("resolved %q, want %q", got, want)
	}

	if _, err := resolveConceptPath(cfg, "ffffffff-ffff-7fff-8fff-ffffffffffff"); err == nil {
		t.Error("expected not-found error for unknown id")
	}
}
