package cluster

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"testing"
)

func TestLeidenTwoCliquesOneBridge(t *testing.T) {
	a := []string{"a1", "a2", "a3", "a4"}
	b := []string{"b1", "b2", "b3", "b4"}
	nodes := append(append([]string{}, a...), b...)
	edges := append(clique(a...), clique(b...)...)
	edges = append(edges, [2]string{"a1", "b1"}) // bridge

	assign, err := Leiden{Seed: 1}.Detect(nodes, edges)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
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

func TestLeidenIsDeterministic(t *testing.T) {
	t.Run("small", func(t *testing.T) {
		nodes := []string{"a1", "a2", "a3", "a4", "b1", "b2", "b3", "b4", "lone"}
		edges := append(clique("a1", "a2", "a3", "a4"), clique("b1", "b2", "b3", "b4")...)
		edges = append(edges, [2]string{"a2", "b3"})
		assertDeterministic(t, nodes, edges)
	})
	// The big graph is the one that actually exercises multi-level aggregation
	// and the randomized refinement sampling; the 9-node fixture above would
	// pass even with the RNG plumbing broken.
	t.Run("planted", func(t *testing.T) {
		nodes, edges := plantedPartition(6, 50, 0.35, 0.01, 42)
		assertDeterministic(t, nodes, edges)
	})
}

func assertDeterministic(t *testing.T, nodes []string, edges [][2]string) {
	t.Helper()
	first, err := Leiden{Seed: 1}.Detect(nodes, edges)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		again, err := Leiden{Seed: 1}.Detect(nodes, edges)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(first, again) {
			t.Fatalf("run %d differs from the first", i)
		}
	}
}

func TestLeidenEmptyGraph(t *testing.T) {
	assign, err := Leiden{Seed: 1}.Detect(nil, nil)
	if err != nil || len(assign) != 0 {
		t.Fatalf("empty graph: %v %v", assign, err)
	}
}

// TestLeidenCoversEveryNode protects store.SetCommunities, which leaves nodes
// missing from the assignment stamped with their previous community — so a
// detector that drops isolated nodes silently preserves stale data.
func TestLeidenCoversEveryNode(t *testing.T) {
	nodes := []string{"a1", "a2", "a3", "lone1", "lone2", "a1"} // note the duplicate
	edges := clique("a1", "a2", "a3")
	edges = append(edges, [2]string{"a1", "ghost"}) // endpoint not in nodes

	assign, err := Leiden{Seed: 1}.Detect(nodes, edges)
	if err != nil {
		t.Fatal(err)
	}
	if len(assign) != 5 {
		t.Fatalf("assign covers %d ids, want 5 distinct: %v", len(assign), assign)
	}
	for _, id := range []string{"a1", "a2", "a3", "lone1", "lone2"} {
		if _, ok := assign[id]; !ok {
			t.Errorf("%s missing from the assignment", id)
		}
	}
	if _, ok := assign["ghost"]; ok {
		t.Error("unknown endpoint leaked into the assignment")
	}
	if assign["lone1"] == assign["lone2"] {
		t.Errorf("isolated nodes share a community: %v", assign)
	}
	if assign["lone1"] == assign["a1"] {
		t.Errorf("isolated node joined the clique: %v", assign)
	}
}

// TestLeidenCommunitiesAreConnected is the guarantee test: it fails loudly for
// every way the implementation can silently degrade into Louvain (skipping
// refinement, aggregating the unrefined partition, restarting from singletons,
// or dropping a well-connectedness check).
func TestLeidenCommunitiesAreConnected(t *testing.T) {
	type graph struct {
		name  string
		nodes []string
		edges [][2]string
	}
	var cases []graph

	a := []string{"a1", "a2", "a3", "a4"}
	b := []string{"b1", "b2", "b3", "b4"}
	twoCliques := append(clique(a...), clique(b...)...)
	twoCliques = append(twoCliques, [2]string{"a1", "b1"})
	cases = append(cases, graph{"two-cliques", append(append([]string{}, a...), b...), twoCliques})

	hn, he := hubRing(6, 5)
	cases = append(cases, graph{"hub-ring", hn, he})

	for _, seed := range []uint64{1, 2, 3, 4, 5} {
		n, e := plantedPartition(5, 20, 0.4, 0.03, seed)
		cases = append(cases, graph{fmt.Sprintf("planted-seed-%d", seed), n, e})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assign, err := Leiden{Seed: 1}.Detect(tc.nodes, tc.edges)
			if err != nil {
				t.Fatal(err)
			}
			if bad, ok := connectedWithin(assign, tc.edges); !ok {
				t.Errorf("community %d is internally disconnected — refinement is not doing its job", bad)
			}
		})
	}
}

// TestRefineSplitsDisconnectedCommunity is the white-box test of Leiden's
// defining contract, and the one that actually fails if refinement is removed
// or weakened.
//
// It hands refine() a partition that is internally disconnected on purpose —
// two triangles with no edge between them, declared to be one community, which
// is precisely the state Louvain can reach and cannot repair. Refinement must
// never leave nodes from both triangles in one sub-community.
//
// (An end-to-end differential test against gonum's Louvain was tried first: a
// sweep over ~5000 hub-ring, connected-caveman and planted-partition graphs ×
// seeds never caught gonum returning a disconnected community, so such a test
// would have been a permanent skip. This tests the invariant directly instead.)
func TestRefineSplitsDisconnectedCommunity(t *testing.T) {
	left := []string{"a0", "a1", "a2"}
	right := []string{"b0", "b1", "b2"}
	ids := append(append([]string{}, left...), right...)
	edges := append(clique(left...), clique(right...)...) // deliberately no bridge

	index, sorted := indexOf(ids)
	g := newWeighted(index, len(sorted), edges)
	st := &leidenState{resolution: DefaultResolution, theta: defaultRandomness}
	sizeOf := st.sizes(g)

	// One community holding both disconnected triangles.
	part := singletons(g, sizeOf)
	for v := range part.member {
		part.member[v] = 0
	}
	part.compact(g, sizeOf)
	if part.nComm != 1 {
		t.Fatalf("setup: nComm = %d, want 1", part.nComm)
	}

	refined := st.refine(g, part, sizeOf, rand.New(rand.NewPCG(1, 1)))

	// No refined community may straddle the two triangles.
	side := map[int32]string{}
	for _, id := range left {
		side[index[id]] = "left"
	}
	for _, id := range right {
		side[index[id]] = "right"
	}
	seen := map[int32]string{}
	for v, c := range refined.member {
		s := side[int32(v)]
		if prev, ok := seen[c]; ok && prev != s {
			t.Fatalf("refined community %d holds both triangles — refinement is not splitting "+
				"disconnected communities", c)
		}
		seen[c] = s
	}

	// And every refined community must itself be connected.
	assign := map[string]int{}
	for v, id := range sorted {
		assign[id] = int(refined.member[v])
	}
	if bad, ok := connectedWithin(assign, edges); !ok {
		t.Errorf("refined community %d is internally disconnected", bad)
	}
}

// hubRing builds the classic stranded-cut-vertex witness: `groups` cliques of
// `per` nodes, chained in a ring by single edges, plus one hub joined to two
// nodes of every clique. Louvain can place the hub with one clique and then
// migrate that clique's members away, leaving the hub's community in pieces.
func hubRing(groups, per int) ([]string, [][2]string) {
	var nodes []string
	var edges [][2]string
	for g := 0; g < groups; g++ {
		var members []string
		for i := 0; i < per; i++ {
			members = append(members, fmt.Sprintf("c%d_%d", g, i))
		}
		nodes = append(nodes, members...)
		edges = append(edges, clique(members...)...)
	}
	nodes = append(nodes, "hub")
	for g := 0; g < groups; g++ {
		edges = append(edges,
			[2]string{"hub", fmt.Sprintf("c%d_0", g)},
			[2]string{"hub", fmt.Sprintf("c%d_1", g)},
		)
		next := (g + 1) % groups
		edges = append(edges, [2]string{
			fmt.Sprintf("c%d_%d", g, per-1),
			fmt.Sprintf("c%d_%d", next, per-2),
		})
	}
	return nodes, edges
}

// TestLevelAggregatesTheRefinedPartition guards the single easiest way to
// silently turn this implementation back into Louvain: aggregating the coarse
// partition instead of the refined one. That swap is invisible in Detect's
// output on almost every graph — a sweep over thousands of hub-ring,
// caveman and planted graphs could not distinguish them — so it has to be
// asserted structurally, here.
func TestLevelAggregatesTheRefinedPartition(t *testing.T) {
	a := []string{"a1", "a2", "a3", "a4"}
	b := []string{"b1", "b2", "b3", "b4"}
	ids := append(append([]string{}, a...), b...)
	edges := append(clique(a...), clique(b...)...)
	edges = append(edges, [2]string{"a1", "b1"})

	index, sorted := indexOf(ids)
	g := newWeighted(index, len(sorted), edges)
	st := &leidenState{resolution: DefaultResolution, theta: defaultRandomness}
	sizeOf := st.sizes(g)
	part := singletons(g, sizeOf)

	res := st.level(g, part, sizeOf, rand.New(rand.NewPCG(1, 1)))
	if res.done {
		t.Fatal("level reported done on the first pass; expected an aggregation")
	}
	if res.refined == nil || res.graph == nil {
		t.Fatal("level returned no refinement or no aggregate graph")
	}
	// The aggregate graph must have exactly one node per REFINED community.
	if res.graph.n != res.refined.nComm {
		t.Errorf("aggregate graph has %d nodes but the refined partition has %d communities "+
			"— aggregation is not running on the refined partition",
			res.graph.n, res.refined.nComm)
	}
	// A refinement is by definition no coarser than the partition it refines.
	if res.refined.nComm < part.nComm {
		t.Errorf("refined into %d communities, coarser than part's %d", res.refined.nComm, part.nComm)
	}
	// Every refined community must lie wholly inside one coarse community.
	owner := map[int32]int32{}
	for v, rc := range res.refined.member {
		pc := part.member[v]
		if prev, ok := owner[rc]; ok && prev != pc {
			t.Fatalf("refined community %d straddles coarse communities %d and %d", rc, prev, pc)
		}
		owner[rc] = pc
	}
}

func TestLeidenResolution(t *testing.T) {
	// Four cliques chained by single bridges: coarse resolution should merge
	// them, fine resolution should split them further.
	var nodes []string
	var edges [][2]string
	for g := 0; g < 4; g++ {
		var members []string
		for i := 0; i < 6; i++ {
			members = append(members, fmt.Sprintf("g%d_%d", g, i))
		}
		nodes = append(nodes, members...)
		edges = append(edges, clique(members...)...)
		if g > 0 {
			edges = append(edges, [2]string{fmt.Sprintf("g%d_0", g-1), fmt.Sprintf("g%d_0", g)})
		}
	}

	count := func(res float64) int {
		assign, err := Leiden{Seed: 1, Resolution: res}.Detect(nodes, edges)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[int]bool{}
		for _, c := range assign {
			seen[c] = true
		}
		return len(seen)
	}

	coarse, base, fine := count(0.2), count(1.0), count(4.0)
	if coarse > base {
		t.Errorf("resolution 0.2 gave %d communities, 1.0 gave %d — coarse should not exceed base", coarse, base)
	}
	if fine < base {
		t.Errorf("resolution 4.0 gave %d communities, 1.0 gave %d — fine should not be coarser", fine, base)
	}
}

// TestLeidenQualityNotWorseThanLouvain catches a half-degraded implementation:
// a broken refinement usually loses modularity rather than changing the
// partition in some harmless way.
func TestLeidenQualityNotWorseThanLouvain(t *testing.T) {
	for _, seed := range []uint64{7, 8, 9} {
		nodes, edges := plantedPartition(5, 24, 0.4, 0.02, seed)
		leiden, err := Leiden{Seed: 1}.Detect(nodes, edges)
		if err != nil {
			t.Fatal(err)
		}
		louvain, err := Louvain{Seed: 1}.Detect(nodes, edges)
		if err != nil {
			t.Fatal(err)
		}
		ql := modularityOf(nodes, edges, leiden, 1.0)
		qv := modularityOf(nodes, edges, louvain, 1.0)
		if ql < qv-1e-9 {
			t.Errorf("seed %d: leiden modularity %.6f < louvain %.6f", seed, ql, qv)
		}
	}
}
