package config

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// actorRe is the OKF section 7 actor convention, for the identity fields
// generated.by and verified[].by: human:<id>, process:<id>, or
// <producer>/<version>. Keep it in step with #Actor in concept-schema.cue;
// TestActorConventionAgreesWithTheSchema runs one table through both.
//
// It exists because the trust tier keys on the exact prefix "human:" (section
// 5.3), so a typo cost a human review its standing without a word: humna:alice,
// Human:alice and a bare "alice" all read as machine-confirmed. An id may hold
// inner spaces (a Windows login can) but not surrounding ones, and a version
// may hold a colon (ollama/llama3:8b). A producer may not, so a misspelt
// prefix cannot pass as one.
var actorRe = regexp.MustCompile(`^(human|process):\S(.*\S)?$|^[^\s/:]+/\S(.*\S)?$`)

// ValidateActor reports whether s follows the actor convention.
func ValidateActor(s string) error {
	if actorRe.MatchString(s) {
		return nil
	}
	hint := ""
	if p, _, ok := strings.Cut(s, ":"); ok && strings.EqualFold(strings.TrimSpace(p), "human") {
		hint = "; the trust tier keys on the exact lowercase prefix human:"
	}
	return fmt.Errorf("actor %q is not human:<id>, process:<id> or <producer>/<version> (OKF section 7)%s", s, hint)
}

// checkActors applies ValidateActor to generated.by and each verified[].by,
// naming the field that failed. Anything it cannot decode, or an absent by, is
// left for the schema to report.
func checkActors(frontmatter []byte) error {
	var doc struct {
		Generated *Generated   `yaml:"generated"`
		Verified  VerifiedList `yaml:"verified"`
	}
	if yaml.Unmarshal(frontmatter, &doc) != nil {
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
