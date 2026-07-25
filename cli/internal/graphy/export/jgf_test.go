package export

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/store"
)

// renderJGF writes the document and decodes it into untyped JSON, so the tests
// assert the shape a JGF consumer actually sees rather than round-tripping
// through the same structs that produced it.
func renderJGF(t *testing.T, g *Graph) (string, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if err := WriteJGF(&buf, g, "graphy test"); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	graph, ok := doc["graph"].(map[string]any)
	if !ok {
		t.Fatalf("no top-level \"graph\" object: %v", doc)
	}
	return buf.String(), graph
}

// TestJGFShape checks the parts of the spec a consumer keys on: a single
// "graph", nodes as an object keyed by id, and edges as an array of
// source/target/relation.
func TestJGFShape(t *testing.T) {
	_, graph := renderJGF(t, formatFixture())

	if graph["type"] != JGFType {
		t.Errorf("type = %v, want %q", graph["type"], JGFType)
	}
	if graph["directed"] != true {
		t.Errorf("directed = %v, want true", graph["directed"])
	}
	if graph["id"] != "/repo/myproj" {
		t.Errorf("id = %v", graph["id"])
	}

	nodes, ok := graph["nodes"].(map[string]any)
	if !ok {
		t.Fatalf("nodes is %T, want a JSON object keyed by node id", graph["nodes"])
	}
	if len(nodes) != 4 {
		t.Errorf("got %d nodes, want 4", len(nodes))
	}
	if _, ok := nodes["go:function:a.go#Run"]; !ok {
		t.Errorf("nodes are not keyed by graphy node id: %v", keysOf(nodes))
	}

	edges, ok := graph["edges"].([]any)
	if !ok {
		t.Fatalf("edges is %T, want an array", graph["edges"])
	}
	if len(edges) != 3 {
		t.Fatalf("got %d edges, want 3", len(edges))
	}
	for _, raw := range edges {
		e := raw.(map[string]any)
		for _, field := range []string{"source", "target", "relation"} {
			if s, _ := e[field].(string); s == "" {
				t.Errorf("edge missing %s: %v", field, e)
			}
		}
		if _, ok := nodes[e["source"].(string)]; !ok {
			t.Errorf("edge source %v is not a declared node", e["source"])
		}
		if _, ok := nodes[e["target"].(string)]; !ok {
			t.Errorf("edge target %v is not a declared node", e["target"])
		}
	}
}

func TestJGFMetadata(t *testing.T) {
	_, graph := renderJGF(t, formatFixture())
	meta := graph["metadata"].(map[string]any)
	if meta["generator"] != "graphy test" || meta["root"] != "/repo/myproj" {
		t.Errorf("graph metadata = %v", meta)
	}
	stats := meta["stats"].(map[string]any)
	if stats["nodes"] != 4.0 || stats["edges"] != 3.0 || stats["communities"] != 2.0 {
		t.Errorf("stats = %v", stats)
	}
	// The partition is restated at the top so a consumer can group by
	// subsystem without scanning every node.
	communities := meta["communities"].([]any)
	if len(communities) != 2 {
		t.Fatalf("communities = %v", communities)
	}
	first := communities[0].(map[string]any)
	if first["id"] != 0.0 || first["label"] != "core" || first["size"] != 2.0 {
		t.Errorf("community[0] = %v", first)
	}

	nodes := graph["nodes"].(map[string]any)
	run := nodes["go:function:a.go#Run"].(map[string]any)
	if run["label"] != "Run" {
		t.Errorf("node label = %v", run["label"])
	}
	nm := run["metadata"].(map[string]any)
	if nm["kind"] != "function" || nm["language"] != "go" || nm["source_file"] != "a.go" {
		t.Errorf("node metadata = %v", nm)
	}
	if nm["community"] != 0.0 || nm["community_label"] != "core" {
		t.Errorf("community denormalization = %v / %v", nm["community"], nm["community_label"])
	}
	span := nm["source_location"].(map[string]any)
	if span["start_line"] != 10.0 || span["end_line"] != 20.0 {
		t.Errorf("source_location = %v", span)
	}

	// An unclustered node must not be reported as a member of community 0.
	ext := nodes["go:external:#net/http"].(map[string]any)["metadata"].(map[string]any)
	if _, ok := ext["community"]; ok {
		t.Errorf("unclustered node claims community %v", ext["community"])
	}
	if ext["external"] != true {
		t.Errorf("external = %v, want true", ext["external"])
	}
	if _, ok := ext["source_location"]; ok {
		t.Error("a node with no span emitted source_location")
	}

	rationale := nodes["go:rationale:a.go#Run@WHY@0"].(map[string]any)["metadata"].(map[string]any)
	if !strings.HasPrefix(rationale["body"].(string), "WHY: retries are capped at 3") {
		t.Errorf("rationale body = %v", rationale["body"])
	}

	var explains map[string]any
	for _, raw := range graph["edges"].([]any) {
		if e := raw.(map[string]any); e["relation"] == "explains" {
			explains = e
		}
	}
	em := explains["metadata"].(map[string]any)
	if em["confidence"] != "EXTRACTED" || em["site_file"] != "a.go" || em["site_line"] != 12.0 {
		t.Errorf("edge metadata = %v", em)
	}
}

// TestJGFDoesNotEscapeHTML: Go's encoder escapes <, > and & into \u00XX
// sequences by default, which is valid JSON but turns every generic and
// channel direction in a signature into unreadable noise.
func TestJGFDoesNotEscapeHTML(t *testing.T) {
	out, _ := renderJGF(t, formatFixture())
	for _, esc := range []string{"\\u003c", "\\u003e", "\\u0026"} {
		if strings.Contains(out, esc) {
			t.Errorf("signatures were HTML-escaped (found %s)", esc)
		}
	}
	if !strings.Contains(out, "func Handle(c <-chan T) error") {
		t.Error("the signature is not present verbatim")
	}
}

func TestJGFIsDeterministic(t *testing.T) {
	var first, second bytes.Buffer
	if err := WriteJGF(&first, formatFixture(), "graphy test"); err != nil {
		t.Fatal(err)
	}
	g := formatFixture()
	reversed := make([]model.Node, len(g.Nodes))
	for i, n := range g.Nodes {
		reversed[len(g.Nodes)-1-i] = n
	}
	shuffled := NewGraph(g.Root, reversed, g.Edges, []store.Community{g.Communities[1], g.Communities[0]}, g.Files)
	if err := WriteJGF(&second, shuffled, "graphy test"); err != nil {
		t.Fatal(err)
	}
	if first.String() != second.String() {
		t.Error("JGF output depends on input order")
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
