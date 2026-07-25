// Package analyze derives graph insights: god nodes, community summaries,
// confidence breakdowns, and orphan counts — the inputs to GRAPH_REPORT.md.
package analyze

import (
	"sort"

	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/store"
)

// CommunitySummary describes one detected community.
type CommunitySummary struct {
	ID    int
	Label string
	Size  int
	Top   []model.Node // highest-degree members
}

// Report is everything the rendered report needs.
type Report struct {
	Root        string
	Stats       model.Stats
	Languages   map[string]int
	Confidence  map[model.Confidence]int
	GodNodes    []model.Node
	Communities []CommunitySummary
	Orphans     int
	// Cluster is how the communities were produced. Rendering it makes
	// GRAPH_REPORT.md a visible regression signal if the flag plumbing breaks;
	// it is the zero value for stores built before it was recorded.
	Cluster store.ClusterMeta
}

// labelPriority prefers meaningful code entities when naming a community.
var labelPriority = map[model.Kind]int{
	model.KindModule:    6,
	model.KindClass:     5,
	model.KindStruct:    5,
	model.KindInterface: 5,
	model.KindResource:  5,
	model.KindFunction:  4,
	model.KindMethod:    3,
	model.KindType:      3,
	model.KindDoc:       2,
	model.KindFile:      1,
}

// CommunityLabels names each community after its most prominent member:
// highest kind-priority, then highest degree.
func CommunityLabels(nodes []model.Node, assign map[string]int) map[int]string {
	best := map[int]model.Node{}
	for _, n := range nodes {
		c, ok := assign[n.ID]
		if !ok {
			continue
		}
		cur, seen := best[c]
		if !seen || better(n, cur) {
			best[c] = n
		}
	}
	labels := make(map[int]string, len(best))
	for c, n := range best {
		labels[c] = n.Label
	}
	return labels
}

func better(a, b model.Node) bool {
	pa, pb := labelPriority[a.Kind], labelPriority[b.Kind]
	if pa != pb {
		return pa > pb
	}
	if a.Degree != b.Degree {
		return a.Degree > b.Degree
	}
	return a.ID < b.ID // deterministic tiebreak
}

// Build computes the report from a fully-built store.
func Build(s *store.Store, root string) (*Report, error) {
	r := &Report{
		Root:       root,
		Languages:  map[string]int{},
		Confidence: map[model.Confidence]int{},
	}

	var nodes []model.Node
	if err := s.EachNode(func(n model.Node) error {
		nodes = append(nodes, n)
		return nil
	}); err != nil {
		return nil, err
	}
	r.Stats.Nodes = len(nodes)
	for _, n := range nodes {
		if n.Language != "" {
			r.Languages[n.Language]++
		}
		if n.Degree == 0 {
			r.Orphans++
		}
	}

	if err := s.EachEdge(func(e model.Edge) error {
		r.Stats.Edges++
		r.Confidence[e.Confidence]++
		return nil
	}); err != nil {
		return nil, err
	}

	if err := s.EachFile(func(store.FileRecord) error {
		r.Stats.Files++
		return nil
	}); err != nil {
		return nil, err
	}

	// God nodes: highest-degree entities, skipping bare file scaffolding.
	ranked := make([]model.Node, 0, len(nodes))
	for _, n := range nodes {
		if n.Kind == model.KindFile || n.Kind == model.KindHeading {
			continue
		}
		ranked = append(ranked, n)
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Degree != ranked[j].Degree {
			return ranked[i].Degree > ranked[j].Degree
		}
		return ranked[i].ID < ranked[j].ID
	})
	if len(ranked) > 10 {
		ranked = ranked[:10]
	}
	r.GodNodes = ranked

	byID := map[string]model.Node{}
	for _, n := range nodes {
		byID[n.ID] = n
	}
	if err := s.EachCommunity(func(c store.Community) error {
		summary := CommunitySummary{ID: c.ID, Label: c.Label, Size: len(c.NodeIDs)}
		members := make([]model.Node, 0, len(c.NodeIDs))
		for _, id := range c.NodeIDs {
			members = append(members, byID[id])
		}
		sort.Slice(members, func(i, j int) bool {
			if members[i].Degree != members[j].Degree {
				return members[i].Degree > members[j].Degree
			}
			return members[i].ID < members[j].ID
		})
		if len(members) > 5 {
			members = members[:5]
		}
		summary.Top = members
		r.Communities = append(r.Communities, summary)
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Slice(r.Communities, func(i, j int) bool {
		if r.Communities[i].Size != r.Communities[j].Size {
			return r.Communities[i].Size > r.Communities[j].Size
		}
		return r.Communities[i].ID < r.Communities[j].ID
	})
	r.Stats.Communities = len(r.Communities)

	meta, err := s.GetClusterMeta()
	if err != nil {
		return nil, err
	}
	r.Cluster = meta
	return r, nil
}
