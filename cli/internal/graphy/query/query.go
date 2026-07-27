// Package query answers deterministic graph questions: entity resolution,
// shortest paths, entity cards, neighborhood expansion, and a keyword search
// that doubles as the offline fallback for natural-language queries.
package query

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"

	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/store"
)

// AmbiguousError reports an entity string that matched several nodes.
type AmbiguousError struct {
	Arg        string
	Candidates []model.Node
}

func (e *AmbiguousError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%q is ambiguous — candidates:\n", e.Arg)
	for _, n := range e.Candidates {
		fmt.Fprintf(&b, "  %s (%s, %s)\n", n.ID, n.Kind, n.File)
	}
	return strings.TrimRight(b.String(), "\n")
}

// ResolveEntity maps a user-supplied string to one node: exact id, then a
// repo-relative file path, then unique exact label, then unique
// case-insensitive substring of a label.
//
// The file-path step matters more than it looks. An agent is almost always
// working *in* a file, and matching labels alone meant the one identifier it
// certainly has — the path it is editing — resolved to nothing unless some node
// happened to be labelled with it.
func ResolveEntity(s *store.Store, arg string) (*model.Node, error) {
	if n, err := s.GetNode(arg); err != nil {
		return nil, err
	} else if n != nil {
		return n, nil
	}
	if n, err := fileNode(s, arg); err != nil {
		return nil, err
	} else if n != nil {
		return n, nil
	}
	var exact, fuzzy []model.Node
	lower := strings.ToLower(arg)
	if err := s.EachNode(func(n model.Node) error {
		switch {
		case n.Label == arg:
			exact = append(exact, n)
		case strings.Contains(strings.ToLower(n.Label), lower):
			fuzzy = append(fuzzy, n)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	pick := func(list []model.Node) (*model.Node, error) {
		if len(list) == 1 {
			return &list[0], nil
		}
		sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
		if len(list) > 10 {
			list = list[:10]
		}
		return nil, &AmbiguousError{Arg: arg, Candidates: list}
	}
	if len(exact) > 0 {
		return pick(exact)
	}
	if len(fuzzy) > 0 {
		return pick(fuzzy)
	}
	return nil, fmt.Errorf("no entity matches %q", arg)
}

// fileNode returns the file-level node for a repo-relative path, or nil.
// Every extractor guarantees its first node is the file node.
func fileNode(s *store.Store, path string) (*model.Node, error) {
	rec, err := s.GetFile(strings.TrimPrefix(filepath.ToSlash(path), "./"))
	if err != nil || rec == nil || len(rec.NodeIDs) == 0 {
		return nil, err
	}
	return s.GetNode(rec.NodeIDs[0])
}

// FileNodes returns every entity declared in a repo-relative file, ordered by
// source position.
//
// This is the "I am editing this file, what is in it" question. The mapping has
// always been in the store's file records; nothing exposed it, so an agent
// holding a path had to guess a symbol name to get into the graph at all.
func FileNodes(s *store.Store, path string) ([]model.Node, error) {
	rec, err := s.GetFile(strings.TrimPrefix(filepath.ToSlash(path), "./"))
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, fmt.Errorf("no extracted file %q", path)
	}
	out := make([]model.Node, 0, len(rec.NodeIDs))
	for _, id := range rec.NodeIDs {
		n, err := s.GetNode(id)
		if err != nil {
			return nil, err
		}
		if n != nil {
			out = append(out, *n)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Span.StartLine != out[j].Span.StartLine {
			return out[i].Span.StartLine < out[j].Span.StartLine
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// Direction selects which way an expansion or edge listing travels.
type Direction string

const (
	// Out follows edges from the entity: what it uses.
	Out Direction = "out"
	// In follows edges into the entity: what uses it — the impact question.
	In Direction = "in"
	// Both ignores direction.
	Both Direction = "both"
)

// ParseDirection validates a direction string ("" means Both).
func ParseDirection(s string) (Direction, error) {
	switch Direction(strings.ToLower(strings.TrimSpace(s))) {
	case "", Both:
		return Both, nil
	case Out:
		return Out, nil
	case In:
		return In, nil
	}
	return "", fmt.Errorf("invalid direction %q (want out, in or both)", s)
}

// ParseRelations validates relation names against the model vocabulary.
// An unknown name is an error rather than a silent empty filter, which would
// quietly widen the query to everything.
func ParseRelations(names []string) ([]model.Relation, error) {
	var out []model.Relation
	for _, raw := range names {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" {
			continue
		}
		rel := model.Relation(name)
		if !model.ValidRelation(rel) {
			return nil, fmt.Errorf("unknown relation %q (known: %s)", raw,
				strings.Join(model.RelationNames(), ", "))
		}
		out = append(out, rel)
	}
	return out, nil
}

// ParseKinds validates node-kind names against the model vocabulary.
func ParseKinds(names []string) ([]model.Kind, error) {
	var out []model.Kind
	for _, raw := range names {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" {
			continue
		}
		kind := model.Kind(name)
		if !model.ValidKind(kind) {
			return nil, fmt.Errorf("unknown kind %q (known: %s)", raw,
				strings.Join(model.KindNames(), ", "))
		}
		out = append(out, kind)
	}
	return out, nil
}

// NeighborOptions filters a neighbourhood expansion.
type NeighborOptions struct {
	Depth     int // clamped to [1, MaxDepth]
	Direction Direction
	Relations []model.Relation // empty = all
	Kinds     []model.Kind     // empty = all; never filters the seeds
	// Limit caps the returned nodes, most connected first. 0 = DefaultLimit.
	Limit int
	// IncludeTests keeps entities declared in test files. Off by default: a
	// well-tested function's neighbourhood is mostly its own test suite, which
	// crowds out the callers an impact question is actually about.
	IncludeTests bool
}

// NeighborResult is a bounded neighbourhood.
type NeighborResult struct {
	Seeds []string      `json:"seeds"`
	Nodes []model.Node  `json:"nodes"`
	Edges []model.Edge  `json:"edges"`
	Total int           `json:"total"`     // nodes found before the limit
	More  bool          `json:"truncated"` // Total exceeded Limit
}

// DefaultLimit bounds any traversal that does not ask for a size.
const DefaultLimit = 50

// Neighbors expands seeds by BFS and returns the induced subgraph, filtered and
// capped.
//
// This is the primitive an agent needs and could not reach: Subgraph has always
// done the expansion, but its only caller picks seeds from an LLM plan or a
// keyword score, so an agent that already knew which entity it cared about had
// no way to say "expand from this one". Everything here is deterministic — no
// model, no ranking heuristic — and bounded, so the answer is both reproducible
// and affordable.
func Neighbors(s *store.Store, seeds []string, opts NeighborOptions) (*NeighborResult, error) {
	depth := opts.Depth
	if depth < 1 {
		depth = 1
	}
	if depth > MaxDepth {
		depth = MaxDepth
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	dir := opts.Direction
	if dir == "" {
		dir = Both
	}
	allowRel := map[model.Relation]bool{}
	for _, r := range opts.Relations {
		allowRel[r] = true
	}
	keepRel := func(r model.Relation) bool { return len(allowRel) == 0 || allowRel[r] }
	allowKind := map[model.Kind]bool{}
	for _, k := range opts.Kinds {
		allowKind[k] = true
	}

	seen := map[string]bool{}
	isSeed := map[string]bool{}
	var frontier []string
	res := &NeighborResult{}
	for _, id := range seeds {
		n, err := s.GetNode(id)
		if err != nil {
			return nil, err
		}
		if n == nil || seen[id] {
			continue
		}
		seen[id] = true
		isSeed[id] = true
		frontier = append(frontier, id)
		res.Seeds = append(res.Seeds, id)
	}

	for d := 0; d < depth && len(frontier) > 0; d++ {
		var next []string
		for _, id := range frontier {
			edges, err := adjacent(s, id, dir)
			if err != nil {
				return nil, err
			}
			for _, e := range edges {
				if !keepRel(e.Relation) {
					continue
				}
				other := e.Target
				if other == id {
					other = e.Source
				}
				if seen[other] {
					continue
				}
				n, err := s.GetNode(other)
				if err != nil {
					return nil, err
				}
				// Kind and test filters apply to what expansion *reaches*, never
				// to the seeds: asking about a test function must still answer.
				if n == nil || (len(allowKind) > 0 && !allowKind[n.Kind]) || (!opts.IncludeTests && n.Test) {
					continue
				}
				seen[other] = true
				next = append(next, other)
			}
		}
		frontier = next
	}

	for id := range seen {
		n, err := s.GetNode(id)
		if err != nil {
			return nil, err
		}
		if n != nil {
			res.Nodes = append(res.Nodes, *n)
		}
	}
	// Seeds first, then most connected: a truncated neighbourhood should keep
	// the entities the rest of it hangs off.
	sort.Slice(res.Nodes, func(i, j int) bool {
		if a, b := isSeed[res.Nodes[i].ID], isSeed[res.Nodes[j].ID]; a != b {
			return a
		}
		if res.Nodes[i].Degree != res.Nodes[j].Degree {
			return res.Nodes[i].Degree > res.Nodes[j].Degree
		}
		return res.Nodes[i].ID < res.Nodes[j].ID
	})
	res.Total = len(res.Nodes)
	if len(res.Nodes) > limit {
		res.Nodes = res.Nodes[:limit]
		res.More = true
	}

	kept := make(map[string]bool, len(res.Nodes))
	for _, n := range res.Nodes {
		kept[n.ID] = true
	}
	if err := s.EachEdge(func(e model.Edge) error {
		if kept[e.Source] && kept[e.Target] && keepRel(e.Relation) {
			res.Edges = append(res.Edges, e)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Slice(res.Edges, func(i, j int) bool {
		a, b := res.Edges[i], res.Edges[j]
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.Relation != b.Relation {
			return a.Relation < b.Relation
		}
		return a.Target < b.Target
	})
	return res, nil
}

// adjacent returns the edges leaving, entering, or touching a node.
func adjacent(s *store.Store, id string, dir Direction) ([]model.Edge, error) {
	var edges []model.Edge
	if dir == Out || dir == Both {
		out, err := s.OutEdges(id)
		if err != nil {
			return nil, err
		}
		edges = append(edges, out...)
	}
	if dir == In || dir == Both {
		in, err := s.InEdges(id)
		if err != nil {
			return nil, err
		}
		edges = append(edges, in...)
	}
	return edges, nil
}

// EdgeQuery filters one entity's edges.
type EdgeQuery struct {
	Direction Direction
	Relations []model.Relation // empty = all
	Limit     int              // 0 = DefaultLimit
	// IncludeTests keeps edges whose other end is test code.
	IncludeTests bool
}

// EdgeHit is one edge with its resolved other end, so a caller never has to
// look the node up again just to print a name.
type EdgeHit struct {
	Edge  model.Edge `json:"edge"`
	Other model.Node `json:"other"`
	// Forward is true when Other is the edge's target.
	Forward bool `json:"forward"`
}

// EdgeResult is a bounded edge listing.
type EdgeResult struct {
	Node  model.Node `json:"node"`
	Hits  []EdgeHit  `json:"edges"`
	Total int        `json:"total"`
	More  bool       `json:"truncated"`
}

// Edges lists one entity's edges, filtered and capped.
//
// This is what answers "who calls X" deterministically. Explain returns every
// edge unfiltered, so an agent asking an impact question had to fetch a hub's
// entire adjacency — hundreds of edges carrying full node ids — and filter it
// itself, paying for every relation it did not want.
func Edges(s *store.Store, id string, q EdgeQuery) (*EdgeResult, error) {
	n, err := s.GetNode(id)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return nil, fmt.Errorf("no node %q", id)
	}
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	dir := q.Direction
	if dir == "" {
		dir = Both
	}
	allow := map[model.Relation]bool{}
	for _, r := range q.Relations {
		allow[r] = true
	}

	res := &EdgeResult{Node: *n}
	collect := func(edges []model.Edge, forward bool) error {
		for _, e := range edges {
			if len(allow) > 0 && !allow[e.Relation] {
				continue
			}
			otherID := e.Target
			if !forward {
				otherID = e.Source
			}
			other, err := s.GetNode(otherID)
			if err != nil {
				return err
			}
			if other == nil {
				other = &model.Node{ID: otherID, Label: otherID}
			} else if other.Test && !q.IncludeTests {
				continue
			}
			res.Hits = append(res.Hits, EdgeHit{Edge: e, Other: *other, Forward: forward})
		}
		return nil
	}
	if dir == Out || dir == Both {
		out, err := s.OutEdges(id)
		if err != nil {
			return nil, err
		}
		if err := collect(out, true); err != nil {
			return nil, err
		}
	}
	if dir == In || dir == Both {
		in, err := s.InEdges(id)
		if err != nil {
			return nil, err
		}
		if err := collect(in, false); err != nil {
			return nil, err
		}
	}
	// Most trustworthy first, then most connected, so a truncated list keeps
	// what a reader would have wanted.
	sort.SliceStable(res.Hits, func(i, j int) bool {
		a, b := res.Hits[i], res.Hits[j]
		if a.Edge.Relation != b.Edge.Relation {
			return a.Edge.Relation < b.Edge.Relation
		}
		if ca, cb := confRank(a.Edge.Confidence), confRank(b.Edge.Confidence); ca != cb {
			return ca > cb
		}
		if a.Other.Degree != b.Other.Degree {
			return a.Other.Degree > b.Other.Degree
		}
		return a.Other.ID < b.Other.ID
	})
	res.Total = len(res.Hits)
	if len(res.Hits) > limit {
		res.Hits = res.Hits[:limit]
		res.More = true
	}
	return res, nil
}

func confRank(c model.Confidence) int {
	switch c {
	case model.ConfExtracted:
		return 3
	case model.ConfInferred:
		return 2
	default:
		return 1
	}
}

// Rationale returns the recorded reasoning attached to an entity.
//
// Rationale edges point AT the entity, so this is an incoming `explains`
// filter — deterministic, unlike routing "why is X like this" through the
// natural-language planner, which was the only way to ask from the CLI.
func Rationale(s *store.Store, id string) (*EdgeResult, error) {
	return Edges(s, id, EdgeQuery{
		Direction: In,
		Relations: []model.Relation{model.RelExplains},
		Limit:     DefaultLimit,
		// A rationale written in a test file still explains its subject.
		IncludeTests: true,
	})
}

// Hop is one step along a path.
type Hop struct {
	Edge    model.Edge
	Forward bool // false when the edge is traversed against its direction
}

// Path finds a shortest undirected path between two nodes, reporting each
// hop's true edge direction. Returns nil when no path exists.
func Path(s *store.Store, fromID, toID string) ([]Hop, error) {
	if fromID == toID {
		return []Hop{}, nil
	}
	type visit struct {
		prev string
		hop  Hop
	}
	visited := map[string]visit{fromID: {}}
	frontier := []string{fromID}

	neighbors := func(id string) ([]Hop, error) {
		out, err := s.OutEdges(id)
		if err != nil {
			return nil, err
		}
		in, err := s.InEdges(id)
		if err != nil {
			return nil, err
		}
		hops := make([]Hop, 0, len(out)+len(in))
		for _, e := range out {
			hops = append(hops, Hop{Edge: e, Forward: true})
		}
		for _, e := range in {
			hops = append(hops, Hop{Edge: e, Forward: false})
		}
		return hops, nil
	}

	for len(frontier) > 0 {
		var next []string
		for _, id := range frontier {
			hops, err := neighbors(id)
			if err != nil {
				return nil, err
			}
			for _, h := range hops {
				other := h.Edge.Target
				if !h.Forward {
					other = h.Edge.Source
				}
				if _, seen := visited[other]; seen {
					continue
				}
				visited[other] = visit{prev: id, hop: h}
				if other == toID {
					// Reconstruct.
					var path []Hop
					for cur := toID; cur != fromID; {
						v := visited[cur]
						path = append([]Hop{v.hop}, path...)
						cur = v.prev
					}
					return path, nil
				}
				next = append(next, other)
			}
		}
		frontier = next
	}
	return nil, nil
}

// Card is an entity's full detail for explain output.
type Card struct {
	Node           model.Node
	CommunityLabel string
	Out            []model.Edge
	In             []model.Edge
	// Total and Truncated are set only by ExplainFiltered, so a caller can tell
	// a short edge list from a capped one.
	Total     int  `json:",omitempty"`
	Truncated bool `json:",omitempty"`
}

// ExplainFiltered assembles an entity card whose edge lists are filtered and
// capped. A hub's unfiltered card is hundreds of edges, each carrying two full
// node ids, which is the single most expensive thing an agent can ask for.
func ExplainFiltered(s *store.Store, id string, q EdgeQuery) (*Card, error) {
	card, err := Explain(s, id)
	if err != nil {
		return nil, err
	}
	res, err := Edges(s, id, q)
	if err != nil {
		return nil, err
	}
	card.Out, card.In = nil, nil
	for _, h := range res.Hits {
		if h.Forward {
			card.Out = append(card.Out, h.Edge)
		} else {
			card.In = append(card.In, h.Edge)
		}
	}
	card.Total, card.Truncated = res.Total, res.More
	return card, nil
}

// Explain assembles an entity card.
func Explain(s *store.Store, id string) (*Card, error) {
	n, err := s.GetNode(id)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return nil, fmt.Errorf("no node %q", id)
	}
	card := &Card{Node: *n}
	if card.Out, err = s.OutEdges(id); err != nil {
		return nil, err
	}
	if card.In, err = s.InEdges(id); err != nil {
		return nil, err
	}
	if err := s.EachCommunity(func(c store.Community) error {
		if c.ID == n.Community {
			card.CommunityLabel = c.Label
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return card, nil
}

// KeywordSearch ranks nodes by a case-insensitive match of the query terms
// over label > file > doc, weighted by connectivity. It is the offline
// fallback when no LLM backend is configured.
func KeywordSearch(s *store.Store, q string, limit int) ([]model.Node, error) {
	terms := strings.Fields(strings.ToLower(q))
	if len(terms) == 0 {
		return nil, nil
	}
	type scored struct {
		n     model.Node
		score float64
	}
	var hits []scored
	err := s.EachNode(func(n model.Node) error {
		label := strings.ToLower(n.Label)
		file := strings.ToLower(n.File)
		doc := strings.ToLower(n.Doc)
		// Rationale prose lives in Body, and it is the whole point of a "why
		// do we ..." question — searching it is what makes such questions work
		// offline, without the LLM seed planner.
		body := strings.ToLower(n.Body)
		score := 0.0
		for _, t := range terms {
			switch {
			case label == t:
				score += 10
			case strings.Contains(label, t):
				score += 5
			case strings.Contains(file, t):
				score += 2
			case strings.Contains(doc, t), strings.Contains(body, t):
				score += 1
			}
		}
		if score > 0 {
			score *= 1 + math.Log1p(float64(n.Degree))
			hits = append(hits, scored{n, score})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].n.ID < hits[j].n.ID
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]model.Node, len(hits))
	for i, h := range hits {
		out[i] = h.n
	}
	return out, nil
}
