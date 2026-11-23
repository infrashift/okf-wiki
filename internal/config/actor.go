package config

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// actorRe is the OKF v0.3 section 7 actor convention, for the identity fields
// generated.by and verified[].by: <scheme>:<id> or <producer>/<version>.
// Section 7 defines the schemes human:, team: and process:, and lets producers
// use others, which consumers MUST accept and treat as non-human. Keep it in
// step with #Actor in concept-schema.cue; TestActorConventionAgreesWithTheSchema
// runs one table through both.
//
// The scheme must be lowercase. The trust tier keys on the exact prefix
// "human:" (section 5.3), so Human:alice would read as some other, non-human
// scheme and cost a review its standing without a word. An id may hold inner
// spaces (a Windows login can) but not surrounding ones, and a version may hold
// a colon (ollama/llama3:8b). A producer may not, so a scheme cannot pass as
// one.
var actorRe = regexp.MustCompile(`^[a-z][a-z0-9+.-]*:\S(.*\S)?$|^[^\s/:]+/\S(.*\S)?$`)

// ValidateActor reports whether s follows the actor convention, as okfw
// enforces it on write and in strict validation. Beyond the shape, it refuses a
// scheme that is a near miss of "human" (humna:, huma:, humans:): v0.3 makes
// any unknown scheme a valid non-human actor, so a typo would no longer fail
// the shape check — it would silently demote a human review instead. That guard
// is Go-only; CUE has no edit distance, so ParseAndValidate runs this before
// the schema.
func ValidateActor(s string) error {
	scheme, _, hasColon := strings.Cut(s, ":")
	if hasColon && strings.EqualFold(strings.TrimSpace(scheme), "human") && scheme != "human" {
		return fmt.Errorf("actor %q: the trust tier keys on the exact lowercase prefix human: (OKF section 7)", s)
	}
	if !actorRe.MatchString(s) {
		return fmt.Errorf("actor %q is not <scheme>:<id> (human:, team:, process:, …) or <producer>/<version> (OKF section 7)", s)
	}
	if hasColon && !strings.Contains(scheme, "/") && scheme != "human" && levenshtein(scheme, "human") <= 2 {
		return fmt.Errorf("actor %q: scheme %q looks like a misspelt human:, which would read as a non-human actor (OKF section 7)", s, scheme)
	}
	return nil
}

// IsHumanActor reports whether an actor gives the human-reviewed tier: only
// the exact human: scheme does (OKF section 7).
func IsHumanActor(s string) bool { return strings.HasPrefix(s, HumanActorPrefix) }

func levenshtein(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// checkActors applies ValidateActor to generated.by and each verified[].by,
// naming the field that failed. Anything it cannot decode, or an absent by, is
// left for the schema to report. sources[].author also follows section 7, but
// okfw only warns about it (ActorWarnings): the spec's own example bundles
// hold free-form authors.
func checkActorsNode(m *yaml.Node) error {
	if m == nil {
		return nil
	}
	var doc struct {
		Generated *Generated   `yaml:"generated"`
		Verified  VerifiedList `yaml:"verified"`
	}
	if m.Decode(&doc) != nil {
		return nil
	}
	if g := doc.Generated; g != nil && g.By != "" {
		if err := ValidateActor(g.By); err != nil {
			return fmt.Errorf("generated.by: %w", err)
		}
	}
	for i, v := range doc.Verified {
		if v.By == "" {
			continue
		}
		if err := ValidateActor(v.By); err != nil {
			return fmt.Errorf("verified[%d].by: %w", i, err)
		}
	}
	return nil
}
