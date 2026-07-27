package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

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
	for _, tool := range []string{`"search"`, `"create"`, `"read"`, `"update"`, `"graph"`} {
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
