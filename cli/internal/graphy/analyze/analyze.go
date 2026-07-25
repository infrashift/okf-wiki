// Package analyze derives graph insights: god nodes, community summaries,
// confidence breakdowns, and orphan counts — the inputs to GRAPH_REPORT.md.
package analyze

import (
	"sort"
	"strings"

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
	// Rationale counts recorded intent by marker (WHY, DECISION, ADR, …).
	Rationale map[string]int
	// ADRs are the architecture decision records found in the repo.
	ADRs []model.Node
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
	// Explicitly last. An absent kind already scores 0, but naming it stops a
	// future contributor from "helpfully" ranking prose — a community called
	// "WHY: don't hold the mutex here" would be a poor subsystem name.
	model.KindRationale: 0,
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
		Rationale:  map[string]int{},
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
		if n.Kind != model.KindRationale {
			continue
		}
		if _, marker, _, ok := model.ParseRationaleQual(qualOf(n.ID)); ok {
			r.Rationale[marker]++
			if marker == "ADR" {
				r.ADRs = append(r.ADRs, n)
			}
		}
	}
	sort.Slice(r.ADRs, func(i, j int) bool { return r.ADRs[i].ID < r.ADRs[j].ID })

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

	// God nodes: highest-degree entities, skipping bare file scaffolding and
	// prose (a rationale's degree says nothing about a codebase's hot spots).
	ranked := make([]model.Node, 0, len(nodes))
	for _, n := range nodes {
		switch n.Kind {
		case model.KindFile, model.KindHeading, model.KindRationale:
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

// qualOf returns the qualified-name half of a node id
// ("<lang>:<kind>:<file>#<qual>").
func qualOf(nodeID string) string {
	if i := strings.LastIndex(nodeID, "#"); i >= 0 {
		return nodeID[i+1:]
	}
	return ""
}
