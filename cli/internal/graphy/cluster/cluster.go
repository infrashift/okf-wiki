// Package cluster assigns graph nodes to communities. Detection is LLM-free.
//
// The Detector interface isolates the algorithm choice. Two are available:
// Leiden (the default), a pure-Go implementation that guarantees every
// returned community is internally connected; and gonum's Louvain, kept
// selectable because it is what earlier graphs were built with. Both are
// deterministic given a seed, and both number communities by their
// lexicographically-smallest member so ids are comparable across runs.
package cluster

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// Detector assigns a community id to every node id.
//
// edges is a multiset of undirected pairs; implementations may treat repeated
// pairs as edge weight. Self-loops and pairs naming ids absent from nodeIDs
// are ignored. Implementations must return an entry for every distinct id in
// nodeIDs, including isolated nodes — store.SetCommunities leaves nodes
// missing from the assignment stamped with their previous community, so a
// partial assignment silently preserves stale data.
type Detector interface {
	Detect(nodeIDs []string, edges [][2]string) (map[string]int, error)
}

// Algorithm names a community detection implementation.
type Algorithm string

const (
	AlgoLeiden  Algorithm = "leiden"
	AlgoLouvain Algorithm = "louvain"
)

// Defaults applied wherever a zero value is supplied, so that a zero-valued
// options struct means "Leiden, resolution 1, seed 1" everywhere.
const (
	DefaultAlgorithm  = AlgoLeiden
	DefaultResolution = 1.0
	DefaultSeed       = uint64(1)
)

// Algorithms lists the selectable algorithm names, for flag help and errors.
func Algorithms() []string { return []string{string(AlgoLeiden), string(AlgoLouvain)} }

// ParseAlgorithm normalizes and validates a user-supplied algorithm name.
// The empty string yields DefaultAlgorithm.
func ParseAlgorithm(s string) (Algorithm, error) {
	switch Algorithm(strings.ToLower(strings.TrimSpace(s))) {
	case "":
		return DefaultAlgorithm, nil
	case AlgoLeiden:
		return AlgoLeiden, nil
	case AlgoLouvain:
		return AlgoLouvain, nil
	default:
		return "", fmt.Errorf("unknown clustering algorithm %q (want one of: %s)",
			s, strings.Join(Algorithms(), ", "))
	}
}

// New returns the detector for an algorithm. A zero resolution or seed means
// the package default.
func New(algo Algorithm, resolution float64, seed uint64) (Detector, error) {
	a, err := ParseAlgorithm(string(algo))
	if err != nil {
		return nil, err
	}
	if resolution < 0 || math.IsNaN(resolution) || math.IsInf(resolution, 0) {
		return nil, fmt.Errorf("invalid clustering resolution %v (want a finite value >= 0)", resolution)
	}
	if resolution == 0 {
		resolution = DefaultResolution
	}
	if seed == 0 {
		seed = DefaultSeed
	}
	switch a {
	case AlgoLouvain:
		return Louvain{Resolution: resolution, Seed: seed}, nil
	default:
		return Leiden{Resolution: resolution, Seed: seed}, nil
	}
}

// sortUnique returns the node ids sorted and deduplicated. Deduplication
// matters: a repeated id would otherwise claim two dense indices, and the
// orphaned one would surface as a phantom singleton community.
func sortUnique(ids []string) []string {
	sorted := append([]string(nil), ids...)
	slices.Sort(sorted)
	return slices.Compact(sorted)
}

// renumber maps a dense membership array back onto node ids, numbering
// communities by their lexicographically-smallest member so that the returned
// ids are stable regardless of the order an algorithm happened to discover
// the communities in.
func renumber(sortedIDs []string, member []int32) map[string]int {
	type comm struct {
		smallest string
		members  []string
	}
	byID := map[int32]int{} // community label → index into comms
	var comms []comm
	for i, id := range sortedIDs {
		c := member[i]
		idx, seen := byID[c]
		if !seen {
			idx = len(comms)
			byID[c] = idx
			comms = append(comms, comm{smallest: id})
		}
		comms[idx].members = append(comms[idx].members, id)
	}
	// sortedIDs is ascending, so each community's first-seen member is already
	// its smallest; sorting the communities by it gives the stable numbering.
	slices.SortFunc(comms, func(a, b comm) int { return strings.Compare(a.smallest, b.smallest) })

	assign := make(map[string]int, len(sortedIDs))
	for id, c := range comms {
		for _, m := range c.members {
			assign[m] = id
		}
	}
	return assign
}
