package main

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGenerateBasicNote_NoProject(t *testing.T) {
	note := generateBasicNote("20260201100000-abc", "My Idea", "", nil)

	if !strings.Contains(note, `type: "note"`) {
		t.Error("Expected type: note")
	}
	if !strings.Contains(note, `title: "My Idea"`) {
		t.Error("Expected title")
	}
	if strings.Contains(note, "project:") {
		t.Error("Note without project should not have project field")
	}
	if !strings.Contains(note, "tags: []") {
		t.Error("Expected empty tags as tags: []")
	}
	if !strings.Contains(note, "# My Idea") {
		t.Error("Expected markdown heading")
	}
}

func TestGenerateBasicNote_WithProject(t *testing.T) {
	note := generateBasicNote("20260201100000-abc", "Project Note", "backend", nil)

	if !strings.Contains(note, `project: "backend"`) {
		t.Error("Expected project: backend")
	}
}

func TestGenerateBasicNote_WithTags(t *testing.T) {
	note := generateBasicNote("20260201100000-abc", "Tagged Note", "", []string{"golang", "api"})

	if !strings.Contains(note, `- "golang"`) {
		t.Error("Expected golang tag")
	}
	if !strings.Contains(note, `- "api"`) {
		t.Error("Expected api tag")
	}
	if strings.Contains(note, "tags: []") {
		t.Error("Should not have empty tags when tags provided")
	}
}

func TestGenerateBasicNote_EmptyTags(t *testing.T) {
	note := generateBasicNote("20260201100000-abc", "No Tags", "", []string{})

	if !strings.Contains(note, "tags: []") {
		t.Error("Expected tags: [] for empty tag slice")
	}
}

func TestGenerateBasicNote_NilTags(t *testing.T) {
	note := generateBasicNote("20260201100000-abc", "Nil Tags", "", nil)

	if !strings.Contains(note, "tags: []") {
		t.Error("Expected tags: [] for nil tag slice")
	}
}

func TestGenerateBasicNote_HasFrontmatter(t *testing.T) {
	note := generateBasicNote("20260201100000-abc", "Test", "", nil)

	if !strings.HasPrefix(note, "---\n") {
		t.Error("Expected frontmatter to start with ---")
	}
	// Count --- delimiters (should be exactly 2)
	count := strings.Count(note, "---\n")
	if count != 2 {
		t.Errorf("Expected 2 frontmatter delimiters, got %d", count)
	}
}

func TestAddFrontmatterTags_AppendsToExisting(t *testing.T) {
	content := []byte(`---
id: "20260201100000-abc"
title: "Test Note"
tags:
  - "golang"
created: "2026-02-01T10:00:00Z"
---

# Test Note
`)
	result, err := addFrontmatterTags(content, []string{"api", "security"})
	if err != nil {
		t.Fatalf("addFrontmatterTags failed: %v", err)
	}

	s := string(result)
	if !strings.Contains(s, `- "golang"`) {
		t.Error("Expected existing golang tag to be preserved")
	}
	if !strings.Contains(s, `- "api"`) {
		t.Error("Expected api tag to be added")
	}
	if !strings.Contains(s, `- "security"`) {
		t.Error("Expected security tag to be added")
	}
	if !strings.Contains(s, "# Test Note") {
		t.Error("Expected body content to be preserved")
	}
}

func TestAddFrontmatterTags_EmptyInlineArray(t *testing.T) {
	content := []byte(`---
id: "20260201100000-abc"
title: "Test Note"
tags: []
created: "2026-02-01T10:00:00Z"
---

# Test Note
`)
	result, err := addFrontmatterTags(content, []string{"newtag"})
	if err != nil {
		t.Fatalf("addFrontmatterTags failed: %v", err)
	}

	s := string(result)
	if !strings.Contains(s, `- "newtag"`) {
		t.Error("Expected newtag to be added")
	}
	if strings.Contains(s, "tags: []") {
		t.Error("Should not keep inline empty array when adding tags")
	}
	// Verify the YAML is valid by checking tags: followed by list items
	if !strings.Contains(s, "tags:\n  - \"newtag\"") {
		t.Errorf("Expected tags:\\n  - \"newtag\" but got:\n%s", s)
	}
	// Verify created field is preserved and not broken
	if !strings.Contains(s, "created:") {
		t.Error("Expected created field to be preserved")
	}
}

func TestAddFrontmatterTags_NoTagsField(t *testing.T) {
	content := []byte(`---
id: "20260201100000-abc"
title: "Test Note"
created: "2026-02-01T10:00:00Z"
---

# Test Note
`)
	result, err := addFrontmatterTags(content, []string{"first-tag"})
	if err != nil {
		t.Fatalf("addFrontmatterTags failed: %v", err)
	}

	s := string(result)
	if !strings.Contains(s, "tags:") {
		t.Error("Expected tags: field to be added")
	}
	if !strings.Contains(s, `- "first-tag"`) {
		t.Error("Expected first-tag to be added")
	}
}

func TestAddFrontmatterTags_NoFrontmatter(t *testing.T) {
	content := []byte("# Just a heading\n\nSome content.\n")
	_, err := addFrontmatterTags(content, []string{"tag"})
	if err == nil {
		t.Error("Expected error for content without frontmatter")
	}
}

func TestUpdateFrontmatter_UpdatesExistingField(t *testing.T) {
	content := []byte(`---
id: "20260201100000-abc"
title: "Old Title"
project: "old-proj"
---

# Old Title
`)
	result, err := updateFrontmatter(content, map[string]string{
		"project": "new-proj",
	})
	if err != nil {
		t.Fatalf("updateFrontmatter failed: %v", err)
	}

	s := string(result)
	if !strings.Contains(s, `project: "new-proj"`) {
		t.Error("Expected project to be updated to new-proj")
	}
	if strings.Contains(s, `project: "old-proj"`) {
		t.Error("Old project value should be replaced")
	}
}

func TestUpdateFrontmatter_AddsNewField(t *testing.T) {
	content := []byte(`---
id: "20260201100000-abc"
title: "Test"
---

# Test
`)
	result, err := updateFrontmatter(content, map[string]string{
		"project": "my-proj",
	})
	if err != nil {
		t.Fatalf("updateFrontmatter failed: %v", err)
	}

	s := string(result)
	if !strings.Contains(s, `project: "my-proj"`) {
		t.Error("Expected project field to be added")
	}
}

func TestUpdateFrontmatterRemove_RemovesField(t *testing.T) {
	content := []byte(`---
id: "20260201100000-abc"
title: "Test"
completed: "2026-02-01"
status: "closed"
---

# Test
`)
	result, err := updateFrontmatterRemove(content, map[string]string{
		"status": "open",
	}, []string{"completed"})
	if err != nil {
		t.Fatalf("updateFrontmatterRemove failed: %v", err)
	}

	s := string(result)
	if !strings.Contains(s, `status: "open"`) {
		t.Error("Expected status to be updated to open")
	}
	if strings.Contains(s, "completed") {
		t.Error("Expected completed field to be removed")
	}
}

func TestNewV7WithTime(t *testing.T) {
	target := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	id := newV7WithTime(target)

	// Should be valid UUID
	u, err := uuid.Parse(id)
	if err != nil {
		t.Fatalf("newV7WithTime produced invalid UUID: %v", err)
	}

	// Should be version 7
	if u.Version() != 7 {
		t.Errorf("expected version 7, got %d", u.Version())
	}

	// Should be 36 chars
	if len(id) != 36 {
		t.Errorf("expected 36 chars, got %d", len(id))
	}

	// Round-trip: extract time back
	extracted, err := timeFromID(id)
	if err != nil {
		t.Fatalf("timeFromID failed: %v", err)
	}
	if extracted.Unix() != target.Unix() {
		t.Errorf("expected time %v, got %v", target, extracted)
	}
}

func TestTimeFromID_Legacy(t *testing.T) {
	id := "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	got, err := timeFromID(id)
	if err != nil {
		t.Fatalf("timeFromID failed: %v", err)
	}
	expected := time.Date(2026, 2, 13, 10, 45, 0, 0, time.UTC)
	if got.Unix() != expected.Unix() {
		t.Errorf("expected %v, got %v", expected, got)
	}
}

func TestTimeFromID_UUIDv7(t *testing.T) {
	id := uuid.Must(uuid.NewV7()).String()
	now := time.Now()
	got, err := timeFromID(id)
	if err != nil {
		t.Fatalf("timeFromID failed: %v", err)
	}
	// Should be within 1 second of now
	diff := now.Sub(got)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("extracted time %v differs from now %v by %v", got, now, diff)
	}
}

func TestTimeFromID_Invalid(t *testing.T) {
	_, err := timeFromID("not-a-valid-id")
	if err == nil {
		t.Error("expected error for invalid ID")
	}
}
