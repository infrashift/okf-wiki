package bundle

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"

	"okfw/cli/internal/config"
)

// OKF v0.2 first wrote stale_after, sources[].last_modified and usage_window as
// bare YYYY-MM-DD dates; the spec now requires every timestamp to be a datetime
// with an explicit offset (section 5). These are the keys that used to hold
// dates. generated.at and verified[].at were always datetimes.
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

// UpgradeDateOnlyTimestamps rewrites bare dates in the keys OKF moved to
// datetimes as midnight UTC, returning the rewritten content and the keys it
// changed — or (nil, nil) when nothing needed changing.
//
// Midnight UTC is a choice, not a recovery: a bare date never said which
// timezone it meant. It is the reading okfw's own staleness check gave these
// values before the change, so a concept that was stale stays stale. The
// timestamp is left alone; reformatting a field is not a change to content.
func UpgradeDateOnlyTimestamps(content []byte) ([]byte, []string, error) {
	var changed []string
	out, err := Edit(content, func(m *yaml.Node) error {
		for key, n := range dateFieldNodes(m) {
			if n == nil || n.Kind != yaml.ScalarNode || !config.IsDateOnly(n.Value) {
				continue
			}
			n.Value += "T00:00:00Z"
			n.Tag = "" // let the encoder re-resolve; keep the author's quoting style
			changed = append(changed, key)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if len(changed) == 0 {
		return nil, nil, nil
	}
	sort.Strings(changed)
	return out, changed, nil
}

// TimestampsNeedingRepair lists timestamp keys whose value is neither a valid
// OKF datetime nor a bare date migrate can upgrade — most often a datetime with
// no offset. Those name an instant only the author knows, so migrate reports
// them rather than guessing.
func TimestampsNeedingRepair(content []byte) ([]string, error) {
	var bad []string
	_, err := Edit(content, func(m *yaml.Node) error {
		check := func(key string, n *yaml.Node) {
			if n == nil || n.Kind != yaml.ScalarNode || config.IsDateOnly(n.Value) {
				return
			}
			if _, err := config.ParseDateTime(n.Value); err != nil {
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
