package config

import (
	"strings"
	"testing"
	"time"
)

// concept wraps frontmatter in a minimally valid concept.
func concept(extra string) []byte {
	return []byte(`---
id: "019680f0-7e00-7abc-8def-1234567890ab"
title: "T"
type: "note"
tags: []
created: "2026-07-01T10:00:00Z"
timestamp: "2026-07-01T10:00:00Z"
` + extra + `---

# T
`)
}

// --- lifecycle (OKF section 5.4) ---

func TestLifecycleStatusDefaultsToStable(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"", "stable"},
		{"status: \"draft\"\n", "draft"},
		{"status: \"stable\"\n", "stable"},
		{"status: \"deprecated\"\n", "deprecated"},
	} {
		z, err := ParseAndValidate(concept(tc.raw))
		if err != nil {
			t.Fatalf("%q: %v", tc.raw, err)
		}
		if got := z.LifecycleStatus(); got != tc.want {
			t.Errorf("%q: LifecycleStatus() = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

// The removed todo feature's values are no longer a valid lifecycle status.
func TestLegacyTodoStatusIsRejected(t *testing.T) {
	for _, v := range []string{"open", "in_progress", "closed"} {
		if _, err := ParseAndValidate(concept("status: \"" + v + "\"\n")); err == nil {
			t.Errorf("status %q should be rejected; run okfw migrate", v)
		}
	}
}

func TestStaleAfter(t *testing.T) {
	today := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{"", false},
		{"stale_after: \"2026-07-24\"\n", true},
		{"stale_after: \"2026-07-25\"\n", true}, // inclusive on the boundary day
		{"stale_after: \"2026-07-26\"\n", false},
	} {
		z, err := ParseAndValidate(concept(tc.raw))
		if err != nil {
			t.Fatalf("%q: %v", tc.raw, err)
		}
		if got := z.IsStale(today); got != tc.want {
			t.Errorf("%q: IsStale = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

func TestStaleAfterRejectsNonDates(t *testing.T) {
	if _, err := ParseAndValidate(concept("stale_after: \"not-a-date\"\n")); err == nil {
		t.Error("stale_after must be YYYY-MM-DD")
	}
	// A datetime in the field still resolves rather than erroring at read time.
	z := &Concept{StaleAfter: "2026-07-24T12:00:00Z"}
	if !z.IsStale(time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)) {
		t.Error("a full datetime in stale_after should still compare")
	}
}

// --- trust (OKF sections 5.2-5.3) ---

// A single bare mapping counts as a one-element list. OKF makes this a MUST in
// both section 5.2 and section 11.
func TestVerifiedBareMappingNormalizesToOneElementList(t *testing.T) {
	z, err := ParseAndValidate(concept(`verified:
  by: "human:ryan"
  at: "2026-07-20T10:00:00Z"
`))
	if err != nil {
		t.Fatalf("bare verified mapping must be accepted: %v", err)
	}
	if len(z.Verified) != 1 {
		t.Fatalf("got %d entries, want 1: %+v", len(z.Verified), z.Verified)
	}
	if z.Verified[0].By != "human:ryan" {
		t.Errorf("by = %q", z.Verified[0].By)
	}
	if z.TrustTier() != TrustHumanReviewed {
		t.Errorf("tier = %q, want %q", z.TrustTier(), TrustHumanReviewed)
	}
}

func TestVerifiedList(t *testing.T) {
	z, err := ParseAndValidate(concept(`verified:
  - by: "reference_agent/gemini-2.5-pro"
    at: "2026-07-18T10:00:00Z"
  - by: "human:ryan"
    at: "2026-07-20T10:00:00Z"
`))
	if err != nil {
		t.Fatalf("verified list: %v", err)
	}
	if len(z.Verified) != 2 {
		t.Fatalf("got %d entries", len(z.Verified))
	}
	// "How recently" is the latest at (OKF section 5.2).
	want := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	if got := z.LastVerifiedAt(); !got.Equal(want) {
		t.Errorf("LastVerifiedAt = %v, want %v", got, want)
	}
}

func TestTrustTier(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{"absent", "", TrustUnverified},
		{"machine only", "verified:\n  - by: \"process:nightly\"\n    at: \"2026-07-20T10:00:00Z\"\n", TrustMachineConfirmed},
		{"agent only", "verified:\n  - by: \"reference_agent/gemini\"\n    at: \"2026-07-20T10:00:00Z\"\n", TrustMachineConfirmed},
		{"human anywhere", "verified:\n  - by: \"process:nightly\"\n    at: \"2026-07-19T10:00:00Z\"\n  - by: \"human:ryan\"\n    at: \"2026-07-20T10:00:00Z\"\n", TrustHumanReviewed},
		// The reference bundles use team:, which is deliberately NOT human.
		{"team is not human", "verified:\n  - by: \"team:data-platform\"\n    at: \"2026-07-20T10:00:00Z\"\n", TrustMachineConfirmed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			z, err := ParseAndValidate(concept(tc.raw))
			if err != nil {
				t.Fatalf("%v", err)
			}
			if got := z.TrustTier(); got != tc.want {
				t.Errorf("TrustTier = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGeneratedAtMustAgreeWithTimestamp(t *testing.T) {
	if _, err := ParseAndValidate(concept(`generated:
  by: "okfw/0.2"
  at: "2026-07-01T10:00:00Z"
`)); err != nil {
		t.Errorf("matching generated.at should validate: %v", err)
	}
	// The comparison is on the instant, not the text, so a different offset
	// spelling of the same moment still agrees.
	if _, err := ParseAndValidate(concept(`generated:
  by: "okfw/0.2"
  at: "2026-07-01T12:00:00+02:00"
`)); err != nil {
		t.Errorf("same instant in another offset should validate: %v", err)
	}
	if _, err := ParseAndValidate(concept(`generated:
  by: "okfw/0.2"
  at: "2026-07-02T10:00:00Z"
`)); err == nil {
		t.Error("generated.at disagreeing with timestamp should be rejected")
	}
	// generated.by alone is fine — at is optional.
	if _, err := ParseAndValidate(concept("generated:\n  by: \"okfw/0.2\"\n")); err != nil {
		t.Errorf("generated without at should validate: %v", err)
	}
}

func TestGeneratedRequiresBy(t *testing.T) {
	if _, err := ParseAndValidate(concept("generated:\n  at: \"2026-07-01T10:00:00Z\"\n")); err == nil {
		t.Error("generated.by is required within generated")
	}
}

// --- provenance (OKF section 5.1) ---

func TestSources(t *testing.T) {
	z, err := ParseAndValidate(concept(`usage_window:
  from: "2026-01-01"
  to: "2026-06-30"
sources:
  - resource: "https://example.com/schema"
    id: "ga4-schema"
    title: "GA4 export schema"
    author: "team:ga4-docs"
    usage_count: 412
    last_modified: "2026-05-01"
  - resource: "all queries in project X"
`))
	if err != nil {
		t.Fatalf("sources: %v", err)
	}
	if len(z.Sources) != 2 {
		t.Fatalf("got %d sources", len(z.Sources))
	}
	if z.Sources[0].ID != "ga4-schema" || z.Sources[0].UsageCount == nil || *z.Sources[0].UsageCount != 412 {
		t.Errorf("first source = %+v", z.Sources[0])
	}
	// A scope descriptor is a legal resource, not a path.
	if z.Sources[1].Resource != "all queries in project X" {
		t.Errorf("second source = %+v", z.Sources[1])
	}
	if z.UsageWindow == nil || z.UsageWindow.From != "2026-01-01" {
		t.Errorf("usage_window = %+v", z.UsageWindow)
	}
}

func TestSourceRequiresResource(t *testing.T) {
	_, err := ParseAndValidate(concept("sources:\n  - id: \"no-resource\"\n"))
	if err == nil {
		t.Fatal("a source entry without resource must be rejected")
	}
	if !strings.Contains(err.Error(), "resource") {
		t.Errorf("error should name the missing field, got: %v", err)
	}
}

// --- the x: escape hatch ---

// The schema is closed, so this is the only way to attach custom metadata.
func TestExtensionBlockAcceptsArbitraryKeys(t *testing.T) {
	z, err := ParseAndValidate(concept(`x:
  anything: "goes"
  nested:
    deeper: 42
  a_list:
    - 1
    - 2
`))
	if err != nil {
		t.Fatalf("the x: block must accept arbitrary keys: %v", err)
	}
	if z.X["anything"] != "goes" {
		t.Errorf("x.anything = %v", z.X["anything"])
	}
	if _, ok := z.X["nested"]; !ok {
		t.Error("nested extension data should survive parsing")
	}
}

// Outside x:, the closed schema still catches typos — that is the trade the
// divergence buys.
func TestUndeclaredTopLevelKeyIsRejected(t *testing.T) {
	for _, typo := range []string{"descrption", "tag", "statuss", "todo_status"} {
		if _, err := ParseAndValidate(concept(typo + ": \"x\"\n")); err == nil {
			t.Errorf("undeclared top-level key %q should be rejected", typo)
		}
	}
}

// --- kind ---

func TestKindNoLongerAcceptsTodo(t *testing.T) {
	if _, err := ParseAndValidate(concept("kind: \"todo\"\n")); err == nil {
		t.Error("kind: todo was removed and should be rejected")
	}
	for _, k := range []string{"note", "daily-note", "issue"} {
		if _, err := ParseAndValidate(concept("kind: \"" + k + "\"\n")); err != nil {
			t.Errorf("kind %q should be valid: %v", k, err)
		}
	}
}

func TestGetKindFallsBackToType(t *testing.T) {
	// Legacy notes stored the kind in `type`.
	z := &Concept{Type: "daily-note"}
	if got := z.GetKind(); got != "daily-note" {
		t.Errorf("GetKind = %q", got)
	}
	if !z.IsDailyNote() {
		t.Error("IsDailyNote should be true")
	}
	// A free-form type is not a kind.
	z = &Concept{Type: "BigQuery Table"}
	if got := z.GetKind(); got != "note" {
		t.Errorf("GetKind = %q, want the note default", got)
	}
}
