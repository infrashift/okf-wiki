package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"okfw/cli/internal/bundle"
	"okfw/cli/internal/config"
)

// mcpTestServer returns a server bound to a fresh initialized wiki.
func mcpTestServer(t *testing.T) (*mcpServer, string) {
	t.Helper()
	project := t.TempDir()
	if err := runInit(project); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	return &mcpServer{startDir: project, out: &strings.Builder{}}, project
}

// callTool invokes a tool and returns the decoded first text block.
func callTool(t *testing.T, s *mcpServer, name, args string) (string, bool) {
	t.Helper()
	res := s.callTool(name, json.RawMessage(args))
	if res == nil || len(res.Content) == 0 {
		t.Fatalf("tool %s returned no content", name)
	}
	return res.Content[0].Text, res.IsError
}

func TestMCPHandshakeAndToolList(t *testing.T) {
	s, _ := mcpTestServer(t)

	resp := s.handleLine(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{}}}`)
	if resp == nil || resp.Error != nil {
		t.Fatalf("initialize failed: %+v", resp)
	}
	result, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(result), `"protocolVersion":"2025-03-26"`) {
		t.Errorf("initialize should echo the client protocol version, got %s", result)
	}

	// Notifications get no response.
	if resp := s.handleLine(`{"jsonrpc":"2.0","method":"notifications/initialized"}`); resp != nil {
		t.Error("notification must not produce a response")
	}

	resp = s.handleLine(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	result, _ = json.Marshal(resp.Result)
	for _, tool := range []string{`"okfw_search"`, `"okfw_create"`, `"okfw_read"`, `"okfw_update"`, `"okfw_graph"`} {
		if !strings.Contains(string(result), tool) {
			t.Errorf("tools/list missing %s", tool)
		}
	}

	resp = s.handleLine(`{"jsonrpc":"2.0","id":3,"method":"nope"}`)
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Errorf("unknown method should return -32601, got %+v", resp)
	}
}

func TestMCPCreateSearchReadUpdate(t *testing.T) {
	s, _ := mcpTestServer(t)

	// create a note
	text, isErr := callTool(t, s, "create",
		`{"title":"Protocol Design","dir":"research","tags":["net"],"description":"How the protocol works"}`)
	if isErr {
		t.Fatalf("create failed: %s", text)
	}
	var created struct{ ID, Path string }
	if err := json.Unmarshal([]byte(text), &created); err != nil || created.ID == "" || created.Path == "" {
		t.Fatalf("create should return {id,path}, got %s", text)
	}
	if !strings.HasPrefix(created.Path, "research/") {
		t.Errorf("path should be wiki-relative, got %s", created.Path)
	}

	// search finds it
	text, isErr = callTool(t, s, "search", `{"q":"protocol"}`)
	if isErr {
		t.Fatalf("search failed: %s", text)
	}
	if !strings.Contains(text, "Protocol Design") || !strings.Contains(text, `"total":1`) {
		t.Errorf("search should find the note, got %s", text)
	}

	// read by wiki-relative path and by id
	for _, ref := range []string{created.Path, created.ID} {
		text, isErr = callTool(t, s, "read", fmt.Sprintf(`{"ref":%q}`, ref))
		if isErr || !strings.Contains(text, "Protocol Design") {
			t.Errorf("read %q failed: %s", ref, text)
		}
	}

	// update: add tags
	text, isErr = callTool(t, s, "update",
		fmt.Sprintf(`{"ref":%q,"action":"add_tags","value":"deep, dive"}`, created.Path))
	if isErr {
		t.Fatalf("add_tags failed: %s", text)
	}
	text, _ = callTool(t, s, "read", fmt.Sprintf(`{"ref":%q}`, created.Path))
	if !strings.Contains(text, `"deep"`) || !strings.Contains(text, `"dive"`) {
		t.Errorf("tags not written:\n%s", text)
	}

	// OKF lifecycle: set_lifecycle and set_stale_after
	text, isErr = callTool(t, s, "update",
		fmt.Sprintf(`{"ref":%q,"action":"set_lifecycle","value":"deprecated"}`, created.Path))
	if isErr {
		t.Fatalf("set_lifecycle failed: %s", text)
	}
	text, isErr = callTool(t, s, "update",
		fmt.Sprintf(`{"ref":%q,"action":"set_stale_after","value":"2026-12-31"}`, created.Path))
	if isErr {
		t.Fatalf("set_stale_after failed: %s", text)
	}
	text, _ = callTool(t, s, "read", fmt.Sprintf(`{"ref":%q}`, created.Path))
	if !strings.Contains(text, `status: "deprecated"`) || !strings.Contains(text, `stale_after: "2026-12-31"`) {
		t.Errorf("lifecycle fields not written:\n%s", text)
	}

	// Invalid values are rejected rather than written through.
	if _, isErr = callTool(t, s, "update",
		fmt.Sprintf(`{"ref":%q,"action":"set_lifecycle","value":"closed"}`, created.Path)); !isErr {
		t.Error("set_lifecycle should reject a non-lifecycle value")
	}
	if _, isErr = callTool(t, s, "update",
		fmt.Sprintf(`{"ref":%q,"action":"set_stale_after","value":"not-a-date"}`, created.Path)); !isErr {
		t.Error("set_stale_after should reject a malformed date")
	}

	// daily
	text, isErr = callTool(t, s, "create", `{"kind":"daily"}`)
	if isErr || !strings.Contains(text, "daily/") {
		t.Errorf("daily create failed: %s", text)
	}
}

func TestMCPGraphBacklinks(t *testing.T) {
	s, project := mcpTestServer(t)
	cfg, err := config.Discover(project)
	if err != nil {
		t.Fatal(err)
	}

	// target + a note linking to it
	text, _ := callTool(t, s, "create", `{"title":"Target"}`)
	var target struct{ ID, Path string }
	_ = json.Unmarshal([]byte(text), &target)

	linker := "---\n" +
		"id: \"019680f0-7e00-7abc-8def-1234567890ab\"\n" +
		"title: \"Linker\"\n" +
		"type: \"note\"\n" +
		"tags: []\n" +
		"created: \"2026-02-13T10:45:00Z\"\n" +
		"timestamp: \"2026-02-13T10:45:00Z\"\n" +
		"---\n\nSee [Target](/" + target.Path + ").\n"
	writeConceptFile(t, cfg.RootPath, "linker--aaaa0001.md", linker)

	// graph db doesn't exist yet: the tool must sync it on first use.
	text, isErr := callTool(t, s, "graph", fmt.Sprintf(`{"action":"backlinks","ref":%q}`, target.Path))
	if isErr {
		t.Fatalf("backlinks failed: %s", text)
	}
	if !strings.Contains(text, "Linker") {
		t.Errorf("backlinks should include Linker, got %s", text)
	}
}

func TestMCPNoWiki(t *testing.T) {
	s := &mcpServer{startDir: t.TempDir(), out: &strings.Builder{}}
	text, isErr := callTool(t, s, "search", `{}`)
	if !isErr || !strings.Contains(text, "okfw init") {
		t.Errorf("tools without a wiki should error with init advice, got %s", text)
	}
}

func TestMCPBodyWriting(t *testing.T) {
	s, _ := mcpTestServer(t)

	// create with body
	text, isErr := callTool(t, s, "create",
		`{"title":"Body Note","body":"First paragraph.\n\n## Section\n\nDetails here."}`)
	if isErr {
		t.Fatalf("create with body failed: %s", text)
	}
	var created struct{ Path string }
	_ = json.Unmarshal([]byte(text), &created)

	text, _ = callTool(t, s, "read", fmt.Sprintf(`{"ref":%q}`, created.Path))
	if !strings.Contains(text, "First paragraph.") || !strings.Contains(text, "## Section") {
		t.Errorf("body not written:\n%s", text)
	}

	// append
	text, isErr = callTool(t, s, "update",
		fmt.Sprintf(`{"ref":%q,"action":"append","value":"Appended line."}`, created.Path))
	if isErr {
		t.Fatalf("append failed: %s", text)
	}
	text, _ = callTool(t, s, "read", fmt.Sprintf(`{"ref":%q}`, created.Path))
	if !strings.Contains(text, "Appended line.") || !strings.Contains(text, "First paragraph.") {
		t.Errorf("append should preserve existing body:\n%s", text)
	}

	// set_body replaces the body but keeps frontmatter
	text, isErr = callTool(t, s, "update",
		fmt.Sprintf(`{"ref":%q,"action":"set_body","value":"# Body Note\n\nRewritten."}`, created.Path))
	if isErr {
		t.Fatalf("set_body failed: %s", text)
	}
	text, _ = callTool(t, s, "read", fmt.Sprintf(`{"ref":%q}`, created.Path))
	if strings.Contains(text, "First paragraph.") {
		t.Errorf("set_body should replace the old body:\n%s", text)
	}
	if !strings.Contains(text, "Rewritten.") || !strings.Contains(text, `title: "Body Note"`) {
		t.Errorf("set_body should keep frontmatter and write the new body:\n%s", text)
	}

	// the rewritten note must still validate
	if !strings.Contains(text, `type: "note"`) {
		t.Errorf("frontmatter lost after set_body:\n%s", text)
	}
}

// TestMCPSearchFiltersByKindAndType: kind and type are the agent's levers for
// narrowing a mixed bundle, and each hit must say what it is so a mixed result
// set stays legible after the fact.
func TestMCPSearchFiltersByKindAndType(t *testing.T) {
	s, project := mcpTestServer(t)
	cfg, err := config.Discover(project)
	if err != nil {
		t.Fatal(err)
	}

	if _, isErr := callTool(t, s, "create", `{"title":"Handwritten Note","body":"about alpha"}`); isErr {
		t.Fatal("create failed")
	}
	p := writeConceptFile(t, cfg.RootPath, "alpha--0001.md", `---
id: "019680f0-7e00-7abc-8def-100000000000"
title: "Alpha Outage"
type: "Incident"
kind: "issue"
tags: []
created: "2026-02-13T11:00:00Z"
timestamp: "2026-02-13T11:00:00Z"
---

# Alpha Outage
`)
	if err := bundle.AutoSync(cfg, p); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		args, want, notWant string
	}{
		{`{"kind":"note"}`, "Handwritten Note", "Alpha Outage"},
		{`{"kind":"issue"}`, "Alpha Outage", "Handwritten Note"},
		{`{"type":"Incident"}`, "Alpha Outage", "Handwritten Note"},
	} {
		out, isErr := callTool(t, s, "search", tc.args)
		if isErr {
			t.Fatalf("search %s errored: %s", tc.args, out)
		}
		if !strings.Contains(out, tc.want) {
			t.Errorf("search %s missing %q: %s", tc.args, tc.want, out)
		}
		if strings.Contains(out, tc.notWant) {
			t.Errorf("search %s should not return %q: %s", tc.args, tc.notWant, out)
		}
	}

	out, _ := callTool(t, s, "search", `{"kind":"issue"}`)
	if !strings.Contains(out, `"kind":"issue"`) || !strings.Contains(out, `"type":"Incident"`) {
		t.Errorf("hits do not carry kind/type: %s", out)
	}
}

// TestMCPReadCapsAndSaysHowToContinue: the three narrowing paths worked and the
// default — the one an agent hits without thinking — returned the whole file
// with no bound and no marker.
func TestMCPReadCapsAndSaysHowToContinue(t *testing.T) {
	s, project := mcpTestServer(t)
	cfg, err := config.Discover(project)
	if err != nil {
		t.Fatal(err)
	}
	var body strings.Builder
	for i := 0; i < 500; i++ {
		fmt.Fprintf(&body, "line %d\n", i)
	}
	p := writeConceptFile(t, cfg.RootPath, "long--0001.md", `---
id: "019680f0-7e00-7abc-8def-100000000000"
title: "Long"
type: "note"
tags: []
created: "2026-02-13T11:00:00Z"
timestamp: "2026-02-13T11:00:00Z"
---

`+body.String())
	_ = p

	out, isErr := callTool(t, s, "read", `{"ref":"long--0001.md"}`)
	if isErr {
		t.Fatalf("read errored: %s", out)
	}
	if strings.Contains(out, "line 400") {
		t.Error("read returned the whole file")
	}
	if !strings.Contains(out, "more lines") || !strings.Contains(out, "offset:201") {
		t.Errorf("read did not say how to continue:\n%s", out[len(out)-200:])
	}

	// An explicit window gets the same marker; it used to get none.
	out, _ = callTool(t, s, "read", `{"ref":"long--0001.md","offset":1,"limit":10}`)
	if !strings.Contains(out, "offset:11") {
		t.Errorf("explicit window did not announce its cap:\n%s", out)
	}
}

// TestMCPGraphHonoursLimitOnEveryAction: limit was defaulted for all four
// actions and passed to exactly one, so three were unbounded while the schema
// advertised a default of 10.
func TestMCPGraphHonoursLimitOnEveryAction(t *testing.T) {
	s, project := mcpTestServer(t)
	cfg, err := config.Discover(project)
	if err != nil {
		t.Fatal(err)
	}

	const target = "019680f0-7e00-7abc-8def-1000000000ff"
	writeConceptFile(t, cfg.RootPath, "target--00ff.md", `---
id: "`+target+`"
title: "Target"
type: "note"
tags:
  - "shared"
created: "2026-02-13T11:00:00Z"
timestamp: "2026-02-13T11:00:00Z"
---

# Target
`)
	for i := 0; i < 15; i++ {
		id := fmt.Sprintf("019680f0-7e00-7abc-8def-2000000000%02d", i)
		writeConceptFile(t, cfg.RootPath, fmt.Sprintf("linker%d--%02d.md", i, i), `---
id: "`+id+`"
title: "Linker`+fmt.Sprint(i)+`"
type: "note"
tags:
  - "shared"
created: "2026-02-13T11:00:00Z"
timestamp: "2026-02-13T11:00:00Z"
---

# Linker

Points at [Target](/target--00ff.md) and at [Nowhere](/gone-`+fmt.Sprint(i)+`.md).
`)
	}

	for _, tc := range []struct{ name, args, key string }{
		{"backlinks", `{"action":"backlinks","ref":"` + target + `","limit":5}`, "nodes"},
		{"tag", `{"action":"tag","ref":"shared","limit":5}`, "nodes"},
		{"cluster", `{"action":"cluster","ref":"` + target + `","limit":5}`, "nodes"},
		{"dangling", `{"action":"dangling","limit":5}`, "dangling"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, isErr := callTool(t, s, "graph", tc.args)
			if isErr {
				t.Fatalf("graph %s errored: %s", tc.name, out)
			}
			var got map[string]json.RawMessage
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("not an object: %v\n%s", err, out)
			}
			var rows []json.RawMessage
			if err := json.Unmarshal(got[tc.key], &rows); err != nil {
				t.Fatalf("%s is not a list: %v\n%s", tc.key, err, out)
			}
			if len(rows) > 5 {
				t.Errorf("%s returned %d rows for limit 5", tc.name, len(rows))
			}
			if _, ok := got["total"]; !ok {
				t.Errorf("%s does not report a total: %s", tc.name, out)
			}
		})
	}
}

// TestMCPToolNamesAreNamespacedAndAliased: the tools are advertised as okfw_*
// so they read unambiguously in a client that flattens names across servers —
// `read` and `search` are not distinctive — while the bare names that shipped
// first keep dispatching, since breaking a prompt or a saved skill that hard-
// codes them buys nothing.
func TestMCPToolNamesAreNamespacedAndAliased(t *testing.T) {
	s, _ := mcpTestServer(t)

	for _, tool := range mcpTools() {
		name, _ := tool["name"].(string)
		if !strings.HasPrefix(name, "okfw_") {
			t.Errorf("tool %q is not namespaced", name)
		}
	}

	for _, bare := range []string{"search", "create", "read", "update", "graph"} {
		res := s.callTool(bare, json.RawMessage(`{}`))
		if res == nil || len(res.Content) == 0 {
			t.Fatalf("legacy name %q returned nothing", bare)
		}
		if strings.Contains(res.Content[0].Text, "unknown tool") {
			t.Errorf("legacy name %q no longer dispatches", bare)
		}
		// And the namespaced spelling reaches the same handler.
		ns := s.callTool("okfw_"+bare, json.RawMessage(`{}`))
		if ns == nil || len(ns.Content) == 0 {
			t.Fatalf("okfw_%s returned nothing", bare)
		}
		if ns.Content[0].Text != res.Content[0].Text {
			t.Errorf("okfw_%s and %s disagree:\n%s\n%s", bare, bare, ns.Content[0].Text, res.Content[0].Text)
		}
	}

	if res := s.callTool("okfw_nonexistent", nil); res == nil || !res.IsError {
		t.Error("an undeclared tool should error")
	}
}

// TestEveryMCPToolIsCappedAndSaysSo is the counterweight on okfw's MCP surface.
//
// Every other test here asserts a tool returns some particular thing, so all of
// them push toward returning more and nothing pushed back — which is how `read`
// came to return whole files unbounded, and how `graph` came to apply its
// advertised limit to one action out of four.
//
// The tool list is driven from mcpTools(), so a tool added later cannot escape.
func TestEveryMCPToolIsCappedAndSaysSo(t *testing.T) {
	s, project := mcpTestServer(t)
	cfg, err := config.Discover(project)
	if err != nil {
		t.Fatal(err)
	}

	// A bundle big enough that an unbounded tool shows it.
	var long strings.Builder
	for i := 0; i < 2000; i++ {
		fmt.Fprintf(&long, "line %d of a concept nobody wants in full\n", i)
	}
	for i := 0; i < 30; i++ {
		writeConceptFile(t, cfg.RootPath, fmt.Sprintf("bulk%02d--%02d.md", i, i), fmt.Sprintf(`---
id: "019680f0-7e00-7abc-8def-3000000000%02d"
title: "Bulk %d"
type: "note"
tags:
  - "bulk"
created: "2026-02-13T11:00:00Z"
timestamp: "2026-02-13T11:00:00Z"
---

# Bulk %d

Links to [Bulk 0](/bulk00--00.md) and [Nowhere](/gone-%d.md).

%s`, i, i, i, i, long.String()))
	}
	if _, isErr := callTool(t, s, "okfw_graph", `{"action":"dangling"}`); isErr {
		t.Fatal("graph sync failed")
	}

	args := map[string]string{
		"okfw_search": `{"q":"bulk"}`,
		"okfw_create": `{"title":"Probe"}`,
		"okfw_read":   `{"ref":"bulk00--00.md"}`,
		"okfw_update": `{"ref":"bulk00--00.md","action":"add_tags","value":"probed"}`,
		"okfw_graph":  `{"action":"tag","ref":"bulk"}`,
	}

	const maxUncapped = 64 * 1024

	for _, tool := range mcpTools() {
		name, _ := tool["name"].(string)
		a, ok := args[name]
		if !ok {
			t.Errorf("tool %s has no entry in this test; add one so it is bounded too", name)
			continue
		}
		t.Run(name, func(t *testing.T) {
			out, isErr := callTool(t, s, name, a)
			if isErr {
				t.Fatalf("%s errored: %s", name, out)
			}
			if len(out) <= maxUncapped {
				return
			}
			var got map[string]json.RawMessage
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("%s returned %d bytes of non-JSON with no cap", name, len(out))
			}
			if _, ok := got["truncated"]; !ok {
				t.Errorf("%s returned %d bytes without announcing a cap", name, len(out))
			}
		})
	}
}
