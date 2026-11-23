package main

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/infrashift/okf-wiki/cli/internal/okfwskills"
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

// TestMCPGuidanceNeverDiscardsUnverifiedHits pins the one sentence that
// stopped agents dropping correct answers. Every hit carries trust, almost
// every concept is unverified, and "weigh each hit by trust" alone read to
// Sonnet 5.5 as "rely on none": in the okfw-benchmark token runs it searched,
// got the right concept ranked first, and submitted no answer at all — 7 of
// 108 tasks under --read-only, whose shorter instructions gave that line more
// weight. Every surface that tells an agent about trust must also say that it
// ranks and never excludes.
func TestMCPGuidanceNeverDiscardsUnverifiedHits(t *testing.T) {
	const rule = "never rule one out"
	for name, text := range map[string]string{
		"mcpInstructions":         mcpInstructions,
		"mcpReadOnlyInstructions": mcpReadOnlyInstructions,
	} {
		if !strings.Contains(text, rule) || !strings.Contains(text, "unverified is the usual state") {
			t.Errorf("%s must say trust ranks hits but never rules one out", name)
		}
	}
	// The read-only text loses the write guidance's [Title](/dir/file.md), the
	// only place the full text shows a concept path, so it must say it itself.
	if !strings.Contains(mcpReadOnlyInstructions, "/dir/file.md") {
		t.Error("mcpReadOnlyInstructions must show what a concept path looks like")
	}
	for _, tool := range mcpTools() {
		if tool["name"] != "okfw_search" {
			continue
		}
		if d, _ := tool["description"].(string); !strings.Contains(d, "an unverified hit that answers the question is the answer") {
			t.Errorf("okfw_search's description must say an unverified hit still answers: %s", d)
		}
	}
	skill, _, err := okfwskills.Docs("")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(strings.Fields(skill), " "), "Trust ranks hits but never rules one out") {
		t.Error("the skill must say trust ranks hits but never rules one out")
	}
}
