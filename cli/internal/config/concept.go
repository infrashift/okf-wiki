package config

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Trust tiers derived from `verified` (OKF v0.2 section 5.3).
const (
	TrustUnverified       = "unverified"
	TrustMachineConfirmed = "machine-confirmed"
	TrustHumanReviewed    = "human-reviewed"
)

// HumanActorPrefix marks a hand-authored or human-confirmed actor. The trust
// tier keys off this prefix and nothing else: "team:..." is *not* human.
const HumanActorPrefix = "human:"

// Generated records who wrote the concept and when (OKF section 5.2).
type Generated struct {
	By string `yaml:"by"`
	At string `yaml:"at,omitempty"` // ISO 8601 datetime
}

// Verification is one verification event (OKF section 5.2).
type Verification struct {
	By string `yaml:"by"`
	At string `yaml:"at,omitempty"` // ISO 8601 datetime
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
		if value.Tag == "!!null" {
			*v = nil
			return nil
		}
	}
	return fmt.Errorf("verified must be a mapping or a list of mappings")
}

// UsageWindow frames usage_count values with a date range (OKF section 5.1).
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
	Type        string   `yaml:"type"` // OKF required: free-form concept kind
	Title       string   `yaml:"title"`
	Description string   `yaml:"description,omitempty"` // one-sentence summary
	Resource    string   `yaml:"resource,omitempty"`    // URI of the underlying asset
	Tags        []string `yaml:"tags"`

	// OKF lifecycle. Absent means "stable".
	Status     string `yaml:"status,omitempty"`
	StaleAfter string `yaml:"stale_after,omitempty"`

	// OKF trust
	Generated *Generated   `yaml:"generated,omitempty"`
	Verified  VerifiedList `yaml:"verified,omitempty"`

	// OKF provenance
	Sources     []Source     `yaml:"sources,omitempty"`
	UsageWindow *UsageWindow `yaml:"usage_window,omitempty"`

	// okfw extensions
	ID        string    `yaml:"id"`
	Kind      string    `yaml:"kind,omitempty"` // "note" (default), "daily-note", "issue"
	Project   string    `yaml:"project,omitempty"`
	Parent    string    `yaml:"parent,omitempty"`
	Created   time.Time `yaml:"created"`
	Timestamp time.Time `yaml:"timestamp"` // mirrors generated.at

	// X is the open extension block: user-defined metadata, passed through
	// untouched. okfw claims no keys inside it.
	X map[string]interface{} `yaml:"x,omitempty"`

	// Graphy carries the graphy-export extension block; nil on ordinary concepts.
	Graphy *GraphyMeta `yaml:"graphy,omitempty"`
}

// GraphyMeta is the graphy extension block: the graph identity and provenance
// of an exported code concept.
type GraphyMeta struct {
	NodeID    string `yaml:"node_id"`
	Language  string `yaml:"language,omitempty"`
	Entity    string `yaml:"entity,omitempty"` // function, method, struct, community, ...
	Commit    string `yaml:"commit,omitempty"`
	Generator string `yaml:"generator,omitempty"`
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
	// A concept carrying a graphy block was written by `graphy export okfw`.
	// Classify it on read so bundles exported before `kind: code` existed are
	// still separable from hand-written notes, without rewriting a file.
	if z.Graphy != nil {
		return "code"
	}
	return "note"
}

// IsDailyNote returns true if this concept is a daily note.
func (z *Concept) IsDailyNote() bool { return z.GetKind() == "daily-note" }

// LifecycleStatus returns the OKF lifecycle status, defaulting to "stable"
// (OKF section 5.4: absent status means stable).
func (z *Concept) LifecycleStatus() string {
	switch z.Status {
	case "draft", "stable", "deprecated":
		return z.Status
	}
	return "stable"
}

// TrustTier derives the trust tier from `verified` (OKF section 5.3). Keyed
// strictly on the "human:" actor prefix.
func (z *Concept) TrustTier() string {
	if len(z.Verified) == 0 {
		return TrustUnverified
	}
	for _, v := range z.Verified {
		if strings.HasPrefix(v.By, HumanActorPrefix) {
			return TrustHumanReviewed
		}
	}
	return TrustMachineConfirmed
}

// LastVerifiedAt returns the most recent verification time, or the zero time
// when there is none. "How recently" is the latest `at` (OKF section 5.2).
func (z *Concept) LastVerifiedAt() time.Time {
	var latest time.Time
	for _, v := range z.Verified {
		t, err := time.Parse(time.RFC3339, v.At)
		if err != nil {
			continue
		}
		if t.After(latest) {
			latest = t
		}
	}
	return latest
}

// IsStale reports whether the concept is stale as of today: stale_after is an
// absolute date and the comparison is inclusive of the boundary day
// (OKF section 5.5). An absent or unparseable stale_after is never stale.
func (z *Concept) IsStale(today time.Time) bool {
	raw := z.StaleAfter
	if len(raw) > 10 {
		raw = raw[:10] // tolerate a full datetime
	}
	cutoff, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return false
	}
	y, m, d := today.Date()
	return !time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Before(cutoff)
}

// ParseFrontmatter extracts and parses YAML frontmatter from markdown content.
// Frontmatter must be enclosed between "---" markers at the start of the file.
func ParseFrontmatter(content []byte) (*Concept, error) {
	frontmatter, err := ExtractFrontmatter(content)
	if err != nil {
		return nil, err
	}

	var z Concept
	if err := yaml.Unmarshal(frontmatter, &z); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	return &z, nil
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
// are conditionally required, and the two timestamps that must agree.
//
// These used to live in a ValidateConcept function with no callers anywhere —
// production or test — so none of them ran. They are now part of
// ParseAndValidate, which every validation path goes through.
func ValidateBusinessRules(z *Concept) error {
	for i, s := range z.Sources {
		if strings.TrimSpace(s.Resource) == "" {
			return fmt.Errorf("sources[%d]: resource is required within an entry", i)
		}
	}
	// generated.at mirrors timestamp; a disagreement means one was hand-edited
	// and consumers would disagree about when the content last changed.
	if z.Generated != nil && z.Generated.At != "" && !z.Timestamp.IsZero() {
		at, err := time.Parse(time.RFC3339, z.Generated.At)
		if err != nil {
			return fmt.Errorf("generated.at is not an RFC 3339 datetime: %q", z.Generated.At)
		}
		if !at.Equal(z.Timestamp) {
			return fmt.Errorf("generated.at (%s) and timestamp (%s) disagree; they record the same event",
				at.Format(time.RFC3339), z.Timestamp.Format(time.RFC3339))
		}
	}
	return nil
}

// ParseAndValidate extracts frontmatter from content, validates it against the
// CUE schema, and applies the business rules.
func ParseAndValidate(content []byte) (*Concept, error) {
	frontmatter, err := ExtractFrontmatter(content)
	if err != nil {
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

	return &z, nil
}
