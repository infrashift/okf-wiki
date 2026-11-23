package config

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// okfw's own frontmatter keys. They live in the extension namespace OKF v0.3
// section 4.1 reserves for producers — "Future OKF versions will not define
// keys that start with x-. Producers SHOULD name their own keys
// x-<producer>-<name>" — so a future OKF version can never give one of them a
// different meaning. Before that, okfw wrote them bare (id, kind, project,
// parent, created, timestamp), and OKF v0.1 had already given `timestamp` a
// meaning of its own (the generation time, Appendix B).
const (
	KeyID        = "x-okfw-id"
	KeyKind      = "x-okfw-kind"
	KeyProject   = "x-okfw-project"
	KeyParent    = "x-okfw-parent"
	KeyCreated   = "x-okfw-created"
	KeyTimestamp = "x-okfw-timestamp"
)

// LegacyKeys maps each bare key okfw wrote before the namespace to its
// x-okfw- name, in the order okfw wrote them. Reads accept either; writes and
// strict validation use only the new names; `okfw migrate`, and any okfw edit
// of a file okfw wrote, renames them.
var LegacyKeys = []struct{ Old, New string }{
	{"id", KeyID},
	{"kind", KeyKind},
	{"project", KeyProject},
	{"parent", KeyParent},
	{"created", KeyCreated},
	{"timestamp", KeyTimestamp},
}

// legacyKeys holds the pre-namespace keys as decoded, inlined into Concept so
// one decode reads both spellings.
type legacyKeys struct {
	LegacyID        string    `yaml:"id,omitempty"`
	LegacyKind      string    `yaml:"kind,omitempty"`
	LegacyProject   string    `yaml:"project,omitempty"`
	LegacyParent    string    `yaml:"parent,omitempty"`
	LegacyCreated   time.Time `yaml:"created,omitempty"`
	LegacyTimestamp time.Time `yaml:"timestamp,omitempty"`
}

// adoptLegacy fills each okfw field the x-okfw- key left empty from its bare
// predecessor. Where a file holds both, the new key wins.
func (z *Concept) adoptLegacy() {
	if z.ID == "" {
		z.ID = z.LegacyID
	}
	if z.Kind == "" {
		z.Kind = z.LegacyKind
	}
	if z.Project == "" {
		z.Project = z.LegacyProject
	}
	if z.Parent == "" {
		z.Parent = z.LegacyParent
	}
	if z.Created.IsZero() {
		z.Created = z.LegacyCreated
	}
	if z.Timestamp.IsZero() {
		z.Timestamp = z.LegacyTimestamp
	}
}

var okfwIDShape = regexp.MustCompile(`^([0-9]{14}-)?[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// IsOkfwIDShaped reports whether s looks like an okfw id: a UUID, or the
// legacy YYYYMMDDHHmmss-UUID (#ConceptID in the schema).
func IsOkfwIDShaped(s string) bool { return okfwIDShape.MatchString(s) }

// LegacyKeysToUpgrade returns the bare keys of a frontmatter mapping that okfw
// itself wrote before the namespace, or nil. It answers only for a mapping
// okfw wrote — an `id` shaped like an okfw id, or both `created` and
// `timestamp` — because another OKF producer may use a bare `id` or `kind` of
// its own, and relabelling that as okfw's would be wrong.
func LegacyKeysToUpgrade(m *yaml.Node) []string {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	present := map[string]*yaml.Node{}
	for i := 0; i+1 < len(m.Content); i += 2 {
		present[m.Content[i].Value] = m.Content[i+1]
	}
	id := present["id"]
	ours := (id != nil && id.Kind == yaml.ScalarNode && IsOkfwIDShaped(id.Value)) ||
		(present["created"] != nil && present["timestamp"] != nil)
	if !ours {
		return nil
	}
	var out []string
	for _, k := range LegacyKeys {
		if present[k.Old] != nil {
			out = append(out, k.Old)
		}
	}
	return out
}

// UpgradeLegacyKeys renames, in place, the bare keys LegacyKeysToUpgrade
// reports to their x-okfw- names, keeping each one's position, value and
// quoting. Where the new key already exists, the bare one is dropped. It
// reports the bare keys it changed.
func UpgradeLegacyKeys(m *yaml.Node) []string {
	old := LegacyKeysToUpgrade(m)
	if len(old) == 0 {
		return nil
	}
	newOf := map[string]string{}
	for _, k := range LegacyKeys {
		newOf[k.Old] = k.New
	}
	has := map[string]bool{}
	for i := 0; i+1 < len(m.Content); i += 2 {
		has[m.Content[i].Value] = true
	}
	want := map[string]bool{}
	for _, k := range old {
		want[k] = true
	}
	kept := m.Content[:0]
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		if want[k.Value] {
			if has[newOf[k.Value]] {
				continue // the new key wins; drop the old one
			}
			k.Value = newOf[k.Value]
		}
		kept = append(kept, k, v)
	}
	m.Content = kept
	return old
}

// checkLegacyKeys refuses, with the fix, a concept okfw wrote under the bare
// key names. The closed schema would reject them too, but only as "field not
// allowed", which does not say what to do.
func checkLegacyKeys(frontmatter []byte) error {
	var doc yaml.Node
	if yaml.Unmarshal(frontmatter, &doc) != nil || len(doc.Content) == 0 {
		return nil
	}
	if old := LegacyKeysToUpgrade(doc.Content[0]); len(old) > 0 {
		return fmt.Errorf("frontmatter uses okfw's pre-namespace keys (%s); okfw's keys are now x-okfw-*: run `okfw migrate`",
			strings.Join(old, ", "))
	}
	return nil
}
