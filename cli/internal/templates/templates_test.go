package templates

import (
	"strings"
	"testing"
)

func TestBuiltinTemplates(t *testing.T) {
	expectedTemplates := []string{"meeting", "book-review", "snippet", "project-idea", "user-story", "feature", "daily", "issue"}

	for _, name := range expectedTemplates {
		tmpl, ok := BuiltinTemplates[name]
		if !ok {
			t.Errorf("Expected template %q not found", name)
			continue
		}

		if tmpl.Name != name {
			t.Errorf("Template name mismatch: expected %q, got %q", name, tmpl.Name)
		}

		if tmpl.Description == "" {
			t.Errorf("Template %q has empty description", name)
		}

		if len(tmpl.Tags) == 0 {
			t.Errorf("Template %q has no tags", name)
		}

		if tmpl.Body == "" {
			t.Errorf("Template %q has empty body", name)
		}
	}
}

func TestGet(t *testing.T) {
	tests := []struct {
		name      string
		expectErr bool
	}{
		{"meeting", false},
		{"book-review", false},
		{"snippet", false},
		{"project-idea", false},
		{"user-story", false},
		{"feature", false},
		{"daily", false},
		{"todo", true}, // removed with the task-tracking feature
		{"issue", false},
		{"nonexistent", true},
		{"", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := Get(tt.name)
			if tt.expectErr {
				if err == nil {
					t.Errorf("Expected error for template %q, got nil", tt.name)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for template %q: %v", tt.name, err)
				}
				if tmpl == nil {
					t.Errorf("Expected template for %q, got nil", tt.name)
				}
			}
		})
	}
}

func TestList(t *testing.T) {
	names := List()

	if len(names) != 8 {
		t.Errorf("Expected 8 templates, got %d", len(names))
	}

	expected := map[string]bool{
		"meeting":      true,
		"book-review":  true,
		"snippet":      true,
		"project-idea": true,
		"user-story":   true,
		"feature":      true,
		"daily":        true,
		"issue":        true,
	}

	for _, name := range names {
		if !expected[name] {
			t.Errorf("Unexpected template in list: %q", name)
		}
	}
}

func TestTemplateRender(t *testing.T) {
	tmpl, _ := Get("meeting")

	data := TemplateData{
		ID:      "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		Title:   "Test Meeting",
		Date:    "2026-02-13",
		Project: "test-project",
	}

	result, err := tmpl.Render(data)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	if !strings.Contains(result, "Test Meeting") {
		t.Errorf("Rendered template should contain title, got: %s", result)
	}

	if !strings.Contains(result, "2026-02-13") {
		t.Errorf("Rendered template should contain date, got: %s", result)
	}
}

func TestGenerateFrontmatter(t *testing.T) {
	tmpl, _ := Get("meeting")

	frontmatter := tmpl.GenerateFrontmatter("20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", "Test Meeting", "test-project", []string{"extra-tag"})

	if !strings.Contains(frontmatter, `id: "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"`) {
		t.Errorf("Frontmatter should contain ID")
	}

	if !strings.Contains(frontmatter, `title: "Test Meeting"`) {
		t.Errorf("Frontmatter should contain title")
	}

	if !strings.Contains(frontmatter, `project: "test-project"`) {
		t.Errorf("Frontmatter should contain project")
	}

	if strings.Contains(frontmatter, "category:") {
		t.Errorf("Frontmatter should not contain category")
	}

	if !strings.Contains(frontmatter, `"meeting"`) {
		t.Errorf("Frontmatter should contain template tag 'meeting'")
	}

	if !strings.Contains(frontmatter, `"extra-tag"`) {
		t.Errorf("Frontmatter should contain extra tag")
	}
}

func TestGenerateFrontmatter_NoDuplicateTags(t *testing.T) {
	tmpl, _ := Get("meeting")

	// "meeting" is already a template tag, should not be duplicated
	frontmatter := tmpl.GenerateFrontmatter("20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", "Test", "proj", []string{"meeting", "extra"})

	// Count occurrences of "meeting" in tags section
	count := strings.Count(frontmatter, `"meeting"`)
	if count != 1 {
		t.Errorf("Tag 'meeting' should appear exactly once, but appeared %d times", count)
	}
}

func TestGenerateNote(t *testing.T) {
	tmpl, _ := Get("user-story")

	note, err := tmpl.GenerateNote("20260213110000-ffffffff-1111-2222-3333-444444444444", "Login Feature", "my-project", nil)
	if err != nil {
		t.Fatalf("GenerateNote failed: %v", err)
	}

	// Should have frontmatter
	if !strings.HasPrefix(note, "---\n") {
		t.Errorf("Note should start with frontmatter delimiter")
	}

	// Should have ID
	if !strings.Contains(note, `id: "20260213110000-ffffffff-1111-2222-3333-444444444444"`) {
		t.Errorf("Note should contain ID in frontmatter")
	}

	// Should have title in frontmatter
	if !strings.Contains(note, `title: "Login Feature"`) {
		t.Errorf("Note should contain title in frontmatter")
	}

	// Should have user-story tag
	if !strings.Contains(note, `"user-story"`) {
		t.Errorf("Note should contain user-story tag")
	}

	// Should have body content
	if !strings.Contains(note, "User Story") {
		t.Errorf("Note should contain user story body content")
	}

	// Should have acceptance criteria section
	if !strings.Contains(note, "Acceptance Criteria") {
		t.Errorf("Note should contain Acceptance Criteria section")
	}
}

func TestGenerateFrontmatter_OKFFields(t *testing.T) {
	id := "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

	// A plain note: OKF `type` is always written and defaults to "note";
	// the structured `kind` is omitted (note is the default); timestamp present.
	note, _ := Get("meeting")
	fm := note.GenerateFrontmatter(id, "M", "p", nil)
	if !strings.Contains(fm, `type: "note"`) {
		t.Error("note should always write OKF type")
	}
	if strings.Contains(fm, "kind:") {
		t.Error("note frontmatter should omit the default kind")
	}
	if !strings.Contains(fm, "timestamp:") {
		t.Error("frontmatter should include timestamp")
	}

	// A non-default kind: both `type` and `kind` are written.
	issue, _ := Get("issue")
	fm = issue.GenerateFrontmatter(id, "T", "p", nil)
	if !strings.Contains(fm, `type: "issue"`) || !strings.Contains(fm, `kind: "issue"`) {
		t.Errorf("issue should write both type and kind:\n%s", fm)
	}
	if !strings.Contains(fm, "timestamp:") {
		t.Error("issue frontmatter should include timestamp")
	}
}
