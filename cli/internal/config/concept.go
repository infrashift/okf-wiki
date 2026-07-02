package config

import (
	"bytes"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Concept represents a bundle note's frontmatter.
type Concept struct {
	ID          string    `yaml:"id"`
	Title       string    `yaml:"title"`
	Type        string    `yaml:"type"`                  // OKF required: free-form concept kind, always written
	Kind        string    `yaml:"kind,omitempty"`        // structured: "note" (default), "todo", "daily-note", "issue"
	Description string    `yaml:"description,omitempty"` // OKF recommended: one-sentence summary
	Resource    string    `yaml:"resource,omitempty"`    // OKF recommended: URI of the underlying asset
	Project     string    `yaml:"project,omitempty"`
	Tags        []string  `yaml:"tags"`
	Created     time.Time `yaml:"created"`
	Timestamp   time.Time `yaml:"timestamp"` // OKF recommended: last meaningful change
	Parent      string    `yaml:"parent,omitempty"`

	// Todo-specific fields (only used when Kind == "todo")
	Status    string `yaml:"status,omitempty"`    // "open", "in_progress", "closed"
	Due       string `yaml:"due,omitempty"`       // YYYY-MM-DD
	Completed string `yaml:"completed,omitempty"` // YYYY-MM-DD (set when closed)
	Priority  string `yaml:"priority,omitempty"`  // "high", "medium", "low"
}

// GetKind returns the structured concept kind, defaulting to "note".
// For backward compatibility with legacy notes that stored the kind in the
// `type` field (pre-OKF), it falls back to a recognized `type` value.
func (z *Concept) GetKind() string {
	if z.Kind != "" {
		return z.Kind
	}
	switch z.Type {
	case "note", "todo", "daily-note", "issue":
		return z.Type
	}
	return "note"
}

// IsTodo returns true if this concept is a todo item.
func (z *Concept) IsTodo() bool {
	return z.GetKind() == "todo"
}

// IsDailyNote returns true if this concept is a daily note.
func (z *Concept) IsDailyNote() bool {
	return z.GetKind() == "daily-note"
}

// ParseFrontmatter extracts and parses YAML frontmatter from markdown content.
// Frontmatter must be enclosed between "---" markers at the start of the file.
func ParseFrontmatter(content []byte) (*Concept, error) {
	frontmatter, err := ExtractFrontmatter(content)
	if err != nil {
		return nil, err
	}

	var z Concept
	if err := yaml.Unmarshal(frontmatter, &z); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	return &z, nil
}

// ExtractFrontmatter extracts raw YAML frontmatter bytes from markdown content.
func ExtractFrontmatter(content []byte) ([]byte, error) {
	// Check for opening delimiter
	if !bytes.HasPrefix(content, []byte("---")) {
		return nil, fmt.Errorf("frontmatter must start with ---")
	}

	// Find the closing delimiter
	rest := content[3:]
	// Skip newline after opening ---
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}

	endIdx := bytes.Index(rest, []byte("\n---"))
	if endIdx == -1 {
		// Try with \r\n
		endIdx = bytes.Index(rest, []byte("\r\n---"))
		if endIdx == -1 {
			return nil, fmt.Errorf("frontmatter closing --- not found")
		}
	}

	return rest[:endIdx], nil
}

// ExtractBody returns the markdown body following the frontmatter block.
// Content without a frontmatter block is returned unchanged; content whose
// frontmatter never closes yields an empty body. Handles both LF and CRLF
// line endings.
func ExtractBody(content []byte) string {
	if !bytes.HasPrefix(content, []byte("---")) {
		return string(content)
	}

	rest := content[3:]
	// Skip the line ending after the opening ---
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}

	endIdx := bytes.Index(rest, []byte("\n---"))
	if endIdx == -1 {
		return ""
	}

	body := rest[endIdx+4:]
	// Skip the line ending after the closing ---
	if len(body) > 1 && body[0] == '\r' && body[1] == '\n' {
		body = body[2:]
	} else if len(body) > 0 && body[0] == '\n' {
		body = body[1:]
	}

	return string(body)
}

// ValidateConcept validates a Concept struct against the CUE schema and business rules.
func ValidateConcept(z *Concept) error {
	// Convert to YAML and validate against CUE schema
	data, err := yaml.Marshal(z)
	if err != nil {
		return fmt.Errorf("failed to marshal concept: %w", err)
	}

	if err := ValidateConceptYAML(data); err != nil {
		return err
	}

	// Additional business rule validation for todos
	if z.IsTodo() {
		if err := validateTodoFields(z); err != nil {
			return err
		}
	}

	return nil
}

// validateTodoFields validates todo-specific business rules.
func validateTodoFields(z *Concept) error {
	// Status is required for todos
	if z.Status == "" {
		return fmt.Errorf("todo requires status field (open, in_progress, or closed)")
	}

	// Validate status values
	switch z.Status {
	case "open", "in_progress", "closed":
		// Valid
	default:
		return fmt.Errorf("invalid status '%s': must be open, in_progress, or closed", z.Status)
	}

	// Validate priority if provided
	if z.Priority != "" {
		switch z.Priority {
		case "high", "medium", "low":
			// Valid
		default:
			return fmt.Errorf("invalid priority '%s': must be high, medium, or low", z.Priority)
		}
	}

	return nil
}

// ParseAndValidate extracts frontmatter from content and validates it against the schema.
func ParseAndValidate(content []byte) (*Concept, error) {
	frontmatter, err := ExtractFrontmatter(content)
	if err != nil {
		return nil, err
	}

	// Validate raw YAML against schema
	if err := ValidateConceptYAML(frontmatter); err != nil {
		return nil, err
	}

	// Parse into struct
	var z Concept
	if err := yaml.Unmarshal(frontmatter, &z); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	return &z, nil
}
