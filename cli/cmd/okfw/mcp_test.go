package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	if !strings.Contains(text, `status: "deprecated"`) || !strings.Contains(text, `stale_after: "2026-12-31T00:00:00Z"`) {
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

// TestMCPUpdateKeepsACRLFConceptCRLF: every update action writes into a file
// saved on Windows; each used to write its own lines as LF.
func TestMCPUpdateKeepsACRLFConceptCRLF(t *testing.T) {
	s, project := mcpTestServer(t)
	text, isErr := callTool(t, s, "create", `{"title":"Windows Note","body":"Saved on Windows."}`)
	if isErr {
		t.Fatalf("create failed: %s", text)
	}
	var created struct{ Path string }
	_ = json.Unmarshal([]byte(text), &created)
	path := filepath.Join(project, ".okfw", created.Path)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes.ReplaceAll(content, []byte("\n"), []byte("\r\n")), 0644); err != nil {
		t.Fatal(err)
	}

	for _, upd := range []string{
		`"action":"append","value":"Appended.\nTwo lines."`,
		`"action":"set_lifecycle","value":"draft"`,
		`"action":"add_tags","value":"windows"`,
		`"action":"set_body","value":"Rewritten.\n\nNew paragraph."`,
		`"action":"verify","value":"human:tester"`,
	} {
		if text, isErr := callTool(t, s, "update", fmt.Sprintf(`{"ref":%q,%s}`, created.Path, upd)); isErr {
			t.Fatalf("update {%s} failed: %s", upd, text)
		}
		got, _ := os.ReadFile(path)
		if bytes.Count(got, []byte("\n")) != bytes.Count(got, []byte("\r\n")) {
			t.Fatalf("update {%s} mixed line endings into a CRLF concept:\n%q", upd, got)
		}
		if _, err := config.ParseAndValidate(got); err != nil {
			t.Fatalf("update {%s} left an invalid concept: %v\n%q", upd, err, got)
		}
	}
}

// TestMCPCitationsMustResolve: an agent appending a [^label] claim before the
// source exists is refused with the fix, the fix lets the same append through,
// and add_source will not create a second entry under an id already taken.
func TestMCPCitationsMustResolve(t *testing.T) {
	s, _ := mcpTestServer(t)
	text, isErr := callTool(t, s, "create", `{"title":"GA4 Export","body":"The export."}`)
	if isErr {
		t.Fatalf("create failed: %s", text)
	}
	var created struct{ Path string }
	_ = json.Unmarshal([]byte(text), &created)
	update := func(action, value string) (string, bool) {
		return callTool(t, s, "update", fmt.Sprintf(`{"ref":%q,"action":%q,"value":%q}`, created.Path, action, value))
	}

	claim := "Sharded daily.[^ga4-schema]"
	if text, isErr := update("append", claim); !isErr || !strings.Contains(text, "add-source") {
		t.Fatalf("append citing nothing: isErr=%v %s; want a refusal naming the fix", isErr, text)
	}
	source := `{"resource":"https://example.com/schema","id":"ga4-schema"}`
	if text, isErr := update("add_source", source); isErr {
		t.Fatalf("add_source: %s", text)
	}
	if text, isErr := update("append", claim); isErr {
		t.Fatalf("append after add_source: %s", text)
	}
	if text, isErr := update("add_source", source); !isErr || !strings.Contains(text, "already has an entry") {
		t.Errorf("second add_source with the same id: isErr=%v %s", isErr, text)
	}
}

// TestMCPRefusesAMalformedActor: an agent passing Human:alice would have
// recorded a review that reads back as machine-confirmed.
func TestMCPRefusesAMalformedActor(t *testing.T) {
	s, _ := mcpTestServer(t)
	text, isErr := callTool(t, s, "create", `{"title":"Reviewed","body":"Checked."}`)
	if isErr {
		t.Fatalf("create failed: %s", text)
	}
	var created struct{ Path string }
	_ = json.Unmarshal([]byte(text), &created)
	for _, action := range []string{"verify", "set_generated"} {
		text, isErr := callTool(t, s, "update",
			fmt.Sprintf(`{"ref":%q,"action":%q,"value":"Human:alice"}`, created.Path, action))
		if !isErr || !strings.Contains(text, "lowercase prefix human:") {
			t.Errorf("%s Human:alice: isErr=%v %s; want a refusal with the hint", action, isErr, text)
		}
	}
}

// TestMCPSearchFiltersByKindAndType: kind and type are the agent's levers for
// narrowing a mixed bundle, and each hit must say what it is so a mixed result
// set stays legible after the fact.
// TestMCPSearchHitsSayHowFarToTrustThem: trust on every hit, status and stale
// only when they are the exception, so a current stable hit costs nothing.
func TestMCPSearchHitsSayHowFarToTrustThem(t *testing.T) {
	s, project := mcpTestServer(t)
	cfg, err := config.Discover(project)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ file, title, extra string }{
		{"a--0001.md", "Ledger Reviewed", "verified:\n  by: \"human:ryan\"\n"},
		{"b--0002.md", "Ledger Retired", "status: \"deprecated\"\nstale_after: \"2020-01-01T00:00:00Z\"\n"},
	} {
		writeConceptFile(t, cfg.RootPath, c.file, "---\nid: \"019680f0-7e00-7abc-8def-"+
			strings.Repeat(c.file[:1], 12)+"\"\ntitle: \""+c.title+"\"\ntype: \"note\"\ntags: []\n"+
			"created: \"2026-02-13T11:00:00Z\"\ntimestamp: \"2026-02-13T11:00:00Z\"\n"+c.extra+"---\n\nledger\n")
	}
	text, isErr := callTool(t, s, "search", `{"q":"ledger"}`)
	if isErr {
		t.Fatalf("search failed: %s", text)
	}
	var out struct{ Results []map[string]interface{} }
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatal(err)
	}
	byTitle := map[string]map[string]interface{}{}
	for _, h := range out.Results {
		byTitle[h["title"].(string)] = h
	}
	reviewed, retired := byTitle["Ledger Reviewed"], byTitle["Ledger Retired"]
	if reviewed == nil || retired == nil {
		t.Fatalf("missing hits: %s", text)
	}
	if reviewed["trust"] != "human-reviewed" || retired["trust"] != "unverified" {
		t.Errorf("trust = %v / %v, want human-reviewed / unverified", reviewed["trust"], retired["trust"])
	}
	if _, has := reviewed["status"]; has {
		t.Errorf("a stable hit carries status: %v", reviewed)
	}
	if _, has := reviewed["stale"]; has {
		t.Errorf("a current hit carries stale: %v", reviewed)
	}
	if retired["status"] != "deprecated" || retired["stale"] != true {
		t.Errorf("retired hit = %v, want status deprecated and stale true", retired)
	}
}

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

// TestMCPReadSectionIsCappedAndFindsRealHeadings: a section read returned the
// whole section, however long, and took any "# " line for a heading — so a
// shell comment in a code block ended the section, and a YAML comment in the
// frontmatter could be "found" as one.
func TestMCPReadSectionIsCappedAndFindsRealHeadings(t *testing.T) {
	s, project := mcpTestServer(t)
	cfg, err := config.Discover(project)
	if err != nil {
		t.Fatal(err)
	}
	var long strings.Builder
	for i := 0; i < 500; i++ {
		fmt.Fprintf(&long, "row %d\n", i)
	}
	writeConceptFile(t, cfg.RootPath, "sections--0001.md", `---
id: "019680f0-7e00-7abc-8def-100000000001"
title: "Sections"
# Setup
type: "note"
tags: []
created: "2026-02-13T11:00:00Z"
timestamp: "2026-02-13T11:00:00Z"
---

# Sections

## Setup ##

`+"```bash\n# install the tools\nmake install\n```"+`

After the code block.

## Data

`+long.String()+`
## Tail

The end.
`)

	out, isErr := callTool(t, s, "read", `{"ref":"sections--0001.md","section":"setup"}`)
	if isErr {
		t.Fatalf("section read errored: %s", out)
	}
	if !strings.HasPrefix(out, "## Setup ##") {
		t.Errorf("matched the frontmatter comment, or missed the closing-#s heading:\n%s", out)
	}
	if !strings.Contains(out, "After the code block.") || strings.Contains(out, "## Data") {
		t.Errorf("section should run past the code comment and stop at ## Data:\n%s", out)
	}

	out, _ = callTool(t, s, "read", `{"ref":"sections--0001.md","section":"Data"}`)
	if strings.Contains(out, "row 300") {
		t.Error("section read returned the whole section uncapped")
	}
	if !strings.Contains(out, `continue with section:"Data", offset:201`) {
		t.Errorf("section read did not say how to continue:\n%s", out[max(0, len(out)-200):])
	}
	out, _ = callTool(t, s, "read", `{"ref":"sections--0001.md","section":"Data","offset":201,"limit":400}`)
	if !strings.HasPrefix(out, "row 198\n") || !strings.Contains(out, "row 499") || strings.Contains(out, "## Tail") {
		t.Errorf("continuing a section read should page within it:\n%.200s", out)
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
			// cluster capped inside the walk, so it reported the capped count
			// as its total and never said truncated. dangling returned no rows
			// at all: a link to a missing file never became a graph edge.
			var total int
			if err := json.Unmarshal(got["total"], &total); err != nil || total < 15 {
				t.Errorf("%s total = %s, want the uncapped count (15+)", tc.name, got["total"])
			}
			if string(got["truncated"]) != "true" {
				t.Errorf("%s withheld rows without saying truncated: %s", tc.name, out)
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

// TestMCPRefsCannotLeaveTheBundle: a ref is supplied by the client unvetted.
// It used to be joined to the bundle root with no containment check, so
// okfw_read returned /etc/hostname and okfw_update set_body rewrote a markdown
// file anywhere on disk.
func TestMCPRefsCannotLeaveTheBundle(t *testing.T) {
	s, _ := mcpTestServer(t)
	outside := filepath.Join(t.TempDir(), "victim.md")
	const original = "---\ntype: \"note\"\n---\n\noriginal\n"
	if err := os.WriteFile(outside, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{outside, "../../../../../../" + strings.TrimPrefix(outside, "/")} {
		if out, isErr := callTool(t, s, "read", fmt.Sprintf(`{"ref":%q}`, ref)); !isErr {
			t.Errorf("read %s succeeded: %s", ref, out)
		}
		if out, isErr := callTool(t, s, "update",
			fmt.Sprintf(`{"ref":%q,"action":"set_body","value":"PWNED"}`, ref)); !isErr {
			t.Errorf("update %s succeeded: %s", ref, out)
		}
	}
	if got, _ := os.ReadFile(outside); string(got) != original {
		t.Errorf("file outside the bundle was rewritten:\n%s", got)
	}
}

// TestMCPReadAcceptsAnOKFConceptID: the spec names a concept by its path minus
// ".md", and that spelling used to come back "file not found".
func TestMCPReadAcceptsAnOKFConceptID(t *testing.T) {
	s, _ := mcpTestServer(t)
	text, isErr := callTool(t, s, "create", `{"title":"Income Statement","dir":"metrics"}`)
	if isErr {
		t.Fatalf("create: %s", text)
	}
	var created struct{ Path string }
	if err := json.Unmarshal([]byte(text), &created); err != nil {
		t.Fatal(err)
	}
	id := strings.TrimSuffix(created.Path, ".md")
	if out, isErr := callTool(t, s, "read", fmt.Sprintf(`{"ref":%q}`, id)); isErr || !strings.Contains(out, "Income Statement") {
		t.Errorf("read %s: %s", id, out)
	}
}

// TestMCPSearchSurvivesBadPaging: a negative offset sliced out of range and
// the panic killed the stdio server, so every later call hung.
func TestMCPSearchSurvivesBadPaging(t *testing.T) {
	s, _ := mcpTestServer(t)
	if _, isErr := callTool(t, s, "create", `{"title":"Paged"}`); isErr {
		t.Fatal("create failed")
	}
	for _, args := range []string{`{"offset":-1}`, `{"limit":-5,"offset":-100}`, `{"limit":100000}`} {
		if out, isErr := callTool(t, s, "search", args); isErr || !strings.Contains(out, "Paged") {
			t.Errorf("search %s: %s", args, out)
		}
	}
	if resp := s.handleLine(`{"jsonrpc":"2.0","id":9,"method":"ping"}`); resp == nil || resp.Error != nil {
		t.Errorf("server unresponsive after bad paging: %+v", resp)
	}
}

// TestGraphQueryClusterSaysWhatItsLimitWithheld: the CLI's cluster stopped its
// walk at --limit, so like the MCP action it could not report what it left out.
func TestGraphQueryClusterSaysWhatItsLimitWithheld(t *testing.T) {
	cfg := bundleAt(t)
	writeConceptFile(t, cfg.RootPath, "hub--0000.md",
		conceptContent("019680f0-7e00-7abc-8def-400000000000", "Hub", "note", "# Hub"))
	for i := 1; i <= 12; i++ {
		writeConceptFile(t, cfg.RootPath, fmt.Sprintf("spoke--%04d.md", i),
			conceptContent(fmt.Sprintf("019680f0-7e00-7abc-8def-4000000000%02d", i), "Spoke", "note",
				"# Spoke\n\n[Hub](/hub--0000.md)"))
	}

	out, err := captureStdout(t, func() error {
		_, err := runCmd(t, rootCmd(), "graph-query", "cluster", "hub--0000", "--limit", "5", "--json")
		return err
	})
	if err != nil {
		t.Fatalf("cluster: %v", err)
	}
	var got struct {
		Nodes     []json.RawMessage `json:"nodes"`
		Total     int               `json:"total"`
		Truncated bool              `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if len(got.Nodes) != 5 || got.Total != 13 || !got.Truncated {
		t.Errorf("cluster --limit 5 = %d nodes, total %d, truncated %v; want 5, 13, true",
			len(got.Nodes), got.Total, got.Truncated)
	}

	out, _ = captureStdout(t, func() error {
		_, err := runCmd(t, rootCmd(), "graph-query", "cluster", "hub--0000", "--limit", "5", "--json=false")
		return err
	})
	if !strings.Contains(out, "… 8 more in reach — raise --limit") {
		t.Errorf("text output did not say what it withheld:\n%s", out)
	}
}
