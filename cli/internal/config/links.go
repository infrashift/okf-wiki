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

// ReadConceptKey reads the concept at bundleRel and returns its ConceptKey, or
// "" when there is no concept there.
func ReadConceptKey(bundleRoot, bundleRel string) string {
	content, err := os.ReadFile(filepath.Join(bundleRoot, filepath.FromSlash(bundleRel)))
	if err != nil {
		return ""
	}
	z, err := ParseFrontmatter(content)
	if err != nil {
		return ""
	}
	return ConceptKey(z, bundleRel)
}

// ResolveBodyLinks resolves the internal markdown links in body to concept
// keys. fromDir is the linking file's bundle-relative directory; keyOf maps a
// bundle-relative target path to its concept key, or "" to skip the link; what
// a missing target maps to is keyOf's call. Results are deduplicated, in order.
//
// keyOf is a parameter so a caller that has already read the bundle answers
// from memory: resolving by reading each target's file cost a graph query six
// file opens per concept.
func ResolveBodyLinks(body, fromDir string, keyOf func(rel string) string) []string {
	var keys []string
	seen := make(map[string]bool)
	for _, href := range graph.ExtractLinkTargets(body) {
		rel := graph.NormalizeTarget(href, fromDir)
		if rel == "" {
			continue
		}
		key := keyOf(rel)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	return keys
}
