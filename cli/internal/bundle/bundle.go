package bundle

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"okfw/cli/internal/config"
	"okfw/cli/internal/templates"
)

// Spec describes a concept to create.
type Spec struct {
	Title       string
	Project     string
	Dir         string // bundle subdirectory (--path)
	Tags        []string
	Description string
	Template    string
	Body        string // markdown appended after the generated content
}

// CreateConcept writes a new concept (optionally from a template) and returns
// its absolute path and id. The caller is responsible for AutoSync.
func CreateConcept(cfg *config.Config, spec Spec) (string, string, error) {
	id := uuid.Must(uuid.NewV7()).String()

	var content string
	if spec.Template != "" {
		tmpl, err := templates.Get(spec.Template)
		if err != nil {
			return "", "", fmt.Errorf("template error: %w", err)
		}
		content, err = tmpl.GenerateNote(id, spec.Title, spec.Project, spec.Tags)
		if err != nil {
			return "", "", fmt.Errorf("failed to generate note from template: %w", err)
		}
	} else {
		content = GenerateBasicNote(id, spec.Title, spec.Project, spec.Tags)
	}

	outputDir, err := ConceptDir(cfg, spec.Dir, spec.Project)
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create directory: %w", err)
	}

	if spec.Description != "" {
		updated, derr := UpdateFrontmatter([]byte(content), map[string]string{"description": spec.Description})
		if derr != nil {
			return "", "", fmt.Errorf("failed to set description: %w", derr)
		}
		content = string(updated)
	}

	if spec.Body != "" {
		content = strings.TrimRight(content, "\n") + "\n\n" + strings.TrimRight(spec.Body, "\n") + "\n"
	}

	if err := ValidateGenerated([]byte(content)); err != nil {
		return "", "", err
	}

	filePath := filepath.Join(outputDir, NoteFilename(outputDir, spec.Title, id))
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return "", "", fmt.Errorf("failed to write file: %w", err)
	}
	return filePath, id, nil
}

// GenerateBasicNote renders minimal, OKF-valid concept markdown.
func GenerateBasicNote(id, title, project string, tags []string) string {
	now := time.Now()

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.WriteString(fmt.Sprintf("id: %q\n", id))
	buf.WriteString(fmt.Sprintf("title: %q\n", title))
	buf.WriteString("type: \"note\"\n")

	if project != "" {
		buf.WriteString(fmt.Sprintf("project: %q\n", project))
	}

	buf.WriteString("tags:")
	if len(tags) > 0 {
		buf.WriteString("\n")
		for _, tag := range tags {
			buf.WriteString(fmt.Sprintf("  - %q\n", tag))
		}
	} else {
		buf.WriteString(" []\n")
	}

	stamp := now.Format(time.RFC3339)
	buf.WriteString(fmt.Sprintf("created: %q\n", stamp))
	buf.WriteString(fmt.Sprintf("timestamp: %q\n", stamp))
	buf.WriteString("---\n\n")
	buf.WriteString(fmt.Sprintf("# %s\n\n", title))

	return buf.String()
}

// ResolveConceptPath resolves a concept ID or file path to an absolute file path.
func ResolveConceptPath(cfg *config.Config, idOrPath string) (string, error) {
	// If it looks like a file path, use it directly
	if strings.HasSuffix(idOrPath, ".md") || strings.Contains(idOrPath, "/") {
		if _, err := os.Stat(idOrPath); err != nil {
			return "", fmt.Errorf("file not found: %s", idOrPath)
		}
		return filepath.Abs(idOrPath)
	}

	// Otherwise it is an ID: search the whole bundle for a concept whose
	// frontmatter id matches. Concepts may live in any directory, and
	// filenames are slug--shortid.md rather than <id>.md.
	var found string
	err := filepath.Walk(cfg.RootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if SkipWalkDir(cfg.RootPath, path, info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".md") || IsReservedBase(info.Name()) {
			return nil
		}
		// Fast path: a legacy <id>.md filename.
		if info.Name() == idOrPath+".md" {
			found = path
			return filepath.SkipAll
		}
		if config.ReadConceptID(path) == idOrPath {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && err != filepath.SkipAll {
		return "", fmt.Errorf("error searching for concept: %w", err)
	}

	if found != "" {
		return filepath.Abs(found)
	}

	return "", fmt.Errorf("concept not found: %s", idOrPath)
}
