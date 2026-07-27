package query

import (
	"path/filepath"
	"strings"
	"testing"

	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/store"
)

// communityFixture: two labelled communities, and a store handle for the
// collision case to reuse.
func communityFixture(t *testing.T, labels map[int]string) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	var nodes []model.Node
	assign := map[string]int{}
	for id := range labels {
		for i := 0; i < 3; i++ {
			nid := strings.Join([]string{"go:function:c", itoa(id), ".go#F", itoa(i)}, "")
			nodes = append(nodes, model.Node{
				ID: nid, Label: "F" + itoa(id) + itoa(i), Kind: model.KindFunction,
				Language: "go", File: "c" + itoa(id) + ".go", Degree: i,
			})
			assign[nid] = id
		}
	}
	if err := s.PutFile(store.FileRecord{Path: "all", NodeIDs: ids(nodes)}, nodes); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCommunities(assign, labels); err != nil {
		t.Fatal(err)
	}
	return s
}

func itoa(i int) string { return string(rune('0' + i)) }

// TestCommunityResolvesByLabelSlugAndNodeID: the strings that reach this are
// whatever `okfw search --node-id` reported, whatever the concept title said,
// and whatever the user typed.
func TestCommunityResolvesByLabelSlugAndNodeID(t *testing.T) {
	s := communityFixture(t, map[int]string{0: "Graph Store", 1: "extract"})

	for _, ref := range []string{
		"Graph Store",              // the label
		"graph-store",              // the slug
		"community:graph-store",    // the node id okfw search reports
		"Community: Graph Store",   // the exported concept's title
		"graph store",              // case-insensitive
	} {
		card, err := Community(s, ref, 0)
		if err != nil {
			t.Errorf("Community(%q): %v", ref, err)
			continue
		}
		if card.Label != "Graph Store" {
			t.Errorf("Community(%q) resolved to %q", ref, card.Label)
		}
		if card.Total != 3 {
			t.Errorf("Community(%q) has %d members, want 3", ref, card.Total)
		}
	}

	if _, err := Community(s, "nonesuch", 0); err == nil {
		t.Error("an unknown community resolved")
	}
}

// TestCommunityMembersAreBoundedAndOrdered: most connected first, so a capped
// list keeps the entities the subsystem hangs off.
func TestCommunityMembersAreBoundedAndOrdered(t *testing.T) {
	s := communityFixture(t, map[int]string{0: "core"})
	card, err := Community(s, "core", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(card.Members) != 2 || !card.More || card.Total != 3 {
		t.Errorf("members=%d truncated=%v total=%d", len(card.Members), card.More, card.Total)
	}
	if card.Members[0].Degree < card.Members[1].Degree {
		t.Errorf("members are not ordered by degree: %+v", card.Members)
	}
}

// TestCommunityRefusesAnAmbiguousLabel: uniqueSlug disambiguates on export, so
// two same-labelled communities are two different subsystems and silently
// picking one would answer about the wrong code.
func TestCommunityRefusesAnAmbiguousLabel(t *testing.T) {
	s := communityFixture(t, map[int]string{0: "core", 1: "core"})
	_, err := Community(s, "core", 0)
	if err == nil {
		t.Fatal("an ambiguous label resolved")
	}
	if !strings.Contains(err.Error(), "matches 2 communities") {
		t.Errorf("error does not report the ambiguity: %v", err)
	}
}
