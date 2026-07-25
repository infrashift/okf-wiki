package cluster

import (
	"reflect"
	"testing"
)

// clique returns all intra-edges of the given node ids.
func clique(ids ...string) [][2]string {
	var edges [][2]string
	for i := range ids {
		for j := i + 1; j < len(ids); j++ {
			edges = append(edges, [2]string{ids[i], ids[j]})
		}
	}
	return edges
}

func TestLouvainTwoCliquesOneBridge(t *testing.T) {
	a := []string{"a1", "a2", "a3", "a4"}
	b := []string{"b1", "b2", "b3", "b4"}
	nodes := append(append([]string{}, a...), b...)
	edges := append(clique(a...), clique(b...)...)
	edges = append(edges, [2]string{"a1", "b1"}) // bridge

	assign, err := Louvain{Seed: 1}.Detect(nodes, edges)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	// Each clique lands in one community; the two differ.
	for _, group := range [][]string{a, b} {
		c := assign[group[0]]
		for _, id := range group[1:] {
			if assign[id] != c {
				t.Errorf("%s community = %d, want %d (assign: %v)", id, assign[id], c, assign)
			}
		}
	}
	if assign["a1"] == assign["b1"] {
		t.Errorf("cliques merged into one community: %v", assign)
	}
}

func TestLouvainIsDeterministic(t *testing.T) {
	nodes := []string{"a1", "a2", "a3", "a4", "b1", "b2", "b3", "b4", "lone"}
	edges := append(clique("a1", "a2", "a3", "a4"), clique("b1", "b2", "b3", "b4")...)
	edges = append(edges, [2]string{"a2", "b3"})

	first, err := Louvain{Seed: 1}.Detect(nodes, edges)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		again, err := Louvain{Seed: 1}.Detect(nodes, edges)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(first, again) {
			t.Fatalf("run %d differs: %v vs %v", i, again, first)
		}
	}
}

func TestLouvainEmptyGraph(t *testing.T) {
	assign, err := Louvain{Seed: 1}.Detect(nil, nil)
	if err != nil || len(assign) != 0 {
		t.Fatalf("empty graph: %v %v", assign, err)
	}
}
