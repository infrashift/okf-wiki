package bundle

import (
	"bytes"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/infrashift/okf-wiki/internal/config"
	"github.com/infrashift/okf-wiki/internal/graph"
)

// refNode is one path-valued frontmatter field located in the yaml.Node tree.
type refNode struct {
	field string
	node  *yaml.Node
}

// pathRefNodes walks the path-valued fields of a frontmatter mapping — the
// same six, in the same order, as config.Concept.PathRefs — returning the
// scalar node of each so a caller can rewrite it in place. Rewriting the node
// rather than the text keeps comments, quoting and key order (see Edit).
func pathRefNodes(m *yaml.Node) []refNode {
	var out []refNode
	scalar := func(field string, parent *yaml.Node, key string) {
		if parent == nil || parent.Kind != yaml.MappingNode {
			return
		}
		if v := MapGet(parent, key); v != nil && v.Kind == yaml.ScalarNode && v.Value != "" {
			out = append(out, refNode{field, v})
		}
	}
	scalar("resource", m, "resource")
	if seq := MapGet(m, "sources"); seq != nil && seq.Kind == yaml.SequenceNode {
		for i, s := range seq.Content {
			scalar("sources["+strconv.Itoa(i)+"].resource", s, "resource")
		}
	}
	scalar("computation", m, "computation")
	scalar("executor.resource", MapGet(m, "executor"), "resource")
	scalar("attester.resource", MapGet(m, "attester"), "resource")
	scalar("superseded_by", m, "superseded_by")
	return out
}

// RefExists returns the existence check that turns on OKF v0.3 section 6.2's
// compatibility fallback (graph.ResolveRef) for a bundle that declares
// okf_version 0.2 or earlier, or none — every bundle okfw wrote before v0.3
// spelled bare path fields from the root. It is nil for a v0.3 bundle, where a
// bare path resolves from its file's directory and nothing else.
func RefExists(bundleRoot string) func(rel string) bool {
	if !config.LegacyPathFallback(config.ReadDeclaredOKFVersion(bundleRoot)) {
		return nil
	}
	return func(rel string) bool {
		_, err := os.Stat(filepath.Join(bundleRoot, filepath.FromSlash(rel)))
		return err == nil
	}
}

// RewritePathRefs applies rewrite to every path-valued frontmatter field that
// names a concept, with the contract of graph.RewriteLinks: rewrite receives
// the path with any #fragment removed and returns its replacement, or "" (or
// the same path) to leave it; the fragment is kept. Since OKF v0.3 a path
// field resolves exactly as a body link does (section 6), so callers pass the
// closures they use for body links and the value is handed over as written.
//
// exists is RefExists for the bundle. When it is non-nil and a bare value only
// resolves through the v0.2 fallback (from the bundle root), rewrite sees it
// spelled bundle-absolute, "/"+value, which is what it means. Content is
// returned unchanged, and not re-encoded, when nothing is rewritten.
func RewritePathRefs(content []byte, fromDir string, rewrite func(href string) string, exists func(rel string) bool) ([]byte, error) {
	apply := func(value string) (string, bool) {
		rel := graph.ResolveRef(value, fromDir, exists)
		if rel == "" {
			return "", false
		}
		p, frag := value, ""
		if i := strings.IndexByte(value, '#'); i >= 0 {
			p, frag = value[:i], value[i:]
		}
		href := strings.TrimSpace(p)
		if rel != graph.NormalizeRef(value, fromDir) {
			href = "/" + href // resolved by the v0.2 fallback: from the root
		}
		v := rewrite(href)
		if v == "" || v == href {
			return "", false
		}
		return v + frag, true
	}
	z, err := config.ParseFrontmatter(content)
	if err != nil {
		return content, nil
	}
	changes := false
	for _, r := range z.PathRefs() {
		if _, ok := apply(r.Value); ok {
			changes = true
			break
		}
	}
	if !changes {
		return content, nil
	}
	return Edit(content, func(m *yaml.Node) error {
		for _, r := range pathRefNodes(m) {
			if v, ok := apply(r.node.Value); ok {
				r.node.Value = v
			}
		}
		return nil
	})
}

// mayReference reports whether content's frontmatter could hold a path to
// target (bundle-relative): any spelling of one ends in target's file name,
// unless it is percent-encoded. move and delete walk every concept, and parsing
// each one's YAML to find out made a move on 3,000 concepts 50% slower.
func mayReference(content []byte, target string) bool {
	fm, err := config.ExtractFrontmatter(content)
	if err != nil {
		return false
	}
	return bytes.Contains(fm, []byte(path.Base(target))) || bytes.IndexByte(fm, '%') >= 0
}

// linksTo reports whether a concept at fromDir, with this content, links to
// target (bundle-relative) from its body or a path-valued frontmatter field.
// exists is RefExists for the bundle.
func linksTo(content []byte, fromDir, target string, exists func(rel string) bool) bool {
	for _, href := range graph.ExtractLinkTargets(config.ExtractBody(content)) {
		if graph.NormalizeTarget(href, fromDir) == target {
			return true
		}
	}
	if !mayReference(content, target) {
		return false
	}
	if z, err := config.ParseFrontmatter(content); err == nil {
		for _, r := range z.PathRefs() {
			if graph.ResolveRef(r.Value, fromDir, exists) == target {
				return true
			}
		}
	}
	return false
}
