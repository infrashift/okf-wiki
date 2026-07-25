package extract

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestGoGolden(t *testing.T)         { golden(t, "go", "sample.go") }
func TestMarkdownGolden(t *testing.T)   { golden(t, "markdown", "sample.md") }
func TestPythonGolden(t *testing.T)     { golden(t, "python", "sample.py") }
func TestTypescriptGolden(t *testing.T) { golden(t, "typescript", "sample.ts") }
func TestJavascriptGolden(t *testing.T) { golden(t, "javascript", "sample.js") }
func TestYAMLGolden(t *testing.T)       { golden(t, "yaml", "sample.yaml") }
func TestJSONGolden(t *testing.T)       { golden(t, "json", "sample.json") }

func TestYAMLExtractShape(t *testing.T) {
	session, err := NewSession([]string{"yaml"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	src, err := os.ReadFile(filepath.Join("testdata", "yaml", "sample.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := session.Extract(detect.File{Path: "deploy/compose.yaml", Language: "yaml"}, src)
	if err != nil {
		t.Fatal(err)
	}

	if res.Nodes[0].Kind != model.KindFile {
		t.Errorf("Nodes[0] = %+v", res.Nodes[0])
	}
	byLabel := map[string]model.Node{}
	for _, n := range res.Nodes[1:] {
		if n.Kind != model.KindConfig {
			t.Errorf("non-config entry node: %+v", n)
		}
		byLabel[n.Label] = n
	}
	// Top-level keys + second-level children from doc 0.
	for _, want := range []string{"version", "services", "services.web", "services.db", "x-scripts"} {
		if _, ok := byLabel[want]; !ok {
			t.Errorf("missing entry %q (have %v)", want, labelsOf(res.Nodes[1:]))
		}
	}
	// Doc 1 is a k8s manifest: one resource node, not key soup.
	if _, ok := byLabel["Deployment/web"]; !ok {
		t.Errorf("missing k8s manifest node (have %v)", labelsOf(res.Nodes[1:]))
	}
	if _, ok := byLabel["apiVersion"]; ok {
		t.Errorf("k8s manifest leaked top-level keys")
	}

	// Path refs: attributed to their entry, both root- and file-relative
	// readings emitted.
	type ref struct{ from, name string }
	refs := map[ref]bool{}
	for _, r := range res.Refs {
		if r.Relation != model.RelReferences {
			t.Errorf("unexpected relation %s", r.Relation)
		}
		refs[ref{r.FromID, r.Name}] = true
	}
	servicesID := byLabel["services"].ID
	if !refs[ref{servicesID, "scripts/start.sh"}] {
		t.Errorf("root-relative ref missing: %v", refs)
	}
	if !refs[ref{servicesID, "deploy/scripts/start.sh"}] {
		t.Errorf("file-relative ref missing: %v", refs)
	}
	if !refs[ref{byLabel["x-scripts"].ID, "tools/deploy.sh"}] {
		t.Errorf("../ ref not normalized: %v", refs)
	}
}

func TestJSONExtractShape(t *testing.T) {
	session, err := NewSession([]string{"json"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	src, err := os.ReadFile(filepath.Join("testdata", "json", "sample.json"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := session.Extract(detect.File{Path: "package.json", Language: "json"}, src)
	if err != nil {
		t.Fatal(err)
	}
	byLabel := map[string]model.Node{}
	for _, n := range res.Nodes[1:] {
		byLabel[n.Label] = n
	}
	for _, want := range []string{"name", "scripts", "scripts.build", "dependencies.left-pad"} {
		if _, ok := byLabel[want]; !ok {
			t.Errorf("missing entry %q (have %v)", want, labelsOf(res.Nodes[1:]))
		}
	}
	found := false
	for _, r := range res.Refs {
		if r.Name == "src/index.js" {
			found = true
		}
	}
	if !found {
		t.Errorf("src/index.js ref missing: %+v", res.Refs)
	}
}

func TestConfigParseFailureDegradesToFileNode(t *testing.T) {
	session, err := NewSession([]string{"yaml"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	src := []byte("key: {{ .Values.name }}\n  broken: [unclosed\n")
	res, err := session.Extract(detect.File{Path: "chart/tpl.yaml", Language: "yaml"}, src)
	if err != nil {
		t.Fatalf("template YAML must not error: %v", err)
	}
	if len(res.Nodes) != 1 || res.Nodes[0].Kind != model.KindFile {
		t.Errorf("expected bare file node, got %+v", res.Nodes)
	}
}

func TestConfigEntryCap(t *testing.T) {
	session, err := NewSession([]string{"json"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	var b strings.Builder
	b.WriteString(`{"packages": {`)
	for i := 0; i < 500; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `"node_modules/dep%d": {}`, i)
	}
	b.WriteString("}}")
	res, err := session.Extract(detect.File{Path: "package-lock.json", Language: "json"}, []byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	// file node + capped entries
	if len(res.Nodes) > maxConfigEntries+1 {
		t.Errorf("entry cap not applied: %d nodes", len(res.Nodes))
	}
}

func labelsOf(nodes []model.Node) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.Label
	}
	return out
}

func TestPythonExtractShape(t *testing.T) {
	session, err := NewSession([]string{"python"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	src, err := os.ReadFile(filepath.Join("testdata", "python", "sample.py"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := session.Extract(detect.File{Path: "sample.py", Language: "python"}, src)
	if err != nil {
		t.Fatal(err)
	}
	byLabel := map[string]model.Node{}
	for _, n := range res.Nodes {
		byLabel[n.Label] = n
	}
	if byLabel["Dog"].Kind != model.KindClass || byLabel["Dog"].Doc != "A loyal creature." {
		t.Errorf("Dog = %+v", byLabel["Dog"])
	}
	if byLabel["Dog.bark"].Kind != model.KindMethod {
		t.Errorf("Dog.bark = %+v", byLabel["Dog.bark"])
	}
	if byLabel["kennel"].Kind != model.KindFunction || byLabel["kennel"].Doc != "Build a default kennel." {
		t.Errorf("kennel = %+v", byLabel["kennel"])
	}
	rels := map[string]model.Relation{}
	for _, r := range res.Refs {
		rels[r.Name] = r.Relation
	}
	if rels["Animal"] != model.RelInherits {
		t.Errorf("Dog(Animal) inherit ref missing: %v", rels)
	}
	if rels["os.path"] != model.RelImports || rels["collections"] != model.RelImports {
		t.Errorf("imports missing: %v", rels)
	}
	if rels["self.bark"] != model.RelCalls {
		t.Errorf("method call ref missing: %v", rels)
	}
}

func TestTypescriptExtractShape(t *testing.T) {
	session, err := NewSession([]string{"typescript"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	src, err := os.ReadFile(filepath.Join("testdata", "typescript", "sample.ts"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := session.Extract(detect.File{Path: "sample.ts", Language: "typescript"}, src)
	if err != nil {
		t.Fatal(err)
	}
	byLabel := map[string]model.Node{}
	for _, n := range res.Nodes {
		byLabel[n.Label] = n
	}
	checks := []struct {
		label string
		kind  model.Kind
	}{
		{"Point", model.KindInterface},
		{"Pair", model.KindType},
		{"Shape", model.KindClass},
		{"Circle", model.KindClass},
		{"Circle.area", model.KindMethod},
		{"totalArea", model.KindFunction},
		{"describe", model.KindFunction},
	}
	for _, c := range checks {
		n, ok := byLabel[c.label]
		if !ok || n.Kind != c.kind {
			t.Errorf("%s = %+v, want kind %s", c.label, n, c.kind)
		}
	}
	if byLabel["Circle"].Doc != "A circle." {
		t.Errorf("Circle doc = %q", byLabel["Circle"].Doc)
	}
	type key struct {
		name string
		rel  model.Relation
	}
	rels := map[key]bool{}
	for _, r := range res.Refs {
		rels[key{r.Name, r.Relation}] = true
	}
	if !rels[key{"Shape", model.RelInherits}] {
		t.Errorf("extends ref missing: %v", rels)
	}
	if !rels[key{"Point", model.RelImplements}] {
		t.Errorf("implements ref missing: %v", rels)
	}
	if !rels[key{"./util", model.RelImports}] {
		t.Errorf("import ref missing: %v", rels)
	}
	if !rels[key{"helper", model.RelCalls}] {
		t.Errorf("helper call ref missing: %v", rels)
	}
}

func TestTSXParses(t *testing.T) {
	session, err := NewSession([]string{"typescript"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	src := []byte(`/** A view. */
export function View({ n }: { n: number }) {
  return <div>{n}</div>;
}
`)
	res, err := session.Extract(detect.File{Path: "view.tsx", Language: "typescript"}, src)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range res.Nodes {
		if n.Label == "View" && n.Kind == model.KindFunction {
			found = true
		}
	}
	if !found {
		t.Errorf("TSX function not extracted: %+v", res.Nodes)
	}
}

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
