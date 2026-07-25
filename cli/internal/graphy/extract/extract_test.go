package extract

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"okfw/cli/internal/graphy/detect"
	"okfw/cli/internal/graphy/model"
)

var update = flag.Bool("update", false, "rewrite golden files")

// golden compares an extraction Result against testdata/<lang>/<name>.golden.json.
// Goldens double as canaries for grammar upgrades: a tree-sitter bump that
// changes extraction output fails here loudly.
func golden(t *testing.T, lang, srcName string) {
	t.Helper()
	session, err := NewSession([]string{lang})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer session.Close()

	srcPath := filepath.Join("testdata", lang, srcName)
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	file := detect.File{
		Path:     "testdata/" + lang + "/" + srcName,
		AbsPath:  srcPath,
		Language: lang,
	}
	res, err := session.Extract(file, src)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	got, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')

	goldenPath := srcPath + ".golden.json"
	if *update {
		if err := os.WriteFile(goldenPath, got, 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("missing golden (run with -update): %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("extraction drifted from golden %s.\nGot:\n%s", goldenPath, got)
	}
}

func TestGoGolden(t *testing.T)       { golden(t, "go", "sample.go") }
func TestMarkdownGolden(t *testing.T) { golden(t, "markdown", "sample.md") }

func TestGoExtractShape(t *testing.T) {
	session, err := NewSession([]string{"go"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	src, err := os.ReadFile(filepath.Join("testdata", "go", "sample.go"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := session.Extract(detect.File{Path: "sample.go", Language: "go"}, src)
	if err != nil {
		t.Fatal(err)
	}

	if res.Nodes[0].Kind != model.KindFile {
		t.Errorf("Nodes[0] kind = %s, want file", res.Nodes[0].Kind)
	}
	byLabel := map[string]model.Node{}
	for _, n := range res.Nodes {
		byLabel[n.Label] = n
	}
	checks := []struct {
		label string
		kind  model.Kind
	}{
		{"Greeter", model.KindStruct},
		{"Mood", model.KindInterface},
		{"Alias", model.KindType},
		{"NewGreeter", model.KindFunction},
		{"Greeter.Greet", model.KindMethod},
		{"format", model.KindFunction},
	}
	for _, c := range checks {
		n, ok := byLabel[c.label]
		if !ok {
			t.Errorf("missing node %q", c.label)
			continue
		}
		if n.Kind != c.kind {
			t.Errorf("%s kind = %s, want %s", c.label, n.Kind, c.kind)
		}
	}
	if byLabel["Greeter"].Doc != "Greeter greets people." {
		t.Errorf("Greeter doc = %q", byLabel["Greeter"].Doc)
	}

	refNames := map[string]model.Relation{}
	for _, r := range res.Refs {
		refNames[r.Name] = r.Relation
	}
	if refNames["fmt"] != model.RelImports || refNames["strings"] != model.RelImports {
		t.Errorf("imports missing: %v", refNames)
	}
	if refNames["format"] != model.RelCalls {
		t.Errorf("intra-file call ref missing: %v", refNames)
	}
	if refNames["Greeter"] != model.RelDefines {
		t.Errorf("method-owner defines ref missing: %v", refNames)
	}
}

func TestMarkdownExtractShape(t *testing.T) {
	session, err := NewSession([]string{"markdown"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	src, err := os.ReadFile(filepath.Join("testdata", "markdown", "sample.md"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := session.Extract(detect.File{Path: "docs/sample.md", Language: "markdown"}, src)
	if err != nil {
		t.Fatal(err)
	}

	if res.Nodes[0].Kind != model.KindDoc || res.Nodes[0].Doc != "Sample Document" {
		t.Errorf("doc node = %+v", res.Nodes[0])
	}
	headings := 0
	for _, n := range res.Nodes[1:] {
		if n.Kind != model.KindHeading {
			t.Errorf("unexpected node kind %s", n.Kind)
		}
		headings++
	}
	// H1 + one deduplicated H2; fenced pseudo-heading and H3 excluded.
	if headings != 2 {
		t.Errorf("headings = %d, want 2 (%+v)", headings, res.Nodes)
	}

	var targets []string
	for _, r := range res.Refs {
		if r.Relation == model.RelReferences {
			targets = append(targets, r.Name)
		}
	}
	want := map[string]bool{"docs/design.md": true, "src/main.go": true}
	if len(targets) != 2 || !want[targets[0]] || !want[targets[1]] {
		t.Errorf("link targets = %v, want docs/design.md + src/main.go", targets)
	}
}
