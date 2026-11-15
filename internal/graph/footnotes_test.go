package graph

import (
	"reflect"
	"testing"
)

func TestFootnotesFindsReferencesAndDefinitions(t *testing.T) {
	body := "Sharded daily.[^ga4-schema] Renewed yearly.[^Spec] Again.[^ga4-schema]\n" +
		"\n" +
		"[^ga4-schema]: GA4 BigQuery Export schema\n" +
		"   [^note]: indented up to three spaces is still a definition\n"
	refs, defs := Footnotes(body)
	if want := []string{"ga4-schema", "Spec"}; !reflect.DeepEqual(refs, want) {
		t.Errorf("refs = %q, want %q (deduplicated, definitions not counted)", refs, want)
	}
	if want := []string{"ga4-schema", "note"}; !reflect.DeepEqual(defs, want) {
		t.Errorf("defs = %q, want %q", defs, want)
	}
}

// TestFootnotesIgnoresWhatIsNotACitation: code and regex character classes
// written as examples look like footnotes, and treating them as citations
// would fail validation on a concept that cites nothing.
func TestFootnotesIgnoresWhatIsNotACitation(t *testing.T) {
	body := "Match `[^/]+` or ``a [^b] c`` inline.\n" +
		"```\nre := `[^a-z]`\n[^fenced]: not a definition\n```\n" +
		"~~~\n[^tilde]\n~~~\n" +
		"An escaped \\[^esc] bracket, a [^link](https://example.com), " +
		"[^has space], [^], and [^nested[x]].\n" +
		"    [^deep]: four spaces of indent: code, or a definition in a list item\n"
	refs, defs := Footnotes(body)
	if len(refs) != 0 {
		t.Errorf("refs = %q, want none", refs)
	}
	// The indented line is taken as a definition either way: the scanner cannot
	// tell indented code from a list item's definition, and a validator must
	// err towards a citation resolving rather than reject a valid concept.
	if want := []string{"deep"}; !reflect.DeepEqual(defs, want) {
		t.Errorf("defs = %q, want %q", defs, want)
	}
}

func TestFootnoteKeyMatchesCaseInsensitively(t *testing.T) {
	if FootnoteKey("GA4-Schema") != FootnoteKey("ga4-schema") {
		t.Error("[^GA4-Schema] and [^ga4-schema] are one footnote in markdown")
	}
}
