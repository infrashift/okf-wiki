package bundle

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

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

	// Type overrides the free-form OKF type, which otherwise follows the kind.
	Type string
	// Kind sets the structured kind: "note" or "issue". Empty keeps what the
	// template (or a plain note) writes. Daily notes have their own writer.
	Kind string
	// Frontmatter, when set, edits the frontmatter before the concept is
	// validated and written, so metadata set at creation lands in the same
	// single write as the concept. A create used to be followed by one write
	// per field, and a failure part-way left a half-described concept behind.
	Frontmatter func(m *yaml.Node) error
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

	if spec.Type != "" || spec.Kind != "" || spec.Frontmatter != nil {
		updated, ferr := Edit([]byte(content), func(m *yaml.Node) error {
			if err := setKindAndType(m, spec); err != nil {
				return err
			}
			if spec.Frontmatter != nil {
				return spec.Frontmatter(m)
			}
			return nil
		})
		if ferr != nil {
			return "", "", ferr
		}
		content = string(updated)
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

// setKindAndType applies a Spec's kind and type. An issue with no explicit
// type is written as type "issue", as the issue template does, so the two ways
// of creating one agree.
func setKindAndType(m *yaml.Node, spec Spec) error {
	switch spec.Kind {
	case "":
	case "note":
		// The default kind is never written; see GenerateNote.
		MapDelete(m, "kind")
	case "issue":
		mapSetAfter(m, "kind", "type", QuotedScalar("issue"))
		if spec.Type == "" {
			MapSetString(m, "type", "issue")
		}
	default:
		return fmt.Errorf("invalid kind %q: want note or issue", spec.Kind)
	}
	if spec.Type != "" {
		MapSetString(m, "type", spec.Type)
	}
	return nil
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
	//
	// A unique prefix of at least 8 hex digits also resolves (dashes ignored),
	// which is how the docs and `okfw search` output abbreviate ids.
	compactRef := compactID(idOrPath)
	prefixOK := len(compactRef) >= 8 && isHex(compactRef)

	// Every file declaring the id is collected, not just the first: two files
	// share one when a concept is copied as a template, and stopping at the
	// first match handed a writer whichever copy the walk reached first.
	// Walking the whole bundle is affordable because almost no file needs its
	// YAML parsed: a file must contain the reference, as typed or rewritten
	// with the dashes of either id layout, to declare it. The first eight hex
	// digits alone would not do — they are a UUIDv7's timestamp, shared by
	// every concept written the same day.
	needles := [][]byte{[]byte(idOrPath)}
	if prefixOK {
		for _, cuts := range [][]int{uuidDashes, legacyIDDashes} {
			d := dashed(compactRef, cuts)
			needles = append(needles, []byte(d), []byte(strings.ToUpper(d)))
		}
	}
	mayDeclare := func(content []byte) bool {
		for _, n := range needles {
			if bytes.Contains(content, n) {
				return true
			}
		}
		return false
	}

	var exact, prefixed []string
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
		// A legacy <id>.md filename.
		if info.Name() == idOrPath+".md" {
			exact = append(exact, path)
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil || !mayDeclare(content) {
			return nil
		}
		z, err := config.ParseFrontmatter(content)
		if err != nil {
			return nil
		}
		if z.ID == idOrPath {
			exact = append(exact, path)
			return nil
		}
		if prefixOK && z.ID != "" && strings.HasPrefix(compactID(z.ID), compactRef) {
			prefixed = append(prefixed, path)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("error searching for concept: %w", err)
	}

	relNames := func(paths []string) string {
		names := make([]string, 0, len(paths))
		for _, p := range paths {
			if rel, err := relSlash(cfg.RootPath, p); err == nil {
				names = append(names, rel)
			}
		}
		return strings.Join(names, ", ")
	}
	switch {
	case len(exact) == 1:
		return filepath.Abs(exact[0])
	case len(exact) > 1:
		return "", fmt.Errorf("id %s is declared by %d concepts: %s — give the path of the one you mean, and run `okfw validate --all`",
			idOrPath, len(exact), relNames(exact))
	case len(prefixed) == 1:
		return filepath.Abs(prefixed[0])
	case len(prefixed) > 1:
		return "", fmt.Errorf("id prefix %s is ambiguous; it matches %d concepts: %s",
			idOrPath, len(prefixed), relNames(prefixed))
	}
	// A bare OKF concept ID for a concept at the bundle root.
	if p := filepath.Join(cfg.RootPath, idOrPath+".md"); !IsReservedBase(idOrPath+".md") && fileExists(p) {
		return p, nil
	}

	return "", fmt.Errorf("%w: %s", ErrConceptNotFound, idOrPath)
}

// ErrConceptNotFound is wrapped by ResolveConceptPath when nothing matches, as
// opposed to a reference that matched something it must refuse.
var ErrConceptNotFound = errors.New("concept not found")

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
	return "", fmt.Errorf("%w: no file %s", ErrConceptNotFound, ref)
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

// compactID lowercases an id and drops its dashes, so prefixes compare the
// same whether or not the caller typed them.
func compactID(id string) string { return strings.ToLower(strings.ReplaceAll(id, "-", "")) }

// The compact offsets at which each id layout places a dash: a UUID
// (8-4-4-4-12), and the legacy YYYYMMDDHHmmss-UUIDv4.
var (
	uuidDashes     = []int{8, 12, 16, 20}
	legacyIDDashes = []int{14, 22, 26, 30, 34}
)

// dashed renders a compact id, or a prefix of one, with dashes at cuts.
func dashed(compact string, cuts []int) string {
	var b strings.Builder
	for i := 0; i < len(compact); i++ {
		if slices.Contains(cuts, i) {
			b.WriteByte('-')
		}
		b.WriteByte(compact[i])
	}
	return b.String()
}

func isHex(s string) bool {
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}
