package config

import (
	"os"
	"path/filepath"

	"okfw/cli/internal/graph"
)

// ReadConceptID reads a concept file and returns its frontmatter `id`, or "" if
// the file cannot be read or parsed.
func ReadConceptID(absPath string) string {
	content, err := os.ReadFile(absPath)
	if err != nil {
		return ""
	}
	z, err := ParseFrontmatter(content)
	if err != nil {
		return ""
	}
	return z.ID
}

// ResolveBodyLinkIDs extracts vanilla markdown links from body and resolves each
// to the target concept's `id` by reading the target file's frontmatter.
//
// bundleRoot is the bundle root used to resolve absolute (/-prefixed) links and
// to compute the linking file's bundle-relative directory; fileAbsPath is the
// absolute path of the file that owns body. Unresolvable or external links are
// skipped (OKF tolerates broken links). Results are deduplicated, in order.
func ResolveBodyLinkIDs(body, fileAbsPath, bundleRoot string) []string {
	targets := graph.ExtractLinkTargets(body)
	if len(targets) == 0 {
		return nil
	}

	fromDir := ""
	if rel, err := filepath.Rel(bundleRoot, filepath.Dir(fileAbsPath)); err == nil {
		fromDir = filepath.ToSlash(rel)
		if fromDir == "." {
			fromDir = ""
		}
	}

	var ids []string
	seen := make(map[string]bool)
	for _, href := range targets {
		rel := graph.NormalizeTarget(href, fromDir)
		if rel == "" {
			continue
		}
		targetAbs := filepath.Join(bundleRoot, filepath.FromSlash(rel))
		id := ReadConceptID(targetAbs)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}
