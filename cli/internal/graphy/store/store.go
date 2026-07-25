// Package store persists graphy's knowledge graph in a single bbolt file.
//
// Unlike okfw's graphdb (which derives a fixed set of predicates from note
// frontmatter), this store holds arbitrary typed, confidence-tagged edges.
// Layout:
//
//	nodes       nodeID                → JSON model.Node
//	edges       src \x00 rel \x00 dst → JSON model.Edge
//	rev         dst \x00 rel \x00 src → nil (reverse index; prefix scan = in-edges)
//	files       relPath               → JSON FileRecord (extraction cache)
//	symbols     lang \x00 name        → JSON []nodeID (definition index)
//	communities itoa(id)              → JSON Community
//	exports     nodeID                → JSON ExportRecord (okfw export idempotency)
//	meta        key                   → raw value
package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	bolt "go.etcd.io/bbolt"

	"okfw/cli/internal/graphy/model"
)

const sep = "\x00"

var buckets = []string{"nodes", "edges", "rev", "files", "symbols", "communities", "exports", "meta"}

// FileRecord caches one source file's extraction result so unchanged files
// are skipped on incremental builds and their refs can be re-resolved without
// re-parsing.
type FileRecord struct {
	Path    string       `json:"path"` // repo-relative, slash-separated
	SHA256  string       `json:"sha256"`
	Size    int64        `json:"size"`
	NodeIDs []string     `json:"node_ids"`
	Refs    []model.Ref  `json:"refs,omitempty"`
	Edges   []model.Edge `json:"edges,omitempty"` // intra-file edges, re-emitted on every ReplaceEdges
}

// Community is a detected community with a human label.
type Community struct {
	ID      int      `json:"id"`
	Label   string   `json:"label"`
	NodeIDs []string `json:"node_ids"`
}

// ClusterMeta records how the persisted community assignment was produced, so
// a graph's communities can be reproduced or explained after the fact. It is
// stored as JSON under MetaClusterKey in the meta bucket, and surfaced in
// GRAPH_REPORT.md.
type ClusterMeta struct {
	Algorithm  string  `json:"algorithm"`
	Resolution float64 `json:"resolution"`
	Seed       uint64  `json:"seed"`
}

// MetaClusterKey is the meta bucket key holding the JSON ClusterMeta.
const MetaClusterKey = "cluster"

// ExportRecord maps a graph node to the okfw concept it was exported as.
type ExportRecord struct {
	ConceptID string `json:"concept_id"`
	RelPath   string `json:"rel_path"` // bundle-relative concept file path
	// MemberDigest is a community's node membership as of the last export,
	// packed as sorted 64-bit hashes (see export.memberDigest); it is empty for
	// node records. It has to live here rather than be read from the
	// communities bucket because that bucket is rebuilt from scratch on every
	// build — by export time the previous partition is gone, and comparing
	// against it is the only way to recognize a community whose label changed.
	//
	// The JSON key deliberately differs from the plain id list this replaced,
	// so a record written by an older build decodes with an empty digest
	// instead of failing: it simply gets refreshed on the next export.
	MemberDigest []byte `json:"member_digest,omitempty"`
}

// Store is a graphy graph database handle.
type Store struct {
	db   *bolt.DB
	path string
}

// Open opens (creating if needed) the store and ensures all buckets exist.
func Open(path string) (*Store, error) {
	db, err := bolt.Open(path, 0644, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open graph store: %w", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		for _, b := range buckets {
			if _, err := tx.CreateBucketIfNotExists([]byte(b)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create buckets: %w", err)
	}
	return &Store{db: db, path: path}, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

// Path returns the on-disk database path.
func (s *Store) Path() string { return s.path }

func edgeKey(e model.Edge) []byte {
	return []byte(e.Source + sep + string(e.Relation) + sep + e.Target)
}

func revKey(e model.Edge) []byte {
	return []byte(e.Target + sep + string(e.Relation) + sep + e.Source)
}

// PutFile stores a file's extraction result: its record, nodes, and the
// definition-symbol index entries. Any previous contribution from the same
// path is purged first, so re-extraction is idempotent.
func (s *Store) PutFile(rec FileRecord, nodes []model.Node) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := purgeFileTx(tx, rec.Path); err != nil {
			return err
		}
		nb := tx.Bucket([]byte("nodes"))
		for i := range nodes {
			data, err := json.Marshal(nodes[i])
			if err != nil {
				return err
			}
			if err := nb.Put([]byte(nodes[i].ID), data); err != nil {
				return err
			}
			if err := addSymbolTx(tx, nodes[i]); err != nil {
				return err
			}
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("files")).Put([]byte(rec.Path), data)
	})
}

// PurgeFile removes a file's record, its nodes, and its symbol entries.
// Edges are rebuilt wholesale by ReplaceEdges, so they are not tracked here.
func (s *Store) PurgeFile(relPath string) error {
	return s.db.Update(func(tx *bolt.Tx) error { return purgeFileTx(tx, relPath) })
}

func purgeFileTx(tx *bolt.Tx, relPath string) error {
	fb := tx.Bucket([]byte("files"))
	raw := fb.Get([]byte(relPath))
	if raw == nil {
		return nil
	}
	var rec FileRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return err
	}
	nb := tx.Bucket([]byte("nodes"))
	for _, id := range rec.NodeIDs {
		nraw := nb.Get([]byte(id))
		if nraw != nil {
			var n model.Node
			if err := json.Unmarshal(nraw, &n); err == nil {
				if err := removeSymbolTx(tx, n); err != nil {
					return err
				}
			}
			if err := nb.Delete([]byte(id)); err != nil {
				return err
			}
		}
	}
	return fb.Delete([]byte(relPath))
}

// symbolKeys indexes a definition under its full label and — for qualified
// labels like "Store.Close" — its bare trailing name, so call sites that only
// see "Close" still find it.
func symbolKeys(n model.Node) [][]byte {
	keys := [][]byte{[]byte(n.Language + sep + n.Label)}
	if i := strings.LastIndex(n.Label, "."); i >= 0 && i+1 < len(n.Label) {
		keys = append(keys, []byte(n.Language+sep+n.Label[i+1:]))
	}
	return keys
}

func indexableSymbol(n model.Node) bool {
	return model.Indexable(n) && n.Label != ""
}

func addSymbolTx(tx *bolt.Tx, n model.Node) error {
	if !indexableSymbol(n) {
		return nil
	}
	sb := tx.Bucket([]byte("symbols"))
	for _, key := range symbolKeys(n) {
		var ids []string
		if raw := sb.Get(key); raw != nil {
			if err := json.Unmarshal(raw, &ids); err != nil {
				return err
			}
		}
		present := false
		for _, id := range ids {
			if id == n.ID {
				present = true
				break
			}
		}
		if present {
			continue
		}
		ids = append(ids, n.ID)
		data, err := json.Marshal(ids)
		if err != nil {
			return err
		}
		if err := sb.Put(key, data); err != nil {
			return err
		}
	}
	return nil
}

func removeSymbolTx(tx *bolt.Tx, n model.Node) error {
	if !indexableSymbol(n) {
		return nil
	}
	sb := tx.Bucket([]byte("symbols"))
	for _, key := range symbolKeys(n) {
		raw := sb.Get(key)
		if raw == nil {
			continue
		}
		var ids []string
		if err := json.Unmarshal(raw, &ids); err != nil {
			return err
		}
		out := ids[:0]
		for _, id := range ids {
			if id != n.ID {
				out = append(out, id)
			}
		}
		if len(out) == 0 {
			if err := sb.Delete(key); err != nil {
				return err
			}
			continue
		}
		data, err := json.Marshal(out)
		if err != nil {
			return err
		}
		if err := sb.Put(key, data); err != nil {
			return err
		}
	}
	return nil
}

// LookupSymbol returns the definition node ids indexed under (lang, name).
func (s *Store) LookupSymbol(lang, name string) ([]string, error) {
	var ids []string
	err := s.db.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket([]byte("symbols")).Get([]byte(lang + sep + name))
		if raw == nil {
			return nil
		}
		return json.Unmarshal(raw, &ids)
	})
	return ids, err
}

// ReplaceEdges rewrites the edges and rev buckets wholesale. Resolution runs
// globally on every build, so edge state is always derived fresh.
func (s *Store) ReplaceEdges(edges []model.Edge) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		for _, name := range []string{"edges", "rev"} {
			if err := tx.DeleteBucket([]byte(name)); err != nil {
				return err
			}
			if _, err := tx.CreateBucket([]byte(name)); err != nil {
				return err
			}
		}
		eb := tx.Bucket([]byte("edges"))
		rb := tx.Bucket([]byte("rev"))
		for i := range edges {
			data, err := json.Marshal(edges[i])
			if err != nil {
				return err
			}
			if err := eb.Put(edgeKey(edges[i]), data); err != nil {
				return err
			}
			if err := rb.Put(revKey(edges[i]), nil); err != nil {
				return err
			}
		}
		return nil
	})
}

// RecomputeDegrees recounts each node's degree (in+out) and stores it back.
func (s *Store) RecomputeDegrees() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		deg := map[string]int{}
		err := tx.Bucket([]byte("edges")).ForEach(func(k, v []byte) error {
			var e model.Edge
			if err := json.Unmarshal(v, &e); err != nil {
				return err
			}
			deg[e.Source]++
			deg[e.Target]++
			return nil
		})
		if err != nil {
			return err
		}
		nb := tx.Bucket([]byte("nodes"))
		// Collect first — never mutate a bucket mid-iteration.
		type upd struct {
			key  []byte
			node model.Node
		}
		var updates []upd
		err = nb.ForEach(func(k, v []byte) error {
			var n model.Node
			if err := json.Unmarshal(v, &n); err != nil {
				return err
			}
			if d := deg[n.ID]; d != n.Degree {
				n.Degree = d
				updates = append(updates, upd{append([]byte(nil), k...), n})
			}
			return nil
		})
		if err != nil {
			return err
		}
		for _, u := range updates {
			data, err := json.Marshal(u.node)
			if err != nil {
				return err
			}
			if err := nb.Put(u.key, data); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetNode fetches a node by id; returns nil when absent.
func (s *Store) GetNode(id string) (*model.Node, error) {
	var n *model.Node
	err := s.db.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket([]byte("nodes")).Get([]byte(id))
		if raw == nil {
			return nil
		}
		n = &model.Node{}
		return json.Unmarshal(raw, n)
	})
	return n, err
}

// EachNode iterates all nodes.
func (s *Store) EachNode(fn func(model.Node) error) error {
	return s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("nodes")).ForEach(func(k, v []byte) error {
			var n model.Node
			if err := json.Unmarshal(v, &n); err != nil {
				return err
			}
			return fn(n)
		})
	})
}

// EachEdge iterates all edges.
func (s *Store) EachEdge(fn func(model.Edge) error) error {
	return s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("edges")).ForEach(func(k, v []byte) error {
			var e model.Edge
			if err := json.Unmarshal(v, &e); err != nil {
				return err
			}
			return fn(e)
		})
	})
}

// OutEdges returns edges originating at id (prefix scan of edges).
func (s *Store) OutEdges(id string) ([]model.Edge, error) {
	var out []model.Edge
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket([]byte("edges")).Cursor()
		prefix := []byte(id + sep)
		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			var e model.Edge
			if err := json.Unmarshal(v, &e); err != nil {
				return err
			}
			out = append(out, e)
		}
		return nil
	})
	return out, err
}

// InEdges returns edges pointing at id (prefix scan of rev, then edge fetch).
func (s *Store) InEdges(id string) ([]model.Edge, error) {
	var in []model.Edge
	err := s.db.View(func(tx *bolt.Tx) error {
		eb := tx.Bucket([]byte("edges"))
		c := tx.Bucket([]byte("rev")).Cursor()
		prefix := []byte(id + sep)
		for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
			parts := bytes.SplitN(k, []byte(sep), 3)
			if len(parts) != 3 {
				continue
			}
			ekey := append(append(append([]byte(nil), parts[2]...), sep...), parts[1]...)
			ekey = append(append(ekey, sep...), parts[0]...)
			raw := eb.Get(ekey)
			if raw == nil {
				continue
			}
			var e model.Edge
			if err := json.Unmarshal(raw, &e); err != nil {
				return err
			}
			in = append(in, e)
		}
		return nil
	})
	return in, err
}

// GetFile returns a file's cached extraction record, or nil when unknown.
func (s *Store) GetFile(relPath string) (*FileRecord, error) {
	var rec *FileRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket([]byte("files")).Get([]byte(relPath))
		if raw == nil {
			return nil
		}
		rec = &FileRecord{}
		return json.Unmarshal(raw, rec)
	})
	return rec, err
}

// EachFile iterates all cached file records.
func (s *Store) EachFile(fn func(FileRecord) error) error {
	return s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("files")).ForEach(func(k, v []byte) error {
			var rec FileRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			return fn(rec)
		})
	})
}

// SetCommunities replaces the communities bucket and stamps each node's
// community id.
func (s *Store) SetCommunities(assign map[string]int, labels map[int]string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket([]byte("communities")); err != nil {
			return err
		}
		cb, err := tx.CreateBucket([]byte("communities"))
		if err != nil {
			return err
		}
		members := map[int][]string{}
		for id, c := range assign {
			members[c] = append(members[c], id)
		}
		for c, ids := range members {
			com := Community{ID: c, Label: labels[c], NodeIDs: ids}
			data, err := json.Marshal(com)
			if err != nil {
				return err
			}
			if err := cb.Put([]byte(strconv.Itoa(c)), data); err != nil {
				return err
			}
		}
		nb := tx.Bucket([]byte("nodes"))
		type upd struct {
			key  []byte
			node model.Node
		}
		var updates []upd
		err = nb.ForEach(func(k, v []byte) error {
			var n model.Node
			if err := json.Unmarshal(v, &n); err != nil {
				return err
			}
			if c, ok := assign[n.ID]; ok && c != n.Community {
				n.Community = c
				updates = append(updates, upd{append([]byte(nil), k...), n})
			}
			return nil
		})
		if err != nil {
			return err
		}
		for _, u := range updates {
			data, err := json.Marshal(u.node)
			if err != nil {
				return err
			}
			if err := nb.Put(u.key, data); err != nil {
				return err
			}
		}
		return nil
	})
}

// EachCommunity iterates all communities.
func (s *Store) EachCommunity(fn func(Community) error) error {
	return s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("communities")).ForEach(func(k, v []byte) error {
			var c Community
			if err := json.Unmarshal(v, &c); err != nil {
				return err
			}
			return fn(c)
		})
	})
}

// GetExport returns the okfw export record for a node, or nil.
func (s *Store) GetExport(nodeID string) (*ExportRecord, error) {
	var rec *ExportRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket([]byte("exports")).Get([]byte(nodeID))
		if raw == nil {
			return nil
		}
		rec = &ExportRecord{}
		return json.Unmarshal(raw, rec)
	})
	return rec, err
}

// PutExport stores the okfw export record for a node.
func (s *Store) PutExport(nodeID string, rec ExportRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("exports")).Put([]byte(nodeID), data)
	})
}

// PutExports writes several export records in a single transaction. An export
// refreshes one record per community, and a separate commit (and fsync) for
// each would dominate the cost of an otherwise no-op run.
func (s *Store) PutExports(recs map[string]ExportRecord) error {
	if len(recs) == 0 {
		return nil
	}
	keys := make([]string, 0, len(recs))
	for k := range recs {
		keys = append(keys, k)
	}
	sort.Strings(keys) // deterministic write order
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("exports"))
		for _, k := range keys {
			data, err := json.Marshal(recs[k])
			if err != nil {
				return err
			}
			if err := b.Put([]byte(k), data); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteExport removes a node's export record.
func (s *Store) DeleteExport(nodeID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("exports")).Delete([]byte(nodeID))
	})
}

// EachExport iterates all export records keyed by node id.
func (s *Store) EachExport(fn func(nodeID string, rec ExportRecord) error) error {
	return s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("exports")).ForEach(func(k, v []byte) error {
			var rec ExportRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			return fn(string(k), rec)
		})
	})
}

// PutMeta stores a metadata value.
func (s *Store) PutMeta(key, value string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("meta")).Put([]byte(key), []byte(value))
	})
}

// GetMeta fetches a metadata value ("" when absent).
func (s *Store) GetMeta(key string) (string, error) {
	var v string
	err := s.db.View(func(tx *bolt.Tx) error {
		v = string(tx.Bucket([]byte("meta")).Get([]byte(key)))
		return nil
	})
	return v, err
}

// DeleteMeta removes a metadata key. Deleting an absent key is not an error.
func (s *Store) DeleteMeta(key string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("meta")).Delete([]byte(key))
	})
}

// PutClusterMeta records how the current community assignment was produced.
func (s *Store) PutClusterMeta(m ClusterMeta) error {
	blob, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return s.PutMeta(MetaClusterKey, string(blob))
}

// GetClusterMeta returns the recorded clustering parameters. A store built
// before this was recorded yields the zero value and no error, so callers can
// render "unknown" rather than fail.
func (s *Store) GetClusterMeta() (ClusterMeta, error) {
	var m ClusterMeta
	raw, err := s.GetMeta(MetaClusterKey)
	if err != nil || raw == "" {
		return m, err
	}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return ClusterMeta{}, nil // unreadable legacy value: treat as absent
	}
	return m, nil
}

// Counts returns node and edge totals.
func (s *Store) Counts() (nodes, edges int, err error) {
	err = s.db.View(func(tx *bolt.Tx) error {
		nodes = tx.Bucket([]byte("nodes")).Stats().KeyN
		edges = tx.Bucket([]byte("edges")).Stats().KeyN
		return nil
	})
	return nodes, edges, err
}
