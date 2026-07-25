package export

import (
	"bytes"
	"encoding/xml"
	"strconv"
	"strings"
	"testing"

	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/store"
)

func renderGraphML(t *testing.T, g *Graph) (string, graphmlDocument) {
	t.Helper()
	var buf bytes.Buffer
	if err := WriteGraphML(&buf, g, "graphy test"); err != nil {
		t.Fatal(err)
	}
	var doc graphmlDocument
	if err := xml.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("output is not well-formed GraphML: %v\n%s", err, buf.String())
	}
	return buf.String(), doc
}

func TestGraphMLStructure(t *testing.T) {
	out, doc := renderGraphML(t, formatFixture())

	if doc.Graph.EdgeDefault != "directed" {
		t.Errorf("edgedefault = %q, want directed", doc.Graph.EdgeDefault)
	}
	if len(doc.Graph.Nodes) != 4 || len(doc.Graph.Edges) != 3 {
		t.Fatalf("got %d nodes / %d edges, want 4 / 3", len(doc.Graph.Nodes), len(doc.Graph.Edges))
	}
	if !strings.Contains(out, `xmlns="`+graphmlNS+`"`) {
		t.Error("the GraphML namespace declaration is missing")
	}
	if !strings.Contains(doc.Desc, "4 nodes, 3 edges") || !strings.Contains(doc.Desc, "graphy test") {
		t.Errorf("desc = %q", doc.Desc)
	}
	// Edge ids have to be unique: parallel edges between the same pair are
	// legal in this graph.
	seen := map[string]bool{}
	for _, e := range doc.Graph.Edges {
		if e.ID == "" || seen[e.ID] {
			t.Errorf("edge id %q is empty or duplicated", e.ID)
		}
		seen[e.ID] = true
	}
}

// TestGraphMLDeclaresEveryEmittedKey is the invariant that keeps the document
// readable by anything: a <data> whose key was never declared, or declared for
// the other element type, is what makes Gephi and networkx reject a file.
func TestGraphMLDeclaresEveryEmittedKey(t *testing.T) {
	_, doc := renderGraphML(t, formatFixture())
	declared := map[string]graphmlKey{}
	for _, k := range doc.Keys {
		if prev, dup := declared[k.ID]; dup {
			t.Errorf("key id %q declared twice (%v and %v)", k.ID, prev, k)
		}
		declared[k.ID] = k
	}

	check := func(kind string, values []graphmlData) {
		for _, d := range values {
			k, ok := declared[d.Key]
			if !ok {
				t.Errorf("%s carries undeclared key %q", kind, d.Key)
				continue
			}
			if k.For != kind {
				t.Errorf("key %q is declared for=%q but used on a %s", d.Key, k.For, kind)
			}
			switch k.Type {
			case "int":
				// A key declared int must never carry a blank or non-numeric
				// value; readers parse on the declared type.
				if _, err := strconv.Atoi(d.Value); err != nil {
					t.Errorf("key %q is int but carries %q", d.Key, d.Value)
				}
			case "boolean":
				if d.Value != "true" && d.Value != "false" {
					t.Errorf("key %q is boolean but carries %q", d.Key, d.Value)
				}
			}
		}
	}
	for _, n := range doc.Graph.Nodes {
		check("node", n.Data)
	}
	for _, e := range doc.Graph.Edges {
		check("edge", e.Data)
	}
}

func TestGraphMLNodeAndEdgeAttributes(t *testing.T) {
	_, doc := renderGraphML(t, formatFixture())
	values := func(elemID string) map[string]string {
		out := map[string]string{}
		for _, n := range doc.Graph.Nodes {
			if n.ID == elemID {
				for _, d := range n.Data {
					out[d.Key] = d.Value
				}
			}
		}
		return out
	}

	run := values("go:function:a.go#Run")
	for key, want := range map[string]string{
		"label": "Run", "kind": "function", "language": "go", "source_file": "a.go",
		"start_line": "10", "end_line": "20", "signature": "func Run() error",
		"community": "0", "community_label": "core", "degree": "3",
	} {
		if run[key] != want {
			t.Errorf("Run.%s = %q, want %q", key, run[key], want)
		}
	}
	if _, ok := run["external"]; ok {
		t.Error("external was emitted for a node that is not external")
	}

	ext := values("go:external:#net/http")
	if ext["external"] != "true" {
		t.Errorf("external node: external = %q", ext["external"])
	}
	// Never clustered — and community 0 must not be invented for it.
	if _, ok := ext["community"]; ok {
		t.Errorf("unclustered node claims community %q", ext["community"])
	}
	if _, ok := ext["source_file"]; ok {
		t.Error("external node has no source file but emitted one")
	}

	var calls graphmlEdge
	for _, e := range doc.Graph.Edges {
		if e.Source == "go:function:a.go#Run" && e.Target == "go:function:b.go#Handle" {
			calls = e
		}
	}
	got := map[string]string{}
	for _, d := range calls.Data {
		got[d.Key] = d.Value
	}
	for key, want := range map[string]string{
		"relation": "calls", "confidence": "EXTRACTED", "site_file": "a.go", "site_line": "14",
	} {
		if got[key] != want {
			t.Errorf("edge.%s = %q, want %q", key, got[key], want)
		}
	}
}

// TestGraphMLEscapesMarkup: signatures are full of <, > and &, and a code graph
// exports them verbatim. They must survive as text rather than becoming markup.
func TestGraphMLEscapesMarkup(t *testing.T) {
	sig := `func F[T any](in <-chan T, m map[string]T) (bool, error) // a && b, "quoted"`
	g := NewGraph("/repo", []model.Node{{
		ID: "go:function:a.go#F", Label: "F", Kind: model.KindFunction,
		Signature: sig, Doc: "compares a<b & b>c", Degree: 1,
	}}, nil, nil, 1)

	out, doc := renderGraphML(t, g)
	if strings.Contains(out, "<-chan") {
		t.Error("a raw < from a signature reached the document unescaped")
	}
	got := map[string]string{}
	for _, d := range doc.Graph.Nodes[0].Data {
		got[d.Key] = d.Value
	}
	if got["signature"] != sig {
		t.Errorf("signature did not round-trip:\n got %q\nwant %q", got["signature"], sig)
	}
	if got["doc"] != "compares a<b & b>c" {
		t.Errorf("doc did not round-trip: %q", got["doc"])
	}
}

// TestGraphMLSurvivesControlCharacters: XML 1.0 has no representation for most
// control characters, so a stray one in a doc comment must not produce a
// document no parser will accept.
func TestGraphMLSurvivesControlCharacters(t *testing.T) {
	g := NewGraph("/repo", []model.Node{{
		ID: "go:function:a.go#F", Label: "F", Kind: model.KindFunction,
		Doc: "before\x00\x08after", Body: "tab\tand\nnewline", Degree: 1,
	}}, nil, nil, 1)
	if _, doc := renderGraphML(t, g); len(doc.Graph.Nodes) != 1 {
		t.Fatal("node lost")
	}
}

// TestGraphMLIsDeterministic: the file is meant to be committed and diffed, so
// two exports of the same graph have to be byte-identical regardless of the
// order the store handed things over in.
func TestGraphMLIsDeterministic(t *testing.T) {
	var first, second bytes.Buffer
	if err := WriteGraphML(&first, formatFixture(), "graphy test"); err != nil {
		t.Fatal(err)
	}
	// Same graph, assembled in a different order.
	g := formatFixture()
	reversed := make([]model.Node, len(g.Nodes))
	for i, n := range g.Nodes {
		reversed[len(g.Nodes)-1-i] = n
	}
	shuffled := NewGraph(g.Root, reversed, g.Edges, []store.Community{g.Communities[1], g.Communities[0]}, g.Files)
	if err := WriteGraphML(&second, shuffled, "graphy test"); err != nil {
		t.Fatal(err)
	}
	if first.String() != second.String() {
		t.Error("GraphML output depends on input order")
	}
}
