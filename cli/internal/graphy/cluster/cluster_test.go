package cluster

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
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

// connectedWithin reports whether every community in assign induces a
// connected subgraph. On failure it returns the id of one offending community.
// This is the property Leiden guarantees and Louvain does not.
func connectedWithin(assign map[string]int, edges [][2]string) (int, bool) {
	members := map[int][]string{}
	for id, c := range assign {
		members[c] = append(members[c], id)
	}
	adj := map[string][]string{}
	for _, e := range edges {
		if assign[e[0]] == assign[e[1]] {
			adj[e[0]] = append(adj[e[0]], e[1])
			adj[e[1]] = append(adj[e[1]], e[0])
		}
	}
	comms := make([]int, 0, len(members))
	for c := range members {
		comms = append(comms, c)
	}
	sort.Ints(comms) // deterministic failure reporting
	for _, c := range comms {
		group := members[c]
		if len(group) < 2 {
			continue
		}
		sort.Strings(group)
		seen := map[string]bool{group[0]: true}
		stack := []string{group[0]}
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, m := range adj[n] {
				if !seen[m] {
					seen[m] = true
					stack = append(stack, m)
				}
			}
		}
		if len(seen) != len(group) {
			return c, false
		}
	}
	return 0, true
}

// plantedPartition generates a graph with `groups` planted communities of
// `perGroup` nodes each: an intra-group edge with probability pIn, an
// inter-group edge with probability pOut. Deterministic given seed.
func plantedPartition(groups, perGroup int, pIn, pOut float64, seed uint64) ([]string, [][2]string) {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b9))
	var nodes []string
	for g := 0; g < groups; g++ {
		for i := 0; i < perGroup; i++ {
			nodes = append(nodes, fmt.Sprintf("g%02d_n%03d", g, i))
		}
	}
	var edges [][2]string
	for i := range nodes {
		for j := i + 1; j < len(nodes); j++ {
			p := pOut
			if i/perGroup == j/perGroup {
				p = pIn
			}
			if rng.Float64() < p {
				edges = append(edges, [2]string{nodes[i], nodes[j]})
			}
		}
	}
	return nodes, edges
}

// modularityOf scores an assignment, treating repeated pairs as edge weight —
// the same undirected weighted projection the detectors build.
func modularityOf(nodes []string, edges [][2]string, assign map[string]int, gamma float64) float64 {
	known := map[string]bool{}
	for _, n := range nodes {
		known[n] = true
	}
	strength := map[string]float64{}
	internal := map[int]float64{}
	var total float64
	for _, e := range edges {
		a, b := e[0], e[1]
		if !known[a] || !known[b] || a == b {
			continue
		}
		strength[a]++
		strength[b]++
		total += 2
		if assign[a] == assign[b] {
			internal[assign[a]] += 2
		}
	}
	if total == 0 {
		return 0
	}
	tot := map[int]float64{}
	for n, s := range strength {
		tot[assign[n]] += s
	}
	q := 0.0
	for c, in := range internal {
		q += in / total
		_ = c
	}
	for c, s := range tot {
		q -= gamma * (s / total) * (s / total)
		_ = c
	}
	return q
}

func TestSortUniqueDedupes(t *testing.T) {
	got := sortUnique([]string{"b", "a", "b", "c", "a"})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("sortUnique = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sortUnique = %v, want %v", got, want)
		}
	}
}

func TestRenumberOrdersBySmallestMember(t *testing.T) {
	sorted := []string{"a", "b", "c", "d"}
	// Labels chosen so the raw ids do NOT match the expected output order.
	assign := renumber(sorted, []int32{9, 4, 9, 4})
	if assign["a"] != 0 || assign["c"] != 0 {
		t.Errorf("a/c = %d/%d, want 0/0 (community of the smallest member)", assign["a"], assign["c"])
	}
	if assign["b"] != 1 || assign["d"] != 1 {
		t.Errorf("b/d = %d/%d, want 1/1", assign["b"], assign["d"])
	}
}

func TestParseAlgorithm(t *testing.T) {
	for _, tc := range []struct {
		in      string
		want    Algorithm
		wantErr bool
	}{
		{"", AlgoLeiden, false},
		{"leiden", AlgoLeiden, false},
		{"LOUVAIN", AlgoLouvain, false},
		{"  louvain  ", AlgoLouvain, false},
		{"leidan", "", true},
	} {
		got, err := ParseAlgorithm(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseAlgorithm(%q) = %v, want error", tc.in, got)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("ParseAlgorithm(%q) = %v, %v; want %v", tc.in, got, err, tc.want)
		}
	}
}

func TestNewDetector(t *testing.T) {
	d, err := New("", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	l, ok := d.(Leiden)
	if !ok {
		t.Fatalf("default detector = %T, want Leiden", d)
	}
	if l.Resolution != DefaultResolution || l.Seed != DefaultSeed {
		t.Errorf("defaults not applied: %+v", l)
	}

	d, err = New(AlgoLouvain, 2, 7)
	if err != nil {
		t.Fatal(err)
	}
	if lv, ok := d.(Louvain); !ok || lv.Resolution != 2 || lv.Seed != 7 {
		t.Errorf("louvain detector = %#v", d)
	}

	for _, bad := range []float64{-1, math.NaN(), math.Inf(1)} {
		if _, err := New(AlgoLeiden, bad, 1); err == nil {
			t.Errorf("New(resolution=%v) = nil error, want rejection", bad)
		}
	}
	if _, err := New("nope", 1, 1); err == nil {
		t.Error("New with unknown algorithm: want error")
	}
}
