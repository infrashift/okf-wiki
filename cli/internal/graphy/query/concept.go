package query

import (
	"path/filepath"

	"okfw/cli/internal/graphy/store"
)

// The bridge between a code-graph node and the wiki concept documenting it.
//
// The mapping has always been persisted — `graphy export okfw` writes an
// ExportRecord per node — but nothing read it back, so the two halves of the
// system named the same entity with no shared identifier. An agent that found a
// function in the graph could not open its concept, and one that found a
// concept could not ask the graph anything further about it. Both directions
// are cheap lookups; the only reason they did not exist is that nobody exposed
// the bucket.

// Concept locates the wiki concept a graph node was exported as.
type Concept struct {
	NodeID string `json:"node_id"`
	// ConceptID is the concept's OKF id (a UUIDv7), stable across renames.
	ConceptID string `json:"concept_id"`
	// RelPath is bundle-relative; Path is absolute when the bundle root is known.
	RelPath string `json:"path"`
	Path    string `json:"abs_path,omitempty"`
	Bundle  string `json:"bundle,omitempty"`
}

// ConceptFor returns the concept a node was exported as, or nil when the node
// has never been exported.
func ConceptFor(s *store.Store, nodeID string) (*Concept, error) {
	rec, err := s.GetExport(nodeID)
	if err != nil || rec == nil {
		return nil, err
	}
	return withBundle(s, nodeID, rec.ConceptID, rec.RelPath)
}

// ConceptIndex maps node ids to their concepts in one pass, for callers
// annotating a whole result set. Returns an empty map when nothing has been
// exported, so callers need no special case.
func ConceptIndex(s *store.Store) (map[string]Concept, error) {
	bundle, err := s.GetMeta(store.MetaBundleKey)
	if err != nil {
		return nil, err
	}
	out := map[string]Concept{}
	if err := s.EachExport(func(nodeID string, rec store.ExportRecord) error {
		c := Concept{NodeID: nodeID, ConceptID: rec.ConceptID, RelPath: rec.RelPath, Bundle: bundle}
		if bundle != "" {
			c.Path = filepath.Join(bundle, filepath.FromSlash(rec.RelPath))
		}
		out[nodeID] = c
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func withBundle(s *store.Store, nodeID, conceptID, relPath string) (*Concept, error) {
	bundle, err := s.GetMeta(store.MetaBundleKey)
	if err != nil {
		return nil, err
	}
	c := &Concept{NodeID: nodeID, ConceptID: conceptID, RelPath: relPath, Bundle: bundle}
	if bundle != "" {
		c.Path = filepath.Join(bundle, filepath.FromSlash(relPath))
	}
	return c, nil
}

// ConceptsForFile returns the concepts exported for the entities declared in a
// repo-relative file, in source order.
//
// A file node is never exported — selectItems skips KindFile — yet both
// `graphy concept` and the graphy_concept MCP tool advertise a repo-relative
// path, and ResolveEntity honours it. So a path resolved to the file node and
// then failed with "run `graphy export okfw`": advice that could never work,
// because re-exporting will never produce a concept for a file. The answerable
// question for a path is "what in here has been documented".
func ConceptsForFile(s *store.Store, path string) ([]Concept, error) {
	nodes, err := FileNodes(s, path)
	if err != nil {
		return nil, err
	}
	out := make([]Concept, 0, len(nodes))
	for _, n := range nodes {
		c, err := ConceptFor(s, n.ID)
		if err != nil {
			return nil, err
		}
		if c != nil {
			out = append(out, *c)
		}
	}
	return out, nil
}
