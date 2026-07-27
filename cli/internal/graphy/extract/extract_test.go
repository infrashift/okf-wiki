package extract

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

// TestTSXGolden covers the one place a tree-sitter upgrade is most likely to
// drift silently: .tsx is parsed by a separately compiled grammar
// (tstsx.LanguageTSX), and it was the only registered grammar with no golden.
func TestTSXGolden(t *testing.T)       { golden(t, "typescript", "sample.tsx") }
func TestYAMLGolden(t *testing.T)      { golden(t, "yaml", "sample.yaml") }
func TestJSONGolden(t *testing.T)      { golden(t, "json", "sample.json") }
func TestCUEGolden(t *testing.T)       { golden(t, "cue", "sample.cue") }
func TestTerraformGolden(t *testing.T) { golden(t, "terraform", "sample.tf") }
func TestSQLGolden(t *testing.T)       { golden(t, "sql", "sample.sql") }
func TestAnsibleGolden(t *testing.T)   { golden(t, "yaml", "playbook.yml") }

func TestAnsibleExtractShape(t *testing.T) {
	session, err := NewSession([]string{"yaml"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	src, err := os.ReadFile(filepath.Join("testdata", "yaml", "playbook.yml"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := session.Extract(detect.File{Path: "site.yml", Language: "yaml"}, src)
	if err != nil {
		t.Fatal(err)
	}
	byLabel := map[string]model.Node{}
	for _, n := range res.Nodes[1:] {
		byLabel[n.Label] = n
	}
	named, ok := byLabel["play: Provision web servers"]
	if !ok || named.Kind != model.KindConfig {
		t.Errorf("named play node missing: %v", labelsOf(res.Nodes[1:]))
	}
	if _, ok := byLabel["play: dbservers"]; !ok {
		t.Errorf("unnamed play should be labeled by hosts: %v", labelsOf(res.Nodes[1:]))
	}

	type ref struct {
		from, name string
		rel        model.Relation
	}
	refs := map[ref]bool{}
	for _, r := range res.Refs {
		refs[ref{r.FromID, r.Name, r.Relation}] = true
	}
	// Role references from the play: bare, mapping-form, and include_role.
	for _, role := range []string{"role:common", "role:nginx", "role:app"} {
		if !refs[ref{named.ID, role, model.RelImports}] {
			t.Errorf("%s ref missing: %v", role, refs)
		}
	}
	// Path values inside the play attribute to the play node.
	if !refs[ref{named.ID, "vars/common.yml", model.RelReferences}] {
		t.Errorf("vars_files path ref missing: %v", refs)
	}
	if !refs[ref{named.ID, "tasks/harden.yml", model.RelReferences}] {
		t.Errorf("include_tasks path ref missing: %v", refs)
	}
}

func TestAnsibleTaskListRoleRefs(t *testing.T) {
	session, err := NewSession([]string{"yaml"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	src := []byte(`- name: Reuse the base role
  include_role:
    name: base
- name: Copy a file
  copy:
    src: files/app.conf
    dest: /etc/app.conf
`)
	res, err := session.Extract(detect.File{Path: "roles/web/tasks/main.yml", Language: "yaml"}, src)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Nodes) != 1 {
		t.Errorf("task lists should not produce entry nodes: %v", labelsOf(res.Nodes))
	}
	found := false
	for _, r := range res.Refs {
		if r.Name == "role:base" && r.Relation == model.RelImports {
			found = true
		}
	}
	if !found {
		t.Errorf("include_role in task list missing: %+v", res.Refs)
	}
}

func TestSQLExtractShape(t *testing.T) {
	session, err := NewSession([]string{"sql"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	src, err := os.ReadFile(filepath.Join("testdata", "sql", "sample.sql"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := session.Extract(detect.File{Path: "db/schema.sql", Language: "sql"}, src)
	if err != nil {
		t.Fatal(err)
	}
	byLabel := map[string]model.Node{}
	for _, n := range res.Nodes[1:] {
		byLabel[n.Label] = n
	}
	checks := []struct {
		label string
		kind  model.Kind
	}{
		{"users", model.KindStruct},
		{"orders", model.KindStruct},
		{"order_summary", model.KindType},
		{"touch_updated_at", model.KindFunction},
	}
	for _, c := range checks {
		n, ok := byLabel[c.label]
		if !ok || n.Kind != c.kind {
			t.Errorf("%s = %+v, want kind %s (have %v)", c.label, n, c.kind, labelsOf(res.Nodes[1:]))
		}
	}
	if len(res.Nodes) != 5 {
		t.Errorf("commented-out CREATE leaked or node missing: %v", labelsOf(res.Nodes[1:]))
	}

	type ref struct{ from, name string }
	refs := map[ref]bool{}
	for _, r := range res.Refs {
		refs[ref{r.FromID, r.Name}] = true
	}
	if !refs[ref{byLabel["orders"].ID, "users"}] {
		t.Errorf("orders FK -> users missing: %v", refs)
	}
	// Self-referencing FK is emitted as a ref; resolution drops self-edges.
	if !refs[ref{byLabel["orders"].ID, "orders"}] {
		t.Errorf("self FK ref missing: %v", refs)
	}
	view := byLabel["order_summary"].ID
	if !refs[ref{view, "users"}] || !refs[ref{view, "orders"}] {
		t.Errorf("view FROM/JOIN refs missing: %v", refs)
	}
	// Tables must not emit FROM refs (their bodies have none), and string
	// literals must not leak keywords.
	if refs[ref{byLabel["users"].ID, "nothing"}] {
		t.Errorf("string literal leaked a REFERENCES match")
	}
}

func TestCUEExtractShape(t *testing.T) {
	session, err := NewSession([]string{"cue"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	src, err := os.ReadFile(filepath.Join("testdata", "cue", "sample.cue"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := session.Extract(detect.File{Path: "schema/sample.cue", Language: "cue"}, src)
	if err != nil {
		t.Fatal(err)
	}
	byLabel := map[string]model.Node{}
	for _, n := range res.Nodes[1:] {
		byLabel[n.Label] = n
	}
	if n := byLabel["#Base"]; n.Kind != model.KindType || n.Doc != "#Base is the shared shape." {
		t.Errorf("#Base = %+v", n)
	}
	if n := byLabel["#Concept"]; n.Kind != model.KindType {
		t.Errorf("#Concept = %+v", n)
	}
	if n := byLabel["defaults"]; n.Kind != model.KindConfig {
		t.Errorf("defaults should be a config entry, got %+v", n)
	}

	type ref struct {
		name string
		rel  model.Relation
	}
	refs := map[ref]bool{}
	for _, r := range res.Refs {
		refs[ref{r.Name, r.Relation}] = true
	}
	if !refs[ref{"strings", model.RelImports}] {
		t.Errorf("import ref missing: %v", refs)
	}
	// #Concept embeds #Base.
	if !refs[ref{"#Base", model.RelReferences}] {
		t.Errorf("#Concept -> #Base reference missing: %v", refs)
	}
}

func TestTerraformExtractShape(t *testing.T) {
	session, err := NewSession([]string{"terraform"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	src, err := os.ReadFile(filepath.Join("testdata", "terraform", "sample.tf"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := session.Extract(detect.File{Path: "infra/sample.tf", Language: "terraform"}, src)
	if err != nil {
		t.Fatal(err)
	}
	byLabel := map[string]model.Node{}
	for _, n := range res.Nodes[1:] {
		byLabel[n.Label] = n
	}
	checks := []struct {
		label string
		kind  model.Kind
	}{
		{"var.region", model.KindVariable},
		{"var.instance_type", model.KindVariable},
		{"local.name_prefix", model.KindVariable},
		{"provider.aws", model.KindConfig},
		{"data.aws_ami.ubuntu", model.KindResource},
		{"aws_instance.web", model.KindResource},
		{"module.vpc", model.KindModule},
		{"output.instance_ip", model.KindVariable},
	}
	for _, c := range checks {
		n, ok := byLabel[c.label]
		if !ok || n.Kind != c.kind {
			t.Errorf("%s = %+v, want kind %s (have %v)", c.label, n, c.kind, labelsOf(res.Nodes[1:]))
		}
	}

	type ref struct {
		from, name string
		rel        model.Relation
	}
	refs := map[ref]bool{}
	for _, r := range res.Refs {
		refs[ref{r.FromID, r.Name, r.Relation}] = true
	}
	web := byLabel["aws_instance.web"].ID
	if !refs[ref{web, "data.aws_ami.ubuntu", model.RelReferences}] {
		t.Errorf("resource->data ref missing: %v", refs)
	}
	if !refs[ref{web, "var.instance_type", model.RelReferences}] {
		t.Errorf("resource->var ref missing: %v", refs)
	}
	if !refs[ref{web, "local.name_prefix", model.RelReferences}] {
		t.Errorf("resource->local ref missing: %v", refs)
	}
	if !refs[ref{byLabel["module.vpc"].ID, "./modules/vpc", model.RelImports}] {
		t.Errorf("module source import ref missing: %v", refs)
	}
	if !refs[ref{byLabel["output.instance_ip"].ID, "aws_instance.web", model.RelReferences}] {
		t.Errorf("output->resource ref missing: %v", refs)
	}
	if !refs[ref{byLabel["local.name_prefix"].ID, "var.region", model.RelReferences}] {
		t.Errorf("local->var ref missing: %v", refs)
	}
}

func TestTfvarsReferencesVariables(t *testing.T) {
	session, err := NewSession([]string{"terraform"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	src := []byte("region = \"eu-west-1\"\ninstance_type = \"t3.micro\"\n")
	res, err := session.Extract(detect.File{Path: "envs/prod.tfvars", Language: "terraform"}, src)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Nodes) != 1 {
		t.Errorf("tfvars should only have a file node: %v", labelsOf(res.Nodes))
	}
	names := map[string]bool{}
	for _, r := range res.Refs {
		names[r.Name] = true
	}
	if !names["var.region"] || !names["var.instance_type"] {
		t.Errorf("tfvars refs = %v", names)
	}
}

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

	// References come from links AND from backticked identifiers in prose, each
	// owned by the H1/H2 that encloses it.
	byOwner := map[string]map[string]bool{}
	for _, r := range res.Refs {
		if r.Relation != model.RelReferences {
			continue
		}
		if byOwner[r.FromID] == nil {
			byOwner[r.FromID] = map[string]bool{}
		}
		byOwner[r.FromID][r.Name] = true
		if r.Line == 0 {
			t.Errorf("ref %q has no line", r.Name)
		}
	}
	const (
		h1 = "markdown:heading:docs/sample.md#sample-document"
		h2 = "markdown:heading:docs/sample.md#architecture"
	)
	wantByOwner := map[string]map[string]bool{
		h1: {"docs/design.md": true, "src/main.go": true},
		h2: {
			"Bootstrap":  true, // a backticked identifier in prose
			"store.Open": true, // "()" trimmed: prose writes calls, the index does not
			// A line anchor survives so resolution can aim at the declaration
			// on that line rather than at the whole file.
			"src/main.go#L42": true,
			// Prose under the H3 belongs to the enclosing H2, since only H1/H2
			// become nodes.
			"Architecture": true,
		},
	}
	if !reflect.DeepEqual(byOwner, wantByOwner) {
		t.Errorf("references by owner =\n  %v\nwant\n  %v", byOwner, wantByOwner)
	}
	// Everything deliberately excluded: an external link, a flag, a path, an
	// English filler word, and an identifier inside a fenced block.
	for owner, names := range byOwner {
		for _, unwanted := range []string{"example.com", "--min-degree", "cmd/graphy/main.go", "the", "FencedIdentifier"} {
			if names[unwanted] {
				t.Errorf("%s: %q should not be a reference", owner, unwanted)
			}
		}
	}
}

// Rationale goldens. The go and yaml fixtures both carry a marker inside a
// string literal, so these double as regression canaries for the scanner's
// literal tracking.
func TestGoRationaleGolden(t *testing.T)   { golden(t, "go", "rationale.go") }
func TestYAMLRationaleGolden(t *testing.T) { golden(t, "yaml", "rationale.yaml") }
func TestADRGolden(t *testing.T)           { golden(t, "markdown", "0001-use-bbolt.md") }
