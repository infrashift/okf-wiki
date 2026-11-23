package config

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// concept wraps frontmatter in a minimally valid concept.
func concept(extra string) []byte {
	return []byte(`---
x-okfw-id: "019680f0-7e00-7abc-8def-1234567890ab"
title: "T"
type: "note"
tags: []
x-okfw-created: "2026-07-01T10:00:00Z"
x-okfw-timestamp: "2026-07-01T10:00:00Z"
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

// stale_after is an instant (OKF section 5.5): stale means now >= stale_after,
// compared across offsets, not a calendar day in whatever zone the reader is in.
func TestStaleAfterIsAnInstant(t *testing.T) {
	sydney := time.FixedZone("AEST", 10*3600)
	// 2026-07-25T10:00:00+10:00 is 2026-07-25T00:00:00Z.
	now := time.Date(2026, 7, 25, 10, 0, 0, 0, sydney)
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{"", false},
		{"stale_after: \"2026-07-24T23:59:59Z\"\n", true},
		{"stale_after: \"2026-07-25T00:00:00Z\"\n", true}, // inclusive at the instant
		{"stale_after: \"2026-07-25T00:00:01Z\"\n", false},
		{"stale_after: \"2026-07-25T09:59:59+10:00\"\n", true},
		{"stale_after: \"2026-07-24T20:00:01-04:00\"\n", false}, // 00:00:01Z
		{"stale_after: 2026-07-25T00:00:00Z\n", true},           // unquoted
		{"stale_after: \"2026-07-24T00:00:00.5Z\"\n", true},     // fractional seconds
	} {
		z, err := ParseAndValidate(concept(tc.raw))
		if err != nil {
			t.Fatalf("%q: %v", tc.raw, err)
		}
		if got := z.IsStale(now); got != tc.want {
			t.Errorf("%q: IsStale = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

// OKF v0.3 section 5: every OKF timestamp is a date-time with an offset or a
// full-date (the start of that day in UTC), quoted or YAML-native. A date-time
// without an offset, or anything else, is invalid.
func TestTimestampFieldsTakeTheTwoV03Forms(t *testing.T) {
	fields := map[string]string{
		"stale_after":             "stale_after: %s\n",
		"generated.at":            "generated:\n  by: \"human:me\"\n  at: %s\n",
		"verified.at":             "verified:\n  by: \"human:me\"\n  at: %s\n",
		"usage_window.from":       "usage_window:\n  from: %s\n",
		"sources.last_modified":   "sources:\n  - resource: \"x\"\n    last_modified: %s\n",
		"sources.usage_window.to": "sources:\n  - resource: \"x\"\n    usage_window:\n      to: %s\n",
	}
	for name, tmpl := range fields {
		for _, bad := range []string{
			`"2026-12-31T00:00:00"`,      // no offset
			`"2026-12-31T00:00:00Zjunk"`, // trailing garbage
			`"2026-12-31 00:00:00Z"`,     // space separator
			`"2026-12"`,                  // partial date
			`"not-a-date"`,
		} {
			if _, err := ParseAndValidate(concept(fmt.Sprintf(tmpl, bad))); err == nil {
				t.Errorf("%s = %s was accepted", name, bad)
			}
		}
		for _, good := range []string{
			`"2026-12-31T00:00:00Z"`, `"2026-12-31T08:30:00+05:30"`, `2026-12-31T00:00:00.123Z`,
			`"2026-12-31"`, `2026-12-31`, // full-date, quoted and YAML-native
		} {
			if _, err := ParseAndValidate(concept(fmt.Sprintf(tmpl, good))); err != nil {
				t.Errorf("%s = %s was rejected: %v", name, good, err)
			}
		}
	}
}

// okfw's own created and timestamp keep the stricter date-time rule.
func TestOkfwTimestampsStillRequireADateTime(t *testing.T) {
	bad := strings.Replace(string(concept("")), `created: "2026-07-01T10:00:00Z"`, `created: "2026-07-01"`, 1)
	if _, err := ParseAndValidate([]byte(bad)); err == nil {
		t.Error("created as a bare date was accepted")
	}
}

// A full-date means the start of that day in UTC, wherever the reader is.
func TestDateOnlyStaleAfterIsUTCMidnight(t *testing.T) {
	z, err := ParseAndValidate(concept("stale_after: 2026-07-25\n"))
	if err != nil {
		t.Fatal(err)
	}
	if z.IsStale(time.Date(2026, 7, 24, 23, 59, 59, 0, time.UTC)) {
		t.Error("stale one second before UTC midnight")
	}
	if !z.IsStale(time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)) {
		t.Error("not stale at UTC midnight")
	}
}

func TestNormalizeDateTime(t *testing.T) {
	for in, want := range map[string]string{
		"2026-12-31":                "2026-12-31T00:00:00Z",
		"2026-12-31T00:00:00Z":      "2026-12-31T00:00:00Z",
		"2026-12-31T08:30:00+05:30": "2026-12-31T08:30:00+05:30", // kept as written
	} {
		if got, err := NormalizeDateTime(in); err != nil || got != want {
			t.Errorf("NormalizeDateTime(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"2026-13-01", "2026-12-31T00:00:00", "tomorrow", ""} {
		if got, err := NormalizeDateTime(bad); err == nil {
			t.Errorf("NormalizeDateTime(%q) = %q, want an error", bad, got)
		}
	}
}

// OKF v0.3 section 5.5: a consumer MUST NOT treat a stale_after it cannot
// parse as fresh. Empty means absent, which is never stale.
func TestMalformedStaleAfterIsStale(t *testing.T) {
	now := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC) // long before every value
	for _, v := range []string{"2026-01-01T00:00:00", "garbage", "2026-13-45"} {
		if !IsStale(v, now) {
			t.Errorf("IsStale(%q) = false; an unparseable value must not read as fresh", v)
		}
	}
	if IsStale("", now) {
		t.Error("an empty stale_after is absent, never stale")
	}
	if IsStale("2026-01-01", now) {
		t.Error("a valid future date reads as stale")
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
		// Not an actor okfw will write or validate, but a third-party concept
		// may carry one, and reading it must not promote it to human.
		{"team is not human", "verified:\n  - by: \"team:data-platform\"\n    at: \"2026-07-20T10:00:00Z\"\n", TrustMachineConfirmed},
		{"capitalised Human is not human", "verified:\n  - by: \"Human:ryan\"\n    at: \"2026-07-20T10:00:00Z\"\n", TrustMachineConfirmed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Lenient, as every read is: trust tier is a read-side derivation.
			z, err := ParseFrontmatter(concept(tc.raw))
			if err != nil {
				t.Fatalf("%v", err)
			}
			if got := z.TrustTier(); got != tc.want {
				t.Errorf("TrustTier = %q, want %q", got, tc.want)
			}
		})
	}
}

// v0.3 section 5.2: generated.at is the last meaningful change and timestamp
// is okfw's last write; they may differ.
func TestGeneratedAtNeedNotMatchTimestamp(t *testing.T) {
	if _, err := ParseAndValidate(concept(`generated:
  by: "github.com/infrashift/okf-wiki/0.2"
  at: "2026-06-02T10:00:00Z"
`)); err != nil {
		t.Errorf("generated.at older than timestamp should validate: %v", err)
	}
	if _, err := ParseAndValidate(concept("generated:\n  by: \"github.com/infrashift/okf-wiki/0.2\"\n")); err != nil {
		t.Errorf("generated without at should validate: %v", err)
	}
}

// v0.3 section 5.3: only current events count. An event before generated.at
// is history: the reviewer did not read the rewritten text.
func TestTrustTierCountsOnlyCurrentEvents(t *testing.T) {
	for _, tc := range []struct {
		name, raw, want string
	}{
		{"review after rewrite", "generated:\n  by: \"a/1\"\n  at: \"2026-07-01T00:00:00Z\"\nverified:\n  - by: \"human:r\"\n    at: \"2026-07-02T00:00:00Z\"\n", TrustHumanReviewed},
		{"review at the same instant", "generated:\n  by: \"a/1\"\n  at: \"2026-07-01T00:00:00Z\"\nverified:\n  - by: \"human:r\"\n    at: \"2026-07-01T00:00:00Z\"\n", TrustHumanReviewed},
		{"review before rewrite", "generated:\n  by: \"a/1\"\n  at: \"2026-07-03T00:00:00Z\"\nverified:\n  - by: \"human:r\"\n    at: \"2026-07-02T00:00:00Z\"\n", TrustUnverified},
		{"stale human, current machine", "generated:\n  by: \"a/1\"\n  at: \"2026-07-03T00:00:00Z\"\nverified:\n  - by: \"human:r\"\n    at: \"2026-07-02T00:00:00Z\"\n  - by: \"process:n\"\n    at: \"2026-07-04T00:00:00Z\"\n", TrustMachineConfirmed},
		{"no generated: all count", "verified:\n  - by: \"human:r\"\n    at: \"2020-01-01T00:00:00Z\"\n", TrustHumanReviewed},
		{"event without at is ignored", "verified:\n  - by: \"human:r\"\n", TrustUnverified},
		{"unreadable generated.at: nothing is current", "generated:\n  by: \"a/1\"\n  at: \"soon\"\nverified:\n  - by: \"human:r\"\n    at: \"2026-07-02T00:00:00Z\"\n", TrustUnverified},
		{"date-only events compare as UTC midnight", "generated:\n  by: \"a/1\"\n  at: \"2026-07-02T00:00:00Z\"\nverified:\n  - by: \"human:r\"\n    at: 2026-07-02\n", TrustHumanReviewed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			z, err := ParseFrontmatter(concept(tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			if got := z.TrustTier(); got != tc.want {
				t.Errorf("TrustTier = %q, want %q", got, tc.want)
			}
		})
	}
}

// v0.3 section 5.2: by and at are REQUIRED in a verified event.
func TestVerifiedEventRequiresAt(t *testing.T) {
	if _, err := ParseAndValidate(concept("verified:\n  by: \"human:r\"\n")); err == nil {
		t.Error("a verified event without at was accepted")
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
  from: "2026-01-01T00:00:00Z"
  to: "2026-06-30T00:00:00Z"
sources:
  - resource: "https://example.com/schema"
    id: "ga4-schema"
    title: "GA4 export schema"
    author: "team:ga4-docs"
    usage_count: 412
    last_modified: "2026-05-01T00:00:00Z"
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
	if z.UsageWindow == nil || z.UsageWindow.From != "2026-01-01T00:00:00Z" {
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

// TestSourceIDsMustBeUnique: sources[].id is the join key for [^id] footnotes,
// so two entries sharing one make a citation resolvable to either. Markdown
// matches footnote labels case-insensitively, so case does not separate them.
func TestSourceIDsMustBeUnique(t *testing.T) {
	for _, second := range []string{"ga4", "GA4"} {
		_, err := ParseAndValidate(concept("sources:\n" +
			"  - resource: \"https://a.example\"\n    id: \"ga4\"\n" +
			"  - resource: \"https://b.example\"\n    id: \"" + second + "\"\n"))
		if err == nil || !strings.Contains(err.Error(), "share id") {
			t.Errorf("ids ga4 and %s: err = %v, want a shared-id rejection", second, err)
		}
	}
	// Entries without an id do not collide with one another.
	if _, err := ParseAndValidate(concept("sources:\n" +
		"  - resource: \"https://a.example\"\n  - resource: \"https://b.example\"\n")); err != nil {
		t.Errorf("two id-less sources: %v", err)
	}
}

// citing returns a valid concept with the given sources block and body.
func citing(sources, body string) []byte {
	c := string(concept(sources))
	return []byte(c[:strings.Index(c, "---\n\n")+5] + body)
}

// TestFootnoteMustResolve: a [^label] matching neither a source nor a
// definition attributes its claim to nothing.
func TestFootnoteMustResolve(t *testing.T) {
	source := "sources:\n  - resource: \"https://example.com/schema\"\n    id: \"ga4-schema\"\n"
	for _, tc := range []struct {
		name, sources, body string
		ok                  bool
	}{
		{"cites a source", source, "Sharded daily.[^ga4-schema]\n", true},
		{"cites a source, other case", source, "Sharded daily.[^GA4-Schema]\n", true},
		{"ordinary footnote", "", "A claim.[^1]\n\n[^1]: An aside.\n", true},
		{"source and definition", source, "Sharded.[^ga4-schema]\n\n[^ga4-schema]: GA4 schema\n", true},
		{"example in code", "", "Use `[^a-z]` or:\n\n```\n[^x]\n```\n", true},
		{"cites nothing", "", "Sharded daily.[^ga4-schema]\n", false},
		{"source renamed away", source, "Sharded daily.[^ga4]\n", false},
		{"orphaned definition elsewhere", "", "A.[^a] B.[^b]\n\n[^a]: defined\n", false},
	} {
		_, err := ParseAndValidate(citing(tc.sources, tc.body))
		if tc.ok && err != nil {
			t.Errorf("%s: rejected: %v", tc.name, err)
		}
		if !tc.ok && (err == nil || !strings.Contains(err.Error(), "cites nothing")) {
			t.Errorf("%s: err = %v, want a cites-nothing rejection", tc.name, err)
		}
	}
	// The error names every unresolved label, and only those.
	_, err := ParseAndValidate(citing("", "A.[^a] B.[^b] C.[^c]\n\n[^b]: defined\n"))
	if err == nil || !strings.Contains(err.Error(), "[^a], [^c]") || strings.Contains(err.Error(), "[^b]") {
		t.Errorf("err = %v, want it to name [^a] and [^c] only", err)
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
	if _, err := ParseAndValidate(concept("x-okfw-kind: \"todo\"\n")); err == nil {
		t.Error("kind: todo was removed and should be rejected")
	}
	for _, k := range []string{"note", "daily-note", "issue"} {
		if _, err := ParseAndValidate(concept("x-okfw-kind: \"" + k + "\"\n")); err != nil {
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
	// A free-form type is not a kind.
	z = &Concept{Type: "BigQuery Table"}
	if got := z.GetKind(); got != "note" {
		t.Errorf("GetKind = %q, want the note default", got)
	}
}

// --- lenient reads ---

// A field with the wrong shape costs only that field on read. Failing the whole
// parse made the concept vanish from search, the graph, the listings and id
// lookup, so it could not be found to be fixed; validation stays strict.
func TestParseFrontmatterToleratesABadField(t *testing.T) {
	for name, extra := range map[string]string{
		"scalar tags":  "tags: foo\n",
		"bad created":  "x-okfw-created: \"Jan 15 2024\"\n",
		"list for x":   "x: [1, 2]\n",
		"map for type": "status: {a: 1}\n",
	} {
		content := []byte("---\nx-okfw-id: \"019680f0-7e00-7abc-8def-1234567890ab\"\ntitle: \"Kept\"\ntype: \"note\"\n" + extra + "---\n\nbody\n")
		z, err := ParseFrontmatter(content)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if z.Title != "Kept" || z.ID == "" || z.Type != "note" {
			t.Errorf("%s: the good fields were lost: %+v", name, z)
		}
		if _, err := ParseAndValidate(content); err == nil {
			t.Errorf("%s: strict validation accepted it", name)
		}
	}
	for name, content := range map[string]string{
		"not a mapping": "---\n- a\n- b\n---\n",
		"bad yaml":      "---\ntitle: [unclosed\n---\n",
	} {
		if _, err := ParseFrontmatter([]byte(content)); err == nil {
			t.Errorf("%s: parsed without error", name)
		}
	}
}

func TestConceptKey(t *testing.T) {
	if got := ConceptKey(&Concept{ID: "abc"}, "dir/x.md"); got != "abc" {
		t.Errorf("with an id: %q", got)
	}
	if got := ConceptKey(&Concept{}, "dir/x.MD"); got != "dir/x" {
		t.Errorf("without an id: %q", got)
	}
}

// --- Attested Computation (OKF section 10) ---

// The closed schema rejected every section 10 key, so okfw could not hold an
// Attested Computation concept at all; x: cannot carry spec-defined keys.
func TestAttestedComputationKeysValidate(t *testing.T) {
	content, err := os.ReadFile("../../cli/testdata/valid_concept_attested.md")
	if err != nil {
		t.Fatal(err)
	}
	z, err := ParseAndValidate(content)
	if err != nil {
		t.Fatalf("the spec's own example does not validate: %v", err)
	}
	if z.Runtime != "bigquery" {
		t.Errorf("runtime = %q", z.Runtime)
	}

	withoutRuntime := strings.Replace(string(content), "runtime: bigquery\n", "", 1)
	if _, err := ParseAndValidate([]byte(withoutRuntime)); err == nil || !strings.Contains(err.Error(), "requires runtime") {
		t.Errorf("an Attested Computation without runtime: err = %v", err)
	}
	for name, bad := range map[string]string{
		"parameter without type": "parameters:\n  - { name: year }\n",
		"unknown executor key":   "executor:\n  resource: x.md\n  shell: true\n",
		"receipt not a list":     "executor:\n  receipt: job_id\n",
	} {
		if _, err := ParseAndValidate(concept(bad)); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

// OKF v0.3 section 4.1: extension keys may appear at any level, and x- is the
// namespace the spec promises never to claim. The closed schema admits them
// everywhere, and still rejects any other unknown key.
func TestExtensionKeysAtEveryLevel(t *testing.T) {
	for name, raw := range map[string]string{
		"top level":    "x-acme-owner: \"fpa\"\n",
		"generated":    "generated:\n  by: \"a/1\"\n  x-acme-run: 7\n",
		"verified":     "verified:\n  - by: \"human:r\"\n    at: \"2026-07-02T00:00:00Z\"\n    x-acme-ticket: \"T-1\"\n",
		"source":       "sources:\n  - resource: \"x\"\n    x-acme-weight: 0.5\n",
		"usage_window": "usage_window:\n  from: \"2026-01-01\"\n  x-acme-note: \"q1\"\n",
		"executor":     "runtime: \"bigquery\"\nexecutor:\n  resource: \"/r.md\"\n  x-acme-region: \"eu\"\n",
		"legacy x:":    "x:\n  anything: [1, 2]\n",
	} {
		if _, err := ParseAndValidate(concept(raw)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, raw := range map[string]string{
		"top level":   "owner: \"fpa\"\n",
		"in a source": "sources:\n  - resource: \"x\"\n    weight: 0.5\n",
	} {
		if _, err := ParseAndValidate(concept(raw)); err == nil {
			t.Errorf("%s: an unknown non-x- key was accepted", name)
		}
	}
}

// OKF v0.3 section 4: producers MUST NOT write duplicate keys. yaml.v3 lets a
// struct decode keep the last one silently, so okfw checks the node tree.
func TestDuplicateKeysAreRejected(t *testing.T) {
	for _, raw := range []string{
		"description: \"a\"\ndescription: \"b\"\n",
		"generated:\n  by: \"a/1\"\n  by: \"b/2\"\n",
	} {
		if _, err := ParseAndValidate(concept(raw)); err == nil || !strings.Contains(err.Error(), "duplicate") {
			t.Errorf("%q: err = %v, want a duplicate-key error", raw, err)
		}
	}
}

// OKF v0.3 section 11.2: one item stands for a one-item list, a scalar tags is
// one tag, and null or empty means absent.
func TestSingleItemAndEmptyValues(t *testing.T) {
	z, err := ParseFrontmatter([]byte("---\ntype: T\ntags: a, b\nsources:\n  resource: \"x\"\nstale_after: \"\"\nverified: ~\n---\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(z.Tags) != 1 || z.Tags[0] != "a, b" {
		t.Errorf("tags = %q, want one tag, unsplit", z.Tags)
	}
	if len(z.Sources) != 1 || z.Sources[0].Resource != "x" {
		t.Errorf("sources = %+v, want one entry", z.Sources)
	}
	if z.IsStale(time.Now()) || z.Verified != nil {
		t.Errorf("empty values should read as absent: %+v", z)
	}
}

func TestUnknownStatusIsNotCurrent(t *testing.T) {
	z, err := ParseFrontmatter(concept("status: \"retired\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if z.LifecycleStatus() != "retired" || z.IsCurrent() {
		t.Errorf("status %q current=%v; an unknown status is kept raw and is not current", z.LifecycleStatus(), z.IsCurrent())
	}
	if w := ConceptWarnings(z); len(w) == 0 {
		t.Error("an unknown status should be warned about")
	}
}

func TestLegacyPathFallbackByVersion(t *testing.T) {
	for v, want := range map[string]bool{"": true, "0.1": true, "0.2": true, "0.3": false, "1.0": false, "garbage": false} {
		if got := LegacyPathFallback(v); got != want {
			t.Errorf("LegacyPathFallback(%q) = %v, want %v", v, got, want)
		}
	}
}
