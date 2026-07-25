package extract

import (
	"fmt"

	yaml "gopkg.in/yaml.v3"

	"okfw/cli/internal/graphy/model"
)

// Ansible semantic layer on top of the YAML extractor.
//
// Ansible is plain YAML, but two things the generic extractor can't see carry
// its architecture: playbooks are *sequences* of plays (which the generic
// mapping-walk skips entirely), and roles are referenced by NAME, resolved by
// directory convention (roles/<name>/...), not by path. This layer turns
// plays into nodes and role references into "role:<name>" import refs that
// the build resolves against the roles/ convention.
//
// Handler/notify edges (also name-based) are deliberately out of scope for
// now: they would require nodeifying every named task.

// ansiblePlay is one play in a playbook document.
type ansiblePlay struct {
	node *yaml.Node // the play mapping
	name string     // play name ("" when unnamed)
}

// ansiblePlays reports the plays when doc is an Ansible playbook: a sequence
// whose mapping items carry a "hosts" key.
func ansiblePlays(doc *yaml.Node) []ansiblePlay {
	if doc.Kind != yaml.SequenceNode {
		return nil
	}
	var plays []ansiblePlay
	for _, item := range doc.Content {
		if item.Kind != yaml.MappingNode || mapValue(item, "hosts") == nil {
			return nil // any non-play item means this isn't a playbook
		}
		plays = append(plays, ansiblePlay{node: item, name: mapScalar(item, "name")})
	}
	return plays
}

// playLabel renders a play's display name and stable qual.
func playLabel(p ansiblePlay, index int) (label, qual string) {
	if p.name != "" {
		return "play: " + p.name, p.name
	}
	hosts := mapScalar(p.node, "hosts")
	if hosts != "" {
		return "play: " + hosts, fmt.Sprintf("play[%d]:%s", index, hosts)
	}
	return fmt.Sprintf("play[%d]", index), fmt.Sprintf("play[%d]", index)
}

// ansibleRoleRefs walks a subtree for role references — `roles:` lists and
// include_role/import_role tasks — and emits "role:<name>" import refs the
// build resolves via the roles/<name>/ convention.
func ansibleRoleRefs(root *yaml.Node, fromID string, refs *[]model.Ref) {
	seen := map[string]bool{}
	emit := func(name string, line int) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		*refs = append(*refs, model.Ref{
			FromID:   fromID,
			Name:     "role:" + name,
			Relation: model.RelImports,
			Line:     line,
		})
	}

	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		if n == nil {
			return
		}
		if n.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(n.Content); i += 2 {
				key, val := n.Content[i], n.Content[i+1]
				if key.Kind != yaml.ScalarNode {
					continue
				}
				switch key.Value {
				case "roles":
					// roles: [web, {role: db, tags: [...]}]
					if val.Kind == yaml.SequenceNode {
						for _, entry := range val.Content {
							switch entry.Kind {
							case yaml.ScalarNode:
								emit(entry.Value, entry.Line)
							case yaml.MappingNode:
								if name := mapScalar(entry, "role"); name != "" {
									emit(name, entry.Line)
								} else if name := mapScalar(entry, "name"); name != "" {
									emit(name, entry.Line)
								}
							}
						}
					}
				case "include_role", "import_role":
					// - include_role: {name: web}
					if val.Kind == yaml.MappingNode {
						emit(mapScalar(val, "name"), val.Line)
					}
				}
			}
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(root)
}
