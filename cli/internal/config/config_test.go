package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConceptSchema(t *testing.T) {
	schema, err := LoadConceptSchema()
	if err != nil {
		t.Fatalf("LoadConceptSchema() error = %v", err)
	}

	if schema.Err() != nil {
		t.Fatalf("schema has error: %v", schema.Err())
	}
}

func TestDiscover(t *testing.T) {
	project := t.TempDir()
	wikiDir := filepath.Join(project, WikiDirName)
	if err := os.MkdirAll(wikiDir, 0755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(project, "src", "internal", "deep")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}

	// Found at the start directory itself.
	cfg, err := Discover(project)
	if err != nil {
		t.Fatalf("Discover(project) error = %v", err)
	}
	if cfg.RootPath != wikiDir {
		t.Errorf("RootPath = %q, want %q", cfg.RootPath, wikiDir)
	}
	if cfg.IndexPath != filepath.Join(wikiDir, IndexDirName) {
		t.Errorf("IndexPath = %q, want %q", cfg.IndexPath, filepath.Join(wikiDir, IndexDirName))
	}
	if cfg.GraphPath != GraphDBName {
		t.Errorf("GraphPath = %q, want %q", cfg.GraphPath, GraphDBName)
	}
	if cfg.TodosPath != TodosDirName {
		t.Errorf("TodosPath = %q, want %q", cfg.TodosPath, TodosDirName)
	}

	// Found from a nested subdirectory (walks upward).
	cfg, err = Discover(nested)
	if err != nil {
		t.Fatalf("Discover(nested) error = %v", err)
	}
	if cfg.RootPath != wikiDir {
		t.Errorf("RootPath from nested dir = %q, want %q", cfg.RootPath, wikiDir)
	}
}

func TestDiscover_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := Discover(dir)
	if err == nil {
		t.Fatal("Discover() expected error when no .okfw exists")
	}
	var noWiki *ErrNoWiki
	if !errors.As(err, &noWiki) {
		t.Fatalf("expected *ErrNoWiki, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "okfw init") {
		t.Errorf("error should advise running okfw init, got: %v", err)
	}
}

func TestDiscover_IgnoresOkfwFile(t *testing.T) {
	// A plain file named .okfw must not count as a wiki.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, WikiDirName), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(dir); err == nil {
		t.Error("Discover() should not treat a .okfw file as a wiki")
	}
}

func TestValidateConceptYAML_Valid(t *testing.T) {
	validYAML := `
id: "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
title: "Test Concept"
type: "note"
project: "test-project"
tags:
  - "test"
created: "2026-02-13T10:45:00Z"
timestamp: "2026-02-13T10:45:00Z"
`
	if err := ValidateConceptYAML([]byte(validYAML)); err != nil {
		t.Errorf("ValidateConceptYAML() error = %v", err)
	}
}

func TestValidateConceptYAML_ValidWithParent(t *testing.T) {
	validYAML := `
id: "20260213110000-ffffffff-1111-2222-3333-444444444444"
title: "Child Concept"
type: "note"
project: "test-project"
tags:
  - "child"
created: "2026-02-13T11:00:00Z"
timestamp: "2026-02-13T10:45:00Z"
parent: "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
`
	if err := ValidateConceptYAML([]byte(validYAML)); err != nil {
		t.Errorf("ValidateConceptYAML() error = %v", err)
	}
}

func TestValidateConceptYAML_ValidUUIDv7(t *testing.T) {
	validYAML := `
id: "019680f0-7e00-7abc-8def-1234567890ab"
title: "UUIDv7 Concept"
type: "note"
tags: []
created: "2026-04-10T12:00:00Z"
timestamp: "2026-02-13T10:45:00Z"
`
	if err := ValidateConceptYAML([]byte(validYAML)); err != nil {
		t.Errorf("ValidateConceptYAML() UUIDv7 should be valid, got error = %v", err)
	}
}

func TestValidateConceptYAML_InvalidID(t *testing.T) {
	invalidYAML := `
id: "12345"
title: "Test"
type: "note"
project: "test"
tags: []
created: "2026-02-13T10:45:00Z"
timestamp: "2026-02-13T10:45:00Z"
`
	if err := ValidateConceptYAML([]byte(invalidYAML)); err == nil {
		t.Error("ValidateConceptYAML() expected error for invalid ID, got nil")
	}
}

func TestValidateConceptYAML_EmptyTitle(t *testing.T) {
	invalidYAML := `
id: "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
title: ""
type: "note"
project: "test"
tags: []
created: "2026-02-13T10:45:00Z"
timestamp: "2026-02-13T10:45:00Z"
`
	if err := ValidateConceptYAML([]byte(invalidYAML)); err == nil {
		t.Error("ValidateConceptYAML() expected error for empty title, got nil")
	}
}

func TestValidateConceptYAML_WithoutProject(t *testing.T) {
	// Notes can omit project entirely
	validYAML := `
id: "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
title: "Quick Idea"
type: "note"
tags: []
created: "2026-02-13T10:45:00Z"
timestamp: "2026-02-13T10:45:00Z"
`
	if err := ValidateConceptYAML([]byte(validYAML)); err != nil {
		t.Errorf("ValidateConceptYAML() note without project should be valid, got error = %v", err)
	}
}

func TestValidateConceptYAML_LegacyCategoryTolerated(t *testing.T) {
	// Notes written before category removal may still carry the field; it is
	// tolerated as an ignored legacy key (OKF permissive consumption).
	legacyYAML := `
id: "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
title: "Test"
type: "note"
project: "test"
category: "untethered"
tags: []
created: "2026-02-13T10:45:00Z"
timestamp: "2026-02-13T10:45:00Z"
`
	if err := ValidateConceptYAML([]byte(legacyYAML)); err != nil {
		t.Errorf("ValidateConceptYAML() legacy category should be tolerated, got error = %v", err)
	}
}

func TestValidateConceptYAML_MissingCreated(t *testing.T) {
	invalidYAML := `
id: "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
title: "Test"
type: "note"
project: "test"
tags: []
`
	if err := ValidateConceptYAML([]byte(invalidYAML)); err == nil {
		t.Error("ValidateConceptYAML() expected error for missing created, got nil")
	}
}

func TestExtractFrontmatter(t *testing.T) {
	content := []byte(`---
id: "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
title: "Test"
type: "note"
---

# Content here
`)
	frontmatter, err := ExtractFrontmatter(content)
	if err != nil {
		t.Fatalf("ExtractFrontmatter() error = %v", err)
	}

	expected := `id: "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
title: "Test"
type: "note"`
	if string(frontmatter) != expected {
		t.Errorf("ExtractFrontmatter() = %q, want %q", string(frontmatter), expected)
	}
}

func TestExtractFrontmatter_NoOpeningDelimiter(t *testing.T) {
	content := []byte(`id: "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
title: "Test"
type: "note"
---
`)
	_, err := ExtractFrontmatter(content)
	if err == nil {
		t.Error("ExtractFrontmatter() expected error for missing opening delimiter")
	}
}

func TestExtractFrontmatter_NoClosingDelimiter(t *testing.T) {
	content := []byte(`---
id: "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
title: "Test"
type: "note"
`)
	_, err := ExtractFrontmatter(content)
	if err == nil {
		t.Error("ExtractFrontmatter() expected error for missing closing delimiter")
	}
}

func TestParseFrontmatter(t *testing.T) {
	content := []byte(`---
id: "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
title: "Test Concept"
type: "note"
project: "test-project"
tags:
  - "test"
created: "2026-02-13T10:45:00Z"
timestamp: "2026-02-13T10:45:00Z"
---

# Content
`)
	z, err := ParseFrontmatter(content)
	if err != nil {
		t.Fatalf("ParseFrontmatter() error = %v", err)
	}

	if z.ID != "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
		t.Errorf("ID = %q, want %q", z.ID, "20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	}

	if z.Title != "Test Concept" {
		t.Errorf("Title = %q, want %q", z.Title, "Test Concept")
	}

	if z.Project != "test-project" {
		t.Errorf("Project = %q, want %q", z.Project, "test-project")
	}

	if len(z.Tags) != 1 || z.Tags[0] != "test" {
		t.Errorf("Tags = %v, want [test]", z.Tags)
	}
}

func TestParseAndValidate_ValidFile(t *testing.T) {
	// Read the valid test file
	content, err := os.ReadFile("../../testdata/valid_concept.md")
	if err != nil {
		t.Skipf("testdata file not found: %v", err)
	}

	z, err := ParseAndValidate(content)
	if err != nil {
		t.Fatalf("ParseAndValidate() error = %v", err)
	}

	if z.ID != "019680f0-7e00-7abc-8def-1234567890ab" {
		t.Errorf("ID = %q, want %q", z.ID, "019680f0-7e00-7abc-8def-1234567890ab")
	}
}

func TestParseAndValidate_ValidFileWithParent(t *testing.T) {
	content, err := os.ReadFile("../../testdata/valid_concept_with_parent.md")
	if err != nil {
		t.Skipf("testdata file not found: %v", err)
	}

	z, err := ParseAndValidate(content)
	if err != nil {
		t.Fatalf("ParseAndValidate() error = %v", err)
	}

	if z.Parent != "019680f0-7e00-7abc-8def-1234567890ab" {
		t.Errorf("Parent = %q, want %q", z.Parent, "019680f0-7e00-7abc-8def-1234567890ab")
	}
}

func TestParseAndValidate_InvalidFile(t *testing.T) {
	content, err := os.ReadFile("../../testdata/invalid_concept.md")
	if err != nil {
		t.Skipf("testdata file not found: %v", err)
	}

	_, err = ParseAndValidate(content)
	if err == nil {
		t.Error("ParseAndValidate() expected error for invalid file, got nil")
	}
}

func TestParseAndValidate_NoProject(t *testing.T) {
	content, err := os.ReadFile("../../testdata/valid_concept_no_project.md")
	if err != nil {
		t.Skipf("testdata file not found: %v", err)
	}

	z, err := ParseAndValidate(content)
	if err != nil {
		t.Fatalf("ParseAndValidate() error = %v", err)
	}

	if z.Project != "" {
		t.Errorf("Project = %q, want empty string", z.Project)
	}
}

func TestExtractBody(t *testing.T) {
	cases := map[string]struct {
		content string
		want    string
	}{
		"lf": {
			content: "---\ntitle: \"T\"\n---\n\n# Body\n",
			want:    "\n# Body\n",
		},
		"crlf": {
			content: "---\r\ntitle: \"T\"\r\n---\r\n\r\n# Body\r\n",
			want:    "\r\n# Body\r\n",
		},
		"no frontmatter": {
			content: "# Just a heading\n",
			want:    "# Just a heading\n",
		},
		"unclosed frontmatter": {
			content: "---\ntitle: \"T\"\n",
			want:    "",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := ExtractBody([]byte(c.content)); got != c.want {
				t.Errorf("ExtractBody(%q) = %q, want %q", c.content, got, c.want)
			}
		})
	}
}
