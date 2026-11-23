package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// DuplicateKeys returns the dotted paths of keys that appear more than once in
// one mapping of the frontmatter, at any depth. OKF v0.3 section 4 makes the
// frontmatter a YAML 1.2 mapping, where keys are unique, and says producers
// MUST NOT write duplicates. yaml.v3 rejects a duplicate when decoding into a
// map, but not when decoding into a struct or a yaml.Node — which is how okfw
// reads and edits — so a duplicate went unnoticed and the last value won
// silently in one reader and the first in another.
func DuplicateKeys(frontmatter []byte) []string {
	var doc yaml.Node
	if err := yaml.Unmarshal(frontmatter, &doc); err != nil || len(doc.Content) == 0 {
		return nil
	}
	return DuplicateKeysNode(doc.Content[0])
}

// DuplicateKeysNode is DuplicateKeys over an already parsed mapping node.
func DuplicateKeysNode(m *yaml.Node) []string {
	if m == nil {
		return nil
	}
	var out []string
	var walk func(n *yaml.Node, prefix string)
	walk = func(n *yaml.Node, prefix string) {
		switch n.Kind {
		case yaml.MappingNode:
			seen := make(map[string]bool, len(n.Content)/2)
			for i := 0; i+1 < len(n.Content); i += 2 {
				k := n.Content[i].Value
				p := k
				if prefix != "" {
					p = prefix + "." + k
				}
				if seen[k] {
					out = append(out, p)
				}
				seen[k] = true
				walk(n.Content[i+1], p)
			}
		case yaml.SequenceNode:
			for i, c := range n.Content {
				walk(c, fmt.Sprintf("%s[%d]", prefix, i))
			}
		}
	}
	walk(m, "")
	return out
}

func checkDuplicateKeysNode(m *yaml.Node) error {
	if d := DuplicateKeysNode(m); len(d) > 0 {
		return fmt.Errorf("duplicate frontmatter key(s): %s (OKF section 4: producers MUST NOT write duplicate keys)", strings.Join(d, ", "))
	}
	return nil
}
