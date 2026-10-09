package main

// MCP (Model Context Protocol) stdio server: `okfw mcp`.
//
// Designed to minimize agent token usage compared to raw CLI access:
//   - 5 consolidated tools instead of one per CLI command — tool schemas are
//     injected into every agent context, so the surface is kept small and the
//     descriptions terse.
//   - Compact JSON responses: empty fields omitted, paths wiki-relative,
//     search capped at 10 results by default.
//
// The protocol is newline-delimited JSON-RPC 2.0 over stdio (tools-only MCP);
// implemented directly to avoid a dependency for ~five methods.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"okfw/cli/internal/bundle"
	"okfw/cli/internal/config"
	"okfw/cli/internal/graph"
	"okfw/cli/internal/search"
)

const mcpProtocolVersion = "2024-11-05"

type mcpServer struct {
	startDir string // wiki discovery start; "" = process cwd
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

// toolResult is the MCP tools/call result: content blocks + error flag.
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

// mcpTools returns the tool definitions. Descriptions are deliberately terse:
// they are loaded into the agent's context on every session.
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
	enum := func(desc string, vals ...string) map[string]interface{} {
		return map[string]interface{}{"type": "string", "description": desc, "enum": vals}
	}
	num := func(desc string) map[string]interface{} {
		return map[string]interface{}{"type": "integer", "description": desc}
	}

	return []map[string]interface{}{
		{
			"name":        "okfw_search",
			"description": "Search wiki concepts. Returns {total,results:[{id,title,path,kind,type,project,tags,snippet}]}; path is wiki-relative (use with read).",
			"inputSchema": obj(map[string]interface{}{
				"q":       str("full-text query"),
				"project": str("filter by project"),
				"tag":     str("filter by tag"),
				"kind":    enum("filter by kind", "note", "daily-note", "issue"),
				"type":    str("filter by OKF free-form type, e.g. Playbook, Metric"),
				"status":  enum("OKF lifecycle status filter", "draft", "stable", "deprecated"),
				"stale":   map[string]interface{}{"type": "boolean", "description": "only concepts whose stale_after has passed"},
				"limit":   num("max results (default 10)"),
				"offset":  num("skip this many results; `truncated` reports next_offset"),
			}),
		},
		{
			"name":        "okfw_create",
			"description": "Create a concept. Returns {id,path}.",
			"inputSchema": obj(map[string]interface{}{
				"kind":        enum("what to create (default note)", "note", "daily"),
				"title":       str("title (required for note)"),
				"project":     str("optional project metadata (also used as folder)"),
				"dir":         str("wiki subdirectory, e.g. research/protocols"),
				"tags":        map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				"description": str("one-sentence OKF description"),
				"body":        str("markdown body content"),
				"template":    str("template name (see okfw templates)"),
				"date":        str("daily note date YYYY-MM-DD (default today)"),
			}),
		},
		{
			"name":        "okfw_read",
			"description": "Read a concept's markdown by id or wiki-relative path. Capped at 200 lines; narrow with section, or page with offset/limit.",
			"inputSchema": obj(map[string]interface{}{
				"ref":     str("concept id or path"),
				"section": str("read only this markdown section, by heading text"),
				"offset":  num("first line to read (1-based)"),
				"limit":   num("maximum lines to read"),
			}, "ref"),
		},
		{
			"name":        "okfw_update",
			"description": "Update a concept: metadata, OKF trust/provenance (verify, set_generated, set_resource, add_source), or its markdown body (append / set_body).",
			"inputSchema": obj(map[string]interface{}{
				"ref":    str("concept id or path"),
				"action": enum("what to change", "set_project", "set_lifecycle", "set_stale_after", "add_tags", "append", "set_body", "verify", "set_generated", "set_resource", "add_source"),
				"value":  str("project | draft|stable|deprecated | ISO 8601 datetime with offset, e.g. 2026-12-31T00:00:00Z (set_stale_after) | comma-separated tags | markdown (append/set_body) | actor like human:me (verify, set_generated; optional) | URI (set_resource) | resource string or JSON {resource,id,title,author,last_modified,usage_count,usage_window} (add_source)"),
			}, "ref", "action", "value"),
		},
		{
			"name":        "okfw_graph",
			"description": "Link-graph queries. backlinks|tag -> {nodes:[{id,title,path}],total}; cluster adds edges; dangling -> {dangling:[{from,from_path,to}],total}. Capped at `limit`; `truncated` means more exist.",
			"inputSchema": obj(map[string]interface{}{
				"action": enum("query", "backlinks", "cluster", "tag", "dangling"),
				"ref":    str("concept id/path (backlinks, cluster) or tag name (tag)"),
				"depth":  num("cluster max hops (default unlimited)"),
				"limit":  num("max nodes (default 10)"),
			}, "action"),
		},
	}
}

// serve runs the newline-delimited JSON-RPC loop until stdin closes.
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

// handleLine processes one JSON-RPC message; returns nil for notifications.
func (s *mcpServer) handleLine(line string) *rpcResponse {
	var req rpcRequest
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		return &rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"),
			Error: &rpcError{Code: -32700, Message: "parse error"}}
	}
	if req.ID == nil {
		return nil // notification (e.g. notifications/initialized)
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
			"serverInfo":      map[string]interface{}{"name": "okfw", "version": config.Version},
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

func (s *mcpServer) cfg() (*config.Config, error) {
	return config.Discover(s.startDir)
}

// relPath converts an absolute concept path to a wiki-relative one (shorter
// for the agent, and accepted back by the read/update tools).
func relPath(cfg *config.Config, abs string) string {
	if rel, err := filepath.Rel(cfg.RootPath, abs); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return abs
}

func (s *mcpServer) callTool(name string, args json.RawMessage) (res *toolResult) {
	// One bad argument must not take the server down with it: a panic here
	// kills the stdio loop, and the client sees every later call hang.
	defer func() {
		if r := recover(); r != nil {
			res = errResult(fmt.Errorf("internal error in %s: %v", name, r))
		}
	}()
	if args == nil {
		args = json.RawMessage("{}")
	}
	cfg, err := s.cfg()
	if err != nil {
		return errResult(err)
	}
	// Both spellings dispatch. The tools are advertised as okfw_* so they read
	// unambiguously in a client that flattens tool names across servers, but
	// the bare names shipped first and may be baked into a prompt or a saved
	// skill somewhere, and breaking those buys nothing.
	switch strings.TrimPrefix(name, "okfw_") {
	case "search":
		return s.toolSearch(cfg, args)
	case "create":
		return s.toolCreate(cfg, args)
	case "read":
		return s.toolRead(cfg, args)
	case "update":
		return s.toolUpdate(cfg, args)
	case "graph":
		return s.toolGraph(cfg, args)
	}
	return errResult(fmt.Errorf("unknown tool: %s", name))
}

type mcpHit struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Path  string `json:"path"`
	// Kind and Type say what a hit *is*, so a caller can route on it without
	// reading the concept.
	Kind    string   `json:"kind,omitempty"`
	Type    string   `json:"type,omitempty"`
	Project string   `json:"project,omitempty"`
	Tags    []string `json:"tags,omitempty"`
	Snippet string   `json:"snippet,omitempty"`
}

// maxSearchLimit bounds a single search page. The caller can page past it
// with offset; nothing bounded it before, so `limit: 100000` dumped the bundle.
const maxSearchLimit = 100

func (s *mcpServer) toolSearch(cfg *config.Config, args json.RawMessage) *toolResult {
	var a struct {
		Q       string `json:"q"`
		Project string `json:"project"`
		Tag     string `json:"tag"`
		Kind    string `json:"kind"`
		Type    string `json:"type"`
		Status  string `json:"status"`
		Stale   bool   `json:"stale"`
		Limit   int    `json:"limit"`
		Offset  int    `json:"offset"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return errResult(err)
	}
	if a.Limit <= 0 {
		a.Limit = 10
	}
	a.Limit = min(a.Limit, maxSearchLimit)
	a.Offset = max(a.Offset, 0)

	opts := search.Options{
		Query:   a.Q,
		Project: a.Project,
		Kind:    a.Kind,
		Type:    a.Type,
		Status:  a.Status,
		Stale:   a.Stale,
		Limit:   a.Limit,
		Offset:  a.Offset,
	}
	if a.Tag != "" {
		opts.Tags = []string{a.Tag}
	}
	results, total, err := search.Search(cfg.RootPath, opts)
	if err != nil {
		return errResult(err)
	}

	hits := make([]mcpHit, 0, len(results))
	for _, r := range results {
		hits = append(hits, mcpHit{
			ID:      r.ID,
			Title:   r.Title,
			Path:    relPath(cfg, r.FilePath),
			Kind:    r.Kind,
			Type:    r.Type,
			Project: r.Project,
			Tags:    r.Tags,
			Snippet: r.Snippet,
		})
	}
	out := map[string]interface{}{"total": total, "results": hits}
	// A bare total the caller has to compare against len(results) is not an
	// announcement. Say it, and say how to get the rest.
	if shown := a.Offset + len(hits); total > shown {
		out["truncated"] = true
		out["next_offset"] = shown
	}
	return jsonResult(out)
}

func (s *mcpServer) toolCreate(cfg *config.Config, args json.RawMessage) *toolResult {
	var a struct {
		Kind        string   `json:"kind"`
		Title       string   `json:"title"`
		Project     string   `json:"project"`
		Dir         string   `json:"dir"`
		Tags        []string `json:"tags"`
		Description string   `json:"description"`
		Template    string   `json:"template"`
		Body        string   `json:"body"`
		Date        string   `json:"date"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return errResult(err)
	}

	switch a.Kind {
	case "daily":
		date := time.Now()
		if a.Date != "" {
			var err error
			date, err = time.Parse("2006-01-02", a.Date)
			if err != nil {
				return errResult(fmt.Errorf("invalid date %q: want YYYY-MM-DD", a.Date))
			}
		}
		path, _, err := createOrOpenDaily(cfg, date)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(map[string]string{"path": relPath(cfg, path)})

	case "", "note":
		if a.Title == "" {
			return errResult(fmt.Errorf("title is required for notes"))
		}
		path, id, err := bundle.CreateConcept(cfg, bundle.Spec{
			Title:       a.Title,
			Project:     a.Project,
			Dir:         a.Dir,
			Tags:        a.Tags,
			Description: a.Description,
			Template:    a.Template,
			Body:        a.Body,
		})
		if err != nil {
			return errResult(err)
		}
		if err := bundle.AutoSync(cfg, path); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to index note: %v\n", err)
		}
		return jsonResult(map[string]string{"id": id, "path": relPath(cfg, path)})
	}
	return errResult(fmt.Errorf("unknown kind %q: want note or daily", a.Kind))
}

func (s *mcpServer) toolRead(cfg *config.Config, args json.RawMessage) *toolResult {
	var a struct {
		Ref     string `json:"ref"`
		Section string `json:"section"`
		Offset  int    `json:"offset"`
		Limit   int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &a); err != nil || a.Ref == "" {
		return errResult(fmt.Errorf("ref is required"))
	}
	path, err := bundle.ResolveConceptPath(cfg, a.Ref)
	if err != nil {
		return errResult(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return errResult(err)
	}
	// A generated entity card is small, but a hand-written concept is not, and
	// an agent following links through a bundle pays for every one it opens.
	// Section and line windows let it read the part it came for.
	if a.Section != "" {
		body, ok := markdownSection(string(content), a.Section)
		if !ok {
			return errResult(fmt.Errorf("no section %q in %s", a.Section, a.Ref))
		}
		return textResult(body)
	}
	if a.Offset > 0 || a.Limit > 0 {
		limit := a.Limit
		if limit <= 0 {
			limit = defaultReadLines
		}
		return textResult(readWindow(string(content), a.Offset, limit))
	}
	// A read that asked for no window used to return the whole file, uncapped
	// and unmarked — the one path through this handler with no bound, in the
	// tool an agent calls most.
	return textResult(readWindow(string(content), 1, defaultReadLines))
}

// defaultReadLines bounds a read that asked for no window.
//
// Generated concepts are held to 4000 bytes by
// TestExportStaysWithinItsTokenBudget — roughly 100 lines — so this never bites
// one, and the graph→wiki bridge is unaffected. It bounds a hand-written
// concept, which nothing bounded, at about 3k tokens.
const defaultReadLines = 200

// markdownSection extracts one heading's content, matched case-insensitively,
// through to the next heading of the same or a shallower level.
func markdownSection(content, want string) (string, bool) {
	lines := strings.Split(content, "\n")
	want = strings.ToLower(strings.TrimSpace(want))
	start, level := -1, 0
	for i, line := range lines {
		n := 0
		for n < len(line) && line[n] == '#' {
			n++
		}
		if n == 0 || n >= len(line) || line[n] != ' ' {
			continue
		}
		if start < 0 {
			if strings.ToLower(strings.TrimSpace(line[n+1:])) == want {
				start, level = i, n
			}
			continue
		}
		if n <= level {
			return strings.Join(lines[start:i], "\n"), true
		}
	}
	if start < 0 {
		return "", false
	}
	return strings.Join(lines[start:], "\n"), true
}

// lineWindow returns limit lines starting at the 1-based offset, and how many
// lines it withheld after them.
func lineWindow(content string, offset, limit int) (string, int) {
	lines := strings.Split(content, "\n")
	if offset > 0 {
		if offset-1 >= len(lines) {
			return "", 0
		}
		lines = lines[offset-1:]
	}
	more := 0
	if limit > 0 && limit < len(lines) {
		more = len(lines) - limit
		lines = lines[:limit]
	}
	return strings.Join(lines, "\n"), more
}

// readWindow renders a line window with its continuation marker.
func readWindow(content string, offset, limit int) string {
	if offset < 1 {
		offset = 1
	}
	text, more := lineWindow(content, offset, limit)
	if more == 0 {
		return text
	}
	return fmt.Sprintf("%s\n\n… %d more lines — continue with offset:%d, or pass section:\"<heading>\"",
		text, more, offset+limit)
}

func (s *mcpServer) toolUpdate(cfg *config.Config, args json.RawMessage) *toolResult {
	var a struct {
		Ref    string `json:"ref"`
		Action string `json:"action"`
		Value  string `json:"value"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return errResult(err)
	}
	if a.Ref == "" || a.Action == "" {
		return errResult(fmt.Errorf("ref and action are required"))
	}
	// verify and set_generated default their actor, so value is optional for
	// them; everything else needs one.
	if a.Value == "" && a.Action != "verify" && a.Action != "set_generated" {
		return errResult(fmt.Errorf("value is required for %s", a.Action))
	}

	path, err := bundle.ResolveConceptPath(cfg, a.Ref)
	if err != nil {
		return errResult(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return errResult(err)
	}

	var newContent []byte
	switch a.Action {
	case "set_project":
		newContent, err = bundle.UpdateFrontmatter(content, map[string]string{"project": a.Value})
	case "set_lifecycle":
		switch a.Value {
		case "draft", "stable", "deprecated":
		default:
			return errResult(fmt.Errorf("invalid lifecycle status %q: want draft, stable, or deprecated", a.Value))
		}
		newContent, err = bundle.UpdateFrontmatter(content, map[string]string{"status": a.Value})
	case "set_stale_after":
		v, nerr := config.NormalizeDateTime(a.Value)
		if nerr != nil {
			return errResult(fmt.Errorf("invalid stale_after: %w", nerr))
		}
		newContent, err = bundle.UpdateFrontmatter(content, map[string]string{"stale_after": v})
	case "add_tags":
		var tags []string
		for _, t := range strings.Split(a.Value, ",") {
			if t = strings.TrimSpace(t); t != "" {
				tags = append(tags, t)
			}
		}
		newContent, err = bundle.AddFrontmatterTags(content, tags)
		if err == nil {
			newContent, err = bundle.UpdateFrontmatter(newContent, nil) // bump timestamp
		}
	case "append":
		appended := append([]byte(strings.TrimRight(string(content), "\n")), []byte("\n\n"+strings.TrimRight(a.Value, "\n")+"\n")...)
		newContent, err = bundle.UpdateFrontmatter(appended, nil) // bump timestamp
	case "set_body":
		fm, ferr := config.ExtractFrontmatter(content)
		if ferr != nil {
			return errResult(ferr)
		}
		rebuilt := "---\n" + string(fm) + "\n---\n\n" + strings.TrimRight(a.Value, "\n") + "\n"
		newContent, err = bundle.UpdateFrontmatter([]byte(rebuilt), nil) // bump timestamp

	// The OKF trust and provenance families. These share their mutators with
	// the CLI writers rather than reimplementing them, so `update` cannot drift
	// away from `okfw verify` / `add-source` / `set-resource` / `set-generated`
	// the way it had by simply not existing.
	case "verify":
		actor := a.Value
		if actor == "" {
			actor = defaultHumanActor()
		}
		newContent, err = applyOKFEdit(content, verifyMutator(actor, time.Now().Format(time.RFC3339)))
	case "set_generated":
		actor := a.Value
		if actor == "" {
			actor = defaultHumanActor()
		}
		newContent, err = applyOKFEdit(content, setGeneratedMutator(actor))
	case "set_resource":
		newContent, err = applyOKFEdit(content, setResourceMutator(a.Value))
	case "add_source":
		var e sourceEntry
		// A single resource string is the common case; JSON carries the rest.
		if strings.HasPrefix(strings.TrimSpace(a.Value), "{") {
			var in struct {
				Resource     string `json:"resource"`
				ID           string `json:"id"`
				Title        string `json:"title"`
				Author       string `json:"author"`
				LastModified string `json:"last_modified"`
				UsageCount   *int   `json:"usage_count"`
				UsageWindow  string `json:"usage_window"`
			}
			if jerr := json.Unmarshal([]byte(a.Value), &in); jerr != nil {
				return errResult(fmt.Errorf("add_source value is neither a resource string nor a JSON object: %w", jerr))
			}
			from, to, werr := parseUsageWindow(in.UsageWindow)
			if werr != nil {
				return errResult(werr)
			}
			if in.LastModified, werr = normalizeOptionalDateTime("last_modified", in.LastModified); werr != nil {
				return errResult(werr)
			}
			e = sourceEntry{
				resource: in.Resource, id: in.ID, title: in.Title, author: in.Author,
				lastModified: in.LastModified, from: from, to: to,
			}
			if in.UsageCount != nil {
				e.usageCount, e.hasUsage = *in.UsageCount, true
			}
		} else {
			e = sourceEntry{resource: a.Value}
		}
		if strings.TrimSpace(e.resource) == "" {
			return errResult(fmt.Errorf("add_source requires a resource"))
		}
		newContent, err = applyOKFEdit(content, addSourceMutator(e))
	default:
		return errResult(fmt.Errorf("unknown action %q", a.Action))
	}
	if err != nil {
		return errResult(err)
	}

	if err := os.WriteFile(path, newContent, 0644); err != nil {
		return errResult(err)
	}
	if err := bundle.AutoSync(cfg, path); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to index: %v\n", err)
	}
	return jsonResult(map[string]string{"path": relPath(cfg, path), "status": "ok"})
}

type mcpNode struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Path  string `json:"path"`
}

func (s *mcpServer) toolGraph(cfg *config.Config, args json.RawMessage) *toolResult {
	var a struct {
		Action string `json:"action"`
		Ref    string `json:"ref"`
		Depth  int    `json:"depth"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return errResult(err)
	}
	if a.Limit <= 0 {
		a.Limit = 10
	}

	g, err := bundle.LoadGraph(cfg.RootPath)
	if err != nil {
		return errResult(err)
	}

	// backlinks/cluster take a concept ref; resolve paths to ids.
	id := a.Ref
	if a.Action == "backlinks" || a.Action == "cluster" {
		if id == "" {
			return errResult(fmt.Errorf("ref is required for %s", a.Action))
		}
		if strings.HasSuffix(id, ".md") || strings.Contains(id, "/") {
			p, err := bundle.ResolveConceptPath(cfg, id)
			if err != nil {
				return errResult(err)
			}
			content, err := os.ReadFile(p)
			if err != nil {
				return errResult(err)
			}
			z, err := config.ParseFrontmatter(content)
			if err != nil || z.ID == "" {
				return errResult(fmt.Errorf("not a concept: %s", a.Ref))
			}
			id = z.ID
		}
	}

	toNodes := func(records []*graph.Node) []mcpNode {
		nodes := make([]mcpNode, 0, len(records))
		for _, n := range records {
			nodes = append(nodes, mcpNode{ID: n.ID, Title: n.Title, Path: relPath(cfg, n.FilePath)})
		}
		return nodes
	}
	// nodeEnvelope caps a node list and says what it withheld. The limit was
	// advertised in the schema and applied to exactly one of the four actions,
	// so three were unbounded while claiming a default of 10. Capping here
	// rather than in the graph package keeps the query signatures — also used
	// by `okfw graph-query` and the Neovim plugin — out of it.
	nodeEnvelope := func(records []*graph.Node) map[string]interface{} {
		total := len(records)
		out := map[string]interface{}{"total": total}
		if total > a.Limit {
			records = records[:a.Limit]
			out["truncated"] = true
		}
		out["nodes"] = toNodes(records)
		return out
	}

	type edge struct {
		From     string `json:"from"`
		FromPath string `json:"from_path,omitempty"`
		To       string `json:"to"`
	}

	switch a.Action {
	case "backlinks":
		return jsonResult(nodeEnvelope(g.Backlinks(id)))
	case "cluster":
		if g.GetNode(id) == nil {
			return errResult(fmt.Errorf("node %q not found", id))
		}
		records := g.FindConnected(id, a.Limit, a.Depth)
		edges := g.GetEdges(records)
		out := nodeEnvelope(records)
		es := make([]edge, 0, len(edges))
		for _, e := range edges {
			es = append(es, edge{From: e.From, To: e.To})
		}
		out["edges"] = es
		return jsonResult(out)
	case "tag":
		if a.Ref == "" {
			return errResult(fmt.Errorf("ref (tag name) is required for tag"))
		}
		return jsonResult(nodeEnvelope(g.ByTag(a.Ref)))
	case "dangling":
		edges := g.Dangling()
		total := len(edges)
		if total > a.Limit {
			edges = edges[:a.Limit]
		}
		es := make([]edge, 0, len(edges))
		for _, e := range edges {
			// The source concept's path, so a dangling link can be opened and
			// fixed without a second lookup per row.
			from := edge{From: e.From, To: e.To}
			if n := g.GetNode(e.From); n != nil {
				from.FromPath = relPath(cfg, n.FilePath)
			}
			es = append(es, from)
		}
		out := map[string]interface{}{"dangling": es, "total": total}
		if total > len(es) {
			out["truncated"] = true
		}
		return jsonResult(out)
	}
	return errResult(fmt.Errorf("unknown action %q", a.Action))
}

// applyOKFEdit runs an OKF-family frontmatter mutator, stamping the timestamp
// and guarding the result the same way the CLI's editConcept does.
func applyOKFEdit(content []byte, fn func(*yaml.Node) error) ([]byte, error) {
	out, err := bundle.Edit(content, func(m *yaml.Node) error {
		if err := fn(m); err != nil {
			return err
		}
		stamp := time.Now().Format(time.RFC3339)
		bundle.MapSetString(m, "timestamp", stamp)
		if bundle.MapGet(m, "generated") != nil {
			if err := bundle.SetPath(m, "generated.at", stamp); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := bundle.GuardMutation(content, out); err != nil {
		return nil, err
	}
	return out, nil
}
