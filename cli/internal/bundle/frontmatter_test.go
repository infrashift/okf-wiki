package bundle

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// graphyConcept mirrors what `graphy export okfw` writes: okfw's quoted house
// style plus a nested extension block. Editing one of these is the case the old
// line-oriented editor could not handle.
const graphyConcept = `---
id: "019680f0-7e00-7abc-8def-1234567890ab"
title: "ParseFrontmatter"
type: "code-function"
description: "Extracts YAML frontmatter."
tags:
  - "code"
created: "2026-07-25T11:30:18-05:00"
timestamp: "2026-07-25T11:30:18-05:00"
graphy:
  node_id: "go:function:concept.go#ParseFrontmatter"
  language: "go"
  entity: "function"
---

# ParseFrontmatter

Body text.

<!-- graphy:node go:function:concept.go#ParseFrontmatter -->
`

func TestEditPreservesUnknownKeysAndComments(t *testing.T) {
	content := []byte(`---
# leading comment
id: "019680f0-7e00-7abc-8def-1234567890ab"
title: "Test"
unknown_key: "kept"   # trailing comment
nested:
  deep: "value"
---

# Test
`)
	out, err := UpdateFrontmatter(content, map[string]string{"timestamp": "2026-07-25T12:00:00Z"})
	if err != nil {
		t.Fatalf("UpdateFrontmatter: %v", err)
	}
	s := string(out)
	for _, want := range []string{
		"# leading comment",
		`unknown_key: "kept"`,
		"# trailing comment",
		"nested:",
		`deep: "value"`,
		"# Test",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("round-trip dropped %q\ngot:\n%s", want, s)
		}
	}
}

func TestEditPreservesKeyOrder(t *testing.T) {
	out, err := UpdateFrontmatter([]byte(graphyConcept), map[string]string{"timestamp": "2026-07-25T13:00:00Z"})
	if err != nil {
		t.Fatalf("UpdateFrontmatter: %v", err)
	}
	want := []string{"id:", "title:", "type:", "description:", "tags:", "created:", "timestamp:", "graphy:"}
	s := string(out)
	at := 0
	for _, key := range want {
		i := strings.Index(s[at:], "\n"+key)
		if i < 0 {
			t.Fatalf("key %q missing or out of order in:\n%s", key, s)
		}
		at += i + 1
	}
}

func TestEditKeepsHouseStyleIndent(t *testing.T) {
	out, err := UpdateFrontmatter([]byte(graphyConcept), nil)
	if err != nil {
		t.Fatalf("UpdateFrontmatter: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "tags:\n  - \"code\"") {
		t.Errorf("sequence should indent by 2, got:\n%s", s)
	}
	if !strings.Contains(s, "graphy:\n  node_id:") {
		t.Errorf("nested mapping should indent by 2, got:\n%s", s)
	}
}

func TestEditOnlyChangesTheTargetedLine(t *testing.T) {
	before := []byte(graphyConcept)
	after, err := UpdateFrontmatter(before, map[string]string{"timestamp": "2026-07-25T14:00:00Z"})
	if err != nil {
		t.Fatalf("UpdateFrontmatter: %v", err)
	}
	bl := strings.Split(string(before), "\n")
	al := strings.Split(string(after), "\n")
	if len(bl) != len(al) {
		t.Fatalf("line count changed %d -> %d:\n%s", len(bl), len(al), after)
	}
	var diffs []string
	for i := range bl {
		if bl[i] != al[i] {
			diffs = append(diffs, bl[i]+" -> "+al[i])
		}
	}
	if len(diffs) != 1 || !strings.Contains(diffs[0], "timestamp") {
		t.Errorf("expected exactly the timestamp line to change, got %v", diffs)
	}
}

// SetPath creating a block that does not exist yet is the whole reason for the
// yaml.Node rewrite. Exercised through Edit rather than UpdateFrontmatter so it
// tests the mechanism alone: an arbitrary new block is not in the concept
// schema, and UpdateFrontmatter's mutation guard would rightly reject it.
func TestSetPathCreatesNestedBlock(t *testing.T) {
	out, err := Edit([]byte(graphyConcept), func(m *yaml.Node) error {
		return SetPath(m, "scratch.status", "closed")
	})
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if !strings.Contains(string(out), "scratch:\n  status: \"closed\"") {
		t.Errorf("expected a nested scratch.status block, got:\n%s", out)
	}
}

// A mutation that would break a valid concept must be refused before anything
// reaches disk.
func TestUpdateFrontmatterRefusesToBreakAValidConcept(t *testing.T) {
	if _, err := UpdateFrontmatter([]byte(graphyConcept), map[string]string{"id": "not-a-uuid"}); err == nil {
		t.Error("expected an invalid id to be refused")
	}
	if _, err := UpdateFrontmatter([]byte(graphyConcept), map[string]string{"title": ""}); err == nil {
		t.Error("expected an empty title to be refused")
	}
	if _, err := UpdateFrontmatter([]byte(graphyConcept), map[string]string{"undeclared_field": "x"}); err == nil {
		t.Error("expected an undeclared field to be refused by the closed schema")
	}
}

// An already-invalid concept must stay editable, or the CLI could never be used
// to repair one.
func TestUpdateFrontmatterStillEditsAnAlreadyInvalidConcept(t *testing.T) {
	broken := []byte("---\nid: \"12345\"\ntitle: \"\"\n---\n\n# Broken\n")
	out, err := UpdateFrontmatter(broken, map[string]string{"title": "Repaired"})
	if err != nil {
		t.Fatalf("editing an already-invalid concept should be allowed: %v", err)
	}
	if !strings.Contains(string(out), `title: "Repaired"`) {
		t.Errorf("edit not applied:\n%s", out)
	}
}

func TestSetPathUpdatesExistingNestedKey(t *testing.T) {
	out, err := UpdateFrontmatter([]byte(graphyConcept), map[string]string{"graphy.language": "python"})
	if err != nil {
		t.Fatalf("UpdateFrontmatter: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `language: "python"`) {
		t.Errorf("nested key not updated:\n%s", s)
	}
	if strings.Contains(s, `language: "go"`) {
		t.Errorf("old nested value survived:\n%s", s)
	}
	if strings.Count(s, "language:") != 1 {
		t.Errorf("nested key duplicated instead of updated:\n%s", s)
	}
}

func TestDeletePathPrunesEmptyParent(t *testing.T) {
	content := []byte(`---
id: "019680f0-7e00-7abc-8def-1234567890ab"
title: "T"
x:
  only_key: "v"
---

# T
`)
	out, err := UpdateFrontmatterRemove(content, nil, []string{"x.only_key"})
	if err != nil {
		t.Fatalf("UpdateFrontmatterRemove: %v", err)
	}
	if strings.Contains(string(out), "x:") {
		t.Errorf("emptied parent mapping should be pruned, got:\n%s", out)
	}
}

func TestUpdateFrontmatterUpdatesExistingTopLevelField(t *testing.T) {
	content := []byte("---\nid: \"x\"\ntitle: \"Old Title\"\nproject: \"old-proj\"\n---\n\n# Old Title\n")
	out, err := UpdateFrontmatter(content, map[string]string{"project": "new-proj"})
	if err != nil {
		t.Fatalf("UpdateFrontmatter: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `project: "new-proj"`) {
		t.Errorf("project not updated:\n%s", s)
	}
	if strings.Contains(s, "old-proj") {
		t.Errorf("old value survived:\n%s", s)
	}
	if strings.Count(s, "project:") != 1 {
		t.Errorf("project duplicated instead of replaced:\n%s", s)
	}
}

func TestUpdateFrontmatterAddsNewTopLevelField(t *testing.T) {
	content := []byte("---\nid: \"x\"\ntitle: \"Test\"\n---\n\n# Test\n")
	out, err := UpdateFrontmatter(content, map[string]string{"project": "my-proj"})
	if err != nil {
		t.Fatalf("UpdateFrontmatter: %v", err)
	}
	if !strings.Contains(string(out), `project: "my-proj"`) {
		t.Errorf("project not added:\n%s", out)
	}
}

func TestUpdateFrontmatterRemoveRemovesTopLevelField(t *testing.T) {
	content := []byte("---\nid: \"x\"\ntitle: \"Test\"\nstale_after: \"2026-02-01\"\nstatus: \"draft\"\n---\n\n# Test\n")
	out, err := UpdateFrontmatterRemove(content, map[string]string{"status": "stable"}, []string{"stale_after"})
	if err != nil {
		t.Fatalf("UpdateFrontmatterRemove: %v", err)
	}
	s := string(out)
	if strings.Contains(s, "stale_after:") {
		t.Errorf("stale_after should be gone:\n%s", s)
	}
	if !strings.Contains(s, `status: "stable"`) {
		t.Errorf("status should be updated:\n%s", s)
	}
}

func TestAddFrontmatterTagsAppendsToExistingList(t *testing.T) {
	content := []byte("---\nid: \"x\"\ntitle: \"T\"\ntags:\n  - \"golang\"\ncreated: \"2026-02-01T10:00:00Z\"\n---\n\n# T\n")
	out, err := AddFrontmatterTags(content, []string{"api", "security"})
	if err != nil {
		t.Fatalf("AddFrontmatterTags: %v", err)
	}
	s := string(out)
	for _, want := range []string{`- "golang"`, `- "api"`, `- "security"`, "# T", "created:"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

// "tags: []" must become a block list rather than gaining a second tags key.
func TestAddFrontmatterTagsReplacesEmptyInlineArray(t *testing.T) {
	content := []byte("---\nid: \"x\"\ntitle: \"T\"\ntags: []\ncreated: \"2026-02-01T10:00:00Z\"\n---\n\n# T\n")
	out, err := AddFrontmatterTags(content, []string{"newtag"})
	if err != nil {
		t.Fatalf("AddFrontmatterTags: %v", err)
	}
	s := string(out)
	if strings.Contains(s, "tags: []") {
		t.Errorf("inline empty array should be replaced:\n%s", s)
	}
	if !strings.Contains(s, "tags:\n  - \"newtag\"") {
		t.Errorf("expected a block list, got:\n%s", s)
	}
	if strings.Count(s, "tags:") != 1 {
		t.Errorf("tags key duplicated:\n%s", s)
	}
	if !strings.Contains(s, "created:") {
		t.Errorf("created lost:\n%s", s)
	}
}

func TestAddFrontmatterTagsCreatesMissingTagsField(t *testing.T) {
	content := []byte("---\nid: \"x\"\ntitle: \"T\"\ncreated: \"2026-02-01T10:00:00Z\"\n---\n\n# T\n")
	out, err := AddFrontmatterTags(content, []string{"first-tag"})
	if err != nil {
		t.Fatalf("AddFrontmatterTags: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "tags:") || !strings.Contains(s, `- "first-tag"`) {
		t.Errorf("tags field not created:\n%s", s)
	}
}

// "tags:" with no value parses as null, not as a sequence.
func TestAddFrontmatterTagsHandlesNullTags(t *testing.T) {
	content := []byte("---\nid: \"x\"\ntitle: \"T\"\ntags:\ncreated: \"2026-02-01T10:00:00Z\"\n---\n\n# T\n")
	out, err := AddFrontmatterTags(content, []string{"t"})
	if err != nil {
		t.Fatalf("AddFrontmatterTags: %v", err)
	}
	if !strings.Contains(string(out), `- "t"`) {
		t.Errorf("tag not added to a null tags field:\n%s", out)
	}
}

func TestAddFrontmatterTagsIsIdempotent(t *testing.T) {
	once, err := AddFrontmatterTags([]byte(graphyConcept), []string{"api"})
	if err != nil {
		t.Fatalf("AddFrontmatterTags: %v", err)
	}
	twice, err := AddFrontmatterTags(once, []string{"api"})
	if err != nil {
		t.Fatalf("AddFrontmatterTags: %v", err)
	}
	if string(once) != string(twice) {
		t.Errorf("re-adding a tag changed the file:\n%s", twice)
	}
	if n := strings.Count(string(twice), `- "api"`); n != 1 {
		t.Errorf("expected 1 api tag, got %d", n)
	}
}

func TestAddFrontmatterTagsKeepsBodyByteIdentical(t *testing.T) {
	const body = "\n# ParseFrontmatter\n\nBody text.\n\n<!-- graphy:node go:function:concept.go#ParseFrontmatter -->\n"
	out, err := AddFrontmatterTags([]byte(graphyConcept), []string{"new"})
	if err != nil {
		t.Fatalf("AddFrontmatterTags: %v", err)
	}
	if !strings.HasSuffix(string(out), body) {
		t.Errorf("body was altered, got:\n%q", string(out))
	}
}

// A frontmatter line longer than bufio.Scanner's 64KB default used to stop the
// scan silently, and the truncated block was written back over a good file.
func TestLongFrontmatterLineSurvives(t *testing.T) {
	long := strings.Repeat("x", 100_000)
	content := []byte("---\nid: \"019680f0-7e00-7abc-8def-1234567890ab\"\ntitle: \"T\"\ndescription: \"" + long + "\"\ntags: []\n---\n\n# T\n")
	out, err := UpdateFrontmatter(content, map[string]string{"project": "p"})
	if err != nil {
		t.Fatalf("UpdateFrontmatter: %v", err)
	}
	var got map[string]any
	fm, _, _, err := splitFrontmatter(out)
	if err != nil {
		t.Fatalf("splitFrontmatter: %v", err)
	}
	if err := yaml.Unmarshal(fm, &got); err != nil {
		t.Fatalf("result is not valid YAML: %v", err)
	}
	if got["description"] != long {
		t.Errorf("long value truncated: got %d chars, want %d", len(got["description"].(string)), len(long))
	}
	if got["title"] != "T" || got["project"] != "p" {
		t.Errorf("fields lost after long line: %v", got)
	}
}

func TestNoFrontmatterIsAnError(t *testing.T) {
	for _, tc := range []struct{ name, content string }{
		{"no delimiter", "# Just a heading\n"},
		{"unterminated", "---\nid: \"x\"\n\n# Body\n"},
		{"not a mapping", "---\n- a\n- b\n---\n\n# Body\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := UpdateFrontmatter([]byte(tc.content), nil); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

// A body line of "---" must not be mistaken for the closing delimiter, and a
// value containing "\n---" must not end the block early.
func TestSplitFrontmatterRequiresDelimiterOnItsOwnLine(t *testing.T) {
	content := []byte("---\nid: \"x\"\ndescription: |\n  before\n  ---not-a-delimiter\n  after\n---\n\n# Body\n\n---\n\nAfter a rule.\n")
	fm, closing, rest, err := splitFrontmatter(content)
	if err != nil {
		t.Fatalf("splitFrontmatter: %v", err)
	}
	if !strings.Contains(string(fm), "---not-a-delimiter") {
		t.Errorf("frontmatter cut short: %q", fm)
	}
	if string(closing) != "---" {
		t.Errorf("closing = %q", closing)
	}
	if !strings.Contains(string(rest), "After a rule.") {
		t.Errorf("body lost the horizontal rule section: %q", rest)
	}
}

func TestSplitFrontmatterHandlesCRLF(t *testing.T) {
	content := []byte("---\r\nid: \"x\"\r\ntitle: \"T\"\r\n---\r\n\r\n# Body\r\n")
	fm, closing, rest, err := splitFrontmatter(content)
	if err != nil {
		t.Fatalf("splitFrontmatter: %v", err)
	}
	if !strings.Contains(string(fm), `id: "x"`) {
		t.Errorf("fm = %q", fm)
	}
	if string(closing) != "---\r" {
		t.Errorf("closing should keep its CR, got %q", closing)
	}
	if !strings.Contains(string(rest), "# Body") {
		t.Errorf("rest = %q", rest)
	}
}

func TestUpdateFrontmatterAppendsDeterministically(t *testing.T) {
	content := []byte("---\nid: \"x\"\ntitle: \"T\"\n---\n\n# T\n")
	updates := map[string]string{"alpha": "1", "beta": "2", "gamma": "3", "delta": "4", "epsilon": "5"}
	var prev string
	for i := 0; i < 8; i++ {
		u := map[string]string{"timestamp": "t"}
		for k, v := range updates {
			u[k] = v
		}
		out, err := UpdateFrontmatter(content, u)
		if err != nil {
			t.Fatal(err)
		}
		if i > 0 && string(out) != prev {
			t.Fatalf("appended field order is not deterministic:\n%s\n---\n%s", prev, out)
		}
		prev = string(out)
	}
}

func TestTouchTimestampLeavesExplicitValue(t *testing.T) {
	got := TouchTimestamp(map[string]string{"timestamp": "explicit"})
	if got["timestamp"] != "explicit" {
		t.Errorf("timestamp = %q, want explicit", got["timestamp"])
	}
	if ts := TouchTimestamp(nil)["timestamp"]; ts == "" {
		t.Error("TouchTimestamp(nil) should stamp a timestamp")
	}
}

// generated.at and timestamp record the same event, and ValidateBusinessRules
// rejects a disagreement — so one edit must move both.
func TestUpdateFrontmatterKeepsGeneratedAtInSync(t *testing.T) {
	content := []byte(`---
id: "019680f0-7e00-7abc-8def-1234567890ab"
title: "T"
type: "note"
tags: []
created: "2026-07-01T10:00:00Z"
timestamp: "2026-07-01T10:00:00Z"
generated:
  by: "human:ryan"
  at: "2026-07-01T10:00:00Z"
---

# T
`)
	out, err := UpdateFrontmatter(content, map[string]string{"timestamp": "2026-07-25T12:00:00Z"})
	if err != nil {
		t.Fatalf("UpdateFrontmatter: %v", err)
	}
	s := string(out)
	if strings.Count(s, `"2026-07-25T12:00:00Z"`) != 2 {
		t.Errorf("timestamp and generated.at should both advance:\n%s", s)
	}
	if strings.Contains(s, `at: "2026-07-01T10:00:00Z"`) {
		t.Errorf("generated.at was left behind:\n%s", s)
	}
	if !strings.Contains(s, `by: "human:ryan"`) {
		t.Errorf("generated.by must not be disturbed:\n%s", s)
	}
}

// Without an actor there is nothing truthful to put in generated.by, so no
// generated block is invented.
func TestUpdateFrontmatterDoesNotInventGeneratedBlock(t *testing.T) {
	out, err := UpdateFrontmatter([]byte(graphyConcept), map[string]string{"timestamp": "2026-07-25T12:00:00Z"})
	if err != nil {
		t.Fatalf("UpdateFrontmatter: %v", err)
	}
	if strings.Contains(string(out), "generated:") {
		t.Errorf("a generated block should not be created:\n%s", out)
	}
}
