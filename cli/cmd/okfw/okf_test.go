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

	if err := runIndexMd(cfg, root, true); err != nil {
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

// TestRunLog: log.md covers the bundle's history only, links resolve from the
// bundle root, and dates run newest first (OKF section 9). It used to log the
// whole repository under links like /.okfw/x.md, in commit order.
func TestRunLog(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	project := t.TempDir()
	root := filepath.Join(project, ".okfw")
	cfg := testCfg(root)
	git := func(date string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", project,
			"-c", "user.email=t@t", "-c", "user.name=t", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+date+"T12:00:00Z", "GIT_COMMITTER_DATE="+date+"T12:00:00Z")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("2026-01-01", "init", "-q")
	writeConceptFile(t, project, "README.md", "# Project\n")
	writeConceptFile(t, root, "notes/alpha--0001.md", conceptContent("019680f0-7e00-7abc-8def-1234567890ab", "Alpha", "note", "# Alpha"))
	writeConceptFile(t, root, "beta--0002.md", conceptContent("01968109-4700-7def-9abc-ffffffffffff", "Beta", "note", "# Beta"))
	git("2026-02-01", "add", "-A")
	git("2026-02-01", "commit", "-q", "-m", "Add Alpha and [Beta]")
	// Committed later but authored earlier: commit order is not date order.
	writeConceptFile(t, root, "notes/alpha--0001.md", conceptContent("019680f0-7e00-7abc-8def-1234567890ab", "Alpha", "note", "# Alpha v2"))
	git("2026-01-15", "commit", "-q", "-am", "Revise Alpha")
	if err := os.Remove(filepath.Join(root, "beta--0002.md")); err != nil {
		t.Fatal(err)
	}
	git("2026-03-01", "commit", "-q", "-am", "Drop Beta")

	if err := runLog(cfg, root); err != nil {
		t.Fatalf("runLog: %v", err)
	}
	logmd := readFile(t, filepath.Join(root, "log.md"))
	if v := bundle.CheckLogFile([]byte(logmd)); len(v) > 0 {
		t.Errorf("log.md breaks OKF section 9: %v\n%s", v, logmd)
	}
	march, feb, jan := strings.Index(logmd, "## 2026-03-01"), strings.Index(logmd, "## 2026-02-01"), strings.Index(logmd, "## 2026-01-15")
	if march < 0 || feb < march || jan < feb {
		t.Errorf("dates are not newest first:\n%s", logmd)
	}
	for _, want := range []string{
		"* **Creation**: [notes/alpha--0001.md](/notes/alpha--0001.md) — Add Alpha and \\[Beta\\]",
		"* **Update**: [notes/alpha--0001.md](/notes/alpha--0001.md) — Revise Alpha",
		"* **Removal**: `/beta--0002.md` — Drop Beta",
	} {
		if !strings.Contains(logmd, want) {
			t.Errorf("missing %q in:\n%s", want, logmd)
		}
	}
	if strings.Contains(logmd, "README") || strings.Contains(logmd, ".okfw") {
		t.Errorf("log.md reaches outside the bundle:\n%s", logmd)
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

// TestDailyNotesAreOnePerDay: createOrOpenDaily used to return the first *.md
// in the month directory, so every later day of the month reopened the first
// day's note — or the directory's index.md.
func TestDailyNotesAreOnePerDay(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(root)
	paths := map[string]string{}
	for _, d := range []string{"2026-03-01", "2026-03-02", "2026-03-15"} {
		date, _ := time.Parse("2006-01-02", d)
		p, created, err := createOrOpenDaily(cfg, date)
		if err != nil || !created {
			t.Fatalf("%s: created=%v err=%v", d, created, err)
		}
		if !strings.Contains(readFile(t, p), "title: \""+d+" ") {
			t.Errorf("%s: note at %s is for another day", d, p)
		}
		paths[d] = p
	}
	if len(map[string]bool{paths["2026-03-01"]: true, paths["2026-03-02"]: true, paths["2026-03-15"]: true}) != 3 {
		t.Fatalf("days share a note: %v", paths)
	}
	date, _ := time.Parse("2006-01-02", "2026-03-02")
	again, created, err := createOrOpenDaily(cfg, date)
	if err != nil || created || again != paths["2026-03-02"] {
		t.Errorf("reopening 2026-03-02 gave %s (created=%v, err=%v), want %s", again, created, err, paths["2026-03-02"])
	}
}

// A backdated daily note lists under the day it is for, not the day it was
// written: the listing used to date notes by `created`.
func TestBackdatedDailyNoteListsUnderItsOwnDate(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(root)
	day := time.Now().AddDate(0, 0, -3)
	date, _ := time.Parse("2006-01-02", day.Format("2006-01-02"))
	if _, _, err := createOrOpenDaily(cfg, date); err != nil {
		t.Fatal(err)
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := listDailyNotes(cfg, false, false, true)
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if want := `"date": "` + day.Format("2006-01-02") + `"`; !strings.Contains(string(out), want) {
		t.Errorf("listing should carry %s:\n%s", want, out)
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

// Two valid concepts sharing an id pass every per-file check; only the bundle
// view can see them, and lookup by that id now refuses, so --all must say which
// files they are.
func TestRunValidateAllReportsADuplicatedID(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(root)
	const id = "019680f0-7e00-7abc-8def-1234567890ab"
	note := bundle.GenerateBasicNote(id, "Fine", "", nil)
	writeConceptFile(t, root, "fine--0001.md", note)
	writeConceptFile(t, root, "sub/copy-of-fine--0001.md", note)
	writeConceptFile(t, root, "other--0002.md",
		bundle.GenerateBasicNote("019680f0-7e00-7abc-8def-0000000000ff", "Other", "", nil))

	out, err := captureStdout(t, func() error { return runValidateAll(cfg, root) })
	if err == nil || !strings.Contains(err.Error(), "1 id(s) are declared by more than one concept") {
		t.Errorf("err = %v, want the duplicate reported as a failure", err)
	}
	if !strings.Contains(out, "id "+id+" is declared by 2 concepts: fine--0001.md, sub/copy-of-fine--0001.md") {
		t.Errorf("output does not name both copies:\n%s", out)
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

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := fn()
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out), err
}

// TestValidateOKFOnABundleOkfwDidNotWrite: §11 asks for parseable frontmatter
// with a non-empty type, plus reserved files shaped per §8 and §9 — nothing
// about okfw's own fields. A nested directory named ephemeral is part of the
// bundle; only the root's is okfw's scratch area.
func TestValidateOKFOnABundleOkfwDidNotWrite(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(root)
	writeConceptFile(t, root, "index.md", "---\nokf_version: \"0.2\"\n---\n\n# Finance\n\n* [Revenue](computations/revenue.md) - Revenue.\n")
	writeConceptFile(t, root, "computations/revenue.md", "---\ntype: Attested Computation\ntags: finance\n---\n# Revenue\n\nSee [profit](/computations/profit.md \"P\").\n")
	writeConceptFile(t, root, "computations/profit.md", "---\ntype: Attested Computation\n---\n# Profit\n")
	writeConceptFile(t, root, "notes/ephemeral/scratch.md", "---\ntype: Note\n---\n# Scratch\n")
	writeConceptFile(t, root, "log.md", "# Log\n\n## 2026-05-22\n* **Update**: revenue\n\n## 2026-05-15\n* **Creation**: profit\n")

	out, err := captureStdout(t, func() error { return runValidateOKF(cfg, root) })
	if err != nil {
		t.Fatalf("conformant bundle rejected: %v\n%s", err, out)
	}
	if !strings.Contains(out, "3 concept(s), 0 violation(s), 0 broken link(s), 0 warning(s)") {
		t.Errorf("summary:\n%s", out)
	}

	// Reserved files that break their structure are violations.
	writeConceptFile(t, root, "computations/index.md", "---\nokf_version: \"0.2\"\n---\n# Computations\n")
	writeConceptFile(t, root, "log.md", "## 2026-02-01\n* a\n\n## 2026-03-01\n* b\n")
	out, err = captureStdout(t, func() error { return runValidateOKF(cfg, root) })
	if err == nil || !strings.Contains(out, "computations/index.md: frontmatter is only permitted") ||
		!strings.Contains(out, "log.md: line 4: 2026-03-01 follows 2026-02-01") {
		t.Errorf("err = %v, output:\n%s", err, out)
	}
}

func TestValidateOKFReportsAnotherVersionWithoutFailing(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(root)
	writeConceptFile(t, root, "index.md", "---\nokf_version: \"1.0\"\n---\n\n# /\n")
	writeConceptFile(t, root, "a.md", "---\ntype: Note\n---\n")
	out, err := captureStdout(t, func() error { return runValidateOKF(cfg, root) })
	if err != nil || !strings.Contains(out, "declares OKF 1.0") {
		t.Errorf("err = %v, output:\n%s", err, out)
	}
}

// TestValidateOKFWarnsOnABrokenFrontmatterPath: a path-valued field naming a
// concept is a link (section 6.2), so a broken one is reported like a broken
// body link, naming the field. A URL, a scope descriptor or a non-markdown
// file is not a concept link and is not checked.
func TestValidateOKFWarnsOnABrokenFrontmatterPath(t *testing.T) {
	root := t.TempDir()
	cfg := testCfg(root)
	writeConceptFile(t, root, "tables/orders.md", "---\ntype: Table\n---\n")
	writeConceptFile(t, root, "report.md", "---\ntype: Report\n"+
		"resource: /tables/orders.md\n"+
		"sources:\n  - resource: https://example.com/x.md\n  - resource: all queries in project X\n"+
		"  - resource: ./notes/gone.md\n"+
		"attester:\n  resource: references/check.py\n---\n")
	out, err := captureStdout(t, func() error { return runValidateOKF(cfg, root) })
	if err != nil {
		t.Fatalf("a broken link is a warning, not a violation: %v\n%s", err, out)
	}
	if !strings.Contains(out, "sources[2].resource target not found: ./notes/gone.md") {
		t.Errorf("missing the broken source warning:\n%s", out)
	}
	if n := strings.Count(out, "target not found"); n != 1 {
		t.Errorf("%d broken-link warnings, want 1:\n%s", n, out)
	}
}
