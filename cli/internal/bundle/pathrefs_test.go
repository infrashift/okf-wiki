package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/infrashift/okf-wiki/internal/config"
)

// referrer writes a concept whose frontmatter carries extra, verbatim.
func referrer(t *testing.T, cfg *config.Config, rel, extra string) string {
	t.Helper()
	conceptSeq++
	content := fmt.Sprintf("---\nid: \"019680f0-7e00-7abc-8def-%012x\"\n", conceptSeq) +
		"title: \"Referrer\"\ntype: \"note\"\ntags: []\n" +
		"created: \"2026-01-01T00:00:00Z\"\ntimestamp: \"2026-01-01T00:00:00Z\"\n" +
		extra + "---\n\n# Referrer\n"
	p := filepath.Join(cfg.RootPath, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

// v03Bundle marks the test bundle OKF v0.3, so a bare path resolves from its
// file's directory with no v0.2 fallback (section 6.2).
func v03Bundle(t *testing.T, cfg *config.Config) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(cfg.RootPath, "index.md"), []byte("---\nokf_version: \"0.3\"\n---\n\n# /\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

// allPathRefs fills every path field of OKF v0.3 section 6.2, from a concept
// at finance/report.md, in each spelling section 6 allows.
const allPathRefs = `resource: "/tables/orders.md"
sources:
  - resource: "https://example.com/schema"
  - resource: "all queries in project X"
  - resource: "../notes/derived.md#method" # cites a section
computation: "revenue.md"
executor:
  resource: "/references/skills/run-on-bq.md"
attester:
  resource: "/references/attesters/sql-equality.py"
superseded_by: "../notes/derived.md"
`

// TestPathRefFieldsMatchTheRewriter: the graph reads path-valued fields from
// Concept.PathRefs and move rewrites them through pathRefNodes. A field only
// one of them knew would be an edge move breaks, or a rewrite the graph never
// shows, so the two lists must agree.
func TestPathRefFieldsMatchTheRewriter(t *testing.T) {
	content := []byte("---\n" + allPathRefs + "---\n")
	z, err := config.ParseFrontmatter(content)
	if err != nil {
		t.Fatal(err)
	}
	var read []string
	for _, r := range z.PathRefs() {
		read = append(read, r.Field+"="+r.Value)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(content[4:len(content)-4], &doc); err != nil {
		t.Fatal(err)
	}
	var walked []string
	for _, r := range pathRefNodes(doc.Content[0]) {
		walked = append(walked, r.field+"="+r.node.Value)
	}
	if !reflect.DeepEqual(read, walked) {
		t.Errorf("PathRefs:\n%q\npathRefNodes:\n%q", read, walked)
	}
	if len(read) != 8 {
		t.Errorf("got %d refs, want all 8 values across the six fields: %q", len(read), read)
	}
}

// TestFrontmatterPathsAreGraphEdges: OKF v0.3 section 6.4 makes a path field
// that resolves to a concept a graph edge. URLs, scope descriptors and
// non-markdown files are not.
func TestFrontmatterPathsAreGraphEdges(t *testing.T) {
	cfg, _ := testCfg(t)
	v03Bundle(t, cfg)
	concept(t, cfg, "tables/orders.md", "Orders", "# Orders\n")
	concept(t, cfg, "notes/derived.md", "Derived", "# Derived\n")
	concept(t, cfg, "finance/revenue.md", "Revenue", "# Revenue\n")
	concept(t, cfg, "revenue.md", "Decoy", "# Decoy\n") // a bare path is not from the root
	referrer(t, cfg, "finance/report.md", allPathRefs)

	g, err := LoadGraph(cfg.RootPath)
	if err != nil {
		t.Fatal(err)
	}
	srcKey, _ := ConceptKeyOf(cfg, "finance/report.md")
	for _, target := range []string{"tables/orders.md", "notes/derived.md", "finance/revenue.md"} {
		key, _ := ConceptKeyOf(cfg, target)
		back := g.Backlinks(key)
		if len(back) != 1 || back[0].ID != srcKey {
			t.Errorf("backlinks of %s = %v, want the referrer", target, back)
		}
	}
	if key, _ := ConceptKeyOf(cfg, "revenue.md"); len(g.Backlinks(key)) != 0 {
		t.Error("a bare path resolved from the bundle root in a v0.3 bundle")
	}
	// executor.resource names a concept that does not exist: dangling, by path.
	var dangling []string
	for _, e := range g.Dangling() {
		dangling = append(dangling, e.To)
	}
	if want := []string{"references/skills/run-on-bq"}; !reflect.DeepEqual(dangling, want) {
		t.Errorf("dangling = %q, want %q (the URL, scope and .py are not links)", dangling, want)
	}

	inbound, err := InboundLinks(cfg.RootPath, "tables/orders.md")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"finance/report.md"}; !reflect.DeepEqual(inbound, want) {
		t.Errorf("delete would report inbound %q, want %q", inbound, want)
	}
}

// TestMoveRetargetsFrontmatterPaths: a move rewrote body links at both ends and
// left frontmatter paths naming the old location, so the edge broke silently.
// Under OKF v0.3 a path field reads exactly as a body link does, so it is
// rewritten the same way, each value keeping its form.
func TestMoveRetargetsFrontmatterPaths(t *testing.T) {
	cfg, _ := testCfg(t)
	v03Bundle(t, cfg)
	orders := concept(t, cfg, "tables/orders.md", "Orders", "# Orders\n")
	concept(t, cfg, "notes/derived.md", "Derived", "# Derived\n")
	revenue := concept(t, cfg, "finance/revenue.md", "Revenue", "# Revenue\n")
	src := referrer(t, cfg, "finance/report.md", allPathRefs)
	bystander := referrer(t, cfg, "other.md", "resource: \"bigquery://p.d.orders\"\n")
	before, _ := os.ReadFile(bystander)

	// Inbound: the referrer's paths follow their targets.
	if _, err := MoveConcept(cfg, orders, filepath.Join(cfg.RootPath, "warehouse", "orders.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := MoveConcept(cfg, revenue, filepath.Join(cfg.RootPath, "computations", "revenue.md")); err != nil {
		t.Fatal(err)
	}
	// Own: moving the referrer keeps its relative paths resolving.
	moved := filepath.Join(cfg.RootPath, "reports", "q3", "report.md")
	if _, err := MoveConcept(cfg, src, moved); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(moved)
	z, err := config.ParseFrontmatter(got)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"resource":            "/warehouse/orders.md",            // absolute form kept
		"sources[2].resource": "../../notes/derived.md#method",   // relative form, fragment kept
		"computation":         "../../computations/revenue.md",   // bare relative, retargeted
		"executor.resource":   "/references/skills/run-on-bq.md", // absolute survives the referrer's move
		"superseded_by":       "../../notes/derived.md",          // lifecycle path, retargeted
		"sources[0].resource": "https://example.com/schema",      // not paths: untouched
		"sources[1].resource": "all queries in project X",
		"attester.resource":   "/references/attesters/sql-equality.py",
	}
	for _, r := range z.PathRefs() {
		if want[r.Field] != r.Value {
			t.Errorf("%s = %q, want %q", r.Field, r.Value, want[r.Field])
		}
	}
	if !strings.Contains(string(got), "# cites a section") {
		t.Errorf("the rewrite dropped a frontmatter comment:\n%s", got)
	}
	if after, _ := os.ReadFile(bystander); string(after) != string(before) {
		t.Errorf("a concept with no path to the moved one was rewritten:\n%s", after)
	}

	g, err := LoadGraph(cfg.RootPath)
	if err != nil {
		t.Fatal(err)
	}
	var dangling []string
	for _, e := range g.Dangling() {
		dangling = append(dangling, e.To)
	}
	if want := []string{"references/skills/run-on-bq"}; !reflect.DeepEqual(dangling, want) {
		t.Errorf("after the moves dangling = %q, want only the edge that was already dangling", dangling)
	}
}

// TestMoveWritesFrontmatterPathsLikeBodyLinks: under OKF v0.3 a bare
// frontmatter path is relative, as a body link is, so ../a/x.md seen from a/
// becomes x.md — which now names a/x.md, not the decoy at the root.
func TestMoveWritesFrontmatterPathsLikeBodyLinks(t *testing.T) {
	cfg, _ := testCfg(t)
	v03Bundle(t, cfg)
	concept(t, cfg, "a/x.md", "X", "# X\n")
	concept(t, cfg, "x.md", "Decoy", "# Decoy\n")
	src := referrer(t, cfg, "b/c.md", "resource: \"../a/x.md\"\n")

	moved := filepath.Join(cfg.RootPath, "a", "c.md")
	if _, err := MoveConcept(cfg, src, moved); err != nil {
		t.Fatal(err)
	}
	z, err := config.ParseFrontmatter(mustRead(t, moved))
	if err != nil {
		t.Fatal(err)
	}
	if z.Resource != "x.md" {
		t.Errorf("resource = %q, want x.md", z.Resource)
	}
}

// TestLegacyBundleBarePathsFallBackToTheRoot: a bundle declaring 0.2 (or no
// version) gets section 6.2's compatibility reading — a bare path that names
// nothing from its file's directory is tried from the root — in the graph, in
// delete's inbound report, and in a move, which writes the new path
// bundle-absolute so it no longer depends on the fallback.
func TestLegacyBundleBarePathsFallBackToTheRoot(t *testing.T) {
	cfg, _ := testCfg(t)
	if err := os.WriteFile(filepath.Join(cfg.RootPath, "index.md"), []byte("---\nokf_version: \"0.2\"\n---\n\n# /\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run := concept(t, cfg, "skills/run.md", "Run", "# Run\n")
	src := referrer(t, cfg, "computations/revenue.md", "executor:\n  resource: \"skills/run.md\"\n")

	g, err := LoadGraph(cfg.RootPath)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := ConceptKeyOf(cfg, "skills/run.md")
	if len(g.Backlinks(key)) != 1 {
		t.Errorf("the v0.2 fallback did not resolve a root-relative path")
	}
	if inbound, _ := InboundLinks(cfg.RootPath, "skills/run.md"); len(inbound) != 1 {
		t.Errorf("inbound = %q, want the referrer", inbound)
	}
	if _, err := MoveConcept(cfg, run, filepath.Join(cfg.RootPath, "skills", "bq", "run.md")); err != nil {
		t.Fatal(err)
	}
	z, err := config.ParseFrontmatter(mustRead(t, src))
	if err != nil {
		t.Fatal(err)
	}
	if z.Executor == nil || z.Executor.Resource != "/skills/bq/run.md" {
		t.Errorf("executor.resource = %+v, want /skills/bq/run.md", z.Executor)
	}
}

// TestPrefilterKeepsPercentEncodedPaths: move and delete skip parsing any
// frontmatter that does not contain the target's file name, which a
// percent-encoded path does not.
func TestPrefilterKeepsPercentEncodedPaths(t *testing.T) {
	cfg, _ := testCfg(t)
	target := concept(t, cfg, "notes/my note.md", "Mine", "# Mine\n")
	src := referrer(t, cfg, "a.md", "resource: \"/notes/my%20note.md\"\n")

	inbound, err := InboundLinks(cfg.RootPath, "notes/my note.md")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a.md"}; !reflect.DeepEqual(inbound, want) {
		t.Errorf("inbound = %q, want %q", inbound, want)
	}
	if _, err := MoveConcept(cfg, target, filepath.Join(cfg.RootPath, "kept", "mine.md")); err != nil {
		t.Fatal(err)
	}
	z, err := config.ParseFrontmatter(mustRead(t, src))
	if err != nil {
		t.Fatal(err)
	}
	if z.Resource != "/kept/mine.md" {
		t.Errorf("resource = %q, want the moved path", z.Resource)
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
