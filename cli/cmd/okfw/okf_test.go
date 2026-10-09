package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"okfw/cli/internal/bundle"
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

// --- daily notes listing ---

func TestListDailyNotes_SlugFilenames(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(root)

	// Create a daily note the same way createOrOpenDaily does: a
	// slug--shortid.md filename whose date lives in the frontmatter.
	now := time.Now()
	id := bundle.NewV7WithTime(now)
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
		bundle.Slugify(title) + "--" + bundle.ShortID(id, 8) + ".md"
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
	if !strings.Contains(gi, "ephemeral/") {
		t.Errorf(".gitignore missing %q:\n%s", "ephemeral/", gi)
	}
	// The derived stores are gone; init must not resurrect them by habit.
	for _, gone := range []string{".index", ".graph.db"} {
		if strings.Contains(gi, gone) {
			t.Errorf(".gitignore still names the removed %q:\n%s", gone, gi)
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

	if err := bundle.AutoSync(cfg, p); err != nil {
		t.Fatalf("bundle.AutoSync: %v", err)
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

// --- runValidateAll (strict) vs runValidateOKF (permissive) ---

func TestRunValidateAllIsStricterThanOKFConformance(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(root)

	// Conformant per OKF section 11 (parseable frontmatter, non-empty type) but
	// missing okfw's required id/tags/created/timestamp.
	writeConceptFile(t, root, "sparse--0001.md", "---\ntype: \"Reference\"\ntitle: \"Sparse\"\n---\n\n# Sparse\n")

	if err := runValidateOKF(cfg, root); err != nil {
		t.Errorf("--okf should accept a spec-conformant concept: %v", err)
	}
	if err := runValidateAll(cfg, root); err == nil {
		t.Error("--all should reject a concept missing okfw's required fields")
	}
}

func TestRunValidateAllAcceptsAConformantBundle(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(root)
	writeConceptFile(t, root, "ok--0001.md",
		bundle.GenerateBasicNote("019680f0-7e00-7abc-8def-1234567890ab", "Fine", "", nil))

	if err := runValidateAll(cfg, root); err != nil {
		t.Errorf("a valid bundle should pass --all: %v", err)
	}
}

// --- runMigrate ---

const legacyTodoConcept = `---
id: "019680f0-7e00-7abc-8def-1234567890ab"
title: "Ship it"
type: "todo"
kind: "todo"
status: "closed"
due: "2026-08-01"
completed: "2026-07-20"
priority: "high"
tags: []
created: "2026-07-01T10:00:00Z"
timestamp: "2026-07-20T10:00:00Z"
---

# Ship it
`

func TestRunMigrateStripsLegacyTodoFields(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(root)
	p := writeConceptFile(t, root, "ship-it--0001.md", legacyTodoConcept)

	// The bundle does not validate before migrating.
	if err := runValidateAll(cfg, root); err == nil {
		t.Fatal("a legacy todo should fail the current schema")
	}

	if err := runMigrate(cfg, root, false); err != nil {
		t.Fatalf("runMigrate: %v", err)
	}

	got := readFile(t, p)
	if !strings.Contains(got, `status: "deprecated"`) {
		t.Errorf("closed should become deprecated:\n%s", got)
	}
	for _, gone := range []string{"due:", "completed:", "priority:", "kind:"} {
		if strings.Contains(got, gone) {
			t.Errorf("%s should be gone:\n%s", gone, got)
		}
	}
	if err := runValidateAll(cfg, root); err != nil {
		t.Errorf("the bundle should validate after migrating: %v", err)
	}
}

func TestRunMigrateDryRunWritesNothing(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(root)
	p := writeConceptFile(t, root, "ship-it--0001.md", legacyTodoConcept)

	if err := runMigrate(cfg, root, true); err != nil {
		t.Fatalf("runMigrate --dry-run: %v", err)
	}
	if got := readFile(t, p); got != legacyTodoConcept {
		t.Errorf("--dry-run must not modify the file:\n%s", got)
	}
}

func TestRunMigrateIsIdempotent(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(root)
	p := writeConceptFile(t, root, "ship-it--0001.md", legacyTodoConcept)

	if err := runMigrate(cfg, root, false); err != nil {
		t.Fatal(err)
	}
	once := readFile(t, p)
	if err := runMigrate(cfg, root, false); err != nil {
		t.Fatal(err)
	}
	if twice := readFile(t, p); twice != once {
		t.Errorf("a second migrate changed the file:\n%s\n---\n%s", once, twice)
	}
}

func TestRunMigrateLeavesCleanConceptsAlone(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(root)
	p := writeConceptFile(t, root, "clean--0001.md",
		bundle.GenerateBasicNote("019680f0-7e00-7abc-8def-1234567890ab", "Clean", "", nil))
	before := readFile(t, p)

	if err := runMigrate(cfg, root, false); err != nil {
		t.Fatalf("runMigrate: %v", err)
	}
	if after := readFile(t, p); after != before {
		t.Errorf("a clean concept must not be rewritten:\n%s", after)
	}
}

// legacyDatedConcept is what okfw wrote before OKF required datetimes: bare
// dates in every field that is now an instant, quoted and unquoted.
const legacyDatedConcept = `---
id: "019680f0-7e00-7abc-8def-1234567890ab"
title: "Pricing"
type: "note"
tags: []
stale_after: "2026-12-31"
usage_window:
  from: 2026-01-01
  to: "2026-06-30"
sources:
  - resource: "https://example.com/schema"
    last_modified: "2026-05-01"
    usage_window: {from: "2026-02-01", to: "2026-03-01"}
created: "2026-02-13T10:45:00Z"
timestamp: "2026-02-13T10:45:00Z"
---

# Pricing
`

func TestRunMigrateUpgradesDatesToDatetimes(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(root)
	p := writeConceptFile(t, root, "pricing--0001.md", legacyDatedConcept)

	if err := runValidateAll(cfg, root); err == nil {
		t.Fatal("bare dates should fail the current schema")
	}
	if err := runMigrate(cfg, root, false); err != nil {
		t.Fatalf("runMigrate: %v", err)
	}
	got := readFile(t, p)
	z, err := config.ParseAndValidate([]byte(got))
	if err != nil {
		t.Fatalf("migrated concept does not validate: %v\n%s", err, got)
	}
	if z.StaleAfter != "2026-12-31T00:00:00Z" ||
		z.UsageWindow.From != "2026-01-01T00:00:00Z" || z.UsageWindow.To != "2026-06-30T00:00:00Z" ||
		z.Sources[0].LastModified != "2026-05-01T00:00:00Z" ||
		z.Sources[0].UsageWindow.From != "2026-02-01T00:00:00Z" || z.Sources[0].UsageWindow.To != "2026-03-01T00:00:00Z" {
		t.Errorf("not every date was upgraded:\n%s", got)
	}
	// Reformatting a field is not a change to content.
	if !z.Timestamp.Equal(time.Date(2026, 2, 13, 10, 45, 0, 0, time.UTC)) {
		t.Errorf("timestamp moved to %s", z.Timestamp)
	}
	if !strings.HasSuffix(got, "\n# Pricing\n") {
		t.Errorf("body changed:\n%s", got)
	}

	once := got
	if err := runMigrate(cfg, root, false); err != nil {
		t.Fatal(err)
	}
	if twice := readFile(t, p); twice != once {
		t.Errorf("a second migrate changed the file:\n%s\n---\n%s", once, twice)
	}
}

// A datetime with no offset names an instant only its author knows; migrate
// must neither guess nor fail the whole run over it.
func TestRunMigrateLeavesUnrepairableTimestampsAlone(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(root)
	content := strings.Replace(legacyDatedConcept, `stale_after: "2026-12-31"`, `stale_after: "2026-12-31T00:00:00"`, 1)
	p := writeConceptFile(t, root, "pricing--0001.md", content)

	if err := runMigrate(cfg, root, false); err != nil {
		t.Fatalf("runMigrate: %v", err)
	}
	got := readFile(t, p)
	if !strings.Contains(got, `stale_after: "2026-12-31T00:00:00"`) {
		t.Errorf("an offset-less datetime was rewritten:\n%s", got)
	}
	if !strings.Contains(got, `last_modified: "2026-05-01T00:00:00Z"`) {
		t.Errorf("the repairable fields beside it were not upgraded:\n%s", got)
	}
	bad, err := bundle.TimestampsNeedingRepair([]byte(got))
	if err != nil || len(bad) != 1 || bad[0] != "stale_after" {
		t.Errorf("TimestampsNeedingRepair = %v, %v; want [stale_after]", bad, err)
	}
}
