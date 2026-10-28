package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"okfw/cli/internal/config"
)

// createdPath creates a concept through the MCP tool and returns its absolute
// path.
func createdPath(t *testing.T, s *mcpServer, project, args string) string {
	t.Helper()
	text, isErr := callTool(t, s, "okfw_create", args)
	if isErr {
		t.Fatalf("create %s: %s", args, text)
	}
	var c struct{ Path string }
	if err := json.Unmarshal([]byte(text), &c); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(project, ".okfw", filepath.FromSlash(c.Path))
}

// TestMCPInstructionsStayShort: initialize carries the conventions, because
// they are the only guidance every MCP client receives — and they ride in the
// agent's context all session, so they stay small. Once the wiki has
// directories, the instructions name them, so a first listing is chosen rather
// than guessed.
func TestMCPInstructionsStayShort(t *testing.T) {
	s, project := mcpTestServer(t)

	instructions := func() string {
		resp := s.handleLine(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{}}}`)
		if resp == nil || resp.Error != nil {
			t.Fatalf("initialize failed: %+v", resp)
		}
		got, _ := resp.Result.(map[string]interface{})["instructions"].(string)
		return got
	}

	got := instructions()
	if got != mcpInstructions {
		t.Errorf("an empty wiki should get the static instructions, got:\n%s", got)
	}
	if len(mcpInstructions) > 1200 {
		t.Errorf("instructions are %d bytes; they stay in every agent's context", len(mcpInstructions))
	}
	for _, must := range []string{"okfw_search", `okfw_read {"ref":"/"}`, "index.md", "description", "deprecat"} {
		if !strings.Contains(got, must) {
			t.Errorf("instructions do not mention %q", must)
		}
	}

	createdPath(t, s, project, `{"title":"An issue","dir":"issues"}`)
	createdPath(t, s, project, `{"title":"A decision","dir":"decisions/adr"}`)
	got = instructions()
	if !strings.Contains(got, "decisions/, issues/") {
		t.Errorf("instructions should name the top-level directories, got:\n%s", got)
	}
	if strings.Contains(got, "ephemeral") || strings.Contains(got, "adr/") {
		t.Errorf("instructions should name only top-level, walkable directories, got:\n%s", got)
	}
}

// TestMCPReadListsADirectory: OKF section 8's index.md listings are the
// progressive-disclosure layer, and okfw_read could not reach them — every ref
// had to name a concept.
func TestMCPReadListsADirectory(t *testing.T) {
	s, project := mcpTestServer(t)
	createdPath(t, s, project, `{"title":"Login fails","dir":"issues","description":"The login form rejects valid passwords."}`)

	root, isErr := callTool(t, s, "okfw_read", `{"ref":"/"}`)
	if isErr || !strings.Contains(root, "issues/") || !strings.Contains(root, "okf_version") {
		t.Errorf(`read "/" should return the root listing, got: %s`, root)
	}
	for _, ref := range []string{"issues/", "issues", "/issues/", "issues/index.md"} {
		out, isErr := callTool(t, s, "okfw_read", fmt.Sprintf(`{"ref":%q}`, ref))
		if isErr || !strings.Contains(out, "Login fails") || !strings.Contains(out, "rejects valid passwords") {
			t.Errorf("read %q should return the issues listing with descriptions, got: %s", ref, out)
		}
	}
	if out, isErr := callTool(t, s, "okfw_read", `{"ref":"issues/","section":"Concepts"}`); isErr || !strings.Contains(out, "Login fails") {
		t.Errorf("a listing should be readable by section, got: %s", out)
	}

	// A concept still wins over a directory of the same name when the ref is
	// not spelled as a directory.
	createdPath(t, s, project, `{"title":"Notes"}`)
	if out, isErr := callTool(t, s, "okfw_read", `{"ref":"nowhere/"}`); !isErr {
		t.Errorf("a missing directory should be an error, got: %s", out)
	}
	for _, ref := range []string{"../", "../../", ".git/", "issues/../../", "ephemeral/../../"} {
		if out, isErr := callTool(t, s, "okfw_read", fmt.Sprintf(`{"ref":%q}`, ref)); !isErr {
			t.Errorf("read %q must not leave the bundle, got: %s", ref, out)
		}
	}

	// A symlinked directory that points out of the bundle is refused too.
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "index.md"), []byte("# secret\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(project, ".okfw", "escape")); err != nil {
		t.Skip("symlinks unsupported:", err)
	}
	if out, isErr := callTool(t, s, "okfw_read", `{"ref":"escape/"}`); !isErr {
		t.Errorf("a symlink out of the bundle must be refused, got: %s", out)
	}
}

// TestMCPCreateSetsMetadataInOneWrite: an issue with its lifecycle, trust and
// provenance took one create and four updates, and a failure part-way left a
// half-described concept on disk.
func TestMCPCreateSetsMetadataInOneWrite(t *testing.T) {
	s, project := mcpTestServer(t)
	path := createdPath(t, s, project, `{"title":"Login fails","kind":"issue","dir":"issues",
		"tags":["open"],"description":"The login form rejects valid passwords.",
		"status":"draft","stale_after":"2026-12-31","resource":"https://example.com/login",
		"generated_by":"claude-code/1.0"}`)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	c, err := config.ParseAndValidate(content)
	if err != nil {
		t.Fatalf("created concept does not validate: %v\n%s", err, content)
	}
	if c.GetKind() != "issue" || c.Type != "issue" {
		t.Errorf("kind issue should write kind and type issue, got kind=%q type=%q", c.GetKind(), c.Type)
	}
	if c.Status != "draft" || c.StaleAfter != "2026-12-31T00:00:00Z" || c.Resource != "https://example.com/login" {
		t.Errorf("metadata not set: status=%q stale_after=%q resource=%q", c.Status, c.StaleAfter, c.Resource)
	}
	if c.Generated == nil || c.Generated.By != "claude-code/1.0" {
		// ParseAndValidate above rejects a generated.at that disagrees with
		// timestamp, so the "at" half is already checked.
		t.Errorf("generated should be claude-code/1.0 at the timestamp, got %+v (timestamp %v)", c.Generated, c.Timestamp)
	}
	if !strings.Contains(string(content), "type: \"issue\"\nkind: \"issue\"") {
		t.Errorf("kind should sit after type, as the issue template writes it:\n%s", content)
	}

	typed := createdPath(t, s, project, `{"title":"Q3 revenue","type":"Metric"}`)
	if content, _ := os.ReadFile(typed); !strings.Contains(string(content), `type: "Metric"`) || strings.Contains(string(content), "kind:") {
		t.Errorf("type should override and a note writes no kind:\n%s", content)
	}

	// Every bad field is refused before anything is written.
	before := countConcepts(t, project)
	for _, args := range []string{
		`{"title":"Bad actor","generated_by":"Human:ryan"}`,
		`{"title":"Bad status","status":"open"}`,
		`{"title":"Bad date","stale_after":"soon"}`,
		`{"title":"Bad kind","kind":"todo"}`,
		`{"kind":"daily","status":"draft"}`,
	} {
		if out, isErr := callTool(t, s, "okfw_create", args); !isErr {
			t.Errorf("create %s should fail, got: %s", args, out)
		}
	}
	if after := countConcepts(t, project); after != before {
		t.Errorf("a refused create wrote %d concept(s)", after-before)
	}
}

func countConcepts(t *testing.T, project string) int {
	t.Helper()
	n := 0
	filepath.Walk(filepath.Join(project, ".okfw"), func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() && strings.HasSuffix(p, ".md") && fi.Name() != "index.md" {
			n++
		}
		return nil
	})
	return n
}

// TestMCPRemoveTags: closing an issue swaps its state tag, and nothing could
// remove a tag except a hand edit.
func TestMCPRemoveTags(t *testing.T) {
	s, project := mcpTestServer(t)
	path := createdPath(t, s, project, `{"title":"Login fails","kind":"issue","tags":["open","auth"]}`)

	if out, isErr := callTool(t, s, "okfw_update", fmt.Sprintf(`{"ref":%q,"action":"remove_tags","value":"open, absent"}`, path)); isErr {
		t.Fatalf("remove_tags: %s", out)
	}
	content, _ := os.ReadFile(path)
	c, err := config.ParseAndValidate(content)
	if err != nil {
		t.Fatalf("concept does not validate after remove_tags: %v", err)
	}
	if strings.Join(c.Tags, ",") != "auth" {
		t.Errorf("tags after removing open: %v", c.Tags)
	}

	if out, isErr := callTool(t, s, "okfw_update", fmt.Sprintf(`{"ref":%q,"action":"remove_tags","value":"auth"}`, path)); isErr {
		t.Fatalf("remove_tags: %s", out)
	}
	content, _ = os.ReadFile(path)
	if _, err := config.ParseAndValidate(content); err != nil || !strings.Contains(string(content), "tags: []") {
		t.Errorf("removing every tag should leave a valid `tags: []`, got err=%v:\n%s", err, content)
	}
}

// TestMCPSearchScopesByDirAndTags: a bundle's directories are its disclosure
// structure, so a search can be held to one; and the CLI's repeatable --tag
// had no MCP equivalent.
func TestMCPSearchScopesByDirAndTags(t *testing.T) {
	s, project := mcpTestServer(t)
	createdPath(t, s, project, `{"title":"Login fails","dir":"issues/auth","tags":["open","auth"]}`)
	createdPath(t, s, project, `{"title":"Login closed","dir":"issues","tags":["resolved","auth"]}`)
	createdPath(t, s, project, `{"title":"Login design","dir":"decisions","tags":["auth"]}`)

	count := func(args string) int {
		out, isErr := callTool(t, s, "okfw_search", args)
		if isErr {
			t.Fatalf("search %s: %s", args, out)
		}
		var r struct{ Total int }
		json.Unmarshal([]byte(out), &r)
		return r.Total
	}
	for args, want := range map[string]int{
		`{"q":"login"}`:                                  3,
		`{"q":"login","dir":"issues"}`:                   2,
		`{"q":"login","dir":"/issues/auth/"}`:            1,
		`{"tags":["auth","open"]}`:                       1,
		`{"tag":"auth","tags":["resolved"]}`:             1,
		`{"dir":"issues","tags":["auth"]}`:               2,
		`{"dir":"decisions","tags":["auth","resolved"]}`: 0,
	} {
		if got := count(args); got != want {
			t.Errorf("search %s: %d hits, want %d", args, got, want)
		}
	}
	for _, dir := range []string{"../", "nowhere", ".git"} {
		if out, isErr := callTool(t, s, "okfw_search", fmt.Sprintf(`{"dir":%q}`, dir)); !isErr {
			t.Errorf("search dir %q should fail, got: %s", dir, out)
		}
	}
}
