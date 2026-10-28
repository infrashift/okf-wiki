package bundle

import (
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

// TouchTimestamp ensures the updates map stamps a fresh `timestamp` unless the
// caller set one explicitly.
func TouchTimestamp(updates map[string]string) map[string]string {
	if updates == nil {
		updates = map[string]string{}
	}
	if _, ok := updates["timestamp"]; !ok {
		updates["timestamp"] = time.Now().Format(time.RFC3339)
	}
	return updates
}

// sortedKeys returns a map's keys in a stable order. Appended fields used to
// land in Go's randomized map order, so two runs of the same update could
// produce different frontmatter.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// UpdateFrontmatter sets fields in a concept's YAML frontmatter, bumping
// `timestamp` unless the caller supplied one. Keys may be dotted paths
// ("okfw.status"); intermediate mappings are created as needed.
func UpdateFrontmatter(content []byte, updates map[string]string) ([]byte, error) {
	updates = TouchTimestamp(updates)
	out, err := Edit(content, func(m *yaml.Node) error {
		for _, k := range sortedKeys(updates) {
			if err := SetPath(m, k, updates[k]); err != nil {
				return err
			}
		}
		return syncGeneratedAt(m, updates["timestamp"])
	})
	if err != nil {
		return nil, err
	}
	if err := guardMutation(content, out); err != nil {
		return nil, err
	}
	return out, nil
}

// syncGeneratedAt keeps `generated.at` equal to `timestamp`. Both record when the
// content last meaningfully changed, and ValidateBusinessRules rejects a
// disagreement, so exactly one code path updates both.
//
// It only touches an existing `generated` block: creating one would need a
// `generated.by` actor, and inventing an attribution is worse than omitting it.
func syncGeneratedAt(m *yaml.Node, timestamp string) error {
	if timestamp == "" {
		return nil
	}
	if MapGet(m, "generated") == nil {
		return nil
	}
	return SetPath(m, "generated.at", timestamp)
}

// UpdateFrontmatterRemove sets fields and removes others. Removals are applied
// first so a field can be moved in a single call.
func UpdateFrontmatterRemove(content []byte, updates map[string]string, remove []string) ([]byte, error) {
	updates = TouchTimestamp(updates)
	out, err := Edit(content, func(m *yaml.Node) error {
		for _, path := range remove {
			DeletePath(m, path)
		}
		for _, k := range sortedKeys(updates) {
			if err := SetPath(m, k, updates[k]); err != nil {
				return err
			}
		}
		return syncGeneratedAt(m, updates["timestamp"])
	})
	if err != nil {
		return nil, err
	}
	if err := guardMutation(content, out); err != nil {
		return nil, err
	}
	return out, nil
}

// RemoveFrontmatterTags removes tags from the `tags` sequence. A tag that is not
// present is skipped, so the call is idempotent. The sequence stays, empty if
// need be: `tags` is a required field.
func RemoveFrontmatterTags(content []byte, tags []string) ([]byte, error) {
	drop := map[string]bool{}
	for _, t := range tags {
		drop[t] = true
	}
	out, err := Edit(content, func(m *yaml.Node) error {
		cur := MapGet(m, "tags")
		if cur == nil || cur.Kind != yaml.SequenceNode {
			return nil
		}
		kept := cur.Content[:0]
		for _, n := range cur.Content {
			if n.Kind == yaml.ScalarNode && drop[n.Value] {
				continue
			}
			kept = append(kept, n)
		}
		cur.Content = kept
		if len(kept) == 0 {
			cur.Style = yaml.FlowStyle // render `tags: []`, as a new concept does
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := guardMutation(content, out); err != nil {
		return nil, err
	}
	return out, nil
}

// AddFrontmatterTags appends tags to the `tags` sequence, creating it when
// absent. Tags already present are skipped, so the call is idempotent.
func AddFrontmatterTags(content []byte, tags []string) ([]byte, error) {
	out, err := Edit(content, func(m *yaml.Node) error {
		existing := map[string]bool{}
		for _, t := range SeqStrings(m, "tags") {
			existing[t] = true
		}
		for _, t := range tags {
			if existing[t] {
				continue
			}
			if err := AppendToSeq(m, "tags", QuotedScalar(t)); err != nil {
				return err
			}
			existing[t] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := guardMutation(content, out); err != nil {
		return nil, err
	}
	return out, nil
}
