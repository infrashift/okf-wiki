package cluster

import (
	"math"
	"testing"
)

// indexOf builds the sorted-id → dense-index map the detectors use.
func indexOf(ids []string) (map[string]int32, []string) {
	sorted := sortUnique(ids)
	index := make(map[string]int32, len(sorted))
	for i, id := range sorted {
		index[id] = int32(i)
	}
	return index, sorted
}

func TestNewWeightedAccumulatesMultiplicity(t *testing.T) {
	index, sorted := indexOf([]string{"a", "b", "c"})
	// (a,b) three times, once in reverse; (b,c) once; a self-loop; an unknown id.
	g := newWeighted(index, len(sorted), [][2]string{
		{"a", "b"}, {"a", "b"}, {"b", "a"},
		{"b", "c"},
		{"a", "a"},
		{"a", "zzz"},
	})

	if g.n != 3 {
		t.Fatalf("n = %d, want 3", g.n)
	}
	if got := edgeWeight(g, index["a"], index["b"]); got != 3 {
		t.Errorf("w(a,b) = %v, want 3 (multiplicity becomes weight)", got)
	}
	if got := edgeWeight(g, index["b"], index["c"]); got != 1 {
		t.Errorf("w(b,c) = %v, want 1", got)
	}
	// Self-loops and unknown endpoints are dropped, so a has exactly one neighbour.
	if nbrs, _ := g.neighbours(index["a"]); len(nbrs) != 1 {
		t.Errorf("a neighbours = %v, want just b", nbrs)
	}
	// total == 2m: 2*(3+1) = 8.
	if g.total != 8 {
		t.Errorf("total = %v, want 8", g.total)
	}
	if g.strength[index["b"]] != 4 {
		t.Errorf("strength(b) = %v, want 4", g.strength[index["b"]])
	}
}

func TestNewWeightedNeighboursAreSorted(t *testing.T) {
	ids := []string{"a", "b", "c", "d", "e"}
	index, sorted := indexOf(ids)
	// Deliberately supplied out of order.
	g := newWeighted(index, len(sorted), [][2]string{
		{"c", "a"}, {"e", "a"}, {"b", "a"}, {"d", "a"},
	})
	nbrs, _ := g.neighbours(index["a"])
	for i := 1; i < len(nbrs); i++ {
		if nbrs[i-1] >= nbrs[i] {
			t.Fatalf("neighbours of a not ascending: %v", nbrs)
		}
	}
	if len(nbrs) != 4 {
		t.Fatalf("a has %d neighbours, want 4", len(nbrs))
	}
}

func TestWeightedStrengthSumsToTotal(t *testing.T) {
	nodes, edges := plantedPartition(4, 8, 0.7, 0.05, 11)
	index, sorted := indexOf(nodes)
	g := newWeighted(index, len(sorted), edges)

	var sum float64
	for _, s := range g.strength {
		sum += s
	}
	if math.Abs(sum-g.total) > 1e-9 {
		t.Errorf("sum(strength) = %v, total = %v", sum, g.total)
	}
}

func TestAggregateCollapsesCommunities(t *testing.T) {
	// Two triangles joined by a single bridge edge.
	ids := []string{"a1", "a2", "a3", "b1", "b2", "b3"}
	index, sorted := indexOf(ids)
	edges := append(clique("a1", "a2", "a3"), clique("b1", "b2", "b3")...)
	edges = append(edges, [2]string{"a3", "b1"})
	g := newWeighted(index, len(sorted), edges)

	sizeOf := append([]float64(nil), g.strength...)
	p := singletons(g, sizeOf)
	// Force the intended two-community split.
	for _, id := range []string{"a1", "a2", "a3"} {
		p.member[index[id]] = 0
	}
	for _, id := range []string{"b1", "b2", "b3"} {
		p.member[index[id]] = 1
	}
	p.compact(g, sizeOf)

	agg, rep := g.aggregate(p)
	if agg.n != 2 {
		t.Fatalf("aggregate n = %d, want 2", agg.n)
	}
	if len(rep) != 2 {
		t.Fatalf("rep = %v, want 2 representatives", rep)
	}
	// Each triangle contributes 3 internal edges.
	for v := 0; v < 2; v++ {
		if agg.selfWt[v] != 3 {
			t.Errorf("selfWt[%d] = %v, want 3", v, agg.selfWt[v])
		}
	}
	// The single bridge survives as one cross edge of weight 1.
	if got := edgeWeight(agg, 0, 1); got != 1 {
		t.Errorf("bridge weight = %v, want 1", got)
	}
	// Aggregation preserves total weight: 2m is unchanged by collapsing.
	if agg.total != g.total {
		t.Errorf("aggregate total = %v, want %v (unchanged)", agg.total, g.total)
	}
}

func TestCompactRenumbersBySmallestMember(t *testing.T) {
	ids := []string{"a", "b", "c", "d"}
	index, sorted := indexOf(ids)
	g := newWeighted(index, len(sorted), [][2]string{{"a", "b"}, {"c", "d"}})
	sizeOf := append([]float64(nil), g.strength...)
	p := singletons(g, sizeOf)

	// Sparse, out-of-order community labels.
	p.member[index["a"]] = 7
	p.member[index["b"]] = 7
	p.member[index["c"]] = 3
	p.member[index["d"]] = 3
	p.compact(g, sizeOf)

	if p.nComm != 2 {
		t.Fatalf("nComm = %d, want 2", p.nComm)
	}
	// "a" is the smallest index, so its community renumbers to 0.
	if p.member[index["a"]] != 0 || p.member[index["b"]] != 0 {
		t.Errorf("a/b = %d/%d, want 0/0", p.member[index["a"]], p.member[index["b"]])
	}
	if p.member[index["c"]] != 1 || p.member[index["d"]] != 1 {
		t.Errorf("c/d = %d/%d, want 1/1", p.member[index["c"]], p.member[index["d"]])
	}
	for c := 0; c < p.nComm; c++ {
		if p.count[c] != 2 {
			t.Errorf("count[%d] = %d, want 2", c, p.count[c])
		}
	}
}

// edgeWeight returns w(a,b), or 0 when they are not adjacent.
func edgeWeight(g *weighted, a, b int32) float64 {
	nbrs, wts := g.neighbours(a)
	for i, u := range nbrs {
		if u == b {
			return wts[i]
		}
	}
	return 0
}
