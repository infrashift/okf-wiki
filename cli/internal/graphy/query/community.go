package query

import (
	"fmt"
	"sort"
	"strings"

	"okfw/cli/internal/bundle"
	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/store"
)

// A community is the one thing the wiki could name and the graph could not.
//
// `graphy export okfw` writes a concept per community, stamped with a
// `graphy.node_id` of "community:<slug>", and `okfw search --node-id` finds it.
// Coming back the other way there was nothing: ResolveEntity only searches the
// nodes bucket, and no community node exists there. So a subsystem concept was
// a dead end — and the concept body itself told the reader to run
// `graphy explain "Community: <label>"`, which Explain can only reject.

// CommunityCard is a subsystem: its label, its membership, and the concept
// documenting it.
type CommunityCard struct {
	ID    int    `json:"id"`
	Label string `json:"label"`
	Slug  string `json:"slug"`
	// Members is capped at Limit, most connected first; Total is the true size.
	Members   []model.Node `json:"members"`
	Total     int          `json:"total"`
	More      bool         `json:"truncated"`
	Concept   *Concept     `json:"concept,omitempty"`
	Languages []string     `json:"languages,omitempty"`
}

// Community resolves a subsystem by slug, label, exports key, or concept id.
//
// The key translation is the trap: the exports-bucket key is
// "\x00community:<slug>" while the concept's graphy.node_id — the string
// `okfw search --node-id` hands back — is "community:<slug>". Looking the
// latter up in the exports bucket finds nothing.
func Community(s *store.Store, ref string, limit int) (*CommunityCard, error) {
	if limit <= 0 {
		limit = DefaultLimit
	}
	want := strings.TrimSpace(ref)
	want = strings.TrimPrefix(want, "\x00")
	want = strings.TrimPrefix(want, "community:")
	// The exported concept titles communities "Community: <label>".
	if rest, ok := strings.CutPrefix(want, "Community: "); ok {
		want = rest
	}
	if want == "" {
		return nil, fmt.Errorf("community reference is empty")
	}
	lower := strings.ToLower(want)

	var matches []store.Community
	if err := s.EachCommunity(func(c store.Community) error {
		if c.Label == want || bundle.Slugify(c.Label) == bundle.Slugify(want) ||
			strings.ToLower(c.Label) == lower {
			matches = append(matches, c)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no community matches %q (list them with `graphy report --section Communities`)", ref)
	}
	if len(matches) > 1 {
		// uniqueSlug disambiguates on export, so two same-labelled communities
		// are two different subsystems and picking one silently would be wrong.
		sort.Slice(matches, func(i, j int) bool { return matches[i].ID < matches[j].ID })
		ids := make([]string, len(matches))
		for i, c := range matches {
			ids[i] = fmt.Sprintf("%d", c.ID)
		}
		return nil, fmt.Errorf("%q matches %d communities (ids %s); refer to one by its exported concept id",
			ref, len(matches), strings.Join(ids, ", "))
	}

	c := matches[0]
	card := &CommunityCard{ID: c.ID, Label: c.Label, Slug: bundle.Slugify(c.Label)}

	langs := map[string]bool{}
	members := make([]model.Node, 0, len(c.NodeIDs))
	for _, id := range c.NodeIDs {
		n, err := s.GetNode(id)
		if err != nil {
			return nil, err
		}
		if n == nil {
			continue
		}
		members = append(members, *n)
		if n.Language != "" {
			langs[n.Language] = true
		}
	}
	sort.Slice(members, func(i, j int) bool {
		if members[i].Degree != members[j].Degree {
			return members[i].Degree > members[j].Degree
		}
		return members[i].ID < members[j].ID
	})
	card.Total = len(members)
	if len(members) > limit {
		members = members[:limit]
		card.More = true
	}
	card.Members = members
	for l := range langs {
		card.Languages = append(card.Languages, l)
	}
	sort.Strings(card.Languages)

	// The concept, if the graph has been exported. Keyed by the exports key,
	// not by the node_id the wiki reports.
	if rec, err := s.GetExport("\x00community:" + card.Slug); err != nil {
		return nil, err
	} else if rec != nil {
		card.Concept, err = withBundle(s, "community:"+card.Slug, rec.ConceptID, rec.RelPath)
		if err != nil {
			return nil, err
		}
	}
	return card, nil
}
