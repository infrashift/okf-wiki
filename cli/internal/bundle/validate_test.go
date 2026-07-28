package bundle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"okfw/cli/internal/config"
)

const legacyTodo = `---
id: "019680f0-7e00-7abc-8def-1234567890ab"
title: "Ship it"
type: "todo"
kind: "todo"
status: "closed"
due: "2026-08-01"
completed: "2026-07-20"
priority: "high"
tags:
  - "release"
created: "2026-07-01T10:00:00Z"
timestamp: "2026-07-20T10:00:00Z"
---

# Ship it
`

func TestValidateGenerated(t *testing.T) {
	good := GenerateBasicNote("019680f0-7e00-7abc-8def-1234567890ab", "Fine", "", nil)
	if err := ValidateGenerated([]byte(good)); err != nil {
		t.Errorf("a generated note should validate: %v", err)
	}
	if err := ValidateGenerated([]byte("---\nid: \"nope\"\n---\n")); err == nil {
		t.Error("expected an error for an invalid concept")
	}
}

// --- StripLegacyTodoFields ---

func TestStripLegacyTodoFieldsClosedBecomesDeprecated(t *testing.T) {
	out, dropped, err := StripLegacyTodoFields([]byte(legacyTodo))
	if err != nil {
		t.Fatalf("StripLegacyTodoFields: %v", err)
	}
	if out == nil {
		t.Fatal("expected a rewrite")
	}
	s := string(out)

	// A closed todo is most faithfully a deprecated concept.
	if !strings.Contains(s, `status: "deprecated"`) {
		t.Errorf("closed should become deprecated:\n%s", s)
	}
	for _, gone := range []string{"due:", "completed:", "priority:", "kind:"} {
		if strings.Contains(s, gone) {
			t.Errorf("%s should have been removed:\n%s", gone, s)
		}
	}
	// The timestamp records the last meaningful change; dropping retired
	// bookkeeping is not one.
	if !strings.Contains(s, `timestamp: "2026-07-20T10:00:00Z"`) {
		t.Errorf("timestamp must be preserved:\n%s", s)
	}
	if !strings.Contains(s, `- "release"`) || !strings.Contains(s, "# Ship it") {
		t.Errorf("tags and body must survive:\n%s", s)
	}
	if _, err := config.ParseAndValidate(out); err != nil {
		t.Fatalf("migrated concept must validate: %v\n%s", err, s)
	}

	for _, want := range []string{"status=closed", "kind=todo", "due", "completed", "priority"} {
		found := false
		for _, d := range dropped {
			if d == want {
				found = true
			}
		}
		if !found {
			t.Errorf("dropped list missing %q, got %v", want, dropped)
		}
	}
}

func TestStripLegacyTodoFieldsOpenLosesStatusEntirely(t *testing.T) {
	src := strings.Replace(legacyTodo, `status: "closed"`, `status: "open"`, 1)
	out, _, err := StripLegacyTodoFields([]byte(src))
	if err != nil {
		t.Fatalf("StripLegacyTodoFields: %v", err)
	}
	// Absent status means stable (OKF section 5.4), which is right for a task
	// that was never finished.
	if strings.Contains(string(out), "status:") {
		t.Errorf("an open todo should lose status entirely:\n%s", out)
	}
	if _, err := config.ParseAndValidate(out); err != nil {
		t.Fatalf("must validate: %v\n%s", err, out)
	}
}

func TestStripLegacyTodoFieldsIsANoOpOnCleanConcepts(t *testing.T) {
	clean := GenerateBasicNote("019680f0-7e00-7abc-8def-1234567890ab", "Clean", "", nil)
	out, dropped, err := StripLegacyTodoFields([]byte(clean))
	if err != nil {
		t.Fatalf("StripLegacyTodoFields: %v", err)
	}
	if out != nil || dropped != nil {
		t.Errorf("a clean concept needs no rewrite, got %d dropped: %s", len(dropped), out)
	}
}

func TestStripLegacyTodoFieldsIsIdempotent(t *testing.T) {
	once, _, err := StripLegacyTodoFields([]byte(legacyTodo))
	if err != nil {
		t.Fatal(err)
	}
	twice, dropped, err := StripLegacyTodoFields(once)
	if err != nil {
		t.Fatal(err)
	}
	if twice != nil {
		t.Errorf("second pass should find nothing to do, got: %s (dropped %v)", twice, dropped)
	}
}

// An OKF lifecycle status must not be mistaken for legacy todo state.
func TestStripLegacyTodoFieldsLeavesRealLifecycleStatusAlone(t *testing.T) {
	src := strings.Replace(legacyTodo, `status: "closed"`, `status: "draft"`, 1)
	out, dropped, err := StripLegacyTodoFields([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `status: "draft"`) {
		t.Errorf("a real lifecycle status must survive:\n%s", out)
	}
	for _, d := range dropped {
		if strings.HasPrefix(d, "status=") {
			t.Errorf("draft should not be reported as dropped: %v", dropped)
		}
	}
}

// --- PruneIndex ---



func TestLiveConceptIDs(t *testing.T) {
	cfg, root := testCfg(t)
	_, id, err := CreateConcept(cfg, Spec{Title: "Real"})
	if err != nil {
		t.Fatal(err)
	}
	// A reserved file and a hidden directory must not contribute ids.
	if err := os.WriteFile(filepath.Join(root, "index.md"),
		[]byte("---\nid: \"019680f0-7e00-7abc-8def-999999999999\"\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".index"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".index", "x.md"),
		[]byte("---\nid: \"019680f0-7e00-7abc-8def-888888888888\"\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}

	live, err := LiveConceptIDs(root)
	if err != nil {
		t.Fatalf("LiveConceptIDs: %v", err)
	}
	if !live[id] {
		t.Errorf("missing the real concept %s", id)
	}
	if len(live) != 1 {
		t.Errorf("got %d live ids, want 1: %v", len(live), live)
	}
}
