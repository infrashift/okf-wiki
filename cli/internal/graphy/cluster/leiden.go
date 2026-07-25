package cluster

import (
	"math"
	"math/rand/v2"
	"slices"
)

// Quality selects the objective the algorithm optimizes.
type Quality string

const (
	// QualityModularity is Newman modularity with a resolution parameter
	// (equivalently, leidenalg's RBConfiguration). Node size is the node's
	// strength. This is the default and matches Louvain's semantics, so the
	// resolution parameter means the same thing for both detectors.
	QualityModularity Quality = "modularity"
	// QualityCPM is the Constant Potts Model: node size is 1, and the
	// resolution is an absolute edge-density threshold rather than a relative
	// one. Not surfaced on the CLI — at resolution 1 on a sparse code graph it
	// degenerates to near-singletons, which would wreck community labelling,
	// the report, and the okfw export's two-member filter.
	QualityCPM Quality = "cpm"
)

// Leiden runs the Leiden algorithm (Traag, Waltman & van Eck 2019) over the
// undirected weighted projection of the graph. Unlike Louvain it guarantees
// that every returned community is internally connected, which matters here
// because communities become subsystem concepts in the exported bundle.
//
// Repeated pairs in edges accumulate as edge weight. Assignments are
// reproducible for a given Seed on a given platform.
type Leiden struct {
	Resolution float64 // 0 → DefaultResolution
	Seed       uint64  // 0 → DefaultSeed
	Randomness float64 // θ for refinement merge sampling; 0 → 0.01
	Quality    Quality // "" → QualityModularity
	MaxLevels  int     // 0 → 32; a safety bound, convergence is typically < 10
}

// defaultRandomness is leidenalg's default θ: near-greedy, but genuinely
// randomized so refinement can escape a locally-attractive merge.
const defaultRandomness = 0.01

// epsilon guards float comparisons so borderline noise cannot flip a branch
// and break determinism.
const epsilon = 1e-12

// maxLevels bounds the aggregation loop. Real graphs converge in well under
// ten levels; this only exists so a pathological input cannot spin forever.
const maxLevels = 32

// Detect implements Detector. Community ids are renumbered deterministically:
// communities are ordered by their lexicographically-smallest member.
func (l Leiden) Detect(nodeIDs []string, edges [][2]string) (map[string]int, error) {
	sorted := sortUnique(nodeIDs)
	if len(sorted) == 0 {
		return map[string]int{}, nil
	}

	index := make(map[string]int32, len(sorted))
	for i, id := range sorted {
		index[id] = int32(i)
	}
	g := newWeighted(index, len(sorted), edges)

	resolution := l.Resolution
	if resolution == 0 {
		resolution = DefaultResolution
	}
	seed := l.Seed
	if seed == 0 {
		seed = DefaultSeed
	}
	theta := l.Randomness
	if theta <= 0 {
		theta = defaultRandomness
	}
	levels := l.MaxLevels
	if levels <= 0 {
		levels = maxLevels
	}
	st := &leidenState{quality: l.Quality, resolution: resolution, theta: theta}
	rng := rand.New(rand.NewPCG(seed, seed))

	// mapping tracks, for each ORIGINAL node, the index of the current-level
	// node it now lives inside.
	mapping := make([]int32, g.n)
	for i := range mapping {
		mapping[i] = int32(i)
	}

	sizeOf := st.sizes(g)
	part := singletons(g, sizeOf)

	for level := 0; level < levels; level++ {
		res := st.level(g, part, sizeOf, rng)
		if res.done {
			break
		}
		// refined.member doubles as the level's node→aggregate-node map:
		// compact() numbers refined communities by first appearance in an
		// ascending scan, and aggregate() assigns aggregate indices the same
		// way, so the two numberings coincide.
		for i := range mapping {
			mapping[i] = res.refined.member[mapping[i]]
		}
		g, part = res.graph, res.part
		sizeOf = st.sizes(g)
	}

	member := make([]int32, len(sorted))
	for i := range member {
		member[i] = part.member[mapping[i]]
	}
	return renumber(sorted, member), nil
}

// levelResult is one aggregation level's output. done means the partition has
// stopped changing and the caller should stop.
type levelResult struct {
	graph   *weighted  // the aggregate graph for the next level
	part    *partition // the next level's starting partition
	refined *partition // this level's refinement; also the node→aggregate map
	done    bool
}

// level runs one Leiden level: local moving, refinement, aggregation. It
// mutates part in place during local moving.
//
// This is a named step rather than an inline loop body so that the wiring
// between its three phases is unit-testable — swapping refined for part in the
// aggregate call below is the single easiest way to silently turn this
// algorithm back into Louvain, and it is not detectable from Detect's output
// on most graphs. See TestLevelAggregatesTheRefinedPartition.
func (st *leidenState) level(g *weighted, part *partition, sizeOf []float64, rng *rand.Rand) levelResult {
	moved := st.fastLocalMove(g, part, sizeOf, rng)
	part.compact(g, sizeOf)
	if part.nComm == g.n {
		// Every node is its own community: nothing left to aggregate.
		return levelResult{done: true}
	}

	// LEIDEN: the refinement pass is what separates Leiden from Louvain.
	// Deleting it forfeits the connectivity guarantee.
	refined := st.refine(g, part, sizeOf, rng)

	if !moved && refined.nComm == g.n {
		return levelResult{done: true}
	}

	// LEIDEN: aggregate on the REFINED partition, not on part.
	agg, rep := g.aggregate(refined)

	// LEIDEN: carry the coarse partition forward instead of restarting from
	// singletons — each refined community resumes inside the community its
	// representative belonged to.
	coarse := st.project(part, agg, rep)

	return levelResult{graph: agg, part: coarse, refined: refined}
}

// leidenState carries the quality-function parameters through the passes.
type leidenState struct {
	quality    Quality
	resolution float64
	theta      float64
}

// sizes returns the per-node "size" the quality function uses: node strength
// for modularity, 1 for CPM.
func (st *leidenState) sizes(g *weighted) []float64 {
	out := make([]float64, g.n)
	if st.quality == QualityCPM {
		for i := range out {
			out[i] = 1
		}
		return out
	}
	copy(out, g.strength)
	return out
}

// gamma is the resolution scaled into the units the move gain expects:
// γ/2m for modularity, γ for CPM.
func (st *leidenState) gamma(g *weighted) float64 {
	if st.quality == QualityCPM {
		return st.resolution
	}
	if g.total == 0 {
		return 0
	}
	return st.resolution / g.total
}

// fastLocalMove moves nodes between communities until no move improves the
// quality function, using a work queue rather than repeated full sweeps: only
// nodes whose neighbourhood actually changed are reconsidered. Reports
// whether anything moved.
func (st *leidenState) fastLocalMove(g *weighted, p *partition, sizeOf []float64, rng *rand.Rand) bool {
	gamma := st.gamma(g)

	queue := make([]int32, g.n)
	for i := range queue {
		queue[i] = int32(i)
	}
	rng.Shuffle(len(queue), func(i, j int) { queue[i], queue[j] = queue[j], queue[i] })
	inQueue := make([]bool, g.n)
	for i := range inQueue {
		inQueue[i] = true
	}

	// Scratch state for accumulating e(v, C). Indexed by community id, with a
	// touched-list so it clears in O(deg) — never range a map here, or the
	// candidate order (and so the tie-break) becomes nondeterministic.
	weightTo := make([]float64, p.nComm)
	touched := make([]int32, 0, 16)

	// free is a stack of community slots believed empty. Entries may be stale
	// (a slot can be refilled), so pops are validated against count.
	free := make([]int32, 0, 16)

	moved := false
	for head := 0; head < len(queue); head++ {
		v := queue[head]
		inQueue[v] = false

		origin := p.member[v]
		p.remove(v, sizeOf[v], g.strength[v])

		nbrs, wts := g.neighbours(v)
		for i, u := range nbrs {
			c := p.member[u]
			if weightTo[c] == 0 {
				touched = append(touched, c)
			}
			weightTo[c] += wts[i]
		}

		// Baseline candidate: stand alone in a fresh community, which scores
		// exactly 0. Removing v always empties at least one slot somewhere, so
		// this option is essentially always available; falling back to origin
		// if it is not is harmless.
		emptyC := int32(-1)
		if p.count[origin] == 0 {
			emptyC = origin
		} else {
			for len(free) > 0 {
				c := free[len(free)-1]
				free = free[:len(free)-1]
				if p.count[c] == 0 {
					emptyC = c
					break
				}
			}
		}
		best, bestGain := emptyC, 0.0

		for _, c := range touched {
			if p.count[c] == 0 {
				continue // already represented by the baseline
			}
			gain := weightTo[c] - gamma*sizeOf[v]*p.size[c]
			switch {
			case gain > bestGain+epsilon:
				best, bestGain = c, gain
			case gain > bestGain-epsilon && (best < 0 || c < best):
				best, bestGain = c, gain // tie: lowest community index wins
			}
		}
		if best < 0 {
			best = origin
		}
		if emptyC >= 0 && emptyC != origin && best != emptyC {
			free = append(free, emptyC) // unused; keep the slot findable
		}

		p.insert(v, best, sizeOf[v], g.strength[v])

		if best != origin {
			moved = true
			if p.count[origin] == 0 {
				free = append(free, origin)
			}
			for _, u := range nbrs {
				if p.member[u] != best && !inQueue[u] {
					inQueue[u] = true
					queue = append(queue, u)
				}
			}
		}

		for _, c := range touched {
			weightTo[c] = 0
		}
		touched = touched[:0]
	}
	return moved
}

// refine splits each community of p into well-connected sub-communities. It
// starts from ALL singletons — deliberately finer than p — and only ever
// merges a node into a subcommunity it has an edge to and that is itself
// well-connected to the surrounding community. That restriction is what
// guarantees every resulting community is internally connected.
func (st *leidenState) refine(g *weighted, p *partition, sizeOf []float64, rng *rand.Rand) *partition {
	refined := singletons(g, sizeOf)

	// Group nodes by their p-community in a single ascending scan, so each
	// subset is index-sorted and the subsets themselves appear in order of
	// their smallest member.
	subsets := make([][]int32, p.nComm)
	for v := int32(0); v < int32(g.n); v++ {
		c := p.member[v]
		subsets[c] = append(subsets[c], v)
	}
	for _, s := range subsets {
		if len(s) > 1 {
			st.mergeNodesSubset(g, refined, s, sizeOf, rng)
		}
	}
	refined.compact(g, sizeOf)
	return refined
}

// mergeNodesSubset merges the nodes of one community S into well-connected
// sub-communities of S.
func (st *leidenState) mergeNodesSubset(g *weighted, refined *partition, s []int32, sizeOf []float64, rng *rand.Rand) {
	gamma := st.gamma(g)

	inS := make(map[int32]bool, len(s))
	for _, v := range s {
		inS[v] = true
	}
	var sizeS float64
	for _, v := range s {
		sizeS += sizeOf[v]
	}

	// eIn[v] is v's edge weight to the rest of S; eOut[c] is refined community
	// c's edge weight to the rest of S, maintained as nodes are merged in.
	eIn := make(map[int32]float64, len(s))
	for _, v := range s {
		nbrs, wts := g.neighbours(v)
		var w float64
		for i, u := range nbrs {
			if inS[u] {
				w += wts[i]
			}
		}
		eIn[v] = w
	}
	eOut := make(map[int32]float64, len(s))
	for _, v := range s {
		eOut[refined.member[v]] = eIn[v]
	}

	order := append([]int32(nil), s...)
	rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })

	weightTo := map[int32]float64{}
	var cands []int32
	for _, v := range order {
		// LEIDEN: only nodes still alone in their refined community are
		// considered. Revisiting absorbed nodes would turn refinement into a
		// second local-move pass and void the connectivity proof.
		if refined.count[refined.member[v]] != 1 {
			continue
		}
		// LEIDEN: v itself must be well-connected to S, or it stays a
		// singleton — this is what strands genuinely peripheral nodes instead
		// of glueing them onto an arbitrary neighbour.
		if eIn[v] < gamma*sizeOf[v]*(sizeS-sizeOf[v])-epsilon {
			continue
		}

		clear(weightTo)
		nbrs, wts := g.neighbours(v)
		for i, u := range nbrs {
			if inS[u] && u != v {
				weightTo[refined.member[u]] += wts[i]
			}
		}

		// Candidates come from the adjacency, then are SORTED — map iteration
		// order must never reach the sampling step.
		cands = cands[:0]
		for c := range weightTo {
			cands = append(cands, c)
		}
		slices.Sort(cands)
		cands = slices.DeleteFunc(cands, func(c int32) bool {
			if c == refined.member[v] {
				return true
			}
			// LEIDEN: the target sub-community must itself be well-connected
			// to S, not merely adjacent to v.
			if eOut[c] < gamma*refined.size[c]*(sizeS-refined.size[c])-epsilon {
				return true
			}
			return weightTo[c]-gamma*sizeOf[v]*refined.size[c] < -epsilon
		})
		if len(cands) == 0 {
			continue
		}

		best := math.Inf(-1)
		gains := make([]float64, len(cands))
		for i, c := range cands {
			gains[i] = weightTo[c] - gamma*sizeOf[v]*refined.size[c]
			if gains[i] > best {
				best = gains[i]
			}
		}
		// Sample rather than take the argmax, weighting by exp(Δ/θ). The
		// exponent is clamped so a large quality spread cannot underflow to
		// NaN.
		weights := make([]float64, len(cands))
		var sum float64
		for i := range cands {
			e := (gains[i] - best) / st.theta
			if e < -700 {
				weights[i] = 0
			} else {
				weights[i] = math.Exp(e)
			}
			sum += weights[i]
		}
		chosen := cands[len(cands)-1] // float-shortfall fallback
		if sum > 0 {
			r := rng.Float64() * sum
			acc := 0.0
			for i, c := range cands {
				acc += weights[i]
				if acc > r {
					chosen = c
					break
				}
			}
		}

		from := refined.member[v]
		refined.remove(v, sizeOf[v], g.strength[v])
		refined.insert(v, chosen, sizeOf[v], g.strength[v])
		delete(eOut, from)
		// v's edges to S now split into edges internal to `chosen` and edges
		// still leaving it.
		var toChosen float64
		for i, u := range nbrs {
			if inS[u] && refined.member[u] == chosen && u != v {
				toChosen += wts[i]
			}
		}
		eOut[chosen] += eIn[v] - 2*toChosen
	}
}

// project seeds the aggregate graph's partition from the coarse partition:
// the aggregate node standing for refined community R resumes in the
// community that R's representative belonged to in p. This is well-defined
// because every refined community lies wholly inside one p-community.
func (st *leidenState) project(p *partition, agg *weighted, rep []int32) *partition {
	sizeOf := st.sizes(agg)
	coarse := singletons(agg, sizeOf)
	for a, r := range rep {
		coarse.member[a] = p.member[r]
	}
	coarse.compact(agg, sizeOf)
	return coarse
}
