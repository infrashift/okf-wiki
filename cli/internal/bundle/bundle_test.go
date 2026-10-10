package bundle

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"okfw/cli/internal/config"
)

func testCfg(t *testing.T) (*config.Config, string) {
	t.Helper()
	root := t.TempDir()
	return config.ConfigFor(root), root
}

// --- GenerateBasicNote ---

func TestGenerateBasicNoteIsValid(t *testing.T) {
	const id = "019680f0-7e00-7abc-8def-1234567890ab"
	out := GenerateBasicNote(id, "My Note", "proj", []string{"a", "b"})

	if _, err := config.ParseAndValidate([]byte(out)); err != nil {
		t.Fatalf("generated note does not satisfy the concept schema: %v\n%s", err, out)
	}
	for _, want := range []string{
		`id: "` + id + `"`,
		`title: "My Note"`,
		`type: "note"`,
		`project: "proj"`,
		`- "a"`,
		"# My Note",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestGenerateBasicNoteWithoutProjectOrTags(t *testing.T) {
	out := GenerateBasicNote("019680f0-7e00-7abc-8def-1234567890ab", "Bare", "", nil)
	if strings.Contains(out, "project:") {
		t.Error("an empty project should be omitted, not written blank")
	}
	if !strings.Contains(out, "tags: []") {
		t.Errorf("empty tags should render as an inline empty list, got:\n%s", out)
	}
	if _, err := config.ParseAndValidate([]byte(out)); err != nil {
		t.Fatalf("bare note does not validate: %v", err)
	}
}

// A title with characters that need YAML escaping must not corrupt the block.
func TestGenerateBasicNoteQuotesAwkwardTitles(t *testing.T) {
	for _, title := range []string{`He said "hi"`, "colon: here", "back\\slash", "trail  "} {
		out := GenerateBasicNote("019680f0-7e00-7abc-8def-1234567890ab", title, "", nil)
		z, err := config.ParseAndValidate([]byte(out))
		if err != nil {
			t.Fatalf("title %q produced invalid frontmatter: %v\n%s", title, err, out)
		}
		if z.Title != title {
			t.Errorf("title round-trip: got %q, want %q", z.Title, title)
		}
	}
}

// --- CreateConcept ---

func TestCreateConceptBasic(t *testing.T) {
	cfg, root := testCfg(t)
	path, id, err := CreateConcept(cfg, Spec{Title: "Hello World", Tags: []string{"x"}})
	if err != nil {
		t.Fatalf("CreateConcept: %v", err)
	}
	if filepath.Dir(path) != root {
		t.Errorf("expected the bundle root, got %q", path)
	}
	if base := filepath.Base(path); !strings.HasPrefix(base, "hello-world--") {
		t.Errorf("filename = %q", base)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	z, err := config.ParseAndValidate(content)
	if err != nil {
		t.Fatalf("created concept does not validate: %v\n%s", err, content)
	}
	if z.ID != id {
		t.Errorf("frontmatter id %q != returned id %q", z.ID, id)
	}
}

func TestCreateConceptWithPathDescriptionAndBody(t *testing.T) {
	cfg, root := testCfg(t)
	path, _, err := CreateConcept(cfg, Spec{
		Title:       "Nested Note",
		Dir:         "area/sub",
		Description: "One sentence summary.",
		Body:        "Extra body paragraph.",
	})
	if err != nil {
		t.Fatalf("CreateConcept: %v", err)
	}
	if want := filepath.Join(root, "area", "sub"); filepath.Dir(path) != want {
		t.Errorf("dir = %q, want %q", filepath.Dir(path), want)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	z, err := config.ParseAndValidate(content)
	if err != nil {
		t.Fatalf("does not validate: %v\n%s", err, content)
	}
	if z.Description != "One sentence summary." {
		t.Errorf("description = %q", z.Description)
	}
	if !strings.Contains(string(content), "Extra body paragraph.") {
		t.Errorf("body not appended:\n%s", content)
	}
}

func TestCreateConceptWithTemplate(t *testing.T) {
	cfg, _ := testCfg(t)
	path, _, err := CreateConcept(cfg, Spec{Title: "Standup", Template: "meeting", Tags: []string{"team"}})
	if err != nil {
		t.Fatalf("CreateConcept: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.ParseAndValidate(content); err != nil {
		t.Fatalf("templated concept does not validate: %v\n%s", err, content)
	}
	// The meeting template contributes its own tag alongside the caller's.
	if !strings.Contains(string(content), `- "team"`) {
		t.Errorf("caller tag missing:\n%s", content)
	}
}

func TestCreateConceptRejectsUnknownTemplate(t *testing.T) {
	cfg, _ := testCfg(t)
	if _, _, err := CreateConcept(cfg, Spec{Title: "X", Template: "nope"}); err == nil {
		t.Error("expected an error for an unknown template")
	}
}

func TestCreateConceptRejectsEscapingPath(t *testing.T) {
	cfg, _ := testCfg(t)
	for _, dir := range []string{"../escape", ".index"} {
		if _, _, err := CreateConcept(cfg, Spec{Title: "X", Dir: dir}); err == nil {
			t.Errorf("Dir %q should be rejected", dir)
		}
	}
}

func TestCreateConceptTwiceDoesNotOverwrite(t *testing.T) {
	cfg, _ := testCfg(t)
	first, _, err := CreateConcept(cfg, Spec{Title: "Same Title"})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := CreateConcept(cfg, Spec{Title: "Same Title"})
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("two concepts with the same title collided on one filename")
	}
	for _, p := range []string{first, second} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s missing: %v", p, err)
		}
	}
}

// --- ResolveConceptPath ---

func TestResolveConceptPathByIDAnywhereInBundle(t *testing.T) {
	cfg, root := testCfg(t)
	const id = "019680f0-7e00-7abc-8def-1234567890ab"
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(deep, "buried--019680f07e00.md")
	if err := os.WriteFile(want, []byte(GenerateBasicNote(id, "Buried", "", nil)), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveConceptPath(cfg, id)
	if err != nil {
		t.Fatalf("ResolveConceptPath: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Legacy bundles named files <id>.md; that fast path must still resolve.
func TestResolveConceptPathLegacyFilename(t *testing.T) {
	cfg, root := testCfg(t)
	const id = "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	want := filepath.Join(root, id+".md")
	if err := os.WriteFile(want, []byte("---\nid: \""+id+"\"\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveConceptPath(cfg, id)
	if err != nil {
		t.Fatalf("ResolveConceptPath: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveConceptPathNotFound(t *testing.T) {
	cfg, _ := testCfg(t)
	if _, err := ResolveConceptPath(cfg, "019680f0-7e00-7abc-8def-000000000000"); err == nil {
		t.Error("expected a not-found error")
	}
}

func TestResolveConceptPathSkipsReservedAndHidden(t *testing.T) {
	cfg, root := testCfg(t)
	const id = "019680f0-7e00-7abc-8def-1234567890ab"
	// A reserved file and a file inside a skipped dot-dir both carry the id;
	// neither may be resolved.
	if err := os.WriteFile(filepath.Join(root, "index.md"), []byte("---\nid: \""+id+"\"\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".index"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".index", "x.md"), []byte("---\nid: \""+id+"\"\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got, err := ResolveConceptPath(cfg, id); err == nil {
		t.Errorf("resolved to %q; reserved and hidden files must be skipped", got)
	}
}

func TestResolveConceptPathAcceptsDirectFilePath(t *testing.T) {
	cfg, root := testCfg(t)
	p := filepath.Join(root, "direct--0001.md")
	if err := os.WriteFile(p, []byte("---\nid: \"x\"\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveConceptPath(cfg, p)
	if err != nil {
		t.Fatalf("ResolveConceptPath: %v", err)
	}
	if got != p {
		t.Errorf("got %q, want %q", got, p)
	}
	if _, err := ResolveConceptPath(cfg, filepath.Join(root, "missing.md")); err == nil {
		t.Error("expected an error for a missing file path")
	}
}

// An OKF concept ID is the bundle-relative path minus ".md" (section 2). It
// must resolve in every spelling an agent is likely to produce.
func TestResolveConceptPathAcceptsOKFConceptIDs(t *testing.T) {
	cfg, root := testCfg(t)
	nested := filepath.Join(root, "metrics", "income--0001.md")
	top := filepath.Join(root, "overview--0002.md")
	for _, p := range []string{nested, top} {
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("---\nid: \"x\"\n---\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for ref, want := range map[string]string{
		"metrics/income--0001":     nested,
		"metrics/income--0001.md":  nested,
		"/metrics/income--0001.md": nested,
		"/metrics/income--0001":    nested,
		"overview--0002":           top,
		"overview--0002.md":        top,
	} {
		got, err := ResolveConceptPath(cfg, ref)
		if err != nil {
			t.Errorf("%s: %v", ref, err)
			continue
		}
		if got != want {
			t.Errorf("%s resolved to %q, want %q", ref, got, want)
		}
	}
	// A bare reserved name is not a concept.
	if err := os.WriteFile(filepath.Join(root, "index.md"), []byte("# /\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got, err := ResolveConceptPath(cfg, "index"); err == nil {
		t.Errorf("bare \"index\" resolved to %q; reserved files are not concepts", got)
	}
}

// Every caller reads or rewrites what the resolver returns, and an MCP client
// supplies the reference unvetted, so nothing outside the bundle may resolve —
// not by "..", not by absolute path, not through a symlink.
func TestResolveConceptPathRefusesFilesOutsideTheBundle(t *testing.T) {
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.md")
	if err := os.WriteFile(secret, []byte("---\nid: \"s\"\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, root := testCfg(t)
	if err := os.Symlink(secret, filepath.Join(root, "link.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linkdir")); err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(root, secret)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{
		secret,                // absolute
		filepath.ToSlash(rel), // ../<tmp>/secret.md
		"link.md",             // symlinked file
		"linkdir/secret.md",   // symlinked directory
		"linkdir/secret",      // ...as a concept id
	} {
		got, err := ResolveConceptPath(cfg, ref)
		if err == nil {
			t.Errorf("%s resolved to %q outside the bundle", ref, got)
			continue
		}
		if !strings.Contains(err.Error(), "outside the wiki") {
			t.Errorf("%s: err = %v, want an outside-the-wiki refusal", ref, err)
		}
	}
}

// TestConcurrentCreatesOfOneTitleAllSurvive: the short id in a filename is the
// UUIDv7 millisecond prefix, so creates of one title in the same millisecond
// pick the same name. A stat-then-write claimed it for all of them; measured,
// 40 parallel `okfw create "Same"` runs all reported success and left 35 files.
func TestConcurrentCreatesOfOneTitleAllSurvive(t *testing.T) {
	cfg, _ := testCfg(t)
	const n = 40
	type result struct{ path, id string }
	results := make([]result, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i].path, results[i].id, errs[i] = CreateConcept(cfg, Spec{Title: "Same"})
		}()
	}
	wg.Wait()

	seen := map[string]bool{}
	for i, r := range results {
		if errs[i] != nil {
			t.Fatalf("create %d: %v", i, errs[i])
		}
		if seen[r.path] {
			t.Errorf("two creates returned %s", r.path)
		}
		seen[r.path] = true
		content, err := os.ReadFile(r.path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), `id: "`+r.id+`"`) {
			t.Errorf("%s does not hold the concept created for it (id %s)", r.path, r.id)
		}
	}
}

// The docs and search output abbreviate ids (`okfw set-lifecycle 019f0494
// draft`); the resolver used to accept only a full id, so every such example
// failed with "concept not found".
func TestResolveConceptPathAcceptsAUniqueIDPrefix(t *testing.T) {
	cfg, root := testCfg(t)
	write := func(rel, id string) string {
		p := filepath.Join(root, rel)
		if err := os.WriteFile(p, []byte(GenerateBasicNote(id, "X", "", nil)), 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a := write("a--0001.md", "019f0494-1111-7abc-8def-1234567890ab")
	write("b--0002.md", "019f0495-2222-7abc-8def-1234567890ab")
	write("c--0003.md", "019f0495-3333-7abc-8def-1234567890ab")

	for _, ref := range []string{"019f0494", "019F0494", "019f04941111", "019f0494-1111"} {
		if got, err := ResolveConceptPath(cfg, ref); err != nil || got != a {
			t.Errorf("%s: got %q, %v; want %s", ref, got, err, a)
		}
	}
	if _, err := ResolveConceptPath(cfg, "019f0495"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("an ambiguous prefix: err = %v", err)
	}
	if _, err := ResolveConceptPath(cfg, "019f049"); err == nil {
		t.Error("a 7-digit prefix resolved; prefixes need at least 8 digits")
	}
}

// A concept copied as a template keeps its id. Lookup stopped at the first
// file declaring it, so a writer edited whichever copy the walk reached first.
func TestResolveConceptPathRefusesADuplicatedID(t *testing.T) {
	cfg, root := testCfg(t)
	const id = "019f0494-1111-7abc-8def-1234567890ab"
	for _, rel := range []string{"a--0001.md", "copy-of-a--0001.md"} {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(GenerateBasicNote(id, "A", "", nil)), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, ref := range []string{id, "019f04941111"} {
		_, err := ResolveConceptPath(cfg, ref)
		if err == nil || !strings.Contains(err.Error(), "a--0001.md") || !strings.Contains(err.Error(), "copy-of-a--0001.md") {
			t.Errorf("%s: err = %v, want a refusal naming both copies", ref, err)
		}
	}
	// A path still picks one copy out.
	if got, err := ResolveConceptPath(cfg, "copy-of-a--0001.md"); err != nil || filepath.Base(got) != "copy-of-a--0001.md" {
		t.Errorf("by path: got %q, %v", got, err)
	}
}

// Lookup skips parsing any file that does not contain the reference in one of
// the id layouts. Each case here is one the pre-filter must not hide.
func TestResolveConceptPathFindsEveryIDLayoutThroughItsPrefilter(t *testing.T) {
	cfg, root := testCfg(t)
	write := func(rel, id string) string {
		p := filepath.Join(root, rel)
		if err := os.WriteFile(p, []byte(GenerateBasicNote(id, "X", "", nil)), 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	legacy := write("legacy--0001.md", "20240115103000-9b2c3d4e-5f60-4a1b-8c2d-3e4f5a6b7c8d")
	upper := write("upper--0002.md", "019F0494-2222-7ABC-8DEF-1234567890AB")
	odd := write("odd--0003.md", "my-hand-written-id")

	for ref, want := range map[string]string{
		"20240115103000-9b2c3d4e-5f60-4a1b-8c2d-3e4f5a6b7c8d": legacy,
		"20240115":                legacy,
		"202401151030009b2c":      legacy,
		"20240115103000-9b2c3d4e": legacy,
		"019f0494-2222":           upper,
		"019f04942222":            upper,
		"my-hand-written-id":      odd,
	} {
		if got, err := ResolveConceptPath(cfg, ref); err != nil || got != want {
			t.Errorf("%s: got %q, %v; want %s", ref, got, err, want)
		}
	}
}
