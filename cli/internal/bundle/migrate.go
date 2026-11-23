package bundle

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/infrashift/okf-wiki/internal/config"
)

// dateFieldNodes returns the timestamp keys that may hold a bare date: OKF v0.2
// first wrote stale_after, sources[].last_modified and usage_window as dates,
// an erratum then required datetimes, and v0.3 accepts both forms again.
func dateFieldNodes(m *yaml.Node) map[string]*yaml.Node {
	nodes := map[string]*yaml.Node{"stale_after": MapGet(m, "stale_after")}
	window := func(prefix string, w *yaml.Node) {
		if w == nil || w.Kind != yaml.MappingNode {
			return
		}
		nodes[prefix+".from"] = MapGet(w, "from")
		nodes[prefix+".to"] = MapGet(w, "to")
	}
	window("usage_window", MapGet(m, "usage_window"))
	if seq := MapGet(m, "sources"); seq != nil && seq.Kind == yaml.SequenceNode {
		for i, src := range seq.Content {
			if src.Kind != yaml.MappingNode {
				continue
			}
			p := fmt.Sprintf("sources[%d]", i)
			nodes[p+".last_modified"] = MapGet(src, "last_modified")
			window(p+".usage_window", MapGet(src, "usage_window"))
		}
	}
	return nodes
}

// TimestampsNeedingRepair lists timestamp keys whose value is in neither OKF
// v0.3 form (section 5) — most often a datetime with no offset. Those name an
// instant only the author knows, so migrate reports them rather than guessing.
func TimestampsNeedingRepair(content []byte) ([]string, error) {
	var bad []string
	_, err := Edit(content, func(m *yaml.Node) error {
		check := func(key string, n *yaml.Node) {
			if n == nil || n.Kind != yaml.ScalarNode || n.Value == "" {
				return
			}
			if _, err := config.ParseTimestamp(n.Value); err != nil {
				bad = append(bad, key)
			}
		}
		for key, n := range dateFieldNodes(m) {
			check(key, n)
		}
		if g := MapGet(m, "generated"); g != nil && g.Kind == yaml.MappingNode {
			check("generated.at", MapGet(g, "at"))
		}
		switch v := MapGet(m, "verified"); {
		case v == nil:
		case v.Kind == yaml.MappingNode:
			check("verified.at", MapGet(v, "at"))
		case v.Kind == yaml.SequenceNode:
			for i, e := range v.Content {
				if e.Kind == yaml.MappingNode {
					check(fmt.Sprintf("verified[%d].at", i), MapGet(e, "at"))
				}
			}
		}
		return nil
	})
	sort.Strings(bad)
	return bad, err
}

// UpgradeOkfwKeys renames the bare keys okfw wrote before its namespace — id,
// kind, project, parent, created, timestamp — to x-okfw-*, keeping each
// key's place, value and quoting. It returns the rewritten content and the
// keys it renamed, or (nil, nil) when there is nothing to rename, including
// for a concept another producer wrote (config.LegacyKeysToUpgrade).
func UpgradeOkfwKeys(content []byte) ([]byte, []string, error) {
	if !hasFrontmatter(content) {
		return nil, nil, nil
	}
	fm, _, _, err := splitFrontmatter(content)
	if err != nil {
		return nil, nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(fm, &doc); err != nil || len(doc.Content) == 0 {
		return nil, nil, nil
	}
	old := config.LegacyKeysToUpgrade(doc.Content[0])
	if len(old) == 0 {
		return nil, nil, nil
	}
	out, err := Edit(content, func(*yaml.Node) error { return nil }) // Edit renames them
	if err != nil {
		return nil, nil, err
	}
	return out, old, nil
}
