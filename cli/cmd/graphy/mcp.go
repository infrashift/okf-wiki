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
	"fmt"
	"io"
	"os"
	"strings"

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

func graphyVersion() string { return version }

func mcpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve graph queries over MCP (stdio)",
		Long: `Runs a Model Context Protocol stdio server exposing graphy_query,
graphy_path, graphy_explain, graphy_report, and graphy_update. Register it in
an agent's MCP config with command "graphy" and args ["mcp"].`,
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

	return []map[string]interface{}{
		{
			"name":        "graphy_query",
			"description": "Ask the code knowledge graph a question. Returns relevant entities + relationships. Prefer this over scanning files for architecture/impact questions.",
			"inputSchema": obj(map[string]interface{}{
				"question": str("natural-language question about the codebase"),
				"depth":    num("expansion depth 1-2 (default 1)"),
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
			"description": "Entity card: location, signature, doc, community, and all in/out edges with confidence.",
			"inputSchema": obj(map[string]interface{}{
				"entity": str("entity name or id"),
			}, "entity"),
		},
		{
			"name":        "graphy_report",
			"description": "The repository's GRAPH_REPORT.md: stats, god nodes, communities. Read before architecture questions.",
			"inputSchema": obj(map[string]interface{}{}),
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
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if resp := s.handleLine(line); resp != nil {
			data, err := json.Marshal(resp)
			if err != nil {
				continue
			}
			fmt.Fprintf(s.out, "%s\n", data)
		}
	}
	return scanner.Err()
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
	case "graphy_report":
		return s.toolReport()
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
	Degree int    `json:"degree,omitempty"`
}

type mcpEdge struct {
	From       string `json:"from"`
	Rel        string `json:"rel"`
	To         string `json:"to"`
	Confidence string `json:"conf"`
}

func compactNodes(nodes []queryNode) []mcpNode {
	out := make([]mcpNode, len(nodes))
	for i, n := range nodes {
		out[i] = mcpNode{
			ID: n.ID, Label: n.Label, Kind: string(n.Kind),
			File: n.File, Line: n.Span.StartLine, Doc: n.Doc, Degree: n.Degree,
		}
	}
	return out
}

func (s *mcpServer) toolQuery(args json.RawMessage) *toolResult {
	var p struct {
		Question string `json:"question"`
		Depth    int    `json:"depth"`
	}
	if err := json.Unmarshal(args, &p); err != nil || p.Question == "" {
		return errResult(fmt.Errorf("question is required"))
	}
	return s.withStore(func(w *workspace.Workspace, st *store.Store) *toolResult {
		backend, err := llm.FromEnv("")
		if err != nil && err != llm.ErrNoBackend {
			return errResult(err)
		}
		res, err := query.NL(context.Background(), st, backend, p.Question, query.NLOptions{Depth: p.Depth})
		if err != nil {
			return errResult(err)
		}
		edges := make([]mcpEdge, 0, len(res.Edges))
		for _, e := range res.Edges {
			edges = append(edges, mcpEdge{From: e.Source, Rel: string(e.Relation), To: e.Target, Confidence: string(e.Confidence)})
		}
		return jsonResult(map[string]interface{}{
			"mode":  res.Mode,
			"nodes": compactNodes(res.Nodes),
			"edges": edges,
		})
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

func (s *mcpServer) toolExplain(args json.RawMessage) *toolResult {
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
		edges := func(list []queryEdge) []mcpEdge {
			out := make([]mcpEdge, len(list))
			for i, e := range list {
				out[i] = mcpEdge{From: e.Source, Rel: string(e.Relation), To: e.Target, Confidence: string(e.Confidence)}
			}
			return out
		}
		return jsonResult(map[string]interface{}{
			"node":      compactNodes([]queryNode{card.Node})[0],
			"signature": card.Node.Signature,
			"community": card.CommunityLabel,
			"out":       edges(card.Out),
			"in":        edges(card.In),
		})
	})
}

func (s *mcpServer) toolReport() *toolResult {
	return s.withStore(func(w *workspace.Workspace, st *store.Store) *toolResult {
		content, err := os.ReadFile(w.ReportPath())
		if err != nil {
			return errResult(fmt.Errorf("no report (run `graphy build`): %w", err))
		}
		return textResult(string(content))
	})
}

func (s *mcpServer) toolUpdate() *toolResult {
	return s.withStore(func(w *workspace.Workspace, st *store.Store) *toolResult {
		stats, err := build.Run(w, st, build.Options{})
		if err != nil {
			return errResult(err)
		}
		if err := writeGraphJSON(w, st, w.JSONPath()); err != nil {
			return errResult(err)
		}
		return jsonResult(map[string]interface{}{
			"extracted": stats.FilesExtracted,
			"purged":    stats.FilesPurged,
			"nodes":     stats.Nodes,
			"edges":     stats.Edges,
		})
	})
}
