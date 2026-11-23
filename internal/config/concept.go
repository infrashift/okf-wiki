package config

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/infrashift/okf-wiki/internal/graph"
)

// Trust tiers derived from the current `verified` events (OKF v0.3 section 5.3).
const (
	TrustUnverified       = "unverified"
	TrustMachineConfirmed = "machine-confirmed"
	TrustHumanReviewed    = "human-reviewed"
)

// HumanActorPrefix marks a hand-authored or human-confirmed actor. The trust
// tier keys off this prefix and nothing else: "team:..." is *not* human.
const HumanActorPrefix = "human:"

// Generated records who wrote the concept and when its content last
// meaningfully changed (OKF section 5.2). A trivial edit — format, whitespace,
// link repair — does not move At, and okfw's own `timestamp` is not it.
type Generated struct {
	By string `yaml:"by"`
	At string `yaml:"at,omitempty"` // OKF timestamp (section 5)
}

// Verification is one verification event (OKF section 5.2). v0.3 makes both
// keys REQUIRED in an event; a consumer ignores an event missing either.
type Verification struct {
	By string `yaml:"by"`
	At string `yaml:"at,omitempty"` // OKF timestamp (section 5)
}

// VerifiedList is the `verified` field. OKF section 5.2 and section 11 make it a
// MUST that a single bare mapping is treated as a one-element list, so the
// decoder accepts both shapes.
type VerifiedList []Verification

// UnmarshalYAML normalizes a bare mapping into a one-element list.
func (v *VerifiedList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.MappingNode:
		var one Verification
		if err := value.Decode(&one); err != nil {
			return err
		}
		*v = VerifiedList{one}
		return nil
	case yaml.SequenceNode:
		var many []Verification
		if err := value.Decode(&many); err != nil {
			return err
		}
		*v = many
		return nil
	case yaml.ScalarNode:
		if value.Tag == "!!null" || value.Value == "" {
			*v = nil
			return nil
		}
	}
	return fmt.Errorf("verified must be a mapping or a list of mappings")
}

// StringList is a list-valued field that OKF v0.3 section 11.2 lets hold one
// item: a scalar is a one-item list (a scalar `tags: a, b` is the single tag
// "a, b" — consumers MUST NOT split it at commas), and null or an empty string
// is absent.
type StringList []string

// UnmarshalYAML accepts a scalar or a sequence of scalars.
func (l *StringList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		if value.Tag == "!!null" || value.Value == "" {
			*l = nil
			return nil
		}
		*l = StringList{value.Value}
		return nil
	case yaml.SequenceNode:
		var many []string
		if err := value.Decode(&many); err != nil {
			return err
		}
		*l = many
		return nil
	}
	return fmt.Errorf("must be a string or a list of strings")
}

// SourceList is `sources`; like `verified`, one bare mapping is a one-item
// list (section 11.2).
type SourceList []Source

// UnmarshalYAML normalizes a bare mapping into a one-element list.
func (l *SourceList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.MappingNode:
		var one Source
		if err := value.Decode(&one); err != nil {
			return err
		}
		*l = SourceList{one}
		return nil
	case yaml.SequenceNode:
		var many []Source
		if err := value.Decode(&many); err != nil {
			return err
		}
		*l = many
		return nil
	case yaml.ScalarNode:
		if value.Tag == "!!null" || value.Value == "" {
			*l = nil
			return nil
		}
	}
	return fmt.Errorf("sources must be a mapping or a list of mappings")
}

// UsageWindow frames usage_count values with a datetime range (OKF section 5.1).
type UsageWindow struct {
	From string `yaml:"from,omitempty"`
	To   string `yaml:"to,omitempty"`
}

// Source is one provenance entry (OKF section 5.1).
type Source struct {
	Resource     string       `yaml:"resource"` // required within an entry
	ID           string       `yaml:"id,omitempty"`
	Title        string       `yaml:"title,omitempty"`
	Author       string       `yaml:"author,omitempty"`
	UsageCount   *int         `yaml:"usage_count,omitempty"`
	LastModified string       `yaml:"last_modified,omitempty"`
	UsageWindow  *UsageWindow `yaml:"usage_window,omitempty"`
}

// Concept represents a bundle note's frontmatter.
type Concept struct {
	// OKF core
	Type        string     `yaml:"type"` // OKF required: free-form concept kind
	Title       string     `yaml:"title"`
	Description string     `yaml:"description,omitempty"` // one-sentence summary
	Resource    string     `yaml:"resource,omitempty"`    // URI of the underlying asset
	Tags        StringList `yaml:"tags"`

	// OKF lifecycle. Absent means "stable". SupersededBy is a path field
	// (section 6.2) with a meaning only under status: deprecated (section 5.4).
	Status       string `yaml:"status,omitempty"`
	SupersededBy string `yaml:"superseded_by,omitempty"`
	StaleAfter   string `yaml:"stale_after,omitempty"`

	// OKF trust
	Generated *Generated   `yaml:"generated,omitempty"`
	Verified  VerifiedList `yaml:"verified,omitempty"`

	// OKF Attested Computation (section 10). Only the keys okfw reads are
	// typed: runtime for the business rule, and the three that hold a path
	// (section 6.2), which are edges in the link graph. The rest pass through.
	Runtime     string       `yaml:"runtime,omitempty"`
	Computation string       `yaml:"computation,omitempty"`
	Executor    *ResourceRef `yaml:"executor,omitempty"`
	Attester    *ResourceRef `yaml:"attester,omitempty"`

	// OKF provenance
	Sources     SourceList   `yaml:"sources,omitempty"`
	UsageWindow *UsageWindow `yaml:"usage_window,omitempty"`

	// okfw extensions
	ID        string    `yaml:"id"`
	Kind      string    `yaml:"kind,omitempty"` // "note" (default), "daily-note", "issue"
	Project   string    `yaml:"project,omitempty"`
	Parent    string    `yaml:"parent,omitempty"`
	Created   time.Time `yaml:"created"`
	Timestamp time.Time `yaml:"timestamp"` // okfw's last-write time; not generated.at

	// X is the open extension block: user-defined metadata, passed through
	// untouched. okfw claims no keys inside it.
	X map[string]interface{} `yaml:"x,omitempty"`
}

// GetKind returns the structured concept kind, defaulting to "note".
// For backward compatibility with legacy notes that stored the kind in the
// `type` field (pre-OKF), it falls back to a recognized `type` value.
func (z *Concept) GetKind() string {
	if z.Kind != "" {
		return z.Kind
	}
	switch z.Type {
	case "note", "daily-note", "issue":
		return z.Type
	}
	return "note"
}

// LifecycleStatus returns the OKF lifecycle status as written, "stable" when
// absent (OKF section 5.4). An unknown value is returned raw — v0.3 says it is
// not stable and a consumer SHOULD show it — so callers compare against the
// three known values rather than assume one of them.
func (z *Concept) LifecycleStatus() string {
	s := strings.TrimSpace(z.Status)
	if s == "" {
		return "stable"
	}
	return s
}

// IsKnownStatus reports whether s is one of the section 5.4 lifecycle values.
func IsKnownStatus(s string) bool {
	return s == "draft" || s == "stable" || s == "deprecated"
}

// IsCurrent reports whether the concept counts as current knowledge (section
// 5.4): neither deprecated nor carrying a status outside the vocabulary.
func (z *Concept) IsCurrent() bool {
	s := z.LifecycleStatus()
	return s == "draft" || s == "stable"
}

// TrustTier derives the trust tier from the current `verified` events (OKF
// v0.3 section 5.3), keyed strictly on the "human:" actor scheme. An event is
// current when its `at` is at or after generated.at, or when there is no
// `generated`; an event missing `by` or `at` is ignored (section 5.2). After a
// rewrite the earlier reviews stay in the file as history but no longer count:
// the human did not read the new text.
func (z *Concept) TrustTier() string {
	tier := TrustUnverified
	for _, v := range z.CurrentVerifications() {
		if IsHumanActor(v.By) {
			return TrustHumanReviewed
		}
		tier = TrustMachineConfirmed
	}
	return tier
}

// CurrentVerifications returns the verification events that count toward the
// trust tier (section 5.3). When generated.at is present but cannot be read,
// no event can be shown to be current, so none is: the tier can only go down,
// which is the safe direction. A `generated` block with no `at` records no
// change time to compare against, so every complete event counts.
func (z *Concept) CurrentVerifications() []Verification {
	var baseline time.Time
	if z.Generated != nil && z.Generated.At != "" {
		t, err := ParseTimestamp(z.Generated.At)
		if err != nil {
			return nil
		}
		baseline = t
	}
	var out []Verification
	for _, v := range z.Verified {
		if strings.TrimSpace(v.By) == "" || v.At == "" {
			continue
		}
		at, err := ParseTimestamp(v.At)
		if err != nil {
			continue
		}
		if !baseline.IsZero() && at.Before(baseline) {
			continue
		}
		out = append(out, v)
	}
	return out
}

// LastVerifiedAt returns the most recent verification time, or the zero time
// when there is none. "How recently" is the latest `at` (OKF section 5.2).
func (z *Concept) LastVerifiedAt() time.Time {
	var latest time.Time
	for _, v := range z.Verified {
		t, err := ParseTimestamp(v.At)
		if err != nil {
			continue
		}
		if t.After(latest) {
			latest = t
		}
	}
	return latest
}

// IsStale reports whether the concept is stale at now (OKF section 5.5).
func (z *Concept) IsStale(now time.Time) bool { return IsStale(z.StaleAfter, now) }

// ParseFrontmatter extracts and parses YAML frontmatter from markdown content.
// Frontmatter must be enclosed between "---" markers at the start of the file.
//
// It is lenient by design, because every read path goes through it: a field
// whose value has the wrong shape (`tags: foo`, an unparseable `created`) is
// left at its zero value instead of failing the whole concept. Failing used to
// make the concept vanish from search, the graph, the listings and id lookup,
// with no warning — so it could not even be found to be fixed. Strictness lives
// in ParseAndValidate, which `okfw validate` and every writer use. Unparseable
// YAML, or frontmatter that is not a mapping, is still an error.
func ParseFrontmatter(content []byte) (*Concept, error) {
	frontmatter, err := ExtractFrontmatter(content)
	if err != nil {
		return nil, err
	}

	var z Concept
	if err := yaml.Unmarshal(frontmatter, &z); err == nil {
		return &z, nil
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(frontmatter, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return &Concept{}, nil
	}
	m := doc.Content[0]
	if m.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("failed to parse frontmatter: not a mapping")
	}
	// Decode one key at a time, so a bad value costs only its own field.
	z = Concept{}
	for i := 0; i+1 < len(m.Content); i += 2 {
		one := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: m.Content[i : i+2]}
		_ = one.Decode(&z)
	}
	return &z, nil
}

// IsConcept reports whether parsed frontmatter makes a file a concept: OKF
// v0.3 section 3.2 says a .md file with no frontmatter, or no non-empty
// `type`, is not one, and consumers skip it. That is what lets a bundle hold a
// README.md or a SKILL.md.
func IsConcept(z *Concept) bool { return z != nil && strings.TrimSpace(z.Type) != "" }

// ConceptKey identifies a concept in a bundle: its frontmatter id, or, for a
// concept without one, its OKF concept ID (section 2) — the bundle-relative path
// minus ".md". okfw requires an id of the concepts it writes, but a bundle can
// hold concepts it did not write, and the spec identifies those by path.
func ConceptKey(z *Concept, bundleRel string) string {
	if z.ID != "" {
		return z.ID
	}
	rel := strings.TrimPrefix(filepath.ToSlash(bundleRel), "/")
	if strings.HasSuffix(strings.ToLower(rel), ".md") {
		rel = rel[:len(rel)-3]
	}
	return rel
}

// ExtractFrontmatter extracts raw YAML frontmatter bytes from markdown content.
func ExtractFrontmatter(content []byte) ([]byte, error) {
	// Check for opening delimiter
	if !bytes.HasPrefix(content, []byte("---")) {
		return nil, fmt.Errorf("frontmatter must start with ---")
	}

	// Find the closing delimiter
	rest := content[3:]
	// Skip newline after opening ---
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}

	endIdx := bytes.Index(rest, []byte("\n---"))
	if endIdx == -1 {
		// Try with \r\n
		endIdx = bytes.Index(rest, []byte("\r\n---"))
		if endIdx == -1 {
			return nil, fmt.Errorf("frontmatter closing --- not found")
		}
	}

	return rest[:endIdx], nil
}

// ExtractBody returns the markdown body following the frontmatter block.
// Content without a frontmatter block is returned unchanged; content whose
// frontmatter never closes yields an empty body. Handles both LF and CRLF
// line endings.
func ExtractBody(content []byte) string {
	if !bytes.HasPrefix(content, []byte("---")) {
		return string(content)
	}

	rest := content[3:]
	// Skip the line ending after the opening ---
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}

	endIdx := bytes.Index(rest, []byte("\n---"))
	if endIdx == -1 {
		return ""
	}

	body := rest[endIdx+4:]
	// Skip the line ending after the closing ---
	if len(body) > 1 && body[0] == '\r' && body[1] == '\n' {
		body = body[2:]
	} else if len(body) > 0 && body[0] == '\n' {
		body = body[1:]
	}

	return string(body)
}

// ValidateBusinessRules checks the constraints CUE cannot express: fields that
// are conditionally required, and source ids that must be unique.
//
// These used to live in a ValidateConcept function with no callers anywhere —
// production or test — so none of them ran. They are now part of
// ParseAndValidate, which every validation path goes through.
func ValidateBusinessRules(z *Concept) error {
	// OKF section 10.2: runtime is "REQUIRED for this type". A CUE comprehension
	// on type cannot be checked while the schema's type is still abstract.
	if z.Type == "Attested Computation" && strings.TrimSpace(z.Runtime) == "" {
		return fmt.Errorf("an Attested Computation requires runtime (OKF section 10.2)")
	}
	// sources[].id is the join key for [^id] footnotes (OKF section 5.1). Two
	// entries sharing one leave a citation resolvable to either, which is the
	// silent misattribution keyed labels exist to prevent. Compared the way
	// markdown compares footnote labels, case-insensitively: [^GA4] and [^ga4]
	// are one footnote, so ids differing only in case cannot be cited apart.
	firstWithID := make(map[string]int)
	for i, s := range z.Sources {
		if strings.TrimSpace(s.Resource) == "" {
			return fmt.Errorf("sources[%d]: resource is required within an entry", i)
		}
		if s.ID == "" {
			continue
		}
		if j, dup := firstWithID[graph.FootnoteKey(s.ID)]; dup {
			return fmt.Errorf("sources[%d] and sources[%d] share id %q; a [^%s] footnote cannot tell them apart",
				j, i, s.ID, s.ID)
		}
		firstWithID[graph.FootnoteKey(s.ID)] = i
	}
	// generated.at no longer mirrors timestamp. In v0.3 it is the last
	// *meaningful* change, which a tag edit or link repair must not move, while
	// timestamp records every write; they are allowed to differ.
	return nil
}

// ConceptWarnings reports what OKF v0.3 lets a concept hold but okfw flags:
// values a consumer ignores or reads in a way the author probably did not
// mean. None of them makes the concept invalid.
func ConceptWarnings(z *Concept) []string {
	var w []string
	if z.SupersededBy != "" && z.LifecycleStatus() != "deprecated" {
		w = append(w, "superseded_by is set but status is not deprecated; it has a meaning only for a deprecated concept (OKF section 5.4)")
	}
	if s := z.LifecycleStatus(); !IsKnownStatus(s) {
		w = append(w, fmt.Sprintf("status %q is not draft, stable or deprecated; consumers treat the concept as not current (OKF section 5.4)", s))
	}
	if IsMalformedTimestamp(z.StaleAfter) {
		w = append(w, fmt.Sprintf("stale_after %q does not parse, so the concept reads as stale (OKF section 5.5)", z.StaleAfter))
	}
	for i, v := range z.Verified {
		if strings.TrimSpace(v.By) == "" || v.At == "" {
			w = append(w, fmt.Sprintf("verified[%d] lacks by or at and is ignored (OKF section 5.2)", i))
		}
	}
	if len(z.Verified) > 0 && len(z.CurrentVerifications()) == 0 && z.Generated != nil && z.Generated.At != "" {
		w = append(w, "every verified event predates generated.at, so none is current and the concept is unverified (OKF section 5.3)")
	}
	for i, s := range z.Sources {
		if s.Author != "" && ValidateActor(s.Author) != nil {
			w = append(w, fmt.Sprintf("sources[%d].author %q does not follow the actor convention (OKF section 7)", i, s.Author))
		}
	}
	return w
}

// ValidateCitations rejects a footnote reference that resolves to nothing: a
// [^label] in body matching neither a sources[].id nor a [^label]: definition.
// Such a reference attributes its claim to no source and renders as literal
// text. A reference with a definition but no source passes, because it cannot
// be told apart from an ordinary footnote.
func ValidateCitations(z *Concept, body string) error {
	refs, defs := graph.Footnotes(body)
	if len(refs) == 0 {
		return nil
	}
	known := make(map[string]bool)
	for _, d := range defs {
		known[graph.FootnoteKey(d)] = true
	}
	for _, s := range z.Sources {
		if s.ID != "" {
			known[graph.FootnoteKey(s.ID)] = true
		}
	}
	var missing []string
	for _, r := range refs {
		if !known[graph.FootnoteKey(r)] {
			missing = append(missing, "[^"+r+"]")
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("footnote %s cites nothing: no sources[].id or definition matches it "+
		"(add the source with `okfw add-source --id <label>` or the update tool's add_source action, "+
		"or define the footnote)",
		strings.Join(missing, ", "))
}

// ParseAndValidate extracts frontmatter from content, validates it against the
// CUE schema, and applies the business rules.
func ParseAndValidate(content []byte) (*Concept, error) {
	frontmatter, err := ExtractFrontmatter(content)
	if err != nil {
		return nil, err
	}

	if err := checkDuplicateKeys(frontmatter); err != nil {
		return nil, err
	}

	// Before the schema, which reports a bad actor inside verified's
	// mapping-or-list disjunction only as "empty disjunction", and one in
	// generated.by by printing the regular expression it failed.
	if err := checkActors(frontmatter); err != nil {
		return nil, err
	}

	// Validate raw YAML against schema
	if err := ValidateConceptYAML(frontmatter); err != nil {
		return nil, err
	}

	// Parse into struct
	var z Concept
	if err := yaml.Unmarshal(frontmatter, &z); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	if err := ValidateBusinessRules(&z); err != nil {
		return nil, err
	}
	if err := ValidateCitations(&z, ExtractBody(content)); err != nil {
		return nil, err
	}

	return &z, nil
}
