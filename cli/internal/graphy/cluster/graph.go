package cluster

import (
	"slices"
)

// weighted is an immutable, undirected, weighted graph in CSR (compressed
// sparse row) form. Neighbour lists are sorted ascending by index, which is
// what makes every traversal over this structure deterministic — unlike a
// map-of-maps, which would leak Go's randomized map iteration into the
// algorithm's decisions.
//
// At level 0 every node is an original graph node. After aggregation a node
// represents a whole community, and selfWt carries the weight of the edges
// that were internal to it.
type weighted struct {
	n        int
	offset   []int32   // len n+1; neighbours of v are adj[offset[v]:offset[v+1]]
	adj      []int32   // len 2m
	wt       []float64 // len 2m, parallel to adj
	selfWt   []float64 // len n; internal weight of an aggregate node (0 at level 0)
	strength []float64 // len n; sum of incident wt + 2*selfWt
	total    float64   // sum(strength) == 2m
}

// neighbours returns v's adjacency slice and the parallel weight slice.
func (g *weighted) neighbours(v int32) ([]int32, []float64) {
	lo, hi := g.offset[v], g.offset[v+1]
	return g.adj[lo:hi], g.wt[lo:hi]
}

// pairKey packs an undirected endpoint pair into a single sortable uint64.
// The lower index always occupies the high half, so (a,b) and (b,a) collide
// on purpose.
func pairKey(a, b int32) uint64 {
	if a > b {
		a, b = b, a
	}
	return uint64(uint32(a))<<32 | uint64(uint32(b))
}

func unpairKey(k uint64) (int32, int32) {
	return int32(uint32(k >> 32)), int32(uint32(k))
}

// buildCSR turns an accumulated undirected edge map into a weighted graph.
// The keys are extracted and sorted before the fill pass: never iterate the
// accumulation map to build the CSR, or neighbour order becomes random and
// the whole determinism story collapses.
func buildCSR(n int, acc map[uint64]float64, selfWt []float64) *weighted {
	keys := make([]uint64, 0, len(acc))
	for k := range acc {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	g := &weighted{
		n:        n,
		offset:   make([]int32, n+1),
		selfWt:   selfWt,
		strength: make([]float64, n),
	}
	if g.selfWt == nil {
		g.selfWt = make([]float64, n)
	}

	// Counting pass: degree of each endpoint.
	counts := make([]int32, n)
	for _, k := range keys {
		a, b := unpairKey(k)
		counts[a]++
		counts[b]++
	}
	for i := 0; i < n; i++ {
		g.offset[i+1] = g.offset[i] + counts[i]
	}
	total := g.offset[n]
	g.adj = make([]int32, total)
	g.wt = make([]float64, total)

	// Fill pass. Keys are sorted by (min, max), so walking them in order and
	// appending at a per-node cursor leaves each neighbour list ascending:
	// for node v, the keys where v is the low endpoint arrive in ascending
	// high-endpoint order, and are all greater than v; the keys where v is
	// the high endpoint arrive earlier and are all less than v.
	cursor := make([]int32, n)
	copy(cursor, g.offset[:n])
	for _, k := range keys {
		a, b := unpairKey(k)
		w := acc[k]
		g.adj[cursor[a]], g.wt[cursor[a]] = b, w
		cursor[a]++
		g.adj[cursor[b]], g.wt[cursor[b]] = a, w
		cursor[b]++
	}

	for v := 0; v < n; v++ {
		s := 2 * g.selfWt[v]
		for _, w := range g.wt[g.offset[v]:g.offset[v+1]] {
			s += w
		}
		g.strength[v] = s
		g.total += s
	}
	return g
}

// newWeighted builds the level-0 graph from the Detector inputs. Duplicate
// pairs — in either direction — accumulate into a single weighted edge, so
// edge multiplicity becomes edge weight. Self-loops and pairs naming ids
// absent from index are dropped, matching Louvain's undirected projection.
func newWeighted(index map[string]int32, n int, edges [][2]string) *weighted {
	acc := make(map[uint64]float64, len(edges))
	for _, e := range edges {
		a, aok := index[e[0]]
		b, bok := index[e[1]]
		if !aok || !bok || a == b {
			continue
		}
		acc[pairKey(a, b)]++
	}
	return buildCSR(n, acc, nil)
}

// partition is a flat membership array plus the per-community aggregates the
// quality function needs. Community ids are dense in [0, nComm).
type partition struct {
	member []int32   // len n; community index per node
	size   []float64 // per community: sum of member node sizes
	tot    []float64 // per community: sum of member strengths
	count  []int32   // per community: member node count
	nComm  int
}

// singletons puts every node in its own community, numbered by node index.
func singletons(g *weighted, sizeOf []float64) *partition {
	p := &partition{
		member: make([]int32, g.n),
		size:   make([]float64, g.n),
		tot:    make([]float64, g.n),
		count:  make([]int32, g.n),
		nComm:  g.n,
	}
	for v := 0; v < g.n; v++ {
		p.member[v] = int32(v)
		p.size[v] = sizeOf[v]
		p.tot[v] = g.strength[v]
		p.count[v] = 1
	}
	return p
}

// remove takes v out of its community, updating the aggregates.
func (p *partition) remove(v int32, sizeV, strengthV float64) {
	c := p.member[v]
	p.size[c] -= sizeV
	p.tot[c] -= strengthV
	p.count[c]--
	p.member[v] = -1
}

// insert places v into community c, updating the aggregates.
func (p *partition) insert(v, c int32, sizeV, strengthV float64) {
	p.member[v] = c
	p.size[c] += sizeV
	p.tot[c] += strengthV
	p.count[c]++
}

// compact renumbers communities to 0..k-1 by first appearance in an ascending
// 0..n-1 scan (so, by smallest member), drops empties, and recomputes the
// aggregates from the membership array.
func (p *partition) compact(g *weighted, sizeOf []float64) {
	remap := make(map[int32]int32, p.nComm)
	next := int32(0)
	for v := range p.member {
		c := p.member[v]
		if _, seen := remap[c]; !seen {
			remap[c] = next
			next++
		}
	}
	size := make([]float64, next)
	tot := make([]float64, next)
	count := make([]int32, next)
	for v := range p.member {
		nc := remap[p.member[v]]
		p.member[v] = nc
		size[nc] += sizeOf[v]
		tot[nc] += g.strength[v]
		count[nc]++
	}
	p.size, p.tot, p.count, p.nComm = size, tot, count, int(next)
}

// aggregate collapses each community of part into a single node. It returns
// the aggregate graph and, for each aggregate node, the index of one
// representative original node (the smallest), which the caller uses to carry
// the coarser partition forward.
//
// Aggregate node indices are assigned by first appearance in an ascending
// 0..n-1 scan, so the aggregate graph's node order still respects "smallest
// original member first" and stays deterministic.
func (g *weighted) aggregate(part *partition) (*weighted, []int32) {
	commToAgg := make(map[int32]int32, part.nComm)
	var rep []int32
	for v := int32(0); v < int32(g.n); v++ {
		c := part.member[v]
		if _, seen := commToAgg[c]; !seen {
			commToAgg[c] = int32(len(rep))
			rep = append(rep, v)
		}
	}
	nAgg := len(rep)

	// Map every original node to its aggregate node once, up front.
	aggOf := make([]int32, g.n)
	for v := int32(0); v < int32(g.n); v++ {
		aggOf[v] = commToAgg[part.member[v]]
	}

	acc := make(map[uint64]float64)
	selfWt := make([]float64, nAgg)
	for v := int32(0); v < int32(g.n); v++ {
		av := aggOf[v]
		selfWt[av] += g.selfWt[v]
		nbrs, wts := g.neighbours(v)
		for i, u := range nbrs {
			au := aggOf[u]
			if au == av {
				// Each internal edge is seen twice (once from each endpoint),
				// and selfWt counts an edge once, so halve it.
				selfWt[av] += wts[i] / 2
				continue
			}
			if v < u { // count each cross edge once
				acc[pairKey(av, au)] += wts[i]
			}
		}
	}
	return buildCSR(nAgg, acc, selfWt), rep
}
