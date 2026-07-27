package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"okfw/cli/internal/config"
	"okfw/cli/internal/graphy/build"
	"okfw/cli/internal/graphy/export"
	"okfw/cli/internal/graphy/store"
	"okfw/cli/internal/graphy/workspace"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// fixtureRepo writes a small cross-referencing repo and returns its root. The
// WHY comment gives graphy_rationale something to find.
func fixtureRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "go.mod", "module example.com/fixture\n\ngo 1.23\n")
	write(t, root, "util/util.go", `package util

// WHY: callers pass untrusted input, so Helper never allocates.
func Helper(x string) string { return x }
`)
	write(t, root, "main.go", `package main

import "example.com/fixture/util"

func main() { run() }

func run() string { return util.Helper("hi") }
`)
	return root
}

// builtWorkspace writes the fixture repo and builds its graph.
func builtWorkspace(t *testing.T) string {
	t.Helper()
	root := fixtureRepo(t)
	w, err := workspace.Init(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(w.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := build.Run(w, s, build.Options{}); err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := writeGraphJSON(w, s, w.JSONPath()); err != nil {
		t.Fatal(err)
	}
	return root
}

// mcpTestServer binds a server to a freshly built workspace. startDir is the
// seam that makes this possible without chdir'ing the test process.
func mcpTestServer(t *testing.T) *mcpServer {
	t.Helper()
	return &mcpServer{startDir: builtWorkspace(t), out: &strings.Builder{}}
}

// callTool invokes a tool and returns its first text block.
func callTool(t *testing.T, s *mcpServer, name, args string) (string, bool) {
	t.Helper()
	res := s.callTool(name, json.RawMessage(args))
	if res == nil || len(res.Content) == 0 {
		t.Fatalf("tool %s returned no content", name)
	}
	return res.Content[0].Text, res.IsError
}

func TestMCPHandshakeAndToolList(t *testing.T) {
	s := &mcpServer{startDir: t.TempDir(), out: &strings.Builder{}}

	resp := s.handleLine(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{}}}`)
	if resp == nil || resp.Error != nil {
		t.Fatalf("initialize failed: %+v", resp)
	}
	result, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(result), `"protocolVersion":"2025-03-26"`) {
		t.Errorf("initialize should echo the client protocol version, got %s", result)
	}

	if resp := s.handleLine(`{"jsonrpc":"2.0","method":"notifications/initialized"}`); resp != nil {
		t.Error("notification must not produce a response")
	}

	resp = s.handleLine(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	result, _ = json.Marshal(resp.Result)
	for _, tool := range []string{
		`"graphy_query"`, `"graphy_path"`, `"graphy_explain"`,
		`"graphy_neighbors"`, `"graphy_callers"`, `"graphy_file_nodes"`, `"graphy_concept"`,
		`"graphy_rationale"`, `"graphy_report"`, `"graphy_update"`,
	} {
		if !strings.Contains(string(result), tool) {
			t.Errorf("tools/list missing %s", tool)
		}
	}

	resp = s.handleLine(`{"jsonrpc":"2.0","id":3,"method":"nope"}`)
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Errorf("unknown method should return -32601, got %+v", resp)
	}

	if resp := s.handleLine(`not json`); resp == nil || resp.Error == nil || resp.Error.Code != -32700 {
		t.Errorf("malformed line should return -32700, got %+v", resp)
	}
}

// TestMCPToolsListDeclaresEveryHandler pins the two halves together: a tool
// advertised with no handler is a tool an agent calls and gets "unknown tool"
// from, and a handler with no declaration is unreachable.
func TestMCPToolsListDeclaresEveryHandler(t *testing.T) {
	s := mcpTestServer(t)
	for _, tool := range mcpTools() {
		name, _ := tool["name"].(string)
		if name == "" {
			t.Fatalf("tool declared without a name: %v", tool)
		}
		res := s.callTool(name, nil)
		if res != nil && len(res.Content) > 0 &&
			strings.Contains(res.Content[0].Text, "unknown tool") {
			t.Errorf("tool %s is declared but has no handler", name)
		}
	}
	if res := s.callTool("graphy_nonexistent", nil); res == nil || !res.IsError {
		t.Error("an undeclared tool should return an error result")
	}
}

func TestMCPExplainAndPath(t *testing.T) {
	s := mcpTestServer(t)

	out, isErr := callTool(t, s, "graphy_explain", `{"entity":"Helper"}`)
	if isErr {
		t.Fatalf("graphy_explain errored: %s", out)
	}
	if !strings.Contains(out, "util/util.go") {
		t.Errorf("explain should report the entity's file, got %s", out)
	}

	out, isErr = callTool(t, s, "graphy_path", `{"from":"run","to":"Helper"}`)
	if isErr {
		t.Fatalf("graphy_path errored: %s", out)
	}
	if !strings.Contains(out, "Helper") {
		t.Errorf("path should reach Helper, got %s", out)
	}

	if out, isErr := callTool(t, s, "graphy_explain", `{"entity":"NoSuchEntity"}`); !isErr {
		t.Errorf("explain of a missing entity should be an error, got %s", out)
	}
	if out, isErr := callTool(t, s, "graphy_explain", `{}`); !isErr {
		t.Errorf("explain without an entity should be an error, got %s", out)
	}
}

func TestMCPRationaleAndReport(t *testing.T) {
	s := mcpTestServer(t)

	out, isErr := callTool(t, s, "graphy_rationale", `{"entity":"Helper"}`)
	if isErr {
		t.Fatalf("graphy_rationale errored: %s", out)
	}
	if !strings.Contains(out, "untrusted input") {
		t.Errorf("rationale should surface the WHY comment, got %s", out)
	}

	out, isErr = callTool(t, s, "graphy_report", `{}`)
	if isErr {
		t.Fatalf("graphy_report errored: %s", out)
	}
	if !strings.Contains(out, "Graph Report") {
		t.Errorf("report should return GRAPH_REPORT.md, got %s", out)
	}
}

// TestMCPQueryDepthIsValidated pins the bound the tool schema advertises. It
// was documentation-only: the handler forwarded any depth straight into a BFS.
func TestMCPQueryDepthIsValidated(t *testing.T) {
	s := mcpTestServer(t)

	for _, depth := range []string{"50", "-1", "3"} {
		out, isErr := callTool(t, s, "graphy_query", `{"question":"what calls Helper","depth":`+depth+`}`)
		if !isErr {
			t.Errorf("depth %s should be rejected, got %s", depth, out)
		}
	}
	// An absent depth decodes to 0, so 0 has to mean "unset" — there is no
	// third state to distinguish it from. Both forms let the planner choose.
	for _, args := range []string{`{"question":"what calls Helper"}`, `{"question":"what calls Helper","depth":0}`} {
		if out, isErr := callTool(t, s, "graphy_query", args); isErr {
			t.Errorf("query %s should succeed, got %s", args, out)
		}
	}
	if out, isErr := callTool(t, s, "graphy_query", `{}`); !isErr {
		t.Errorf("query without a question should be an error, got %s", out)
	}
}

func TestMCPWithoutGraphDatabase(t *testing.T) {
	// A workspace that exists but was never built.
	root := t.TempDir()
	if _, err := workspace.Init(root); err != nil {
		t.Fatal(err)
	}
	s := &mcpServer{startDir: root, out: &strings.Builder{}}

	out, isErr := callTool(t, s, "graphy_explain", `{"entity":"Helper"}`)
	if !isErr {
		t.Fatalf("expected an error with no graph database, got %s", out)
	}
	if !strings.Contains(out, "graphy build") {
		t.Errorf("error should tell the user to build, got %s", out)
	}
}

// TestMCPUpdateInheritsClusterSettings covers the MCP half of the reversion
// bug: the tool takes no flags, so inheriting the recorded settings is the
// only way a louvain-clustered repo stays louvain-clustered.
func TestMCPUpdateInheritsClusterSettings(t *testing.T) {
	root := fixtureRepo(t)
	w, err := workspace.Init(root)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(w.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := build.Run(w, st, build.Options{
		Cluster: build.ClusterOptions{Algorithm: "louvain", Resolution: 1.5, Seed: 7},
	}); err != nil {
		t.Fatal(err)
	}
	st.Close()

	s := &mcpServer{startDir: root, out: &strings.Builder{}}
	if out, isErr := callTool(t, s, "graphy_update", `{}`); isErr {
		t.Fatalf("graphy_update errored: %s", out)
	}

	st, err = store.Open(w.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	meta, err := st.GetClusterMeta()
	if err != nil {
		t.Fatal(err)
	}
	if meta.Algorithm != "louvain" || meta.Resolution != 1.5 || meta.Seed != 7 {
		t.Errorf("graphy_update reverted clustering to %+v, want louvain/1.5/7", meta)
	}
}

// TestMCPServeEmitsOneResponsePerRequest guards the serve loop: a response
// that cannot be marshalled used to be dropped, leaving the client waiting on
// an id that never got an answer.
func TestMCPServeEmitsOneResponsePerRequest(t *testing.T) {
	out := &strings.Builder{}
	s := &mcpServer{startDir: t.TempDir(), out: out}
	in := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"ping"}`,
		``, // blank lines are skipped, not answered
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	}, "\n"))

	if err := s.serve(in); err != nil {
		t.Fatalf("serve: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 responses (ping, tools/list), got %d: %q", len(lines), out.String())
	}
	for _, line := range lines {
		var resp rpcResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Errorf("response is not valid JSON: %v (%s)", err, line)
		}
		if resp.ID == nil {
			t.Errorf("response is missing its request id: %s", line)
		}
	}
}

// TestMCPTraversalTools covers the deterministic tools an agent reaches for
// when it already knows what it cares about — the case graphy_query, whose
// seeds come from an LLM plan or a keyword score, cannot serve.
func TestMCPTraversalTools(t *testing.T) {
	s := mcpTestServer(t)

	// callers answers the impact question directly.
	out, isErr := callTool(t, s, "graphy_callers", `{"entity":"Helper"}`)
	if isErr {
		t.Fatalf("graphy_callers errored: %s", out)
	}
	if !strings.Contains(out, `"rel":"calls"`) || !strings.Contains(out, "#run") {
		t.Errorf("graphy_callers should report run calling Helper: %s", out)
	}

	// file_nodes is the way in from a file the agent is editing.
	out, isErr = callTool(t, s, "graphy_file_nodes", `{"file":"util/util.go"}`)
	if isErr {
		t.Fatalf("graphy_file_nodes errored: %s", out)
	}
	if !strings.Contains(out, "#Helper") {
		t.Errorf("graphy_file_nodes should list Helper: %s", out)
	}

	// neighbors expands from a seed the caller chose, filtered by relation.
	out, isErr = callTool(t, s, "graphy_neighbors", `{"entity":"Helper","depth":1,"relations":["calls"]}`)
	if isErr {
		t.Fatalf("graphy_neighbors errored: %s", out)
	}
	if !strings.Contains(out, "#run") {
		t.Errorf("graphy_neighbors should reach run: %s", out)
	}

	// A misspelled relation is an error, not a silently unfiltered result that
	// would read as "everything matched".
	if out, isErr = callTool(t, s, "graphy_neighbors", `{"entity":"Helper","relations":["callz"]}`); !isErr {
		t.Errorf("unknown relation should error, got: %s", out)
	}
	if out, isErr = callTool(t, s, "graphy_explain", `{"entity":"Helper","direction":"sideways"}`); !isErr {
		t.Errorf("unknown direction should error, got: %s", out)
	}

	// explain narrows to one relation and one direction.
	out, isErr = callTool(t, s, "graphy_explain", `{"entity":"Helper","direction":"in","relations":["calls"]}`)
	if isErr {
		t.Fatalf("filtered graphy_explain errored: %s", out)
	}
	if strings.Contains(out, `"out":[{`) {
		t.Errorf("direction=in should return no outgoing edges: %s", out)
	}

	// A named report section avoids the whole document.
	out, isErr = callTool(t, s, "graphy_report", `{"section":"Stats"}`)
	if isErr {
		t.Fatalf("graphy_report section errored: %s", out)
	}
	if !strings.Contains(out, "## Stats") || strings.Contains(out, "## Communities") {
		t.Errorf("section should return Stats alone: %s", out)
	}
	if out, isErr = callTool(t, s, "graphy_report", `{"section":"Nonexistent"}`); !isErr {
		t.Errorf("unknown section should error, got: %s", out)
	}
}

// TestMCPConceptBridge covers the crossing point between the two tools. Before
// this the graph knew node ids and the wiki knew concept ids, and nothing
// mapped one to the other — each half was a dead end at the boundary.
func TestMCPConceptBridge(t *testing.T) {
	s := mcpTestServer(t)

	// Nothing exported yet: an honest error, not an empty success.
	out, isErr := callTool(t, s, "graphy_concept", `{"entity":"Helper"}`)
	if !isErr {
		t.Errorf("want an error before any export, got: %s", out)
	}

	// Export into a bundle, then the mapping resolves.
	bundleRoot := t.TempDir()
	okfwDir := filepath.Join(bundleRoot, ".okfw")
	if err := os.MkdirAll(okfwDir, 0755); err != nil {
		t.Fatal(err)
	}
	w, err := workspace.Discover(s.startDir)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(w.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := export.ToOkfw(w.Root, st, export.OkfwOptions{
		Bundle: config.ConfigFor(okfwDir), MinDegree: 1,
	}); err != nil {
		st.Close()
		t.Fatal(err)
	}
	st.Close()

	out, isErr = callTool(t, s, "graphy_concept", `{"entity":"Helper"}`)
	if isErr {
		t.Fatalf("graphy_concept errored: %s", out)
	}
	var c struct {
		NodeID    string `json:"node_id"`
		ConceptID string `json:"concept_id"`
		Path      string `json:"abs_path"`
	}
	if err := json.Unmarshal([]byte(out), &c); err != nil {
		t.Fatalf("graphy_concept result is not JSON: %s", out)
	}
	if !strings.HasSuffix(c.NodeID, "#Helper") || c.ConceptID == "" {
		t.Errorf("graphy_concept = %+v", c)
	}
	// The path must be openable, which is the whole reason the bundle root is
	// recorded alongside the bundle-relative record.
	if _, err := os.Stat(c.Path); err != nil {
		t.Errorf("concept path is not readable: %v", err)
	}

	// Every node any tool returns now carries its concept, so an agent never
	// needs a second call to find out whether there is prose to read.
	out, isErr = callTool(t, s, "graphy_file_nodes", `{"file":"util/util.go"}`)
	if isErr {
		t.Fatalf("graphy_file_nodes errored: %s", out)
	}
	if !strings.Contains(out, `"concept":"graphy/`) {
		t.Errorf("nodes should carry their concept path: %s", out)
	}
}

// TestMCPQueryAnnouncesItsCap pins the wire contract Wave 1 introduced:
// graphy_query is bounded, it says when it withheld rows, and its edges index
// into the node array rather than repeating two full node ids apiece.
func TestMCPQueryAnnouncesItsCap(t *testing.T) {
	s := mcpTestServer(t)

	out, isErr := callTool(t, s, "graphy_query", `{"question":"run","limit":1}`)
	if isErr {
		t.Fatalf("graphy_query errored: %s", out)
	}
	var got struct {
		Nodes []struct {
			ID string `json:"id"`
		} `json:"nodes"`
		Edges []struct {
			From int `json:"from"`
			To   int `json:"to"`
		} `json:"edges"`
		Truncated bool `json:"truncated"`
		Total     int  `json:"total"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("graphy_query result is not the declared shape: %v\n%s", err, out)
	}
	if len(got.Nodes) != 1 {
		t.Errorf("limit:1 returned %d nodes", len(got.Nodes))
	}
	if !got.Truncated || got.Total <= 1 {
		t.Errorf("withheld rows were not announced: truncated=%v total=%d", got.Truncated, got.Total)
	}
	for _, e := range got.Edges {
		if e.From < 0 || e.From >= len(got.Nodes) || e.To < 0 || e.To >= len(got.Nodes) {
			t.Errorf("edge endpoint %d->%d is not an index into %d nodes", e.From, e.To, len(got.Nodes))
		}
	}
	if strings.Contains(out, `"from":"`) {
		t.Errorf("edges still carry full node ids: %s", out)
	}

	if out, isErr := callTool(t, s, "graphy_query", `{"question":"run","limit":-1}`); !isErr {
		t.Errorf("a negative limit should error, got: %s", out)
	}
}

// TestMCPExplainOmitsTheConstantEndpoint: a card's edges all touch the entity
// the card is about, so naming it on every edge is pure repetition.
func TestMCPExplainOmitsTheConstantEndpoint(t *testing.T) {
	s := mcpTestServer(t)
	out, isErr := callTool(t, s, "graphy_explain", `{"entity":"Helper"}`)
	if isErr {
		t.Fatalf("graphy_explain errored: %s", out)
	}
	if strings.Contains(out, `"from":`) || strings.Contains(out, `"to":`) {
		t.Errorf("card edges should name only their other end (`node`): %s", out)
	}
	if !strings.Contains(out, `"node":`) {
		t.Errorf("card edges lost their other end: %s", out)
	}
}
