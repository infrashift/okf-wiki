package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"okfw/cli/internal/graphy/build"
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
