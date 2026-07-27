package bundle

import (
	"os"
	"path/filepath"
	"strings"
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
