package bundle

import (
	"bytes"
	"path"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"okfw/cli/internal/config"
	"okfw/cli/internal/graph"
)

// refNode is one path-valued frontmatter field located in the yaml.Node tree.
type refNode struct {
	field string
	node  *yaml.Node
}

// pathRefNodes walks the path-valued fields of a frontmatter mapping — the
// same five, in the same order, as config.Concept.PathRefs — returning the
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
	return out
}

// RewritePathRefs applies rewrite to every path-valued frontmatter field that
// names a concept (graph.NormalizeRef), with the contract of
// graph.RewriteLinks: rewrite receives the path as a body link would spell it
// (graph.RefLink) with any #fragment removed, and returns its replacement, or
// "" (or the same path) to leave it; the fragment is kept. Callers can
// therefore pass the closures they use for body links, and the replacement is
// translated back: a bare path, which rewrite sees as bundle-absolute, is
// written back bare, and a bare replacement, which in a body link is relative,
// gains a "./" so it is not read from the bundle root. Content is returned
// unchanged, and not re-encoded, when nothing is rewritten.
func RewritePathRefs(content []byte, fromDir string, rewrite func(href string) string) ([]byte, error) {
	apply := func(value string) (string, bool) {
		if graph.NormalizeRef(value, fromDir) == "" {
			return "", false
		}
		p, frag := value, ""
		if i := strings.IndexByte(value, '#'); i >= 0 {
			p, frag = value[:i], value[i:]
		}
		href := graph.RefLink(p)
		v := rewrite(href)
		if v == "" || v == href {
			return "", false
		}
		switch {
		case strings.HasPrefix(v, "/"):
			if href != p {
				v = v[1:]
			}
		case graph.RefLink(v) != v:
			v = "./" + v
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
func linksTo(content []byte, fromDir, target string) bool {
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
			if graph.NormalizeRef(r.Value, fromDir) == target {
				return true
			}
		}
	}
	return false
}
