package templates

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"
	"time"
)

//go:embed meeting.md
var meetingTemplate string

//go:embed book-review.md
var bookReviewTemplate string

//go:embed snippet.md
var snippetTemplate string

//go:embed project-idea.md
var projectIdeaTemplate string

//go:embed user-story.md
var userStoryTemplate string

//go:embed feature.md
var featureTemplate string

//go:embed daily.md
var dailyTemplate string

//go:embed issue.md
var issueTemplate string

// Template represents a note template
type Template struct {
	Name        string
	Description string
	Kind        string // structured kind: "note" (default), "daily-note", "issue"
	Tags        []string
	Body        string
}

// TemplateData holds the data for template rendering
type TemplateData struct {
	ID      string
	Title   string
	Date    string
	Project string
}

// BuiltinTemplates returns all built-in templates
var BuiltinTemplates = map[string]*Template{
	"meeting": {
		Name:        "meeting",
		Description: "Meeting notes with attendees and action items",
		Tags:        []string{"meeting"},
		Body:        meetingTemplate,
	},
	"book-review": {
		Name:        "book-review",
		Description: "Book review with rating and key takeaways",
		Tags:        []string{"book", "review"},
		Body:        bookReviewTemplate,
	},
	"snippet": {
		Name:        "snippet",
		Description: "Code snippet with context and explanation",
		Tags:        []string{"code", "snippet"},
		Body:        snippetTemplate,
	},
	"project-idea": {
		Name:        "project-idea",
		Description: "Project idea with goals and next steps",
		Tags:        []string{"idea", "project"},
		Body:        projectIdeaTemplate,
	},
	"user-story": {
		Name:        "user-story",
		Description: "User story in standard format with acceptance criteria",
		Tags:        []string{"user-story", "requirements"},
		Body:        userStoryTemplate,
	},
	"feature": {
		Name:        "feature",
		Description: "Feature specification with requirements and design notes",
		Tags:        []string{"feature", "spec"},
		Body:        featureTemplate,
	},
	"daily": {
		Name:        "daily",
		Description: "Daily note for capturing thoughts, tasks, and reflections",
		Kind:        "daily-note",
		Tags:        []string{"daily"},
		Body:        dailyTemplate,
	},
	"issue": {
		Name:        "issue",
		Description: "Issue tracking like GitHub (bug, enhancement, question)",
		Kind:        "issue",
		Tags:        []string{"issue"},
		Body:        issueTemplate,
	},
}

// Get returns a template by name
func Get(name string) (*Template, error) {
	t, ok := BuiltinTemplates[name]
	if !ok {
		return nil, fmt.Errorf("template not found: %s", name)
	}
	return t, nil
}

// List returns all available template names
func List() []string {
	names := make([]string, 0, len(BuiltinTemplates))
	for name := range BuiltinTemplates {
		names = append(names, name)
	}
	return names
}

// Render renders a template with the given data
func (t *Template) Render(data TemplateData) (string, error) {
	tmpl, err := template.New(t.Name).Parse(t.Body)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// GenerateFrontmatter generates YAML frontmatter for a template
func (t *Template) GenerateFrontmatter(id, title, project string, extraTags []string) string {
	now := time.Now()

	// Merge template tags with extra tags
	allTags := make([]string, 0, len(t.Tags)+len(extraTags))
	allTags = append(allTags, t.Tags...)
	for _, tag := range extraTags {
		// Avoid duplicates
		found := false
		for _, existing := range allTags {
			if existing == tag {
				found = true
				break
			}
		}
		if !found {
			allTags = append(allTags, tag)
		}
	}

	kind := t.Kind
	if kind == "" {
		kind = "note"
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.WriteString(fmt.Sprintf("id: %q\n", id))
	buf.WriteString(fmt.Sprintf("title: %q\n", title))

	// OKF requires a non-empty `type` on every concept. Default it to the
	// structured kind; callers may override with a descriptive value later.
	buf.WriteString(fmt.Sprintf("type: %q\n", kind))

	// Write the structured `kind` only when it differs from the default,
	// to keep plain-note frontmatter clean.
	if kind != "note" {
		buf.WriteString(fmt.Sprintf("kind: %q\n", kind))
	}

	if project != "" {
		buf.WriteString(fmt.Sprintf("project: %q\n", project))
	}

	buf.WriteString("tags:\n")
	for _, tag := range allTags {
		buf.WriteString(fmt.Sprintf("  - %q\n", tag))
	}

	stamp := now.Format(time.RFC3339)
	buf.WriteString(fmt.Sprintf("created: %q\n", stamp))
	buf.WriteString(fmt.Sprintf("timestamp: %q\n", stamp))
	buf.WriteString("---\n\n")

	return buf.String()
}

// GenerateNote generates a complete note from a template
func (t *Template) GenerateNote(id, title, project string, extraTags []string) (string, error) {
	frontmatter := t.GenerateFrontmatter(id, title, project, extraTags)

	data := TemplateData{
		ID:      id,
		Title:   title,
		Date:    time.Now().Format("2006-01-02"),
		Project: project,
	}

	body, err := t.Render(data)
	if err != nil {
		return "", err
	}

	return frontmatter + body, nil
}
