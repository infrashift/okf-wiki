package search

import (
	"strings"
	"testing"
)

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
