// Package bundle provides the shared write-path for OKF bundles: concept
// creation, filename conventions, frontmatter editing, moves and deletes, and
// index.md maintenance.
package bundle

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"okfw/cli/internal/config"
)

// Slugify converts a title into a lowercase, ASCII, hyphen-separated slug
// suitable for a filename. Reserved OKF basenames are guarded.
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevHyphen := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevHyphen = false
		case r == ' ' || r == '-' || r == '_' || r == '/' || r == '.':
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "note"
	}
	if slug == "index" || slug == "log" {
		slug += "-note" // never collide with reserved OKF filenames
	}
	return slug
}

// ShortID returns the leading hex of a UUID/legacy id for filename disambiguation.
func ShortID(id string, n int) string {
	hex := strings.ReplaceAll(id, "-", "")
	if n > len(hex) {
		n = len(hex)
	}
	return hex[:n]
}

// NoteFilename builds a hybrid "slug--shortid.md" filename for title/id, unique
// within dir. The short id defaults to the first 12 hex chars of the id — the full
// UUIDv7 millisecond timestamp — so filenames match the frontmatter id's first two
// groups and sort chronologically; on collision it is extended into the random bits.
// Returns the base name only.
//
// The answer is only true at the moment of asking; WriteNewConcept is what a
// writer should use.
func NoteFilename(dir, title, id string) string {
	for _, name := range noteFilenames(title, id) {
		if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
			return name
		}
	}
	return noteFilenames(title, id)[0]
}

// noteFilenames lists the candidate names for title/id in order of preference.
func noteFilenames(title, id string) []string {
	slug := Slugify(title)
	var names []string
	for n := 12; n <= 32; n += 2 {
		names = append(names, slug+"--"+ShortID(id, n)+".md")
	}
	// Fallback: full compact id (effectively always unique).
	return append(names, slug+"--"+strings.ReplaceAll(id, "-", "")+".md")
}

// WriteNewConcept writes content to a new "slug--shortid.md" file in dir and
// returns its path. Each name is claimed with O_EXCL, so two writers racing for
// one name cannot both win. Checking with NoteFilename and then writing let
// concurrent creates of the same title in the same millisecond — which share a
// 12-character short id — pick one name, and all but one were silently
// overwritten while every caller reported success.
func WriteNewConcept(dir, title, id string, content []byte) (string, error) {
	for _, name := range noteFilenames(title, id) {
		p := filepath.Join(dir, name)
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("failed to write file: %w", err)
		}
		if _, err := f.Write(content); err != nil {
			f.Close()
			os.Remove(p)
			return "", fmt.Errorf("failed to write file: %w", err)
		}
		if err := f.Close(); err != nil {
			os.Remove(p)
			return "", fmt.Errorf("failed to write file: %w", err)
		}
		return p, nil
	}
	return "", fmt.Errorf("no free filename for %q in %s", title, dir)
}

// SanitizeSubdir cleans a user/agent-requested bundle subdirectory and rejects
// any path that would escape the bundle root or land inside the bundle's own
// internals. Returns "" if invalid.
//
// Dot-directories are rejected because SkipWalkDir skips them: a concept written
// to one would be invisible to search, index.md, and every other walk.
func SanitizeSubdir(p string) string {
	p = strings.TrimSpace(filepath.ToSlash(p))
	p = strings.TrimPrefix(p, "/")
	cleaned := filepath.ToSlash(filepath.Clean(p))
	if cleaned == "." || cleaned == "" {
		return ""
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return ""
	}
	for _, seg := range strings.Split(cleaned, "/") {
		if strings.HasPrefix(seg, ".") {
			return ""
		}
	}
	return cleaned
}

// ConceptDir resolves where a new concept should be written: an explicit --path
// (sanitized, in-bundle) takes precedence; otherwise the project name is used as
// a top-level folder; otherwise the bundle root.
func ConceptDir(cfg *config.Config, pathFlag, project string) (string, error) {
	if pathFlag != "" {
		sub := SanitizeSubdir(pathFlag)
		if sub == "" {
			return "", fmt.Errorf("invalid --path %q: must be a subdirectory within the bundle", pathFlag)
		}
		return filepath.Join(cfg.RootPath, filepath.FromSlash(sub)), nil
	}
	if project != "" {
		if sub := SanitizeSubdir(project); sub != "" {
			return filepath.Join(cfg.RootPath, filepath.FromSlash(sub)), nil
		}
	}
	return cfg.RootPath, nil
}

// IsReservedBase reports whether name is an OKF reserved filename (index.md/log.md).
func IsReservedBase(name string) bool {
	return name == "index.md" || name == "log.md"
}

// RelDir returns the directory portion of a slash-separated bundle-relative path.
func RelDir(rel string) string {
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		return rel[:i]
	}
	return ""
}

// SkipWalkDir reports whether a bundle walk should skip the directory: hidden
// directories and the ephemeral export scratch area, but never the root itself.
// ephemeral/ is okfw's own directory at the walk root only; a "ephemeral"
// directory deeper down is an ordinary part of the bundle, and skipping it at
// any depth hid its concepts from every command, validate --okf included.
func SkipWalkDir(root, p, name string) bool {
	if p == root {
		return false
	}
	return strings.HasPrefix(name, ".") || (name == "ephemeral" && filepath.Dir(p) == root)
}
