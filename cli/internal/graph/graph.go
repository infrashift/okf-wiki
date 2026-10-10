package graph

import (
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
)

// Node represents a concept in the graph.
type Node struct {
	ID       string
	Title    string
	FilePath string
	Project  string
	Parent   string   // Optional parent concept ID
	Children []string // Child concept IDs
	Links    []string // Concept keys of its link targets — body links, then path-valued frontmatter fields; a missing target has no node
	Tags     []string // Frontmatter tags, for tag queries
}

// Graph represents the bundle link graph.
type Graph struct {
	nodes     map[string]*Node
	backlinks map[string][]string // target ID → IDs of nodes whose Links contain it
}

// New creates a new empty graph.
func New() *Graph {
	return &Graph{
		nodes: make(map[string]*Node),
	}
}

// AddNode adds a node to the graph.
func (g *Graph) AddNode(node *Node) {
	g.nodes[node.ID] = node
}

// GetNode returns a node by ID.
func (g *Graph) GetNode(id string) *Node {
	return g.nodes[id]
}

// NodeCount returns the number of nodes in the graph.
func (g *Graph) NodeCount() int {
	return len(g.nodes)
}

// AllNodes returns all nodes in the graph, in id order.
func (g *Graph) AllNodes() []*Node {
	nodes := make([]*Node, 0, len(g.nodes))
	for _, id := range g.sortedIDs() {
		nodes = append(nodes, g.nodes[id])
	}
	return nodes
}

// sortedIDs returns every node id in order. Anything whose output depends on
// the order nodes are visited in goes through it rather than ranging over the
// map, whose order Go randomises on every range: a capped query reached a
// different subset of a hub's neighbours on every run.
func (g *Graph) sortedIDs() []string {
	ids := make([]string, 0, len(g.nodes))
	for id := range g.nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// BuildRelationships processes parent fields and builds bidirectional
// relationships, including the reverse-link index used by FindConnected.
// Idempotent: safe to call again after adding nodes. Children and backlinks
// are recorded in id order, which is the order FindConnected visits them in.
func (g *Graph) BuildRelationships() {
	for _, node := range g.nodes {
		node.Children = nil
	}
	g.backlinks = make(map[string][]string, len(g.nodes))
	for _, id := range g.sortedIDs() {
		node := g.nodes[id]
		// Add this node as a child of its parent
		if node.Parent != "" {
			if parent, ok := g.nodes[node.Parent]; ok {
				parent.Children = append(parent.Children, node.ID)
			}
		}
		// Index reverse links (who links TO each target)
		for _, linkID := range node.Links {
			g.backlinks[linkID] = append(g.backlinks[linkID], node.ID)
		}
	}
}

// backlinksTo returns the IDs of nodes whose Links point at id, building the
// index on first use if BuildRelationships hasn't been called.
func (g *Graph) backlinksTo(id string) []string {
	if g.backlinks == nil {
		g.BuildRelationships()
	}
	return g.backlinks[id]
}

// bfsItem tracks a node ID and its distance from the start node.
type bfsItem struct {
	id    string
	depth int
}

// FindConnected finds all nodes connected to the given node ID up to the limit.
// Uses BFS to find the closest related nodes first.
// If maxDepth > 0, limits traversal to that many hops from the start node.
func (g *Graph) FindConnected(startID string, limit int, maxDepth int) []*Node {
	if limit <= 0 {
		limit = 10
	}

	startNode := g.nodes[startID]
	if startNode == nil {
		return nil
	}

	visited := make(map[string]bool)
	result := make([]*Node, 0, limit)
	queue := []bfsItem{{id: startID, depth: 0}}

	for len(queue) > 0 && len(result) < limit {
		current := queue[0]
		queue = queue[1:]

		if visited[current.id] {
			continue
		}
		visited[current.id] = true

		node := g.nodes[current.id]
		if node == nil {
			continue
		}

		result = append(result, node)

		// Stop expanding if we've reached max depth
		if maxDepth > 0 && current.depth >= maxDepth {
			continue
		}

		nextDepth := current.depth + 1

		// Add connected nodes to queue
		// Parent
		if node.Parent != "" && !visited[node.Parent] {
			queue = append(queue, bfsItem{id: node.Parent, depth: nextDepth})
		}
		// Children
		for _, childID := range node.Children {
			if !visited[childID] {
				queue = append(queue, bfsItem{id: childID, depth: nextDepth})
			}
		}
		// Explicit links
		for _, linkID := range node.Links {
			if !visited[linkID] {
				queue = append(queue, bfsItem{id: linkID, depth: nextDepth})
			}
		}
		// Reverse links (notes that link TO this node), via the index
		for _, srcID := range g.backlinksTo(node.ID) {
			if !visited[srcID] {
				queue = append(queue, bfsItem{id: srcID, depth: nextDepth})
			}
		}
	}

	return result
}

// FindAllConnected returns all connected components starting from any node.
func (g *Graph) FindAllConnected(limit int, maxDepth int) []*Node {
	if limit <= 0 {
		limit = 10
	}

	if len(g.nodes) == 0 {
		return nil
	}

	// Start from the node with the most connections for a meaningful graph;
	// among equals, the lowest id.
	var startID string
	maxConns := -1
	for _, id := range g.sortedIDs() {
		node := g.nodes[id]
		// Live links only: a link to a missing concept connects nothing.
		conns := len(node.Children)
		for _, l := range node.Links {
			if g.nodes[l] != nil {
				conns++
			}
		}
		if node.Parent != "" {
			conns++
		}
		if conns > maxConns {
			maxConns = conns
			startID = id
		}
	}

	return g.FindConnected(startID, limit, maxDepth)
}

// Edge represents a directed edge in the graph.
type Edge struct {
	From  string
	To    string
	Label string // "parent", "child", "link"
}

// GetEdges returns all edges for the given nodes.
func (g *Graph) GetEdges(nodes []*Node) []Edge {
	nodeSet := make(map[string]bool)
	for _, n := range nodes {
		nodeSet[n.ID] = true
	}

	var edges []Edge
	seen := make(map[string]bool)

	for _, node := range nodes {
		// Parent edge (child -> parent)
		if node.Parent != "" && nodeSet[node.Parent] {
			key := node.ID + "->" + node.Parent
			if !seen[key] {
				edges = append(edges, Edge{From: node.ID, To: node.Parent, Label: "parent"})
				seen[key] = true
			}
		}

		// Link edges
		for _, linkID := range node.Links {
			if nodeSet[linkID] {
				key := node.ID + "->" + linkID
				if !seen[key] {
					edges = append(edges, Edge{From: node.ID, To: linkID, Label: "link"})
					seen[key] = true
				}
			}
		}
	}

	return edges
}

// GenerateASCIITree renders nodes and edges as a Unicode box-drawing tree.
//
// The root is startNodeID (if provided and present), otherwise the most-connected
// node. Children are sorted by ID for stable output. Directional markers show
// where the link lives: "←" means the tree-child links back to its
// tree-parent; "→" means the tree-parent links forward to the tree-child.
// Cross-edges (links between nodes already placed in the tree) are shown as
// inline annotations, e.g. "(also → Node B)". Disconnected nodes appear as
// separate single-line roots at the bottom.
func GenerateASCIITree(nodes []*Node, edges []Edge, startNodeID string) string {
	if len(nodes) == 0 {
		return "(no nodes found)"
	}

	// Build lookup map
	nodeByID := make(map[string]*Node)
	for _, n := range nodes {
		nodeByID[n.ID] = n
	}

	// Build adjacency: outgoing children per node, with edge label + direction
	type child struct {
		id      string
		label   string
		reverse bool // true when the other node links TO this one
	}
	outgoing := make(map[string][]child) // parent → children (forward edges)
	incoming := make(map[string][]child) // target → sources  (reverse edges)

	for _, e := range edges {
		outgoing[e.From] = append(outgoing[e.From], child{id: e.To, label: e.Label})
		incoming[e.To] = append(incoming[e.To], child{id: e.From, label: e.Label, reverse: true})
	}

	// Pick root
	rootID := startNodeID
	if rootID == "" || nodeByID[rootID] == nil {
		// Most-connected node
		maxConns := -1
		for _, n := range nodes {
			conns := len(outgoing[n.ID]) + len(incoming[n.ID])
			if conns > maxConns || (conns == maxConns && n.ID < rootID) {
				maxConns = conns
				rootID = n.ID
			}
		}
	}

	// BFS spanning tree from root
	type treeChild struct {
		id      string
		label   string
		reverse bool
	}
	treeChildren := make(map[string][]treeChild)
	visited := make(map[string]bool)

	queue := []string{rootID}
	visited[rootID] = true

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		// Collect all neighbours: forward edges + reverse edges
		var neighbours []treeChild
		for _, c := range outgoing[cur] {
			if !visited[c.id] {
				neighbours = append(neighbours, treeChild{id: c.id, label: c.label})
			}
		}
		for _, c := range incoming[cur] {
			if !visited[c.id] {
				neighbours = append(neighbours, treeChild{id: c.id, label: c.label, reverse: true})
			}
		}

		// Sort by ID for deterministic output. Stable, so a neighbour that is
		// both linked to and linking back keeps its forward entry, which comes
		// first and wins below: the link lives in the tree parent.
		sort.SliceStable(neighbours, func(i, j int) bool {
			return neighbours[i].id < neighbours[j].id
		})

		for _, nb := range neighbours {
			if !visited[nb.id] {
				visited[nb.id] = true
				treeChildren[cur] = append(treeChildren[cur], nb)
				queue = append(queue, nb.id)
			}
		}
	}

	// Render tree
	var sb strings.Builder
	nodeTitle := func(id string) string {
		if n := nodeByID[id]; n != nil && n.Title != "" {
			return n.Title
		}
		return id
	}

	// Identify cross-edges: edges not captured in the spanning tree.
	// These occur when a node links to another node that was already visited
	// during BFS, so the edge couldn't be represented as a tree branch.
	treeEdgeSet := make(map[string]bool)
	for parentID, children := range treeChildren {
		for _, ch := range children {
			if ch.reverse {
				treeEdgeSet[ch.id+"->"+parentID] = true
			} else {
				treeEdgeSet[parentID+"->"+ch.id] = true
			}
		}
	}
	crossLinks := make(map[string][]string) // source ID → target titles
	for _, e := range edges {
		key := e.From + "->" + e.To
		if !treeEdgeSet[key] && visited[e.From] && visited[e.To] {
			crossLinks[e.From] = append(crossLinks[e.From], nodeTitle(e.To))
		}
	}
	for id := range crossLinks {
		sort.Strings(crossLinks[id])
	}

	crossAnnotation := func(id string) string {
		if targets := crossLinks[id]; len(targets) > 0 {
			return fmt.Sprintf(" (also → %s)", strings.Join(targets, ", "))
		}
		return ""
	}

	// Root line (no label)
	sb.WriteString(fmt.Sprintf("%s %s%s\n", rootID, nodeTitle(rootID), crossAnnotation(rootID)))

	// Recursive render
	var render func(parentID string, prefix string)
	render = func(parentID string, prefix string) {
		children := treeChildren[parentID]
		for i, ch := range children {
			isLast := i == len(children)-1
			connector := "├── "
			childPrefix := "│   "
			if isLast {
				connector = "└── "
				childPrefix = "    "
			}

			label := fmt.Sprintf(" [%s]", ch.label)
			marker := ""
			if ch.reverse {
				marker = " ←"
			} else if ch.label == "link" {
				marker = " →"
			}

			sb.WriteString(fmt.Sprintf("%s%s%s %s%s%s%s\n",
				prefix, connector, ch.id, nodeTitle(ch.id), marker, label, crossAnnotation(ch.id)))

			render(ch.id, prefix+childPrefix)
		}
	}
	render(rootID, "")

	// Disconnected nodes as separate roots
	for _, n := range nodes {
		if !visited[n.ID] {
			sb.WriteString(fmt.Sprintf("%s %s\n", n.ID, nodeTitle(n.ID)))
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

// GenerateMermaid renders nodes and edges as Mermaid flowchart syntax.
//
// Output can be pasted into any Mermaid renderer (mermaid.live, GitHub markdown,
// NeoVim plugins). Arrow direction: A --> B means A links to B.
func GenerateMermaid(nodes []*Node, edges []Edge) string {
	var sb strings.Builder
	sb.WriteString("graph LR;")

	// Sort nodes for deterministic output
	sortedNodes := make([]*Node, len(nodes))
	copy(sortedNodes, nodes)
	sort.Slice(sortedNodes, func(i, j int) bool {
		return sortedNodes[i].ID < sortedNodes[j].ID
	})

	// Declare all nodes with their titles
	for _, n := range sortedNodes {
		title := n.Title
		if title == "" {
			title = n.ID
		}
		title = strings.ReplaceAll(title, `"`, "'")
		sb.WriteString(fmt.Sprintf("\n  %s[\"%s\"];", n.ID, title))
	}

	// Sort edges for deterministic output
	sortedEdges := make([]Edge, len(edges))
	copy(sortedEdges, edges)
	sort.Slice(sortedEdges, func(i, j int) bool {
		if sortedEdges[i].From != sortedEdges[j].From {
			return sortedEdges[i].From < sortedEdges[j].From
		}
		return sortedEdges[i].To < sortedEdges[j].To
	})

	for _, e := range sortedEdges {
		sb.WriteString(fmt.Sprintf("\n  %s --> %s;", e.From, e.To))
	}

	return sb.String()
}

// --- Vanilla markdown link helpers (OKF) ---

// ExtractLinkTargets returns the deduplicated destinations of internal markdown
// links in body, in order of first appearance and as written (still
// percent-encoded, fragment removed). External links are excluded. See
// linkDests for what counts as a link.
func ExtractLinkTargets(body string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, d := range linkDests(body) {
		href := body[d.start:d.end]
		if !seen[href] {
			seen[href] = true
			out = append(out, href)
		}
	}
	return out
}

// NormalizeTarget converts a markdown href into a clean bundle-relative path
// (forward slashes, no leading slash, percent-decoded). Relative hrefs resolve against fromDir,
// the linking file's bundle-relative directory. Returns "" for external links.
func NormalizeTarget(href, fromDir string) string {
	if strings.Contains(href, "://") {
		return ""
	}
	if i := strings.IndexByte(href, '#'); i >= 0 {
		href = href[:i]
	}
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	if u, err := url.PathUnescape(href); err == nil {
		href = u
	}
	var p string
	if strings.HasPrefix(href, "/") {
		p = path.Clean(strings.TrimPrefix(href, "/"))
	} else {
		p = path.Clean(path.Join(fromDir, href))
	}
	// A link that climbs out of the bundle names nothing in it, and resolving
	// it anyway made readers open files outside the bundle.
	if p == ".." || strings.HasPrefix(p, "../") {
		return ""
	}
	return strings.TrimPrefix(p, "/")
}

// NormalizeRef resolves a path-valued frontmatter field (OKF section 6.2) the
// way NormalizeTarget resolves a body link, and returns "" unless it names a
// markdown file inside the bundle. The same field may hold a URL, a scope
// descriptor ("all queries in project X") or a non-concept file such as an
// attester's .py, and none of those is an edge between concepts.
func NormalizeRef(value, fromDir string) string {
	v := strings.TrimSpace(value)
	if i := strings.IndexByte(v, '#'); i >= 0 {
		v = v[:i]
	}
	if !isInternalMarkdown(v) {
		return ""
	}
	return NormalizeTarget(v, fromDir)
}

// RewriteLinks applies rewrite to the destination of every internal markdown
// link in body. rewrite receives the destination as written (fragment removed)
// and returns a replacement path; returning "" or the same value leaves the
// link unchanged. The fragment, any title and the link text are kept.
//
// The replacement is spliced at the destination's own offsets rather than by
// substring substitution: when a link's text equals its target —
// "[/dir/a.md](/dir/a.md)", which is exactly what the plugin's link-insert
// keymap produces — replacing the first occurrence of the href rewrote the
// visible text and left the actual target untouched.
//
// A replacement containing characters that end a bare destination (spaces,
// parentheses, angle brackets) is percent-encoded, unless the original was
// written in angle brackets, where spaces are legal.
func RewriteLinks(body string, rewrite func(href string) string) string {
	dests := linkDests(body)
	if len(dests) == 0 {
		return body
	}
	var b strings.Builder
	last := 0
	for _, d := range dests {
		href := body[d.start:d.end]
		nh := rewrite(href)
		if nh == "" || nh == href {
			continue
		}
		if d.angled {
			nh = strings.NewReplacer("<", "%3C", ">", "%3E", "\n", "%0A").Replace(nh)
		} else {
			nh = encodeDest(nh)
		}
		b.WriteString(body[last:d.start])
		b.WriteString(nh)
		last = d.end
	}
	if last == 0 {
		return body
	}
	b.WriteString(body[last:])
	return b.String()
}

// encodeDest percent-encodes the characters that would end or break a bare
// link destination.
func encodeDest(s string) string {
	return strings.NewReplacer(" ", "%20", "\t", "%09", "(", "%28", ")", "%29", "<", "%3C", ">", "%3E").Replace(s)
}
