package main

// MCP (Model Context Protocol) stdio server: `graphy mcp`.
//
// Same design as okfw's server (cli/cmd/okfw/mcp.go): newline-delimited
// JSON-RPC 2.0 over stdio, tools-only, no SDK dependency, terse tool
// descriptions because schemas ride in every agent context. The store is
// opened per tool call so a long-lived server never holds the bbolt lock
// against a concurrent `graphy build`.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"okfw/cli/internal/graphy/build"
	"okfw/cli/internal/graphy/llm"
	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/query"
	"okfw/cli/internal/graphy/store"
	"okfw/cli/internal/graphy/workspace"
)

// Aliases keep the tool handlers terse.
type (
	queryNode = model.Node
	queryEdge = model.Edge
)

const mcpProtocolVersion = "2024-11-05"

// maxRPCLine caps one JSON-RPC request line.
const maxRPCLine = 4 * 1024 * 1024

// toolCallTimeout bounds a single tool call. The serve loop is sequential, so
// without it one hung LLM request wedges the whole server with no way to
// cancel from the client side.
const toolCallTimeout = 90 * time.Second

func graphyVersion() string { return version }

func mcpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve graph queries over MCP (stdio)",
		Long: `Runs a Model Context Protocol stdio server exposing graphy_query,
graphy_path, graphy_explain, graphy_rationale, graphy_report, and graphy_update.
Register it in an agent's MCP config with command "graphy" and args ["mcp"].`,
		RunE: func(cmd *cobra.Command, args []string) error {
			srv := &mcpServer{out: os.Stdout}
			return srv.serve(os.Stdin)
		},
	}
}

type mcpServer struct {
	startDir string // workspace discovery start; "" = process cwd
	out      io.Writer
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type toolResult struct {
	Content []toolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func textResult(text string) *toolResult {
	return &toolResult{Content: []toolContent{{Type: "text", Text: text}}}
}

func jsonResult(v interface{}) *toolResult {
	data, err := json.Marshal(v)
	if err != nil {
		return errResult(fmt.Errorf("encode result: %w", err))
	}
	return textResult(string(data))
}

func errResult(err error) *toolResult {
	r := textResult(err.Error())
	r.IsError = true
	return r
}

func mcpTools() []map[string]interface{} {
	obj := func(props map[string]interface{}, required ...string) map[string]interface{} {
		s := map[string]interface{}{"type": "object", "properties": props}
		if len(required) > 0 {
			s["required"] = required
		}
		return s
	}
	str := func(desc string) map[string]interface{} {
		return map[string]interface{}{"type": "string", "description": desc}
	}
	num := func(desc string) map[string]interface{} {
		return map[string]interface{}{"type": "integer", "description": desc}
	}
	boolean := func(desc string) map[string]interface{} {
		return map[string]interface{}{"type": "boolean", "description": desc}
	}
	strs := func(desc string) map[string]interface{} {
		return map[string]interface{}{
			"type": "array", "items": map[string]interface{}{"type": "string"},
			"description": desc,
		}
	}

	return []map[string]interface{}{
		{
			"name":        "graphy_query",
			"description": "Ask the code knowledge graph a question. Returns relevant entities + relationships; edges index into `nodes` by position. " +
				"Capped at `limit` nodes — `truncated` means raise limit or narrow the question. Prefer this over scanning files for architecture/impact questions.",
			"inputSchema": obj(map[string]interface{}{
				"question": str("natural-language question about the codebase"),
				"depth":    num("expansion depth 1-2 (default 1)"),
				"limit":    num("maximum nodes (default 50)"),
				"tests":    boolean("include entities declared in test files (default false)"),
			}, "question"),
		},
		{
			"name":        "graphy_path",
			"description": "Shortest path between two code entities (by name or id). Shows how they connect.",
			"inputSchema": obj(map[string]interface{}{
				"from": str("entity name or id"),
				"to":   str("entity name or id"),
			}, "from", "to"),
		},
		{
			"name":        "graphy_explain",
			"description": "Entity card: location, signature, doc, community, and its edges with confidence. Each edge names only its other end (`node`); this entity is the constant side. Narrow with relations/direction on hub entities.",
			"inputSchema": obj(map[string]interface{}{
				"entity":    str("entity name, node id, or repo-relative file path"),
				"relations": strs("restrict to these relations, e.g. [\"calls\"]"),
				"direction": str("out, in, or both (default both)"),
				"limit":     num("maximum edges (default 50)"),
				"tests":     boolean("include entities declared in test files (default false)"),
			}, "entity"),
		},
		{
			"name": "graphy_neighbors",
			"description": "Entities around one you already know, to a bounded depth, filtered by relation and kind. Edges index into `nodes` by position. " +
				"Deterministic — prefer this over graphy_query when you know the starting entity.",
			"inputSchema": obj(map[string]interface{}{
				"entity":    str("entity name, node id, or repo-relative file path"),
				"depth":     num("expansion depth 1-2 (default 1)"),
				"direction": str("out, in, or both (default both)"),
				"relations": strs("restrict to these relations"),
				"kinds":     strs("restrict to these node kinds, e.g. [\"function\"]"),
				"limit":     num("maximum nodes (default 50)"),
				"tests":     boolean("include entities declared in test files (default false)"),
			}, "entity"),
		},
		{
			"name":        "graphy_callers",
			"description": "Impact analysis: what calls, imports, implements, tests or documents an entity. Ask before changing it.",
			"inputSchema": obj(map[string]interface{}{
				"entity":    str("entity name, node id, or repo-relative file path"),
				"relations": strs("restrict to these relations"),
				"limit":     num("maximum results (default 50)"),
				"tests":     boolean("include callers declared in test files (default false)"),
			}, "entity"),
		},
		{
			"name":        "graphy_file_nodes",
			"description": "The entities declared in a repo-relative file, in source order. Use this to enter the graph from a file you are editing.",
			"inputSchema": obj(map[string]interface{}{
				"file": str("repo-relative file path"),
			}, "file"),
		},
		{
			"name": "graphy_concept",
			"description": "The wiki concept documenting an entity: its OKF id and bundle path. " +
				"A repo-relative file path returns every concept documented in that file. " +
				"Use it to cross from the code graph into the wiki, then read the concept with okfw.",
			"inputSchema": obj(map[string]interface{}{
				"entity": str("entity name, node id, or repo-relative file path"),
			}, "entity"),
		},
		{
			"name": "graphy_community",
			"description": "A subsystem: its members most connected first, and the concept documenting it. " +
				"Communities are not nodes, so graphy_explain cannot answer for one — pass the `community:<slug>` node id okfw search reports here instead.",
			"inputSchema": obj(map[string]interface{}{
				"community": str("community label, slug, or community:<slug> node id"),
				"limit":     num("maximum members (default 50)"),
			}, "community"),
		},
		{
			"name":        "graphy_rationale",
			"description": "Why an entity is the way it is: the WHY/DECISION/HACK comments and architecture decision records attached to it. Read this before changing unfamiliar code.",
			"inputSchema": obj(map[string]interface{}{
				"entity": str("entity name or id"),
			}, "entity"),
		},
		{
			"name":        "graphy_report",
			"description": "The repository's GRAPH_REPORT.md: stats, god nodes, communities. Read before architecture questions. Pass a section to avoid the whole document.",
			"inputSchema": obj(map[string]interface{}{
				"section": str("print only this section, by heading text; omit for the whole report"),
			}),
		},
		{
			"name":        "graphy_update",
			"description": "Incrementally refresh the graph after code changes. Run after editing files.",
			"inputSchema": obj(map[string]interface{}{}),
		},
	}
}

func (s *mcpServer) serve(in io.Reader) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), maxRPCLine)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		s.emit(s.handleLine(line))
	}
	// A line over the buffer cap is one client's mistake, not a reason to take
	// the server down: report it and keep serving. Scanner cannot continue
	// after ErrTooLong, so the connection still ends — but the client learns
	// why instead of watching the process vanish.
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			s.emit(&rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"),
				Error: &rpcError{Code: -32700, Message: fmt.Sprintf("request exceeds %d bytes", maxRPCLine)}})
		}
		return err
	}
	return nil
}

// emit writes one response, falling back to a JSON-RPC error when the response
// itself cannot be marshalled. Dropping it instead would leave the client
// waiting forever on a request id that never gets an answer.
func (s *mcpServer) emit(resp *rpcResponse) {
	if resp == nil {
		return
	}
	data, err := json.Marshal(resp)
	if err != nil {
		fallback := &rpcResponse{JSONRPC: "2.0", ID: resp.ID,
			Error: &rpcError{Code: -32603, Message: "internal error: " + err.Error()}}
		if data, err = json.Marshal(fallback); err != nil {
			return // cannot happen: the fallback is plain strings
		}
	}
	fmt.Fprintf(s.out, "%s\n", data)
}

func (s *mcpServer) handleLine(line string) *rpcResponse {
	var req rpcRequest
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		return &rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"),
			Error: &rpcError{Code: -32700, Message: "parse error"}}
	}
	if req.ID == nil {
		return nil // notification
	}
	resp := &rpcResponse{JSONRPC: "2.0", ID: req.ID}

	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		version := mcpProtocolVersion
		if p.ProtocolVersion != "" {
			version = p.ProtocolVersion
		}
		resp.Result = map[string]interface{}{
			"protocolVersion": version,
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
			"serverInfo":      map[string]interface{}{"name": "graphy", "version": graphyVersion()},
		}
	case "ping":
		resp.Result = map[string]interface{}{}
	case "tools/list":
		resp.Result = map[string]interface{}{"tools": mcpTools()}
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			resp.Error = &rpcError{Code: -32602, Message: "invalid params"}
			break
		}
		resp.Result = s.callTool(p.Name, p.Arguments)
	default:
		resp.Error = &rpcError{Code: -32601, Message: "method not found: " + req.Method}
	}
	return resp
}

// withStore discovers the workspace and opens the store for one tool call.
func (s *mcpServer) withStore(fn func(*workspace.Workspace, *store.Store) *toolResult) *toolResult {
	start := s.startDir
	if start == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return errResult(err)
		}
		start = cwd
	}
	w, err := workspace.Discover(start)
	if err != nil {
		return errResult(err)
	}
	if _, err := os.Stat(w.DBPath()); err != nil {
		return errResult(fmt.Errorf("no graph database at %s (run `graphy build`)", w.DBPath()))
	}
	st, err := store.Open(w.DBPath())
	if err != nil {
		return errResult(err)
	}
	defer st.Close()
	return fn(w, st)
}

func (s *mcpServer) callTool(name string, args json.RawMessage) *toolResult {
	if args == nil {
		args = json.RawMessage("{}")
	}
	switch name {
	case "graphy_query":
		return s.toolQuery(args)
	case "graphy_path":
		return s.toolPath(args)
	case "graphy_explain":
		return s.toolExplain(args)
	case "graphy_neighbors":
		return s.toolNeighbors(args)
	case "graphy_callers":
		return s.toolCallers(args)
	case "graphy_file_nodes":
		return s.toolFileNodes(args)
	case "graphy_concept":
		return s.toolConcept(args)
	case "graphy_community":
		return s.toolCommunity(args)
	case "graphy_rationale":
		return s.toolRationale(args)
	case "graphy_report":
		return s.toolReport(args)
	case "graphy_update":
		return s.toolUpdate()
	default:
		return errResult(fmt.Errorf("unknown tool: %s", name))
	}
}

// mcpNode is the compact node shape returned to agents.
type mcpNode struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Kind   string `json:"kind"`
	File   string `json:"file,omitempty"`
	Line   int    `json:"line,omitempty"`
	Doc    string `json:"doc,omitempty"`
	Body   string `json:"body,omitempty"` // rationale prose, clipped
	Degree int    `json:"degree,omitempty"`
	Test   bool   `json:"test,omitempty"`
	// Concept is the wiki concept documenting this entity, when it was
	// exported. Carried on every node so an agent never has to make a second
	// call just to find out whether there is prose to read.
	Concept string `json:"concept,omitempty"`
}

// mcpEdge references its endpoints by index into the same response's `nodes`
// array.
//
// Two full node ids per edge was two thirds of every subgraph payload — a
// depth-2 query on this repository's own graph carried 111KB of edges against
// 56KB of nodes, and every one of those id strings was already sitting in
// `nodes`. Edges are always induced over the nodes that survived the cap, so an
// index is always defined.
type mcpEdge struct {
	From       int    `json:"from"`
	Rel        string `json:"rel"`
	To         int    `json:"to"`
	Confidence string `json:"conf"`
}

// mcpCardEdge is an edge on an entity card, which names only the other end.
//
// A card's In and Out lists come from InEdges/OutEdges on one node, so every
// Out edge has from == the entity and every In edge has to == the entity. The
// old shape emitted that constant once per edge, up to the 50-edge cap.
type mcpCardEdge struct {
	Rel        string `json:"rel"`
	Node       string `json:"node"` // the other end; the entity is the card's `id`
	Confidence string `json:"conf"`
}

// nodeIndex maps node id to position in a rendered node list, for mcpEdge.
func nodeIndex(nodes []mcpNode) map[string]int {
	idx := make(map[string]int, len(nodes))
	for i, n := range nodes {
		idx[n.ID] = i
	}
	return idx
}

// compactEdges projects edges to index form, dropping any edge whose endpoints
// did not both survive the node cap.
func compactEdges(edges []queryEdge, idx map[string]int) []mcpEdge {
	out := make([]mcpEdge, 0, len(edges))
	for _, e := range edges {
		from, ok := idx[e.Source]
		if !ok {
			continue
		}
		to, ok := idx[e.Target]
		if !ok {
			continue
		}
		out = append(out, mcpEdge{From: from, Rel: string(e.Relation), To: to,
			Confidence: string(e.Confidence)})
	}
	return out
}

// maxNodeBody caps the rationale prose carried inline on a node.
//
// Body is unbounded — a rationale node holds up to 2000 characters of comment —
// and a depth-2 expansion can return dozens of them. Truncating here keeps the
// summary useful while leaving the full text one graphy_rationale call away.
const maxNodeBody = 400

func compactNodes(nodes []queryNode) []mcpNode {
	return compactNodesWith(nodes, nil)
}

// compactNodesWith projects nodes to the wire shape, annotating each with the
// concept documenting it when a concept index is supplied.
func compactNodesWith(nodes []queryNode, concepts map[string]query.Concept) []mcpNode {
	out := make([]mcpNode, len(nodes))
	for i, n := range nodes {
		out[i] = mcpNode{
			ID: n.ID, Label: n.Label, Kind: string(n.Kind),
			File: n.File, Line: n.Span.StartLine, Doc: n.Doc,
			Body: clip(n.Body, maxNodeBody), Degree: n.Degree, Test: n.Test,
		}
		if c, ok := concepts[n.ID]; ok {
			out[i].Concept = c.RelPath
		}
	}
	return out
}

// conceptIndex loads the node→concept mapping, tolerating its absence: a repo
// that has never been exported simply has no concepts, which is not an error.
func conceptIndex(st *store.Store) map[string]query.Concept {
	idx, err := query.ConceptIndex(st)
	if err != nil {
		return nil
	}
	return idx
}

// clip truncates on a rune boundary, marking that it did.
func clip(s string, max int) string {
	if len(s) <= max {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return strings.TrimRight(string(r[:max]), " \n\t") + "…"
}

func (s *mcpServer) toolQuery(args json.RawMessage) *toolResult {
	var p struct {
		Question string `json:"question"`
		Depth    int    `json:"depth"`
		Limit    int    `json:"limit"`
		Tests    bool   `json:"tests"`
	}
	if err := json.Unmarshal(args, &p); err != nil || p.Question == "" {
		return errResult(fmt.Errorf("question is required"))
	}
	if p.Depth != 0 && (p.Depth < 1 || p.Depth > query.MaxDepth) {
		return errResult(fmt.Errorf("depth must be between 1 and %d", query.MaxDepth))
	}
	if p.Limit < 0 {
		return errResult(fmt.Errorf("limit must not be negative"))
	}
	return s.withStore(func(w *workspace.Workspace, st *store.Store) *toolResult {
		backend, err := llm.FromEnv("")
		if err != nil && err != llm.ErrNoBackend {
			return errResult(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), toolCallTimeout)
		defer cancel()
		res, err := query.NL(ctx, st, backend, p.Question, query.NLOptions{
			Depth: p.Depth, Limit: p.Limit, IncludeTests: p.Tests,
		})
		if err != nil {
			return errResult(err)
		}
		nodes := compactNodesWith(res.Nodes, conceptIndex(st))
		out := map[string]interface{}{
			"mode":  res.Mode,
			"nodes": nodes,
			"edges": compactEdges(res.Edges, nodeIndex(nodes)),
		}
		if res.More {
			out["truncated"] = true
			out["total"] = res.Total
		}
		return jsonResult(out)
	})
}

func (s *mcpServer) toolPath(args json.RawMessage) *toolResult {
	var p struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.Unmarshal(args, &p); err != nil || p.From == "" || p.To == "" {
		return errResult(fmt.Errorf("from and to are required"))
	}
	return s.withStore(func(w *workspace.Workspace, st *store.Store) *toolResult {
		from, err := query.ResolveEntity(st, p.From)
		if err != nil {
			return errResult(err)
		}
		to, err := query.ResolveEntity(st, p.To)
		if err != nil {
			return errResult(err)
		}
		hops, err := query.Path(st, from.ID, to.ID)
		if err != nil {
			return errResult(err)
		}
		if hops == nil {
			return textResult(fmt.Sprintf("no path between %s and %s", from.Label, to.Label))
		}
		var b strings.Builder
		b.WriteString(from.ID)
		for _, h := range hops {
			if h.Forward {
				fmt.Fprintf(&b, " -[%s,%s]-> %s", h.Edge.Relation, h.Edge.Confidence, h.Edge.Target)
			} else {
				fmt.Fprintf(&b, " <-[%s,%s]- %s", h.Edge.Relation, h.Edge.Confidence, h.Edge.Source)
			}
		}
		return textResult(b.String())
	})
}

// traverseParams are the filters the deterministic traversal tools share.
type traverseParams struct {
	Entity    string   `json:"entity"`
	File      string   `json:"file"`
	Depth     int      `json:"depth"`
	Direction string   `json:"direction"`
	Relations []string `json:"relations"`
	Kinds     []string `json:"kinds"`
	Limit     int      `json:"limit"`
	Tests     bool     `json:"tests"`
}

// edgeQuery converts the wire params into a query, validating the vocabulary so
// a misspelled relation is an error rather than a silently unfiltered result.
func (p traverseParams) edgeQuery(dir query.Direction) (query.EdgeQuery, error) {
	rels, err := query.ParseRelations(p.Relations)
	if err != nil {
		return query.EdgeQuery{}, err
	}
	if p.Direction != "" {
		if dir, err = query.ParseDirection(p.Direction); err != nil {
			return query.EdgeQuery{}, err
		}
	}
	return query.EdgeQuery{
		Direction: dir, Relations: rels, Limit: p.Limit, IncludeTests: p.Tests,
	}, nil
}

// compactCardEdges projects one entity's edges, naming only the other end.
// `self` is the entity the card is about, which sits on the constant side.
func compactCardEdges(list []queryEdge, self string) []mcpCardEdge {
	out := make([]mcpCardEdge, len(list))
	for i, e := range list {
		other := e.Target
		if other == self {
			other = e.Source
		}
		out[i] = mcpCardEdge{Rel: string(e.Relation), Node: other,
			Confidence: string(e.Confidence)}
	}
	return out
}

func (s *mcpServer) toolExplain(args json.RawMessage) *toolResult {
	var p traverseParams
	if err := json.Unmarshal(args, &p); err != nil || p.Entity == "" {
		return errResult(fmt.Errorf("entity is required"))
	}
	q, err := p.edgeQuery(query.Both)
	if err != nil {
		return errResult(err)
	}
	return s.withStore(func(w *workspace.Workspace, st *store.Store) *toolResult {
		n, err := query.ResolveEntity(st, p.Entity)
		if err != nil {
			return errResult(err)
		}
		card, err := query.ExplainFiltered(st, n.ID, q)
		if err != nil {
			return errResult(err)
		}
		out := map[string]interface{}{
			"node":      compactNodesWith([]queryNode{card.Node}, conceptIndex(st))[0],
			"signature": card.Node.Signature,
			"community": card.CommunityLabel,
			"out":       compactCardEdges(card.Out, card.Node.ID),
			"in":        compactCardEdges(card.In, card.Node.ID),
		}
		// Only ever set when edges were withheld, so a caller can tell a short
		// list from a capped one instead of assuming it saw everything.
		if card.Truncated {
			out["truncated"] = true
			out["total"] = card.Total
		}
		return jsonResult(out)
	})
}

func (s *mcpServer) toolNeighbors(args json.RawMessage) *toolResult {
	var p traverseParams
	if err := json.Unmarshal(args, &p); err != nil || p.Entity == "" {
		return errResult(fmt.Errorf("entity is required"))
	}
	rels, err := query.ParseRelations(p.Relations)
	if err != nil {
		return errResult(err)
	}
	kinds, err := query.ParseKinds(p.Kinds)
	if err != nil {
		return errResult(err)
	}
	dir, err := query.ParseDirection(p.Direction)
	if err != nil {
		return errResult(err)
	}
	if p.Depth != 0 && (p.Depth < 1 || p.Depth > query.MaxDepth) {
		return errResult(fmt.Errorf("depth must be between 1 and %d", query.MaxDepth))
	}
	return s.withStore(func(w *workspace.Workspace, st *store.Store) *toolResult {
		n, err := query.ResolveEntity(st, p.Entity)
		if err != nil {
			return errResult(err)
		}
		res, err := query.Neighbors(st, []string{n.ID}, query.NeighborOptions{
			Depth: p.Depth, Direction: dir, Relations: rels, Kinds: kinds,
			Limit: p.Limit, IncludeTests: p.Tests,
		})
		if err != nil {
			return errResult(err)
		}
		nodes := compactNodesWith(res.Nodes, conceptIndex(st))
		out := map[string]interface{}{
			"seed":  n.ID,
			"nodes": nodes,
			"edges": compactEdges(res.Edges, nodeIndex(nodes)),
		}
		if res.More {
			out["truncated"] = true
			out["total"] = res.Total
		}
		return jsonResult(out)
	})
}

func (s *mcpServer) toolCallers(args json.RawMessage) *toolResult {
	var p traverseParams
	if err := json.Unmarshal(args, &p); err != nil || p.Entity == "" {
		return errResult(fmt.Errorf("entity is required"))
	}
	q, err := p.edgeQuery(query.In)
	if err != nil {
		return errResult(err)
	}
	q.Direction = query.In // callers is the incoming direction by definition
	return s.withStore(func(w *workspace.Workspace, st *store.Store) *toolResult {
		n, err := query.ResolveEntity(st, p.Entity)
		if err != nil {
			return errResult(err)
		}
		res, err := query.Edges(st, n.ID, q)
		if err != nil {
			return errResult(err)
		}
		concepts := conceptIndex(st)
		callers := make([]map[string]interface{}, 0, len(res.Hits))
		for _, h := range res.Hits {
			callers = append(callers, map[string]interface{}{
				"rel":  string(h.Edge.Relation),
				"conf": string(h.Edge.Confidence),
				"node": compactNodesWith([]queryNode{h.Other}, concepts)[0],
			})
		}
		out := map[string]interface{}{"entity": n.ID, "callers": callers}
		if res.More {
			out["truncated"] = true
			out["total"] = res.Total
		}
		return jsonResult(out)
	})
}

// toolConcept crosses from the code graph into the wiki. The mapping is written
// by `graphy export okfw` and was until now readable by nothing at all, so the
// two tools described the same entity with no shared identifier.
func (s *mcpServer) toolConcept(args json.RawMessage) *toolResult {
	var p traverseParams
	if err := json.Unmarshal(args, &p); err != nil || p.Entity == "" {
		return errResult(fmt.Errorf("entity is required"))
	}
	return s.withStore(func(w *workspace.Workspace, st *store.Store) *toolResult {
		n, err := query.ResolveEntity(st, p.Entity)
		if err != nil {
			return errResult(err)
		}
		// A file node is never exported, so asking it for "its" concept could
		// only ever fail — and the failure advised an export that would never
		// produce one. What a path can answer is which of the entities it
		// declares have been documented.
		if n.Kind == model.KindFile {
			concepts, err := query.ConceptsForFile(st, n.File)
			if err != nil {
				return errResult(err)
			}
			return jsonResult(map[string]interface{}{"file": n.File, "concepts": concepts})
		}
		c, err := query.ConceptFor(st, n.ID)
		if err != nil {
			return errResult(err)
		}
		if c == nil {
			return errResult(fmt.Errorf("%s has no exported concept (run `graphy export okfw`)", n.Label))
		}
		return jsonResult(c)
	})
}

// toolCommunity closes the wiki→graph direction for subsystems: a community
// concept carries a `community:<slug>` node id that nothing could resolve,
// because communities are not nodes.
func (s *mcpServer) toolCommunity(args json.RawMessage) *toolResult {
	var p struct {
		Community string `json:"community"`
		Limit     int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &p); err != nil || p.Community == "" {
		return errResult(fmt.Errorf("community is required"))
	}
	return s.withStore(func(w *workspace.Workspace, st *store.Store) *toolResult {
		card, err := query.Community(st, p.Community, p.Limit)
		if err != nil {
			return errResult(err)
		}
		out := map[string]interface{}{
			"label":     card.Label,
			"slug":      card.Slug,
			"languages": card.Languages,
			"members":   compactNodesWith(card.Members, conceptIndex(st)),
		}
		if card.Concept != nil {
			out["concept"] = card.Concept.RelPath
		}
		if card.More {
			out["truncated"] = true
			out["total"] = card.Total
		}
		return jsonResult(out)
	})
}

func (s *mcpServer) toolFileNodes(args json.RawMessage) *toolResult {
	var p traverseParams
	if err := json.Unmarshal(args, &p); err != nil || p.File == "" {
		return errResult(fmt.Errorf("file is required"))
	}
	return s.withStore(func(w *workspace.Workspace, st *store.Store) *toolResult {
		nodes, err := query.FileNodes(st, p.File)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(map[string]interface{}{
			"file":  p.File,
			"nodes": compactNodesWith(nodes, conceptIndex(st)),
		})
	})
}

// toolRationale answers "why is this code like this" directly. It exists as a
// first-class tool because that question is the whole point of rationale
// nodes; without it an agent would have to infer the answer from an edge list.
func (s *mcpServer) toolRationale(args json.RawMessage) *toolResult {
	var p struct {
		Entity string `json:"entity"`
	}
	if err := json.Unmarshal(args, &p); err != nil || p.Entity == "" {
		return errResult(fmt.Errorf("entity is required"))
	}
	return s.withStore(func(w *workspace.Workspace, st *store.Store) *toolResult {
		n, err := query.ResolveEntity(st, p.Entity)
		if err != nil {
			return errResult(err)
		}
		card, err := query.Explain(st, n.ID)
		if err != nil {
			return errResult(err)
		}
		// Rationale points AT the entity, so it arrives on the incoming side.
		var out []mcpNode
		for _, e := range card.In {
			if e.Relation != model.RelExplains {
				continue
			}
			src, err := st.GetNode(e.Source)
			if err != nil || src == nil {
				continue
			}
			out = append(out, compactNodes([]queryNode{*src})[0])
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return jsonResult(map[string]interface{}{
			"entity":    n.Label,
			"rationale": out,
		})
	})
}

func (s *mcpServer) toolReport(args json.RawMessage) *toolResult {
	var p struct {
		Section string `json:"section"`
	}
	// An absent or malformed argument object means "the whole report", which is
	// the pre-existing behaviour and the sensible default.
	_ = json.Unmarshal(args, &p)
	return s.withStore(func(w *workspace.Workspace, st *store.Store) *toolResult {
		content, err := os.ReadFile(w.ReportPath())
		if err != nil {
			return errResult(fmt.Errorf("no report (run `graphy build`): %w", err))
		}
		if p.Section == "" {
			return textResult(string(content))
		}
		body, ok := reportSection(string(content), p.Section)
		if !ok {
			return errResult(fmt.Errorf("no section %q (available: %s)",
				p.Section, strings.Join(reportSections(string(content)), ", ")))
		}
		return textResult(body)
	})
}

func (s *mcpServer) toolUpdate() *toolResult {
	return s.withStore(func(w *workspace.Workspace, st *store.Store) *toolResult {
		// This tool exposes no flags, so inheriting the graph's recorded
		// clustering settings is the only way a louvain-clustered repo stays
		// louvain-clustered when an agent refreshes it.
		co, err := inheritedClusterOptions(st)
		if err != nil {
			return errResult(err)
		}
		stats, err := build.Run(w, st, build.Options{Cluster: co})
		if err != nil {
			return errResult(err)
		}
		if err := writeGraphJSON(w, st, w.JSONPath()); err != nil {
			return errResult(err)
		}
		out := map[string]interface{}{
			"extracted": stats.FilesExtracted,
			"purged":    stats.FilesPurged,
			"nodes":     stats.Nodes,
			"edges":     stats.Edges,
		}
		if len(stats.Failures) > 0 {
			skipped := make([]string, 0, len(stats.Failures))
			for _, fe := range stats.Failures {
				skipped = append(skipped, fe.Error())
			}
			out["skipped"] = skipped
		}
		return jsonResult(out)
	})
}
