package config

import (
	"strconv"

	"github.com/infrashift/okf-wiki/internal/graph"
)

// ResourceRef is the part of an Attested Computation's executor or attester
// that okfw reads: the path it names (OKF section 10.2).
type ResourceRef struct {
	Resource string `yaml:"resource,omitempty"`
}

// PathRef is one path-valued frontmatter field (OKF section 6.2) and its value
// as written.
type PathRef struct {
	Field string // e.g. "sources[1].resource"
	Value string
}

// PathRefs returns the concept's non-empty path-valued fields (OKF v0.3
// section 6.2): resource, sources[].resource, computation, executor.resource,
// attester.resource and superseded_by.
//
// A path in one of these is a link: section 5.1 says a source naming another
// concept "already exists in the bundle graph". okfw read only body links, so
// such an edge was missing from backlinks and delete's inbound report, and a
// move silently broke it. bundle.pathRefNodes walks the same fields of a
// yaml.Node to rewrite them; TestPathRefFieldsMatchTheRewriter keeps the two
// lists in step.
func (z *Concept) PathRefs() []PathRef {
	var out []PathRef
	add := func(field, v string) {
		if v != "" {
			out = append(out, PathRef{field, v})
		}
	}
	add("resource", z.Resource)
	for i, s := range z.Sources {
		add("sources["+strconv.Itoa(i)+"].resource", s.Resource)
	}
	add("computation", z.Computation)
	if z.Executor != nil {
		add("executor.resource", z.Executor.Resource)
	}
	if z.Attester != nil {
		add("attester.resource", z.Attester.Resource)
	}
	add("superseded_by", z.SupersededBy)
	return out
}

// ResolveConceptLinks resolves a concept's outbound links to concept keys: its
// body links, then the path-valued frontmatter fields that name a concept
// (graph.ResolveRef, section 6.4). keyOf is as for ResolveBodyLinks; exists
// turns on the v0.2 path fallback (nil for a v0.3 bundle). Deduplicated, in
// order.
func ResolveConceptLinks(z *Concept, body, fromDir string, keyOf func(rel string) string, exists func(rel string) bool) []string {
	keys := ResolveBodyLinks(body, fromDir, keyOf)
	seen := make(map[string]bool, len(keys))
	for _, k := range keys {
		seen[k] = true
	}
	for _, r := range z.PathRefs() {
		rel := graph.ResolveRef(r.Value, fromDir, exists)
		if rel == "" {
			continue
		}
		if k := keyOf(rel); k != "" && !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	return keys
}
