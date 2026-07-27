// Package graphdb provides persistent graph storage for a bundle
// using bbolt as the storage backend.
package graphdb

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"okfw/cli/internal/config"

	bolt "go.etcd.io/bbolt"
)

// Bucket names.
var (
	bucketNodes     = []byte("nodes")
	bucketEdges     = []byte("edges")
	bucketBacklinks = []byte("backlinks")
	bucketTags      = []byte("tags")
	bucketMeta      = []byte("meta")
)

// Edge predicates.
const (
	PredicateLinksTo  = "links_to"
	PredicateParentOf = "parent_of"
	PredicateChildOf  = "child_of"
	PredicateTagged   = "tagged"
)

// NodeRecord represents a concept node stored in bbolt.
type NodeRecord struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	FilePath string   `json:"file_path"`
	Project  string   `json:"project,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Parent   string   `json:"parent,omitempty"`
	Links    []string `json:"links,omitempty"`
	ModTime  int64    `json:"mod_time"`
}

// EdgeRecord represents a directed edge stored in bbolt.
type EdgeRecord struct {
	From      string `json:"from"`
	Predicate string `json:"predicate"`
	To        string `json:"to"`
}

// edgeKey builds the composite key for an edge: from\x00predicate\x00to
func edgeKey(from, predicate, to string) []byte {
	return []byte(from + "\x00" + predicate + "\x00" + to)
}

// edgePrefix returns the prefix for all edges originating from a given node.
func edgePrefix(from string) []byte {
	return []byte(from + "\x00")
}

// SyncResult reports what changed during a sync operation.
type SyncResult struct {
	Added    int
	Updated  int
	Deleted  int
	Dangling int
	Total    int
}

// Store provides persistent graph storage backed by bbolt.
type Store struct {
	db   *bolt.DB
	path string
}

// Open opens or creates a graphdb at the given path.
func Open(path string) (*Store, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{
		Timeout: 1 * time.Second,
	})
	if err != nil {
		if err == bolt.ErrTimeout {
			return nil, fmt.Errorf("graph database is locked by another okfw process: %s", path)
		}
		return nil, fmt.Errorf("failed to open graph database: %w", err)
	}

	// Ensure all buckets exist.
	err = db.Update(func(tx *bolt.Tx) error {
		for _, name := range [][]byte{bucketNodes, bucketEdges, bucketBacklinks, bucketTags, bucketMeta} {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return fmt.Errorf("failed to create bucket %s: %w", name, err)
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	return &Store{db: db, path: path}, nil
}

// ErrNoStore reports that a bundle has no graph database.
var ErrNoStore = errors.New("no graph database in this bundle")

// OpenExisting opens a graph database that must already exist.
//
// Open creates the file, which is right for graph-sync and wrong for a
// read-only query. One `okfw graph-query dangling` on a bundle that had never
// run graph-sync left an *empty* store behind — and every command that decides
// between the store and a filesystem scan by whether that file exists then took
// the store branch and returned nothing, permanently. The advice lives in the
// error so callers stay at a bare `return err`.
func OpenExisting(path string) (*Store, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("%w: %s — run `okfw graph-sync` to build it",
			ErrNoStore, path)
	}
	return Open(path)
}

// Close closes the underlying bbolt database.
func (s *Store) Close() error {
	return s.db.Close()
}

// Path returns the database file path.
func (s *Store) Path() string {
	return s.path
}

// Sync walks the filesystem at rootPath and synchronizes the graph database.
// It adds/updates changed nodes, removes deleted nodes, and rebuilds all edges.
func (s *Store) Sync(rootPath string) (*SyncResult, error) {
	info, err := os.Stat(rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat root path %q: %w", rootPath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root path %q is not a directory", rootPath)
	}

	// Collect all concept files from filesystem.
	type fileEntry struct {
		path    string
		modTime int64
	}
	var files []fileEntry
	err = filepath.Walk(rootPath, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Skip hidden dirs and ephemeral/, but never the walk root itself
		// (the bundle root is the hidden .okfw directory).
		if fi.IsDir() && p != rootPath && (strings.HasPrefix(fi.Name(), ".") || fi.Name() == "ephemeral") {
			return filepath.SkipDir
		}
		if !fi.IsDir() && strings.HasSuffix(p, ".md") && fi.Name() != "index.md" && fi.Name() != "log.md" {
			files = append(files, fileEntry{path: p, modTime: fi.ModTime().Unix()})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	result := &SyncResult{}

	// Track which IDs exist on the filesystem for deletion detection.
	fsIDs := make(map[string]bool)

	// Process each file.
	for _, f := range files {
		node, err := parseConceptFile(f.path, rootPath)
		if err != nil {
			continue // skip non-concept or malformed files
		}
		node.ModTime = f.modTime
		fsIDs[node.ID] = true

		// Check if node already exists and is unchanged.
		existing, err := s.getNodeRecord(node.ID)
		if err != nil {
			return nil, err
		}
		if existing != nil && existing.ModTime >= node.ModTime {
			result.Total++
			continue // unchanged
		}

		if existing == nil {
			result.Added++
		} else {
			result.Updated++
		}
		result.Total++

		if err := s.upsertNode(node); err != nil {
			return nil, fmt.Errorf("failed to upsert node %s: %w", node.ID, err)
		}
	}

	// Detect and purge deleted nodes.
	deleted, err := s.purgeDeletedNodes(fsIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to purge deleted nodes: %w", err)
	}
	result.Deleted = deleted

	// Count dangling edges.
	dangling, err := s.countDanglingEdges()
	if err != nil {
		return nil, fmt.Errorf("failed to count dangling edges: %w", err)
	}
	result.Dangling = dangling

	// Update last sync timestamp.
	err = s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketMeta)
		return b.Put([]byte("last_sync"), []byte(fmt.Sprintf("%d", time.Now().Unix())))
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// SyncFile synchronizes a single file into the graph database. bundleRoot is the
// bundle root used to resolve the file's markdown links to target concept IDs.
func (s *Store) SyncFile(filePath, bundleRoot string) error {
	fi, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File deleted — try to find and purge any node with this filepath.
			return s.purgeNodeByFilePath(filePath)
		}
		return fmt.Errorf("failed to stat file: %w", err)
	}

	node, err := parseConceptFile(filePath, bundleRoot)
	if err != nil {
		return fmt.Errorf("failed to parse concept: %w", err)
	}
	node.ModTime = fi.ModTime().Unix()

	return s.upsertNode(node)
}

// PurgeNode removes a node and all its associated edges from the database.
func (s *Store) PurgeNode(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return purgeNodeTx(tx, id)
	})
}

// purgeNodeTx removes a node and all edges within a transaction.
func purgeNodeTx(tx *bolt.Tx, id string) error {
	nodes := tx.Bucket(bucketNodes)
	edges := tx.Bucket(bucketEdges)
	backlinks := tx.Bucket(bucketBacklinks)
	tags := tx.Bucket(bucketTags)

	// Load the node to get its tags and links for index cleanup.
	nodeData := nodes.Get([]byte(id))
	if nodeData == nil {
		return nil // nothing to purge
	}

	var node NodeRecord
	if err := json.Unmarshal(nodeData, &node); err != nil {
		return fmt.Errorf("failed to unmarshal node %s: %w", id, err)
	}

	// Remove all outgoing edges (prefix scan on "id\x00").
	prefix := edgePrefix(id)
	c := edges.Cursor()
	for k, _ := c.Seek(prefix); k != nil && hasPrefix(k, prefix); k, _ = c.Next() {
		if err := edges.Delete(k); err != nil {
			return err
		}
	}

	// Remove all incoming edges (scan for edges where target == id).
	// Also clean up backlinks index.
	c = edges.Cursor()
	for k, v := c.First(); k != nil; k, v = c.Next() {
		var e EdgeRecord
		if err := json.Unmarshal(v, &e); err != nil {
			continue
		}
		if e.To == id {
			if err := edges.Delete(k); err != nil {
				return err
			}
		}
	}

	// Remove this node from backlinks index (as a target).
	if err := backlinks.Delete([]byte(id)); err != nil {
		return err
	}

	// Remove this node from other nodes' backlink lists (as a source).
	for _, linkID := range node.Links {
		if err := removeFromIDList(backlinks, linkID, id); err != nil {
			return err
		}
	}
	if node.Parent != "" {
		if err := removeFromIDList(backlinks, node.Parent, id); err != nil {
			return err
		}
	}

	// Remove this node from tag indices.
	for _, tag := range node.Tags {
		if err := removeFromIDList(tags, tag, id); err != nil {
			return err
		}
	}

	// Remove the node itself.
	return nodes.Delete([]byte(id))
}

// upsertNode inserts or updates a node and rebuilds its edges.
func (s *Store) upsertNode(node *NodeRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		nodes := tx.Bucket(bucketNodes)
		edges := tx.Bucket(bucketEdges)
		backlinksB := tx.Bucket(bucketBacklinks)
		tagsB := tx.Bucket(bucketTags)

		// If node already exists, clean up its old edges first.
		if existing := nodes.Get([]byte(node.ID)); existing != nil {
			var old NodeRecord
			if err := json.Unmarshal(existing, &old); err == nil {
				// Remove old outgoing edges.
				prefix := edgePrefix(node.ID)
				c := edges.Cursor()
				for k, _ := c.Seek(prefix); k != nil && hasPrefix(k, prefix); k, _ = c.Next() {
					if err := edges.Delete(k); err != nil {
						return err
					}
				}
				// Remove from old backlink targets.
				for _, linkID := range old.Links {
					if err := removeFromIDList(backlinksB, linkID, node.ID); err != nil {
						return err
					}
				}
				if old.Parent != "" {
					if err := removeFromIDList(backlinksB, old.Parent, node.ID); err != nil {
						return err
					}
				}
				// Remove from old tag indices.
				for _, tag := range old.Tags {
					if err := removeFromIDList(tagsB, tag, node.ID); err != nil {
						return err
					}
				}
			}
		}

		// Store the node.
		data, err := json.Marshal(node)
		if err != nil {
			return fmt.Errorf("failed to marshal node: %w", err)
		}
		if err := nodes.Put([]byte(node.ID), data); err != nil {
			return err
		}

		// Insert edges: links_to
		for _, linkID := range node.Links {
			e := EdgeRecord{From: node.ID, Predicate: PredicateLinksTo, To: linkID}
			eData, _ := json.Marshal(e)
			if err := edges.Put(edgeKey(node.ID, PredicateLinksTo, linkID), eData); err != nil {
				return err
			}
			if err := addToIDList(backlinksB, linkID, node.ID); err != nil {
				return err
			}
		}

		// Insert edges: parent_of / child_of
		if node.Parent != "" {
			// This node is a child of parent.
			e := EdgeRecord{From: node.ID, Predicate: PredicateChildOf, To: node.Parent}
			eData, _ := json.Marshal(e)
			if err := edges.Put(edgeKey(node.ID, PredicateChildOf, node.Parent), eData); err != nil {
				return err
			}
			// Parent has this node as child (reverse).
			e2 := EdgeRecord{From: node.Parent, Predicate: PredicateParentOf, To: node.ID}
			eData2, _ := json.Marshal(e2)
			if err := edges.Put(edgeKey(node.Parent, PredicateParentOf, node.ID), eData2); err != nil {
				return err
			}
			if err := addToIDList(backlinksB, node.Parent, node.ID); err != nil {
				return err
			}
		}

		// Insert edges: tagged
		for _, tag := range node.Tags {
			e := EdgeRecord{From: node.ID, Predicate: PredicateTagged, To: tag}
			eData, _ := json.Marshal(e)
			if err := edges.Put(edgeKey(node.ID, PredicateTagged, tag), eData); err != nil {
				return err
			}
			if err := addToIDList(tagsB, tag, node.ID); err != nil {
				return err
			}
		}

		return nil
	})
}

// getNodeRecord retrieves a node by ID, or nil if not found.
func (s *Store) getNodeRecord(id string) (*NodeRecord, error) {
	var node *NodeRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketNodes)
		data := b.Get([]byte(id))
		if data == nil {
			return nil
		}
		node = &NodeRecord{}
		return json.Unmarshal(data, node)
	})
	return node, err
}

// GetNode retrieves a single node by ID.
func (s *Store) GetNode(id string) (*NodeRecord, error) {
	return s.getNodeRecord(id)
}

// NodeCount returns the number of nodes in the store.
func (s *Store) NodeCount() (int, error) {
	var count int
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketNodes)
		count = b.Stats().KeyN
		return nil
	})
	return count, err
}

// purgeDeletedNodes removes nodes that exist in the DB but not in fsIDs.
func (s *Store) purgeDeletedNodes(fsIDs map[string]bool) (int, error) {
	// First, collect IDs to delete (can't modify during iteration).
	var toDelete []string
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketNodes)
		return b.ForEach(func(k, _ []byte) error {
			if !fsIDs[string(k)] {
				toDelete = append(toDelete, string(k))
			}
			return nil
		})
	})
	if err != nil {
		return 0, err
	}

	for _, id := range toDelete {
		if err := s.PurgeNode(id); err != nil {
			return 0, err
		}
	}
	return len(toDelete), nil
}

// purgeNodeByFilePath finds a node by file path and purges it.
func (s *Store) purgeNodeByFilePath(filePath string) error {
	absPath, _ := filepath.Abs(filePath)
	var targetID string
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketNodes)
		return b.ForEach(func(k, v []byte) error {
			var n NodeRecord
			if err := json.Unmarshal(v, &n); err != nil {
				return nil
			}
			if n.FilePath == absPath {
				targetID = n.ID
			}
			return nil
		})
	})
	if err != nil {
		return err
	}
	if targetID != "" {
		return s.PurgeNode(targetID)
	}
	return nil
}

// countDanglingEdges counts edges where the target node doesn't exist.
func (s *Store) countDanglingEdges() (int, error) {
	var count int
	err := s.db.View(func(tx *bolt.Tx) error {
		edges := tx.Bucket(bucketEdges)
		nodes := tx.Bucket(bucketNodes)
		return edges.ForEach(func(_, v []byte) error {
			var e EdgeRecord
			if err := json.Unmarshal(v, &e); err != nil {
				return nil
			}
			// Tags point to tag strings, not node IDs.
			if e.Predicate == PredicateTagged {
				return nil
			}
			if nodes.Get([]byte(e.To)) == nil {
				count++
			}
			return nil
		})
	})
	return count, err
}

// parseConceptFile parses a concept markdown file into a NodeRecord. bundleRoot
// is used to resolve the body's vanilla markdown links to target concept IDs.
func parseConceptFile(filePath, bundleRoot string) (*NodeRecord, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	z, err := config.ParseFrontmatter(content)
	if err != nil {
		return nil, err
	}

	if z.ID == "" {
		return nil, fmt.Errorf("not a valid concept: no id field")
	}

	absPath, _ := filepath.Abs(filePath)

	body := config.ExtractBody(content)
	links := config.ResolveBodyLinkIDs(body, absPath, bundleRoot)

	return &NodeRecord{
		ID:       z.ID,
		Title:    z.Title,
		FilePath: absPath,
		Project:  z.Project,
		Tags:     z.Tags,
		Parent:   z.Parent,
		Links:    links,
	}, nil
}

// hasPrefix checks if a byte slice has the given prefix.
func hasPrefix(s, prefix []byte) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i, b := range prefix {
		if s[i] != b {
			return false
		}
	}
	return true
}

// addToIDList appends an ID to a JSON-encoded string list in a bucket.
func addToIDList(b *bolt.Bucket, key, id string) error {
	var ids []string
	if data := b.Get([]byte(key)); data != nil {
		if err := json.Unmarshal(data, &ids); err != nil {
			return fmt.Errorf("corrupt ID list for key %q: %w", key, err)
		}
	}
	// Check for duplicates.
	for _, existing := range ids {
		if existing == id {
			return nil
		}
	}
	ids = append(ids, id)
	data, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	return b.Put([]byte(key), data)
}

// removeFromIDList removes an ID from a JSON-encoded string list in a bucket.
func removeFromIDList(b *bolt.Bucket, key, id string) error {
	var ids []string
	data := b.Get([]byte(key))
	if data == nil {
		return nil
	}
	if err := json.Unmarshal(data, &ids); err != nil {
		return fmt.Errorf("corrupt ID list for key %q: %w", key, err)
	}
	filtered := ids[:0]
	for _, existing := range ids {
		if existing != id {
			filtered = append(filtered, existing)
		}
	}
	if len(filtered) == 0 {
		return b.Delete([]byte(key))
	}
	data, err := json.Marshal(filtered)
	if err != nil {
		return err
	}
	return b.Put([]byte(key), data)
}
