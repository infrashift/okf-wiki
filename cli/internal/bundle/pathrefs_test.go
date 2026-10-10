package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"okfw/cli/internal/config"
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

const allFiveRefs = `resource: "/tables/orders.md"
sources:
  - resource: "https://example.com/schema"
  - resource: "all queries in project X"
  - resource: "../notes/derived.md#method" # cites a section
computation: "computations/revenue.md"
executor:
  resource: "references/skills/run-on-bq.md"
attester:
  resource: "references/attesters/sql-equality.py"
`

// TestPathRefFieldsMatchTheRewriter: the graph reads path-valued fields from
// Concept.PathRefs and move rewrites them through pathRefNodes. A field only
// one of them knew would be an edge move breaks, or a rewrite the graph never
// shows, so the two lists must agree.
func TestPathRefFieldsMatchTheRewriter(t *testing.T) {
	content := []byte("---\n" + allFiveRefs + "---\n")
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
	if len(read) != 7 {
		t.Errorf("got %d refs, want all 7 values across the five fields: %q", len(read), read)
	}
}

// TestFrontmatterPathsAreGraphEdges: OKF section 5.1 says a source naming a
// concept is already an edge in the bundle graph. URLs, scope descriptors and
// non-markdown files are not.
func TestFrontmatterPathsAreGraphEdges(t *testing.T) {
	cfg, _ := testCfg(t)
	concept(t, cfg, "tables/orders.md", "Orders", "# Orders\n")
	concept(t, cfg, "notes/derived.md", "Derived", "# Derived\n")
	concept(t, cfg, "computations/revenue.md", "Revenue", "# Revenue\n")
	referrer(t, cfg, "finance/report.md", allFiveRefs)

	g, err := LoadGraph(cfg.RootPath)
	if err != nil {
		t.Fatal(err)
	}
	srcKey, _ := ConceptKeyOf(cfg, "finance/report.md")
	for _, target := range []string{"tables/orders.md", "notes/derived.md", "computations/revenue.md"} {
		key, _ := ConceptKeyOf(cfg, target)
		back := g.Backlinks(key)
		if len(back) != 1 || back[0].ID != srcKey {
			t.Errorf("backlinks of %s = %v, want the referrer", target, back)
		}
	}
	// executor.resource names a concept that does not exist: dangling, by path,
	// and bare, so from the bundle root.
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
func TestMoveRetargetsFrontmatterPaths(t *testing.T) {
	cfg, _ := testCfg(t)
	orders := concept(t, cfg, "tables/orders.md", "Orders", "# Orders\n")
	concept(t, cfg, "notes/derived.md", "Derived", "# Derived\n")
	revenue := concept(t, cfg, "computations/revenue.md", "Revenue", "# Revenue\n")
	src := referrer(t, cfg, "finance/report.md", allFiveRefs)
	bystander := referrer(t, cfg, "other.md", "resource: \"bigquery://p.d.orders\"\n")
	before, _ := os.ReadFile(bystander)

	// Inbound: the referrer's bundle-absolute and bare paths follow their
	// targets, each keeping its form.
	if _, err := MoveConcept(cfg, orders, filepath.Join(cfg.RootPath, "warehouse", "orders.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := MoveConcept(cfg, revenue, filepath.Join(cfg.RootPath, "finance", "revenue.md")); err != nil {
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
		"resource":            "/warehouse/orders.md",           // absolute form kept
		"sources[2].resource": "../../notes/derived.md#method",  // relative form, fragment kept
		"computation":         "finance/revenue.md",             // bare form kept
		"executor.resource":   "references/skills/run-on-bq.md", // bare paths survive the referrer's own move
		"sources[0].resource": "https://example.com/schema",     // not paths: untouched
		"sources[1].resource": "all queries in project X",
		"attester.resource":   "references/attesters/sql-equality.py",
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

// TestMoveNeverLeavesARelativePathBare: a move rewrites frontmatter paths with
// the closures it uses for body links, where a bare path is relative. In
// frontmatter a bare path is read from the bundle root, so ../a/x.md seen from
// a/ must become ./x.md, not x.md, which names a different file.
func TestMoveNeverLeavesARelativePathBare(t *testing.T) {
	cfg, _ := testCfg(t)
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
	if z.Resource != "./x.md" {
		t.Errorf("resource = %q, want ./x.md", z.Resource)
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
