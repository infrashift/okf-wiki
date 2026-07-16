package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"okfw/cli/internal/config"
)

// --- helpers ---

func testCfg(root string) *config.Config {
	return config.ConfigFor(root)
}

func conceptContent(id, title, typ, body string) string {
	return "---\n" +
		"id: \"" + id + "\"\n" +
		"title: \"" + title + "\"\n" +
		"type: \"" + typ + "\"\n" +
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
	if first != "alpha--019680f07e00.md" {
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
	writeConceptFile(t, root, "notes/alpha--0001.md", conceptContent("019680f0-7e00-7abc-8def-1234567890ab", "Alpha", "note", "# Alpha"))

	if err := runValidateOKF(cfg, root); err != nil {
		t.Fatalf("conformant bundle should pass, got %v", err)
	}

	// Inject a concept with an empty `type` (hard violation).
	writeConceptFile(t, root, "notes/bad--0002.md", conceptContent("01968109-4700-7def-9abc-ffffffffffff", "Bad", "", "# Bad"))
	if err := runValidateOKF(cfg, root); err == nil {
		t.Fatal("expected a conformance violation for empty type")
	}
}

// --- runIndexMd ---

func TestRunIndexMd(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(root)
	writeConceptFile(t, root, "notes/alpha--0001.md", conceptContent("019680f0-7e00-7abc-8def-1234567890ab", "Alpha", "note", "# Alpha"))
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

// --- runLog (requires git) ---

func TestRunLog(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	cfg := testCfg(root)
	writeConceptFile(t, root, "notes/alpha--0001.md", conceptContent("019680f0-7e00-7abc-8def-1234567890ab", "Alpha", "note", "# Alpha"))

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

// --- placement & id resolution ---

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
	// A slug-named concept in an arbitrary subdirectory.
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

// --- daily notes listing ---

func TestListDailyNotes_SlugFilenames(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(root)

	// Create a daily note the same way createOrOpenDaily does: a
	// slug--shortid.md filename whose date lives in the frontmatter.
	now := time.Now()
	id := newV7WithTime(now)
	title := now.Format("2006-01-02") + " " + now.Format("Monday")
	content := "---\n" +
		"id: \"" + id + "\"\n" +
		"title: \"" + title + "\"\n" +
		"type: \"daily-note\"\n" +
		"kind: \"daily-note\"\n" +
		"tags: []\n" +
		"created: \"" + now.Format(time.RFC3339) + "\"\n" +
		"timestamp: \"" + now.Format(time.RFC3339) + "\"\n" +
		"---\n\n# " + title + "\n"
	rel := "daily/" + now.Format("2006") + "/" + now.Format("01") + "/" +
		slugify(title) + "--" + shortID(id, 8) + ".md"
	writeConceptFile(t, root, rel, content)

	// Capture stdout to check the note is listed.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := listDailyNotes(cfg, false, false, true)
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)

	if err != nil {
		t.Fatalf("listDailyNotes: %v", err)
	}
	if !strings.Contains(string(out), title) {
		t.Errorf("daily --list should include %q, got:\n%s", title, out)
	}
	if strings.Contains(string(out), "[]") {
		t.Errorf("daily --list returned no notes:\n%s", out)
	}
}


// --- init ---

func TestRunInit(t *testing.T) {
	project := t.TempDir()

	if err := runInit(project); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	wikiDir := filepath.Join(project, config.WikiDirName)
	if fi, err := os.Stat(wikiDir); err != nil || !fi.IsDir() {
		t.Fatalf(".okfw directory not created: %v", err)
	}

	gi := readFile(t, filepath.Join(wikiDir, ".gitignore"))
	for _, want := range []string{config.IndexDirName + "/", config.GraphDBName, config.TodosDirName + "/", "ephemeral/"} {
		if !strings.Contains(gi, want) {
			t.Errorf(".gitignore missing %q:\n%s", want, gi)
		}
	}

	rootIdx := readFile(t, filepath.Join(wikiDir, "index.md"))
	if !strings.Contains(rootIdx, "okf_version") {
		t.Error("root index.md should declare okf_version")
	}

	// Second run is a no-op: nothing recreated, contents untouched.
	if err := os.WriteFile(filepath.Join(wikiDir, "keep--0001.md"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runInit(project); err != nil {
		t.Fatalf("second runInit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wikiDir, "keep--0001.md")); err != nil {
		t.Error("existing wiki contents must be untouched by a repeat init")
	}
}

func TestDiscoverAfterInit(t *testing.T) {
	project := t.TempDir()
	if err := runInit(project); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	nested := filepath.Join(project, "cmd", "app")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Discover(nested)
	if err != nil {
		t.Fatalf("Discover after init: %v", err)
	}
	want := filepath.Join(project, config.WikiDirName)
	if cfg.RootPath != want {
		t.Errorf("RootPath = %q, want %q", cfg.RootPath, want)
	}
}

// --- index.md auto-maintenance ---

func TestAutoSyncRefreshesIndexMd(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(root)

	// A note created in a brand-new subdirectory (as `create --path` does).
	p := writeConceptFile(t, root, "projects/deep/alpha--0001.md",
		conceptContent("019680f0-7e00-7abc-8def-1234567890ab", "Alpha", "note", "# Alpha"))

	if err := autoSync(cfg, p); err != nil {
		t.Fatalf("autoSync: %v", err)
	}

	// Root index.md: okf_version declaration + link to the subdirectory chain.
	rootIdx := readFile(t, filepath.Join(root, "index.md"))
	if !strings.Contains(rootIdx, "okf_version") {
		t.Error("root index.md should declare okf_version")
	}
	if !strings.Contains(rootIdx, "[projects/](projects/)") {
		t.Errorf("root index.md should link the projects/ subdirectory:\n%s", rootIdx)
	}

	// Intermediate directory: no frontmatter, links its child dir.
	midIdx := readFile(t, filepath.Join(root, "projects", "index.md"))
	if strings.HasPrefix(midIdx, "---") {
		t.Error("non-root index.md must not carry frontmatter")
	}
	if !strings.Contains(midIdx, "[deep/](deep/)") {
		t.Errorf("projects/index.md should link deep/:\n%s", midIdx)
	}

	// Leaf directory: lists the concept by title.
	leafIdx := readFile(t, filepath.Join(root, "projects", "deep", "index.md"))
	if !strings.Contains(leafIdx, "[Alpha](alpha--0001.md)") {
		t.Errorf("leaf index.md should list the concept:\n%s", leafIdx)
	}
}
