// Package bundle provides the shared write-path for OKF bundles: concept
// creation, filename conventions, frontmatter editing, and index/graph
// synchronization. It is used by both the okfw and graphy binaries.
package bundle

import (
	"fmt"
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
func NoteFilename(dir, title, id string) string {
	slug := Slugify(title)
	for n := 12; n <= 32; n += 2 {
		name := slug + "--" + ShortID(id, n) + ".md"
		if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
			return name
		}
	}
	// Fallback: full compact id (effectively always unique).
	return slug + "--" + strings.ReplaceAll(id, "-", "") + ".md"
}

// SanitizeSubdir cleans a user/agent-requested bundle subdirectory and rejects
// any path that would escape the bundle root. Returns "" if invalid.
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
func SkipWalkDir(root, p, name string) bool {
	if p == root {
		return false
	}
	return strings.HasPrefix(name, ".") || name == "ephemeral"
}
