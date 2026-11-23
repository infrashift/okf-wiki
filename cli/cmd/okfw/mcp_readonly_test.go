package main

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

// fixedCost returns the bytes an MCP client sends the model on every turn:
// the tool definitions and the initialize instructions.
func fixedCost(t *testing.T, s *mcpServer) (tools []string, bytes int, instructions string) {
	t.Helper()
	init := s.handleLine(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	instructions, _ = init.Result.(map[string]interface{})["instructions"].(string)
	list := s.handleLine(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	defs := list.Result.(map[string]interface{})["tools"].([]map[string]interface{})
	for _, d := range defs {
		tools = append(tools, d["name"].(string))
	}
	sort.Strings(tools)
	raw, err := json.Marshal(defs)
	if err != nil {
		t.Fatal(err)
	}
	return tools, len(raw) + len(instructions), instructions
}

// TestMCPReadOnly: `okfw mcp --read-only` serves search, read and graph only,
// refuses the write tools under either name, and costs an agent markedly
// less on every turn — the reason it exists.
func TestMCPReadOnly(t *testing.T) {
	rw, project := mcpTestServer(t)
	ro := &mcpServer{startDir: project, out: &strings.Builder{}, readOnly: true}

	rwTools, rwBytes, _ := fixedCost(t, rw)
	roTools, roBytes, roInstr := fixedCost(t, ro)
	if strings.Join(roTools, ",") != "okfw_graph,okfw_read,okfw_search" {
		t.Errorf("read-only tools = %v", roTools)
	}
	if len(rwTools) != 5 {
		t.Errorf("read-write tools = %v", rwTools)
	}
	if roBytes > rwBytes*65/100 {
		t.Errorf("read-only fixed cost %d bytes, read-write %d: want at least 35%% less", roBytes, rwBytes)
	}
	t.Logf("fixed per-turn cost: read-write %d bytes, read-only %d bytes (%d%% less)", rwBytes, roBytes, (rwBytes-roBytes)*100/rwBytes)
	for _, write := range []string{"okfw_create", "okfw_update", "set_lifecycle"} {
		if strings.Contains(roInstr, write) {
			t.Errorf("read-only instructions mention %s:\n%s", write, roInstr)
		}
	}

	for _, name := range []string{"okfw_create", "create", "okfw_update", "update"} {
		out, isErr := callTool(t, ro, name, `{"title":"Nope","ref":"x","action":"add_tags","value":"y"}`)
		if !isErr || !strings.Contains(out, "read-only") {
			t.Errorf("%s on a read-only server: %q (error=%v), want a read-only refusal", name, out, isErr)
		}
	}
	// Writes still work on the default server, and the read-only one reads them.
	if out, isErr := callTool(t, rw, "okfw_create", `{"title":"Ledger note","body":"ledger"}`); isErr {
		t.Fatalf("create: %s", out)
	}
	out, isErr := callTool(t, ro, "okfw_search", `{"q":"ledger"}`)
	if isErr || !strings.Contains(out, "Ledger note") {
		t.Errorf("read-only search: %s", out)
	}
}
