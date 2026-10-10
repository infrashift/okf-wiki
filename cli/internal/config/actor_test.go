package config

import (
	"strings"
	"testing"
)

var actorCases = []struct {
	actor string
	ok    bool
}{
	{"human:alice", true},
	{"human:John Smith", true}, // a Windows login may hold a space
	{`human:CORP\alice`, true},
	{"process:finance-nightly", true},
	{"reference_agent/gemini-2.5-pro", true},
	{"okfw/0.2", true},
	{"ollama/llama3:8b", true}, // a version may hold a colon

	{"humna:alice", false},
	{"Human:alice", false},
	{"HUMAN:alice", false},
	{"human:", false},
	{"human: alice", false},
	{"human:alice ", false},
	{"human:a\nb", false},
	{"process:", false},
	{"alice", false},
	{"team:data-platform", false},
	{"/v1", false},
	{"agent/", false},
	{"my agent/v1", false},
	{"humna:alice/x", false}, // a producer may not hold a colon
}

// TestActorConventionAgreesWithTheSchema runs one table through actorRe and
// through the CUE #Actor, which must agree: the writers check the first, and
// validate --all goes through the second.
func TestActorConventionAgreesWithTheSchema(t *testing.T) {
	for _, tc := range actorCases {
		goErr := ValidateActor(tc.actor)
		if (goErr == nil) != tc.ok {
			t.Errorf("ValidateActor(%q) = %v, want ok=%v", tc.actor, goErr, tc.ok)
		}
		fm, err := ExtractFrontmatter(concept("generated:\n  by: " + quoteYAML(tc.actor) + "\n"))
		if err != nil {
			t.Fatal(err)
		}
		if cueErr := ValidateConceptYAML(fm); (cueErr == nil) != tc.ok {
			t.Errorf("schema on %q: %v, want ok=%v", tc.actor, cueErr, tc.ok)
		}
	}
}

// TestBadActorIsNamedInEveryIdentityField: the schema alone reported a bad
// actor in verified as "empty disjunction", naming neither field nor value.
func TestBadActorIsNamedInEveryIdentityField(t *testing.T) {
	for _, tc := range []struct{ raw, field string }{
		{"generated:\n  by: \"humna:alice\"\n", "generated.by"},
		{"verified:\n  by: \"humna:alice\"\n  at: \"2026-07-20T10:00:00Z\"\n", "verified[0].by"},
		{"verified:\n  - by: \"human:bob\"\n  - by: \"humna:alice\"\n", "verified[1].by"},
	} {
		_, err := ParseAndValidate(concept(tc.raw))
		if err == nil || !strings.Contains(err.Error(), tc.field) || !strings.Contains(err.Error(), "humna:alice") {
			t.Errorf("%s: err = %v, want it to name the field and the value", tc.field, err)
		}
	}
	// sources[].author is free-form: the spec's own examples use team:.
	if _, err := ParseAndValidate(concept("sources:\n  - resource: \"x\"\n    author: \"team:ga4-docs\"\n")); err != nil {
		t.Errorf("team: author rejected: %v", err)
	}
}

func TestBadHumanPrefixGetsAHint(t *testing.T) {
	for _, a := range []string{"Human:alice", "HUMAN:alice", " human :alice"} {
		if err := ValidateActor(a); err == nil || !strings.Contains(err.Error(), "lowercase prefix human:") {
			t.Errorf("ValidateActor(%q) = %v, want the lowercase hint", a, err)
		}
	}
	if err := ValidateActor("humna:alice"); err == nil || strings.Contains(err.Error(), "lowercase") {
		t.Errorf("a misspelling is not a case problem: %v", err)
	}
}

func quoteYAML(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s) + `"`
}
