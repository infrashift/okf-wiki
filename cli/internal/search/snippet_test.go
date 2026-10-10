package search

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestSnippetNeverSplitsARune: the window is cut at byte offsets, and a cut
// through a multi-byte character put invalid UTF-8 in the snippet, which JSON
// encoding then turned into U+FFFD.
func TestSnippetNeverSplitsARune(t *testing.T) {
	for _, filler := range []string{"é", "日本", "🙂", "aé"} {
		body := strings.Repeat(filler, 200) + " needle " + strings.Repeat(filler, 200)
		s := snippetFor(body, []string{"needle"}, "")
		if !utf8.ValidString(s) {
			t.Errorf("filler %q: snippet is not valid UTF-8: %q", filler, s)
		}
		if !strings.Contains(s, "needle") {
			t.Errorf("filler %q: snippet %q lost the match", filler, s)
		}
	}
}

// TestSnippetFindsTheMatchWhereLoweringChangesLength: the match was located in
// strings.ToLower(body) and its offset used to slice body, but lowering changes
// the byte length of some runes. Ⱥ (2 bytes) lowers to ⱥ (3), so the offset ran
// past the end of body and slicing panicked; İ (2 bytes) lowers to i (1), so the
// window landed short of the match.
func TestSnippetFindsTheMatchWhereLoweringChangesLength(t *testing.T) {
	for _, filler := range []string{"Ⱥ", "İ", "K", "\xff"} {
		body := strings.Repeat(filler, 300) + " needle tail"
		var s string
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("filler %q: snippetFor panicked: %v", filler, r)
				}
			}()
			s = snippetFor(body, []string{"needle"}, "")
		}()
		if !strings.Contains(s, "needle") {
			t.Errorf("filler %q: snippet %q does not contain the match", filler, s)
		}
	}
	// Case-insensitive still: the match is found in the original's casing.
	if s := snippetFor("Ⱥ NEEDLE", []string{"needle"}, ""); !strings.Contains(s, "NEEDLE") {
		t.Errorf("snippet %q lost the original casing", s)
	}
}

// TestSearchSurvivesABodyThatLowersLonger runs the panic through Search itself:
// one such concept took down `okfw search`, and every MCP search with it.
func TestSearchSurvivesABodyThatLowersLonger(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a--0001.md", concept("019680f0-7e00-7abc-8def-100000000000",
		"Alphabet", "Letters", strings.Repeat("Ⱥ", 300)+" needle\n", ""))
	hits, _, err := Search(root, Options{Query: "needle"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || !strings.Contains(hits[0].Snippet, "needle") {
		t.Fatalf("hits = %+v, want one with the match in its snippet", hits)
	}
}

// TestFormattedSnippetIsNotDoublyElided: snippetFor marks its own cuts, and the
// text formatter wrapped every snippet in a second pair of dots.
func TestFormattedSnippetIsNotDoublyElided(t *testing.T) {
	out := FormatResultsFrom([]Result{{Title: "T", ID: "x", Snippet: "...middle..."}}, 1, 0)
	if strings.Contains(out, "......") {
		t.Errorf("snippet elided twice:\n%s", out)
	}
	if !strings.Contains(out, "...middle...") {
		t.Errorf("snippet missing:\n%s", out)
	}
}
