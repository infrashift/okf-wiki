package graph

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// Graph output is a pure function of the files, so the same bundle must give
// the same answer on every run. Each test rebuilds its graph many times: Go
// randomises map iteration per range statement, so a dependence on it shows up
// as a disagreement between rebuilds.
const rebuilds = 50

func ids(ns []*Node) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = n.ID
	}
	return out
}

// hubGraph is a hub with more neighbours — children, backlinks — than a
// query's limit admits, so which of them a capped query returns depends on the
// order the neighbours are visited in.
func hubGraph() *Graph {
	g := New()
	g.AddNode(&Node{ID: "hub", Title: "Hub"})
	for i := 0; i < 12; i++ {
		g.AddNode(&Node{ID: fmt.Sprintf("child%02d", i), Title: "Child", Parent: "hub"})
		g.AddNode(&Node{ID: fmt.Sprintf("ref%02d", i), Title: "Ref", Links: []string{"hub"}})
	}
	g.BuildRelationships()
	return g
}

func TestCappedClusterIsTheSameEveryRun(t *testing.T) {
	want := ids(hubGraph().FindConnected("hub", 6, 0))
	// Children before backlinks, each in id order.
	if exp := []string{"hub", "child00", "child01", "child02", "child03", "child04"}; !reflect.DeepEqual(want, exp) {
		t.Errorf("cluster = %v, want %v", want, exp)
	}
	for i := 0; i < rebuilds; i++ {
		if got := ids(hubGraph().FindConnected("hub", 6, 0)); !reflect.DeepEqual(got, want) {
			t.Fatalf("rebuild %d: cluster = %v, first run gave %v", i, got, want)
		}
	}
}

func TestBacklinkTraversalIsInIDOrder(t *testing.T) {
	g := New()
	g.AddNode(&Node{ID: "hub", Title: "Hub"})
	for i := 0; i < 12; i++ {
		g.AddNode(&Node{ID: fmt.Sprintf("ref%02d", i), Title: "Ref", Links: []string{"hub"}})
	}
	for i := 0; i < rebuilds; i++ {
		got := ids(g.FindConnected("hub", 4, 0))
		if want := []string{"hub", "ref00", "ref01", "ref02"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("rebuild %d: cluster = %v, want %v", i, got, want)
		}
		g.BuildRelationships()
	}
}

// With no start given, the graph starts at the most-connected concept; among
// equals it must pick the same one every time.
func TestUnanchoredGraphPicksTheSameStartEveryRun(t *testing.T) {
	build := func() *Graph {
		g := New()
		for _, p := range []string{"m", "c", "q", "a", "x"} {
			g.AddNode(&Node{ID: p, Title: p, Links: []string{p + "-leaf"}})
			g.AddNode(&Node{ID: p + "-leaf", Title: p + " leaf"})
		}
		return g
	}
	for i := 0; i < rebuilds; i++ {
		if got := build().FindAllConnected(10, 0); got[0].ID != "a" {
			t.Fatalf("rebuild %d: started at %q, want the lowest id among the tied, %q", i, got[0].ID, "a")
		}
	}
}

func TestAllNodesIsInIDOrder(t *testing.T) {
	for i := 0; i < rebuilds; i++ {
		if got := ids(buildTestGraph().AllNodes()); !reflect.DeepEqual(got, []string{"A", "B", "C", "D", "E", "F"}) {
			t.Fatalf("AllNodes = %v", got)
		}
	}
}

// Two concepts linking to each other make each the other's neighbour twice —
// once forward, once reverse. The tree must label the edge the same way every
// run: forward, since the link lives in the tree parent. Twenty of them, because
// sort.Slice is an insertion sort, and so stable, below twelve elements.
func TestASCIITreeLabelsAMutualLinkTheSameEveryRun(t *testing.T) {
	g := New()
	var hubLinks []string
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("b%02d", i)
		hubLinks = append(hubLinks, id)
		g.AddNode(&Node{ID: id, Title: "B", Links: []string{"a"}})
	}
	g.AddNode(&Node{ID: "a", Title: "A", Links: hubLinks})
	nodes := g.FindConnected("a", 30, 0)
	edges := g.GetEdges(nodes)
	for i := 0; i < rebuilds; i++ {
		out := GenerateASCIITree(nodes, edges, "a")
		if n := strings.Count(out, " B → [link]"); n != 20 {
			t.Fatalf("run %d: %d of 20 mutual links labelled forward:\n%s", i, n, out)
		}
	}
}
