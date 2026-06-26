package graphdb

import (
	"encoding/json"
	"fmt"
	"io"

	"okfw/cli/internal/graph"

	bolt "go.etcd.io/bbolt"
)

// GetBacklinks returns all nodes that link to the given node ID.
func (s *Store) GetBacklinks(id string) ([]NodeRecord, error) {
	var sources []string
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketBacklinks)
		data := b.Get([]byte(id))
		if data == nil {
			return nil
		}
		return json.Unmarshal(data, &sources)
	})
	if err != nil {
		return nil, err
	}

	nodes := make([]NodeRecord, 0, len(sources))
	for _, srcID := range sources {
		n, err := s.getNodeRecord(srcID)
		if err != nil {
			return nil, err
		}
		if n != nil {
			nodes = append(nodes, *n)
		}
	}
	return nodes, nil
}

// GetCluster returns a subgraph of nodes within the given depth from the start node.
// It uses BFS, same algorithm as graph.FindConnected, but reads from bbolt.
func (s *Store) GetCluster(startID string, depth, limit int) ([]NodeRecord, []EdgeRecord, error) {
	if limit <= 0 {
		limit = 10
	}

	start, err := s.getNodeRecord(startID)
	if err != nil {
		return nil, nil, err
	}
	if start == nil {
		return nil, nil, fmt.Errorf("node %q not found", startID)
	}

	type bfsItem struct {
		id    string
		depth int
	}

	visited := make(map[string]bool)
	var resultNodes []NodeRecord
	queue := []bfsItem{{id: startID, depth: 0}}

	for len(queue) > 0 && len(resultNodes) < limit {
		current := queue[0]
		queue = queue[1:]

		if visited[current.id] {
			continue
		}
		visited[current.id] = true

		node, err := s.getNodeRecord(current.id)
		if err != nil {
			return nil, nil, err
		}
		if node == nil {
			continue
		}
		resultNodes = append(resultNodes, *node)

		if depth > 0 && current.depth >= depth {
			continue
		}

		nextDepth := current.depth + 1

		// Forward links.
		for _, linkID := range node.Links {
			if !visited[linkID] {
				queue = append(queue, bfsItem{id: linkID, depth: nextDepth})
			}
		}

		// Parent.
		if node.Parent != "" && !visited[node.Parent] {
			queue = append(queue, bfsItem{id: node.Parent, depth: nextDepth})
		}

		// Children (nodes that have this node as parent).
		children, err := s.getChildIDs(node.ID)
		if err != nil {
			return nil, nil, err
		}
		for _, childID := range children {
			if !visited[childID] {
				queue = append(queue, bfsItem{id: childID, depth: nextDepth})
			}
		}

		// Reverse links (backlinks).
		backlinks, err := s.getBacklinkIDs(node.ID)
		if err != nil {
			return nil, nil, err
		}
		for _, blID := range backlinks {
			if !visited[blID] {
				queue = append(queue, bfsItem{id: blID, depth: nextDepth})
			}
		}
	}

	// Collect edges between visited nodes.
	var resultEdges []EdgeRecord
	err = s.db.View(func(tx *bolt.Tx) error {
		edges := tx.Bucket(bucketEdges)
		return edges.ForEach(func(_, v []byte) error {
			var e EdgeRecord
			if err := json.Unmarshal(v, &e); err != nil {
				return nil
			}
			// Only include non-tag edges between visited nodes.
			if e.Predicate == PredicateTagged {
				return nil
			}
			// Skip redundant parent_of edges (child_of already captured).
			if e.Predicate == PredicateParentOf {
				return nil
			}
			if visited[e.From] && visited[e.To] {
				resultEdges = append(resultEdges, e)
			}
			return nil
		})
	})
	if err != nil {
		return nil, nil, err
	}

	return resultNodes, resultEdges, nil
}

// GetByTag returns all nodes tagged with the given tag.
func (s *Store) GetByTag(tag string) ([]NodeRecord, error) {
	var ids []string
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketTags)
		data := b.Get([]byte(tag))
		if data == nil {
			return nil
		}
		return json.Unmarshal(data, &ids)
	})
	if err != nil {
		return nil, err
	}

	nodes := make([]NodeRecord, 0, len(ids))
	for _, id := range ids {
		n, err := s.getNodeRecord(id)
		if err != nil {
			return nil, err
		}
		if n != nil {
			nodes = append(nodes, *n)
		}
	}
	return nodes, nil
}

// GetDanglingEdges returns edges that point to non-existent nodes.
func (s *Store) GetDanglingEdges() ([]EdgeRecord, error) {
	var dangling []EdgeRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		edges := tx.Bucket(bucketEdges)
		nodes := tx.Bucket(bucketNodes)
		return edges.ForEach(func(_, v []byte) error {
			var e EdgeRecord
			if err := json.Unmarshal(v, &e); err != nil {
				return nil
			}
			if e.Predicate == PredicateTagged {
				return nil
			}
			if nodes.Get([]byte(e.To)) == nil {
				dangling = append(dangling, e)
			}
			return nil
		})
	})
	return dangling, err
}

// ToGraph converts the persistent store into an in-memory graph for rendering.
func (s *Store) ToGraph() (*graph.Graph, error) {
	g := graph.New()
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketNodes)
		return b.ForEach(func(_, v []byte) error {
			var nr NodeRecord
			if err := json.Unmarshal(v, &nr); err != nil {
				return nil
			}
			g.AddNode(&graph.Node{
				ID:       nr.ID,
				Title:    nr.Title,
				FilePath: nr.FilePath,
				Project:  nr.Project,
				Category: nr.Category,
				Parent:   nr.Parent,
				Links:    nr.Links,
			})
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	g.BuildRelationships()
	return g, nil
}

// GraphJSON is the JSON structure exported for Neovim integration.
type GraphJSON struct {
	Nodes []NodeRecord `json:"nodes"`
	Edges []EdgeRecord `json:"edges"`
}

// ExportJSON writes the full graph as JSON to the given writer.
func (s *Store) ExportJSON(w io.Writer) error {
	var result GraphJSON

	err := s.db.View(func(tx *bolt.Tx) error {
		// Nodes.
		nodesBucket := tx.Bucket(bucketNodes)
		if err := nodesBucket.ForEach(func(_, v []byte) error {
			var nr NodeRecord
			if err := json.Unmarshal(v, &nr); err != nil {
				return nil
			}
			result.Nodes = append(result.Nodes, nr)
			return nil
		}); err != nil {
			return err
		}

		// Edges (exclude tag edges for the graph export).
		edgesBucket := tx.Bucket(bucketEdges)
		return edgesBucket.ForEach(func(_, v []byte) error {
			var e EdgeRecord
			if err := json.Unmarshal(v, &e); err != nil {
				return nil
			}
			if e.Predicate != PredicateTagged {
				result.Edges = append(result.Edges, e)
			}
			return nil
		})
	})
	if err != nil {
		return err
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

// AllNodes returns all nodes in the store.
func (s *Store) AllNodes() ([]NodeRecord, error) {
	var nodes []NodeRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketNodes)
		return b.ForEach(func(_, v []byte) error {
			var nr NodeRecord
			if err := json.Unmarshal(v, &nr); err != nil {
				return nil
			}
			nodes = append(nodes, nr)
			return nil
		})
	})
	return nodes, err
}

// getBacklinkIDs returns the raw list of source IDs that link to the given target.
func (s *Store) getBacklinkIDs(id string) ([]string, error) {
	var ids []string
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketBacklinks)
		data := b.Get([]byte(id))
		if data == nil {
			return nil
		}
		return json.Unmarshal(data, &ids)
	})
	return ids, err
}

// getChildIDs returns IDs of nodes whose parent is the given ID.
// Uses the edges bucket to find parent_of edges from this node.
func (s *Store) getChildIDs(id string) ([]string, error) {
	var children []string
	prefix := edgeKey(id, PredicateParentOf, "")
	err := s.db.View(func(tx *bolt.Tx) error {
		edges := tx.Bucket(bucketEdges)
		c := edges.Cursor()
		for k, v := c.Seek(prefix); k != nil && hasPrefix(k, prefix); k, v = c.Next() {
			var e EdgeRecord
			if err := json.Unmarshal(v, &e); err != nil {
				continue
			}
			children = append(children, e.To)
		}
		return nil
	})
	return children, err
}
