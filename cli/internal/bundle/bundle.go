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

	filePath, err := WriteNewConcept(outputDir, spec.Title, id, []byte(content))
	if err != nil {
		return "", "", err
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

// ResolveConceptPath resolves a concept reference to the absolute path of a file
// inside the bundle. A reference is one of:
//
//   - the frontmatter id, or a legacy <id>.md filename;
//   - an OKF concept ID (section 2): the bundle-relative path, with or without
//     ".md", optionally with a leading "/";
//   - a file path, absolute or relative to the working directory.
//
// Path forms are tried bundle-relative first. A file that exists but lies
// outside the bundle — via "..", an absolute path, or a symlink — is refused:
// every caller goes on to read or rewrite what this returns, and an MCP client
// supplies the reference unvetted.
func ResolveConceptPath(cfg *config.Config, idOrPath string) (string, error) {
	if strings.HasSuffix(idOrPath, ".md") || strings.Contains(idOrPath, "/") {
		return resolvePathRef(cfg, idOrPath)
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
	// A bare OKF concept ID for a concept at the bundle root.
	if p := filepath.Join(cfg.RootPath, idOrPath+".md"); !IsReservedBase(idOrPath+".md") && fileExists(p) {
		return p, nil
	}

	return "", fmt.Errorf("concept not found: %s", idOrPath)
}

func resolvePathRef(cfg *config.Config, ref string) (string, error) {
	var cands []string
	bundleRel := filepath.Join(cfg.RootPath, filepath.FromSlash(strings.TrimPrefix(ref, "/")))
	if filepath.IsAbs(ref) {
		cands = append(cands, ref)
	}
	cands = append(cands, bundleRel)
	if !strings.HasSuffix(ref, ".md") {
		cands = append(cands, bundleRel+".md")
	}
	if !filepath.IsAbs(ref) {
		if abs, err := filepath.Abs(ref); err == nil {
			cands = append(cands, abs)
		}
	}

	outside := false
	for _, c := range cands {
		fi, err := os.Stat(c)
		if err != nil || fi.IsDir() {
			continue
		}
		if !Within(cfg.RootPath, c) {
			outside = true
			continue
		}
		return filepath.Clean(c), nil
	}
	if outside {
		return "", fmt.Errorf("%s is outside the wiki at %s", ref, cfg.RootPath)
	}
	return "", fmt.Errorf("file not found: %s", ref)
}

// Within reports whether path p lies inside root once symlinks are resolved.
// Both must exist for symlinks to resolve; a path that does not is compared as
// written.
func Within(root, p string) bool {
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	if q, err := filepath.EvalSymlinks(p); err == nil {
		p = q
	}
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}
